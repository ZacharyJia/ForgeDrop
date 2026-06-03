package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"forge-drop/internal/bootstrap"
)

const (
	skillTargetAgents = "agents"
	skillTargetCodex  = "codex"
)

var skillRefPattern = regexp.MustCompile("`([^`\\n]+)`")

func runSkill(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("skill subcommand required: list or install")
	}

	switch args[0] {
	case "list":
		return runSkillList(args[1:])
	case "install":
		return runSkillInstall(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown skill subcommand %q", args[0])
	}
}

func runSkillList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	profileName := ""
	configPath := ""
	authPath := ""
	serverURL := ""
	timeout := 30 * time.Second

	bindProfileFlags(fs, &profileName, &configPath, &authPath)
	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL, overrides config.json")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: forgedrop-ctl skill list [--profile NAME] [--config FILE] [--server URL]")
	}

	paths, err := resolveCLIPaths(profileName, configPath, authPath)
	if err != nil {
		return err
	}
	serverURL, err = loadCLIServerURL(paths.ConfigPath, serverURL)
	if err != nil {
		return err
	}

	client, err := bootstrap.NewClient(serverURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	skills, err := client.ListPublicSkills(ctx)
	if err != nil {
		return err
	}

	type skillSummary struct {
		Name      string `json:"name"`
		FileCount int    `json:"file_count"`
	}
	out := make([]skillSummary, 0, len(skills))
	for _, skill := range skills {
		out = append(out, skillSummary{
			Name:      skill.Name,
			FileCount: len(skill.Files),
		})
	}
	return writeJSON(os.Stdout, map[string]any{"skills": out})
}

func runSkillInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	profileName := ""
	configPath := ""
	authPath := ""
	serverURL := ""
	target := ""
	sourceURL := ""
	force := false
	timeout := 30 * time.Second

	bindProfileFlags(fs, &profileName, &configPath, &authPath)
	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL, overrides config.json")
	fs.StringVar(&target, "target", target, "install target: agents or codex")
	fs.StringVar(&sourceURL, "url", sourceURL, "direct skill bundle URL, e.g. https://host/agents/skill/name")
	fs.BoolVar(&force, "force", force, "overwrite existing local files without confirmation")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	args = reorderInterspersedFlags(fs, args)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("usage: forgedrop-ctl skill install NAME [--target agents|codex] [--profile NAME] [--config FILE] [--server URL]")
	}

	skillName := ""
	if fs.NArg() == 1 {
		skillName = strings.TrimSpace(fs.Arg(0))
	}
	sourceURL = strings.TrimSpace(sourceURL)
	if skillName == "" && sourceURL == "" {
		return fmt.Errorf("skill name or --url is required")
	}
	if skillName != "" && sourceURL != "" {
		return fmt.Errorf("pass either a skill name or --url, not both")
	}

	bundle, sourceDesc, err := loadSkillBundleForInstall(profileName, configPath, authPath, serverURL, sourceURL, skillName, timeout)
	if err != nil {
		return err
	}
	if err := validatePublicSkillBundle(bundle); err != nil {
		return err
	}

	targetRoot, targetKind, err := resolveSkillInstallRoot(target)
	if err != nil {
		return err
	}

	result, err := installSkillBundle(bundle, targetRoot, targetKind, sourceDesc, force)
	if err != nil {
		return err
	}
	return writeJSON(os.Stdout, result)
}

type skillInstallResult struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	InstallDir string `json:"install_dir"`
	Status     string `json:"status"`
	Updated    bool   `json:"updated"`
}

func loadSkillBundleForInstall(profileName, configPath, authPath, serverURL, sourceURL, skillName string, timeout time.Duration) (*bootstrap.PublicSkillBundle, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if sourceURL != "" {
		bundle, err := fetchPublicSkillBundle(ctx, sourceURL)
		return bundle, sourceURL, err
	}

	paths, err := resolveCLIPaths(profileName, configPath, authPath)
	if err != nil {
		return nil, "", err
	}
	serverURL, err = loadCLIServerURL(paths.ConfigPath, serverURL)
	if err != nil {
		return nil, "", err
	}

	client, err := bootstrap.NewClient(serverURL)
	if err != nil {
		return nil, "", err
	}
	bundle, err := client.GetPublicSkill(ctx, skillName)
	if err != nil {
		return nil, "", err
	}
	return bundle, strings.TrimRight(serverURL, "/") + "/agents/skill/" + skillName, nil
}

