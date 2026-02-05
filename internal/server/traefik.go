package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"forge-drop/internal/httpx"
)

const managedTraefikContainerName = "forge-drop-traefik"
const managedTraefikLabelKey = "com.forge-drop.managed"

type traefikStatus struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`

	ACMEMode               string `json:"acme_mode"`
	AlicloudCredentialsSet bool   `json:"alicloud_credentials_set"`

	NetworkName  string `json:"network_name"`
	NetworkExist bool   `json:"network_exists"`

	ContainerName   string `json:"container_name"`
	ContainerExists bool   `json:"container_exists"`
	Managed         bool   `json:"managed"`
	Running         bool   `json:"running"`
	OnNetwork       bool   `json:"on_network"`
	Ports80         bool   `json:"ports_80"`
	Ports443        bool   `json:"ports_443"`
	DockerSockMount bool   `json:"docker_sock_mount"`

	DashboardEnabled bool   `json:"dashboard_enabled"`
	DashboardHost    string `json:"dashboard_host"`
	DashboardURL     string `json:"dashboard_url"`
}

type traefikInstallRequest struct {
	Staging         bool   `json:"staging"`
	EnableDashboard bool   `json:"enable_dashboard"`
	DashboardHost   string `json:"dashboard_host"`
}

func (s *Server) handleAdminTraefik(w http.ResponseWriter, r *http.Request, rest string) {
	rest = strings.TrimPrefix(rest, "/")
	// /traefik/status
	if r.Method == "GET" && (rest == "status" || rest == "/status") {
		st, err := s.getTraefikStatus(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, st)
		return
	}

	// /traefik/install
	if r.Method == "POST" && (rest == "install" || rest == "/install") {
		var req traefikInstallRequest
		_ = httpx.ReadJSON(w, r, &req, 1<<20)
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
		defer cancel()
		st, err := s.installOrRepairTraefik(ctx, req)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, st)
		return
	}

	// /traefik/credentials
	if r.Method == "POST" && (rest == "credentials" || rest == "/credentials") {
		var req struct {
			AccessKey string `json:"alicloud_access_key"`
			SecretKey string `json:"alicloud_secret_key"`
		}
		if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.AccessKey = strings.TrimSpace(req.AccessKey)
		req.SecretKey = strings.TrimSpace(req.SecretKey)
		if req.AccessKey == "" || req.SecretKey == "" {
			httpx.WriteError(w, http.StatusBadRequest, "alicloud_access_key/alicloud_secret_key required")
			return
		}
		secretsDir := filepath.Join(s.opts.DataDir, "traefik", "secrets")
		if err := os.MkdirAll(secretsDir, 0o755); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "mkdir failed")
			return
		}
		akPath := filepath.Join(secretsDir, "ALICLOUD_ACCESS_KEY")
		skPath := filepath.Join(secretsDir, "ALICLOUD_SECRET_KEY")
		if err := os.WriteFile(akPath, []byte(req.AccessKey), 0o600); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "write failed")
			return
		}
		if err := os.WriteFile(skPath, []byte(req.SecretKey), 0o600); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "write failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	httpx.WriteError(w, http.StatusNotFound, "not found")
}

func (s *Server) getTraefikStatus(ctx context.Context) (*traefikStatus, error) {
	// Status is best-effort but should clearly tell the user whether they can
	// expect routing/TLS to work.
	if err := dockerOK(ctx); err != nil {
		return &traefikStatus{
			OK:            false,
			Message:       err.Error(),
			NetworkName:   "",
			NetworkExist:  false,
			ContainerName: managedTraefikContainerName,
		}, nil
	}

	networkName, _ := s.store.GetSetting(ctx, "docker_network")
	networkName = strings.TrimSpace(networkName)
	if networkName == "" {
		networkName = "traefik"
	}

	acmeMode, _ := s.store.GetSetting(ctx, "traefik_acme_mode")
	acmeMode = strings.TrimSpace(acmeMode)
	if acmeMode == "" {
		acmeMode = "tls"
	}
	secretsDir := filepath.Join(s.opts.DataDir, "traefik", "secrets")
	st := &traefikStatus{
		NetworkName:            networkName,
		ContainerName:          managedTraefikContainerName,
		ACMEMode:               acmeMode,
		AlicloudCredentialsSet: fileExists(filepath.Join(secretsDir, "ALICLOUD_ACCESS_KEY")) && fileExists(filepath.Join(secretsDir, "ALICLOUD_SECRET_KEY")),
	}

	st.NetworkExist = dockerNetworkExists(ctx, networkName)

	ins, err := dockerInspectContainer(ctx, managedTraefikContainerName)
	if err != nil {
		// Try to detect an existing user-managed Traefik container (common name).
		if ins2, err2 := dockerInspectContainer(ctx, "traefik"); err2 == nil {
			st.ContainerName = "traefik"
			st.ContainerExists = true
			st.Managed = false
			st.Running = ins2.State.Running
			_, onNet := ins2.NetworkSettings.Networks[networkName]
			st.OnNetwork = onNet
			st.Ports80 = len(ins2.NetworkSettings.Ports["80/tcp"]) > 0
			st.Ports443 = len(ins2.NetworkSettings.Ports["443/tcp"]) > 0
			for _, m := range ins2.Mounts {
				if m.Destination == "/var/run/docker.sock" {
					st.DockerSockMount = true
					break
				}
			}
			st.OK = st.NetworkExist && st.Running && st.OnNetwork && st.Ports80 && st.Ports443
			if st.OK {
				st.Message = "检测到现有 Traefik（非 forge-drop 管理）"
			} else {
				st.Message = "检测到 Traefik 容器，但状态可能不完整（network/端口/挂载）"
			}
			return st, nil
		}

		// container missing is not an error for status.
		st.OK = st.NetworkExist
		if st.NetworkExist {
			st.Message = "Traefik 未安装（可一键安装）"
		} else {
			st.Message = "Docker network 不存在（可一键创建并安装 Traefik）"
		}
		return st, nil
	}

	st.ContainerExists = true
	if ins.Config.Labels != nil {
		st.Managed = strings.EqualFold(ins.Config.Labels[managedTraefikLabelKey], "true")
		st.DashboardEnabled = strings.EqualFold(ins.Config.Labels["traefik.http.routers.traefik-dashboard.tls"], "true") || strings.Contains(ins.Config.Labels["traefik.http.routers.traefik-dashboard.rule"], "Host(")
		st.DashboardHost = extractHostFromRule(ins.Config.Labels["traefik.http.routers.traefik-dashboard.rule"])
		if st.DashboardHost != "" {
			st.DashboardURL = "https://" + st.DashboardHost
		}
	}
	st.Running = ins.State.Running
	_, onNet := ins.NetworkSettings.Networks[networkName]
	st.OnNetwork = onNet
	st.Ports80 = len(ins.NetworkSettings.Ports["80/tcp"]) > 0
	st.Ports443 = len(ins.NetworkSettings.Ports["443/tcp"]) > 0
	for _, m := range ins.Mounts {
		if m.Destination == "/var/run/docker.sock" {
			st.DockerSockMount = true
			break
		}
	}

	st.OK = st.NetworkExist && st.Managed && st.Running && st.OnNetwork && st.Ports80 && st.Ports443 && st.DockerSockMount
	if strings.EqualFold(st.ACMEMode, "dns-alidns") {
		st.OK = st.OK && st.AlicloudCredentialsSet
	}
	if st.OK {
		st.Message = "Traefik 已就绪"
		return st, nil
	}
	st.Message = "Traefik 状态异常（可尝试一键修复）"
	return st, nil
}

func (s *Server) installOrRepairTraefik(ctx context.Context, req traefikInstallRequest) (*traefikStatus, error) {
	// Guard: needs docker.
	if err := dockerOK(ctx); err != nil {
		return nil, err
	}

	networkName, _ := s.store.GetSetting(ctx, "docker_network")
	networkName = strings.TrimSpace(networkName)
	if networkName == "" {
		networkName = "traefik"
	}
	email, _ := s.store.GetSetting(ctx, "traefik_acme_email")
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, fmt.Errorf("traefik_acme_email 未设置，请先在设置页填写邮箱")
	}
	acmeMode, _ := s.store.GetSetting(ctx, "traefik_acme_mode")
	acmeMode = strings.TrimSpace(acmeMode)
	if acmeMode == "" {
		acmeMode = "tls"
	}
	regionID, _ := s.store.GetSetting(ctx, "traefik_alicloud_region_id")
	regionID = strings.TrimSpace(regionID)
	if regionID == "" {
		regionID = "cn-hangzhou"
	}

	baseDomain, _ := s.store.GetSetting(ctx, "base_domain")
	baseDomain = strings.TrimSpace(baseDomain)
	certResolver := "le"

	wildEnabled, _ := s.store.GetSetting(ctx, "traefik_wildcard_enabled")
	wildIncludeApex, _ := s.store.GetSetting(ctx, "traefik_wildcard_include_apex")
	wildcardOn := strings.TrimSpace(wildEnabled) == "1"
	wildcardApexOn := strings.TrimSpace(wildIncludeApex) == "1"
	if req.EnableDashboard {
		host := strings.TrimSpace(req.DashboardHost)
		if host == "" {
			if baseDomain == "" {
				return nil, fmt.Errorf("base_domain 未设置，无法生成 dashboard 域名")
			}
			host = "traefik." + baseDomain
		}
		if !strings.Contains(host, ".") {
			return nil, fmt.Errorf("dashboard_host 必须是完整域名，例如 traefik.example.com")
		}
		req.DashboardHost = host
	}
	if strings.EqualFold(acmeMode, "dns-alidns") && wildcardOn {
		if baseDomain == "" {
			return nil, fmt.Errorf("base_domain 未设置，无法申请通配符证书")
		}
	}

	// Ensure docker network exists.
	if !dockerNetworkExists(ctx, networkName) {
		if err := dockerNetworkCreate(ctx, networkName); err != nil {
			return nil, fmt.Errorf("create docker network %q: %w", networkName, err)
		}
	}

	confDir := filepath.Join(s.opts.DataDir, "traefik")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return nil, err
	}
	confPath := filepath.Join(confDir, "traefik.yml")
	acmePath := filepath.Join(confDir, "acme.json")

	resolver := "le"
	caserver := ""
	if req.Staging {
		caserver = "https://acme-staging-v02.api.letsencrypt.org/directory"
	}
	secretsDir := filepath.Join(s.opts.DataDir, "traefik", "secrets")
	accessKeyFile := filepath.Join(secretsDir, "ALICLOUD_ACCESS_KEY")
	secretKeyFile := filepath.Join(secretsDir, "ALICLOUD_SECRET_KEY")
	credsSet := fileExists(accessKeyFile) && fileExists(secretKeyFile)
	if strings.EqualFold(acmeMode, "dns-alidns") && !credsSet {
		return nil, fmt.Errorf("Aliyun DNS 凭证未配置，请先在设置页填写并保存 ALICLOUD_ACCESS_KEY/ALICLOUD_SECRET_KEY")
	}

	conf := buildTraefikYAML(traefikYAMLOptions{
		Email:       email,
		Resolver:    resolver,
		CAServer:    caserver,
		ACMEMode:    acmeMode,
		DNSProvider: "alidns",
	})
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		return nil, err
	}
	if _, err := os.Stat(acmePath); err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(acmePath, []byte("{}"), 0o600); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	_ = os.Chmod(acmePath, 0o600)

	// If container exists and is not managed, don't touch it.
	ins, err := dockerInspectContainer(ctx, managedTraefikContainerName)
	if err == nil {
		managed := false
		if ins.Config.Labels != nil {
			managed = strings.EqualFold(ins.Config.Labels[managedTraefikLabelKey], "true")
		}
		if !managed {
			return nil, fmt.Errorf("发现同名容器 %q 但不是 forge-drop 管理的，请先手动处理/改名后再一键安装", managedTraefikContainerName)
		}
		_ = dockerRemoveContainer(ctx, managedTraefikContainerName)
	}

	// Start traefik.
	image := "traefik:v3.6"
	args := []string{
		"run", "-d",
		"--name", managedTraefikContainerName,
		"--restart", "unless-stopped",
		"--label", managedTraefikLabelKey + "=true",
		// Required because exposedByDefault=false; we add our own routers via labels.
		"--label", "traefik.enable=true",
		"--network", networkName,
		"-p", "80:80",
		"-p", "443:443",
		"-v", "/var/run/docker.sock:/var/run/docker.sock:ro",
		"-v", confPath + ":/etc/traefik/traefik.yml:ro",
		"-v", acmePath + ":/data/acme.json",
		image,
	}
	if strings.EqualFold(acmeMode, "dns-alidns") {
		args = append(args[:len(args)-1],
			"-e", "ALICLOUD_ACCESS_KEY_FILE=/run/secrets/ALICLOUD_ACCESS_KEY",
			"-e", "ALICLOUD_SECRET_KEY_FILE=/run/secrets/ALICLOUD_SECRET_KEY",
			"-e", "ALICLOUD_REGION_ID="+regionID,
			"-v", accessKeyFile+":/run/secrets/ALICLOUD_ACCESS_KEY:ro",
			"-v", secretKeyFile+":/run/secrets/ALICLOUD_SECRET_KEY:ro",
			image,
		)
	}
	if req.EnableDashboard {
		host := strings.TrimSpace(req.DashboardHost)
		// Expose dashboard via internal service `api@internal`.
		args = append(args[:len(args)-1],
			"--label", "traefik.http.routers.traefik-dashboard.rule=Host(`"+host+"`)",
			"--label", "traefik.http.routers.traefik-dashboard.entrypoints=websecure",
			"--label", "traefik.http.routers.traefik-dashboard.tls=true",
			"--label", "traefik.http.routers.traefik-dashboard.tls.certresolver="+certResolver,
			"--label", "traefik.http.routers.traefik-dashboard.service=api@internal",
			image,
		)
	}

	// Seed a wildcard certificate at startup.
	// This relies on DNS-01 and avoids repeated per-host certificate issuance.
	if strings.EqualFold(acmeMode, "dns-alidns") && wildcardOn {
		seedHost := "acme-wildcard." + baseDomain
		mainDomain := "*." + baseDomain
		args = append(args[:len(args)-1],
			"--label", "traefik.http.routers.fd-acme-wildcard.rule=Host(`"+seedHost+"`)",
			"--label", "traefik.http.routers.fd-acme-wildcard.entrypoints=websecure",
			"--label", "traefik.http.routers.fd-acme-wildcard.tls=true",
			"--label", "traefik.http.routers.fd-acme-wildcard.tls.certresolver="+certResolver,
			"--label", "traefik.http.routers.fd-acme-wildcard.tls.domains[0].main="+mainDomain,
			"--label", "traefik.http.routers.fd-acme-wildcard.service=api@internal",
			image,
		)
		if wildcardApexOn {
			args = append(args[:len(args)-1],
				"--label", "traefik.http.routers.fd-acme-wildcard.tls.domains[0].main="+baseDomain,
				"--label", "traefik.http.routers.fd-acme-wildcard.tls.domains[0].sans="+mainDomain,
				image,
			)
		}
	}

	if _, err := dockerCmd(ctx, args...); err != nil {
		return nil, fmt.Errorf("start traefik: %w", err)
	}

	return s.getTraefikStatus(ctx)
}

type traefikYAMLOptions struct {
	Email       string
	Resolver    string
	CAServer    string
	ACMEMode    string // tls|dns-alidns
	DNSProvider string
}

func buildTraefikYAML(opts traefikYAMLOptions) string {
	// Keep it simple: docker provider, entrypoints web/websecure.
	// For ACME: either tlsChallenge (public) or dnsChallenge (internal).
	// Note: dashboard is enabled but not exposed unless routed.
	email := strings.ReplaceAll(strings.TrimSpace(opts.Email), "'", "")
	resolver := strings.TrimSpace(opts.Resolver)
	if resolver == "" {
		resolver = "le"
	}
	acmeMode := strings.TrimSpace(opts.ACMEMode)
	if acmeMode == "" {
		acmeMode = "tls"
	}

	var b strings.Builder
	b.WriteString("entryPoints:\n")
	b.WriteString("  web:\n")
	b.WriteString("    address: ':80'\n")
	b.WriteString("  websecure:\n")
	b.WriteString("    address: ':443'\n")
	b.WriteString("    http:\n")
	b.WriteString("      tls:\n")
	b.WriteString("        certResolver: " + resolver + "\n")
	b.WriteString("providers:\n")
	b.WriteString("  docker:\n")
	b.WriteString("    endpoint: 'unix:///var/run/docker.sock'\n")
	b.WriteString("    exposedByDefault: false\n")
	b.WriteString("api:\n")
	b.WriteString("  dashboard: true\n")
	b.WriteString("certificatesResolvers:\n")
	b.WriteString("  " + resolver + ":\n")
	b.WriteString("    acme:\n")
	b.WriteString("      email: '" + email + "'\n")
	b.WriteString("      storage: '/data/acme.json'\n")
	if strings.TrimSpace(opts.CAServer) != "" {
		b.WriteString("      caServer: '" + strings.TrimSpace(opts.CAServer) + "'\n")
	}

	if strings.EqualFold(acmeMode, "dns-alidns") {
		provider := strings.TrimSpace(opts.DNSProvider)
		if provider == "" {
			provider = "alidns"
		}
		b.WriteString("      dnsChallenge:\n")
		b.WriteString("        provider: " + provider + "\n")
		b.WriteString("        resolvers:\n")
		b.WriteString("          - '223.5.5.5:53'\n")
		b.WriteString("          - '223.6.6.6:53'\n")
		b.WriteString("        propagation:\n")
		b.WriteString("          delayBeforeChecks: 10s\n")
	} else {
		b.WriteString("      tlsChallenge: {}\n")
	}

	b.WriteString("log:\n")
	b.WriteString("  level: INFO\n")
	return b.String()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func dockerOK(ctx context.Context) error {
	_, err := dockerCmd(ctx, "version")
	if err != nil {
		return fmt.Errorf("docker 不可用：%w", err)
	}
	return nil
}

func dockerCmd(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker %s failed: %w\nOutput: %s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

func dockerNetworkExists(ctx context.Context, name string) bool {
	_, err := dockerCmd(ctx, "network", "inspect", name)
	return err == nil
}

func dockerNetworkCreate(ctx context.Context, name string) error {
	_, err := dockerCmd(ctx, "network", "create", name)
	return err
}

func dockerRemoveContainer(ctx context.Context, name string) error {
	_, err := dockerCmd(ctx, "rm", "-f", name)
	return err
}

type dockerContainerInspect struct {
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool   `json:"Running"`
		Status  string `json:"Status"`
	} `json:"State"`
	NetworkSettings struct {
		Ports    map[string][]any           `json:"Ports"`
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
	Mounts []struct {
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

func dockerInspectContainer(ctx context.Context, name string) (*dockerContainerInspect, error) {
	out, err := dockerCmd(ctx, "inspect", name)
	if err != nil {
		return nil, err
	}
	var arr []dockerContainerInspect
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("container not found")
	}
	return &arr[0], nil
}

func extractHostFromRule(rule string) string {
	// Expect: Host(`traefik.example.com`)
	rule = strings.TrimSpace(rule)
	idx := strings.Index(rule, "Host(`")
	if idx < 0 {
		return ""
	}
	s := rule[idx+len("Host(`"):]
	end := strings.Index(s, "`)")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(s[:end])
}
