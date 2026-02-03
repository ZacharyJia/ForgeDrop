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
    return <div className="loading">加载中...</div>;
  }

  return (
    <div className="apps-page">
      <div className="page-header">
        <h1>应用</h1>
        <button onClick={() => setShowCreate(true)} className="btn-primary">
          + 新建应用
        </button>
      </div>

      {showCreate && (
        <div className="modal-overlay" onClick={() => setShowCreate(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>新建应用</h2>
            <form onSubmit={handleCreate}>
              <div className="form-group">
                <label>应用标识（唯一）</label>
                <input
                  type="text"
                  value={appKey}
                  onChange={(e) => setAppKey(e.target.value)}
                  placeholder="my-app"
                  required
                  pattern="[a-z0-9-]+"
                  title="仅允许小写字母、数字和连字符 (-)"
                />
              </div>
              <div className="form-group">
                <label>显示名称</label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="我的应用"
                  required
                />
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

      <div className="apps-grid">
        {apps?.map((app) => (
          <div key={app.id} className="app-card">
            <div className="app-card-header">
              <h3>{app.name}</h3>
              <button
                className="btn-danger-small"
                onClick={() => {
                  if (confirm(`确认删除应用「${app.name}」？`)) {
                    deleteMutation.mutate(app.id);
                  }
                }}
              >
                删除
              </button>
            </div>
            <div className="app-card-body">
              <div className="app-key">{app.app_key}</div>
              <div className="app-meta">
                创建时间：{app.created_at ? new Date(app.created_at).toLocaleDateString() : '未知'}
              </div>
            </div>
            <Link to={`/apps/${app.id}`} className="app-card-link">
              查看详情 →
            </Link>
          </div>
        ))}
      </div>

      {apps?.length === 0 && (
        <div className="empty-state">
          <p>暂无应用。可以先创建一个应用开始使用。</p>
        </div>
      )}
    </div>
  );
}
