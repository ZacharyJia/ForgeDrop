import React, { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";

export function TokensPage() {
  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState("");
  const [newToken, setNewToken] = useState<string | null>(null);
  
  const queryClient = useQueryClient();
  const { data: tokens, isLoading } = useQuery({
    queryKey: ["tokens"],
    queryFn: api.listTokens,
  });

  const createMutation = useMutation({
    mutationFn: () => api.createToken(name),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["tokens"] });
      setNewToken(data.plain_token);
      setName("");
    },
  });

  const revokeMutation = useMutation({
    mutationFn: api.revokeToken,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tokens"] });
    },
  });

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate();
  };

  const closeNewTokenModal = () => {
    setNewToken(null);
    setShowCreate(false);
  };

  if (isLoading) {
    return <div className="loading">Loading...</div>;
  }

  return (
    <div className="tokens-page">
      <div className="page-header">
        <h1>API Tokens</h1>
        <button onClick={() => setShowCreate(true)} className="btn-primary">
          + Create Token
        </button>
      </div>

      {showCreate && !newToken && (
        <div className="modal-overlay" onClick={() => setShowCreate(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Create API Token</h2>
            <form onSubmit={handleCreate}>
              <div className="form-group">
                <label>Token Name</label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="CI Token for my-app"
                  required
                />
                <p className="help-text">
                  A descriptive name to identify this token
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
                  {createMutation.isPending ? "Creating..." : "Create Token"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {newToken && (
        <div className="modal-overlay" onClick={closeNewTokenModal}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Token Created Successfully</h2>
            <div className="success-message">
              <p><strong>Important:</strong> Copy this token now. You won't be able to see it again!</p>
              <div className="token-display">
                <code>{newToken}</code>
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(newToken);
                    alert("Token copied to clipboard!");
                  }}
                  className="btn-secondary"
                >
                  Copy
                </button>
              </div>
            </div>
            <div className="modal-actions">
              <button onClick={closeNewTokenModal} className="btn-primary">
                Done
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="tokens-list">
        {tokens?.map((token) => (
          <div key={token.id} className="token-card">
            <div className="token-header">
              <h3>{token.name}</h3>
              {!token.revoked_at && (
                <button
                  className="btn-danger-small"
                  onClick={() => {
                    if (confirm(`Revoke token "${token.name}"?`)) {
                      revokeMutation.mutate(token.id);
                    }
                  }}
                >
                  Revoke
                </button>
              )}
            </div>
            <div className="token-body">
              <div className="info-item">
                <strong>Prefix:</strong> <code>{token.prefix}...</code>
              </div>
              <div className="info-item">
                <strong>Status:</strong>{" "}
                {token.revoked_at ? (
                  <span className="badge badge-disabled">Revoked</span>
                ) : (
                  <span className="badge badge-enabled">Active</span>
                )}
              </div>
              <div className="token-meta">
                Created: {new Date(token.created_at).toLocaleDateString()}
                {token.revoked_at && (
                  <> | Revoked: {new Date(token.revoked_at).toLocaleDateString()}</>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>

      {tokens?.length === 0 && (
        <div className="empty-state">
          <p>No API tokens yet.</p>
          <p className="muted">Create tokens to allow CI systems to upload artifacts.</p>
        </div>
      )}
    </div>
  );
}
