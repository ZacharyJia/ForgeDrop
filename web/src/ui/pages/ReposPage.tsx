import React, { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";

export function ReposPage() {
  const [showCreate, setShowCreate] = useState(false);
  const [fullName, setFullName] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  
  const queryClient = useQueryClient();
  const { data: repos, isLoading } = useQuery({
    queryKey: ["repos"],
    queryFn: api.listRepos,
  });

  const createMutation = useMutation({
    mutationFn: () => api.createRepo(fullName, webhookSecret),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["repos"] });
      setShowCreate(false);
      setFullName("");
      setWebhookSecret("");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: api.deleteRepo,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["repos"] });
    },
  });

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate();
  };

  const generateSecret = () => {
    const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
    let secret = "";
    for (let i = 0; i < 32; i++) {
      secret += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    setWebhookSecret(secret);
  };

  if (isLoading) {
    return <div className="loading">Loading...</div>;
  }

  return (
    <div className="repos-page">
      <div className="page-header">
        <h1>Repositories</h1>
        <button onClick={() => setShowCreate(true)} className="btn-primary">
          + Add Repository
        </button>
      </div>

      {showCreate && (
        <div className="modal-overlay" onClick={() => setShowCreate(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Add Repository</h2>
            <form onSubmit={handleCreate}>
              <div className="form-group">
                <label>Repository Full Name</label>
                <input
                  type="text"
                  value={fullName}
                  onChange={(e) => setFullName(e.target.value)}
                  placeholder="owner/repo"
                  required
                />
                <p className="help-text">Format: owner/repository-name</p>
              </div>
              <div className="form-group">
                <label>Webhook Secret</label>
                <div className="input-with-button">
                  <input
                    type="text"
                    value={webhookSecret}
                    onChange={(e) => setWebhookSecret(e.target.value)}
                    placeholder="Generate or enter a secret"
                    required
                  />
                  <button type="button" onClick={generateSecret} className="btn-secondary">
                    Generate
                  </button>
                </div>
                <p className="help-text">
                  Use this secret when configuring the webhook in Forgejo
                </p>
              </div>
              {createMutation.error && (
                <div className="error">{String(createMutation.error)}</div>
              )}
              <div className="modal-actions">
                <button type="button" onClick={() => setShowCreate(false)}>
                  Cancel
                </button>
                <button type="submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? "Adding..." : "Add Repository"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      <div className="repos-list">
        {repos?.map((repo) => (
          <div key={repo.id} className="repo-card">
            <div className="repo-header">
              <h3>{repo.full_name}</h3>
              <button
                className="btn-danger-small"
                onClick={() => {
                  if (confirm(`Delete repository "${repo.full_name}"?`)) {
                    deleteMutation.mutate(repo.id);
                  }
                }}
              >
                Delete
              </button>
            </div>
            <div className="repo-body">
              <div className="info-item">
                <strong>Slug:</strong> {repo.slug}
              </div>
              <div className="info-item">
                <strong>Webhook Secret:</strong> 
                <code className="secret">{repo.webhook_secret}</code>
              </div>
              <div className="repo-meta">
                Added: {new Date(repo.created_at).toLocaleDateString()}
              </div>
            </div>
          </div>
        ))}
      </div>

      {repos?.length === 0 && (
        <div className="empty-state">
          <p>No repositories configured yet.</p>
          <p className="muted">Add repositories to enable webhook-based deployments.</p>
        </div>
      )}
    </div>
  );
}
