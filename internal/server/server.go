package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"forge-drop/internal/db"
	"forge-drop/internal/deploy"
	"forge-drop/internal/dockerx"
)

type Options struct {
	Addr    string
	DataDir string
	Dev     bool
	Logger  *log.Logger
}

type Server struct {
	opts Options

	store    *db.Store
	sqlDB    *db.DB
	deployer *deploy.Deployer
	logger   *log.Logger

	handler http.Handler
}

func New(opts Options) (*Server, error) {
	if opts.DataDir == "" {
		opts.DataDir = "./data"
	}
	if opts.Logger == nil {
		opts.Logger = log.New(os.Stdout, "forge-drop ", log.LstdFlags|log.LUTC)
	}

	if err := os.MkdirAll(opts.DataDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(opts.DataDir, "artifacts"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(opts.DataDir, "runtime"), 0o755); err != nil {
		return nil, err
	}

	sqlDB, err := db.Open(opts.DataDir)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.Migrate(ctx, sqlDB.SQL); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	store := db.NewStore(sqlDB.SQL)
	if err := store.EnsureDefaults(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	dockerClient, err := dockerx.New()
	if err != nil {
		opts.Logger.Printf("warning: docker unavailable (%v); continuing with docker disabled", err)
	}
	deployer := deploy.New(deploy.Options{
		DataDir: opts.DataDir,
		Store:   store,
		Docker:  dockerClient,
		Logger:  opts.Logger,
	})

	s := &Server{
		opts:     opts,
		sqlDB:    sqlDB,
		store:    store,
		deployer: deployer,
		logger:   opts.Logger,
	}
	s.handler = s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Close() error {
	var errs []string
	if s.deployer != nil {
		if err := s.deployer.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if s.sqlDB != nil {
		if err := s.sqlDB.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (s *Server) logf(format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Printf(format, args...)
}

func (s *Server) clientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}

func (s *Server) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xfProto := r.Header.Get("X-Forwarded-Proto"); xfProto != "" {
		scheme = xfProto
	}
	host := r.Host
	return fmt.Sprintf("%s://%s", scheme, host)
}

func (s *Server) isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}
