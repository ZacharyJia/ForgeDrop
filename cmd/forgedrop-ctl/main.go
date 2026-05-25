package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"forge-drop/internal/bootstrap"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "apply":
		if err := runApply(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "forgedrop-ctl: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "forgedrop-ctl: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `forgedrop-ctl

Usage:
  forgedrop-ctl apply --server URL --username USER --password PASS --manifest FILE

Environment variables:
  FORGE_DROP_SERVER
  FORGE_DROP_USERNAME
  FORGE_DROP_PASSWORD
`)
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := envOrDefault("FORGE_DROP_SERVER", "")
	username := envOrDefault("FORGE_DROP_USERNAME", "")
	password := envOrDefault("FORGE_DROP_PASSWORD", "")
	manifestPath := ""
	timeout := 30 * time.Second

	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL, e.g. http://127.0.0.1:8080")
	fs.StringVar(&username, "username", username, "admin username")
	fs.StringVar(&password, "password", password, "admin password")
	fs.StringVar(&manifestPath, "manifest", manifestPath, "path to deploy manifest JSON")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if manifestPath == "" {
		return fmt.Errorf("--manifest is required")
	}

	manifest, err := bootstrap.LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	client, err := bootstrap.NewClient(serverURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := bootstrap.Apply(ctx, client, manifest, bootstrap.ApplyOptions{
		Username: username,
		Password: password,
	})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
