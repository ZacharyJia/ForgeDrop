package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	case "apps":
		if err := runApps(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "forgedrop-ctl: %v\n", err)
			os.Exit(1)
		}
	case "export":
		if err := runExport(os.Args[2:]); err != nil {
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
  forgedrop-ctl apps [--config FILE] [--auth FILE]
  forgedrop-ctl export --app APP_KEY [--out FILE] [--config FILE] [--auth FILE]

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
	client, resolvedToken, err := loadCLIClient(configPath, authPath, serverURL, token)
	if err != nil {
		return err
	}

	manifest, err := bootstrap.LoadManifest(manifestPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := bootstrap.Apply(ctx, client, manifest, bootstrap.ApplyOptions{
		Token: resolvedToken,
	})
	if err != nil {
		return err
	}

	return writeJSON(os.Stdout, result)
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := ""
	authPath := ""
	serverURL := ""
	token := ""
	appKey := ""
	outPath := ""
	timeout := 30 * time.Second

	fs.StringVar(&configPath, "config", defaultConfigPath("config.json"), "path to config.json")
	fs.StringVar(&authPath, "auth", defaultConfigPath("auth.json"), "path to auth.json")
	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL, overrides config.json")
	fs.StringVar(&token, "token", token, "admin token, overrides auth.json")
	fs.StringVar(&appKey, "app", appKey, "app key to export")
	fs.StringVar(&outPath, "out", outPath, "write manifest to file instead of stdout")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(appKey) == "" {
		return fmt.Errorf("--app is required")
	}

	client, _, err := loadCLIClient(configPath, authPath, serverURL, token)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	manifest, err := bootstrap.Export(ctx, client, appKey)
	if err != nil {
		return err
	}

	if strings.TrimSpace(outPath) == "" || outPath == "-" {
		return writeJSON(os.Stdout, manifest)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := writeJSON(f, manifest); err != nil {
		return err
	}
	return nil
}

func runApps(args []string) error {
	fs := flag.NewFlagSet("apps", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := ""
	authPath := ""
	serverURL := ""
	token := ""
	timeout := 30 * time.Second

	fs.StringVar(&configPath, "config", defaultConfigPath("config.json"), "path to config.json")
	fs.StringVar(&authPath, "auth", defaultConfigPath("auth.json"), "path to auth.json")
	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL, overrides config.json")
	fs.StringVar(&token, "token", token, "admin token, overrides auth.json")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}

	client, _, err := loadCLIClient(configPath, authPath, serverURL, token)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	apps, err := client.ListApps(ctx)
	if err != nil {
		return err
	}

	return writeJSON(os.Stdout, apps)
}

func loadCLIClient(configPath, authPath, serverURL, token string) (*bootstrap.Client, string, error) {
	fileConfig, err := loadCLIConfig(configPath)
	if err != nil {
		return nil, "", err
	}
	fileAuth, err := loadCLIAuth(authPath)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(serverURL) == "" {
		serverURL = fileConfig.Server
	}
	if strings.TrimSpace(token) == "" {
		token = fileAuth.Token
	}
	if strings.TrimSpace(serverURL) == "" {
		return nil, "", fmt.Errorf("server is required; set it in %s or pass --server", configPath)
	}
	if strings.TrimSpace(token) == "" {
		return nil, "", fmt.Errorf("token is required; set it in %s or pass --token", authPath)
	}
	client, err := bootstrap.NewClient(serverURL)
	if err != nil {
		return nil, "", err
	}
	client.SetBearerToken(token)
	return client, token, nil
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

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
