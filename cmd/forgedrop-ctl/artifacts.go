package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"forge-drop/internal/bootstrap"
)

func runArtifacts(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("artifacts subcommand required: upload")
	}

	switch args[0] {
	case "upload":
		return runArtifactsUpload(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown artifacts subcommand %q", args[0])
	}
}

func runArtifactsUpload(args []string) error {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	profileName := ""
	configPath := ""
	authPath := ""
	serverURL := ""
	adminToken := ""
	artifactToken := ""
	appKey := ""
	envName := ""
	envKind := ""
	serviceKey := ""
	slotKey := ""
	repoFull := ""
	filePath := ""
	sha := ""
	ref := ""
	changeSet := ""
	deployStrategy := ""
	prNumber := 0
	deploy := true
	createEnv := false
	timeout := 30 * time.Minute

	bindProfileFlags(fs, &profileName, &configPath, &authPath)
	fs.StringVar(&serverURL, "server", serverURL, "forge-drop base URL, overrides config.json")
	fs.StringVar(&adminToken, "token", adminToken, "admin token, overrides auth.json")
	fs.StringVar(&artifactToken, "artifact-token", artifactToken, "artifact upload token; defaults to --token/auth.json")
	fs.StringVar(&appKey, "app", appKey, "app key")
	fs.StringVar(&envName, "env", envName, "target environment name")
	fs.StringVar(&envKind, "env-kind", envKind, "environment kind override (supported: named for preview template uploads)")
	fs.StringVar(&serviceKey, "service", serviceKey, "service key")
	fs.StringVar(&slotKey, "slot", slotKey, "slot key")
	fs.StringVar(&repoFull, "repo", repoFull, "repo full name, for example owner/repo")
	fs.StringVar(&filePath, "file", filePath, "artifact file path")
	fs.StringVar(&sha, "sha", sha, "git sha recorded on the snapshot")
	fs.StringVar(&ref, "ref", ref, "git ref recorded on the snapshot")
	fs.IntVar(&prNumber, "pr-number", prNumber, "preview PR number")
	fs.StringVar(&changeSet, "change-set", changeSet, "preview change set")
	fs.BoolVar(&deploy, "deploy", deploy, "deploy immediately after upload")
	fs.StringVar(&deployStrategy, "deploy-strategy", deployStrategy, "deploy strategy override: recreate or restart")
	fs.BoolVar(&createEnv, "create-env", createEnv, "create the named env first when missing")
	fs.DurationVar(&timeout, "timeout", timeout, "request timeout")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("artifacts upload does not accept positional arguments")
	}

	req, err := buildArtifactUploadRequest(appKey, envName, envKind, serviceKey, slotKey, repoFull, filePath, sha, ref, changeSet, deployStrategy, prNumber, deploy)
	if err != nil {
		return err
	}

	paths, err := resolveCLIPaths(profileName, configPath, authPath)
	if err != nil {
		return err
	}
	serverURL, err = loadCLIServerURL(paths.ConfigPath, serverURL)
	if err != nil {
		return err
	}
	adminToken, artifactToken, err = resolveArtifactUploadTokens(paths.AuthPath, adminToken, artifactToken)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var createdEnv *bootstrap.Env
	envCreated := false
	if createEnv {
		createdEnv, envCreated, err = ensureCLIUploadEnv(ctx, serverURL, adminToken, req.App, req.Env, req.EnvKind)
		if err != nil {
			return err
		}
	}

	uploadClient, err := bootstrap.NewClient(serverURL)
	if err != nil {
		return err
	}
	uploadClient.SetBearerToken(artifactToken)

	uploadResult, err := uploadClient.UploadArtifact(ctx, req)
	if err != nil {
		return err
	}

	return writeJSON(os.Stdout, struct {
		Upload     *bootstrap.ArtifactUploadResponse `json:"upload"`
		EnvCreated bool                              `json:"env_created"`
		CreatedEnv *bootstrap.Env                    `json:"created_env,omitempty"`
	}{
		Upload:     uploadResult,
		EnvCreated: envCreated,
		CreatedEnv: createdEnv,
	})
}

