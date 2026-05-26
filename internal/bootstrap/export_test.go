package bootstrap

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"slices"
	"testing"

	"forge-drop/internal/server"
)

func TestExportManifestEndToEnd(t *testing.T) {
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
	ctx := context.Background()
	if err := client.Setup(ctx, "admin", "secret123"); err != nil {
		t.Fatal(err)
	}
	if err := client.Login(ctx, "admin", "secret123"); err != nil {
		t.Fatal(err)
	}
	adminToken, err := client.CreateToken(ctx, "ctl-admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	client.SetBearerToken(adminToken.PlainToken)

	disabled := false
	manifest := &Manifest{
		Settings: map[string]string{
			"base_domain":    "example.com",
			"docker_network": "traefik",
		},
		Repos: []RepoSpec{
			{FullName: "acme/demo"},
			{FullName: "acme/shared"},
		},
		App: AppSpec{
			AppKey: "demo",
			Name:   "Demo",
			Envs: []EnvSpec{
				{Name: "prod"},
				{Name: "preview"},
				{Name: "staging"},
			},
			Services: []ServiceSpec{
				{
					ServiceKey:         "api",
					Name:               "API",
					Image:              "debian:trixie-slim",
					Command:            "sh -lc \"chmod +x /app/mes && /app/mes\"",
					ContainerPort:      8080,
					RunUser:            "1000:1000",
					Env:                map[string]string{"APP_ENV": "prod"},
					ProdHost:           "api.example.com",
					TraefikEntrypoints: "websecure",
					ComposeTemplate:    "services:\n  app:\n    image: debian:trixie-slim\n",
					DeployStrategy:     "restart",
					Enabled:            &disabled,
					Slots: []SlotSpec{
						{
							SlotKey:       "main",
							Name:          "Main",
							Repos:         []string{"acme/demo", "acme/shared"},
							MountType:     "file",
							ContainerPath: "/app/mes",
						},
					},
				},
			},
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}

	if _, err := Apply(ctx, client, manifest, ApplyOptions{Token: adminToken.PlainToken}); err != nil {
		t.Fatal(err)
	}

	exported, err := Export(ctx, client, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if exported.APIToken != nil {
		t.Fatalf("expected exported manifest to omit api token, got %+v", exported.APIToken)
	}
	if exported.App.AppKey != "demo" || exported.App.Name != "Demo" {
		t.Fatalf("unexpected app export: %+v", exported.App)
	}
	if got := repoNames(exported.Repos); !slices.Equal(got, []string{"acme/demo", "acme/shared"}) {
		t.Fatalf("unexpected repos: %+v", got)
	}
	if got := envNames(exported.App.Envs); !slices.Equal(got, []string{"preview", "prod", "staging"}) {
		t.Fatalf("unexpected envs: %+v", got)
	}
	if exported.Settings["base_domain"] != "example.com" || exported.Settings["docker_network"] != "traefik" {
		t.Fatalf("unexpected settings: %+v", exported.Settings)
	}
	if len(exported.App.Services) != 1 {
		t.Fatalf("expected 1 service, got %+v", exported.App.Services)
	}
	svc := exported.App.Services[0]
	if svc.ServiceKey != "api" || svc.DeployStrategy != "restart" || svc.Enabled == nil || *svc.Enabled != false {
		t.Fatalf("unexpected service export: %+v", svc)
	}
	if len(svc.Slots) != 1 {
		t.Fatalf("expected 1 slot, got %+v", svc.Slots)
	}
	slot := svc.Slots[0]
	if slot.ContainerPath != "/app/mes" || !slices.Equal(slot.Repos, []string{"acme/demo", "acme/shared"}) {
		t.Fatalf("unexpected slot export: %+v", slot)
	}
	if err := exported.Validate(); err != nil {
		t.Fatalf("expected exported manifest to validate, got %v", err)
	}

	result, err := Apply(ctx, client, exported, ApplyOptions{Token: adminToken.PlainToken})
	if err != nil {
		t.Fatal(err)
	}
	if result.App.Action != "unchanged" {
		t.Fatalf("expected unchanged app after export reapply, got %+v", result.App)
	}
	if len(result.Services) != 1 || len(result.Services[0].Slots) != 1 {
		t.Fatalf("unexpected services after export reapply, got %+v", result.Services)
	}

	services, err := client.ListServices(ctx, result.App.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].ServiceKey != "api" || services[0].DeployStrategy != "restart" || services[0].Enabled {
		t.Fatalf("unexpected stored service after export reapply: %+v", services)
	}
}

func repoNames(repos []RepoSpec) []string {
	out := make([]string, 0, len(repos))
	for _, repo := range repos {
		out = append(out, repo.FullName)
	}
	return out
}

func envNames(envs []EnvSpec) []string {
	out := make([]string, 0, len(envs))
	for _, env := range envs {
		out = append(out, env.Name)
	}
	return out
}