func fetchPublicSkillBundle(ctx context.Context, rawURL string) (*bootstrap.PublicSkillBundle, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid skill URL %q", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("GET %s failed: %s", parsed.String(), msg)
	}

	var out bootstrap.PublicSkillBundle
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode %s: %w", parsed.String(), err)
	}
	return &out, nil
}

func validatePublicSkillBundle(bundle *bootstrap.PublicSkillBundle) error {
	if bundle == nil {
		return fmt.Errorf("skill bundle is required")
	}
	bundle.Name = strings.TrimSpace(bundle.Name)
	if err := validateProfileName(bundle.Name); err != nil {
		return fmt.Errorf("invalid skill name %q: %w", bundle.Name, err)
	}
	if len(bundle.Files) == 0 {
		return fmt.Errorf("skill %q has no files", bundle.Name)
	}

	files := make(map[string]string, len(bundle.Files))
	for _, file := range bundle.Files {
		cleanPath, err := cleanBundlePath(file.Path)
		if err != nil {
			return fmt.Errorf("skill %q has invalid file path %q: %w", bundle.Name, file.Path, err)
		}
		if _, exists := files[cleanPath]; exists {
			return fmt.Errorf("skill %q has duplicate file path %q", bundle.Name, cleanPath)
		}
		files[cleanPath] = file.Content
	}

	skillBody, ok := files["SKILL.md"]
	if !ok {
		return fmt.Errorf("skill %q is missing SKILL.md", bundle.Name)
	}

	for _, ref := range extractReferencedBundlePaths(skillBody) {
		if _, ok := files[ref]; !ok {
			return fmt.Errorf("skill %q payload is incomplete: missing referenced file %q", bundle.Name, ref)
		}
	}
	return nil
}

