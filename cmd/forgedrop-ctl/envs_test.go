package main

import (
	"testing"

	"forge-drop/internal/bootstrap"
)

func strPtr(s string) *string { return &s }
func intPtr(n int) *int       { return &n }

func TestFilterEnvsByName(t *testing.T) {
	envs := []bootstrap.Env{
		{ID: "env-prod", Name: "prod", Kind: "named"},
		{ID: "env-staging", Name: "staging", Kind: "named"},
		{ID: "env-pr1", Name: "preview", Kind: "preview", RepoID: strPtr("repo-1"), PRNumber: intPtr(1), RepoFullName: strPtr("acme/demo")},
		{ID: "env-pr2", Name: "preview", Kind: "preview", RepoID: strPtr("repo-1"), PRNumber: intPtr(2), RepoFullName: strPtr("acme/demo")},
	}

	matches := filterEnvs(envs, "prod", "", 0, "")
	if len(matches) != 1 || matches[0].ID != "env-prod" {
		t.Fatalf("unexpected matches: %+v", matches)
	}

	// "preview" matches every PR env by name; resolution must report ambiguity.
	matches = filterEnvs(envs, "preview", "", 0, "")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %+v", matches)
	}

	matches = filterEnvs(envs, "missing", "", 0, "")
	if len(matches) != 0 {
		t.Fatalf("expected no matches, got %+v", matches)
	}
}

func TestFilterEnvsByRepoAndPR(t *testing.T) {
	envs := []bootstrap.Env{
		{ID: "env-pr1", Name: "preview", Kind: "preview", RepoID: strPtr("repo-1"), PRNumber: intPtr(1), RepoFullName: strPtr("acme/demo")},
		{ID: "env-pr2", Name: "preview", Kind: "preview", RepoID: strPtr("repo-1"), PRNumber: intPtr(2), RepoFullName: strPtr("acme/demo")},
		{ID: "env-other", Name: "preview", Kind: "preview", RepoID: strPtr("repo-2"), PRNumber: intPtr(1), RepoFullName: strPtr("acme/other")},
		{ID: "env-cs", Name: "preview", Kind: "preview", RepoID: strPtr("repo-1"), ChangeSet: strPtr("feature-x"), RepoFullName: strPtr("acme/demo")},
	}

	matches := filterEnvs(envs, "", "acme/demo", 2, "")
	if len(matches) != 1 || matches[0].ID != "env-pr2" {
		t.Fatalf("unexpected matches: %+v", matches)
	}

	matches = filterEnvs(envs, "", "ACME/DEMO", 1, "")
	if len(matches) != 1 || matches[0].ID != "env-pr1" {
		t.Fatalf("repo match should be case-insensitive: %+v", matches)
	}

	matches = filterEnvs(envs, "", "acme/demo", 0, "feature-x")
	if len(matches) != 1 || matches[0].ID != "env-cs" {
		t.Fatalf("unexpected matches: %+v", matches)
	}

	matches = filterEnvs(envs, "", "acme/demo", 3, "")
	if len(matches) != 0 {
		t.Fatalf("expected no matches, got %+v", matches)
	}
}

func TestIsPREnv(t *testing.T) {
	cases := []struct {
		name string
		env  bootstrap.Env
		want bool
	}{
		{"named prod", bootstrap.Env{Kind: "named", Name: "prod"}, false},
		{"named preview template", bootstrap.Env{Kind: "named", Name: "preview"}, false},
		{"preview placeholder", bootstrap.Env{Kind: "preview", Name: "preview"}, false},
		{"pr preview", bootstrap.Env{Kind: "preview", RepoID: strPtr("repo-1"), PRNumber: intPtr(7)}, true},
		{"change-set preview", bootstrap.Env{Kind: "preview", RepoID: strPtr("repo-1"), ChangeSet: strPtr("cs-1")}, true},
		{"preview without repo", bootstrap.Env{Kind: "preview", PRNumber: intPtr(7)}, false},
		{"preview with empty repo id", bootstrap.Env{Kind: "preview", RepoID: strPtr("  "), PRNumber: intPtr(7)}, false},
	}
	for _, tc := range cases {
		if got := isPREnv(&tc.env); got != tc.want {
			t.Fatalf("%s: isPREnv = %v, want %v", tc.name, got, tc.want)
		}
	}
}
