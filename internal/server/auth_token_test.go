package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge-drop/internal/auth"
)

func TestAdminTokenCanAccessAdminAPI(t *testing.T) {
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
	plain, err := auth.NewToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.store.CreateAPIToken(ctx, "ctl-admin", "admin", plain); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+plain)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Username != "ctl-admin" {
		t.Fatalf("unexpected body %+v", body)
	}
}

func TestArtifactTokenCannotAccessAdminAPI(t *testing.T) {
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
	plain, err := auth.NewToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.store.CreateAPIToken(ctx, "ci-upload", "artifact", plain); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+plain)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
