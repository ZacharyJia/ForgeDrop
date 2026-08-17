package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

// GET /apps/:id/envs must serialize envs with the same lowercase JSON keys as
// envJSON (used by GET /apps/:id and GET /envs/:id); forgedrop-ctl depends on
// repo_id/pr_number/change_set/repo_full_name to resolve PR preview envs.
func TestAdminListAppEnvsUsesEnvJSONKeys(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Addr:    "127.0.0.1:0",
		DataDir: t.TempDir(),
		Logger:  log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx := context.Background()
	repo, err := srv.store.CreateRepo(ctx, "acme/demo", "secret")
	if err != nil {
		t.Fatal(err)
	}
	app, err := srv.store.CreateApp(ctx, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	pr := 42
	if _, err := srv.store.UpsertPreviewEnv(ctx, app.ID, *repo, &pr, ""); err != nil {
		t.Fatal(err)
	}
	token := newAdminToken(t, srv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/apps/"+app.ID+"/envs", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var envs []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&envs); err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 env, got %d", len(envs))
	}
	env := envs[0]
	if env["kind"] != "preview" {
		t.Fatalf("expected kind=preview, got %v (keys: %v)", env["kind"], env)
	}
	if env["repo_full_name"] != "acme/demo" {
		t.Fatalf("expected repo_full_name=acme/demo, got %v", env["repo_full_name"])
	}
	if pr, ok := env["pr_number"].(float64); !ok || int(pr) != 42 {
		t.Fatalf("expected pr_number=42, got %v", env["pr_number"])
	}
	if _, ok := env["Kind"]; ok {
		t.Fatal("response must not use Go struct field names")
	}
}
