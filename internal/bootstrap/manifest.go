package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

type Manifest struct {
	Settings map[string]string `json:"settings,omitempty"`
	Repos    []RepoSpec        `json:"repos"`
	App      AppSpec           `json:"app"`
	APIToken *APITokenSpec     `json:"api_token,omitempty"`
}

type RepoSpec struct {
	FullName      string `json:"full_name"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

type AppSpec struct {
	AppKey   string        `json:"app_key"`
	Name     string        `json:"name"`
	Envs     []EnvSpec     `json:"envs,omitempty"`
	Services []ServiceSpec `json:"services"`
}

type EnvSpec struct {
	Name string `json:"name"`
}

type ServiceSpec struct {
	ServiceKey         string            `json:"service_key"`
	Name               string            `json:"name"`
	Image              string            `json:"image,omitempty"`
	Command            string            `json:"command,omitempty"`
	ContainerPort      int               `json:"container_port,omitempty"`
	RunUser            string            `json:"run_user,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	ProdHost           string            `json:"prod_host,omitempty"`
	TraefikEntrypoints string            `json:"traefik_entrypoints,omitempty"`
	ComposeTemplate    string            `json:"compose_template"`
	DeployStrategy     string            `json:"deploy_strategy,omitempty"`
	Enabled            *bool             `json:"enabled,omitempty"`
	Slots              []SlotSpec        `json:"slots"`
}

type SlotSpec struct {
	SlotKey       string   `json:"slot_key"`
	Name          string   `json:"name"`
	Repos         []string `json:"repos,omitempty"`
	MountType     string   `json:"mount_type,omitempty"`
	ContainerPath string   `json:"container_path"`
}

type APITokenSpec struct {
	Name           string `json:"name"`
	RotateIfExists bool   `json:"rotate_if_exists,omitempty"`
}

func LoadManifest(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var manifest Manifest
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (m *Manifest) Validate() error {
	if len(m.Repos) == 0 {
		return fmt.Errorf("repos must contain at least one repository")
	}

	repoNames := make(map[string]struct{}, len(m.Repos))
	for i := range m.Repos {
		m.Repos[i].FullName = strings.TrimSpace(m.Repos[i].FullName)
		if m.Repos[i].FullName == "" {
			return fmt.Errorf("repos[%d].full_name is required", i)
		}
		if _, exists := repoNames[m.Repos[i].FullName]; exists {
			return fmt.Errorf("repos[%d].full_name duplicates %q", i, m.Repos[i].FullName)
		}
		repoNames[m.Repos[i].FullName] = struct{}{}
	}

	m.App.AppKey = strings.TrimSpace(m.App.AppKey)
	m.App.Name = strings.TrimSpace(m.App.Name)
	if m.App.AppKey == "" || m.App.Name == "" {
		return fmt.Errorf("app.app_key and app.name are required")
	}
	if len(m.App.Services) == 0 {
		return fmt.Errorf("app.services must contain at least one service")
	}

	envNames := make(map[string]struct{}, len(m.App.Envs))
	for i := range m.App.Envs {
		m.App.Envs[i].Name = strings.TrimSpace(m.App.Envs[i].Name)
		if m.App.Envs[i].Name == "" {
			return fmt.Errorf("app.envs[%d].name is required", i)
		}
		if _, exists := envNames[m.App.Envs[i].Name]; exists {
			return fmt.Errorf("app.envs[%d].name duplicates %q", i, m.App.Envs[i].Name)
		}
		envNames[m.App.Envs[i].Name] = struct{}{}
	}

	serviceKeys := make(map[string]struct{}, len(m.App.Services))
	for i := range m.App.Services {
		svc := &m.App.Services[i]
		svc.ServiceKey = strings.TrimSpace(svc.ServiceKey)
		svc.Name = strings.TrimSpace(svc.Name)
		svc.Image = strings.TrimSpace(svc.Image)
		svc.Command = strings.TrimSpace(svc.Command)
		svc.RunUser = strings.TrimSpace(svc.RunUser)
		svc.ProdHost = strings.TrimSpace(svc.ProdHost)
		svc.TraefikEntrypoints = strings.TrimSpace(svc.TraefikEntrypoints)
		svc.ComposeTemplate = strings.TrimSpace(svc.ComposeTemplate)
		svc.DeployStrategy = strings.TrimSpace(strings.ToLower(svc.DeployStrategy))

		if svc.ServiceKey == "" || svc.Name == "" {
			return fmt.Errorf("app.services[%d].service_key and name are required", i)
		}
		if _, exists := serviceKeys[svc.ServiceKey]; exists {
			return fmt.Errorf("app.services[%d].service_key duplicates %q", i, svc.ServiceKey)
		}
		serviceKeys[svc.ServiceKey] = struct{}{}
		if svc.ComposeTemplate == "" {
			return fmt.Errorf("app.services[%d].compose_template is required", i)
		}
		if svc.DeployStrategy != "" && svc.DeployStrategy != "recreate" && svc.DeployStrategy != "restart" {
			return fmt.Errorf("app.services[%d].deploy_strategy must be recreate or restart", i)
		}
		if len(svc.Slots) == 0 {
			return fmt.Errorf("app.services[%d].slots must contain at least one slot", i)
		}

		slotKeys := make(map[string]struct{}, len(svc.Slots))
		for j := range svc.Slots {
			slot := &svc.Slots[j]
			slot.SlotKey = strings.TrimSpace(slot.SlotKey)
			slot.Name = strings.TrimSpace(slot.Name)
			slot.ContainerPath = strings.TrimSpace(slot.ContainerPath)
			slot.MountType = strings.TrimSpace(strings.ToLower(slot.MountType))
			if slot.MountType == "" {
				slot.MountType = "file"
			}
			if slot.SlotKey == "" || slot.Name == "" || slot.ContainerPath == "" {
				return fmt.Errorf("app.services[%d].slots[%d] requires slot_key, name, and container_path", i, j)
			}
			if slot.MountType != "file" && slot.MountType != "dir" {
				return fmt.Errorf("app.services[%d].slots[%d].mount_type must be file or dir", i, j)
			}
			if _, exists := slotKeys[slot.SlotKey]; exists {
				return fmt.Errorf("app.services[%d].slots[%d].slot_key duplicates %q", i, j, slot.SlotKey)
			}
			slotKeys[slot.SlotKey] = struct{}{}
			if len(slot.Repos) == 0 {
				slot.Repos = []string{m.Repos[0].FullName}
			}
			for k := range slot.Repos {
				slot.Repos[k] = strings.TrimSpace(slot.Repos[k])
				if slot.Repos[k] == "" {
					return fmt.Errorf("app.services[%d].slots[%d].repos[%d] is empty", i, j, k)
				}
				if _, ok := repoNames[slot.Repos[k]]; !ok {
					return fmt.Errorf("app.services[%d].slots[%d].repos[%d] references unknown repo %q", i, j, k, slot.Repos[k])
				}
			}
			slices.Sort(slot.Repos)
			slot.Repos = slices.Compact(slot.Repos)
		}
	}

	if m.APIToken != nil {
		m.APIToken.Name = strings.TrimSpace(m.APIToken.Name)
		if m.APIToken.Name == "" {
			return fmt.Errorf("api_token.name is required")
		}
	}

	return nil
}

func (a AppSpec) DesiredEnvNames() []string {
	if len(a.Envs) == 0 {
		return []string{"prod", "preview"}
	}
	out := make([]string, 0, len(a.Envs))
	for _, env := range a.Envs {
		out = append(out, env.Name)
	}
	return out
}

func (s ServiceSpec) EnabledValue() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}
