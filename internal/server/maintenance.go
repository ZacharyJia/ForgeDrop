package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"forge-drop/internal/httpx"
)

func (s *Server) handleAdminMaintenance(w http.ResponseWriter, r *http.Request, rest string) {
	rest = strings.TrimPrefix(rest, "/")
	if rest == "prune" && r.Method == "POST" {
		var req struct {
			DryRun bool `json:"dry_run"`
			Limit  int  `json:"limit"`
		}
		_ = httpx.ReadJSON(w, r, &req, 1<<20)

		res, err := s.pruneUnreferenced(r.Context(), req.DryRun, req.Limit)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}

	httpx.WriteError(w, http.StatusNotFound, "not found")
}

func (s *Server) pruneUnreferenced(ctx context.Context, dryRun bool, limit int) (map[string]any, error) {
	if limit <= 0 {
		limit = 500
	}
	var removedArtifacts int
	var removedSnapshots int
	var removedBytes int64
	var errs []string

	arts, err := s.store.ListUnreferencedArtifacts(ctx, limit)
	if err != nil {
		return nil, err
	}
	for _, a := range arts {
		artifactDir := filepath.Join(s.opts.DataDir, "artifacts", a.ID)
		if !strings.HasPrefix(filepath.Clean(a.StoredPath), filepath.Clean(artifactDir)+string(os.PathSeparator)) {
			// Safety check: only delete within our managed artifact directory.
			errs = append(errs, fmt.Sprintf("skip artifact %s: unexpected stored_path", a.ID))
			continue
		}
		if !dryRun {
			if err := os.RemoveAll(artifactDir); err != nil {
				errs = append(errs, fmt.Sprintf("remove artifact %s failed: %v", a.ID, err))
				continue
			}
			if err := s.store.DeleteArtifactByID(ctx, a.ID); err != nil {
				errs = append(errs, fmt.Sprintf("delete artifact row %s failed: %v", a.ID, err))
				continue
			}
		}
		removedArtifacts++
		removedBytes += a.SizeBytes
	}

	snaps, err := s.store.ListOrphanSnapshots(ctx, limit)
	if err != nil {
		return nil, err
	}
	for _, id := range snaps {
		if !dryRun {
			if err := s.store.DeleteSnapshotByID(ctx, id); err != nil {
				errs = append(errs, fmt.Sprintf("delete snapshot %s failed: %v", id, err))
				continue
			}
		}
		removedSnapshots++
	}

	out := map[string]any{
		"ok":                true,
		"dry_run":           dryRun,
		"artifacts_removed": removedArtifacts,
		"snapshots_removed": removedSnapshots,
		"bytes_removed":     removedBytes,
		"errors":            errs,
	}
	return out, nil
}
