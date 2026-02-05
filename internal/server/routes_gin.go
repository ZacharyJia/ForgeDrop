package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"forge-drop/internal/auth"
	"forge-drop/internal/httpx"
)

func (s *Server) routes() http.Handler {
	r := gin.New()

	// Minimal middleware stack.
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if s.opts.Dev {
			s.logf("%s %s %s (%s)", c.Request.Method, c.Request.URL.Path, s.clientIP(c.Request), time.Since(start))
		}
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, "ok\n")
	})

	// Public: setup + auth
	r.POST("/api/v1/setup", func(c *gin.Context) {
		h := requireSetupAllowed(s.store, s.handleSetup)
		h(c.Writer, c.Request)
	})
	r.GET("/api/v1/setup/status", func(c *gin.Context) {
		s.handleSetupStatus(c.Writer, c.Request)
	})
	r.POST("/api/v1/auth/login", func(c *gin.Context) { s.handleLogin(c.Writer, c.Request) })
	r.POST("/api/v1/auth/logout", func(c *gin.Context) { s.handleLogout(c.Writer, c.Request) })

	// Admin APIs: session cookie required.
	admin := r.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) {
		// Mirror legacy withJSON behavior
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	admin.Use(s.requireSessionGin())

	admin.GET("/me", func(c *gin.Context) {
		s.handleAdminMe(c.Writer, c.Request)
	})

	admin.GET("/settings", func(c *gin.Context) { s.handleAdminSettings(c.Writer, c.Request, "") })
	admin.PUT("/settings", func(c *gin.Context) { s.handleAdminSettings(c.Writer, c.Request, "") })

	admin.GET("/tokens", func(c *gin.Context) { s.handleAdminTokens(c.Writer, c.Request, "") })
	admin.POST("/tokens", func(c *gin.Context) { s.handleAdminTokens(c.Writer, c.Request, "") })
	admin.DELETE("/tokens/:id", func(c *gin.Context) { s.handleAdminTokens(c.Writer, c.Request, "/"+c.Param("id")) })

	admin.GET("/repos", func(c *gin.Context) { s.handleAdminRepos(c.Writer, c.Request, "") })
	admin.POST("/repos", func(c *gin.Context) { s.handleAdminRepos(c.Writer, c.Request, "") })
	admin.DELETE("/repos/:id", func(c *gin.Context) { s.handleAdminRepos(c.Writer, c.Request, "/"+c.Param("id")) })
	admin.PUT("/repos/:id", func(c *gin.Context) { s.handleAdminRepos(c.Writer, c.Request, "/"+c.Param("id")) })

	admin.GET("/apps", func(c *gin.Context) { s.handleAdminApps(c.Writer, c.Request, "") })
	admin.POST("/apps", func(c *gin.Context) { s.handleAdminApps(c.Writer, c.Request, "") })
	admin.GET("/apps/:appID", func(c *gin.Context) { s.handleAdminApps(c.Writer, c.Request, "/"+c.Param("appID")) })
	admin.DELETE("/apps/:appID", func(c *gin.Context) { s.handleAdminApps(c.Writer, c.Request, "/"+c.Param("appID")) })

	admin.GET("/apps/:appID/services", func(c *gin.Context) { s.handleAdminApps(c.Writer, c.Request, "/"+c.Param("appID")+"/services") })

	// Explicit routes for UI critical paths (avoid legacy JSON parsing issues).
	admin.POST("/apps/:appID/services", func(c *gin.Context) {
		appID := strings.TrimSpace(c.Param("appID"))
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
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.WriteError(c.Writer, http.StatusBadRequest, "invalid json")
			return
		}
		req.ServiceKey = strings.TrimSpace(req.ServiceKey)
		req.Name = strings.TrimSpace(req.Name)
		if req.ServiceKey == "" || req.Name == "" {
			httpx.WriteError(c.Writer, http.StatusBadRequest, "service_key/name required")
			return
		}
		svc, err := s.store.CreateService(c.Request.Context(), appID, req.ServiceKey, req.Name, req.Image, req.Command, req.ContainerPort, req.RunUser, req.Env, req.ProdHost)
		if err != nil {
			httpx.WriteError(c.Writer, http.StatusInternalServerError, "create failed")
			return
		}
		c.JSON(http.StatusCreated, serviceJSON(svc))
	})

	admin.POST("/apps/:appID/envs", func(c *gin.Context) {
		appID := strings.TrimSpace(c.Param("appID"))
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.WriteError(c.Writer, http.StatusBadRequest, "invalid json")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			httpx.WriteError(c.Writer, http.StatusBadRequest, "name required")
			return
		}
		env, err := s.store.CreateNamedEnv(c.Request.Context(), appID, req.Name)
		if err != nil {
			httpx.WriteError(c.Writer, http.StatusInternalServerError, "create failed")
			return
		}
		c.JSON(http.StatusCreated, envJSON(env))
	})
	admin.GET("/apps/:appID/envs", func(c *gin.Context) { s.handleAdminApps(c.Writer, c.Request, "/"+c.Param("appID")+"/envs") })
	admin.GET("/services/:serviceID", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")) })
	admin.PUT("/services/:serviceID", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")) })
	admin.DELETE("/services/:serviceID", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")) })
	admin.GET("/services/:serviceID/status", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/status") })
	admin.GET("/services/:serviceID/logs", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/logs") })
	admin.POST("/services/:serviceID/deploy", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/deploy") })
	admin.POST("/services/:serviceID/redeploy", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/redeploy") })
	admin.GET("/services/:serviceID/compose-template-example", func(c *gin.Context) {
		s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/compose-template-example")
	})
	admin.GET("/services/:serviceID/slots", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/slots") })
	admin.POST("/services/:serviceID/slots", func(c *gin.Context) { s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/slots") })
	admin.PUT("/services/:serviceID/slots/:slotID", func(c *gin.Context) {
		s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/slots/"+c.Param("slotID"))
	})
	admin.DELETE("/services/:serviceID/slots/:slotID", func(c *gin.Context) {
		s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/slots/"+c.Param("slotID"))
	})
	admin.POST("/services/:serviceID/artifacts/upload-batch", func(c *gin.Context) {
		s.handleAdminServices(c.Writer, c.Request, "/"+c.Param("serviceID")+"/artifacts/upload-batch")
	})

	admin.GET("/envs/:envID", func(c *gin.Context) { s.handleAdminEnvs(c.Writer, c.Request, "/"+c.Param("envID")) })
	admin.GET("/envs/:envID/snapshots", func(c *gin.Context) { s.handleAdminEnvs(c.Writer, c.Request, "/"+c.Param("envID")+"/snapshots") })
	admin.GET("/envs/:envID/services/:serviceID/slot-artifacts", func(c *gin.Context) {
		s.handleAdminEnvs(c.Writer, c.Request, "/"+c.Param("envID")+"/services/"+c.Param("serviceID")+"/slot-artifacts")
	})
	admin.POST("/envs/:envID/deploy", func(c *gin.Context) { s.handleAdminEnvs(c.Writer, c.Request, "/"+c.Param("envID")+"/deploy") })
	admin.POST("/envs/:envID/rollback", func(c *gin.Context) { s.handleAdminEnvs(c.Writer, c.Request, "/"+c.Param("envID")+"/rollback") })

	// Artifact upload (token)
	r.POST("/api/v1/artifacts/upload", s.requireBearerTokenGin(), func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		s.handleArtifactUpload(c.Writer, c.Request)
	})
	r.POST("/api/v1/artifacts/upload-batch", s.requireBearerTokenGin(), func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		s.handleArtifactUploadBatch(c.Writer, c.Request)
	})

	// Webhooks
	r.POST("/webhooks/forgejo", func(c *gin.Context) {
		s.handleForgejoWebhook(c.Writer, c.Request)
	})

	// SPA (embedded)
	spa := s.serveSPA()
	r.NoRoute(func(c *gin.Context) {
		// Keep API/webhook paths as 404; serve UI for everything else.
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/webhooks/") {
			httpx.WriteError(c.Writer, http.StatusNotFound, "not found")
			return
		}
		spa.ServeHTTP(c.Writer, c.Request)
	})

	return r
}

func (s *Server) requireSessionGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := auth.GetSessionToken(c.Request)
		if !ok {
			httpx.WriteError(c.Writer, http.StatusUnauthorized, "missing session")
			c.Abort()
			return
		}
		sess, err := s.store.GetSessionByToken(c.Request.Context(), token)
		if err != nil {
			httpx.WriteError(c.Writer, http.StatusUnauthorized, "invalid session")
			c.Abort()
			return
		}
		if time.Now().UTC().After(sess.ExpiresAt) {
			_ = s.store.DeleteSession(c.Request.Context(), sess.ID)
			httpx.WriteError(c.Writer, http.StatusUnauthorized, "session expired")
			c.Abort()
			return
		}
		_ = s.store.TouchSession(c.Request.Context(), sess.ID)

		ctx := context.WithValue(c.Request.Context(), ctxUserID, sess.UserID)
		ctx = context.WithValue(ctx, ctxAuthKind, "session")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func (s *Server) requireBearerTokenGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok, ok := httpx.BearerToken(c.Request)
		if !ok || tok == "" {
			httpx.WriteError(c.Writer, http.StatusUnauthorized, "missing token")
			c.Abort()
			return
		}
		t, err := s.store.FindAPITokenByPlaintext(c.Request.Context(), tok)
		if err != nil {
			httpx.WriteError(c.Writer, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}
		if t.RevokedAt != nil {
			httpx.WriteError(c.Writer, http.StatusUnauthorized, "revoked token")
			c.Abort()
			return
		}
		ctx := context.WithValue(c.Request.Context(), ctxTokenID, t.ID)
		ctx = context.WithValue(ctx, ctxAuthKind, "token")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
