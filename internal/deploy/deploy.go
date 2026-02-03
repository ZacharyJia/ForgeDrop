package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"

	"forge-drop/internal/db"
	"forge-drop/internal/dockerx"
)

type Options struct {
	DataDir string
	Store   *db.Store
	Docker  *dockerx.Client
	Logger  *log.Logger
}

type Deployer struct {
	dataDir string
	store   *db.Store
	docker  *dockerx.Client
	logger  *log.Logger
}

type mount struct {
	slotKey       string
	hostPath      string
	containerPath string
	artifact      db.Artifact
}

func New(opts Options) *Deployer {
	return &Deployer{
		dataDir: opts.DataDir,
		store:   opts.Store,
		docker:  opts.Docker,
		logger:  opts.Logger,
	}
}

func (d *Deployer) Close() error {
	if d.docker != nil {
		return d.docker.Close()
	}
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

	var mounts []mount
	for _, sl := range slots {
		a, ok := artBySlotKey[sl.SlotKey]
		if !ok {
			continue // allow missing slots (newly added or not yet uploaded)
		}
		hostPath := d.runtimeSlotFile(envID, serviceID, sl.SlotKey)
		mounts = append(mounts, mount{
			slotKey:       sl.SlotKey,
			hostPath:      hostPath,
			containerPath: sl.ContainerPath,
			artifact:      a,
		})
	}
	if len(mounts) == 0 {
		return fmt.Errorf("no artifacts available for this service in current snapshot")
	}

	// 1) materialize runtime files
	for _, m := range mounts {
		if err := d.materializeFile(m.hostPath, m.artifact.StoredPath); err != nil {
			return fmt.Errorf("slot %s: %w", m.slotKey, err)
		}
	}

	// 2) ensure container (docker optional)
	if d.docker == nil || !d.docker.Enabled() {
		return nil
	}

	networkName, _ := d.store.GetSetting(ctx, "docker_network")
	hostTpl, _ := d.store.GetSetting(ctx, "preview_host_template")
	baseDomain, _ := d.store.GetSetting(ctx, "base_domain")

	labels := baseLabels(app, env, svc)
	labels["forge-drop.mount_sig"] = mountSig(mounts)

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

	traefikLabels := map[string]string{}
	if hostRule != "" {
		routerName := fmt.Sprintf("fd-%s-%s", env.ID, svc.ServiceKey)
		serviceName := fmt.Sprintf("fd-%s-%s", env.ID, svc.ServiceKey)
		traefikLabels["traefik.enable"] = "true"
		traefikLabels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", hostRule)
		entrypoints := strings.TrimSpace(svc.TraefikEntrypnts)
		if entrypoints == "" {
			entrypoints = "websecure"
		}
		traefikLabels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = entrypoints
		traefikLabels[fmt.Sprintf("traefik.http.routers.%s.tls", routerName)] = "true"
		traefikLabels[fmt.Sprintf("traefik.http.routers.%s.service", routerName)] = serviceName
		traefikLabels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName)] = fmt.Sprintf("%d", svc.ContainerPort)
	}
	for k, v := range traefikLabels {
		labels[k] = v
	}

	name := containerName(env.ID, svc.ServiceKey)
	existingID, existingLabels, err := d.findContainerByNameOrLabels(ctx, name, map[string]string{
		"forge-drop.env_id":     env.ID,
		"forge-drop.service_id": svc.ID,
	})
	if err != nil {
		return err
	}

	needRecreate := false
	if existingID == "" {
		needRecreate = true
	} else {
		if existingLabels["forge-drop.service_revision"] != fmt.Sprintf("%d", svc.Revision) {
			needRecreate = true
		}
		if existingLabels["forge-drop.mount_sig"] != labels["forge-drop.mount_sig"] {
			needRecreate = true
		}
	}

	binds := make([]string, 0, len(mounts))
	for _, m := range mounts {
		binds = append(binds, fmt.Sprintf("%s:%s:ro", m.hostPath, m.containerPath))
	}

	cfg := container.Config{
		Image:  svc.Image,
		Cmd:    []string{"sh", "-lc", svc.Command},
		Labels: labels,
		Env:    envList(svc.Env),
		User:   svc.RunUser,
	}
	hostCfg := container.HostConfig{
		Binds:         binds,
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
	}
	netCfg := network.NetworkingConfig{}
	if strings.TrimSpace(networkName) != "" {
		netCfg.EndpointsConfig = map[string]*network.EndpointSettings{
			networkName: {},
		}
	}

	if needRecreate && existingID != "" {
		_ = d.docker.RemoveContainer(ctx, existingID, true)
		existingID = ""
	}
	if existingID == "" {
		cfg.Labels["forge-drop.service_revision"] = fmt.Sprintf("%d", svc.Revision)
		resp, err := d.docker.CreateContainer(ctx, cfg, hostCfg, netCfg, name)
		if err != nil {
			return err
		}
		if err := d.docker.StartContainer(ctx, resp.ID); err != nil {
			return err
		}
		return nil
	}
	return d.docker.RestartContainer(ctx, existingID)
}

