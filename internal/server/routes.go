package server

import (
	"bytes"
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

	"github.com/gin-gonic/gin"

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

func parseDeployStrategy(v string) string {
	// Default is "recreate" to provide deterministic deployments.
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "recreate"
	}
	switch v {
	case "recreate", "restart":
		return v
	default:
		return "recreate"
	}
}

func resolveDeployStrategy(explicit, appDefault string) string {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return parseDeployStrategy(explicit)
	}
	return parseDeployStrategy(appDefault)
}

func normalizeRepoIDsInput(repoID string, repoIDs []string) []string {
	out := make([]string, 0, len(repoIDs)+1)
	seen := make(map[string]struct{}, len(repoIDs)+1)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	add(repoID)
	for _, rid := range repoIDs {
		add(rid)
	}
	return out
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
		"deploy_strategy":     parseDeployStrategy(svc.DeployStrategy),
		"use_compose":         svc.UseCompose,
		"revision":            svc.Revision,
		"enabled":             svc.Enabled,
		"created_at":          svc.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":          svc.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func slotJSON(sl db.Slot) map[string]any {
	repoIDs := sl.RepoIDs
	if repoIDs == nil {
		repoIDs = []string{}
	}
	return map[string]any{
		"id":             sl.ID,
		"service_id":     sl.ServiceID,
		"slot_key":       sl.SlotKey,
		"name":           sl.Name,
		"repo_id":        sl.PrimaryRepoID(),
		"repo_ids":       repoIDs,
		"mount_type":     sl.MountType,
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
		"change_set":          e.ChangeSet,
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
		"scope":      t.Scope,
		"prefix":     t.Prefix,
		"created_at": t.CreatedAt.UTC().Format(time.RFC3339Nano),
		"revoked_at": nil,
	}
	if t.RevokedAt != nil {
		out["revoked_at"] = t.RevokedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func (s *Server) serveSPA() gin.HandlerFunc {
	sub, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		// should never happen
		return func(c *gin.Context) {
			c.String(http.StatusInternalServerError, "web ui missing")
		}
	}
	fsys := http.FS(sub)
	var indexHTML []byte

	// If the embed only contains a placeholder (e.g. web/dist/.keep), show a
	// helpful message instead of a confusing 404.
	if f, err := sub.Open("index.html"); err != nil {
		return func(c *gin.Context) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusServiceUnavailable)
			_, _ = c.Writer.Write([]byte(`<!doctype html>
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
		}
	} else {
		indexHTML, _ = io.ReadAll(f)
		_ = f.Close()
	}

	serveIndex := func(c *gin.Context) {
		if !s.opts.Dev {
			c.Header("Cache-Control", "no-cache")
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}

	return func(c *gin.Context) {
		// Try to serve a static file first.
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path == "" {
			serveIndex(c)
			return
		}
		if f, err := sub.Open(path); err == nil {
			info, statErr := f.Stat()
			_ = f.Close()
			// Avoid FileServer redirects for index-like requests; respond directly.
			if path == "index.html" {
				serveIndex(c)
				return
			}
			if statErr == nil && !info.IsDir() {
				if !s.opts.Dev {
					c.Header("Cache-Control", "public, max-age=300")
				}
				c.FileFromFS(path, fsys)
				return
			}
		}

		// SPA fallback to index.html without redirect.
		serveIndex(c)
	}
}

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleSetup(c *gin.Context) {
	r := c.Request
	var req setupRequest
	if err := readJSON(c, &req, 1<<20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(c, http.StatusBadRequest, "username/password required")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "hash failed")
		return
	}
	u, err := s.store.CreateUser(r.Context(), req.Username, hash)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "create user failed")
		return
	}
	c.JSON(http.StatusCreated, map[string]any{"user_id": u.ID})
}

func (s *Server) handleSetupStatus(c *gin.Context) {
	r := c.Request
	count, err := s.store.UserCount(r.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "db error")
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"allowed":    count == 0,
		"user_count": count,
	})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(c *gin.Context) {
	r := c.Request
	var req loginRequest
	if err := readJSON(c, &req, 1<<20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.store.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, req.Password) {
		writeError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := auth.NewToken(32)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "token failed")
		return
	}
	_, err = s.store.CreateSession(r.Context(), u.ID, token, 7*24*time.Hour)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "session failed")
		return
	}
	auth.SetSessionCookie(c.Writer, token, auth.CookieOptions{Secure: s.isSecureRequest(r)})
	c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLogout(c *gin.Context) {
	r := c.Request
	_ = r
	auth.ClearSessionCookie(c.Writer)
	c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminMe(c *gin.Context) {
	r := c.Request
	if tokenID := tokenIDFromContext(r.Context()); tokenID != nil {
		tok, err := s.store.GetAPITokenByID(r.Context(), *tokenID)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "invalid token")
			return
		}
		c.JSON(http.StatusOK, map[string]any{
			"id":       tok.ID,
			"username": tok.Name,
		})
		return
	}
	uid := userIDFromContext(r.Context())
	if uid == nil {
		writeError(c, http.StatusUnauthorized, "missing auth")
		return
	}
	u, err := s.store.GetUserByID(r.Context(), *uid)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid session")
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"id":       u.ID,
		"username": u.Username,
	})
}

func (s *Server) handleAdminSettings(c *gin.Context, rest string) {
	r := c.Request
	switch r.Method {
	case "GET":
		baseDomain, _ := s.store.GetSetting(r.Context(), "base_domain")
		namedTpl, _ := s.store.GetSetting(r.Context(), "named_host_template")
		tpl, _ := s.store.GetSetting(r.Context(), "preview_host_template")
		netw, _ := s.store.GetSetting(r.Context(), "docker_network")
		email, _ := s.store.GetSetting(r.Context(), "traefik_acme_email")
		acmeMode, _ := s.store.GetSetting(r.Context(), "traefik_acme_mode")
		regionID, _ := s.store.GetSetting(r.Context(), "traefik_alicloud_region_id")
		wild, _ := s.store.GetSetting(r.Context(), "traefik_wildcard_enabled")
		wildApex, _ := s.store.GetSetting(r.Context(), "traefik_wildcard_include_apex")
		c.JSON(http.StatusOK, map[string]any{
			"base_domain":                   baseDomain,
			"named_host_template":           namedTpl,
			"preview_host_template":         tpl,
			"docker_network":                netw,
			"traefik_acme_email":            email,
			"traefik_acme_mode":             acmeMode,
			"traefik_alicloud_region_id":    regionID,
			"traefik_wildcard_enabled":      wild,
			"traefik_wildcard_include_apex": wildApex,
			"artifact_upload_url":           s.baseURL(r) + "/api/v1/artifacts/upload",
			"forgejo_webhook_url":           s.baseURL(r) + "/webhooks/forgejo",
			"preview_hosting_note":          "configure wildcard DNS and Traefik separately",
			"requires_traefik_label":        true,
		})
		return
	case "PUT":
		var req map[string]string
		if err := readJSON(c, &req, 1<<20); err != nil {
			writeError(c, http.StatusBadRequest, "invalid json")
			return
		}
		for k, v := range req {
			switch k {
			case "base_domain", "named_host_template", "preview_host_template", "docker_network", "traefik_acme_email", "traefik_acme_mode", "traefik_alicloud_region_id", "traefik_wildcard_enabled", "traefik_wildcard_include_apex":
				if err := s.store.SetSetting(r.Context(), k, strings.TrimSpace(v)); err != nil {
					writeError(c, http.StatusInternalServerError, "save failed")
					return
				}
			}
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeError(c, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}

func (s *Server) handleAdminTokens(c *gin.Context, rest string) {
	r := c.Request
	rest = strings.TrimPrefix(rest, "/")
	switch {
	case r.Method == "GET" && rest == "":
		tokens, err := s.store.ListAPITokens(r.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "db error")
			return
		}
		var out []map[string]any
		out = make([]map[string]any, 0, len(tokens))
		for _, t := range tokens {
			tok := t
			out = append(out, apiTokenJSON(&tok))
		}
		c.JSON(http.StatusOK, out)
		return
	case r.Method == "POST" && rest == "":
		var req struct {
			Name  string `json:"name"`
			Scope string `json:"scope"`
		}
		if err := readJSON(c, &req, 1<<20); err != nil {
			writeError(c, http.StatusBadRequest, "invalid json")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeError(c, http.StatusBadRequest, "name required")
			return
		}
		req.Scope = strings.TrimSpace(strings.ToLower(req.Scope))
		if req.Scope == "" {
			req.Scope = "artifact"
		}
		if req.Scope != "artifact" && req.Scope != "admin" {
			writeError(c, http.StatusBadRequest, "scope must be artifact or admin")
			return
		}
		plain, err := auth.NewToken(32)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "token failed")
			return
		}
		t, err := s.store.CreateAPIToken(r.Context(), req.Name, req.Scope, plain)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "create failed")
			return
		}
		c.JSON(http.StatusCreated, map[string]any{
			"token":       apiTokenJSON(t),
			"plain_token": plain,
		})
		return
	case r.Method == "DELETE" && rest != "":
		id := strings.TrimSuffix(rest, "/")
		if id == "" {
			writeError(c, http.StatusBadRequest, "id required")
			return
		}
		if err := s.store.RevokeAPIToken(r.Context(), id); err != nil {
			writeError(c, http.StatusInternalServerError, "revoke failed")
			return
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true})
		return
	case r.Method == "POST" && strings.HasSuffix(rest, "/revoke"):
		id := strings.TrimSuffix(rest, "/revoke")
		id = strings.TrimSuffix(id, "/")
		if id == "" {
			writeError(c, http.StatusBadRequest, "id required")
			return
		}
		if err := s.store.RevokeAPIToken(r.Context(), id); err != nil {
			writeError(c, http.StatusInternalServerError, "revoke failed")
			return
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeError(c, http.StatusNotFound, "not found")
		return
	}
}

func (s *Server) handleAdminRepos(c *gin.Context, rest string) {
	r := c.Request
	rest = strings.TrimPrefix(rest, "/")
	switch {
	case r.Method == "GET" && rest == "":
		repos, err := s.store.ListRepos(r.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "db error")
			return
		}
		var out []map[string]any
		out = make([]map[string]any, 0, len(repos))
		for _, rr := range repos {
			r := rr
			out = append(out, repoJSON(&r))
		}
		c.JSON(http.StatusOK, out)
		return
	case r.Method == "POST" && rest == "":
		var req struct {
			FullName      string `json:"full_name"`
			WebhookSecret string `json:"webhook_secret"`
		}
		if err := readJSON(c, &req, 1<<20); err != nil {
			writeError(c, http.StatusBadRequest, "invalid json")
			return
		}
		req.FullName = strings.TrimSpace(req.FullName)
		if req.FullName == "" {
			writeError(c, http.StatusBadRequest, "full_name required")
			return
		}
		repo, err := s.store.CreateRepo(r.Context(), req.FullName, req.WebhookSecret)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "create failed")
			return
		}
		c.JSON(http.StatusCreated, repoJSON(repo))
		return
	case r.Method == "DELETE" && rest != "":
		id := strings.TrimSuffix(rest, "/")
		if err := s.store.DeleteRepo(r.Context(), id); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeError(c, http.StatusNotFound, "not found")
				return
			}
			writeError(c, http.StatusInternalServerError, "delete failed")
			return
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true})
		return
	case r.Method == "PUT" && rest != "":
		id := rest
		var req struct {
			WebhookSecret string `json:"webhook_secret"`
		}
		if err := readJSON(c, &req, 1<<20); err != nil {
			writeError(c, http.StatusBadRequest, "invalid json")
			return
		}
		if err := s.store.UpdateRepoSecret(r.Context(), id, req.WebhookSecret); err != nil {
			writeError(c, http.StatusInternalServerError, "update failed")
			return
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeError(c, http.StatusNotFound, "not found")
		return
	}
}

func (s *Server) handleAdminApps(c *gin.Context, rest string) {
	r := c.Request
	rest = strings.TrimPrefix(rest, "/")

	// /apps
	if rest == "" {
		switch r.Method {
		case "GET":
			apps, err := s.store.ListApps(r.Context())
			if err != nil {
				writeError(c, http.StatusInternalServerError, "db error")
				return
			}
			var out []map[string]any
			out = make([]map[string]any, 0, len(apps))
			for _, a := range apps {
				app := a
				out = append(out, appJSON(&app))
			}
			c.JSON(http.StatusOK, out)
			return
		case "POST":
			var req struct {
				AppKey string `json:"app_key"`
				Name   string `json:"name"`
			}
			if err := readJSON(c, &req, 1<<20); err != nil {
				writeError(c, http.StatusBadRequest, "invalid json")
				return
			}
			req.AppKey = strings.TrimSpace(req.AppKey)
			req.Name = strings.TrimSpace(req.Name)
			if req.AppKey == "" || req.Name == "" {
				writeError(c, http.StatusBadRequest, "app_key/name required")
				return
			}
			app, err := s.store.CreateApp(r.Context(), req.AppKey, req.Name)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "create failed")
				return
			}
			// Make first-run smoother: create default envs.
			_, _ = s.store.EnsureNamedEnv(r.Context(), app.ID, "prod")
			_, _ = s.store.EnsureNamedEnv(r.Context(), app.ID, "preview")
			c.JSON(http.StatusCreated, appJSON(app))
			return
		default:
			writeError(c, http.StatusMethodNotAllowed, "method not allowed")
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
				writeError(c, http.StatusNotFound, "not found")
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
			c.JSON(http.StatusOK, map[string]any{"app": appJSON(app), "services": outSvcs, "envs": outEnvs})
			return
		case "DELETE":
			if err := s.store.DeleteApp(r.Context(), appID); err != nil {
				writeError(c, http.StatusInternalServerError, "delete failed")
				return
			}
			c.JSON(http.StatusOK, map[string]any{"ok": true})
			return
		default:
			writeError(c, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	// /apps/{appID}/services or /apps/{appID}/envs
	switch parts[1] {
	case "services":
		s.handleAdminAppServices(c, appID, parts[2:])
		return
	case "envs":
		s.handleAdminAppEnvs(c, appID, parts[2:])
		return
	default:
		writeError(c, http.StatusNotFound, "not found")
		return
	}
}

func (s *Server) handleAdminAppServices(c *gin.Context, appID string, rest []string) {
	r := c.Request
	if len(rest) == 0 {
		switch r.Method {
		case "GET":
			svcs, err := s.store.ListServicesByApp(r.Context(), appID)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "db error")
				return
			}
			out := make([]map[string]any, 0, len(svcs))
			for _, svc := range svcs {
				s := svc
				out = append(out, serviceJSON(&s))
			}
			c.JSON(http.StatusOK, out)
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
			if err := readJSON(c, &req, 1<<20); err != nil {
				writeError(c, http.StatusBadRequest, "invalid json")
				return
			}
			req.ServiceKey = strings.TrimSpace(req.ServiceKey)
			req.Name = strings.TrimSpace(req.Name)
			if req.ServiceKey == "" || req.Name == "" {
				writeError(c, http.StatusBadRequest, "service_key/name required")
				return
			}
			svc, err := s.store.CreateService(r.Context(), appID, req.ServiceKey, req.Name, req.Image, req.Command, req.ContainerPort, req.RunUser, req.Env, req.ProdHost)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "create failed")
				return
			}
			c.JSON(http.StatusCreated, serviceJSON(svc))
			return
		default:
			writeError(c, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}
	writeError(c, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminAppEnvs(c *gin.Context, appID string, rest []string) {
	r := c.Request
	if len(rest) == 0 {
		switch r.Method {
		case "GET":
			envs, err := s.store.ListEnvsByApp(r.Context(), appID)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "db error")
				return
			}
			c.JSON(http.StatusOK, envs)
			return
		case "POST":
			var req struct {
				Name string `json:"name"`
			}
			if err := readJSON(c, &req, 1<<20); err != nil {
				writeError(c, http.StatusBadRequest, "invalid json")
				return
			}
			req.Name = strings.TrimSpace(req.Name)
			if req.Name == "" {
				writeError(c, http.StatusBadRequest, "name required")
				return
			}
			env, err := s.store.CreateNamedEnv(r.Context(), appID, req.Name)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "create failed")
				return
			}
			c.JSON(http.StatusCreated, envJSON(env))
			return
		default:
			writeError(c, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}
	writeError(c, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminServices(c *gin.Context, rest string) {
	r := c.Request
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(c, http.StatusNotFound, "not found")
		return
	}
	serviceID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			svc, err := s.store.GetServiceByID(r.Context(), serviceID)
			if err != nil {
				writeError(c, http.StatusNotFound, "not found")
				return
			}
			slots, err := s.store.ListSlotsByService(r.Context(), serviceID)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "db error")
				return
			}
			outSlots := make([]map[string]any, 0, len(slots))
			for _, sl := range slots {
				outSlots = append(outSlots, slotJSON(sl))
			}
			c.JSON(http.StatusOK, map[string]any{"service": serviceJSON(svc), "slots": outSlots})
			return
		case "PUT":
			var req struct {
				Name             *string            `json:"name"`
				ContainerPort    *int               `json:"container_port"`
				Env              *map[string]string `json:"env"`
				ProdHost         *string            `json:"prod_host"`
				TraefikEntrypnts *string            `json:"traefik_entrypoints"`
				ComposeTemplate  *string            `json:"compose_template"`
				DeployStrategy   *string            `json:"deploy_strategy"`
				Enabled          *bool              `json:"enabled"`
			}
			if err := readJSON(c, &req, 1<<20); err != nil {
				writeError(c, http.StatusBadRequest, "invalid json")
				return
			}
			svc, err := s.store.GetServiceByID(r.Context(), serviceID)
			if err != nil {
				writeError(c, http.StatusNotFound, "not found")
				return
			}
			patch := *svc
			if req.Name != nil {
				patch.Name = strings.TrimSpace(*req.Name)
			}
			if req.ContainerPort != nil {
				patch.ContainerPort = *req.ContainerPort
			}
			if req.Env != nil {
				patch.Env = *req.Env
			}
			if req.ProdHost != nil {
				patch.ProdHost = strings.TrimSpace(*req.ProdHost)
			}
			if req.TraefikEntrypnts != nil {
				patch.TraefikEntrypnts = strings.TrimSpace(*req.TraefikEntrypnts)
			}
			if req.ComposeTemplate != nil {
				patch.ComposeTemplate = *req.ComposeTemplate
				patch.UseCompose = true
			}
			if req.DeployStrategy != nil {
				patch.DeployStrategy = parseDeployStrategy(*req.DeployStrategy)
			}
			if req.Enabled != nil {
				patch.Enabled = *req.Enabled
			}
			updated, err := s.store.UpdateService(r.Context(), serviceID, patch)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "update failed")
				return
			}
			c.JSON(http.StatusOK, serviceJSON(updated))
			return
		case "DELETE":
			if err := s.store.DeleteService(r.Context(), serviceID); err != nil {
				writeError(c, http.StatusInternalServerError, "delete failed")
				return
			}
			c.JSON(http.StatusOK, map[string]any{"ok": true})
			return
		default:
			writeError(c, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	if len(parts) >= 2 && parts[1] == "slots" {
		s.handleAdminSlots(c, serviceID, parts[2:])
		return
	}
	if len(parts) >= 2 && parts[1] == "artifacts" {
		s.handleAdminServiceArtifacts(c, serviceID, parts[2:])
		return
	}
	if len(parts) >= 2 && parts[1] == "status" {
		s.handleServiceStatus(c, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "logs" {
		s.handleServiceLogs(c, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "deploy" {
		s.handleServiceDeploy(c, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "redeploy" {
		s.handleServiceRedeploy(c, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "compose-template-example" && r.Method == "GET" {
		s.handleComposeTemplateExample(c)
		return
	}
	writeError(c, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminServiceArtifacts(c *gin.Context, serviceID string, rest []string) {
	r := c.Request
	// /services/{serviceID}/artifacts/upload-batch
	if len(rest) == 1 && rest[0] == "upload-batch" {
		if r.Method != "POST" {
			writeError(c, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleAdminServiceArtifactUploadBatch(c, serviceID)
		return
	}
	writeError(c, http.StatusNotFound, "not found")
}

func (s *Server) handleAdminArtifactDownload(c *gin.Context) {
	r := c.Request
	artifactID := strings.TrimSpace(c.Param("artifactID"))
	if artifactID == "" {
		writeError(c, http.StatusBadRequest, "artifact_id required")
		return
	}

	artifact, err := s.store.GetArtifactByID(r.Context(), artifactID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "artifact not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "db error")
		return
	}

	storedPath := filepath.Clean(strings.TrimSpace(artifact.StoredPath))
	artifactDir := filepath.Clean(filepath.Join(s.opts.DataDir, "artifacts", artifact.ID))
	if storedPath == "" || !strings.HasPrefix(storedPath, artifactDir+string(os.PathSeparator)) {
		writeError(c, http.StatusInternalServerError, "artifact path invalid")
		return
	}
	if _, err := os.Stat(storedPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(c, http.StatusNotFound, "artifact file missing")
			return
		}
		writeError(c, http.StatusInternalServerError, "artifact file inaccessible")
		return
	}

	downloadName := sanitizeFilename(artifact.OriginalFilename)
	c.FileAttachment(storedPath, downloadName)
}

func (s *Server) handleAdminSlots(c *gin.Context, serviceID string, rest []string) {
	r := c.Request
	if len(rest) == 0 {
		switch r.Method {
		case "GET":
			slots, err := s.store.ListSlotsByService(r.Context(), serviceID)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "db error")
				return
			}
			out := make([]map[string]any, 0, len(slots))
			for _, sl := range slots {
				out = append(out, slotJSON(sl))
			}
			c.JSON(http.StatusOK, out)
			return
		case "POST":
			var req struct {
				SlotKey       string   `json:"slot_key"`
				Name          string   `json:"name"`
				RepoID        string   `json:"repo_id"`
				RepoIDs       []string `json:"repo_ids"`
				MountType     string   `json:"mount_type"`
				ContainerPath string   `json:"container_path"`
			}
			if err := readJSON(c, &req, 1<<20); err != nil {
				writeError(c, http.StatusBadRequest, "invalid json")
				return
			}
			req.SlotKey = strings.TrimSpace(req.SlotKey)
			req.Name = strings.TrimSpace(req.Name)
			repoIDs := normalizeRepoIDsInput(req.RepoID, req.RepoIDs)
			if req.SlotKey == "" || req.Name == "" || len(repoIDs) == 0 || req.ContainerPath == "" {
				writeError(c, http.StatusBadRequest, "slot_key/name/repo_ids/container_path required")
				return
			}
			slot, err := s.store.CreateSlot(r.Context(), serviceID, req.SlotKey, req.Name, req.ContainerPath, req.MountType, repoIDs)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "create failed")
				return
			}
			c.JSON(http.StatusCreated, slotJSON(*slot))
			return
		default:
			writeError(c, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	// /services/{id}/slots/{slotID}
	slotID := rest[0]
	switch r.Method {
	case "PUT":
		var req struct {
			Name          string   `json:"name"`
			RepoID        string   `json:"repo_id"`
			RepoIDs       []string `json:"repo_ids"`
			MountType     string   `json:"mount_type"`
			ContainerPath string   `json:"container_path"`
		}
		if err := readJSON(c, &req, 1<<20); err != nil {
			writeError(c, http.StatusBadRequest, "invalid json")
			return
		}
		slot, err := s.store.GetSlotByID(r.Context(), slotID)
		if err != nil {
			writeError(c, http.StatusNotFound, "not found")
			return
		}
		patch := *slot
		if req.Name != "" {
			patch.Name = req.Name
		}
		if req.RepoID != "" {
			patch.RepoID = req.RepoID
		}
		if req.MountType != "" {
			patch.MountType = req.MountType
		}
		if req.ContainerPath != "" {
			patch.ContainerPath = req.ContainerPath
		}
		repoIDs := normalizeRepoIDsInput(req.RepoID, req.RepoIDs)
		replaceRepoIDs := req.RepoID != "" || req.RepoIDs != nil
		if replaceRepoIDs {
			if len(repoIDs) == 0 {
				writeError(c, http.StatusBadRequest, "repo_ids required")
				return
			}
			patch.RepoIDs = repoIDs
		}
		updated, err := s.store.UpdateSlot(r.Context(), slotID, patch, replaceRepoIDs)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "update failed")
			return
		}
		c.JSON(http.StatusOK, slotJSON(*updated))
		return
	case "DELETE":
		if err := s.store.DeleteSlot(r.Context(), slotID); err != nil {
			writeError(c, http.StatusInternalServerError, "delete failed")
			return
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeError(c, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}

func (s *Server) handleAdminEnvs(c *gin.Context, rest string) {
	r := c.Request
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(c, http.StatusNotFound, "not found")
		return
	}
	envID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			// continue below
		case "DELETE":
			if _, err := s.store.GetEnvByID(r.Context(), envID); err != nil {
				writeError(c, http.StatusNotFound, "not found")
				return
			}
			if err := s.deployer.CleanupEnv(r.Context(), envID); err != nil {
				writeError(c, http.StatusInternalServerError, "cleanup failed: "+err.Error())
				return
			}
			if err := s.store.SoftDeleteEnv(r.Context(), envID); err != nil {
				writeError(c, http.StatusInternalServerError, "delete failed")
				return
			}
			c.JSON(http.StatusOK, map[string]any{"ok": true})
			return
		default:
			writeError(c, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		env, err := s.store.GetEnvByID(r.Context(), envID)
		if err != nil {
			writeError(c, http.StatusNotFound, "not found")
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
		c.JSON(http.StatusOK, map[string]any{
			"env":                 envJSON(env),
			"app":                 appJSON(app),
			"services":            outSvcs,
			"current_snapshot_id": cur,
			"slots_by_service":    slotsByService,
		})
		return
	}
	if len(parts) >= 2 && parts[1] == "deploy" && r.Method == "POST" {
		explicit := ""
		if r.ContentLength > 0 {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				writeError(c, http.StatusBadRequest, "read failed")
				return
			}
			defer httpx.DrainAndClose(r.Body)
			if len(bytes.TrimSpace(body)) > 0 {
				var req struct {
					Strategy string `json:"strategy"`
				}
				if err := json.Unmarshal(body, &req); err != nil {
					writeError(c, http.StatusBadRequest, "invalid json")
					return
				}
				explicit = strings.TrimSpace(req.Strategy)
			}
		}
		strategy := strings.TrimSpace(explicit)
		if strategy != "" {
			strategy = parseDeployStrategy(strategy)
		}
		if err := s.deployer.DeployEnv(r.Context(), envID, strategy); err != nil {
			writeError(c, http.StatusInternalServerError, "apply failed: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true})
		return
	}
	if len(parts) >= 4 && parts[1] == "services" && parts[3] == "slot-artifacts" && r.Method == "GET" {
		serviceID := parts[2]
		env, err := s.store.GetEnvByID(r.Context(), envID)
		if err != nil {
			writeError(c, http.StatusNotFound, "unknown env")
			return
		}
		svc, err := s.store.GetServiceByID(r.Context(), serviceID)
		if err != nil {
			writeError(c, http.StatusNotFound, "unknown service")
			return
		}
		if env.AppID != svc.AppID {
			writeError(c, http.StatusBadRequest, "env does not belong to this service's app")
			return
		}
		m, cur, err := s.store.GetEffectiveSlotArtifacts(r.Context(), envID, serviceID)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "db error")
			return
		}
		out := make(map[string]any, len(m))
		for k, a := range m {
			out[k] = artifactJSON(a)
		}
		c.JSON(http.StatusOK, map[string]any{"snapshot_id": cur, "artifacts_by_slot_key": out})
		return
	}
	if len(parts) >= 2 && parts[1] == "sync-preview-snapshot" && r.Method == "POST" {
		env, err := s.store.GetEnvByID(r.Context(), envID)
		if err != nil {
			writeError(c, http.StatusNotFound, "unknown env")
			return
		}
		if env.Kind != "preview" || env.RepoID == nil || (env.PRNumber == nil && env.ChangeSet == nil) {
			writeError(c, http.StatusBadRequest, "only repo-scoped preview env can sync from preview template")
			return
		}
		tplEnvID, err := s.store.GetEnvIDByName(r.Context(), env.AppID, "preview")
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeError(c, http.StatusBadRequest, "preview template env not found")
				return
			}
			writeError(c, http.StatusInternalServerError, "db error")
			return
		}
		tplSnap, err := s.store.GetEnvCurrentSnapshotID(r.Context(), tplEnvID)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "db error")
			return
		}
		if tplSnap == nil {
			writeError(c, http.StatusBadRequest, "preview template has no snapshot yet")
			return
		}
		snapID := strings.TrimSpace(*tplSnap)
		if snapID == "" {
			writeError(c, http.StatusBadRequest, "preview template has no snapshot yet")
			return
		}
		if err := s.store.SetEnvCurrentSnapshot(r.Context(), envID, snapID); err != nil {
			writeError(c, http.StatusInternalServerError, "update failed")
			return
		}
		if err := s.deployer.DeployEnv(r.Context(), envID, ""); err != nil {
			writeError(c, http.StatusInternalServerError, "apply failed: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true, "snapshot_id": snapID})
		return
	}
	if len(parts) >= 2 && parts[1] == "snapshots" && r.Method == "GET" {
		snaps, err := s.store.ListSnapshots(r.Context(), envID)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "db error")
			return
		}
		out := make([]map[string]any, 0, len(snaps))
		for _, sn := range snaps {
			out = append(out, snapshotJSON(sn))
		}
		c.JSON(http.StatusOK, out)
		return
	}
	if len(parts) >= 2 && parts[1] == "rollback" && r.Method == "POST" {
		var req struct {
			SnapshotID string `json:"snapshot_id"`
		}
		if err := readJSON(c, &req, 1<<20); err != nil {
			writeError(c, http.StatusBadRequest, "invalid json")
			return
		}
		req.SnapshotID = strings.TrimSpace(req.SnapshotID)
		if req.SnapshotID == "" {
			writeError(c, http.StatusBadRequest, "snapshot_id required")
			return
		}
		if err := s.store.SetEnvCurrentSnapshot(r.Context(), envID, req.SnapshotID); err != nil {
			writeError(c, http.StatusInternalServerError, "update failed")
			return
		}
		if err := s.deployer.DeployEnv(r.Context(), envID, ""); err != nil {
			writeError(c, http.StatusInternalServerError, "apply failed: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, map[string]any{"ok": true})
		return
	}
	writeError(c, http.StatusNotFound, "not found")
}

func (s *Server) handleArtifactUpload(c *gin.Context) {
	r := c.Request
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid multipart")
		return
	}
	get := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	autoDeploy := parseAutoDeployFlag(get("deploy"))
	deployStrategyRaw := get("deploy_strategy")
	appKey := get("app")
	envName := get("env")
	envKind := strings.ToLower(strings.TrimSpace(get("env_kind")))
	serviceKey := get("service")
	slotKey := get("slot")
	repoFull := get("repo")
	sha := get("sha")
	ref := get("ref")
	prStr := get("pr_number")
	changeSet := get("change_set")
	if strings.TrimSpace(changeSet) == "" {
		changeSet = get("chagne_set")
	}

	if appKey == "" || envName == "" || serviceKey == "" || slotKey == "" || repoFull == "" {
		writeError(c, http.StatusBadRequest, "app/env/service/slot/repo required")
		return
	}

	var prNumber *int
	if strings.TrimSpace(prStr) != "" {
		n, err := strconv.Atoi(prStr)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "invalid pr_number")
			return
		}
		prNumber = &n
	}
	useNamedPreview := strings.EqualFold(envName, "preview") && envKind == "named"
	if useNamedPreview && (prNumber != nil || strings.TrimSpace(changeSet) != "") {
		writeError(c, http.StatusBadRequest, "pr_number/change_set cannot be used when env_kind=named")
		return
	}
	if strings.EqualFold(envName, "preview") && !useNamedPreview && strings.TrimSpace(changeSet) == "" && prNumber == nil {
		writeError(c, http.StatusBadRequest, "pr_number or change_set required for preview (or set env_kind=named)")
		return
	}

	app, err := s.store.GetAppByKey(r.Context(), appKey)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown app")
		return
	}
	repo, err := s.store.GetRepoByFullName(r.Context(), repoFull)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown repo (create it in UI first)")
		return
	}
	svc, err := s.store.GetServiceByKey(r.Context(), app.ID, serviceKey)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown service")
		return
	}
	deployStrategy := resolveDeployStrategy(deployStrategyRaw, svc.DeployStrategy)
	slot, err := s.store.GetSlotByKey(r.Context(), svc.ID, slotKey)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown slot")
		return
	}
	if !slot.AllowsRepo(repo.ID) {
		writeError(c, http.StatusForbidden, "repo not allowed for this slot")
		return
	}

	var envID string
	if strings.EqualFold(envName, "preview") {
		if useNamedPreview {
			id, err := s.store.GetEnvIDByName(r.Context(), app.ID, "preview")
			if err != nil {
				writeError(c, http.StatusBadRequest, "unknown env (create preview env in UI first)")
				return
			}
			envID = id
		} else {
			env, err := s.store.UpsertPreviewEnv(r.Context(), app.ID, *repo, prNumber, changeSet)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "env failed")
				return
			}
			envID = env.ID
		}
	} else {
		id, err := s.store.GetEnvIDByName(r.Context(), app.ID, envName)
		if err != nil {
			writeError(c, http.StatusBadRequest, "unknown env (create it in UI first)")
			return
		}
		envID = id
	}

	file, header, err := r.FormFile("artifact")
	if err != nil {
		writeError(c, http.StatusBadRequest, "missing artifact file")
		return
	}
	defer httpx.DrainAndClose(file)

	artifactID, err := ids.New()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "id failed")
		return
	}
	artifactDir := filepath.Join(s.opts.DataDir, "artifacts", artifactID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		writeError(c, http.StatusInternalServerError, "store failed")
		return
	}
	filename := sanitizeFilename(header.Filename)
	if err := validateUploadByMountType(*slot, filename); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	dstPath := filepath.Join(artifactDir, filename)

	sha256Hex, sizeBytes, err := writeFileAndSHA256(dstPath, file)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "write failed")
		return
	}

	tokenID := tokenIDFromContext(r.Context())
	note := "upload"
	if sha != "" {
		note = "upload sha=" + sha
	}
	if strings.TrimSpace(changeSet) != "" {
		note += " change_set=" + strings.TrimSpace(changeSet)
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
		writeError(c, http.StatusInternalServerError, "db write failed: "+err.Error())
		return
	}

	deployed := false
	if autoDeploy {
		if err := s.deployer.DeployService(r.Context(), envID, svc.ID, deployStrategy); err != nil {
			writeError(c, http.StatusInternalServerError, "deploy failed: "+err.Error())
			return
		}
		deployed = true
	}

	url, _ := s.deployer.ServiceURL(r.Context(), envID, svc.ID)
	c.JSON(http.StatusCreated, map[string]any{
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
		"change_set":     strings.TrimSpace(changeSet),
		"container_id":   nil,
	})
}

func (s *Server) handleArtifactUploadBatch(c *gin.Context) {
	r := c.Request
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid multipart")
		return
	}
	get := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	autoDeploy := parseAutoDeployFlag(get("deploy"))
	deployStrategyRaw := get("deploy_strategy")

	appKey := get("app")
	envName := get("env")
	envKind := strings.ToLower(strings.TrimSpace(get("env_kind")))
	serviceKey := get("service")
	repoFull := get("repo")
	sha := get("sha")
	ref := get("ref")
	prStr := get("pr_number")
	changeSet := get("change_set")
	if strings.TrimSpace(changeSet) == "" {
		changeSet = get("chagne_set")
	}

	if appKey == "" || envName == "" || serviceKey == "" || repoFull == "" {
		writeError(c, http.StatusBadRequest, "app/env/service/repo required")
		return
	}

	var prNumber *int
	if strings.TrimSpace(prStr) != "" {
		n, err := strconv.Atoi(prStr)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "invalid pr_number")
			return
		}
		prNumber = &n
	}
	useNamedPreview := strings.EqualFold(envName, "preview") && envKind == "named"
	if useNamedPreview && (prNumber != nil || strings.TrimSpace(changeSet) != "") {
		writeError(c, http.StatusBadRequest, "pr_number/change_set cannot be used when env_kind=named")
		return
	}
	if strings.EqualFold(envName, "preview") && !useNamedPreview && strings.TrimSpace(changeSet) == "" && prNumber == nil {
		writeError(c, http.StatusBadRequest, "pr_number or change_set required for preview (or set env_kind=named)")
		return
	}

	app, err := s.store.GetAppByKey(r.Context(), appKey)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown app")
		return
	}
	repo, err := s.store.GetRepoByFullName(r.Context(), repoFull)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown repo (create it in UI first)")
		return
	}
	svc, err := s.store.GetServiceByKey(r.Context(), app.ID, serviceKey)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown service")
		return
	}
	deployStrategy := resolveDeployStrategy(deployStrategyRaw, svc.DeployStrategy)

	// Resolve env id
	var envID string
	if strings.EqualFold(envName, "preview") {
		if useNamedPreview {
			id, err := s.store.GetEnvIDByName(r.Context(), app.ID, "preview")
			if err != nil {
				writeError(c, http.StatusBadRequest, "unknown env (create preview env in UI first)")
				return
			}
			envID = id
		} else {
			env, err := s.store.UpsertPreviewEnv(r.Context(), app.ID, *repo, prNumber, changeSet)
			if err != nil {
				writeError(c, http.StatusInternalServerError, "env failed")
				return
			}
			envID = env.ID
		}
	} else {
		id, err := s.store.GetEnvIDByName(r.Context(), app.ID, envName)
		if err != nil {
			writeError(c, http.StatusBadRequest, "unknown env (create it in UI first)")
			return
		}
		envID = id
	}

	slots, err := s.store.ListSlotsByService(r.Context(), svc.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "db error")
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
			writeError(c, http.StatusBadRequest, "unknown slot in upload: "+slotKey)
			return
		}
		if !sl.AllowsRepo(repo.ID) {
			writeError(c, http.StatusForbidden, "repo not allowed for slot: "+slotKey)
			return
		}
		if len(fhs) == 0 {
			continue
		}
		h := fhs[0]
		file, err := h.Open()
		if err != nil {
			writeError(c, http.StatusBadRequest, "open file failed")
			return
		}
		defer httpx.DrainAndClose(file)

		artifactID, err := ids.New()
		if err != nil {
			writeError(c, http.StatusInternalServerError, "id failed")
			return
		}
		artifactDir := filepath.Join(s.opts.DataDir, "artifacts", artifactID)
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			writeError(c, http.StatusInternalServerError, "store failed")
			return
		}
		filename := sanitizeFilename(h.Filename)
		if err := validateUploadByMountType(sl, filename); err != nil {
			writeError(c, http.StatusBadRequest, err.Error())
			return
		}
		dstPath := filepath.Join(artifactDir, filename)
		sha256Hex, sizeBytes, err := writeFileAndSHA256(dstPath, file)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "write failed")
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
		writeError(c, http.StatusBadRequest, "no files uploaded (expected fields like file_<slotKey>)")
		return
	}

	tokenID := tokenIDFromContext(r.Context())
	note := "batch upload"
	if sha != "" {
		note = "batch upload sha=" + sha
	}
	if strings.TrimSpace(changeSet) != "" {
		note += " change_set=" + strings.TrimSpace(changeSet)
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
		writeError(c, http.StatusInternalServerError, "db write failed: "+err.Error())
		return
	}

	deployed := false
	if autoDeploy {
		if err := s.deployer.DeployService(r.Context(), envID, svc.ID, deployStrategy); err != nil {
			writeError(c, http.StatusInternalServerError, "deploy failed: "+err.Error())
			return
		}
		deployed = true
	}

	url, _ := s.deployer.ServiceURL(r.Context(), envID, svc.ID)
	c.JSON(http.StatusCreated, map[string]any{
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
		"change_set":           strings.TrimSpace(changeSet),
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

func isSupportedArchiveFilename(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(n, ".zip") || strings.HasSuffix(n, ".tar") || strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz")
}

func validateUploadByMountType(slot db.Slot, filename string) error {
	if strings.TrimSpace(slot.MountType) != "dir" {
		return nil
	}
	if isSupportedArchiveFilename(filename) {
		return nil
	}
	return errors.New("dir mount only accepts archive files (.zip/.tar/.tar.gz/.tgz)")
}

func (s *Server) handleAdminServiceArtifactUploadBatch(c *gin.Context, serviceID string) {
	r := c.Request
	// Admin-only (session) batch upload for a single service.
	// Form fields:
	// - env_id: target env id (named env)
	// - sha/ref (optional)
	// - file_<slotID>: file for a given slot
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid multipart")
		return
	}
	autoDeploy := parseAutoDeployFlag(strings.TrimSpace(r.FormValue("deploy")))
	deployStrategyRaw := strings.TrimSpace(r.FormValue("deploy_strategy"))
	envID := strings.TrimSpace(r.FormValue("env_id"))
	sha := strings.TrimSpace(r.FormValue("sha"))
	ref := strings.TrimSpace(r.FormValue("ref"))
	if envID == "" {
		writeError(c, http.StatusBadRequest, "env_id required")
		return
	}

	uid := userIDFromContext(r.Context())
	if uid == nil {
		writeError(c, http.StatusUnauthorized, "missing session")
		return
	}

	svc, err := s.store.GetServiceByID(r.Context(), serviceID)
	if err != nil {
		writeError(c, http.StatusNotFound, "unknown service")
		return
	}
	env, err := s.store.GetEnvByID(r.Context(), envID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown env")
		return
	}
	if env.AppID != svc.AppID {
		writeError(c, http.StatusBadRequest, "env does not belong to this service's app")
		return
	}
	if env.Kind != "named" {
		writeError(c, http.StatusBadRequest, "only named env supported for manual upload")
		return
	}
	deployStrategy := resolveDeployStrategy(deployStrategyRaw, svc.DeployStrategy)

	slots, err := s.store.ListSlotsByService(r.Context(), serviceID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "db error")
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
			writeError(c, http.StatusBadRequest, "unknown slot_id in upload: "+slotID)
			return
		}
		if len(fhs) == 0 {
			continue
		}
		h := fhs[0]
		file, err := h.Open()
		if err != nil {
			writeError(c, http.StatusBadRequest, "open file failed")
			return
		}
		defer httpx.DrainAndClose(file)

		artifactID, err := ids.New()
		if err != nil {
			writeError(c, http.StatusInternalServerError, "id failed")
			return
		}
		artifactDir := filepath.Join(s.opts.DataDir, "artifacts", artifactID)
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			writeError(c, http.StatusInternalServerError, "store failed")
			return
		}
		filename := sanitizeFilename(h.Filename)
		if err := validateUploadByMountType(sl, filename); err != nil {
			writeError(c, http.StatusBadRequest, err.Error())
			return
		}
		dstPath := filepath.Join(artifactDir, filename)
		sha256Hex, sizeBytes, err := writeFileAndSHA256(dstPath, file)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "write failed")
			return
		}
		primaryRepoID := sl.PrimaryRepoID()
		if primaryRepoID == "" {
			writeError(c, http.StatusBadRequest, "slot has no repo binding: "+sl.SlotKey)
			return
		}

		entries = append(entries, db.UploadBatchEntry{
			ArtifactID: artifactID,
			SlotID:     sl.ID,
			RepoID:     primaryRepoID,
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
		writeError(c, http.StatusBadRequest, "no files uploaded")
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
		writeError(c, http.StatusInternalServerError, "db write failed: "+err.Error())
		return
	}

	deployed := false
	if autoDeploy {
		if err := s.deployer.DeployService(r.Context(), env.ID, svc.ID, deployStrategy); err != nil {
			writeError(c, http.StatusInternalServerError, "deploy failed: "+err.Error())
			return
		}
		deployed = true
	}

	url, _ := s.deployer.ServiceURL(r.Context(), env.ID, svc.ID)
	c.JSON(http.StatusCreated, map[string]any{
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

func (s *Server) handleServiceStatus(c *gin.Context, serviceID string) {
	r := c.Request
	if r.Method != "GET" {
		writeError(c, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	envID := strings.TrimSpace(r.URL.Query().Get("env_id"))
	st, err := s.deployer.ServiceStatus(r.Context(), envID, serviceID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "status failed: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, st)
}

func (s *Server) handleServiceLogs(c *gin.Context, serviceID string) {
	r := c.Request
	if r.Method != "GET" {
		writeError(c, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	envID := strings.TrimSpace(r.URL.Query().Get("env_id"))
	if envID == "" {
		writeError(c, http.StatusBadRequest, "env_id required")
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
		writeError(c, http.StatusInternalServerError, "logs failed: "+err.Error())
		return
	}
	// Keep JSON to match the SPA fetch client.
	c.JSON(http.StatusOK, map[string]any{"env_id": envID, "service_id": serviceID, "tail": tail, "logs": logs})
}

func (s *Server) handleServiceDeploy(c *gin.Context, serviceID string) {
	r := c.Request
	if r.Method != "POST" {
		writeError(c, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		EnvID    string `json:"env_id"`
		Strategy string `json:"strategy"`
	}
	if err := readJSON(c, &req, 1<<20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid json")
		return
	}
	req.EnvID = strings.TrimSpace(req.EnvID)
	req.Strategy = strings.TrimSpace(req.Strategy)
	if req.EnvID == "" {
		writeError(c, http.StatusBadRequest, "env_id required")
		return
	}

	svc, err := s.store.GetServiceByID(r.Context(), serviceID)
	if err != nil {
		writeError(c, http.StatusNotFound, "unknown service")
		return
	}
	env, err := s.store.GetEnvByID(r.Context(), req.EnvID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown env")
		return
	}
	if env.AppID != svc.AppID {
		writeError(c, http.StatusBadRequest, "env does not belong to this service's app")
		return
	}
	strategy := resolveDeployStrategy(req.Strategy, svc.DeployStrategy)

	if err := s.deployer.DeployService(r.Context(), req.EnvID, serviceID, strategy); err != nil {
		writeError(c, http.StatusInternalServerError, "deploy failed: "+err.Error())
		return
	}

	url, _ := s.deployer.ServiceURL(r.Context(), req.EnvID, serviceID)
	c.JSON(http.StatusOK, map[string]any{"ok": true, "env_id": req.EnvID, "service_id": serviceID, "service_url": url})
}

func (s *Server) handleServiceRedeploy(c *gin.Context, serviceID string) {
	r := c.Request
	if r.Method != "POST" {
		writeError(c, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		EnvID string `json:"env_id"`
	}
	if err := readJSON(c, &req, 1<<20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid json")
		return
	}
	if req.EnvID == "" {
		writeError(c, http.StatusBadRequest, "env_id required")
		return
	}
	if err := s.deployer.DeployService(r.Context(), req.EnvID, serviceID, "recreate"); err != nil {
		writeError(c, http.StatusInternalServerError, "redeploy failed: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, map[string]any{"ok": true})
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

func (s *Server) handleForgejoWebhook(c *gin.Context) {
	r := c.Request
	event := r.Header.Get("X-Forgejo-Event")
	if event == "" {
		event = r.Header.Get("X-Gitea-Event")
	}
	if event != "pull_request" {
		c.JSON(http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		writeError(c, http.StatusBadRequest, "read failed")
		return
	}
	defer httpx.DrainAndClose(r.Body)

	var payload forgejoWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(c, http.StatusBadRequest, "invalid json")
		return
	}
	if payload.Action != "closed" {
		c.JSON(http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}

	repo, err := s.store.GetRepoByFullName(r.Context(), payload.Repo.FullName)
	if err != nil {
		writeError(c, http.StatusBadRequest, "unknown repo")
		return
	}

	if err := verifyForgejoSignature(repo.WebhookSecret, body, r.Header); err != nil {
		writeError(c, http.StatusUnauthorized, "invalid signature")
		return
	}

	envs, err := s.store.FindEnvsForRepoPR(r.Context(), repo.ID, payload.PullRequest.Number)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "db error")
		return
	}
	for _, e := range envs {
		_ = s.deployer.CleanupEnv(r.Context(), e.ID)
		_ = s.store.SoftDeleteEnv(r.Context(), e.ID)
	}
	c.JSON(http.StatusOK, map[string]any{"ok": true, "cleaned": len(envs)})
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

func (s *Server) handleComposeTemplateExample(c *gin.Context) {
	example := `services:
  app:
    image: debian:trixie-slim
    command: sh -lc "chmod +x /app/mes && /app/mes"
    volumes:
      {{- range $slotKey, $hostPath := .Artifacts }}
      - {{$hostPath}}:{{index $.SlotPaths $slotKey}}
      {{- end }}
    labels:
      - traefik.enable=true
      - traefik.http.routers.{{.RouterName}}.rule=Host(` + "`{{.Host}}`" + `)
      - traefik.http.routers.{{.RouterName}}.entrypoints={{.EntryPoints}}
      - traefik.http.routers.{{.RouterName}}.tls=true
      - traefik.http.routers.{{.RouterName}}.tls.certresolver=le
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
# .RepoFullName, .RepoSlug, .PRNumber, .ChangeSet - For preview environments
`
	c.JSON(http.StatusOK, map[string]any{
		"example":     example,
		"description": "Docker Compose template with Go template syntax. Use {{.Variable}} to access data.",
	})
}
