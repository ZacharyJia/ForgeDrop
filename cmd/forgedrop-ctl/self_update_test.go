package main

import "testing"

func TestNormalizeReleaseTag(t *testing.T) {
	cases := map[string]string{
		"":       "",
		"v1.2.3": "v1.2.3",
		"1.2.3":  "v1.2.3",
	}
	for input, want := range cases {
		if got := normalizeReleaseTag(input); got != want {
			t.Fatalf("normalizeReleaseTag(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSelfUpdateAssetName(t *testing.T) {
	if got := selfUpdateAssetName("darwin", "arm64"); got != "forgedrop-ctl-darwin-arm64.tar.gz" {
		t.Fatalf("unexpected unix asset name: %q", got)
	}
	if got := selfUpdateAssetName("windows", "amd64"); got != "forgedrop-ctl-windows-amd64.zip" {
		t.Fatalf("unexpected windows asset name: %q", got)
	}
}

func TestParseChecksumAsset(t *testing.T) {
	raw := []byte("abc123  forgedrop-ctl-darwin-arm64.tar.gz\nfff999  forge-drop-darwin-arm64.tar.gz\n")
	got, err := parseChecksumAsset(raw, "forgedrop-ctl-darwin-arm64.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksumAsset: %v", err)
	}
	if got != "abc123" {
		t.Fatalf("unexpected checksum %q", got)
	}
}
