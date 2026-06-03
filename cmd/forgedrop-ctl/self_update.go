package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"forge-drop/internal/buildinfo"
)

const (
	defaultReleaseRepo   = "ZacharyJia/ForgeDrop"
	githubAPIBaseURL     = "https://api.github.com"
	checksumsAssetName   = "checksums.txt"
	selfUpdateBinaryName = "forgedrop-ctl"
)

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type selfUpdateResult struct {
	Status        string `json:"status"`
	Repo          string `json:"repo"`
	FromVersion   string `json:"from_version"`
	ToVersion     string `json:"to_version"`
	Asset         string `json:"asset,omitempty"`
	Executable    string `json:"executable"`
	RestartNeeded bool   `json:"restart_needed,omitempty"`
}

func runSelfUpdate(args []string) error {
	fs := flag.NewFlagSet("self-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repo := defaultReleaseRepo
	targetVersion := ""
	force := false
	timeout := 60 * time.Second

	fs.StringVar(&repo, "repo", repo, "release repository in OWNER/REPO format")
	fs.StringVar(&targetVersion, "version", targetVersion, "release tag to install (default: latest)")
	fs.BoolVar(&force, "force", force, "reinstall even if the current version matches")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: forgedrop-ctl self-update [--version TAG] [--repo OWNER/REPO]")
	}

	repo = strings.TrimSpace(repo)
	if repo == "" || !strings.Contains(repo, "/") {
		return fmt.Errorf("invalid repo %q: expected OWNER/REPO", repo)
	}
	targetVersion = normalizeReleaseTag(targetVersion)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	release, err := fetchGitHubRelease(ctx, repo, targetVersion)
	if err != nil {
		return err
	}

	current := buildinfo.Current()
	if !force && current.Version != "dev" && current.Version == release.TagName {
		exePath, err := currentExecutablePath()
		if err != nil {
			return err
		}
		return writeJSON(os.Stdout, selfUpdateResult{
			Status:      "up_to_date",
			Repo:        repo,
			FromVersion: current.Version,
			ToVersion:   release.TagName,
			Executable:  exePath,
		})
	}

	assetName := selfUpdateAssetName(runtime.GOOS, runtime.GOARCH)
	archiveAsset, err := findReleaseAsset(release.Assets, assetName)
	if err != nil {
		return err
	}
	checksumAsset, err := findReleaseAsset(release.Assets, checksumsAssetName)
	if err != nil {
		return err
	}

	checksumsRaw, err := downloadURL(ctx, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", checksumAsset.Name, err)
	}
	expectedDigest, err := parseChecksumAsset(checksumsRaw, archiveAsset.Name)
	if err != nil {
		return err
	}

	archiveRaw, err := downloadURL(ctx, archiveAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", archiveAsset.Name, err)
	}
	if err := verifySHA256(archiveRaw, expectedDigest); err != nil {
		return fmt.Errorf("verify %s: %w", archiveAsset.Name, err)
	}

	exePath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	replacementPath, err := extractBinaryFromArchive(archiveAsset.Name, archiveRaw, filepath.Dir(exePath))
	if err != nil {
		return err
	}

	status, restartNeeded, err := replaceExecutable(exePath, replacementPath)
	if err != nil {
		return err
	}

	return writeJSON(os.Stdout, selfUpdateResult{
		Status:        status,
		Repo:          repo,
		FromVersion:   current.Version,
		ToVersion:     release.TagName,
		Asset:         archiveAsset.Name,
		Executable:    exePath,
		RestartNeeded: restartNeeded,
	})
}

func normalizeReleaseTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if strings.HasPrefix(tag, "v") {
		return tag
	}
	return "v" + tag
}

func fetchGitHubRelease(ctx context.Context, repo, tag string) (*githubRelease, error) {
	endpoint := githubAPIBaseURL + "/repos/" + repo + "/releases/latest"
	if tag != "" {
		endpoint = githubAPIBaseURL + "/repos/" + repo + "/releases/tags/" + tag
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "forgedrop-ctl/"+buildinfo.Current().Version)

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
		return nil, fmt.Errorf("GET %s failed: %s", endpoint, msg)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode %s: %w", endpoint, err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, fmt.Errorf("release response missing tag_name")
	}
	return &release, nil
}

