import React, { useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Env, type Slot } from "../../api";
import { useToast } from "../toast";

export function ServiceDetailPage() {
  const { serviceId } = useParams<{ serviceId: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();

  const { data, isLoading } = useQuery({
    queryKey: ["service", serviceId],
    queryFn: () => api.getService(serviceId!),
    enabled: !!serviceId,
    refetchOnWindowFocus: false,
  });

  const appQuery = useQuery({
    queryKey: ["appForService", data?.service?.app_id],
    queryFn: () => api.getApp(data!.service.app_id),
    enabled: !!data?.service?.app_id,
    refetchOnWindowFocus: false,
  });

  const reposQuery = useQuery({
    queryKey: ["repos"],
    queryFn: api.listRepos,
    refetchOnWindowFocus: false,
  });

  const [showCreateSlot, setShowCreateSlot] = useState(false);
  const [newSlot, setNewSlot] = useState({ slot_key: "", name: "", repo_id: "", container_path: "" });

  const createSlotMutation = useMutation({
    mutationFn: () => api.createSlot(serviceId!, newSlot),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["service", serviceId] });
      setShowCreateSlot(false);
      setNewSlot({ slot_key: "", name: "", repo_id: "", container_path: "" });
      toast.success("槽位已创建");
    },
    onError: (e) => toast.error(String(e), "创建失败"),
  });

  const deleteSlotMutation = useMutation({
    mutationFn: ({ slotId }: { slotId: string }) => api.deleteSlot(serviceId!, slotId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["service", serviceId] });
      toast.success("槽位已删除");
    },
    onError: (e) => toast.error(String(e), "删除失败"),
  });

  const envs: Env[] = useMemo(() => {
    return appQuery.data?.envs ?? [];
  }, [appQuery.data]);

  if (isLoading) return <div className="loading">加载中...</div>;
  if (!data) return <div className="error">未找到服务</div>;

  const { service, slots } = data;

  return (
    <div className="service-detail-page">
      <div className="page-header">
        <div>
          <h1>槽位配置：{service.name}</h1>
          <div className="app-key-badge">{service.service_key}</div>
        </div>
        <div className="service-detail-toolbar">
          <Link to={`/services/${service.id}/edit`} className="btn-secondary">
            编辑模板
          </Link>
          <button onClick={() => navigate(-1)} className="btn-secondary">
            ← 返回
          </button>
        </div>
      </div>

      <div className="info-box">
        <div className="info-item"><strong>说明：</strong>上传/部署/状态查看已统一迁移到<strong>环境详情页</strong>。</div>
        <div className="muted">这里仅负责定义 slot（repo 绑定 + 容器挂载路径）。</div>
      </div>

      <div className="section">
        <div className="section-header">
          <h2>环境入口</h2>
          <p className="section-desc">进入环境后可查看该环境下各服务状态、槽位文件版本，并统一上传与部署。</p>
        </div>
        {envs.length === 0 ? (
          <div className="empty-state">
            <p>暂无环境。</p>
            <p className="muted">请先到 App 详情页创建环境（新建 App 会默认创建 prod 与 preview）。</p>
          </div>
        ) : (
          <div className="repos-list">
            {envs.map((e) => (
              <div key={e.id} className="repo-card">
                <div className="repo-header">
                  <div>
                    <h3>{e.name}</h3>
                    <div className="muted">{e.kind === "named" ? "命名环境" : e.kind}</div>
                  </div>
                  <Link to={`/envs/${e.id}`} className="btn-secondary">进入环境</Link>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="section">
        <div className="section-header">
          <h2>槽位（Slots）</h2>
          <p className="section-desc">一个服务可以有多个 slot（例如 jar + config + assets）。</p>
        </div>

        <div className="section-toolbar">
          <div className="section-toolbar-note">当前槽位数：{slots.length}</div>
          <button className="btn-primary" onClick={() => setShowCreateSlot(true)}>
            + 新建槽位
          </button>
        </div>

        {slots.length === 0 ? (
          <div className="empty-state">
            <p>暂无槽位。</p>
            <p className="muted">先创建 slot（例如 main → /app/app.jar），再到环境详情页上传。</p>
          </div>
        ) : (
          <div className="repos-list">
            {slots.map((sl: Slot) => (
              <div key={sl.id} className="repo-card">
                <div className="repo-header">
                  <div>
                    <h3>{sl.name}</h3>
                    <div className="muted">{sl.slot_key}</div>
                  </div>
                  <button
                    className="btn-danger-small"
                    onClick={() => {
                      if (confirm(`确认删除槽位「${sl.name}」？`)) {
                        deleteSlotMutation.mutate({ slotId: sl.id });
                      }
                    }}
                  >
                    删除
                  </button>
                </div>
                <div className="repo-body">
                  <div className="info-item"><strong>repo_id：</strong> <code>{sl.repo_id}</code></div>
                  <div className="info-item"><strong>容器内路径：</strong> <code>{sl.container_path}</code></div>
                </div>
              </div>
            ))}
          </div>
        )}

        {showCreateSlot && (
          <div className="modal-overlay" onClick={() => setShowCreateSlot(false)}>
            <div className="modal" onClick={(e) => e.stopPropagation()}>
              <h2>新建槽位</h2>
              <div className="form-group">
                <label>槽位标识（slot_key）</label>
                <input
                  type="text"
                  value={newSlot.slot_key}
                  onChange={(e) => setNewSlot({ ...newSlot, slot_key: e.target.value })}
                  placeholder="main"
                />
              </div>
              <div className="form-group">
                <label>名称</label>
                <input
                  type="text"
                  value={newSlot.name}
                  onChange={(e) => setNewSlot({ ...newSlot, name: e.target.value })}
                  placeholder="主程序包"
                />
              </div>
              <div className="form-group">
                <label>仓库</label>
                <select value={newSlot.repo_id} onChange={(e) => setNewSlot({ ...newSlot, repo_id: e.target.value })}>
                  <option value="">请选择仓库</option>
                  {(reposQuery.data ?? []).map((r) => (
                    <option key={r.id} value={r.id}>
                      {r.full_name}
                    </option>
                  ))}
                </select>
                <p className="help-text">该槽位只允许来自该仓库的制品上传。</p>
              </div>
              <div className="form-group">
                <label>容器内挂载路径</label>
                <input
                  type="text"
                  value={newSlot.container_path}
                  onChange={(e) => setNewSlot({ ...newSlot, container_path: e.target.value })}
                  placeholder="/app/app.jar"
                />
              </div>
              {createSlotMutation.error && <div className="error">{String(createSlotMutation.error)}</div>}
              <div className="modal-actions">
                <button onClick={() => setShowCreateSlot(false)} className="btn-secondary" type="button">取消</button>
                <button className="btn-primary" onClick={() => createSlotMutation.mutate()} disabled={createSlotMutation.isPending} type="button">
                  {createSlotMutation.isPending ? "创建中..." : "创建"}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
