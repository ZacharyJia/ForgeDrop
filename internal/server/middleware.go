package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"forge-drop/internal/auth"
	"forge-drop/internal/db"
	"forge-drop/internal/httpx"
)

type ctxKey string

const (
	ctxUserID   ctxKey = "user_id"
	ctxTokenID  ctxKey = "token_id"
	ctxIsAdmin  ctxKey = "is_admin"
	ctxAuthKind ctxKey = "auth_kind"
)

func writeError(c *gin.Context, status int, msg string) {
	c.JSON(status, httpx.ErrorResponse{Error: msg})
}

func readJSON(c *gin.Context, dst any, maxBytes int64) error {
	return httpx.ReadJSON(c.Writer, c.Request, dst, maxBytes)
}

func noStoreGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

func (s *Server) withTimeoutGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent any single request from hanging the whole process.
		// Keep upload/download endpoints generous; keep JSON/admin endpoints tight.
		timeout := 15 * time.Second
		p := c.Request.URL.Path
		isArtifactUpload := p == "/api/v1/artifacts/upload" || strings.Contains(p, "/artifacts/upload-batch")
		isArtifactDownload := strings.Contains(p, "/artifacts/") && strings.HasSuffix(p, "/download")
		if isArtifactUpload || isArtifactDownload {
			timeout = 30 * time.Minute
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func requireSetupAllowedGin(store *db.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		count, err := store.UserCount(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, "db error")
			c.Abort()
			return
		}
		if count > 0 {
			writeError(c, http.StatusConflict, "already initialized")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) requireSessionGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := auth.GetSessionToken(c.Request)
		if !ok {
			writeError(c, http.StatusUnauthorized, "missing session")
			c.Abort()
			return
		}
		sess, err := s.store.GetSessionByToken(c.Request.Context(), token)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "invalid session")
			c.Abort()
			return
		}
		if time.Now().UTC().After(sess.ExpiresAt) {
			_ = s.store.DeleteSession(c.Request.Context(), sess.ID)
			writeError(c, http.StatusUnauthorized, "session expired")
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

func (s *Server) requireAdminAuthGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if token, ok := httpx.BearerToken(c.Request); ok && token != "" {
			t, err := s.store.FindAPITokenByPlaintext(c.Request.Context(), token)
			if err != nil {
				writeError(c, http.StatusUnauthorized, "invalid token")
				c.Abort()
				return
			}
			if t.RevokedAt != nil || t.Scope != "admin" {
				writeError(c, http.StatusUnauthorized, "invalid token")
				c.Abort()
				return
			}
			ctx := context.WithValue(c.Request.Context(), ctxTokenID, t.ID)
			ctx = context.WithValue(ctx, ctxAuthKind, "token")
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			return
		}
		s.requireSessionGin()(c)
	}
}

func (s *Server) requireArtifactTokenGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := httpx.BearerToken(c.Request)
		if !ok || token == "" {
			writeError(c, http.StatusUnauthorized, "missing token")
			c.Abort()
			return
		}
		t, err := s.store.FindAPITokenByPlaintext(c.Request.Context(), token)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}
		if t.RevokedAt != nil || t.Scope != "artifact" {
			writeError(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}
		ctx := context.WithValue(c.Request.Context(), ctxTokenID, t.ID)
		ctx = context.WithValue(ctx, ctxAuthKind, "token")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func tokenIDFromContext(ctx context.Context) *string {
	v := ctx.Value(ctxTokenID)
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok && s != "" {
		return &s
	}
	return nil
}

func userIDFromContext(ctx context.Context) *string {
	v := ctx.Value(ctxUserID)
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok && s != "" {
		return &s
	}
	return nil
}
