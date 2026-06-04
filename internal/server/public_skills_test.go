package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestPublicSkillsEndpoint(t *testing.T) {
	s := &Server{opts: Options{Dev: true}}
	h := s.routes()
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/agents/skill")
	if err != nil {
		t.Fatalf("GET /agents/skill failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Skills []struct {
			Name  string `json:"name"`
			Files []struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			} `json:"files"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Skills) == 0 {
		t.Fatalf("expected at least one public skill")
	}

	var found bool
	var foundManual bool
	for _, skill := range body.Skills {
		switch skill.Name {
		case "forge-drop-autodeploy":
			found = true
			if len(skill.Files) == 0 || skill.Files[0].Path != "SKILL.md" {
				t.Fatalf("expected SKILL.md first, got %+v", skill.Files)
			}
			if skill.Files[0].Content == "" {
				t.Fatalf("expected SKILL.md content")
			}
			if len(skill.Files) != 8 {
				t.Fatalf("expected 8 files for forge-drop-autodeploy, got %d: %+v", len(skill.Files), skill.Files)
			}
		case "forgedrop-deploy":
			foundManual = true
			if len(skill.Files) == 0 || skill.Files[0].Path != "SKILL.md" {
				t.Fatalf("expected SKILL.md first for manual deploy skill, got %+v", skill.Files)
			}
			if skill.Files[0].Content == "" {
				t.Fatalf("expected manual deploy SKILL.md content")
			}
			if len(skill.Files) != 5 {
				t.Fatalf("expected 5 files for forgedrop-deploy, got %d: %+v", len(skill.Files), skill.Files)
			}
		}
	}
	if !found {
		t.Fatalf("expected forge-drop-autodeploy in %+v", body.Skills)
	}
	if !foundManual {
		t.Fatalf("expected forgedrop-deploy in %+v", body.Skills)
	}
}

func TestPublicSkillByNameEndpoint(t *testing.T) {
	s := &Server{opts: Options{Dev: true}}
	h := s.routes()
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/agents/skill/forge-drop-autodeploy")
	if err != nil {
		t.Fatalf("GET /agents/skill/:name failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Name  string `json:"name"`
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Name != "forge-drop-autodeploy" {
		t.Fatalf("unexpected skill name %q", body.Name)
	}
	if len(body.Files) == 0 {
		t.Fatalf("expected files in response")
	}
	if len(body.Files) != 8 {
		t.Fatalf("expected 8 files in response, got %d: %+v", len(body.Files), body.Files)
	}
	paths := make([]string, 0, len(body.Files))
	for _, file := range body.Files {
		paths = append(paths, file.Path)
	}
	if !slices.Contains(paths, "assets/upload-deploy-artifact.js") {
		t.Fatalf("expected upload script in response, got %+v", paths)
	}
	if !slices.Contains(paths, "assets/update-pr-preview-comment.js") {
		t.Fatalf("expected preview comment script in response, got %+v", paths)
	}
}

func TestPublicManualDeploySkillByNameEndpoint(t *testing.T) {
	s := &Server{opts: Options{Dev: true}}
	h := s.routes()
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/agents/skill/forgedrop-deploy")
	if err != nil {
		t.Fatalf("GET /agents/skill/:name failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Name  string `json:"name"`
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Name != "forgedrop-deploy" {
		t.Fatalf("unexpected skill name %q", body.Name)
	}
	if len(body.Files) != 5 {
		t.Fatalf("expected 5 files in response, got %d: %+v", len(body.Files), body.Files)
	}
	paths := make([]string, 0, len(body.Files))
	for _, file := range body.Files {
		paths = append(paths, file.Path)
	}
	if !slices.Contains(paths, "assets/deploy-manifest.example.json") {
		t.Fatalf("expected deploy manifest example in response, got %+v", paths)
	}
	if !slices.Contains(paths, "references/manual-deploy-flow.md") {
		t.Fatalf("expected manual deploy reference in response, got %+v", paths)
	}
}
