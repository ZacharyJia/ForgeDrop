package deploy

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
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
	return d.DeployEnv(ctx, envID, "")
}

func (d *Deployer) ApplyService(ctx context.Context, envID, serviceID string) error {
	return d.DeployService(ctx, envID, serviceID, "")
}

func (d *Deployer) DeployService(ctx context.Context, envID, serviceID, strategy string) error {
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

	slots, err := d.store.ListSlotsByService(ctx, serviceID)
	if err != nil {
		return err
	}
	artBySlotKey, _, err := d.store.GetEffectiveSlotArtifacts(ctx, envID, serviceID)
	if err != nil {
		return err
	}

	artifactPaths := make(map[string]string) // slot_key -> host_path for compose templates
	slotPaths := make(map[string]string)     // slot_key -> container_path (from slot)
	for _, sl := range slots {
		a, ok := artBySlotKey[sl.SlotKey]
		if !ok {
			continue // fallback exhausted; keep empty
		}
		hostPath := d.runtimeSlotPath(envID, serviceID, sl.SlotKey, sl.MountType)
		if err := d.materializeMount(hostPath, a.StoredPath, sl.MountType); err != nil {
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
	strategy = strings.TrimSpace(strings.ToLower(strategy))
	if strategy == "" {
		strategy = strings.TrimSpace(strings.ToLower(svc.DeployStrategy))
	}
	if strategy != "recreate" && strategy != "restart" {
		strategy = "recreate"
	}
	return d.applyServiceWithCompose(ctx, env, app, svc, artifactPaths, slotPaths, strategy)
}

func (d *Deployer) applyServiceWithCompose(ctx context.Context, env *db.Env, app *db.App, svc *db.Service, artifactPaths map[string]string, slotPaths map[string]string, strategy string) error {
	networkName, _ := d.store.GetSetting(ctx, "docker_network")
	namedHostTpl, _ := d.store.GetSetting(ctx, "named_host_template")
	hostTpl, _ := d.store.GetSetting(ctx, "preview_host_template")
	baseDomain, _ := d.store.GetSetting(ctx, "base_domain")

	// Determine host
	hostRule := ""
	if env.Kind == "preview" {
		repoSlug := ""
		if env.RepoSlug != nil {
			repoSlug = *env.RepoSlug
		}
		changeSet := ""
		if env.ChangeSet != nil {
			changeSet = strings.TrimSpace(*env.ChangeSet)
		}
		pr := 0
		if env.PRNumber != nil {
			pr = *env.PRNumber
		} else if changeSet != "" {
			pr = syntheticPreviewNumber(changeSet)
		}
		host := renderHostTemplate(hostTpl, app.AppKey, repoSlug, pr, changeSet, svc.ServiceKey, baseDomain)
		if host != "" {
			hostRule = host
		}
	} else if env.Kind == "named" {
		// For named envs, always derive a host (prod host is just an override).
		if env.Name == "prod" && strings.TrimSpace(svc.ProdHost) != "" {
			hostRule = strings.TrimSpace(svc.ProdHost)
		} else {
			host := renderNamedHostTemplate(namedHostTpl, app.AppKey, env.Name, svc.ServiceKey, baseDomain)
			if host != "" {
				hostRule = host
			}
		}
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

	// For deterministic deployments, default to down+up. Restart is a fast path
	// for pure file updates (e.g. jar/config bind mounts).
	if strategy == "recreate" {
		_ = d.composeManager.Down(ctx, env.ID, svc.ID, svc.ServiceKey)
	}

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
	if strategy == "restart" {
		if err := d.composeManager.Restart(ctx, env.ID, svc.ID, svc.ServiceKey); err != nil {
			return fmt.Errorf("docker compose restart: %w", err)
		}
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
	return d.DeployService(ctx, envID, serviceID, "recreate")
}

func (d *Deployer) DeployEnv(ctx context.Context, envID, strategy string) error {
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
		if err := d.DeployService(ctx, envID, svc.ID, strategy); err != nil {
			return fmt.Errorf("service %s: %w", svc.ServiceKey, err)
		}
	}
	return nil
}

// StopEnv stops (but does not remove) the Docker Compose containers of every
// enabled service in the environment. Deploying the env again brings them back.
func (d *Deployer) StopEnv(ctx context.Context, envID string) error {
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
		if err := d.composeManager.Stop(ctx, envID, svc.ID, svc.ServiceKey); err != nil {
			return fmt.Errorf("service %s: %w", svc.ServiceKey, err)
		}
	}
	d.logf("Stopped env %s (id=%s)", env.Name, envID)
	return nil
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
	namedTpl, _ := d.store.GetSetting(ctx, "named_host_template")
	tpl, _ := d.store.GetSetting(ctx, "preview_host_template")

	host := ""
	if env.Kind == "preview" {
		repoSlug := ""
		if env.RepoSlug != nil {
			repoSlug = *env.RepoSlug
		}
		changeSet := ""
		if env.ChangeSet != nil {
			changeSet = strings.TrimSpace(*env.ChangeSet)
		}
		pr := 0
		if env.PRNumber != nil {
			pr = *env.PRNumber
		} else if changeSet != "" {
			pr = syntheticPreviewNumber(changeSet)
		}
		host = renderHostTemplate(tpl, app.AppKey, repoSlug, pr, changeSet, svc.ServiceKey, baseDomain)
	} else if env.Kind == "named" {
		if env.Name == "prod" && strings.TrimSpace(svc.ProdHost) != "" {
			host = strings.TrimSpace(svc.ProdHost)
		} else {
			host = renderNamedHostTemplate(namedTpl, app.AppKey, env.Name, svc.ServiceKey, baseDomain)
		}
	}
	if host == "" {
		return "", false
	}
	return "https://" + host, true
}

func renderNamedHostTemplate(tpl, appKey, envName, serviceKey, baseDomain string) string {
	tpl = strings.TrimSpace(tpl)
	if tpl == "" {
		// Default to a stable, readable host for any named env.
		// Example: myapp-api-staging.example.com
		tpl = "{app}-{service}-{env}.{base_domain}"
	}
	envSlug := slugDNSLabel(envName)
	replacer := strings.NewReplacer(
		"{app}", appKey,
		"{env}", envSlug,
		"{service}", serviceKey,
		"{base_domain}", baseDomain,
	)
	host := replacer.Replace(tpl)
	host = strings.ReplaceAll(host, "..", ".")
	host = strings.Trim(host, ".")
	return host
}

func slugDNSLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "env"
	}
	// Keep ASCII letters/digits, convert everything else to '-'.
	var b strings.Builder
	lastDash := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if ok {
			b.WriteByte(c)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "env"
	}
	return out
}

