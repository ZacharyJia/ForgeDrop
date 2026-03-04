package compose

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"forge-drop/internal/db"
)

// TemplateData contains variables for rendering compose templates
type TemplateData struct {
	// Service info
	ServiceID   string
	ServiceKey  string
	ServiceName string

	// Environment info
	EnvID   string
	EnvName string
	EnvKind string

	// App info
	AppID  string
	AppKey string

	// Artifacts (map of slot_key -> artifact_path)
	Artifacts map[string]string
	// SlotPaths (map of slot_key -> container_path)
	SlotPaths map[string]string

	// Network and routing
	Host           string
	RouterName     string
	TraefikService string // Traefik service name
	Port           int
	Network        string
	BaseDomain     string
	EntryPoints    string

	// Repo info (for preview envs)
	RepoFullName string
	RepoSlug     string
	PRNumber     int
	ChangeSet    string

	// Environment variables from service config
	Env map[string]string

	// Runtime paths
	RuntimeDir string
	DataDir    string
}

// RenderTemplate renders a Docker Compose template with the given data
func RenderTemplate(tmplStr string, data TemplateData) (string, error) {
	tmpl, err := template.New("compose").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// ComposeManager handles Docker Compose operations
type ComposeManager struct {
	dataDir string
}

func NewManager(dataDir string) *ComposeManager {
	return &ComposeManager{dataDir: dataDir}
}

// GetComposeFile returns the path to the compose file for a service
func (m *ComposeManager) GetComposeFile(envID, serviceID string) string {
	return filepath.Join(m.dataDir, "runtime", "env-"+envID, "service-"+serviceID, "docker-compose.yml")
}

// GetProjectName returns the Docker Compose project name
func (m *ComposeManager) GetProjectName(envID, serviceKey string) string {
	svc := strings.ToLower(serviceKey)
	svc = strings.ReplaceAll(svc, "_", "-")
	svc = strings.ReplaceAll(svc, " ", "-")
	return fmt.Sprintf("fd-env-%s-%s", strings.ToLower(envID), svc)
}

// WriteComposeFile writes the rendered compose content to disk
func (m *ComposeManager) WriteComposeFile(envID, serviceID, content string) error {
	path := m.GetComposeFile(envID, serviceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// Up runs docker compose up -d
func (m *ComposeManager) Up(ctx context.Context, envID, serviceID, serviceKey string) error {
	composeFile := m.GetComposeFile(envID, serviceID)
	projectName := m.GetProjectName(envID, serviceKey)

	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", composeFile,
		"-p", projectName,
		"up", "-d", "--remove-orphans")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// Down runs docker compose down
func (m *ComposeManager) Down(ctx context.Context, envID, serviceID, serviceKey string) error {
	composeFile := m.GetComposeFile(envID, serviceID)
	projectName := m.GetProjectName(envID, serviceKey)

	// Check if compose file exists
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return nil // Already cleaned up
	}

	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", composeFile,
		"-p", projectName,
		"down", "--remove-orphans")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// Restart runs docker compose restart
func (m *ComposeManager) Restart(ctx context.Context, envID, serviceID, serviceKey string) error {
	composeFile := m.GetComposeFile(envID, serviceID)
	projectName := m.GetProjectName(envID, serviceKey)

	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", composeFile,
		"-p", projectName,
		"restart")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose restart failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// Ps returns `docker compose ps` output (best-effort).
func (m *ComposeManager) Ps(ctx context.Context, envID, serviceID, serviceKey string) (string, error) {
	composeFile := m.GetComposeFile(envID, serviceID)
	projectName := m.GetProjectName(envID, serviceKey)

	if _, err := os.Stat(composeFile); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("compose file not found")
		}
		return "", err
	}

	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", composeFile,
		"-p", projectName,
		"ps", "--all")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("docker compose ps failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

// LogsOutput returns `docker compose logs` output.
func (m *ComposeManager) LogsOutput(ctx context.Context, envID, serviceID, serviceKey string, tail int) (string, error) {
	composeFile := m.GetComposeFile(envID, serviceID)
	projectName := m.GetProjectName(envID, serviceKey)

	if _, err := os.Stat(composeFile); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("compose file not found")
		}
		return "", err
	}
	if tail <= 0 {
		tail = 200
	}
	if tail > 5000 {
		tail = 5000
	}

	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", composeFile,
		"-p", projectName,
		"logs", "--no-color", "--tail", strconv.Itoa(tail))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("docker compose logs failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

// Logs streams logs from the compose project
func (m *ComposeManager) Logs(ctx context.Context, envID, serviceID, serviceKey string, follow bool) (*exec.Cmd, error) {
	composeFile := m.GetComposeFile(envID, serviceID)
	projectName := m.GetProjectName(envID, serviceKey)

	args := []string{"compose", "-f", composeFile, "-p", projectName, "logs"}
	if follow {
		args = append(args, "-f")
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd, nil
}

// BuildTemplateData creates template data from service, env, and app info
func BuildTemplateData(
	svc *db.Service,
	env *db.Env,
	app *db.App,
	artifacts map[string]string,
	slotPaths map[string]string,
	host string,
	network string,
	baseDomain string,
	dataDir string,
) TemplateData {
	routerName := fmt.Sprintf("fd-%s-%s", env.ID, svc.ServiceKey)
	serviceName := fmt.Sprintf("fd-%s-%s", env.ID, svc.ServiceKey)

	entryPoints := strings.TrimSpace(svc.TraefikEntrypnts)
	if entryPoints == "" {
		entryPoints = "websecure"
	}

	data := TemplateData{
		ServiceID:      svc.ID,
		ServiceKey:     svc.ServiceKey,
		ServiceName:    svc.Name,
		EnvID:          env.ID,
		EnvName:        env.Name,
		EnvKind:        env.Kind,
		AppID:          app.ID,
		AppKey:         app.AppKey,
		Artifacts:      artifacts,
		SlotPaths:      slotPaths,
		Host:           host,
		RouterName:     routerName,
		TraefikService: serviceName,
		Port:           svc.ContainerPort,
		Network:        network,
		BaseDomain:     baseDomain,
		EntryPoints:    entryPoints,
		Env:            svc.Env,
		RuntimeDir:     filepath.Join(dataDir, "runtime", "env-"+env.ID, "service-"+svc.ID),
		DataDir:        dataDir,
	}

	if env.RepoFullName != nil {
		data.RepoFullName = *env.RepoFullName
	}
	if env.RepoSlug != nil {
		data.RepoSlug = *env.RepoSlug
	}
	if env.PRNumber != nil {
		data.PRNumber = *env.PRNumber
	}
	if env.ChangeSet != nil {
		data.ChangeSet = *env.ChangeSet
	}

	return data
}
