package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
  forgedrop-ctl apply --manifest FILE [--config FILE] [--auth FILE]

Default config files:
  ~/.forgedrop/config.json
  ~/.forgedrop/auth.json
`)
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := ""
	authPath := ""
	serverURL := ""
	token := ""
	manifestPath := ""
	timeout := 30 * time.Second

	fs.StringVar(&configPath, "config", defaultConfigPath("config.json"), "path to config.json")
	fs.StringVar(&authPath, "auth", defaultConfigPath("auth.json"), "path to auth.json")
	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL, overrides config.json")
	fs.StringVar(&token, "token", token, "admin token, overrides auth.json")
	fs.StringVar(&manifestPath, "manifest", manifestPath, "path to deploy manifest JSON")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if manifestPath == "" {
		return fmt.Errorf("--manifest is required")
	}
	fileConfig, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}
	fileAuth, err := loadCLIAuth(authPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(serverURL) == "" {
		serverURL = fileConfig.Server
	}
	if strings.TrimSpace(token) == "" {
		token = fileAuth.Token
	}
	if strings.TrimSpace(serverURL) == "" {
		return fmt.Errorf("server is required; set it in %s or pass --server", configPath)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("token is required; set it in %s or pass --token", authPath)
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
		Token: token,
	})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

type cliConfig struct {
	Server string `json:"server"`
}

type cliAuth struct {
	Token string `json:"token"`
}

func defaultConfigPath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".forgedrop", name)
	}
	return filepath.Join(home, ".forgedrop", name)
}

func loadCLIConfig(path string) (*cliConfig, error) {
	var cfg cliConfig
	if err := loadJSONFile(path, &cfg); err != nil {
		return nil, err
	}
	cfg.Server = strings.TrimSpace(cfg.Server)
	return &cfg, nil
}

func loadCLIAuth(path string) (*cliAuth, error) {
	var cfg cliAuth
	if err := loadJSONFile(path, &cfg); err != nil {
		return nil, err
	}
	cfg.Token = strings.TrimSpace(cfg.Token)
	return &cfg, nil
}

func loadJSONFile(path string, dst any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}
