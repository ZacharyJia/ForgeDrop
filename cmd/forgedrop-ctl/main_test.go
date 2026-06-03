package main

import (
	"path/filepath"
	"testing"
)

func TestResolveProfileNamePrecedence(t *testing.T) {
	home := t.TempDir()
	restoreHome := stubUserHomeDir(home)
	defer restoreHome()

	restoreEnv1 := stubLookupEnv(func(key string) (string, bool) {
		return "", false
	})
	defer restoreEnv1()

	if err := writeActiveProfileName("staging"); err != nil {
		t.Fatalf("writeActiveProfileName: %v", err)
	}

	resolved, err := resolveProfileName("")
	if err != nil {
		t.Fatalf("resolveProfileName(active): %v", err)
	}
	if resolved.Name != "staging" || resolved.Source != "active" {
		t.Fatalf("unexpected active resolution: %+v", resolved)
	}

	restoreEnv2 := stubLookupEnv(func(key string) (string, bool) {
		if key == profileEnvVar {
			return "prod", true
		}
		return "", false
	})
	defer restoreEnv2()

	resolved, err = resolveProfileName("")
	if err != nil {
		t.Fatalf("resolveProfileName(env): %v", err)
	}
	if resolved.Name != "prod" || resolved.Source != "env" {
		t.Fatalf("unexpected env resolution: %+v", resolved)
	}

	resolved, err = resolveProfileName("dev")
	if err != nil {
		t.Fatalf("resolveProfileName(flag): %v", err)
	}
	if resolved.Name != "dev" || resolved.Source != "flag" {
		t.Fatalf("unexpected explicit resolution: %+v", resolved)
	}
}

func TestResolveProfileNameFallsBackToDefault(t *testing.T) {
	home := t.TempDir()
	restoreHome := stubUserHomeDir(home)
	defer restoreHome()

	restoreEnv := stubLookupEnv(func(key string) (string, bool) {
		return "", false
	})
	defer restoreEnv()

	resolved, err := resolveProfileName("")
	if err != nil {
		t.Fatalf("resolveProfileName(default): %v", err)
	}
	if resolved.Name != defaultProfileName || resolved.Source != "default" {
		t.Fatalf("unexpected default resolution: %+v", resolved)
	}
}

func TestProfilePathsForName(t *testing.T) {
	home := t.TempDir()
	restoreHome := stubUserHomeDir(home)
	defer restoreHome()

	defaultPaths := profilePathsForName(defaultProfileName)
	if got, want := defaultPaths.ConfigPath, filepath.Join(home, ".forgedrop", "config.json"); got != want {
		t.Fatalf("default config path mismatch: got %q want %q", got, want)
	}
	if got, want := defaultPaths.AuthPath, filepath.Join(home, ".forgedrop", "auth.json"); got != want {
		t.Fatalf("default auth path mismatch: got %q want %q", got, want)
	}

	prodPaths := profilePathsForName("prod")
	if got, want := prodPaths.ConfigPath, filepath.Join(home, ".forgedrop", "profiles", "prod", "config.json"); got != want {
		t.Fatalf("named config path mismatch: got %q want %q", got, want)
	}
	if got, want := prodPaths.AuthPath, filepath.Join(home, ".forgedrop", "profiles", "prod", "auth.json"); got != want {
		t.Fatalf("named auth path mismatch: got %q want %q", got, want)
	}
}

func TestUpdateProfileFilesAndDescribeProfile(t *testing.T) {
	home := t.TempDir()
	restoreHome := stubUserHomeDir(home)
	defer restoreHome()

	if err := updateProfileFiles("prod", "https://prod.example.com", "token-prod"); err != nil {
		t.Fatalf("updateProfileFiles: %v", err)
	}

	info, err := describeProfile("prod")
	if err != nil {
		t.Fatalf("describeProfile: %v", err)
	}
	if !info.Exists {
		t.Fatalf("expected profile to exist")
	}
	if info.Server != "https://prod.example.com" {
		t.Fatalf("unexpected server: %q", info.Server)
	}
	if !info.TokenConfigured {
		t.Fatalf("expected token to be configured")
	}
}

func TestListKnownProfilesIncludesActiveAndNamedProfiles(t *testing.T) {
	home := t.TempDir()
	restoreHome := stubUserHomeDir(home)
	defer restoreHome()

	restoreEnv := stubLookupEnv(func(key string) (string, bool) {
		if key == profileEnvVar {
			return "preview", true
		}
		return "", false
	})
	defer restoreEnv()

	if err := updateProfileFiles("prod", "https://prod.example.com", "token-prod"); err != nil {
		t.Fatalf("updateProfileFiles(prod): %v", err)
	}
	if err := writeActiveProfileName("staging"); err != nil {
		t.Fatalf("writeActiveProfileName: %v", err)
	}

	names, err := listKnownProfiles()
	if err != nil {
		t.Fatalf("listKnownProfiles: %v", err)
	}

	expected := []string{"default", "preview", "prod", "staging"}
	if len(names) != len(expected) {
		t.Fatalf("unexpected profiles: got %v want %v", names, expected)
	}
	for i := range expected {
		if names[i] != expected[i] {
			t.Fatalf("unexpected profiles: got %v want %v", names, expected)
		}
	}
}

func stubUserHomeDir(home string) func() {
	original := userHomeDir
	userHomeDir = func() (string, error) {
		return home, nil
	}
	return func() {
		userHomeDir = original
	}
}

func stubLookupEnv(fn func(string) (string, bool)) func() {
	original := lookupEnv
	lookupEnv = fn
	return func() {
		lookupEnv = original
	}
}
