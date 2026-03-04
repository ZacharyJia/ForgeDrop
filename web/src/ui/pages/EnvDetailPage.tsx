import React, { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Artifact, type EnvDetail, type Service, type Slot } from "../../api";
import { useToast } from "../toast";

function formatBytes(n: number) {
  if (!Number.isFinite(n)) return "-";
  const units = ["B", "KB", "MB", "GB"];
  let x = n;
  let u = 0;
  while (x >= 1024 && u < units.length - 1) {
    x /= 1024;
    u++;
  }
  return `${x.toFixed(u === 0 ? 0 : 1)} ${units[u]}`;
}

function ArtifactRow({ a }: { a: Artifact }) {
  return (
    <div className="info-item">
      <div>
        <strong>{a.original_filename}</strong>
        <span className="muted"> · {formatBytes(a.size_bytes)} · {new Date(a.created_at).toLocaleString()}</span>
      </div>
      <div className="muted">sha={a.sha || "-"} ref={a.ref || "-"}</div>
    </div>
  );
}

function ServicePanel(props: {
  envId: string;
  envKind: string;
  service: Service;
  slots: Slot[];
}) {
  const { envId, service, slots } = props;
  const toast = useToast();
  const queryClient = useQueryClient();

  const statusQuery = useQuery({
    queryKey: ["serviceStatus", service.id, envId],
    queryFn: () => api.getServiceStatus(service.id, envId),
    enabled: !!envId,
    refetchOnWindowFocus: false,
  });

  const artifactsQuery = useQuery({
    queryKey: ["envServiceSlotArtifacts", envId, service.id],
    queryFn: () => api.getEnvServiceSlotArtifacts(envId, service.id),
    enabled: !!envId,
    refetchOnWindowFocus: false,
  });

  const [autoDeploy, setAutoDeploy] = useState(true);
  const [deployStrategy, setDeployStrategy] = useState<'recreate' | 'restart'>(service.deploy_strategy === 'restart' ? 'restart' : 'recreate');
  useEffect(() => {
    setDeployStrategy(service.deploy_strategy === 'restart' ? 'restart' : 'recreate');
  }, [service.deploy_strategy]);
  const [sha, setSha] = useState("");
  const [ref, setRef] = useState("");
  const [filesBySlotID, setFilesBySlotID] = useState<Record<string, File | null>>({});

  const [logsTail, setLogsTail] = useState<number>(200);
  const [logsText, setLogsText] = useState<string>("");

  const fetchLogsMutation = useMutation({
    mutationFn: async () => {
      const res = await api.getServiceLogs(service.id, envId, logsTail);
      return res.logs;
    },
    onSuccess: (logs) => setLogsText(logs),
    onError: (e) => toast.error(String(e), "日志获取失败"),
  });

  const uploadMutation = useMutation({
    mutationFn: async () => {
      const form = new FormData();
      form.append("env_id", envId);
      form.append("deploy", autoDeploy ? "1" : "0");
		form.append("deploy_strategy", deployStrategy);
      if (sha.trim()) form.append("sha", sha.trim());
      if (ref.trim()) form.append("ref", ref.trim());

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
      return api.uploadArtifactsBatch(service.id, form);
    },
    onSuccess: (res) => {
      toast.success(res.deploy_skipped ? "上传完成（待部署）" : "上传并部署完成");
      queryClient.invalidateQueries({ queryKey: ["env", envId] });
      queryClient.invalidateQueries({ queryKey: ["envServiceSlotArtifacts", envId, service.id] });
      queryClient.invalidateQueries({ queryKey: ["serviceStatus", service.id, envId] });
      queryClient.invalidateQueries({ queryKey: ["envSnapshots", envId] });
      setFilesBySlotID({});
    },
    onError: (e) => toast.error(String(e), "上传失败"),
  });

  const deployMutation = useMutation({
    mutationFn: (strategy: 'recreate' | 'restart') => api.deployService(service.id, envId, strategy),
    onSuccess: () => {
      toast.success("已触发部署");
      queryClient.invalidateQueries({ queryKey: ["serviceStatus", service.id, envId] });
    },
    onError: (e) => toast.error(String(e), "部署失败"),
  });

  const artifactsBySlotKey = (artifactsQuery.data?.artifacts_by_slot_key || {}) as Record<string, Artifact>;

  return (
    <details className="service-card" open>
      <summary className="service-header service-panel-summary">
        <div>
          <h3>{service.name}</h3>
          <div className="service-key">{service.service_key}</div>
        </div>
        <div className="service-badges">
          <span className="badge badge-compose">Docker Compose</span>
          {service.enabled ? <span className="badge badge-enabled">已启用</span> : <span className="badge badge-disabled">已禁用</span>}
        </div>
      </summary>

      <div className="service-body service-panel-body">
        <div className="info-box">
          <div className="info-item"><strong>端口：</strong> {service.container_port}</div>
          {statusQuery.data?.service_url && (
            <div className="info-item"><strong>访问地址：</strong> <code>{String(statusQuery.data.service_url)}</code></div>
          )}
          {statusQuery.data?.desired_snapshot_id && (
            <div className="info-item"><strong>当前快照（desired）：</strong> <code>{String(statusQuery.data.desired_snapshot_id)}</code></div>
          )}

          <div className="panel-actions">
            <button className="btn-secondary" type="button" onClick={() => statusQuery.refetch()} disabled={statusQuery.isFetching}>
              {statusQuery.isFetching ? "刷新中..." : "刷新状态"}
            </button>
            <button className="btn-secondary" type="button" onClick={() => deployMutation.mutate('restart')} disabled={deployMutation.isPending}>
              {deployMutation.isPending ? "执行中..." : "快速重启"}
            </button>
            <button className="btn-secondary" type="button" onClick={() => deployMutation.mutate('recreate')} disabled={deployMutation.isPending}>
              {deployMutation.isPending ? "执行中..." : "重建部署"}
            </button>
          </div>

          {statusQuery.data?.ps && (
            <div className="status-block">
              <div className="status-label">docker compose ps</div>
              <pre className="pre-wrap">{String(statusQuery.data.ps)}</pre>
            </div>
          )}
          {statusQuery.data?.ps_error && (
            <div className="error status-error">{String(statusQuery.data.ps_error)}</div>
          )}
          {!statusQuery.data?.ps && !statusQuery.isPending && (
            <div className="status-muted">暂无运行状态（可能尚未部署）。</div>
          )}

          <div className="info-box logs-box">
            <div className="logs-controls">
              <div className="form-group form-group-compact">
                <label>日志 tail</label>
                <input
                  type="number"
                  min={20}
                  value={logsTail}
                  onChange={(e) => {
                    const nextTail = Number.parseInt(e.target.value || "200", 10);
                    setLogsTail(Number.isNaN(nextTail) ? 200 : Math.max(20, nextTail));
                  }}
                  className="logs-tail-input"
                />
              </div>
              <button className="btn-secondary" type="button" onClick={() => fetchLogsMutation.mutate()} disabled={fetchLogsMutation.isPending}>
                {fetchLogsMutation.isPending ? "拉取中..." : "查看日志"}
              </button>
            </div>
            {logsText && (
              <pre className="pre-wrap logs-output">{logsText}</pre>
            )}
          </div>
        </div>

        <div className="info-box">
          <div className="section-header section-header-compact">
            <h2 className="section-title-sm">槽位与文件版本</h2>
            <p className="section-desc">当前环境下，槽位绑定到当前快照（snapshot）的 artifact。</p>
          </div>
          {slots.length === 0 ? (
            <div className="muted">该服务暂无槽位，请先到服务详情页配置槽位。</div>
          ) : (
            <div>
              {slots.map((sl) => {
                const a = artifactsBySlotKey[sl.slot_key];
                return (
                  <div key={sl.id} className="slot-row">
                    <div className="info-item"><strong>{sl.name}</strong> <span className="muted">({sl.slot_key} · {sl.mount_type} → <code>{sl.container_path}</code>)</span></div>
                    {a ? <ArtifactRow a={a} /> : <div className="muted">未上传</div>}
                  </div>
                );
              })}
            </div>
          )}
        </div>

        <div className="form-section">
          <h2>上传到此环境（生成新快照）</h2>
          <div className="form-group">
            <label>
              <input type="checkbox" checked={autoDeploy} onChange={(e) => setAutoDeploy(e.target.checked)} />{" "}
              上传后自动部署
            </label>
            <p className="help-text">关闭后会更新环境的当前快照（desired），但不会触发容器更新。</p>
          </div>

		  {autoDeploy && (
			<div className="form-group">
			  <label>部署方式</label>
			  <select value={deployStrategy} onChange={(e) => setDeployStrategy(e.target.value as any)}>
				<option value="recreate">重建部署（down + up，推荐）</option>
				<option value="restart">快速重启（restart，更快）</option>
			  </select>
			  <p className="help-text">重建部署更确定；快速重启适合仅更新 jar/config 文件的场景。</p>
			</div>
		  )}

          <div className="two-col-grid">
            <div className="form-group">
              <label>sha（可选）</label>
              <input type="text" value={sha} onChange={(e) => setSha(e.target.value)} placeholder="commit sha" />
            </div>
            <div className="form-group">
              <label>ref（可选）</label>
              <input type="text" value={ref} onChange={(e) => setRef(e.target.value)} placeholder="refs/heads/main" />
            </div>
          </div>

          {slots.map((sl) => (
            <div key={sl.id} className="form-group">
              <label>
                {sl.name} <span className="muted">({sl.slot_key})</span>
              </label>
              <input
                type="file"
                onChange={(e) => {
                  const f = e.target.files && e.target.files[0] ? e.target.files[0] : null;
                  setFilesBySlotID((prev) => ({ ...prev, [sl.id]: f }));
                }}
              />
              <p className="help-text">挂载到容器：<code>{sl.container_path}</code></p>
              {sl.mount_type === "dir" ? (
                <p className="help-text">dir 类型仅支持上传 zip/tar/tar.gz/tgz。</p>
              ) : (
                <p className="help-text">file 类型会挂载单个文件。</p>
              )}
            </div>
          ))}

          <div className="form-actions">
            <button className="btn-primary" type="button" onClick={() => uploadMutation.mutate()} disabled={uploadMutation.isPending}>
              {uploadMutation.isPending ? "上传中..." : autoDeploy ? "上传并部署" : "上传（不部署）"}
            </button>
          </div>
        </div>
      </div>
    </details>
  );
}

