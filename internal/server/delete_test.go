package server

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge-drop/internal/auth"
	"forge-drop/internal/db"
	"forge-drop/internal/ids"
)

type deleteFixture struct {
	app     *db.App
	service *db.Service
	env     *db.Env
}

func TestAdminCanDeleteServiceWithSnapshots(t *testing.T) {
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

	fixture := seedDeleteFixture(t, srv)
	token := newAdminToken(t, srv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/services/"+fixture.service.ID, nil)
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

	if _, err := srv.store.GetServiceByID(context.Background(), fixture.service.ID); err == nil {
		t.Fatal("expected service to be deleted")
	}
}

func TestAdminCanDeleteAppWithSnapshots(t *testing.T) {
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

	fixture := seedDeleteFixture(t, srv)
	token := newAdminToken(t, srv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/apps/"+fixture.app.ID, nil)
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

	if _, err := srv.store.GetAppByID(context.Background(), fixture.app.ID); err == nil {
		t.Fatal("expected app to be deleted")
	}
}

func seedDeleteFixture(t *testing.T, srv *Server) deleteFixture {
	t.Helper()

	ctx := context.Background()
	repo, err := srv.store.CreateRepo(ctx, "acme/demo", "secret")
	if err != nil {
		t.Fatal(err)
	}
	app, err := srv.store.CreateApp(ctx, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	service, err := srv.store.CreateService(ctx, app.ID, "web", "Web", "", "", 8080, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	slot, err := srv.store.CreateSlot(ctx, service.ID, "main", "Main", "/app/app.jar", "file", []string{repo.ID})
	if err != nil {
		t.Fatal(err)
	}
	env, err := srv.store.CreateNamedEnv(ctx, app.ID, "prod")
	if err != nil {
		t.Fatal(err)
	}
	artifactID, err := ids.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.store.CreateArtifactAndSnapshot(ctx, db.UploadParams{
		ArtifactID: artifactID,
		AppID:      app.ID,
		ServiceID:  service.ID,
		SlotID:     slot.ID,
		RepoID:     repo.ID,
		EnvID:      env.ID,
		Filename:   "app.jar",
		SizeBytes:  128,
		SHA256Hex:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		StoredPath: "data/artifacts/" + artifactID + "/app.jar",
		Note:       "manual upload",
	}); err != nil {
		t.Fatal(err)
	}

	return deleteFixture{
		app:     app,
		service: service,
		env:     env,
	}
}

func newAdminToken(t *testing.T, srv *Server) string {
	t.Helper()

	token, err := auth.NewToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.store.CreateAPIToken(context.Background(), "ctl-admin", "admin", token); err != nil {
		t.Fatal(err)
	}
	return token
}
