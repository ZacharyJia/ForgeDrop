package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ArtifactUploadRequest struct {
	App            string
	Env            string
	EnvKind        string
	Service        string
	Slot           string
	Repo           string
	SHA            string
	Ref            string
	ChangeSet      string
	DeployStrategy string
	FilePath       string
	Deploy         bool
	PRNumber       *int
}

type ArtifactUploadResponse struct {
	ArtifactID    string `json:"artifact_id"`
	SnapshotID    string `json:"snapshot_id"`
	EnvID         string `json:"env_id"`
	ServiceID     string `json:"service_id"`
	ServiceURL    string `json:"service_url"`
	Deployed      bool   `json:"deployed"`
	DeploySkipped bool   `json:"deploy_skipped"`
	SHA256Hex     string `json:"sha256_hex"`
	StoredPath    string `json:"stored_path"`
	Repo          string `json:"repo"`
	App           string `json:"app"`
	Env           string `json:"env"`
	Service       string `json:"service"`
	Slot          string `json:"slot"`
	ChangeSet     string `json:"change_set"`
}

func (c *Client) UploadArtifact(ctx context.Context, req ArtifactUploadRequest) (*ArtifactUploadResponse, error) {
	if strings.TrimSpace(req.App) == "" {
		return nil, fmt.Errorf("app is required")
	}
	if strings.TrimSpace(req.Env) == "" {
		return nil, fmt.Errorf("env is required")
	}
	if strings.TrimSpace(req.Service) == "" {
		return nil, fmt.Errorf("service is required")
	}
	if strings.TrimSpace(req.Slot) == "" {
		return nil, fmt.Errorf("slot is required")
	}
	if strings.TrimSpace(req.Repo) == "" {
		return nil, fmt.Errorf("repo is required")
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return nil, fmt.Errorf("file path is required")
	}

	f, err := os.Open(req.FilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writeField := func(name, value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		return writer.WriteField(name, value)
	}

	if err := writeField("app", req.App); err != nil {
		return nil, err
	}
	if err := writeField("env", req.Env); err != nil {
		return nil, err
	}
	if err := writeField("env_kind", req.EnvKind); err != nil {
		return nil, err
	}
	if err := writeField("service", req.Service); err != nil {
		return nil, err
	}
	if err := writeField("slot", req.Slot); err != nil {
		return nil, err
	}
	if err := writeField("repo", req.Repo); err != nil {
		return nil, err
	}
	if err := writeField("sha", req.SHA); err != nil {
		return nil, err
	}
	if err := writeField("ref", req.Ref); err != nil {
		return nil, err
	}
	if err := writeField("change_set", req.ChangeSet); err != nil {
		return nil, err
	}
	if err := writeField("deploy_strategy", req.DeployStrategy); err != nil {
		return nil, err
	}
	if req.PRNumber != nil {
		if err := writeField("pr_number", strconv.Itoa(*req.PRNumber)); err != nil {
			return nil, err
		}
	}
	if err := writeField("deploy", map[bool]string{true: "1", false: "0"}[req.Deploy]); err != nil {
		return nil, err
	}

	part, err := writer.CreateFormFile("artifact", filepath.Base(req.FilePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/artifacts/upload", &body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(httpReq)
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
		return nil, fmt.Errorf("POST /api/v1/artifacts/upload failed: %s", msg)
	}

	var out ArtifactUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode POST /api/v1/artifacts/upload: %w", err)
	}
	return &out, nil
}
