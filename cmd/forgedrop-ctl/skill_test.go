package main

import (
	"path/filepath"
	"strings"
	"testing"

	"forge-drop/internal/bootstrap"
)

func TestValidatePublicSkillBundleRequiresReferencedFiles(t *testing.T) {
	bundle := &bootstrap.PublicSkillBundle{
		Name: "demo-skill",
		Files: []bootstrap.PublicSkillFile{
			{
				Path:    "SKILL.md",
				Content: "Read `references/setup.md` before use.\n",
			},
		},
	}

	err := validatePublicSkillBundle(bundle)
	if err == nil || !containsText(err.Error(), `missing referenced file "references/setup.md"`) {
		t.Fatalf("expected missing referenced file error, got %v", err)
	}
}

func TestValidatePublicSkillBundleAcceptsCompleteBundle(t *testing.T) {
	bundle := &bootstrap.PublicSkillBundle{
		Name: "demo-skill",
		Files: []bootstrap.PublicSkillFile{
			{
				Path:    "SKILL.md",
				Content: "Read `references/setup.md` and use `assets/run.sh`.\n",
			},
			{
				Path:    "references/setup.md",
				Content: "setup",
			},
			{
				Path:    "assets/run.sh",
				Content: "#!/bin/sh\n",
			},
		},
	}

	if err := validatePublicSkillBundle(bundle); err != nil {
		t.Fatalf("validatePublicSkillBundle: %v", err)
	}
}

func TestSkillInstallRootForTarget(t *testing.T) {
	home := t.TempDir()
	restoreHome := stubUserHomeDir(home)
	defer restoreHome()

	agentsRoot, err := skillInstallRootForTarget(skillTargetAgents)
	if err != nil {
		t.Fatalf("skillInstallRootForTarget(agents): %v", err)
	}
	if want := filepath.Join(home, ".agents", "skills"); agentsRoot != want {
		t.Fatalf("unexpected agents root: got %q want %q", agentsRoot, want)
	}

	codexRoot, err := skillInstallRootForTarget(skillTargetCodex)
	if err != nil {
		t.Fatalf("skillInstallRootForTarget(codex): %v", err)
	}
	if want := filepath.Join(home, ".codex", "skills"); codexRoot != want {
		t.Fatalf("unexpected codex root: got %q want %q", codexRoot, want)
	}
}

func TestSkillBundlesEqualDetectsDifferences(t *testing.T) {
	left := &bootstrap.PublicSkillBundle{
		Name: "demo-skill",
		Files: []bootstrap.PublicSkillFile{
			{Path: "SKILL.md", Content: "a"},
			{Path: "assets/run.sh", Content: "b"},
		},
	}
	right := &bootstrap.PublicSkillBundle{
		Name: "demo-skill",
		Files: []bootstrap.PublicSkillFile{
			{Path: "SKILL.md", Content: "a"},
			{Path: "assets/run.sh", Content: "changed"},
		},
	}

	if skillBundlesEqual(left, right) {
		t.Fatalf("expected bundles to differ")
	}
}

func TestWriteAndReadInstalledSkillBundleRoundTrip(t *testing.T) {
	root := t.TempDir()
	bundle := &bootstrap.PublicSkillBundle{
		Name: "demo-skill",
		Files: []bootstrap.PublicSkillFile{
			{Path: "SKILL.md", Content: "demo"},
			{Path: "assets/run.sh", Content: "#!/bin/sh\n"},
		},
	}

	installDir := filepath.Join(root, bundle.Name)
	if err := writeInstalledSkillBundle(bundle, installDir); err != nil {
		t.Fatalf("writeInstalledSkillBundle: %v", err)
	}

	readBack, err := readInstalledSkillBundle(installDir, bundle.Name)
	if err != nil {
		t.Fatalf("readInstalledSkillBundle: %v", err)
	}
	if !skillBundlesEqual(bundle, readBack) {
		t.Fatalf("expected round-tripped bundle to match original")
	}
}

func containsText(s, needle string) bool {
	return strings.Contains(s, needle)
}
