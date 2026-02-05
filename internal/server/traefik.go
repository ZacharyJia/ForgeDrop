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
}

type traefikInstallRequest struct {
	Staging bool `json:"staging"`
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

	st := &traefikStatus{
		NetworkName:   networkName,
		ContainerName: managedTraefikContainerName,
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
	conf := buildTraefikYAML(email, resolver, caserver)
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
	image := "traefik:v3.4"
	args := []string{
		"run", "-d",
		"--name", managedTraefikContainerName,
		"--restart", "unless-stopped",
		"--label", managedTraefikLabelKey + "=true",
		"--network", networkName,
		"-p", "80:80",
		"-p", "443:443",
		"-v", "/var/run/docker.sock:/var/run/docker.sock:ro",
		"-v", confPath + ":/etc/traefik/traefik.yml:ro",
		"-v", acmePath + ":/data/acme.json",
		image,
	}
	if _, err := dockerCmd(ctx, args...); err != nil {
		return nil, fmt.Errorf("start traefik: %w", err)
	}

	return s.getTraefikStatus(ctx)
}

func buildTraefikYAML(email, resolver, caserver string) string {
	// Keep it simple: docker provider, entrypoints web/websecure, ACME TLS-ALPN.
	// Note: dashboard is enabled but not exposed unless routed.
	var b strings.Builder
	b.WriteString("entryPoints:\n")
	b.WriteString("  web:\n")
	b.WriteString("    address: ':80'\n")
	b.WriteString("  websecure:\n")
	b.WriteString("    address: ':443'\n")
	b.WriteString("providers:\n")
	b.WriteString("  docker:\n")
	b.WriteString("    endpoint: 'unix:///var/run/docker.sock'\n")
	b.WriteString("    exposedByDefault: false\n")
	b.WriteString("api:\n")
	b.WriteString("  dashboard: true\n")
	b.WriteString("certificatesResolvers:\n")
	b.WriteString("  " + resolver + ":\n")
	b.WriteString("    acme:\n")
	b.WriteString("      email: '" + strings.ReplaceAll(email, "'", "") + "'\n")
	b.WriteString("      storage: '/data/acme.json'\n")
	if caserver != "" {
		b.WriteString("      caServer: '" + caserver + "'\n")
	}
	b.WriteString("      tlsChallenge: {}\n")
	b.WriteString("log:\n")
	b.WriteString("  level: INFO\n")
	return b.String()
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
