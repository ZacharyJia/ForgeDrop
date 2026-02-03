package deploy

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"forge-drop/internal/compose"
	"forge-drop/internal/db"
)

type Options struct {
	DataDir string
	Store   *db.Store
	Logger  *log.Logger
}

type Deployer struct {
	dataDir        string
	store          *db.Store
	logger         *log.Logger
	composeManager *compose.ComposeManager
}

func New(opts Options) *Deployer {
	return &Deployer{
		dataDir:        opts.DataDir,
		store:          opts.Store,
		logger:         opts.Logger,
		composeManager: compose.NewManager(opts.DataDir),
	}
}

func (d *Deployer) Close() error {
	return nil
}

func (d *Deployer) ApplyEnv(ctx context.Context, envID string) error {
	env, err := d.store.GetEnvByID(ctx, envID)
	if err != nil {
		return err
	}
	services, err := d.store.ListServicesByApp(ctx, env.AppID)
	if err != nil {
		return err
	}
	for _, svc := range services {
		if !svc.Enabled {
			continue
		}
		if err := d.ApplyService(ctx, envID, svc.ID); err != nil {
			return fmt.Errorf("service %s: %w", svc.ServiceKey, err)
		}
	}
	return nil
}

func (d *Deployer) ApplyService(ctx context.Context, envID, serviceID string) error {
	env, err := d.store.GetEnvByID(ctx, envID)
	if err != nil {
		return err
	}
	app, err := d.store.GetAppByID(ctx, env.AppID)
	if err != nil {
		return err
	}
	svc, err := d.store.GetServiceByID(ctx, serviceID)
	if err != nil {
		return err
	}
	if !svc.Enabled {
		return fmt.Errorf("service disabled")
	}

	curSnap, err := d.store.GetEnvCurrentSnapshotID(ctx, envID)
	if err != nil {
		return err
	}
	if curSnap == nil {
		return fmt.Errorf("no snapshot yet for env")
	}

	slots, err := d.store.ListSlotsByService(ctx, serviceID)
	if err != nil {
		return err
	}
	artBySlotKey, err := d.store.GetSnapshotSlotArtifacts(ctx, *curSnap, serviceID)
	if err != nil {
		return err
	}

	artifactPaths := make(map[string]string) // slot_key -> host_path for compose templates
	slotPaths := make(map[string]string)     // slot_key -> container_path (from slot)
	for _, sl := range slots {
		a, ok := artBySlotKey[sl.SlotKey]
		if !ok {
			continue // allow missing slots (newly added or not yet uploaded)
		}
		hostPath := d.runtimeSlotFile(envID, serviceID, sl.SlotKey)
		if err := d.materializeFile(hostPath, a.StoredPath); err != nil {
			return fmt.Errorf("slot %s: %w", sl.SlotKey, err)
		}
		artifactPaths[sl.SlotKey] = hostPath
		slotPaths[sl.SlotKey] = sl.ContainerPath
	}
	if len(artifactPaths) == 0 {
		return fmt.Errorf("no artifacts available for this service in current snapshot")
	}

	// Compose-only mode.
	if strings.TrimSpace(svc.ComposeTemplate) == "" {
		return fmt.Errorf("compose template is empty; please configure Docker Compose template first")
	}
	return d.applyServiceWithCompose(ctx, env, app, svc, artifactPaths, slotPaths)
}

func (d *Deployer) applyServiceWithCompose(ctx context.Context, env *db.Env, app *db.App, svc *db.Service, artifactPaths map[string]string, slotPaths map[string]string) error {
	networkName, _ := d.store.GetSetting(ctx, "docker_network")
	hostTpl, _ := d.store.GetSetting(ctx, "preview_host_template")
	baseDomain, _ := d.store.GetSetting(ctx, "base_domain")

	// Determine host
	hostRule := ""
	if env.Kind == "preview" {
		repoSlug := ""
		if env.RepoSlug != nil {
			repoSlug = *env.RepoSlug
		}
		pr := 0
		if env.PRNumber != nil {
			pr = *env.PRNumber
		}
		host := renderHostTemplate(hostTpl, app.AppKey, repoSlug, pr, svc.ServiceKey, baseDomain)
		if host != "" {
			hostRule = host
		}
	} else if env.Kind == "named" && env.Name == "prod" && strings.TrimSpace(svc.ProdHost) != "" {
		hostRule = strings.TrimSpace(svc.ProdHost)
	}

	// Build template data
	templateData := compose.BuildTemplateData(
		svc, env, app,
		artifactPaths,
		slotPaths,
		hostRule,
		networkName,
		baseDomain,
		d.dataDir,
	)

	// Render compose template
	rendered, err := compose.RenderTemplate(svc.ComposeTemplate, templateData)
	if err != nil {
		return fmt.Errorf("render compose template: %w", err)
	}

	// Write compose file
	if err := d.composeManager.WriteComposeFile(env.ID, svc.ID, rendered); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}

	// Deploy with docker compose
	if err := d.composeManager.Up(ctx, env.ID, svc.ID, svc.ServiceKey); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	d.logf("Deployed service %s (env=%s) using Docker Compose", svc.ServiceKey, env.Name)
	return nil
}

