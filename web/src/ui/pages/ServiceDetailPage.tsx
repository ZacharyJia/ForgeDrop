import React, { useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Env, type Slot } from "../../api";

export function ServiceDetailPage() {
  const { serviceId } = useParams<{ serviceId: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["service", serviceId],
    queryFn: () => api.getService(serviceId!),
    enabled: !!serviceId,
  });

  const appQuery = useQuery({
    queryKey: ["appForService", data?.service?.app_id],
    queryFn: () => api.getApp(data!.service.app_id),
    enabled: !!data?.service?.app_id,
  });

  const reposQuery = useQuery({
    queryKey: ["repos"],
    queryFn: api.listRepos,
  });

  const [showCreateSlot, setShowCreateSlot] = useState(false);
  const [newSlot, setNewSlot] = useState({ slot_key: "", name: "", repo_id: "", container_path: "" });

  const createSlotMutation = useMutation({
    mutationFn: () => api.createSlot(serviceId!, newSlot),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["service", serviceId] });
      setShowCreateSlot(false);
      setNewSlot({ slot_key: "", name: "", repo_id: "", container_path: "" });
    },
  });

  const deleteSlotMutation = useMutation({
    mutationFn: ({ slotId }: { slotId: string }) => api.deleteSlot(serviceId!, slotId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["service", serviceId] });
    },
  });

  const envs: Env[] = useMemo(() => {
    const list = appQuery.data?.envs ?? [];
    return list.filter((e) => e.kind === "named");
  }, [appQuery.data]);

  const [selectedEnvID, setSelectedEnvID] = useState<string>("");
  const [sha, setSha] = useState<string>("");
  const [ref, setRef] = useState<string>("");
  const [filesBySlotID, setFilesBySlotID] = useState<Record<string, File | null>>({});
  const [uploadResult, setUploadResult] = useState<any>(null);

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!selectedEnvID) throw new Error("请选择环境");

      const form = new FormData();
      form.append("env_id", selectedEnvID);
      if (sha.trim()) form.append("sha", sha.trim());
      if (ref.trim()) form.append("ref", ref.trim());

      const slots = data?.slots ?? [];
      let count = 0;
      for (const sl of slots) {
        const f = filesBySlotID[sl.id];
        if (!f) continue;
        form.append(`file_${sl.id}`, f);
        count++;
      }
      if (count === 0) {
        throw new Error("请至少选择一个槽位文件再上传");
      }

      return api.uploadArtifactsBatch(serviceId!, form);
    },
    onSuccess: (res) => {
      setUploadResult(res);
      queryClient.invalidateQueries({ queryKey: ["service", serviceId] });
    },
  });

  if (isLoading) {
    return <div className="loading">加载中...</div>;
  }
  if (!data) {
    return <div className="error">未找到服务</div>;
  }

  const { service, slots } = data;

  return (
    <div className="service-detail-page">
      <div className="page-header">
        <div>
          <h1>服务：{service.name}</h1>
          <div className="app-key-badge">{service.service_key}</div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <Link to={`/services/${service.id}/edit`} className="btn-secondary">
            编辑配置
          </Link>
          <button onClick={() => navigate(-1)} className="btn-secondary">
            ← 返回
          </button>
        </div>
      </div>

      <div className="section">
        <div className="section-header">
          <h2>槽位（Artifacts 挂载点）</h2>
          <p className="section-desc">
            槽位用于将上传的制品文件挂载到容器内指定路径。建议先创建槽位，再上传文件。
          </p>
        </div>

        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "1rem" }}>
          <div className="muted">当前槽位数：{slots.length}</div>
          <button className="btn-primary" onClick={() => setShowCreateSlot(true)}>
            + 新建槽位
          </button>
        </div>

        {slots.length === 0 ? (
          <div className="empty-state">
            <p>暂无槽位。</p>
            <p className="muted">先创建槽位（例如 main 对应 /app/app.jar），再上传制品。</p>
          </div>
        ) : (
          <div className="repos-list">
            {slots.map((sl: Slot) => (
              <div key={sl.id} className="repo-card">
                <div className="repo-header">
                  <h3>{sl.name}</h3>
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
                  <div className="info-item"><strong>slot_key：</strong> <code>{sl.slot_key}</code></div>
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
                <select
                  value={newSlot.repo_id}
                  onChange={(e) => setNewSlot({ ...newSlot, repo_id: e.target.value })}
                >
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
                <button
                  className="btn-primary"
                  onClick={() => createSlotMutation.mutate()}
                  disabled={createSlotMutation.isPending}
                  type="button"
                >
                  {createSlotMutation.isPending ? "创建中..." : "创建"}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="section">
        <div className="section-header">
          <h2>手动上传制品（单服务批量）</h2>
          <p className="section-desc">
            用于测试全流程：上传-生成快照-更新当前版本-自动部署。一次上传可同时更新多个槽位，只触发一次部署。
          </p>
        </div>

        {envs.length === 0 ? (
          <div className="info-box">
            <p>该应用暂无命名环境（例如 prod/staging）。</p>
            <p className="muted">请先到应用详情页创建环境。</p>
          </div>
        ) : (
          <div className="form-section">
            <div className="form-group">
              <label>目标环境</label>
              <select value={selectedEnvID} onChange={(e) => setSelectedEnvID(e.target.value)}>
                <option value="">请选择环境</option>
                {envs.map((e) => (
                  <option key={e.id} value={e.id}>
                    {e.name}
                  </option>
                ))}
              </select>
            </div>

            <div className="form-group">
              <label>SHA（可选）</label>
              <input type="text" value={sha} onChange={(e) => setSha(e.target.value)} placeholder="提交 SHA" />
            </div>

            <div className="form-group">
              <label>Ref（可选）</label>
              <input type="text" value={ref} onChange={(e) => setRef(e.target.value)} placeholder="refs/heads/main" />
            </div>

            {slots.length === 0 ? (
              <div className="info-box">
                <p>请先创建槽位后再上传。</p>
              </div>
            ) : (
              <div className="info-box">
                <p className="muted">为需要更新的槽位选择文件，未选择文件的槽位保持不变。</p>
                {slots.map((sl) => (
                  <div key={sl.id} className="form-group">
                    <label>
                      {sl.name}（{sl.slot_key} → <code>{sl.container_path}</code>）
                    </label>
                    <input
                      type="file"
                      onChange={(e) => {
                        const f = e.target.files && e.target.files.length > 0 ? e.target.files[0] : null;
                        setFilesBySlotID((prev) => ({ ...prev, [sl.id]: f }));
                      }}
                    />
                  </div>
                ))}
              </div>
            )}

            {uploadMutation.error && <div className="error">{String(uploadMutation.error)}</div>}
            {uploadResult && (
              <div className="success-message">
                <div>上传成功：快照 <code>{uploadResult.snapshot_id}</code></div>
                {uploadResult.service_url && (
                  <div>
                    访问地址：<code>{uploadResult.service_url}</code>
                  </div>
                )}
              </div>
            )}

            <div className="form-actions">
              <button
                className="btn-primary"
                type="button"
                disabled={uploadMutation.isPending}
                onClick={() => uploadMutation.mutate()}
              >
                {uploadMutation.isPending ? "上传中..." : "上传并部署"}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
