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
    return <div className="loading">加载中...</div>;
  }

  return (
    <div className="tokens-page">
      <div className="page-header">
        <h1>API 令牌</h1>
        <button onClick={() => setShowCreate(true)} className="btn-primary">
          + 创建令牌
        </button>
      </div>

      {showCreate && !newToken && (
        <div className="modal-overlay" onClick={() => setShowCreate(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>创建 API 令牌</h2>
            <form onSubmit={handleCreate}>
              <div className="form-group">
                <label>令牌名称</label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="例如：my-app 的 CI 令牌"
                  required
                />
                <p className="help-text">
                  用于区分用途的备注名称
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
                  {createMutation.isPending ? "创建中..." : "创建"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {newToken && (
        <div className="modal-overlay" onClick={closeNewTokenModal}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>令牌创建成功</h2>
            <div className="success-message">
              <p><strong>重要：</strong>请立即复制该令牌，之后将无法再次查看明文。</p>
              <div className="token-display">
                <code>{newToken}</code>
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(newToken);
                    alert("已复制到剪贴板");
                  }}
                  className="btn-secondary"
                >
                  复制
                </button>
              </div>
            </div>
            <div className="modal-actions">
              <button onClick={closeNewTokenModal} className="btn-primary">
                完成
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
                    if (confirm(`确认撤销令牌「${token.name}」？`)) {
                      revokeMutation.mutate(token.id);
                    }
                  }}
                >
                  撤销
                </button>
              )}
            </div>
            <div className="token-body">
              <div className="info-item">
                <strong>前缀：</strong> <code>{token.prefix}...</code>
              </div>
              <div className="info-item">
                <strong>状态：</strong>{" "}
                {token.revoked_at ? (
                  <span className="badge badge-disabled">已撤销</span>
                ) : (
                  <span className="badge badge-enabled">有效</span>
                )}
              </div>
              <div className="token-meta">
                创建时间：{new Date(token.created_at).toLocaleDateString()}
                {token.revoked_at && (
                  <> | 撤销时间：{new Date(token.revoked_at).toLocaleDateString()}</>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>

      {tokens?.length === 0 && (
        <div className="empty-state">
          <p>暂无 API 令牌。</p>
          <p className="muted">创建令牌后，CI 才能上传制品（artifact）。</p>
        </div>
      )}
    </div>
  );
}
