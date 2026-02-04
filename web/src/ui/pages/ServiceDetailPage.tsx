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
  const [autoDeploy, setAutoDeploy] = useState<boolean>(true);

  const statusQuery = useQuery({
    queryKey: ["serviceStatus", serviceId, selectedEnvID],
    queryFn: () => api.getServiceStatus(serviceId!, selectedEnvID),
    enabled: !!serviceId && !!selectedEnvID,
    refetchOnWindowFocus: false,
  });

  const [logsTail, setLogsTail] = useState<number>(200);
  const [logsText, setLogsText] = useState<string>("");

  const fetchLogsMutation = useMutation({
    mutationFn: async () => {
      if (!selectedEnvID) throw new Error("请选择环境");
      const res = await api.getServiceLogs(serviceId!, selectedEnvID, logsTail);
      return res.logs;
    },
    onSuccess: (logs) => {
      setLogsText(logs);
    },
  });

  const deployMutation = useMutation({
    mutationFn: async () => {
      if (!selectedEnvID) throw new Error("请选择环境");
      return api.deployService(serviceId!, selectedEnvID);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["serviceStatus", serviceId, selectedEnvID] });
      toast.success("已触发部署");
    },
    onError: (e) => toast.error(String(e), "部署失败"),
  });

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!selectedEnvID) throw new Error("请选择环境");

      const form = new FormData();
      form.append("env_id", selectedEnvID);
      form.append("deploy", autoDeploy ? "1" : "0");
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
      queryClient.invalidateQueries({ queryKey: ["serviceStatus", serviceId, selectedEnvID] });
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
              <label>
                <input
                  type="checkbox"
                  checked={autoDeploy}
                  onChange={(e) => setAutoDeploy(e.target.checked)}
                />{" "}
                上传后自动部署
              </label>
              <p className="help-text">关闭后会生成快照并更新当前版本，但不会触发容器更新，需要手动点击部署。</p>
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
                <div>
                  上传成功：快照 <code>{uploadResult.snapshot_id}</code>
                  {uploadResult.deploy_skipped ? <span className="muted">（待部署）</span> : null}
                </div>
                {uploadResult.service_url && (
                  <div>
                    访问地址：<code>{uploadResult.service_url}</code>
                  </div>
                )}
                {uploadResult.deploy_skipped && selectedEnvID && (
                  <div style={{ marginTop: "0.75rem" }}>
                    <button
                      className="btn-primary"
                      type="button"
                      onClick={() => deployMutation.mutate()}
                      disabled={deployMutation.isPending}
                    >
                      {deployMutation.isPending ? "部署中..." : "部署当前版本"}
                    </button>
                    {deployMutation.error && <div className="error" style={{ marginTop: "0.5rem" }}>{String(deployMutation.error)}</div>}
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
                {uploadMutation.isPending ? "上传中..." : autoDeploy ? "上传并部署" : "上传（不部署）"}
              </button>
            </div>

            {selectedEnvID && (
              <div className="section" style={{ marginTop: "1.25rem" }}>
                <div className="section-header">
                  <h2>部署状态</h2>
                  <p className="section-desc">基于 Docker Compose 项目（按环境）查询运行状态与日志。</p>
                </div>

                {statusQuery.isLoading ? (
                  <div className="loading">加载中...</div>
                ) : statusQuery.error ? (
                  <div className="error">{String(statusQuery.error)}</div>
                ) : (
                  <div className="info-box">
                    {statusQuery.data?.note && (
                      <div className="muted" style={{ marginBottom: "0.5rem" }}>{String(statusQuery.data.note)}</div>
                    )}
                    {statusQuery.data?.desired_snapshot_id && (
                      <div className="info-item"><strong>当前快照（desired）：</strong> <code>{String(statusQuery.data.desired_snapshot_id)}</code></div>
                    )}
                    {statusQuery.data?.service_url && (
                      <div className="info-item"><strong>访问地址：</strong> <code>{String(statusQuery.data.service_url)}</code></div>
                    )}
                    <div className="info-item"><strong>Project：</strong> <code>{String(statusQuery.data?.project_name || "")}</code></div>
                    {typeof statusQuery.data?.deployed === "boolean" && (
                      <div className="info-item"><strong>已部署：</strong> <code>{String(statusQuery.data.deployed)}</code></div>
                    )}
                    <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.75rem" }}>
                      <button className="btn-secondary" type="button" onClick={() => statusQuery.refetch()} disabled={statusQuery.isFetching}>
                        {statusQuery.isFetching ? "刷新中..." : "刷新状态"}
                      </button>
                      <button
                        className="btn-secondary"
                        type="button"
                        onClick={() => deployMutation.mutate()}
                        disabled={deployMutation.isPending}
                      >
                        {deployMutation.isPending ? "部署中..." : "部署当前版本"}
                      </button>
                    </div>
                    {statusQuery.data?.ps && (
                      <div style={{ marginTop: "0.75rem" }}>
                        <div className="muted" style={{ marginBottom: "0.25rem" }}>docker compose ps</div>
                        <pre style={{ whiteSpace: "pre-wrap" }}>{String(statusQuery.data.ps)}</pre>
                      </div>
                    )}
                    {statusQuery.data?.ps_error && (
                      <div className="error" style={{ marginTop: "0.5rem" }}>{String(statusQuery.data.ps_error)}</div>
                    )}
                  </div>
                )}

                <div className="info-box" style={{ marginTop: "0.75rem" }}>
                  <div style={{ display: "flex", gap: "0.75rem", alignItems: "center" }}>
                    <div className="form-group" style={{ margin: 0 }}>
                      <label>日志 tail</label>
                      <input
                        type="number"
                        value={logsTail}
                        onChange={(e) => setLogsTail(parseInt(e.target.value || "200", 10))}
                        style={{ width: "120px" }}
                      />
                    </div>
                    <button
                      className="btn-secondary"
                      type="button"
                      onClick={() => fetchLogsMutation.mutate()}
                      disabled={fetchLogsMutation.isPending}
                    >
                      {fetchLogsMutation.isPending ? "拉取中..." : "查看日志"}
                    </button>
                  </div>
                  {fetchLogsMutation.error && <div className="error" style={{ marginTop: "0.5rem" }}>{String(fetchLogsMutation.error)}</div>}
                  {logsText && (
                    <pre style={{ marginTop: "0.75rem", whiteSpace: "pre-wrap" }}>{logsText}</pre>
                  )}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
