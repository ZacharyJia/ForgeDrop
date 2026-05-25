package bootstrap

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge-drop/internal/server"
)

func TestLoadManifestRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(`{
  "repos": [{"full_name":"acme/demo"}],
  "app": {
    "app_key": "demo",
    "name": "Demo",
    "services": [{
      "service_key": "web",
      "name": "Web",
      "compose_template": "services: {}",
      "slots": [{
        "slot_key": "main",
        "name": "Main",
        "container_path": "/app/app.jar",
        "unknown_field": true
      }]
    }]
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestApplyManifestEndToEnd(t *testing.T) {
	t.Parallel()

	srv, err := server.New(server.Options{
		Addr:    "127.0.0.1:0",
		DataDir: t.TempDir(),
		Logger:  log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client, err := NewClient(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	enabled := true
	manifest := &Manifest{
		Settings: map[string]string{
			"base_domain":    "example.com",
			"docker_network": "traefik",
		},
		Repos: []RepoSpec{
			{FullName: "acme/demo", WebhookSecret: "hook-secret"},
		},
		App: AppSpec{
			AppKey: "demo",
			Name:   "Demo",
			Envs: []EnvSpec{
				{Name: "prod"},
				{Name: "staging"},
				{Name: "preview"},
			},
			Services: []ServiceSpec{
				{
					ServiceKey:      "web",
					Name:            "Web",
					Image:           "eclipse-temurin:21-jre",
					Command:         "sh -lc \"java -jar /app/app.jar\"",
					ContainerPort:   8080,
					ComposeTemplate: "services:\n  app:\n    image: eclipse-temurin:21-jre\n    command: sh -lc \"java -jar /app/app.jar\"\n    volumes:\n      {{- range $slotKey, $hostPath := .Artifacts }}\n      - {{$hostPath}}:{{index $.SlotPaths $slotKey}}:ro\n      {{- end }}\n",
					Enabled:         &enabled,
					Slots: []SlotSpec{
						{
							SlotKey:       "main",
							Name:          "Main Artifact",
							ContainerPath: "/app/app.jar",
						},
					},
				},
			},
		},
		APIToken: &APITokenSpec{Name: "ci-demo"},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	result, err := Apply(ctx, client, manifest, ApplyOptions{
		Username: "admin",
		Password: "secret123",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.App.Action != "created" {
		t.Fatalf("expected app creation, got %+v", result.App)
	}
	if len(result.App.Envs) != 3 {
		t.Fatalf("expected 3 envs, got %d", len(result.App.Envs))
	}
	if result.APIToken == nil || result.APIToken.PlainToken == "" {
		t.Fatalf("expected plain token in result, got %+v", result.APIToken)
	}
	if len(result.Services) != 1 || len(result.Services[0].Slots) != 1 {
		t.Fatalf("unexpected services result: %+v", result.Services)
	}

	manifest.APIToken = nil
	result, err = Apply(ctx, client, manifest, ApplyOptions{
		Username: "admin",
		Password: "secret123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.App.Action != "unchanged" {
		t.Fatalf("expected unchanged app on second apply, got %+v", result.App)
	}
	if result.Repos[0].Action != "unchanged" {
		t.Fatalf("expected unchanged repo on second apply, got %+v", result.Repos[0])
	}

	repos, err := client.ListRepos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].FullName != "acme/demo" {
		t.Fatalf("unexpected repos: %+v", repos)
	}
	services, err := client.ListServices(ctx, result.App.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].ServiceKey != "web" {
		t.Fatalf("unexpected services: %+v", services)
	}
	if services[0].ContainerPort != 8080 {
		t.Fatalf("expected initial container port 8080, got %+v", services[0])
	}
	slots, err := client.ListSlots(ctx, services[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 || slots[0].SlotKey != "main" {
		t.Fatalf("unexpected slots: %+v", slots)
	}

	manifest.App.Services[0].ContainerPort = 0
	manifest.App.Services[0].Env = map[string]string{}
	manifest.App.Services[0].TraefikEntrypoints = ""
	manifest.App.Services[0].DeployStrategy = ""

	result, err = Apply(ctx, client, manifest, ApplyOptions{
		Username: "admin",
		Password: "secret123",
	})
	if err != nil {
		t.Fatal(err)
	}

	services, err = client.ListServices(ctx, result.App.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 {
		t.Fatalf("unexpected services after reset: %+v", services)
	}
	if services[0].ContainerPort != 0 {
		t.Fatalf("expected container port to reset to 0, got %+v", services[0])
	}
	if len(services[0].Env) != 0 {
		t.Fatalf("expected env to reset, got %+v", services[0].Env)
	}
	if services[0].TraefikEntrypoints != "" {
		t.Fatalf("expected entrypoints to reset, got %+v", services[0])
	}
	if services[0].DeployStrategy != "recreate" {
		t.Fatalf("expected default deploy strategy recreate, got %+v", services[0])
	}
}
