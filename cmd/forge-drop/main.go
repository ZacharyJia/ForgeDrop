package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"forge-drop/internal/server"
)

func main() {
	var addr string
	var dataDir string
	var dev bool
	flag.StringVar(&addr, "addr", ":8080", "listen address")
	flag.StringVar(&dataDir, "data-dir", "./data", "data directory (db, artifacts, runtime)")
	flag.BoolVar(&dev, "dev", false, "dev mode (less caching, more logging)")
	flag.Parse()

	logger := log.New(os.Stdout, "forge-drop ", log.LstdFlags|log.LUTC)

	srv, err := server.New(server.Options{
		Addr:    addr,
		DataDir: dataDir,
		Dev:     dev,
		Logger:  logger,
	})
	if err != nil {
		logger.Fatalf("init: %v", err)
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Printf("listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	logger.Printf("shutdown complete")
}
