package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"forge-drop/internal/bootstrap"
	"forge-drop/internal/buildinfo"
)

const (
	defaultProfileName = "default"
	profilesDirName    = "profiles"
	activeProfileFile  = "active-profile"
	profileEnvVar      = "FORGEDROP_PROFILE"
)

var (
	profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	userHomeDir        = os.UserHomeDir
	lookupEnv          = os.LookupEnv
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
	case "skill":
		if err := runSkill(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "forgedrop-ctl: %v\n", err)
			os.Exit(1)
		}
	case "self-update":
		if err := runSelfUpdate(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "forgedrop-ctl: %v\n", err)
			os.Exit(1)
		}
	case "profile":
		if err := runProfile(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "forgedrop-ctl: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version":
		printVersion(os.Stdout)
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
  forgedrop-ctl apply --manifest FILE [--profile NAME] [--config FILE] [--auth FILE]
  forgedrop-ctl apps [--profile NAME] [--config FILE] [--auth FILE]
  forgedrop-ctl export --app APP_KEY [--out FILE] [--profile NAME] [--config FILE] [--auth FILE]
  forgedrop-ctl skill list [--profile NAME] [--config FILE] [--server URL]
  forgedrop-ctl skill install NAME [--target agents|codex] [--profile NAME] [--config FILE] [--server URL]
  forgedrop-ctl skill install --url URL [--target agents|codex]
  forgedrop-ctl self-update [--version TAG] [--repo OWNER/REPO]
  forgedrop-ctl version
  forgedrop-ctl profile current
  forgedrop-ctl profile list
  forgedrop-ctl profile use NAME
  forgedrop-ctl profile set NAME [--server URL] [--token TOKEN] [--activate]

Profile resolution order:
  1. --profile
  2. $FORGEDROP_PROFILE
  3. ~/.forgedrop/active-profile
  4. default

Profile files:
  default profile: ~/.forgedrop/config.json + ~/.forgedrop/auth.json
  named profile:   ~/.forgedrop/profiles/<name>/config.json + auth.json
`)
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	profileName := ""
	configPath := ""
	authPath := ""
	serverURL := ""
	token := ""
	manifestPath := ""
	timeout := 30 * time.Second

	bindProfileFlags(fs, &profileName, &configPath, &authPath)
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

	paths, err := resolveCLIPaths(profileName, configPath, authPath)
	if err != nil {
		return err
	}
	client, resolvedToken, err := loadCLIClient(paths.ConfigPath, paths.AuthPath, serverURL, token)
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

	profileName := ""
	configPath := ""
	authPath := ""
	serverURL := ""
	token := ""
	appKey := ""
	outPath := ""
	timeout := 30 * time.Second

	bindProfileFlags(fs, &profileName, &configPath, &authPath)
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

	paths, err := resolveCLIPaths(profileName, configPath, authPath)
	if err != nil {
		return err
	}
	client, _, err := loadCLIClient(paths.ConfigPath, paths.AuthPath, serverURL, token)
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

	return writeJSON(f, manifest)
}

func runApps(args []string) error {
	fs := flag.NewFlagSet("apps", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	profileName := ""
	configPath := ""
	authPath := ""
	serverURL := ""
	token := ""
	timeout := 30 * time.Second

	bindProfileFlags(fs, &profileName, &configPath, &authPath)
	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL, overrides config.json")
	fs.StringVar(&token, "token", token, "admin token, overrides auth.json")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}

	paths, err := resolveCLIPaths(profileName, configPath, authPath)
	if err != nil {
		return err
	}
	client, _, err := loadCLIClient(paths.ConfigPath, paths.AuthPath, serverURL, token)
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

func runProfile(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("profile subcommand required: current, list, use, set")
	}

	switch args[0] {
	case "current":
		return runProfileCurrent(args[1:])
	case "list":
		return runProfileList(args[1:])
	case "use":
		return runProfileUse(args[1:])
	case "set":
		return runProfileSet(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown profile subcommand %q", args[0])
	}
}

func runProfileCurrent(args []string) error {
	fs := flag.NewFlagSet("current", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("profile current does not accept positional arguments")
	}

	resolved, err := resolveProfileName("")
	if err != nil {
		return err
	}
	info, err := describeProfile(resolved.Name)
	if err != nil {
		return err
	}
	active, err := readActiveProfileName()
	if err != nil {
		return err
	}

	return writeJSON(os.Stdout, struct {
		Name            string `json:"name"`
		Source          string `json:"source"`
		ActiveProfile   string `json:"active_profile,omitempty"`
		ConfigPath      string `json:"config_path"`
		AuthPath        string `json:"auth_path"`
		Server          string `json:"server,omitempty"`
		TokenConfigured bool   `json:"token_configured"`
		Exists          bool   `json:"exists"`
	}{
		Name:            info.Name,
		Source:          resolved.Source,
		ActiveProfile:   active,
		ConfigPath:      info.ConfigPath,
		AuthPath:        info.AuthPath,
		Server:          info.Server,
		TokenConfigured: info.TokenConfigured,
		Exists:          info.Exists,
	})
}

func runProfileList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("profile list does not accept positional arguments")
	}

	active, err := readActiveProfileName()
	if err != nil {
		return err
	}
	resolved, err := resolveProfileName("")
	if err != nil {
		return err
	}
	names, err := listKnownProfiles()
	if err != nil {
		return err
	}

	profiles := make([]cliProfileInfo, 0, len(names))
	for _, name := range names {
		info, err := describeProfile(name)
		if err != nil {
			return err
		}
		info.Active = name == active
		info.Effective = name == resolved.Name
		profiles = append(profiles, info)
	}

	return writeJSON(os.Stdout, struct {
		ActiveProfile    string           `json:"active_profile,omitempty"`
		EffectiveProfile string           `json:"effective_profile"`
		Source           string           `json:"source"`
		Profiles         []cliProfileInfo `json:"profiles"`
	}{
		ActiveProfile:    active,
		EffectiveProfile: resolved.Name,
		Source:           resolved.Source,
		Profiles:         profiles,
	})
}

func runProfileUse(args []string) error {
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	args = reorderInterspersedFlags(fs, args)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: forgedrop-ctl profile use NAME")
	}

	name := strings.TrimSpace(fs.Arg(0))
	if err := validateProfileName(name); err != nil {
		return err
	}
	if err := writeActiveProfileName(name); err != nil {
		return err
	}

	info, err := describeProfile(name)
	if err != nil {
		return err
	}
	info.Active = true
	info.Effective = true
	return writeJSON(os.Stdout, info)
}

func runProfileSet(args []string) error {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := ""
	token := ""
	activate := false

	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL")
	fs.StringVar(&token, "token", token, "admin token")
	fs.BoolVar(&activate, "activate", activate, "set this profile as active after updating files")

	args = reorderInterspersedFlags(fs, args)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: forgedrop-ctl profile set NAME [--server URL] [--token TOKEN] [--activate]")
	}

	name := strings.TrimSpace(fs.Arg(0))
	if err := validateProfileName(name); err != nil {
		return err
	}

	serverURL = strings.TrimSpace(serverURL)
	token = strings.TrimSpace(token)
	if serverURL == "" && token == "" {
		return fmt.Errorf("at least one of --server or --token is required")
	}

	if err := updateProfileFiles(name, serverURL, token); err != nil {
		return err
	}
	if activate {
		if err := writeActiveProfileName(name); err != nil {
			return err
		}
	}

	info, err := describeProfile(name)
	if err != nil {
		return err
	}
	info.Active = activate
	info.Effective = activate
	return writeJSON(os.Stdout, info)
}

func bindProfileFlags(fs *flag.FlagSet, profileName, configPath, authPath *string) {
	fs.StringVar(profileName, "profile", "", "profile name (default: --profile, $FORGEDROP_PROFILE, active profile, or default)")
	fs.StringVar(configPath, "config", "", "path to config.json (overrides profile path)")
	fs.StringVar(authPath, "auth", "", "path to auth.json (overrides profile path)")
}

type boolFlag interface {
	IsBoolFlag() bool
}

func reorderInterspersedFlags(fs *flag.FlagSet, args []string) []string {
	if len(args) < 2 {
		return args
	}

	flagArgs := make([]string, 0, len(args))
	positionalArgs := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionalArgs = append(positionalArgs, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionalArgs = append(positionalArgs, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		if flagExpectsValue(fs, arg) && !strings.Contains(arg, "=") && i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}

	return append(flagArgs, positionalArgs...)
}

func flagExpectsValue(fs *flag.FlagSet, arg string) bool {
	name := strings.TrimLeft(arg, "-")
	if idx := strings.IndexByte(name, '='); idx >= 0 {
		name = name[:idx]
	}
	if name == "" {
		return false
	}
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	if bf, ok := f.Value.(boolFlag); ok && bf.IsBoolFlag() {
		return false
	}
	return true
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

func loadCLIServerURL(configPath, serverURL string) (string, error) {
	fileConfig, err := loadCLIConfig(configPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(serverURL) == "" {
		serverURL = fileConfig.Server
	}
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return "", fmt.Errorf("server is required; set it in %s or pass --server", configPath)
	}
	return serverURL, nil
}

type cliConfig struct {
	Server string `json:"server"`
}

type cliAuth struct {
	Token string `json:"token"`
}

type resolvedProfile struct {
	Name   string
	Source string
}

type cliProfilePaths struct {
	Name       string
	ConfigPath string
	AuthPath   string
}

type cliProfileInfo struct {
	Name            string `json:"name"`
	ConfigPath      string `json:"config_path"`
	AuthPath        string `json:"auth_path"`
	Server          string `json:"server,omitempty"`
	TokenConfigured bool   `json:"token_configured"`
	Exists          bool   `json:"exists"`
	Active          bool   `json:"active,omitempty"`
	Effective       bool   `json:"effective,omitempty"`
}

func resolveCLIPaths(profileName, configPath, authPath string) (*cliProfilePaths, error) {
	resolved, err := resolveProfileName(profileName)
	if err != nil {
		return nil, err
	}
	paths := profilePathsForName(resolved.Name)
	if strings.TrimSpace(configPath) != "" {
		paths.ConfigPath = configPath
	}
	if strings.TrimSpace(authPath) != "" {
		paths.AuthPath = authPath
	}
	return &paths, nil
}

func resolveProfileName(explicit string) (*resolvedProfile, error) {
	if name := strings.TrimSpace(explicit); name != "" {
		if err := validateProfileName(name); err != nil {
			return nil, err
		}
		return &resolvedProfile{Name: name, Source: "flag"}, nil
	}

	if raw, ok := lookupEnv(profileEnvVar); ok {
		if name := strings.TrimSpace(raw); name != "" {
			if err := validateProfileName(name); err != nil {
				return nil, fmt.Errorf("invalid %s: %w", profileEnvVar, err)
			}
			return &resolvedProfile{Name: name, Source: "env"}, nil
		}
	}

	active, err := readActiveProfileName()
	if err != nil {
		return nil, err
	}
	if active != "" {
		return &resolvedProfile{Name: active, Source: "active"}, nil
	}

	return &resolvedProfile{Name: defaultProfileName, Source: "default"}, nil
}

func validateProfileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: use letters, numbers, dot, dash, or underscore", name)
	}
	return nil
}

func profilePathsForName(name string) cliProfilePaths {
	if name == defaultProfileName {
		return cliProfilePaths{
			Name:       name,
			ConfigPath: defaultConfigPath("config.json"),
			AuthPath:   defaultConfigPath("auth.json"),
		}
	}

	return cliProfilePaths{
		Name:       name,
		ConfigPath: filepath.Join(configHomeDir(), profilesDirName, name, "config.json"),
		AuthPath:   filepath.Join(configHomeDir(), profilesDirName, name, "auth.json"),
	}
}

func describeProfile(name string) (cliProfileInfo, error) {
	paths := profilePathsForName(name)
	cfg, err := loadCLIConfig(paths.ConfigPath)
	if err != nil {
		return cliProfileInfo{}, err
	}
	auth, err := loadCLIAuth(paths.AuthPath)
	if err != nil {
		return cliProfileInfo{}, err
	}

	return cliProfileInfo{
		Name:            name,
		ConfigPath:      paths.ConfigPath,
		AuthPath:        paths.AuthPath,
		Server:          cfg.Server,
		TokenConfigured: auth.Token != "",
		Exists:          profileExists(name),
	}, nil
}

func listKnownProfiles() ([]string, error) {
	names := map[string]struct{}{
		defaultProfileName: {},
	}

	if active, err := readActiveProfileName(); err != nil {
		return nil, err
	} else if active != "" {
		names[active] = struct{}{}
	}

	if raw, ok := lookupEnv(profileEnvVar); ok {
		if name := strings.TrimSpace(raw); name != "" {
			if err := validateProfileName(name); err != nil {
				return nil, fmt.Errorf("invalid %s: %w", profileEnvVar, err)
			}
			names[name] = struct{}{}
		}
	}

	entries, err := os.ReadDir(filepath.Join(configHomeDir(), profilesDirName))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if err := validateProfileName(name); err != nil {
			return nil, fmt.Errorf("invalid profile directory %q: %w", name, err)
		}
		names[name] = struct{}{}
	}

	list := make([]string, 0, len(names))
	for name := range names {
		list = append(list, name)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i] == defaultProfileName {
			return true
		}
		if list[j] == defaultProfileName {
			return false
		}
		return list[i] < list[j]
	})
	return list, nil
}

func profileExists(name string) bool {
	paths := profilePathsForName(name)
	if fileExists(paths.ConfigPath) || fileExists(paths.AuthPath) {
		return true
	}
	if name == defaultProfileName {
		return false
	}
	return fileExists(filepath.Dir(paths.ConfigPath))
}

func updateProfileFiles(name, serverURL, token string) error {
	paths := profilePathsForName(name)
	if serverURL != "" {
		if err := writeJSONFile(paths.ConfigPath, cliConfig{Server: serverURL}); err != nil {
			return err
		}
	}
	if token != "" {
		if err := writeJSONFile(paths.AuthPath, cliAuth{Token: token}); err != nil {
			return err
		}
	}
	return nil
}

func configHomeDir() string {
	home, err := userHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".forgedrop"
	}
	return filepath.Join(home, ".forgedrop")
}

func activeProfilePath() string {
	return filepath.Join(configHomeDir(), activeProfileFile)
}

func defaultConfigPath(name string) string {
	return filepath.Join(configHomeDir(), name)
}

func readActiveProfileName() (string, error) {
	raw, err := os.ReadFile(activeProfilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	name := strings.TrimSpace(string(raw))
	if name == "" {
		return "", nil
	}
	if err := validateProfileName(name); err != nil {
		return "", fmt.Errorf("invalid active profile in %s: %w", activeProfilePath(), err)
	}
	return name, nil
}

func writeActiveProfileName(name string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	if err := os.MkdirAll(configHomeDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(activeProfilePath(), []byte(name+"\n"), 0o600)
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

func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printVersion(w io.Writer) {
	info := buildinfo.Current()
	fmt.Fprintf(w, "forgedrop-ctl %s (%s, %s)\n", info.Version, info.GOOS, info.GOARCH)
}