func cleanBundlePath(filePath string) (string, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.Contains(filePath, `\`) {
		return "", fmt.Errorf("backslashes are not allowed")
	}

	clean := path.Clean(filePath)
	switch {
	case clean == ".":
		return "", fmt.Errorf("path is empty")
	case strings.HasPrefix(clean, "/"):
		return "", fmt.Errorf("absolute paths are not allowed")
	case clean == "..", strings.HasPrefix(clean, "../"):
		return "", fmt.Errorf("parent traversal is not allowed")
	}
	return clean, nil
}

func extractReferencedBundlePaths(skillBody string) []string {
	seen := map[string]struct{}{"SKILL.md": {}}
	matches := skillRefPattern.FindAllStringSubmatch(skillBody, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		candidate := strings.TrimSpace(match[1])
		if candidate == "" || strings.Contains(candidate, " ") || strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") || strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, "~") {
			continue
		}
		clean, err := cleanBundlePath(candidate)
		if err != nil {
			continue
		}
		if !strings.Contains(clean, "/") && path.Ext(clean) == "" {
			continue
		}
		seen[clean] = struct{}{}
	}

	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	return refs
}

func resolveSkillInstallRoot(target string) (string, string, error) {
	target = strings.TrimSpace(strings.ToLower(target))
	if target == "" {
		if !isInteractiveTerminal() {
			return "", "", fmt.Errorf("--target is required outside an interactive terminal; use agents or codex")
		}
		return promptSkillInstallRoot()
	}

	root, err := skillInstallRootForTarget(target)
	if err != nil {
		return "", "", err
	}
	return root, target, nil
}

func skillInstallRootForTarget(target string) (string, error) {
	home, err := userHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("unable to resolve user home directory")
	}

	switch strings.ToLower(strings.TrimSpace(target)) {
	case skillTargetAgents:
		return filepath.Join(home, ".agents", "skills"), nil
	case skillTargetCodex:
		return filepath.Join(home, ".codex", "skills"), nil
	default:
		return "", fmt.Errorf("unknown target %q: use agents or codex", target)
	}
}

func promptSkillInstallRoot() (string, string, error) {
	agentsRoot, err := skillInstallRootForTarget(skillTargetAgents)
	if err != nil {
		return "", "", err
	}
	codexRoot, err := skillInstallRootForTarget(skillTargetCodex)
	if err != nil {
		return "", "", err
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "Choose install target:\n")
		fmt.Fprintf(os.Stderr, "  1) %s (%s)\n", skillTargetAgents, agentsRoot)
		fmt.Fprintf(os.Stderr, "  2) %s (%s)\n", skillTargetCodex, codexRoot)
		fmt.Fprintf(os.Stderr, "Select [1-2]: ")

		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", "", err
		}
		switch strings.TrimSpace(line) {
		case "1", skillTargetAgents:
			return agentsRoot, skillTargetAgents, nil
		case "2", skillTargetCodex:
			return codexRoot, skillTargetCodex, nil
		case "":
			if err == io.EOF {
				return "", "", fmt.Errorf("no install target selected")
			}
		default:
			fmt.Fprintln(os.Stderr, "Please enter 1 or 2.")
		}
	}
}

func isInteractiveTerminal() bool {
	in, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	out, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (in.Mode()&os.ModeCharDevice) != 0 && (out.Mode()&os.ModeCharDevice) != 0
}

func installSkillBundle(bundle *bootstrap.PublicSkillBundle, targetRoot, targetKind, source string, force bool) (*skillInstallResult, error) {
	installDir := filepath.Join(targetRoot, bundle.Name)
	existing, err := readInstalledSkillBundle(installDir, bundle.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil && skillBundlesEqual(existing, bundle) {
		return &skillInstallResult{
			Name:       bundle.Name,
			Source:     source,
			Target:     targetKind,
			InstallDir: installDir,
			Status:     "up_to_date",
			Updated:    false,
		}, nil
	}

	if existing != nil && !force {
		if !isInteractiveTerminal() {
			return nil, fmt.Errorf("skill %q is already installed at %s and differs from the source; rerun with --force to overwrite", bundle.Name, installDir)
		}
		ok, err := promptOverwriteSkill(bundle.Name, installDir)
		if err != nil {
			return nil, err
		}
		if !ok {
			return &skillInstallResult{
				Name:       bundle.Name,
				Source:     source,
				Target:     targetKind,
				InstallDir: installDir,
				Status:     "skipped",
				Updated:    false,
			}, nil
		}
	}

	if err := writeInstalledSkillBundle(bundle, installDir); err != nil {
		return nil, err
	}
	return &skillInstallResult{
		Name:       bundle.Name,
		Source:     source,
		Target:     targetKind,
		InstallDir: installDir,
		Status:     "installed",
		Updated:    existing != nil,
	}, nil
}

func promptOverwriteSkill(skillName, installDir string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "Skill %q already exists at %s and differs from the source. Overwrite? [y/N]: ", skillName, installDir)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return false, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		default:
			fmt.Fprintln(os.Stderr, "Please answer y or n.")
		}
	}
}

func readInstalledSkillBundle(installDir, name string) (*bootstrap.PublicSkillBundle, error) {
	info, err := os.Stat(installDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("existing skill path %s is not a directory", installDir)
	}

	files := make([]bootstrap.PublicSkillFile, 0, 8)
	err = filepath.WalkDir(installDir, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(installDir, filePath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		rel, err = cleanBundlePath(rel)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		files = append(files, bootstrap.PublicSkillFile{
			Path:    rel,
			Content: string(raw),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &bootstrap.PublicSkillBundle{
		Name:  name,
		Files: files,
	}, nil
}

func skillBundlesEqual(a, b *bootstrap.PublicSkillBundle) bool {
	if a == nil || b == nil {
		return a == b
	}
	if strings.TrimSpace(a.Name) != strings.TrimSpace(b.Name) {
		return false
	}

	mapA := make(map[string]string, len(a.Files))
	for _, file := range a.Files {
		mapA[file.Path] = file.Content
	}
	if len(mapA) != len(b.Files) {
		return false
	}
	for _, file := range b.Files {
		if got, ok := mapA[file.Path]; !ok || got != file.Content {
			return false
		}
	}
	return true
}

func writeInstalledSkillBundle(bundle *bootstrap.PublicSkillBundle, installDir string) error {
	parent := filepath.Dir(installDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}

	tempDir, err := os.MkdirTemp(parent, bundle.Name+".tmp-*")
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(tempDir)
		}
	}()

	for _, file := range bundle.Files {
		rel, err := cleanBundlePath(file.Path)
		if err != nil {
			return err
		}
		dst := filepath.Join(tempDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, []byte(file.Content), 0o644); err != nil {
			return err
		}
	}

	if err := os.RemoveAll(installDir); err != nil {
		return err
	}
	if err := os.Rename(tempDir, installDir); err != nil {
		return err
	}
	success = true
	return nil
}
