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

type envsFlags struct {
	profileName string
	configPath  string
	authPath    string
	serverURL   string
	token       string
	appKey      string
	envName     string
	repoFull    string
	prNumber    int
	changeSet   string
	timeout     time.Duration
}

func runEnvs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("envs subcommand required: stop, delete")
	}

	switch args[0] {
	case "stop":
		return runEnvsStop(args[1:])
	case "delete":
		return runEnvsDelete(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown envs subcommand %q", args[0])
	}
}

func runEnvsStop(args []string) error {
	f, err := parseEnvsFlags("stop", args)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := f.client()
	if err != nil {
		return err
	}
	defer cancel()

	env, err := f.resolveEnv(ctx, client)
	if err != nil {
		return err
	}
	if err := client.StopEnv(ctx, env.ID); err != nil {
		return err
	}

	return writeJSON(os.Stdout, map[string]any{"ok": true, "env": env})
}

func runEnvsDelete(args []string) error {
	f, err := parseEnvsFlags("delete", args)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := f.client()
	if err != nil {
		return err
	}
	defer cancel()

	env, err := f.resolveEnv(ctx, client)
	if err != nil {
		return err
	}
	if !isPREnv(env) {
		return fmt.Errorf("refusing to delete environment %q (id=%s): only PR preview environments can be deleted; named environments (e.g. prod, preview) are protected", env.Name, env.ID)
	}
	if err := client.DeleteEnv(ctx, env.ID); err != nil {
		return err
	}

	return writeJSON(os.Stdout, map[string]any{"ok": true, "env": env})
}

func parseEnvsFlags(command string, args []string) (*envsFlags, error) {
	fs := flag.NewFlagSet("envs "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	f := &envsFlags{}
	bindProfileFlags(fs, &f.profileName, &f.configPath, &f.authPath)
	fs.StringVar(&f.serverURL, "server", f.serverURL, "forge-drop base URL, overrides config.json")
	fs.StringVar(&f.token, "token", f.token, "admin token, overrides auth.json")
	fs.StringVar(&f.appKey, "app", f.appKey, "app key")
	fs.StringVar(&f.envName, "env", f.envName, "environment name (named envs; for PR preview envs use --repo with --pr or --change-set)")
	fs.StringVar(&f.repoFull, "repo", f.repoFull, "repo full name OWNER/REPO of the PR preview env")
	fs.IntVar(&f.prNumber, "pr", f.prNumber, "PR number of the preview env (requires --repo)")
	fs.StringVar(&f.changeSet, "change-set", f.changeSet, "change set of the preview env (requires --repo)")
	fs.DurationVar(&f.timeout, "timeout", 30*time.Second, "request timeout")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(f.appKey) == "" {
		return nil, fmt.Errorf("--app is required")
	}
	if strings.TrimSpace(f.repoFull) == "" && strings.TrimSpace(f.envName) == "" {
		return nil, fmt.Errorf("--env or --repo is required")
	}
	if strings.TrimSpace(f.repoFull) != "" && f.prNumber <= 0 && strings.TrimSpace(f.changeSet) == "" {
		return nil, fmt.Errorf("--pr or --change-set is required with --repo")
	}
	return f, nil
}

func (f *envsFlags) client() (*bootstrap.Client, context.Context, context.CancelFunc, error) {
	paths, err := resolveCLIPaths(f.profileName, f.configPath, f.authPath)
	if err != nil {
		return nil, nil, nil, err
	}
	client, _, err := loadCLIClient(paths.ConfigPath, paths.AuthPath, f.serverURL, f.token)
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	return client, ctx, cancel, nil
}

func (f *envsFlags) resolveEnv(ctx context.Context, client *bootstrap.Client) (*bootstrap.Env, error) {
	apps, err := client.ListApps(ctx)
	if err != nil {
		return nil, err
	}
	appID := ""
	for _, app := range apps {
		if app.AppKey == strings.TrimSpace(f.appKey) {
			appID = app.ID
			break
		}
	}
	if appID == "" {
		return nil, fmt.Errorf("unknown app %q", f.appKey)
	}

	envs, err := client.ListEnvs(ctx, appID)
	if err != nil {
		return nil, err
	}
	matches := filterEnvs(envs, strings.TrimSpace(f.envName), f.repoFull, f.prNumber, f.changeSet)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no matching environment found in app %q", f.appKey)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple environments match; use --repo with --pr or --change-set to select a specific PR preview env")
	}
	return &matches[0], nil
}

// filterEnvs narrows envs down by repo (+ PR number / change set) for PR
// preview envs, or by exact name match otherwise.
func filterEnvs(envs []bootstrap.Env, envName, repoFull string, prNumber int, changeSet string) []bootstrap.Env {
	repoFull = strings.TrimSpace(repoFull)
	changeSet = strings.TrimSpace(changeSet)
	var out []bootstrap.Env
	for _, e := range envs {
		if repoFull != "" {
			if e.RepoFullName == nil || !strings.EqualFold(strings.TrimSpace(*e.RepoFullName), repoFull) {
				continue
			}
			if prNumber > 0 && (e.PRNumber == nil || *e.PRNumber != prNumber) {
				continue
			}
			if changeSet != "" && (e.ChangeSet == nil || *e.ChangeSet != changeSet) {
				continue
			}
			out = append(out, e)
			continue
		}
		if e.Name == envName {
			out = append(out, e)
		}
	}
	return out
}

// isPREnv reports whether e is a repo-scoped PR preview environment, as
// opposed to a named environment (prod, staging, the preview template, ...).
// Only PR preview environments may be deleted via the CLI.
func isPREnv(e *bootstrap.Env) bool {
	if e.Kind != "preview" || e.RepoID == nil || strings.TrimSpace(*e.RepoID) == "" {
		return false
	}
	if e.PRNumber != nil && *e.PRNumber > 0 {
		return true
	}
	return e.ChangeSet != nil && strings.TrimSpace(*e.ChangeSet) != ""
}