func (d *Deployer) RecreateService(ctx context.Context, envID, serviceID string) error {
	if d.docker == nil || !d.docker.Enabled() {
		return errors.New("docker disabled")
	}
	svc, err := d.store.GetServiceByID(ctx, serviceID)
	if err != nil {
		return err
	}
	name := containerName(envID, svc.ServiceKey)
	id, _, err := d.findContainerByNameOrLabels(ctx, name, map[string]string{
		"forge-drop.env_id":     envID,
		"forge-drop.service_id": serviceID,
	})
	if err != nil {
		return err
	}
	if id != "" {
		_ = d.docker.RemoveContainer(ctx, id, true)
	}
	return d.ApplyService(ctx, envID, serviceID)
}

func (d *Deployer) CleanupEnv(ctx context.Context, envID string) error {
	if d.docker != nil && d.docker.Enabled() {
		containers, err := d.docker.ListContainers(ctx, true, map[string]string{"forge-drop.env_id": envID})
		if err == nil {
			for _, c := range containers {
				_ = d.docker.RemoveContainer(ctx, c.ID, true)
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
	if d.docker == nil || !d.docker.Enabled() {
		return map[string]any{"docker": "disabled"}, nil
	}
	containers, err := d.docker.ListContainers(ctx, true, map[string]string{"forge-drop.service_id": serviceID})
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, c := range containers {
		out = append(out, map[string]any{
			"id":     c.ID,
			"names":  c.Names,
			"image":  c.Image,
			"state":  c.State,
			"status": c.Status,
			"labels": c.Labels,
		})
	}
	return map[string]any{"containers": out}, nil
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

func baseLabels(app *db.App, env *db.Env, svc *db.Service) map[string]string {
	labels := map[string]string{
		"forge-drop.app_id":      app.ID,
		"forge-drop.app_key":     app.AppKey,
		"forge-drop.env_id":      env.ID,
		"forge-drop.env_kind":    env.Kind,
		"forge-drop.env_name":    env.Name,
		"forge-drop.service_id":  svc.ID,
		"forge-drop.service_key": svc.ServiceKey,
		"forge-drop.managed":     "true",
	}
	if env.Kind == "preview" {
		if env.RepoFullName != nil {
			labels["forge-drop.repo_full_name"] = *env.RepoFullName
		}
		if env.RepoSlug != nil {
			labels["forge-drop.repo_slug"] = *env.RepoSlug
		}
		if env.PRNumber != nil {
			labels["forge-drop.pr_number"] = fmt.Sprintf("%d", *env.PRNumber)
		}
	}
	return labels
}

func envList(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}

func mountSig(mounts []mount) string {
	// stable signature based on mounted slot set + container path
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].slotKey < mounts[j].slotKey })
	h := sha256.New()
	for _, m := range mounts {
		_, _ = io.WriteString(h, m.slotKey)
		_, _ = io.WriteString(h, "->")
		_, _ = io.WriteString(h, m.containerPath)
		_, _ = io.WriteString(h, ";")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func containerName(envID, serviceKey string) string {
	svc := strings.ToLower(serviceKey)
	svc = strings.ReplaceAll(svc, "_", "-")
	svc = strings.ReplaceAll(svc, " ", "-")
	return fmt.Sprintf("forge-drop-env-%s-%s", strings.ToLower(envID), svc)
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

func (d *Deployer) findContainerByNameOrLabels(ctx context.Context, name string, labels map[string]string) (id string, outLabels map[string]string, err error) {
	if d.docker == nil || !d.docker.Enabled() {
		return "", nil, errors.New("docker disabled")
	}
	// First try exact name match via list all and compare.
	containers, err := d.docker.ListContainers(ctx, true, map[string]string{"forge-drop.managed": "true"})
	if err != nil {
		return "", nil, err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				return c.ID, c.Labels, nil
			}
		}
	}
	// Then try labels match.
	containers, err = d.docker.ListContainers(ctx, true, labels)
	if err != nil {
		return "", nil, err
	}
	if len(containers) == 0 {
		return "", nil, nil
	}
	// If multiple, pick the newest-ish by created? Docker list doesn't include created in types.Container? It does.
	sort.Slice(containers, func(i, j int) bool { return containers[i].Created > containers[j].Created })
	return containers[0].ID, containers[0].Labels, nil
}