func (d *Deployer) logf(format string, args ...any) {
	if d.logger != nil {
		d.logger.Printf(format, args...)
	}
}

func (d *Deployer) RecreateService(ctx context.Context, envID, serviceID string) error {
	svc, err := d.store.GetServiceByID(ctx, serviceID)
	if err != nil {
		return err
	}
	_ = d.composeManager.Down(ctx, envID, svc.ID, svc.ServiceKey)
	return d.ApplyService(ctx, envID, serviceID)
}

func (d *Deployer) CleanupEnv(ctx context.Context, envID string) error {
	// Get all services for this env to clean up compose projects
	env, err := d.store.GetEnvByID(ctx, envID)
	if err == nil {
		services, err := d.store.ListServicesByApp(ctx, env.AppID)
		if err == nil {
			for _, svc := range services {
				_ = d.composeManager.Down(ctx, envID, svc.ID, svc.ServiceKey)
			}
		}
	}

	// runtime dir
	rt := filepath.Join(d.dataDir, "runtime", "env-"+envID)
	_ = os.RemoveAll(rt)
	return nil
}

func (d *Deployer) ServiceURL(ctx context.Context, envID, serviceID string) (string, bool) {
	env, err := d.store.GetEnvByID(ctx, envID)
	if err != nil {
		return "", false
	}
	app, err := d.store.GetAppByID(ctx, env.AppID)
	if err != nil {
		return "", false
	}
	svc, err := d.store.GetServiceByID(ctx, serviceID)
	if err != nil {
		return "", false
	}
	baseDomain, _ := d.store.GetSetting(ctx, "base_domain")
	tpl, _ := d.store.GetSetting(ctx, "preview_host_template")

	host := ""
	if env.Kind == "preview" {
		repoSlug := ""
		if env.RepoSlug != nil {
			repoSlug = *env.RepoSlug
		}
		pr := 0
		if env.PRNumber != nil {
			pr = *env.PRNumber
		}
		host = renderHostTemplate(tpl, app.AppKey, repoSlug, pr, svc.ServiceKey, baseDomain)
	} else if env.Kind == "named" && env.Name == "prod" && strings.TrimSpace(svc.ProdHost) != "" {
		host = strings.TrimSpace(svc.ProdHost)
	}
	if host == "" {
		return "", false
	}
	return "https://" + host, true
}

func (d *Deployer) ServiceStatus(ctx context.Context, serviceID string) (map[string]any, error) {
	// Compose-only: return useful metadata (best-effort).
	// Runtime compose file is per env+service, so this endpoint is informational only.
	return map[string]any{
		"mode": "compose",
		"note": "status is environment-scoped; use env-specific redeploy and compose logs/ps",
	}, nil
}

func (d *Deployer) runtimeSlotFile(envID, serviceID, slotKey string) string {
	return filepath.Join(d.dataDir, "runtime", "env-"+envID, "service-"+serviceID, "slots", slotKey, "file")
}

func (d *Deployer) materializeFile(dstPath, srcPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	tmp := dstPath + ".tmp"
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dstPath)
}

func renderHostTemplate(tpl, appKey, repoSlug string, pr int, serviceKey, baseDomain string) string {
	tpl = strings.TrimSpace(tpl)
	if tpl == "" {
		tpl = "pr-{app}-{repoSlug}-{pr}-{service}.{base_domain}"
	}
	replacer := strings.NewReplacer(
		"{app}", appKey,
		"{repoSlug}", repoSlug,
		"{pr}", fmt.Sprintf("%d", pr),
		"{service}", serviceKey,
		"{base_domain}", baseDomain,
	)
	host := replacer.Replace(tpl)
	host = strings.ReplaceAll(host, "..", ".")
	host = strings.Trim(host, ".")
	return host
}
