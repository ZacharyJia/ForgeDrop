package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"forge-drop/internal/bootstrap"
	"forge-drop/internal/server"
)

func TestRunArtifactsUploadCreatesNamedEnvAndUploads(t *testing.T) {
	srv, ts, client, adminToken := newArtifactUploadTestServer(t)
	defer srv.Close()
	defer ts.Close()

	artifactPath := filepath.Join(t.TempDir(), "app.bin")
	if err := os.WriteFile(artifactPath, []byte("artifact payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(t.TempDir(), "config.json")
	authPath := filepath.Join(t.TempDir(), "auth.json")
	if err := writeJSONFile(configPath, cliConfig{Server: ts.URL}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(authPath, cliAuth{Token: adminToken}); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(t, func() error {
		return runArtifactsUpload([]string{
			"--config", configPath,
			"--auth", authPath,
			"--app", "demo",
			"--env", "qa",
			"--service", "api",
			"--slot", "main",
			"--repo", "acme/demo",
			"--file", artifactPath,
			"--create-env",
			"--deploy=false",
		})
	})
	if err != nil {
		t.Fatalf("runArtifactsUpload: %v", err)
	}

	var result struct {
		Upload struct {
			Env           string `json:"env"`
			ArtifactID    string `json:"artifact_id"`
			DeploySkipped bool   `json:"deploy_skipped"`
		} `json:"upload"`
		EnvCreated bool `json:"env_created"`
		CreatedEnv *struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"created_env"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if !result.EnvCreated {
		t.Fatalf("expected env_created=true, got %+v", result)
	}
	if result.CreatedEnv == nil || result.CreatedEnv.Name != "qa" || result.CreatedEnv.Kind != "named" {
		t.Fatalf("unexpected created env: %+v", result.CreatedEnv)
	}
	if result.Upload.Env != "qa" || result.Upload.ArtifactID == "" || !result.Upload.DeploySkipped {
		t.Fatalf("unexpected upload result: %+v", result.Upload)
	}

	envs, err := client.ListEnvs(context.Background(), mustAppID(t, client, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, env := range envs {
		if env.Name == "qa" && env.Kind == "named" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected qa env in %+v", envs)
	}
}

func TestRunArtifactsUploadUsesArtifactTokenOverride(t *testing.T) {
	srv, ts, client, _ := newArtifactUploadTestServer(t)
	defer srv.Close()
	defer ts.Close()

	ctx := context.Background()
	artifactToken, err := client.CreateToken(ctx, "ci-upload", "artifact")
	if err != nil {
		t.Fatal(err)
	}

	artifactPath := filepath.Join(t.TempDir(), "app.bin")
	if err := os.WriteFile(artifactPath, []byte("artifact payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(t.TempDir(), "config.json")
	authPath := filepath.Join(t.TempDir(), "auth.json")
	if err := writeJSONFile(configPath, cliConfig{Server: ts.URL}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(authPath, cliAuth{Token: "fd_invalid_admin_token"}); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(t, func() error {
		return runArtifactsUpload([]string{
			"--config", configPath,
			"--auth", authPath,
			"--artifact-token", artifactToken.PlainToken,
			"--app", "demo",
			"--env", "prod",
			"--service", "api",
			"--slot", "main",
			"--repo", "acme/demo",
			"--file", artifactPath,
			"--deploy=false",
		})
	})
	if err != nil {
		t.Fatalf("runArtifactsUpload with artifact token: %v", err)
	}

	var result struct {
		Upload struct {
			Env        string `json:"env"`
			ArtifactID string `json:"artifact_id"`
		} `json:"upload"`
		EnvCreated bool `json:"env_created"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if result.EnvCreated {
		t.Fatalf("did not expect env creation, got %+v", result)
	}
	if result.Upload.Env != "prod" || result.Upload.ArtifactID == "" {
		t.Fatalf("unexpected upload result: %+v", result.Upload)
	}
}

func newArtifactUploadTestServer(t *testing.T) (*server.Server, *httptest.Server, *bootstrap.Client, string) {
	t.Helper()

	srv, err := server.New(server.Options{
		Addr:    "127.0.0.1:0",
		DataDir: t.TempDir(),
		Logger:  log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())

	client, err := bootstrap.NewClient(ts.URL)
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

	manifest := &bootstrap.Manifest{
		Repos: []bootstrap.RepoSpec{
			{FullName: "acme/demo"},
		},
		App: bootstrap.AppSpec{
			AppKey: "demo",
			Name:   "Demo",
			Envs: []bootstrap.EnvSpec{
				{Name: "prod"},
			},
			Services: []bootstrap.ServiceSpec{
				{
					ServiceKey:      "api",
					Name:            "API",
					Image:           "busybox:latest",
					Command:         "sh -lc 'sleep 3600'",
					ComposeTemplate: "services:\n  app:\n    image: busybox:latest\n",
					Slots: []bootstrap.SlotSpec{
						{
							SlotKey:       "main",
							Name:          "Main",
							ContainerPath: "/app/app.bin",
						},
					},
				},
			},
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrap.Apply(ctx, client, manifest, bootstrap.ApplyOptions{Token: adminToken.PlainToken}); err != nil {
		t.Fatal(err)
	}

	return srv, ts, client, adminToken.PlainToken
}

func mustAppID(t *testing.T, client *bootstrap.Client, appKey string) string {
	t.Helper()

	apps, err := client.ListApps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range apps {
		if app.AppKey == appKey {
			return app.ID
		}
	}
	t.Fatalf("app %q not found in %+v", appKey, apps)
	return ""
}

func captureStdout(t *testing.T, fn func() error) ([]byte, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	runErr := fn()

	_ = w.Close()
	os.Stdout = oldStdout

	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return out, runErr
}
