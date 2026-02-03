package server

import (
	"context"
	"net/http"
	"strings"
	"time"

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

func (s *Server) withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.GetSessionToken(r)
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "missing session")
			return
		}
		sess, err := s.store.GetSessionByToken(r.Context(), token)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		if time.Now().UTC().After(sess.ExpiresAt) {
			_ = s.store.DeleteSession(r.Context(), sess.ID)
			httpx.WriteError(w, http.StatusUnauthorized, "session expired")
			return
		}
		_ = s.store.TouchSession(r.Context(), sess.ID)

		ctx := context.WithValue(r.Context(), ctxUserID, sess.UserID)
		ctx = context.WithValue(ctx, ctxAuthKind, "session")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requireBearerToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := httpx.BearerToken(r)
		if !ok || token == "" {
			httpx.WriteError(w, http.StatusUnauthorized, "missing token")
			return
		}
		t, err := s.store.FindAPITokenByPlaintext(r.Context(), token)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if t.RevokedAt != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "revoked token")
			return
		}
		ctx := context.WithValue(r.Context(), ctxTokenID, t.ID)
		ctx = context.WithValue(ctx, ctxAuthKind, "token")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

func method(m string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Method, m) {
			w.Header().Set("Allow", m)
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h(w, r)
	}
}

func requireSetupAllowed(store *db.Store, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := store.UserCount(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "db error")
			return
		}
		if c > 0 {
			httpx.WriteError(w, http.StatusConflict, "already initialized")
			return
		}
		h(w, r)
	}
}