func (d *Deployer) ServiceStatus(ctx context.Context, envID, serviceID string) (map[string]any, error) {
	svc, err := d.store.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"mode":        "compose",
		"env_id":      envID,
		"service_id":  serviceID,
		"service_key": svc.ServiceKey,
	}
	if strings.TrimSpace(envID) == "" {
		out["note"] = "env_id required for environment-scoped status (compose ps/logs)"
		return out, nil
	}

	env, err := d.store.GetEnvByID(ctx, envID)
	if err != nil {
		return nil, err
	}
	if env.AppID != svc.AppID {
		return nil, fmt.Errorf("env does not belong to this service's app")
	}

	curSnap, _ := d.store.GetEnvCurrentSnapshotID(ctx, envID)
	out["desired_snapshot_id"] = curSnap

	composeFile := d.composeManager.GetComposeFile(envID, serviceID)
	projectName := d.composeManager.GetProjectName(envID, svc.ServiceKey)
	out["compose_file"] = composeFile
	out["project_name"] = projectName

	if _, err := os.Stat(composeFile); err != nil {
		if os.IsNotExist(err) {
			out["deployed"] = false
			out["note"] = "compose file not found; service may not have been deployed yet"
			return out, nil
		}
		return nil, err
	}
	out["deployed"] = true

	if ps, err := d.composeManager.Ps(ctx, envID, serviceID, svc.ServiceKey); err != nil {
		out["ps_error"] = err.Error()
	} else {
		out["ps"] = ps
	}

	if url, ok := d.ServiceURL(ctx, envID, serviceID); ok {
		out["service_url"] = url
	}
	return out, nil
}

func (d *Deployer) ServiceLogs(ctx context.Context, envID, serviceID string, tail int) (string, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return "", fmt.Errorf("env_id required")
	}
	if tail <= 0 {
		tail = 200
	}
	if tail > 5000 {
		tail = 5000
	}
	svc, err := d.store.GetServiceByID(ctx, serviceID)
	if err != nil {
		return "", err
	}
	env, err := d.store.GetEnvByID(ctx, envID)
	if err != nil {
		return "", err
	}
	if env.AppID != svc.AppID {
		return "", fmt.Errorf("env does not belong to this service's app")
	}
	return d.composeManager.LogsOutput(ctx, envID, serviceID, svc.ServiceKey, tail)
}

