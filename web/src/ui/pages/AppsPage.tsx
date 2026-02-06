import React, { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../../api";
import { useToast } from "../toast";

export function AppsPage() {
  const [showCreate, setShowCreate] = useState(false);
  const [appKey, setAppKey] = useState("");
  const [name, setName] = useState("");
  const toast = useToast();
  
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
      toast.success("应用已创建");
    },
    onError: (e) => toast.error(String(e), "创建失败"),
  });

  const deleteMutation = useMutation({
    mutationFn: api.deleteApp,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      toast.success("应用已删除");
    },
    onError: (e) => toast.error(String(e), "删除失败"),
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
        <div>
          <h1>应用</h1>
          <p className="section-desc">管理应用、服务和环境入口。</p>
        </div>
        <button onClick={() => setShowCreate(true)} className="btn-primary">
          + 新建应用
        </button>
      </div>

      <div className="list-meta">共 {apps?.length || 0} 个应用</div>

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
                <button type="button" className="btn-secondary" onClick={() => setShowCreate(false)}>
                  取消
                </button>
                <button type="submit" className="btn-primary" disabled={createMutation.isPending}>
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
