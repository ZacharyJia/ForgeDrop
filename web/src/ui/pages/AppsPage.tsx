import React, { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../../api";

export function AppsPage() {
  const [showCreate, setShowCreate] = useState(false);
  const [appKey, setAppKey] = useState("");
  const [name, setName] = useState("");
  
  const queryClient = useQueryClient();
  const { data: apps, isLoading } = useQuery({
    queryKey: ["apps"],
    queryFn: api.listApps,
  });

  const createMutation = useMutation({
    mutationFn: () => api.createApp(appKey, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      setShowCreate(false);
      setAppKey("");
      setName("");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: api.deleteApp,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["apps"] });
    },
  });

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate();
  };

  if (isLoading) {
    return <div className="loading">Loading...</div>;
  }

  return (
    <div className="apps-page">
      <div className="page-header">
        <h1>Applications</h1>
        <button onClick={() => setShowCreate(true)} className="btn-primary">
          + New Application
        </button>
      </div>

      {showCreate && (
        <div className="modal-overlay" onClick={() => setShowCreate(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Create Application</h2>
            <form onSubmit={handleCreate}>
              <div className="form-group">
                <label>App Key (unique identifier)</label>
                <input
                  type="text"
                  value={appKey}
                  onChange={(e) => setAppKey(e.target.value)}
                  placeholder="my-app"
                  required
                  pattern="[a-z0-9-]+"
                  title="Lowercase letters, numbers, and hyphens only"
                />
              </div>
              <div className="form-group">
                <label>Display Name</label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="My Application"
                  required
                />
              </div>
              {createMutation.error && (
                <div className="error">{String(createMutation.error)}</div>
              )}
              <div className="modal-actions">
                <button type="button" onClick={() => setShowCreate(false)}>
                  Cancel
                </button>
                <button type="submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? "Creating..." : "Create"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      <div className="apps-grid">
        {apps?.map((app) => (
          <div key={app.id} className="app-card">
            <div className="app-card-header">
              <h3>{app.name}</h3>
              <button
                className="btn-danger-small"
                onClick={() => {
                  if (confirm(`Delete application "${app.name}"?`)) {
                    deleteMutation.mutate(app.id);
                  }
                }}
              >
                Delete
              </button>
            </div>
            <div className="app-card-body">
              <div className="app-key">{app.app_key}</div>
              <div className="app-meta">
                Created: {new Date(app.created_at).toLocaleDateString()}
              </div>
            </div>
            <Link to={`/apps/${app.id}`} className="app-card-link">
              View Details →
            </Link>
          </div>
        ))}
      </div>

      {apps?.length === 0 && (
        <div className="empty-state">
          <p>No applications yet. Create your first application to get started.</p>
        </div>
      )}
    </div>
  );
}