func (d *Deployer) runtimeSlotPath(envID, serviceID, slotKey, mountType string) string {
	if strings.TrimSpace(strings.ToLower(mountType)) == "dir" {
		return filepath.Join(d.dataDir, "runtime", "env-"+envID, "service-"+serviceID, "slots", slotKey, "dir")
	}
	return filepath.Join(d.dataDir, "runtime", "env-"+envID, "service-"+serviceID, "slots", slotKey, "file")
}

func (d *Deployer) materializeMount(dstPath, srcPath, mountType string) error {
	if strings.TrimSpace(strings.ToLower(mountType)) == "dir" {
		return materializeDir(dstPath, srcPath)
	}
	return materializeFile(dstPath, srcPath)
}

func materializeFile(dstPath, srcPath string) error {
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

func materializeDir(dstDir, archivePath string) error {
	tmpDir := dstDir + ".tmp"
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}

	lower := strings.ToLower(strings.TrimSpace(archivePath))
	switch {
	case strings.HasSuffix(lower, ".zip"):
		if err := extractZIP(archivePath, tmpDir); err != nil {
			_ = os.RemoveAll(tmpDir)
			return err
		}
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		if err := extractTarGZ(archivePath, tmpDir); err != nil {
			_ = os.RemoveAll(tmpDir)
			return err
		}
	case strings.HasSuffix(lower, ".tar"):
		if err := extractTar(archivePath, tmpDir); err != nil {
			_ = os.RemoveAll(tmpDir)
			return err
		}
	default:
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("unsupported dir artifact format: %s", filepath.Base(archivePath))
	}

	_ = os.RemoveAll(dstDir)
	return os.Rename(tmpDir, dstDir)
}

func safeJoinExtractPath(root, name string) (string, error) {
	clean := filepath.Clean(name)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("invalid archive entry")
	}
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("unsafe archive entry: %s", name)
	}
	dst := filepath.Join(root, clean)
	prefix := root + string(os.PathSeparator)
	if dst != root && !strings.HasPrefix(dst, prefix) {
		return "", fmt.Errorf("unsafe archive entry: %s", name)
	}
	return dst, nil
}

func extractZIP(srcPath, dstDir string) error {
	r, err := zip.OpenReader(srcPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target, err := safeJoinExtractPath(dstDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = rc.Close()
			_ = out.Close()
			return err
		}
		if err := rc.Close(); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

func extractTar(srcPath, dstDir string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return extractTarReader(tar.NewReader(f), dstDir)
}

func extractTarGZ(srcPath, dstDir string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()
	return extractTarReader(tar.NewReader(gzr), dstDir)
}

func extractTarReader(tr *tar.Reader, dstDir string) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoinExtractPath(dstDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if hdr.Mode > 0 {
				mode = os.FileMode(hdr.Mode & 0o777)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// Ignore symlinks and other special file types for safety.
			continue
		}
	}
}

func renderHostTemplate(tpl, appKey, repoSlug string, pr int, changeSet, serviceKey, baseDomain string) string {
	tpl = strings.TrimSpace(tpl)
	if tpl == "" {
		if strings.TrimSpace(changeSet) != "" {
			tpl = "cs-{app}-{repoSlug}-{change_set}-{service}.{base_domain}"
		} else {
			tpl = "pr-{app}-{repoSlug}-{pr}-{service}.{base_domain}"
		}
	}
	changeSet = slugDNSLabel(changeSet)
	replacer := strings.NewReplacer(
		"{app}", appKey,
		"{repoSlug}", repoSlug,
		"{pr}", fmt.Sprintf("%d", pr),
		"{change_set}", changeSet,
		"{service}", serviceKey,
		"{base_domain}", baseDomain,
	)
	host := replacer.Replace(tpl)
	host = strings.ReplaceAll(host, "..", ".")
	host = strings.Trim(host, ".")
	return host
}

func syntheticPreviewNumber(changeSet string) int {
	changeSet = strings.TrimSpace(changeSet)
	if changeSet == "" {
		return 0
	}
	var sum uint32
	for i := 0; i < len(changeSet); i++ {
		sum = sum*33 + uint32(changeSet[i])
	}
	// Keep it positive and reasonably short for host templates that still use {pr}.
	return int(sum%900000 + 100000)
}
