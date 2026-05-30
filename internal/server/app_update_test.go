package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge-drop/internal/auth"
)

func TestAdminCanUpdateAppName(t *testing.T) {
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
	app, err := srv.store.CreateApp(ctx, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := auth.NewToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.store.CreateAPIToken(ctx, "ctl-admin", "admin", plain); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := []byte(`{"name":"Demo Console"}`)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/apps/"+app.ID, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated struct {
		ID     string `json:"id"`
		AppKey string `json:"app_key"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != app.ID || updated.AppKey != "demo" || updated.Name != "Demo Console" {
		t.Fatalf("unexpected response %+v", updated)
	}

	stored, err := srv.store.GetAppByID(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Demo Console" || stored.AppKey != "demo" {
		t.Fatalf("unexpected stored app %+v", stored)
	}
}

func TestAdminAppUpdateRejectsImmutableFields(t *testing.T) {
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
	app, err := srv.store.CreateApp(ctx, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := auth.NewToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.store.CreateAPIToken(ctx, "ctl-admin", "admin", plain); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := []byte(`{"name":"Demo Console","app_key":"other"}`)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/apps/"+app.ID, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
