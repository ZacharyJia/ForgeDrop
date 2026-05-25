package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) routes() http.Handler {
	r := gin.New()

	// Minimal middleware stack.
	r.Use(gin.Recovery())
	r.Use(s.withTimeoutGin())
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

	// Public: embedded agent skills
	r.GET("/agents/skill", s.handlePublicSkills)
	r.GET("/agents/skill/:name", s.handlePublicSkills)

	// Public: setup + auth
	r.POST("/api/v1/setup", requireSetupAllowedGin(s.store), s.handleSetup)
	r.GET("/api/v1/setup/status", s.handleSetupStatus)
	r.POST("/api/v1/auth/login", s.handleLogin)
	r.POST("/api/v1/auth/logout", s.handleLogout)

	// Admin APIs: session cookie required.
	admin := r.Group("/api/v1/admin")
	admin.Use(noStoreGin())
	admin.Use(s.requireSessionGin())

	admin.GET("/me", s.handleAdminMe)
	admin.GET("/settings", func(c *gin.Context) { s.handleAdminSettings(c, "") })
	admin.PUT("/settings", func(c *gin.Context) { s.handleAdminSettings(c, "") })

	admin.GET("/tokens", func(c *gin.Context) { s.handleAdminTokens(c, "") })
	admin.POST("/tokens", func(c *gin.Context) { s.handleAdminTokens(c, "") })
	admin.DELETE("/tokens/:id", func(c *gin.Context) { s.handleAdminTokens(c, "/"+c.Param("id")) })

	admin.GET("/repos", func(c *gin.Context) { s.handleAdminRepos(c, "") })
	admin.POST("/repos", func(c *gin.Context) { s.handleAdminRepos(c, "") })
	admin.DELETE("/repos/:id", func(c *gin.Context) { s.handleAdminRepos(c, "/"+c.Param("id")) })
	admin.PUT("/repos/:id", func(c *gin.Context) { s.handleAdminRepos(c, "/"+c.Param("id")) })

	admin.GET("/apps", func(c *gin.Context) { s.handleAdminApps(c, "") })
	admin.POST("/apps", func(c *gin.Context) { s.handleAdminApps(c, "") })
	admin.GET("/apps/:appID", func(c *gin.Context) { s.handleAdminApps(c, "/"+c.Param("appID")) })
	admin.DELETE("/apps/:appID", func(c *gin.Context) { s.handleAdminApps(c, "/"+c.Param("appID")) })
	admin.GET("/apps/:appID/services", func(c *gin.Context) { s.handleAdminApps(c, "/"+c.Param("appID")+"/services") })
	admin.POST("/apps/:appID/services", func(c *gin.Context) { s.handleAdminApps(c, "/"+c.Param("appID")+"/services") })
	admin.GET("/apps/:appID/envs", func(c *gin.Context) { s.handleAdminApps(c, "/"+c.Param("appID")+"/envs") })
	admin.POST("/apps/:appID/envs", func(c *gin.Context) { s.handleAdminApps(c, "/"+c.Param("appID")+"/envs") })

	admin.GET("/traefik/status", func(c *gin.Context) { s.handleAdminTraefik(c, "/status") })
	admin.POST("/traefik/install", func(c *gin.Context) { s.handleAdminTraefik(c, "/install") })
	admin.POST("/traefik/credentials", func(c *gin.Context) { s.handleAdminTraefik(c, "/credentials") })

	admin.POST("/maintenance/prune", func(c *gin.Context) { s.handleAdminMaintenance(c, "/prune") })

	admin.GET("/services/:serviceID", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")) })
	admin.PUT("/services/:serviceID", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")) })
	admin.DELETE("/services/:serviceID", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")) })
	admin.GET("/services/:serviceID/status", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")+"/status") })
	admin.GET("/services/:serviceID/logs", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")+"/logs") })
	admin.POST("/services/:serviceID/deploy", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")+"/deploy") })
	admin.POST("/services/:serviceID/redeploy", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")+"/redeploy") })
	admin.GET("/services/:serviceID/compose-template-example", func(c *gin.Context) {
		s.handleAdminServices(c, "/"+c.Param("serviceID")+"/compose-template-example")
	})
	admin.GET("/services/:serviceID/slots", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")+"/slots") })
	admin.POST("/services/:serviceID/slots", func(c *gin.Context) { s.handleAdminServices(c, "/"+c.Param("serviceID")+"/slots") })
	admin.PUT("/services/:serviceID/slots/:slotID", func(c *gin.Context) {
		s.handleAdminServices(c, "/"+c.Param("serviceID")+"/slots/"+c.Param("slotID"))
	})
	admin.DELETE("/services/:serviceID/slots/:slotID", func(c *gin.Context) {
		s.handleAdminServices(c, "/"+c.Param("serviceID")+"/slots/"+c.Param("slotID"))
	})
	admin.POST("/services/:serviceID/artifacts/upload-batch", func(c *gin.Context) {
		s.handleAdminServices(c, "/"+c.Param("serviceID")+"/artifacts/upload-batch")
	})
	admin.GET("/artifacts/:artifactID/download", s.handleAdminArtifactDownload)

	admin.GET("/envs/:envID", func(c *gin.Context) { s.handleAdminEnvs(c, "/"+c.Param("envID")) })
	admin.DELETE("/envs/:envID", func(c *gin.Context) { s.handleAdminEnvs(c, "/"+c.Param("envID")) })
	admin.GET("/envs/:envID/snapshots", func(c *gin.Context) { s.handleAdminEnvs(c, "/"+c.Param("envID")+"/snapshots") })
	admin.GET("/envs/:envID/services/:serviceID/slot-artifacts", func(c *gin.Context) {
		s.handleAdminEnvs(c, "/"+c.Param("envID")+"/services/"+c.Param("serviceID")+"/slot-artifacts")
	})
	admin.POST("/envs/:envID/deploy", func(c *gin.Context) { s.handleAdminEnvs(c, "/"+c.Param("envID")+"/deploy") })
	admin.POST("/envs/:envID/rollback", func(c *gin.Context) { s.handleAdminEnvs(c, "/"+c.Param("envID")+"/rollback") })
	admin.POST("/envs/:envID/sync-preview-snapshot", func(c *gin.Context) {
		s.handleAdminEnvs(c, "/"+c.Param("envID")+"/sync-preview-snapshot")
	})

	// Artifact upload (token)
	r.POST("/api/v1/artifacts/upload", s.requireBearerTokenGin(), noStoreGin(), s.handleArtifactUpload)
	r.POST("/api/v1/artifacts/upload-batch", s.requireBearerTokenGin(), noStoreGin(), s.handleArtifactUploadBatch)

	// Webhooks
	r.POST("/webhooks/forgejo", s.handleForgejoWebhook)

	// SPA (embedded)
	spa := s.serveSPA()
	r.NoRoute(func(c *gin.Context) {
		// Keep API/webhook paths as 404; serve UI for everything else.
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/webhooks/") {
			writeError(c, http.StatusNotFound, "not found")
			return
		}
		spa(c)
	})

	return r
}