func buildArtifactUploadRequest(appKey, envName, envKind, serviceKey, slotKey, repoFull, filePath, sha, ref, changeSet, deployStrategy string, prNumber int, deploy bool) (bootstrap.ArtifactUploadRequest, error) {
	req := bootstrap.ArtifactUploadRequest{
		App:            strings.TrimSpace(appKey),
		Env:            strings.TrimSpace(envName),
		EnvKind:        strings.TrimSpace(envKind),
		Service:        strings.TrimSpace(serviceKey),
		Slot:           strings.TrimSpace(slotKey),
		Repo:           strings.TrimSpace(repoFull),
		FilePath:       strings.TrimSpace(filePath),
		SHA:            strings.TrimSpace(sha),
		Ref:            strings.TrimSpace(ref),
		ChangeSet:      strings.TrimSpace(changeSet),
		DeployStrategy: strings.TrimSpace(deployStrategy),
		Deploy:         deploy,
	}

	if req.App == "" {
		return req, fmt.Errorf("--app is required")
	}
	if req.Env == "" {
		return req, fmt.Errorf("--env is required")
	}
	if req.Service == "" {
		return req, fmt.Errorf("--service is required")
	}
	if req.Slot == "" {
		return req, fmt.Errorf("--slot is required")
	}
	if req.Repo == "" {
		return req, fmt.Errorf("--repo is required")
	}
	if req.FilePath == "" {
		return req, fmt.Errorf("--file is required")
	}
	if _, err := os.Stat(req.FilePath); err != nil {
		return req, err
	}
	if req.EnvKind != "" && !strings.EqualFold(req.EnvKind, "named") {
		return req, fmt.Errorf("--env-kind only supports named")
	}
	if req.DeployStrategy != "" && req.DeployStrategy != "recreate" && req.DeployStrategy != "restart" {
		return req, fmt.Errorf("--deploy-strategy must be recreate or restart")
	}
	if prNumber < 0 {
		return req, fmt.Errorf("--pr-number must be positive")
	}
	if prNumber > 0 {
		req.PRNumber = &prNumber
	}

	isPreview := strings.EqualFold(req.Env, "preview")
	useNamedPreview := isPreview && strings.EqualFold(req.EnvKind, "named")
	if useNamedPreview && (req.PRNumber != nil || req.ChangeSet != "") {
		return req, fmt.Errorf("--pr-number/--change-set cannot be used with --env preview --env-kind named")
	}
	if isPreview && !useNamedPreview && req.PRNumber == nil && req.ChangeSet == "" {
		return req, fmt.Errorf("--pr-number or --change-set is required for preview uploads unless --env-kind named is used")
	}

	return req, nil
}

func resolveArtifactUploadTokens(authPath, adminToken, artifactToken string) (string, string, error) {
	adminToken = strings.TrimSpace(adminToken)
	artifactToken = strings.TrimSpace(artifactToken)
	if adminToken == "" || artifactToken == "" {
		authCfg, err := loadCLIAuth(authPath)
		if err != nil {
			return "", "", err
		}
		if adminToken == "" {
			adminToken = strings.TrimSpace(authCfg.Token)
		}
		if artifactToken == "" {
			artifactToken = adminToken
		}
	}
	if artifactToken == "" {
		return "", "", fmt.Errorf("upload token is required; pass --artifact-token or configure --token/auth.json")
	}
	return adminToken, artifactToken, nil
}

func ensureCLIUploadEnv(ctx context.Context, serverURL, adminToken, appKey, envName, envKind string) (*bootstrap.Env, bool, error) {
	isNamedEnv := !strings.EqualFold(envName, "preview") || strings.EqualFold(envKind, "named")
	if !isNamedEnv {
		return nil, false, nil
	}
	if strings.TrimSpace(adminToken) == "" {
		return nil, false, fmt.Errorf("--create-env requires an admin token; pass --token or configure auth.json")
	}

	client, err := bootstrap.NewClient(serverURL)
	if err != nil {
		return nil, false, err
	}
	client.SetBearerToken(adminToken)

	apps, err := client.ListApps(ctx)
	if err != nil {
		return nil, false, err
	}
	var appID string
	for _, app := range apps {
		if app.AppKey == appKey {
			appID = app.ID
			break
		}
	}
	if appID == "" {
		return nil, false, fmt.Errorf("unknown app %q", appKey)
	}

	envs, err := client.ListEnvs(ctx, appID)
	if err != nil {
		return nil, false, err
	}
	targetKind := "named"
	for _, env := range envs {
		if env.Name == envName && env.Kind == targetKind {
			return &env, false, nil
		}
	}

	created, err := client.CreateEnv(ctx, appID, envName)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}
