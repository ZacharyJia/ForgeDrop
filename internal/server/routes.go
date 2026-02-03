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

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	// Public: setup + auth
	mux.HandleFunc("/api/v1/setup", method("POST", requireSetupAllowed(s.store, s.handleSetup)))
	mux.HandleFunc("/api/v1/auth/login", method("POST", s.handleLogin))
	mux.HandleFunc("/api/v1/auth/logout", method("POST", s.handleLogout))
	mux.Handle("/api/v1/admin/", s.withJSON(s.requireSession(http.HandlerFunc(s.handleAdmin))))
	mux.Handle("/api/v1/artifacts/upload", s.withJSON(s.requireBearerToken(http.HandlerFunc(s.handleArtifactUpload))))

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
		r2.URL.Path = "/index.html"
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
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"base_domain":            baseDomain,
			"preview_host_template":  tpl,
			"docker_network":         netw,
			"artifact_upload_url":    s.baseURL(r) + "/api/v1/artifacts/upload",
			"forgejo_webhook_url":    s.baseURL(r) + "/webhooks/forgejo",
			"preview_hosting_note":   "configure wildcard DNS and Traefik separately",
			"requires_traefik_label": true,
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
			case "base_domain", "preview_host_template", "docker_network":
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
		for _, t := range tokens {
			out = append(out, map[string]any{
				"id":         t.ID,
				"name":       t.Name,
				"prefix":     t.Prefix,
				"created_at": t.CreatedAt,
				"revoked_at": t.RevokedAt,
			})
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
			"id":     t.ID,
			"name":   t.Name,
			"prefix": t.Prefix,
			"token":  plain,
		})
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
		for _, rr := range repos {
			out = append(out, map[string]any{
				"id":             rr.ID,
				"full_name":      rr.FullName,
				"slug":           rr.Slug,
				"webhook_secret": rr.WebhookSecret,
			})
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
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"id":             repo.ID,
			"full_name":      repo.FullName,
			"slug":           repo.Slug,
			"webhook_secret": repo.WebhookSecret,
		})
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
			for _, a := range apps {
				out = append(out, map[string]any{"id": a.ID, "app_key": a.AppKey, "name": a.Name})
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
			httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": app.ID, "app_key": app.AppKey, "name": app.Name})
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
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": app.ID, "app_key": app.AppKey, "name": app.Name, "services": services, "envs": envs})
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
			httpx.WriteJSON(w, http.StatusOK, svcs)
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
			httpx.WriteJSON(w, http.StatusCreated, svc)
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
			httpx.WriteJSON(w, http.StatusCreated, env)
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
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"service": svc, "slots": slots})
			return
		case "PUT":
			var req struct {
				Name             string            `json:"name"`
				Image            string            `json:"image"`
				Command          string            `json:"command"`
				ContainerPort    int               `json:"container_port"`
				RunUser          string            `json:"run_user"`
				Env              map[string]string `json:"env"`
				ProdHost         string            `json:"prod_host"`
				TraefikEntrypnts string            `json:"traefik_entrypoints"`
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
			if req.Image != "" {
				patch.Image = req.Image
			}
			if req.Command != "" {
				patch.Command = req.Command
			}
			if req.ContainerPort != 0 {
				patch.ContainerPort = req.ContainerPort
			}
			if req.RunUser != "" {
				patch.RunUser = req.RunUser
			}
			if req.Env != nil {
				patch.Env = req.Env
			}
			patch.ProdHost = req.ProdHost
			if req.TraefikEntrypnts != "" {
				patch.TraefikEntrypnts = req.TraefikEntrypnts
			}
			patch.Enabled = req.Enabled
			updated, err := s.store.UpdateService(r.Context(), serviceID, patch)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "update failed")
				return
			}
			httpx.WriteJSON(w, http.StatusOK, updated)
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
	if len(parts) >= 2 && parts[1] == "status" {
		s.handleServiceStatus(w, r, serviceID)
		return
	}
	if len(parts) >= 2 && parts[1] == "redeploy" {
		s.handleServiceRedeploy(w, r, serviceID)
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
			httpx.WriteJSON(w, http.StatusOK, slots)
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
			httpx.WriteJSON(w, http.StatusCreated, slot)
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
		httpx.WriteJSON(w, http.StatusOK, updated)
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
		var slotsByService = map[string][]db.Slot{}
		for _, svc := range services {
			ss, _ := s.store.ListSlotsByService(r.Context(), svc.ID)
			slotsByService[svc.ID] = ss
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"env":              env,
			"app":              app,
			"services":         services,
			"current_snapshot": cur,
			"slots_by_service": slotsByService,
		})
		return
	}
	if len(parts) >= 2 && parts[1] == "snapshots" && r.Method == "GET" {
		snaps, err := s.store.ListSnapshots(r.Context(), envID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "db error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, snaps)
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
		Note:       note,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "db write failed: "+err.Error())
		return
	}

	if err := s.deployer.ApplyService(r.Context(), envID, svc.ID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "deploy failed: "+err.Error())
		return
	}

	url, _ := s.deployer.ServiceURL(r.Context(), envID, svc.ID)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"artifact_id":  res.ArtifactID,
		"snapshot_id":  res.SnapshotID,
		"env_id":       envID,
		"service_id":   svc.ID,
		"service_url":  url,
		"sha256_hex":   sha256Hex,
		"stored_path":  dstPath,
		"repo":         repo.FullName,
		"app":          app.AppKey,
		"env":          envName,
		"service":      svc.ServiceKey,
		"slot":         slot.SlotKey,
		"container_id": nil,
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

func (s *Server) handleServiceStatus(w http.ResponseWriter, r *http.Request, serviceID string) {
	if r.Method != "GET" {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	st, err := s.deployer.ServiceStatus(r.Context(), serviceID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "status failed: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, st)
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