func findReleaseAsset(assets []githubReleaseAsset, name string) (*githubReleaseAsset, error) {
	for _, asset := range assets {
		if asset.Name == name {
			return &asset, nil
		}
	}
	return nil, fmt.Errorf("release asset %q not found", name)
}

func selfUpdateAssetName(goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%s-%s-%s%s", selfUpdateBinaryName, goos, goarch, ext)
}

func downloadURL(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "forgedrop-ctl/"+buildinfo.Current().Version)

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
		return nil, fmt.Errorf("GET %s failed: %s", rawURL, msg)
	}
	return io.ReadAll(resp.Body)
}

func parseChecksumAsset(raw []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.TrimSpace(fields[len(fields)-1]) == assetName {
			return strings.TrimSpace(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksum for %q not found in %s", assetName, checksumsAssetName)
}

func verifySHA256(raw []byte, expected string) error {
	sum := sha256.Sum256(raw)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, strings.TrimSpace(expected)) {
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, expected)
	}
	return nil
}

func currentExecutablePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exePath)
	if err == nil && strings.TrimSpace(resolved) != "" {
		return resolved, nil
	}
	return exePath, nil
}

func extractBinaryFromArchive(assetName string, raw []byte, targetDir string) (string, error) {
	baseName := selfUpdateBinaryName
	if runtime.GOOS == "windows" {
		baseName += ".exe"
	}

	tempFile, err := os.CreateTemp(targetDir, baseName+".download-*")
	if err != nil {
		return "", err
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return "", err
	}

	var extractErr error
	switch {
	case strings.HasSuffix(assetName, ".zip"):
		extractErr = extractFromZip(raw, baseName, tempPath)
	case strings.HasSuffix(assetName, ".tar.gz"):
		extractErr = extractFromTarGz(raw, baseName, tempPath)
	default:
		extractErr = fmt.Errorf("unsupported archive format for %s", assetName)
	}
	if extractErr != nil {
		_ = os.Remove(tempPath)
		return "", extractErr
	}
	if err := os.Chmod(tempPath, 0o755); err != nil && runtime.GOOS != "windows" {
		_ = os.Remove(tempPath)
		return "", err
	}
	return tempPath, nil
}

func extractFromTarGz(raw []byte, binaryName, dst string) error {
	gr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.FileInfo().IsDir() {
			continue
		}
		if path.Base(hdr.Name) != binaryName {
			continue
		}
		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	return fmt.Errorf("%s not found in archive", binaryName)
}

func extractFromZip(raw []byte, binaryName, dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if path.Base(file.Name) != binaryName {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		f, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(f, rc); err != nil {
			f.Close()
			rc.Close()
			return err
		}
		if err := f.Close(); err != nil {
			rc.Close()
			return err
		}
		return rc.Close()
	}
	return fmt.Errorf("%s not found in archive", binaryName)
}

func replaceExecutable(exePath, replacementPath string) (status string, restartNeeded bool, err error) {
	finalTempPath := exePath + ".new"
	if runtime.GOOS == "windows" {
		finalTempPath += ".exe"
	}
	if err := os.Rename(replacementPath, finalTempPath); err != nil {
		return "", false, err
	}

	if runtime.GOOS == "windows" {
		if err := scheduleWindowsReplacement(exePath, finalTempPath); err != nil {
			return "", false, err
		}
		return "scheduled", true, nil
	}
	if err := os.Rename(finalTempPath, exePath); err != nil {
		return "", false, err
	}
	return "updated", false, nil
}

func scheduleWindowsReplacement(exePath, replacementPath string) error {
	script, err := os.CreateTemp(filepath.Dir(exePath), "forgedrop-ctl-update-*.cmd")
	if err != nil {
		return err
	}
	scriptPath := script.Name()
	content := fmt.Sprintf("@echo off\r\nping 127.0.0.1 -n 2 >NUL\r\nmove /Y %s %s >NUL\r\ndel /Q %s >NUL\r\n", quoteWindowsArg(replacementPath), quoteWindowsArg(exePath), quoteWindowsArg(scriptPath))
	if _, err := script.WriteString(content); err != nil {
		script.Close()
		return err
	}
	if err := script.Close(); err != nil {
		return err
	}

	cmd := exec.Command("cmd.exe", "/C", scriptPath)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func quoteWindowsArg(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