export function EnvDetailPage() {
  const { envId } = useParams<{ envId: string }>();
  const toast = useToast();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const envQuery = useQuery<EnvDetail>({
    queryKey: ["env", envId],
    queryFn: () => api.getEnv(envId!),
    enabled: !!envId,
    refetchOnWindowFocus: false,
  });

  const snapshotsQuery = useQuery({
    queryKey: ["envSnapshots", envId],
    queryFn: () => api.listEnvSnapshots(envId!),
    enabled: !!envId,
    refetchOnWindowFocus: false,
  });

  const deployEnvMutation = useMutation({
    mutationFn: (strategy: 'recreate' | 'restart') => api.deployEnv(envId!, strategy),
    onSuccess: () => {
      toast.success("已触发环境部署");
      queryClient.invalidateQueries({ queryKey: ["env", envId] });
    },
    onError: (e) => toast.error(String(e), "部署失败"),
  });

  const rollbackMutation = useMutation({
    mutationFn: (snapshotId: string) => api.rollbackEnv(envId!, snapshotId),
    onSuccess: () => {
      toast.success("回滚完成并已应用");
      queryClient.invalidateQueries({ queryKey: ["env", envId] });
      queryClient.invalidateQueries({ queryKey: ["envSnapshots", envId] });
    },
    onError: (e) => toast.error(String(e), "回滚失败"),
  });

  const syncPreviewSnapshotMutation = useMutation({
    mutationFn: () => api.syncEnvFromPreviewSnapshot(envId!),
    onSuccess: (res) => {
      toast.success(`已同步并应用模板快照：${res.snapshot_id}`);
      queryClient.invalidateQueries({ queryKey: ["env", envId] });
      queryClient.invalidateQueries({ queryKey: ["envSnapshots", envId] });
    },
    onError: (e) => toast.error(String(e), "同步失败"),
  });

  const deleteEnvMutation = useMutation({
    mutationFn: () => api.deleteEnv(envId!),
    onSuccess: () => {
      toast.success("环境已删除");
      if (envQuery.data?.app?.id) {
        navigate(`/apps/${envQuery.data.app.id}`);
      } else {
        navigate("/apps");
      }
    },
    onError: (e) => toast.error(String(e), "删除失败"),
  });

  const data = envQuery.data;
  const services = data?.services ?? [];
  const slotsByService = data?.slots_by_service ?? {};
  const isPRPreviewEnv = data?.env?.kind === "preview" && !!data?.env?.repo_id && (!!data?.env?.pr_number || !!data?.env?.change_set);

  const namedEnvInfo = useMemo(() => {
    if (!data?.env) return null;
    if (data.env.kind === "named") return `${data.env.name}（命名环境）`;
    if (data.env.kind === "preview") {
      if (data.env.change_set) {
        return `preview（change_set=${data.env.change_set}）`;
      }
      const pr = data.env.pr_number ? `PR #${data.env.pr_number}` : "-";
      return `preview（${pr}）`;
    }
    return data.env.name;
  }, [data?.env]);

  // Pre-create status queries so the page feels responsive when expanding panels.
  const statusResults = useQueries({
    queries: services.map((svc) => ({
      queryKey: ["serviceStatus", svc.id, envId],
      queryFn: () => api.getServiceStatus(svc.id, envId!),
      enabled: !!envId,
      refetchOnWindowFocus: false,
    })),
  });

  const serviceLinks = useMemo(() => {
    return services
      .map((svc, i) => {
        const url = (statusResults[i] as any)?.data?.service_url as string | undefined;
        return { id: svc.id, name: svc.name, key: svc.service_key, url };
      })
      .filter((x) => !!x.url);
  }, [services, statusResults]);

  if (envQuery.isPending) return <div className="loading">加载中...</div>;
  if (envQuery.isError || !data) return <div className="error">{String(envQuery.error || "未找到环境")}</div>;

  return (
    <div className="env-detail-page">
      <div className="page-header">
        <div>
          <h1>环境：{namedEnvInfo}</h1>
          <div className="muted">
            App：<Link to={`/apps/${data.app.id}`}>{data.app.name}</Link> · EnvID：<code>{data.env.id}</code>
          </div>
        </div>
        <div className="env-toolbar">
          {isPRPreviewEnv && (
            <button
              className="btn-secondary"
              type="button"
              onClick={() => {
                if (confirm("确认从命名环境 preview 同步最新快照，并立即应用到当前预览环境？")) {
                  syncPreviewSnapshotMutation.mutate();
                }
              }}
              disabled={syncPreviewSnapshotMutation.isPending}
            >
              {syncPreviewSnapshotMutation.isPending ? "同步中..." : "同步 Preview 快照"}
            </button>
          )}
          <button
            className="btn-danger-small"
            type="button"
            onClick={() => {
              if (confirm("确认删除该环境？将停止并清理该环境的运行目录（runtime），并从列表中移除。")) {
                deleteEnvMutation.mutate();
              }
            }}
            disabled={deleteEnvMutation.isPending}
          >
            删除环境
          </button>
          <button className="btn-secondary" type="button" onClick={() => envQuery.refetch()} disabled={envQuery.isFetching}>
            {envQuery.isFetching ? "刷新中..." : "刷新"}
          </button>
          <button className="btn-secondary" type="button" onClick={() => deployEnvMutation.mutate('restart')} disabled={deployEnvMutation.isPending}>
            {deployEnvMutation.isPending ? "执行中..." : "快速重启"}
          </button>
          <button className="btn-primary" type="button" onClick={() => deployEnvMutation.mutate('recreate')} disabled={deployEnvMutation.isPending}>
            {deployEnvMutation.isPending ? "执行中..." : "重建部署"}
          </button>
        </div>
      </div>

      <div className="info-box">
        <div className="info-item"><strong>当前快照（desired）：</strong> {data.current_snapshot_id ? <code>{String(data.current_snapshot_id)}</code> : <span className="muted">暂无</span>}</div>
        {data.env.kind === "preview" && !data.env.pr_number && !data.env.change_set && (
          <div className="muted">这是 preview 环境，但缺少 PR/change_set 信息。通常预览环境会由 CI 上传时自动创建。</div>
        )}

        {serviceLinks.length > 0 && (
          <div className="env-links">
            <div className="env-links-title">服务入口</div>
            <div className="url-chips">
              {serviceLinks.map((x) => (
                <a key={x.id} className="url-chip" href={x.url!} target="_blank" rel="noreferrer">
                  <span className="muted">{x.key}</span>
                  <code>{x.url}</code>
                </a>
              ))}
            </div>
          </div>
        )}
      </div>

      <details className="info-box">
        <summary className="collapsible-summary">
          <strong>快照历史</strong> <span className="muted">（回滚会同时应用部署）</span>
        </summary>
        <div className="snapshots-content">
          {snapshotsQuery.isPending ? (
            <div className="muted">加载快照中...</div>
          ) : snapshotsQuery.isError ? (
            <div className="error">{String(snapshotsQuery.error)}</div>
          ) : (snapshotsQuery.data?.length || 0) === 0 ? (
            <div className="muted">暂无快照</div>
          ) : (
            <div>
              {snapshotsQuery.data!.slice(0, 30).map((sn: any) => (
                <div key={sn.id} className="snapshot-row">
                  <div className="snapshot-meta">
                    <div><strong>{new Date(sn.created_at).toLocaleString()}</strong> <span className="muted">·</span> <code>{sn.id}</code></div>
                    {sn.note && <div className="muted snapshot-note">{sn.note}</div>}
                  </div>
                  <button
                    className="btn-secondary"
                    type="button"
                    onClick={() => {
                      if (confirm(`确认回滚到快照 ${sn.id} 并应用部署？`)) {
                        rollbackMutation.mutate(sn.id);
                      }
                    }}
                    disabled={rollbackMutation.isPending}
                  >
                    回滚
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      </details>

      <div className="section">
        <div className="section-header">
          <h2>服务状态与槽位</h2>
          <p className="section-desc">在环境维度统一管理上传、版本与部署。</p>
        </div>

        {services.length === 0 ? (
          <div className="empty-state">
            <p>暂无服务。</p>
            <p className="muted">请先在 App 下创建服务与槽位。</p>
          </div>
        ) : (
          <div className="services-list">
            {services.map((svc) => (
              <ServicePanel
                key={svc.id}
                envId={data.env.id}
                envKind={data.env.kind}
                service={svc}
                slots={slotsByService[svc.id] || []}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
