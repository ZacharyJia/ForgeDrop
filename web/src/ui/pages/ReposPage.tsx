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
    return <div className="loading">加载中...</div>;
  }

  return (
    <div className="repos-page">
      <div className="page-header">
        <h1>仓库</h1>
        <button onClick={() => setShowCreate(true)} className="btn-primary">
          + 添加仓库
        </button>
      </div>

      {showCreate && (
        <div className="modal-overlay" onClick={() => setShowCreate(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>添加仓库</h2>
            <form onSubmit={handleCreate}>
              <div className="form-group">
                <label>仓库全名</label>
                <input
                  type="text"
                  value={fullName}
                  onChange={(e) => setFullName(e.target.value)}
                  placeholder="owner/repo"
                  required
                />
                <p className="help-text">格式：owner/repo</p>
              </div>
              <div className="form-group">
                <label>Webhook 密钥</label>
                <div className="input-with-button">
                  <input
                    type="text"
                    value={webhookSecret}
                    onChange={(e) => setWebhookSecret(e.target.value)}
                    placeholder="生成或手动输入"
                    required
                  />
                  <button type="button" onClick={generateSecret} className="btn-secondary">
                    生成
                  </button>
                </div>
                <p className="help-text">
                  在 Forgejo 配置 Webhook 时使用此密钥
                </p>
              </div>
              {createMutation.error && (
                <div className="error">{String(createMutation.error)}</div>
              )}
              <div className="modal-actions">
                <button type="button" onClick={() => setShowCreate(false)}>
                  取消
                </button>
                <button type="submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? "添加中..." : "添加"}
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
                  if (confirm(`确认删除仓库「${repo.full_name}」？`)) {
                    deleteMutation.mutate(repo.id);
                  }
                }}
              >
                删除
              </button>
            </div>
            <div className="repo-body">
              <div className="info-item">
                <strong>Slug：</strong> {repo.slug}
              </div>
              <div className="info-item">
                <strong>Webhook 密钥：</strong>
                <code className="secret">{repo.webhook_secret}</code>
              </div>
              <div className="repo-meta">
                添加时间：{new Date(repo.created_at).toLocaleDateString()}
              </div>
            </div>
          </div>
        ))}
      </div>

      {repos?.length === 0 && (
        <div className="empty-state">
          <p>暂无仓库配置。</p>
          <p className="muted">添加仓库后可启用基于 Webhook 的自动部署。</p>
        </div>
      )}
    </div>
  );
}
