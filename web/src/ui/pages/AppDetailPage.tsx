import React, { useMemo, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, Link } from "react-router-dom";
import { api } from "../../api";

export function AppDetailPage() {
  const { appId } = useParams<{ appId: string }>();
  const queryClient = useQueryClient();

  const [showCreateService, setShowCreateService] = useState(false);
  const [newServiceKey, setNewServiceKey] = useState("");
  const [newServiceName, setNewServiceName] = useState("");

  const [showCreateEnv, setShowCreateEnv] = useState(false);
  const [newEnvName, setNewEnvName] = useState("");
  
  const { data, isLoading } = useQuery({
    queryKey: ["app", appId],
    queryFn: () => api.getApp(appId!),
    enabled: !!appId,
  });

  const settingsQuery = useQuery({
    queryKey: ["settings"],
    queryFn: api.getSettings,
    refetchOnWindowFocus: false,
  });

  const deleteMutation = useMutation({
    mutationFn: api.deleteService,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app", appId] });
    },
  });

  const createServiceMutation = useMutation({
    mutationFn: () => api.createService(appId!, { service_key: newServiceKey, name: newServiceName }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app", appId] });
      setShowCreateService(false);
      setNewServiceKey("");
      setNewServiceName("");
    },
  });

  const createEnvMutation = useMutation({
    mutationFn: () => api.createEnv(appId!, newEnvName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app", appId] });
      setShowCreateEnv(false);
      setNewEnvName("");
    },
  });

  // Hooks must not be conditional: compute derived values with safe defaults
  // before returning early for loading/error.
  const app = data?.app;
  const services = data?.services ?? [];
  const envs = data?.envs ?? [];
  const appKey = app?.app_key || "";
  const baseDomain = settingsQuery.data?.base_domain || "";
  const namedTpl = settingsQuery.data?.named_host_template || "";
  const previewTpl = settingsQuery.data?.preview_host_template || "";

  const envUrls = useMemo(() => {
    const renderPreviewHost = (tpl: string, appKey2: string, repoSlug: string, pr: number, serviceKey: string, baseDomain2: string) => {
      const t = (tpl || "pr-{app}-{repoSlug}-{pr}-{service}.{base_domain}").trim();
      return t
        .replaceAll("{app}", appKey2)
        .replaceAll("{repoSlug}", repoSlug)
        .replaceAll("{pr}", String(pr))
        .replaceAll("{service}", serviceKey)
        .replaceAll("{base_domain}", baseDomain2);
    };

    const slugDNSLabel = (s: string) => {
      const v = (s || "").trim().toLowerCase();
      if (!v) return "env";
      let out = "";
      let lastDash = false;
      for (let i = 0; i < v.length; i++) {
        const c = v[i];
        const ok = (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9');
        if (ok) {
          out += c;
          lastDash = false;
        } else if (!lastDash) {
          out += '-';
          lastDash = true;
        }
      }
      out = out.replace(/^-+/, "").replace(/-+$/, "");
      return out || "env";
    };

    const renderNamedHost = (tpl: string, appKey2: string, envName: string, serviceKey: string, baseDomain2: string) => {
      const t = (tpl || "{app}-{service}-{env}.{base_domain}").trim();
      const envSlug = slugDNSLabel(envName);
      return t
        .replaceAll("{app}", appKey2)
        .replaceAll("{env}", envSlug)
        .replaceAll("{service}", serviceKey)
        .replaceAll("{base_domain}", baseDomain2);
    };

    const out: Record<string, { label: string; url: string }[]> = {};
    for (const e of envs as any[]) {
      const list: { label: string; url: string }[] = [];

      if (e.kind === "named" && baseDomain) {
        const tpl = (namedTpl || "{app}-{service}-{env}.{base_domain}").trim();
        for (const svc of services) {
          const host = (e.name === "prod" && svc.prod_host) ? svc.prod_host : renderNamedHost(tpl, appKey, e.name, svc.service_key, baseDomain);
          if (host) list.push({ label: svc.service_key, url: `https://${host}` });
        }
      }

      if (e.kind === "preview" && e.repo_slug && e.pr_number && baseDomain && appKey) {
        for (const svc of services) {
          const host = renderPreviewHost(previewTpl, appKey, e.repo_slug, e.pr_number, svc.service_key, baseDomain);
          if (host) list.push({ label: svc.service_key, url: `https://${host}` });
        }
      }

      out[e.id] = list;
    }
    return out;
  }, [envs, services, baseDomain, namedTpl, previewTpl, appKey]);

  if (isLoading) return <div className="loading">加载中...</div>;
  if (!data || !app) return <div className="error">未找到应用</div>;

  return (
    <div className="app-detail-page">
      <div className="page-header">
        <div>
          <h1>{app.name}</h1>
          <div className="app-key-badge">{app.app_key}</div>
        </div>
        <Link to="/apps" className="btn-secondary">
          ← 返回应用列表
        </Link>
      </div>

      <div className="section">
        <div className="section-header">
          <h2>服务</h2>
          <p className="section-desc">
            配置该应用的服务（容器）。当前仅支持 Docker Compose 模板部署。
          </p>
        </div>

        {services.length === 0 ? (
          <div className="empty-state">
            <p>暂无服务配置。</p>
              <p className="muted">
              建议先在此处创建服务与槽位，然后进入环境详情页上传制品并部署。
              </p>
            <button className="btn-primary" onClick={() => setShowCreateService(true)}>
              + 新建服务
            </button>
          </div>
        ) : (
          <>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "1rem" }}>
              <div className="muted">服务数：{services.length}</div>
              <button className="btn-primary" onClick={() => setShowCreateService(true)}>
                + 新建服务
              </button>
            </div>

            <div className="services-list">
              {services.map((service) => (
                <div key={service.id} className="service-card">
                  <div className="service-header">
                    <div>
                      <h3>{service.name}</h3>
                      <div className="service-key">{service.service_key}</div>
                    </div>
                    <div className="service-badges">
                      <span className="badge badge-compose">Docker Compose</span>
                      {service.enabled ? (
                        <span className="badge badge-enabled">已启用</span>
                      ) : (
                        <span className="badge badge-disabled">已禁用</span>
                      )}
                    </div>
                  </div>

                  <div className="service-body">
                    <div className="service-info">
                      <div className="info-item">
                        <strong>模板：</strong> {service.compose_template ? "已配置" : "未设置"}
                      </div>
                      <div className="info-item">
                        <strong>端口：</strong> {service.container_port}
                      </div>
                    </div>

                    {service.prod_host && (
                      <div className="info-item">
                        <strong>生产域名：</strong> {service.prod_host}
                      </div>
                    )}

                    <div className="service-meta">
                      版本：{service.revision} | 更新时间：{new Date(service.updated_at).toLocaleString()}
                    </div>
                  </div>

                  <div className="service-actions">
                      <Link to={`/services/${service.id}`} className="btn-secondary">
                        槽位配置
                      </Link>
                    <Link to={`/services/${service.id}/edit`} className="btn-secondary">
                      编辑配置
                    </Link>
                    <button
                      className="btn-danger-small"
                      onClick={() => {
                        if (confirm(`确认删除服务「${service.name}」？`)) {
                          deleteMutation.mutate(service.id);
                        }
                      }}
                    >
                      删除
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>

        <div className="section">
          <div className="section-header">
            <h2>环境</h2>
            <p className="section-desc">环境是部署/版本/上传的中心入口。建议使用环境详情页完成上传与部署。</p>
          </div>

        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "1rem" }}>
          <div className="muted">环境数：{envs.length}</div>
          <button className="btn-primary" onClick={() => setShowCreateEnv(true)}>
            + 新建环境
          </button>
        </div>

        {envs.length === 0 ? (
          <div className="empty-state">
            <p>暂无环境。</p>
            <p className="muted">建议先创建 prod 环境。</p>
          </div>
        ) : (
          <div className="repos-list">
            {envs.map((e) => (
              <div key={e.id} className="repo-card">
                <div className="repo-header">
                  <div>
                    <h3>
                      {e.kind === "preview" && e.pr_number ? `PR #${e.pr_number}` : e.name}
                    </h3>
                    <div className="muted">
                      {e.kind === "named" ? "命名环境" : "PR 预览"}
                      {e.kind === "preview" && e.repo_slug ? ` · ${e.repo_slug}` : ""}
                    </div>
                  </div>
                  <Link to={`/envs/${e.id}`} className="btn-secondary">进入环境</Link>
                </div>
                <div className="repo-body">
                  {envUrls[e.id] && envUrls[e.id].length > 0 ? (
                    <>
                      <div className="muted" style={{ marginBottom: "0.25rem" }}>入口 URL</div>
                      <div className="url-chips">
                        {envUrls[e.id].slice(0, 6).map((x) => (
                          <a key={x.label + x.url} className="url-chip" href={x.url} target="_blank" rel="noreferrer">
                            <span className="muted">{x.label}</span>
                            <code>{x.url}</code>
                          </a>
                        ))}
                      </div>
                      {envUrls[e.id].length > 6 && <div className="muted" style={{ marginTop: "0.25rem" }}>更多入口请进入环境详情查看</div>}
                    </>
                  ) : (
                    <div className="muted">暂无可计算的 URL（请先配置 base_domain / host template，或该环境暂无服务）</div>
                  )}
                  <div className="repo-meta">创建时间：{new Date(e.created_at).toLocaleString()}</div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {showCreateService && (
        <div className="modal-overlay" onClick={() => setShowCreateService(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>新建服务</h2>
            <div className="form-group">
              <label>服务标识（service_key）</label>
              <input
                type="text"
                value={newServiceKey}
                onChange={(e) => setNewServiceKey(e.target.value)}
                placeholder="api"
              />
              <p className="help-text">建议使用小写字母/数字/连字符。</p>
            </div>
            <div className="form-group">
              <label>服务名称</label>
              <input
                type="text"
                value={newServiceName}
                onChange={(e) => setNewServiceName(e.target.value)}
                placeholder="后端服务"
              />
            </div>
            {createServiceMutation.error && <div className="error">{String(createServiceMutation.error)}</div>}
            <div className="modal-actions">
              <button className="btn-secondary" onClick={() => setShowCreateService(false)} type="button">取消</button>
              <button
                className="btn-primary"
                onClick={() => createServiceMutation.mutate()}
                disabled={createServiceMutation.isPending}
                type="button"
              >
                {createServiceMutation.isPending ? "创建中..." : "创建"}
              </button>
            </div>
          </div>
        </div>
      )}

      {showCreateEnv && (
        <div className="modal-overlay" onClick={() => setShowCreateEnv(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>新建环境</h2>
            <div className="form-group">
              <label>环境名称</label>
              <input
                type="text"
                value={newEnvName}
                onChange={(e) => setNewEnvName(e.target.value)}
                placeholder="prod"
              />
              <p className="help-text">常用：prod、staging</p>
            </div>
            {createEnvMutation.error && <div className="error">{String(createEnvMutation.error)}</div>}
            <div className="modal-actions">
              <button className="btn-secondary" onClick={() => setShowCreateEnv(false)} type="button">取消</button>
              <button
                className="btn-primary"
                onClick={() => createEnvMutation.mutate()}
                disabled={createEnvMutation.isPending}
                type="button"
              >
                {createEnvMutation.isPending ? "创建中..." : "创建"}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="section">
        <h2>部署说明</h2>
        <div className="info-box">
          <p>要部署该应用：</p>
          <ol>
            <li>创建服务与环境</li>
            <li>进入服务详情页创建槽位（挂载路径）</li>
            <li>进入环境详情页上传制品并部署（推荐）</li>
            <li>（可选）在 <Link to="/tokens">API 令牌</Link> 页面创建令牌，用于 CI 上传</li>
          </ol>
          <p className="muted">
            CI 集成示例请参考文档。
          </p>
        </div>
      </div>
    </div>
  );
}
