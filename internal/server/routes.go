package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"forge-drop/internal/auth"
	"forge-drop/internal/db"
	"forge-drop/internal/httpx"
	"forge-drop/internal/ids"
	webui "forge-drop/web"
)

func parseAutoDeployFlag(v string) bool {
	// Default behavior is auto-deploy to keep existing CI/UI flows working.
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return true
	}
	// Treat common falsy values as opt-out.
	if v == "0" || v == "false" || v == "no" || v == "off" {
		return false
	}
	return true
}

func appJSON(a *db.App) map[string]any {
	if a == nil {
		return nil
	}
	return map[string]any{
		"id":         a.ID,
		"app_key":    a.AppKey,
		"name":       a.Name,
		"created_at": a.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func serviceJSON(svc *db.Service) map[string]any {
	if svc == nil {
		return nil
	}
	return map[string]any{
		"id":                  svc.ID,
		"app_id":              svc.AppID,
		"service_key":         svc.ServiceKey,
		"name":                svc.Name,
		"image":               svc.Image,
		"command":             svc.Command,
		"container_port":      svc.ContainerPort,
		"run_user":            svc.RunUser,
		"env":                 svc.Env,
		"prod_host":           svc.ProdHost,
		"traefik_entrypoints": svc.TraefikEntrypnts,
		"compose_template":    svc.ComposeTemplate,
		"use_compose":         svc.UseCompose,
		"revision":            svc.Revision,
		"enabled":             svc.Enabled,
		"created_at":          svc.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":          svc.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func slotJSON(sl db.Slot) map[string]any {
	return map[string]any{
		"id":             sl.ID,
		"service_id":     sl.ServiceID,
		"slot_key":       sl.SlotKey,
		"name":           sl.Name,
		"repo_id":        sl.RepoID,
		"container_path": sl.ContainerPath,
		"created_at":     sl.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":     sl.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func envJSON(e *db.Env) map[string]any {
	if e == nil {
		return nil
	}
	out := map[string]any{
		"id":                  e.ID,
		"app_id":              e.AppID,
		"kind":                e.Kind,
		"name":                e.Name,
		"created_at":          e.CreatedAt.UTC().Format(time.RFC3339Nano),
		"current_snapshot_id": e.CurrentSnapshot,
		"repo_id":             e.RepoID,
		"pr_number":           e.PRNumber,
		"deleted_at":          e.DeletedAt,
		"repo_full_name":      e.RepoFullName,
		"repo_slug":           e.RepoSlug,
	}
	return out
}

func snapshotJSON(sn db.Snapshot) map[string]any {
	out := map[string]any{
		"id":         sn.ID,
		"env_id":     sn.EnvID,
		"created_at": sn.CreatedAt.UTC().Format(time.RFC3339Nano),
		"note":       sn.Note,
	}
	if sn.CreatedByUserID != nil {
		out["created_by_user_id"] = *sn.CreatedByUserID
	} else {
		out["created_by_user_id"] = nil
	}
	if sn.CreatedByToken != nil {
		out["created_by_token_id"] = *sn.CreatedByToken
	} else {
		out["created_by_token_id"] = nil
	}
	return out
}

func artifactJSON(a db.Artifact) map[string]any {
	out := map[string]any{
		"id":                a.ID,
		"app_id":            a.AppID,
		"service_id":        a.ServiceID,
		"slot_id":           a.SlotID,
		"repo_id":           a.RepoID,
		"sha":               a.SHA,
		"ref":               a.Ref,
		"pr_number":         a.PRNumber,
		"original_filename": a.OriginalFilename,
		"size_bytes":        a.SizeBytes,
		"sha256_hex":        a.SHA256Hex,
		"stored_path":       a.StoredPath,
		"created_at":        a.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	return out
}

func repoJSON(r *db.Repo) map[string]any {
	if r == nil {
		return nil
	}
	return map[string]any{
		"id":             r.ID,
		"full_name":      r.FullName,
		"slug":           r.Slug,
		"webhook_secret": r.WebhookSecret,
		"created_at":     r.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func apiTokenJSON(t *db.APIToken) map[string]any {
	if t == nil {
		return nil
	}
	out := map[string]any{
		"id":         t.ID,
		"name":       t.Name,
		"prefix":     t.Prefix,
		"created_at": t.CreatedAt.UTC().Format(time.RFC3339Nano),
		"revoked_at": nil,
	}
	if t.RevokedAt != nil {
		out["revoked_at"] = t.RevokedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

// legacyRoutes is the original net/http mux router.
// It is kept temporarily while migrating to Gin.
func (s *Server) legacyRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	// Public: setup + auth
	mux.Handle("/api/v1/setup", s.withTimeout(http.HandlerFunc(method("POST", requireSetupAllowed(s.store, s.handleSetup)))))
	mux.Handle("/api/v1/setup/status", s.withTimeout(http.HandlerFunc(method("GET", s.handleSetupStatus))))
	mux.Handle("/api/v1/auth/login", s.withTimeout(http.HandlerFunc(method("POST", s.handleLogin))))
	mux.Handle("/api/v1/auth/logout", s.withTimeout(http.HandlerFunc(method("POST", s.handleLogout))))
	mux.Handle("/api/v1/admin/", s.withJSON(s.withTimeout(s.requireSession(http.HandlerFunc(s.handleAdmin)))))
	mux.Handle("/api/v1/artifacts/upload", s.withJSON(s.withTimeout(s.requireBearerToken(http.HandlerFunc(s.handleArtifactUpload)))))
	mux.Handle("/api/v1/artifacts/upload-batch", s.withJSON(s.withTimeout(s.requireBearerToken(http.HandlerFunc(s.handleArtifactUploadBatch)))))

	mux.HandleFunc("/webhooks/forgejo", method("POST", s.handleForgejoWebhook))

	// SPA static (embedded)
	mux.Handle("/", s.serveSPA())

	return s.withAccessLog(mux)
}

func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if s.opts.Dev {
			s.logf("%s %s %s (%s)", r.Method, r.URL.Path, s.clientIP(r), time.Since(start))
		}
	})
}

func (s *Server) serveSPA() http.Handler {
	sub, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		// should never happen
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "web ui missing", http.StatusInternalServerError)
		})
	}
	// If the embed only contains a placeholder (e.g. web/dist/.keep), show a
	// helpful message instead of a confusing 404.
	if f, err := sub.Open("index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>forge-drop UI not built</title>
    <style>
      body { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace; padding: 24px; line-height: 1.5; }
      code, pre { background: #f6f8fa; padding: 2px 6px; border-radius: 6px; }
      pre { padding: 12px; overflow: auto; }
      .muted { color: #666; }
    </style>
  </head>
  <body>
    <h1>Web UI is not built</h1>
    <p class="muted">This binary was built without bundled web assets.</p>
    <p>To build the embedded UI, run:</p>
    <pre><code>npm --prefix web install
npm --prefix web run build
go build ./cmd/forge-drop</code></pre>
    <p>Or use the helper script:</p>
    <pre><code>scripts/build.sh --install</code></pre>
  </body>
</html>`))
		})
	} else {
		_ = f.Close()
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do not serve UI for API routes.
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/webhooks/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve a static file first.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if f, err := sub.Open(path); err == nil {
			_ = f.Close()
			if !s.opts.Dev {
				w.Header().Set("Cache-Control", "public, max-age=300")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback.
		r2 := r.Clone(r.Context())
		// NOTE: net/http's FileServer redirects "/index.html" -> "./" ("/"),
		// which breaks BrowserRouter deep-links (e.g. /apps/xxx) with redirect loops.
		// Serving the directory path lets FileServer return the index file with 200.
		r2.URL.Path = "/"
		if !s.opts.Dev {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r2)
	})
}

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "username/password required")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hash failed")
		return
	}
	u, err := s.store.CreateUser(r.Context(), req.Username, hash)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "create user failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"user_id": u.ID})
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.UserCount(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "db error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"allowed":    c == 0,
		"user_count": c,
	})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.store.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, req.Password) {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := auth.NewToken(32)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token failed")
		return
	}
	_, err = s.store.CreateSession(r.Context(), u.ID, token, 7*24*time.Hour)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "session failed")
		return
	}
	auth.SetSessionCookie(w, token, auth.CookieOptions{Secure: s.isSecureRequest(r)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSessionCookie(w)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	// Path is /api/v1/admin/<resource...>
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/")

	switch {
	case r.Method == "GET" && path == "me":
		s.handleAdminMe(w, r)
		return
	case strings.HasPrefix(path, "settings"):
		s.handleAdminSettings(w, r, strings.TrimPrefix(path, "settings"))
		return
	case strings.HasPrefix(path, "tokens"):
		s.handleAdminTokens(w, r, strings.TrimPrefix(path, "tokens"))
		return
	case strings.HasPrefix(path, "repos"):
		s.handleAdminRepos(w, r, strings.TrimPrefix(path, "repos"))
		return
	case strings.HasPrefix(path, "apps"):
		s.handleAdminApps(w, r, strings.TrimPrefix(path, "apps"))
		return
	case strings.HasPrefix(path, "envs"):
		s.handleAdminEnvs(w, r, strings.TrimPrefix(path, "envs"))
		return
	case strings.HasPrefix(path, "services"):
		s.handleAdminServices(w, r, strings.TrimPrefix(path, "services"))
		return
	case strings.HasPrefix(path, "traefik"):
		s.handleAdminTraefik(w, r, strings.TrimPrefix(path, "traefik"))
		return
	default:
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	uid := userIDFromContext(r.Context())
	if uid == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "missing session")
		return
	}
	u, err := s.store.GetUserByID(r.Context(), *uid)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid session")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":       u.ID,
		"username": u.Username,
	})
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request, rest string) {
	switch r.Method {
	case "GET":
		baseDomain, _ := s.store.GetSetting(r.Context(), "base_domain")
		tpl, _ := s.store.GetSetting(r.Context(), "preview_host_template")
		netw, _ := s.store.GetSetting(r.Context(), "docker_network")
		email, _ := s.store.GetSetting(r.Context(), "traefik_acme_email")
		acmeMode, _ := s.store.GetSetting(r.Context(), "traefik_acme_mode")
		regionID, _ := s.store.GetSetting(r.Context(), "traefik_alicloud_region_id")
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"base_domain":                baseDomain,
			"preview_host_template":      tpl,
			"docker_network":             netw,
			"traefik_acme_email":         email,
			"traefik_acme_mode":          acmeMode,
			"traefik_alicloud_region_id": regionID,
			"artifact_upload_url":        s.baseURL(r) + "/api/v1/artifacts/upload",
			"forgejo_webhook_url":        s.baseURL(r) + "/webhooks/forgejo",
			"preview_hosting_note":       "configure wildcard DNS and Traefik separately",
			"requires_traefik_label":     true,
		})
		return
	case "PUT":
		var req map[string]string
		if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
		for k, v := range req {
			switch k {
			case "base_domain", "preview_host_template", "docker_network", "traefik_acme_email", "traefik_acme_mode", "traefik_alicloud_region_id":
				if err := s.store.SetSetting(r.Context(), k, strings.TrimSpace(v)); err != nil {
					httpx.WriteError(w, http.StatusInternalServerError, "save failed")
					return
				}
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}

func (s *Server) handleAdminTokens(w http.ResponseWriter, r *http.Request, rest string) {
	rest = strings.TrimPrefix(rest, "/")
	switch {
	case r.Method == "GET" && rest == "":
		tokens, err := s.store.ListAPITokens(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "db error")
			return
		}
		var out []map[string]any
		out = make([]map[string]any, 0, len(tokens))
		for _, t := range tokens {
			tok := t
			out = append(out, apiTokenJSON(&tok))
		}
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	case r.Method == "POST" && rest == "":
		var req struct {
			Name string `json:"name"`
		}
		if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			httpx.WriteError(w, http.StatusBadRequest, "name required")
			return
		}
		plain, err := auth.NewToken(32)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "token failed")
			return
		}
		t, err := s.store.CreateAPIToken(r.Context(), req.Name, plain)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "create failed")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"token":       apiTokenJSON(t),
			"plain_token": plain,
		})
		return
	case r.Method == "DELETE" && rest != "":
		id := strings.TrimSuffix(rest, "/")
		if id == "" {
			httpx.WriteError(w, http.StatusBadRequest, "id required")
			return
		}
		if err := s.store.RevokeAPIToken(r.Context(), id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "revoke failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case r.Method == "POST" && strings.HasSuffix(rest, "/revoke"):
		id := strings.TrimSuffix(rest, "/revoke")
		id = strings.TrimSuffix(id, "/")
		if id == "" {
			httpx.WriteError(w, http.StatusBadRequest, "id required")
			return
		}
		if err := s.store.RevokeAPIToken(r.Context(), id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "revoke failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
}

func (s *Server) handleAdminRepos(w http.ResponseWriter, r *http.Request, rest string) {
	rest = strings.TrimPrefix(rest, "/")
	switch {
	case r.Method == "GET" && rest == "":
		repos, err := s.store.ListRepos(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "db error")
			return
		}
		var out []map[string]any
		out = make([]map[string]any, 0, len(repos))
		for _, rr := range repos {
			r := rr
			out = append(out, repoJSON(&r))
		}
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	case r.Method == "POST" && rest == "":
		var req struct {
			FullName      string `json:"full_name"`
			WebhookSecret string `json:"webhook_secret"`
		}
		if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.FullName = strings.TrimSpace(req.FullName)
		if req.FullName == "" {
			httpx.WriteError(w, http.StatusBadRequest, "full_name required")
			return
		}
		repo, err := s.store.CreateRepo(r.Context(), req.FullName, req.WebhookSecret)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "create failed")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, repoJSON(repo))
		return
	case r.Method == "DELETE" && rest != "":
		id := strings.TrimSuffix(rest, "/")
		if err := s.store.DeleteRepo(r.Context(), id); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				httpx.WriteError(w, http.StatusNotFound, "not found")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case r.Method == "PUT" && rest != "":
		id := rest
		var req struct {
			WebhookSecret string `json:"webhook_secret"`
		}
		if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := s.store.UpdateRepoSecret(r.Context(), id, req.WebhookSecret); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "update failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
}

func (s *Server) handleAdminApps(w http.ResponseWriter, r *http.Request, rest string) {
	rest = strings.TrimPrefix(rest, "/")

	// /apps
	if rest == "" {
		switch r.Method {
		case "GET":
			apps, err := s.store.ListApps(r.Context())
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "db error")
				return
			}
			var out []map[string]any
			out = make([]map[string]any, 0, len(apps))
			for _, a := range apps {
				app := a
				out = append(out, appJSON(&app))
			}
			httpx.WriteJSON(w, http.StatusOK, out)
			return
		case "POST":
			var req struct {
				AppKey string `json:"app_key"`
				Name   string `json:"name"`
			}
			if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid json")
				return
			}
			req.AppKey = strings.TrimSpace(req.AppKey)
			req.Name = strings.TrimSpace(req.Name)
			if req.AppKey == "" || req.Name == "" {
				httpx.WriteError(w, http.StatusBadRequest, "app_key/name required")
				return
			}
			app, err := s.store.CreateApp(r.Context(), req.AppKey, req.Name)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "create failed")
				return
			}
			// Make first-run smoother: create default envs.
			_, _ = s.store.EnsureNamedEnv(r.Context(), app.ID, "prod")
			_, _ = s.store.EnsurePreviewPlaceholderEnv(r.Context(), app.ID)
			httpx.WriteJSON(w, http.StatusCreated, appJSON(app))
			return
		default:
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	parts := strings.Split(rest, "/")
	appID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			app, err := s.store.GetAppByID(r.Context(), appID)
			if err != nil {
				httpx.WriteError(w, http.StatusNotFound, "not found")
				return
			}
			services, _ := s.store.ListServicesByApp(r.Context(), appID)
			envs, _ := s.store.ListEnvsByApp(r.Context(), appID)
			outSvcs := make([]map[string]any, 0, len(services))
			for _, svc := range services {
				s := svc
				outSvcs = append(outSvcs, serviceJSON(&s))
			}
			outEnvs := make([]map[string]any, 0, len(envs))
			for _, e := range envs {
				env := e
				outEnvs = append(outEnvs, envJSON(&env))
			}
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"app": appJSON(app), "services": outSvcs, "envs": outEnvs})
			return
		case "DELETE":
			if err := s.store.DeleteApp(r.Context(), appID); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "delete failed")
				return
			}
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		default:
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	// /apps/{appID}/services or /apps/{appID}/envs
	switch parts[1] {
	case "services":
		s.handleAdminAppServices(w, r, appID, parts[2:])
		return
	case "envs":
		s.handleAdminAppEnvs(w, r, appID, parts[2:])
		return
	default:
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
}

func (s *Server) handleAdminAppServices(w http.ResponseWriter, r *http.Request, appID string, rest []string) {
	if len(rest) == 0 {
		switch r.Method {
		case "GET":
			svcs, err := s.store.ListServicesByApp(r.Context(), appID)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "db error")
				return
			}
			out := make([]map[string]any, 0, len(svcs))
			for _, svc := range svcs {
				s := svc
				out = append(out, serviceJSON(&s))
			}
			httpx.WriteJSON(w, http.StatusOK, out)
			return
		case "POST":
			var req struct {
				ServiceKey    string            `json:"service_key"`
				Name          string            `json:"name"`
				Image         string            `json:"image"`
				Command       string            `json:"command"`
				ContainerPort int               `json:"container_port"`
				RunUser       string            `json:"run_user"`
				Env           map[string]string `json:"env"`
				ProdHost      string            `json:"prod_host"`
			}
			if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid json")
				return
			}
			req.ServiceKey = strings.TrimSpace(req.ServiceKey)
			req.Name = strings.TrimSpace(req.Name)
			if req.ServiceKey == "" || req.Name == "" {
				httpx.WriteError(w, http.StatusBadRequest, "service_key/name required")
				return
			}
			svc, err := s.store.CreateService(r.Context(), appID, req.ServiceKey, req.Name, req.Image, req.Command, req.ContainerPort, req.RunUser, req.Env, req.ProdHost)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "create failed")
				return
			}
			httpx.WriteJSON(w, http.StatusCreated, serviceJSON(svc))
			return
		default:
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}
	httpx.WriteError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminAppEnvs(w http.ResponseWriter, r *http.Request, appID string, rest []string) {
	if len(rest) == 0 {
		switch r.Method {
		case "GET":
			envs, err := s.store.ListEnvsByApp(r.Context(), appID)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "db error")
				return
			}
			httpx.WriteJSON(w, http.StatusOK, envs)
			return
		case "POST":
			var req struct {
				Name string `json:"name"`
			}
			if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid json")
				return
			}
			req.Name = strings.TrimSpace(req.Name)
			if req.Name == "" {
				httpx.WriteError(w, http.StatusBadRequest, "name required")
				return
			}
			env, err := s.store.CreateNamedEnv(r.Context(), appID, req.Name)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "create failed")
				return
			}
			httpx.WriteJSON(w, http.StatusCreated, envJSON(env))
			return
		default:
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}
	httpx.WriteError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminServices(w http.ResponseWriter, r *http.Request, rest string) {
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	serviceID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			svc, err := s.store.GetServiceByID(r.Context(), serviceID)
			if err != nil {
				httpx.WriteError(w, http.StatusNotFound, "not found")
				return
			}
			slots, _ := s.store.ListSlotsByService(r.Context(), serviceID)
			outSlots := make([]map[string]any, 0, len(slots))
			for _, sl := range slots {
				outSlots = append(outSlots, slotJSON(sl))
			}
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"service": serviceJSON(svc), "slots": outSlots})
			return
		case "PUT":
			var req struct {
				Name             string            `json:"name"`
				ContainerPort    int               `json:"container_port"`
				Env              map[string]string `json:"env"`
				ProdHost         string            `json:"prod_host"`
				TraefikEntrypnts string            `json:"traefik_entrypoints"`
				ComposeTemplate  string            `json:"compose_template"`
				Enabled          bool              `json:"enabled"`
			}
			if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid json")
				return
			}
			svc, err := s.store.GetServiceByID(r.Context(), serviceID)
			if err != nil {
				httpx.WriteError(w, http.StatusNotFound, "not found")
				return
			}
			patch := *svc
			if req.Name != "" {
				patch.Name = req.Name
			}
			if req.ContainerPort != 0 {
				patch.ContainerPort = req.ContainerPort
			}
			if req.Env != nil {
				patch.Env = req.Env
			}
			patch.ProdHost = req.ProdHost
			if req.TraefikEntrypnts != "" {
				patch.TraefikEntrypnts = req.TraefikEntrypnts
			}
			patch.ComposeTemplate = req.ComposeTemplate
			patch.UseCompose = true
			patch.Enabled = req.Enabled
			updated, err := s.store.UpdateService(r.Context(), serviceID, patch)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "update failed")
				return
			}
			httpx.WriteJSON(w, http.StatusOK, serviceJSON(updated))
			return
		case "DELETE":
			if err := s.store.DeleteService(r.Context(), serviceID); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "delete failed")
				return
			}
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		default:
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	if len(parts) >= 2 && parts[1] == "slots" {
		s.handleAdminSlots(w, r, serviceID, parts[2:])
		return
	}
	if len(parts) >= 2 && parts[1] == "artifacts" {
		s.handleAdminServiceArtifacts(w, r, serviceID, parts[2:])
		return
	}
	if len(parts) >= 2 && parts[1] == "status" {
		s.handleServiceStatus(w, r, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "logs" {
		s.handleServiceLogs(w, r, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "deploy" {
		s.handleServiceDeploy(w, r, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "redeploy" {
		s.handleServiceRedeploy(w, r, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "compose-template-example" && r.Method == "GET" {
		s.handleComposeTemplateExample(w, r)
		return
	}
	httpx.WriteError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminServiceArtifacts(w http.ResponseWriter, r *http.Request, serviceID string, rest []string) {
	// /services/{serviceID}/artifacts/upload-batch
	if len(rest) == 1 && rest[0] == "upload-batch" {
		if r.Method != "POST" {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleAdminServiceArtifactUploadBatch(w, r, serviceID)
		return
	}
	httpx.WriteError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminSlots(w http.ResponseWriter, r *http.Request, serviceID string, rest []string) {
	if len(rest) == 0 {
		switch r.Method {
		case "GET":
			slots, err := s.store.ListSlotsByService(r.Context(), serviceID)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "db error")
				return
			}
			out := make([]map[string]any, 0, len(slots))
			for _, sl := range slots {
				out = append(out, slotJSON(sl))
			}
			httpx.WriteJSON(w, http.StatusOK, out)
			return
		case "POST":
			var req struct {
				SlotKey       string `json:"slot_key"`
				Name          string `json:"name"`
				RepoID        string `json:"repo_id"`
				ContainerPath string `json:"container_path"`
			}
			if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid json")
				return
			}
			req.SlotKey = strings.TrimSpace(req.SlotKey)
			req.Name = strings.TrimSpace(req.Name)
			if req.SlotKey == "" || req.Name == "" || req.RepoID == "" || req.ContainerPath == "" {
				httpx.WriteError(w, http.StatusBadRequest, "slot_key/name/repo_id/container_path required")
				return
			}
			slot, err := s.store.CreateSlot(r.Context(), serviceID, req.SlotKey, req.Name, req.RepoID, req.ContainerPath)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "create failed")
				return
			}
			httpx.WriteJSON(w, http.StatusCreated, slotJSON(*slot))
			return
		default:
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	// /services/{id}/slots/{slotID}
	slotID := rest[0]
	switch r.Method {
	case "PUT":
		var req struct {
			Name          string `json:"name"`
			RepoID        string `json:"repo_id"`
			ContainerPath string `json:"container_path"`
		}
		if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
		slot, err := s.store.GetSlotByID(r.Context(), slotID)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		patch := *slot
		if req.Name != "" {
			patch.Name = req.Name
		}
		if req.RepoID != "" {
			patch.RepoID = req.RepoID
		}
		if req.ContainerPath != "" {
			patch.ContainerPath = req.ContainerPath
		}
		updated, err := s.store.UpdateSlot(r.Context(), slotID, patch)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "update failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, slotJSON(*updated))
		return
	case "DELETE":
		if err := s.store.DeleteSlot(r.Context(), slotID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}

func (s *Server) handleAdminEnvs(w http.ResponseWriter, r *http.Request, rest string) {
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	envID := parts[0]
	if len(parts) == 1 {
		if r.Method != "GET" {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		env, err := s.store.GetEnvByID(r.Context(), envID)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		app, _ := s.store.GetAppByID(r.Context(), env.AppID)
		services, _ := s.store.ListServicesByApp(r.Context(), env.AppID)
		cur, _ := s.store.GetEnvCurrentSnapshotID(r.Context(), envID)
		slotsByService := map[string][]map[string]any{}
		outSvcs := make([]map[string]any, 0, len(services))
		for _, svc := range services {
			svcCopy := svc
			outSvcs = append(outSvcs, serviceJSON(&svcCopy))
			ss, _ := s.store.ListSlotsByService(r.Context(), svc.ID)
			outSlots := make([]map[string]any, 0, len(ss))
			for _, sl := range ss {
				outSlots = append(outSlots, slotJSON(sl))
			}
			slotsByService[svc.ID] = outSlots
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"env":                 envJSON(env),
			"app":                 appJSON(app),
			"services":            outSvcs,
			"current_snapshot_id": cur,
			"slots_by_service":    slotsByService,
		})
		return
	}
	if len(parts) >= 2 && parts[1] == "deploy" && r.Method == "POST" {
		if err := s.deployer.ApplyEnv(r.Context(), envID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "apply failed: "+err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if len(parts) >= 4 && parts[1] == "services" && parts[3] == "slot-artifacts" && r.Method == "GET" {
		serviceID := parts[2]
		env, err := s.store.GetEnvByID(r.Context(), envID)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, "unknown env")
			return
		}
		svc, err := s.store.GetServiceByID(r.Context(), serviceID)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, "unknown service")
			return
		}
		if env.AppID != svc.AppID {
			httpx.WriteError(w, http.StatusBadRequest, "env does not belong to this service's app")
			return
		}
		cur, err := s.store.GetEnvCurrentSnapshotID(r.Context(), envID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "db error")
			return
		}
		if cur == nil {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"snapshot_id": nil, "artifacts_by_slot_key": map[string]any{}})
			return
		}
		m, err := s.store.GetSnapshotSlotArtifacts(r.Context(), *cur, serviceID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "db error")
			return
		}
		out := make(map[string]any, len(m))
		for k, a := range m {
			out[k] = artifactJSON(a)
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"snapshot_id": *cur, "artifacts_by_slot_key": out})
		return
	}
	if len(parts) >= 2 && parts[1] == "snapshots" && r.Method == "GET" {
		snaps, err := s.store.ListSnapshots(r.Context(), envID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "db error")
			return
		}
		out := make([]map[string]any, 0, len(snaps))
		for _, sn := range snaps {
			out = append(out, snapshotJSON(sn))
		}
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
	if len(parts) >= 2 && parts[1] == "rollback" && r.Method == "POST" {
		var req struct {
			SnapshotID string `json:"snapshot_id"`
		}
		if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.SnapshotID = strings.TrimSpace(req.SnapshotID)
		if req.SnapshotID == "" {
			httpx.WriteError(w, http.StatusBadRequest, "snapshot_id required")
			return
		}
		if err := s.store.SetEnvCurrentSnapshot(r.Context(), envID, req.SnapshotID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "update failed")
			return
		}
		if err := s.deployer.ApplyEnv(r.Context(), envID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "apply failed: "+err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	httpx.WriteError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleArtifactUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid multipart")
		return
	}
	get := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	autoDeploy := parseAutoDeployFlag(get("deploy"))
	appKey := get("app")
	envName := get("env")
	serviceKey := get("service")
	slotKey := get("slot")
	repoFull := get("repo")
	sha := get("sha")
	ref := get("ref")
	prStr := get("pr_number")

	if appKey == "" || envName == "" || serviceKey == "" || slotKey == "" || repoFull == "" {
		httpx.WriteError(w, http.StatusBadRequest, "app/env/service/slot/repo required")
		return
	}

	var prNumber *int
	if strings.EqualFold(envName, "preview") {
		if prStr == "" {
			httpx.WriteError(w, http.StatusBadRequest, "pr_number required for preview")
			return
		}
		n, err := strconv.Atoi(prStr)
		if err != nil || n <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid pr_number")
			return
		}
		prNumber = &n
	}

	app, err := s.store.GetAppByKey(r.Context(), appKey)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown app")
		return
	}
	repo, err := s.store.GetRepoByFullName(r.Context(), repoFull)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown repo (create it in UI first)")
		return
	}
	svc, err := s.store.GetServiceByKey(r.Context(), app.ID, serviceKey)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown service")
		return
	}
	slot, err := s.store.GetSlotByKey(r.Context(), svc.ID, slotKey)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown slot")
		return
	}
	if slot.RepoID != repo.ID {
		httpx.WriteError(w, http.StatusForbidden, "repo not allowed for this slot")
		return
	}

	var envID string
	if strings.EqualFold(envName, "preview") {
		env, err := s.store.UpsertPreviewEnv(r.Context(), app.ID, *repo, *prNumber)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "env failed")
			return
		}
		envID = env.ID
	} else {
		id, err := s.store.GetEnvIDByName(r.Context(), app.ID, envName)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "unknown env (create it in UI first)")
			return
		}
		envID = id
	}

	file, header, err := r.FormFile("artifact")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "missing artifact file")
		return
	}
	defer httpx.DrainAndClose(file)

	artifactID, err := ids.New()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "id failed")
		return
	}
	artifactDir := filepath.Join(s.opts.DataDir, "artifacts", artifactID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "store failed")
		return
	}
	filename := sanitizeFilename(header.Filename)
	dstPath := filepath.Join(artifactDir, filename)

	sha256Hex, sizeBytes, err := writeFileAndSHA256(dstPath, file)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "write failed")
		return
	}

	tokenID := tokenIDFromContext(r.Context())
	note := "upload"
	if sha != "" {
		note = "upload sha=" + sha
	}
	res, err := s.store.CreateArtifactAndSnapshot(r.Context(), db.UploadParams{
		ArtifactID: artifactID,
		AppID:      app.ID,
		ServiceID:  svc.ID,
		SlotID:     slot.ID,
		RepoID:     repo.ID,
		EnvID:      envID,
		SHA:        sha,
		Ref:        ref,
		PRNumber:   prNumber,
		Filename:   filename,
		SizeBytes:  sizeBytes,
		SHA256Hex:  sha256Hex,
		StoredPath: dstPath,
		TokenID:    tokenID,
		UserID:     nil,
		Note:       note,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "db write failed: "+err.Error())
		return
	}

	deployed := false
	if autoDeploy {
		if err := s.deployer.ApplyService(r.Context(), envID, svc.ID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "deploy failed: "+err.Error())
			return
		}
		deployed = true
	}

	url, _ := s.deployer.ServiceURL(r.Context(), envID, svc.ID)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"artifact_id":    res.ArtifactID,
		"snapshot_id":    res.SnapshotID,
		"env_id":         envID,
		"service_id":     svc.ID,
		"service_url":    url,
		"deployed":       deployed,
		"deploy_skipped": !deployed,
		"sha256_hex":     sha256Hex,
		"stored_path":    dstPath,
		"repo":           repo.FullName,
		"app":            app.AppKey,
		"env":            envName,
		"service":        svc.ServiceKey,
		"slot":           slot.SlotKey,
		"container_id":   nil,
	})
}

func (s *Server) handleArtifactUploadBatch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid multipart")
		return
	}
	get := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	autoDeploy := parseAutoDeployFlag(get("deploy"))

	appKey := get("app")
	envName := get("env")
	serviceKey := get("service")
	repoFull := get("repo")
	sha := get("sha")
	ref := get("ref")
	prStr := get("pr_number")

	if appKey == "" || envName == "" || serviceKey == "" || repoFull == "" {
		httpx.WriteError(w, http.StatusBadRequest, "app/env/service/repo required")
		return
	}

	var prNumber *int
	if strings.EqualFold(envName, "preview") {
		if prStr == "" {
			httpx.WriteError(w, http.StatusBadRequest, "pr_number required for preview")
			return
		}
		n, err := strconv.Atoi(prStr)
		if err != nil || n <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid pr_number")
			return
		}
		prNumber = &n
	}

	app, err := s.store.GetAppByKey(r.Context(), appKey)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown app")
		return
	}
	repo, err := s.store.GetRepoByFullName(r.Context(), repoFull)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown repo (create it in UI first)")
		return
	}
	svc, err := s.store.GetServiceByKey(r.Context(), app.ID, serviceKey)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown service")
		return
	}

	// Resolve env id
	var envID string
	if strings.EqualFold(envName, "preview") {
		env, err := s.store.UpsertPreviewEnv(r.Context(), app.ID, *repo, *prNumber)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "env failed")
			return
		}
		envID = env.ID
	} else {
		id, err := s.store.GetEnvIDByName(r.Context(), app.ID, envName)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "unknown env (create it in UI first)")
			return
		}
		envID = id
	}

	slots, err := s.store.ListSlotsByService(r.Context(), svc.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "db error")
		return
	}
	slotByKey := make(map[string]db.Slot, len(slots))
	for _, sl := range slots {
		slotByKey[sl.SlotKey] = sl
	}

	files := r.MultipartForm.File
	var entries []db.UploadBatchEntry
	artifactIDsBySlot := make(map[string]string)
	for field, fhs := range files {
		if !strings.HasPrefix(field, "file_") {
			continue
		}
		slotKey := strings.TrimPrefix(field, "file_")
		sl, ok := slotByKey[slotKey]
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "unknown slot in upload: "+slotKey)
			return
		}
		if sl.RepoID != repo.ID {
			httpx.WriteError(w, http.StatusForbidden, "repo not allowed for slot: "+slotKey)
			return
		}
		if len(fhs) == 0 {
			continue
		}
		h := fhs[0]
		file, err := h.Open()
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "open file failed")
			return
		}
		defer httpx.DrainAndClose(file)

		artifactID, err := ids.New()
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "id failed")
			return
		}
		artifactDir := filepath.Join(s.opts.DataDir, "artifacts", artifactID)
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "store failed")
			return
		}
		filename := sanitizeFilename(h.Filename)
		dstPath := filepath.Join(artifactDir, filename)
		sha256Hex, sizeBytes, err := writeFileAndSHA256(dstPath, file)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "write failed")
			return
		}

		entries = append(entries, db.UploadBatchEntry{
			ArtifactID: artifactID,
			SlotID:     sl.ID,
			RepoID:     repo.ID,
			SHA:        sha,
			Ref:        ref,
			PRNumber:   prNumber,
			Filename:   filename,
			SizeBytes:  sizeBytes,
			SHA256Hex:  sha256Hex,
			StoredPath: dstPath,
		})
		artifactIDsBySlot[slotKey] = artifactID
	}

	if len(entries) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "no files uploaded (expected fields like file_<slotKey>)")
		return
	}

	tokenID := tokenIDFromContext(r.Context())
	note := "batch upload"
	if sha != "" {
		note = "batch upload sha=" + sha
	}
	res, err := s.store.CreateArtifactsAndSnapshotBatch(r.Context(), db.UploadBatchParams{
		AppID:     app.ID,
		ServiceID: svc.ID,
		EnvID:     envID,
		Entries:   entries,
		TokenID:   tokenID,
		UserID:    nil,
		Note:      note,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "db write failed: "+err.Error())
		return
	}

	deployed := false
	if autoDeploy {
		if err := s.deployer.ApplyService(r.Context(), envID, svc.ID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "deploy failed: "+err.Error())
			return
		}
		deployed = true
	}

	url, _ := s.deployer.ServiceURL(r.Context(), envID, svc.ID)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":                   true,
		"env_id":               envID,
		"service_id":           svc.ID,
		"snapshot_id":          res.SnapshotID,
		"artifact_ids":         res.ArtifactIDs,
		"artifact_ids_by_slot": artifactIDsBySlot,
		"service_url":          url,
		"deployed":             deployed,
		"deploy_skipped":       !deployed,
		"repo":                 repo.FullName,
		"app":                  app.AppKey,
		"env":                  envName,
		"service":              svc.ServiceKey,
	})
}

func writeFileAndSHA256(dstPath string, src multipart.File) (sha256Hex string, size int64, err error) {
	tmp := dstPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	w := io.MultiWriter(f, h)
	n, err := io.Copy(w, src)
	if err != nil {
		_ = os.Remove(tmp)
		return "", 0, err
	}
	if err := f.Sync(); err != nil {
		_ = os.Remove(tmp)
		return "", 0, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", 0, err
	}
	if err := os.Rename(tmp, dstPath); err != nil {
		_ = os.Remove(tmp)
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "artifact.bin"
	}
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	return name
}

func (s *Server) handleAdminServiceArtifactUploadBatch(w http.ResponseWriter, r *http.Request, serviceID string) {
	// Admin-only (session) batch upload for a single service.
	// Form fields:
	// - env_id: target env id (named env)
	// - sha/ref (optional)
	// - file_<slotID>: file for a given slot
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid multipart")
		return
	}
	autoDeploy := parseAutoDeployFlag(strings.TrimSpace(r.FormValue("deploy")))
	envID := strings.TrimSpace(r.FormValue("env_id"))
	sha := strings.TrimSpace(r.FormValue("sha"))
	ref := strings.TrimSpace(r.FormValue("ref"))
	if envID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "env_id required")
		return
	}

	uid := userIDFromContext(r.Context())
	if uid == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "missing session")
		return
	}

	svc, err := s.store.GetServiceByID(r.Context(), serviceID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "unknown service")
		return
	}
	env, err := s.store.GetEnvByID(r.Context(), envID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown env")
		return
	}
	if env.AppID != svc.AppID {
		httpx.WriteError(w, http.StatusBadRequest, "env does not belong to this service's app")
		return
	}
	if env.Kind != "named" {
		httpx.WriteError(w, http.StatusBadRequest, "only named env supported for manual upload")
		return
	}

	slots, err := s.store.ListSlotsByService(r.Context(), serviceID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "db error")
		return
	}
	slotByID := make(map[string]db.Slot, len(slots))
	for _, sl := range slots {
		slotByID[sl.ID] = sl
	}

	files := r.MultipartForm.File
	var entries []db.UploadBatchEntry
	for field, fhs := range files {
		if !strings.HasPrefix(field, "file_") {
			continue
		}
		slotID := strings.TrimPrefix(field, "file_")
		sl, ok := slotByID[slotID]
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "unknown slot_id in upload: "+slotID)
			return
		}
		if len(fhs) == 0 {
			continue
		}
		h := fhs[0]
		file, err := h.Open()
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "open file failed")
			return
		}
		defer httpx.DrainAndClose(file)

		artifactID, err := ids.New()
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "id failed")
			return
		}
		artifactDir := filepath.Join(s.opts.DataDir, "artifacts", artifactID)
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "store failed")
			return
		}
		filename := sanitizeFilename(h.Filename)
		dstPath := filepath.Join(artifactDir, filename)
		sha256Hex, sizeBytes, err := writeFileAndSHA256(dstPath, file)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "write failed")
			return
		}

		entries = append(entries, db.UploadBatchEntry{
			ArtifactID: artifactID,
			SlotID:     sl.ID,
			RepoID:     sl.RepoID,
			SHA:        sha,
			Ref:        ref,
			PRNumber:   nil,
			Filename:   filename,
			SizeBytes:  sizeBytes,
			SHA256Hex:  sha256Hex,
			StoredPath: dstPath,
		})
	}

	if len(entries) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "no files uploaded")
		return
	}

	note := "manual upload"
	if sha != "" {
		note = "manual upload sha=" + sha
	}

	res, err := s.store.CreateArtifactsAndSnapshotBatch(r.Context(), db.UploadBatchParams{
		AppID:     svc.AppID,
		ServiceID: svc.ID,
		EnvID:     env.ID,
		Entries:   entries,
		UserID:    uid,
		TokenID:   nil,
		Note:      note,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "db write failed: "+err.Error())
		return
	}

	deployed := false
	if autoDeploy {
		if err := s.deployer.ApplyService(r.Context(), env.ID, svc.ID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "deploy failed: "+err.Error())
			return
		}
		deployed = true
	}

	url, _ := s.deployer.ServiceURL(r.Context(), env.ID, svc.ID)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":             true,
		"env_id":         env.ID,
		"service_id":     svc.ID,
		"snapshot_id":    res.SnapshotID,
		"artifact_ids":   res.ArtifactIDs,
		"service_url":    url,
		"deployed":       deployed,
		"deploy_skipped": !deployed,
	})
}

func (s *Server) handleServiceStatus(w http.ResponseWriter, r *http.Request, serviceID string) {
	if r.Method != "GET" {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	envID := strings.TrimSpace(r.URL.Query().Get("env_id"))
	st, err := s.deployer.ServiceStatus(r.Context(), envID, serviceID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "status failed: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, st)
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request, serviceID string) {
	if r.Method != "GET" {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	envID := strings.TrimSpace(r.URL.Query().Get("env_id"))
	if envID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "env_id required")
		return
	}

	tail := 200
	if v := strings.TrimSpace(r.URL.Query().Get("tail")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			// Keep it bounded to avoid returning huge payloads.
			if n > 5000 {
				n = 5000
			}
			tail = n
		}
	}

	logs, err := s.deployer.ServiceLogs(r.Context(), envID, serviceID, tail)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "logs failed: "+err.Error())
		return
	}
	// Keep JSON to match the SPA fetch client.
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"env_id": envID, "service_id": serviceID, "tail": tail, "logs": logs})
}

func (s *Server) handleServiceDeploy(w http.ResponseWriter, r *http.Request, serviceID string) {
	if r.Method != "POST" {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		EnvID string `json:"env_id"`
	}
	if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.EnvID = strings.TrimSpace(req.EnvID)
	if req.EnvID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "env_id required")
		return
	}

	svc, err := s.store.GetServiceByID(r.Context(), serviceID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "unknown service")
		return
	}
	env, err := s.store.GetEnvByID(r.Context(), req.EnvID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown env")
		return
	}
	if env.AppID != svc.AppID {
		httpx.WriteError(w, http.StatusBadRequest, "env does not belong to this service's app")
		return
	}

	if err := s.deployer.ApplyService(r.Context(), req.EnvID, serviceID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "deploy failed: "+err.Error())
		return
	}

	url, _ := s.deployer.ServiceURL(r.Context(), req.EnvID, serviceID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "env_id": req.EnvID, "service_id": serviceID, "service_url": url})
}

func (s *Server) handleServiceRedeploy(w http.ResponseWriter, r *http.Request, serviceID string) {
	if r.Method != "POST" {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		EnvID string `json:"env_id"`
	}
	if err := httpx.ReadJSON(w, r, &req, 1<<20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.EnvID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "env_id required")
		return
	}
	if err := s.deployer.RecreateService(r.Context(), req.EnvID, serviceID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "redeploy failed: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type forgejoWebhook struct {
	Action string `json:"action"`
	Repo   struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest struct {
		Number int  `json:"number"`
		Merged bool `json:"merged"`
	} `json:"pull_request"`
}

func (s *Server) handleForgejoWebhook(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-Forgejo-Event")
	if event == "" {
		event = r.Header.Get("X-Gitea-Event")
	}
	if event != "pull_request" {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "read failed")
		return
	}
	defer httpx.DrainAndClose(r.Body)

	var payload forgejoWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if payload.Action != "closed" {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}

	repo, err := s.store.GetRepoByFullName(r.Context(), payload.Repo.FullName)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "unknown repo")
		return
	}

	if err := verifyForgejoSignature(repo.WebhookSecret, body, r.Header); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	envs, err := s.store.FindEnvsForRepoPR(r.Context(), repo.ID, payload.PullRequest.Number)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "db error")
		return
	}
	for _, e := range envs {
		_ = s.deployer.CleanupEnv(r.Context(), e.ID)
		_ = s.store.SoftDeleteEnv(r.Context(), e.ID)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "cleaned": len(envs)})
}

func verifyForgejoSignature(secret string, body []byte, header http.Header) error {
	if secret == "" {
		return errors.New("missing secret")
	}
	sig := header.Get("X-Forgejo-Signature")
	if sig == "" {
		sig = header.Get("X-Gitea-Signature")
	}
	sig = strings.TrimSpace(sig)
	if sig == "" {
		return errors.New("missing signature header")
	}
	expect := hmacSHA256Hex([]byte(secret), body)
	if !httpx.ConstantTimeEquals(strings.ToLower(sig), strings.ToLower(expect)) {
		return errors.New("signature mismatch")
	}
	return nil
}

func hmacSHA256Hex(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) handleComposeTemplateExample(w http.ResponseWriter, r *http.Request) {
	example := `services:
  app:
    image: eclipse-temurin:17-jre
    command: sh -lc "java -jar /app/app.jar"
    volumes:
      {{- range $slotKey, $hostPath := .Artifacts }}
      - {{$hostPath}}:{{index $.SlotPaths $slotKey}}:ro
      {{- end }}
    environment:
      SPRING_PROFILES_ACTIVE: {{.EnvName}}
      {{- range $key, $value := .Env }}
      {{$key}}: {{$value}}
      {{- end }}
    labels:
      - traefik.enable=true
      - traefik.http.routers.{{.RouterName}}.rule=Host(` + "`{{.Host}}`" + `)
      - traefik.http.routers.{{.RouterName}}.entrypoints={{.EntryPoints}}
      - traefik.http.routers.{{.RouterName}}.tls=true
      - traefik.http.services.{{.TraefikService}}.loadbalancer.server.port={{.Port}}
    networks:
      - {{.Network}}
    restart: unless-stopped

networks:
  {{.Network}}:
    external: true

# Available template variables:
# .ServiceID, .ServiceKey, .ServiceName - Service info
# .EnvID, .EnvName, .EnvKind - Environment info (prod/staging/preview)
# .AppID, .AppKey - Application info
# .Artifacts - Map of slot_key -> artifact_path
# .SlotPaths - Map of slot_key -> container_path (from slot config)
# .Host - Resolved hostname for this service
# .RouterName - Traefik router name
# .TraefikService - Traefik service name
# .Port - Container port
# .Network - Docker network name
# .BaseDomain - Base domain
# .EntryPoints - Traefik entrypoints
# .Env - Environment variables map
# .RuntimeDir - Runtime directory path
# .DataDir - Data directory path
# .RepoFullName, .RepoSlug, .PRNumber - For preview environments
`
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"example":     example,
		"description": "Docker Compose template with Go template syntax. Use {{.Variable}} to access data.",
	})
}
