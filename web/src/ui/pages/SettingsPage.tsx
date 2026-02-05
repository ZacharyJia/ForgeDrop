import React, { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type Settings } from "../../api";
import { useToast } from "../toast";

export function SettingsPage() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const { data: settings, isLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: api.getSettings,
  });

  const [formData, setFormData] = useState<Partial<Settings>>({});

  useEffect(() => {
    if (settings) {
      // Only keep editable fields; the settings response includes computed values.
      setFormData({
        base_domain: settings.base_domain,
        preview_host_template: settings.preview_host_template,
        docker_network: settings.docker_network,
        traefik_acme_email: settings.traefik_acme_email || "",
        traefik_acme_mode: settings.traefik_acme_mode || "tls",
        traefik_alicloud_region_id: settings.traefik_alicloud_region_id || "cn-hangzhou",
      });
    }
  }, [settings]);

  const updateMutation = useMutation({
    mutationFn: () => api.updateSettings(formData),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings"] });
      toast.success("设置已保存");
    },
    onError: (e) => {
      toast.error(String(e), "保存失败");
    },
  });

  const traefikStatusQuery = useQuery({
    queryKey: ["traefikStatus"],
    queryFn: api.getTraefikStatus,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const [showTraefikInstall, setShowTraefikInstall] = useState(false);
  const [traefikStaging, setTraefikStaging] = useState(true);

  const [aliAccessKey, setAliAccessKey] = useState("");
  const [aliSecretKey, setAliSecretKey] = useState("");
  const saveAliCredsMutation = useMutation({
    mutationFn: () => api.setTraefikAliyunCredentials(aliAccessKey.trim(), aliSecretKey.trim()),
    onSuccess: () => {
      toast.success("Aliyun 凭证已保存");
      setAliAccessKey("");
      setAliSecretKey("");
      traefikStatusQuery.refetch();
    },
    onError: (e) => toast.error(String(e), "保存失败"),
  });
  const installTraefikMutation = useMutation({
    mutationFn: () => api.installTraefik(traefikStaging),
    onSuccess: (st) => {
      toast.success(st.message || "Traefik 已安装/修复");
      setShowTraefikInstall(false);
      traefikStatusQuery.refetch();
    },
    onError: (e) => toast.error(String(e), "安装失败"),
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    updateMutation.mutate();
  };

  if (isLoading) {
    return <div className="loading">加载中...</div>;
  }

  const copy = async (txt: string | undefined | null, label: string) => {
    const v = (txt ?? "").trim();
    if (!v) {
      toast.error("没有可复制的内容", label);
      return;
    }
    await navigator.clipboard.writeText(v);
    toast.success("已复制到剪贴板", label);
  };

  return (
    <div className="settings-page">
      <h1>设置</h1>

      <form onSubmit={handleSubmit} className="settings-form">
        <div className="form-section">
          <h2>域名配置</h2>
          
          <div className="form-group">
            <label>基础域名</label>
            <input
              type="text"
              value={formData.base_domain || ""}
              onChange={(e) => setFormData({ ...formData, base_domain: e.target.value })}
              placeholder="example.com"
            />
            <p className="help-text">
              用于部署的基础域名（例如：example.com）
            </p>
          </div>

          <div className="form-group">
            <label>预览环境域名模板</label>
            <input
              type="text"
              value={formData.preview_host_template || ""}
              onChange={(e) => setFormData({ ...formData, preview_host_template: e.target.value })}
              placeholder="pr-{app}-{repoSlug}-{pr}-{service}.{base_domain}"
            />
            <p className="help-text">
              预览环境域名模板，可用变量：
              {"{app}"}、{"{repoSlug}"}、{"{pr}"}、{"{service}"}、{"{base_domain}"}
            </p>
          </div>
        </div>

        <div className="form-section">
          <h2>Docker 配置</h2>
          
          <div className="form-group">
            <label>Docker 网络</label>
            <input
              type="text"
              value={formData.docker_network || ""}
              onChange={(e) => setFormData({ ...formData, docker_network: e.target.value })}
              placeholder="traefik"
            />
            <p className="help-text">
              容器加入的 Docker 网络名称（需已存在，且 Traefik 可访问）
            </p>
          </div>

          <div className="form-group">
            <label>Traefik ACME 邮箱</label>
            <input
              type="text"
              value={formData.traefik_acme_email || ""}
              onChange={(e) => setFormData({ ...formData, traefik_acme_email: e.target.value })}
              placeholder="admin@example.com"
            />
            <p className="help-text">用于 Let's Encrypt 证书申请通知；一键安装 Traefik 需要该字段。</p>
          </div>

          <div className="form-group">
            <label>证书方式</label>
            <select
              value={formData.traefik_acme_mode || "tls"}
              onChange={(e) => setFormData({ ...formData, traefik_acme_mode: e.target.value })}
            >
              <option value="dns-alidns">DNS-01（Aliyun，内网推荐）</option>
              <option value="tls">TLS-ALPN（公网 443）</option>
            </select>
            <p className="help-text">内网使用一般无法完成公网 80/443 验证，建议使用 DNS Challenge。</p>
          </div>

          {formData.traefik_acme_mode === "dns-alidns" && (
            <>
              <div className="form-group">
                <label>Aliyun Region ID</label>
                <input
                  type="text"
                  value={formData.traefik_alicloud_region_id || ""}
                  onChange={(e) => setFormData({ ...formData, traefik_alicloud_region_id: e.target.value })}
                  placeholder="cn-hangzhou"
                />
                <p className="help-text">默认 cn-hangzhou。一般无需修改。</p>
              </div>

              <div className="info-box">
                <div className="info-item"><strong>Aliyun DNS 凭证</strong> <span className="muted">（用于创建 TXT 记录）</span></div>
                <div className="muted">出于安全考虑，保存后不会回显明文。</div>

                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem", marginTop: "0.75rem" }}>
                  <div className="form-group" style={{ margin: 0 }}>
                    <label>ALICLOUD_ACCESS_KEY</label>
                    <input type="text" value={aliAccessKey} onChange={(e) => setAliAccessKey(e.target.value)} placeholder="access key id" />
                  </div>
                  <div className="form-group" style={{ margin: 0 }}>
                    <label>ALICLOUD_SECRET_KEY</label>
                    <input type="password" value={aliSecretKey} onChange={(e) => setAliSecretKey(e.target.value)} placeholder="access key secret" />
                  </div>
                </div>

                <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.75rem" }}>
                  <button className="btn-secondary" type="button" onClick={() => saveAliCredsMutation.mutate()} disabled={saveAliCredsMutation.isPending}>
                    {saveAliCredsMutation.isPending ? "保存中..." : "保存凭证"}
                  </button>
                  {traefikStatusQuery.data && (
                    <span className={traefikStatusQuery.data.alicloud_credentials_set ? "badge badge-enabled" : "badge badge-disabled"}>
                      {traefikStatusQuery.data.alicloud_credentials_set ? "已配置" : "未配置"}
                    </span>
                  )}
                </div>
              </div>
            </>
          )}

          <div className="info-box">
            <div className="info-item"><strong>Traefik 状态：</strong> {traefikStatusQuery.data ? (
              <span className={traefikStatusQuery.data.ok ? "badge badge-enabled" : "badge badge-disabled"}>
                {traefikStatusQuery.data.ok ? "已就绪" : "未就绪"}
              </span>
            ) : (
              <span className="muted">未检测</span>
            )}</div>
            {traefikStatusQuery.isError && (
              <div className="error" style={{ marginTop: "0.5rem" }}>{String(traefikStatusQuery.error)}</div>
            )}
            {traefikStatusQuery.data && (
              <div style={{ marginTop: "0.5rem" }}>
                <div className="muted">{traefikStatusQuery.data.message}</div>
                <div style={{ marginTop: "0.5rem" }} className="muted">
                  network=<code>{traefikStatusQuery.data.network_name}</code> container=<code>{traefikStatusQuery.data.container_name}</code>
                </div>
                <div style={{ marginTop: "0.25rem" }} className="muted">
                  acme=<code>{traefikStatusQuery.data.acme_mode}</code>
                </div>
              </div>
            )}
            <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.75rem", flexWrap: "wrap" }}>
              <button className="btn-secondary" type="button" onClick={() => traefikStatusQuery.refetch()} disabled={traefikStatusQuery.isFetching}>
                {traefikStatusQuery.isFetching ? "检测中..." : "检测 Traefik"}
              </button>
              {traefikStatusQuery.data && !traefikStatusQuery.data.ok && (
                <button className="btn-primary" type="button" onClick={() => setShowTraefikInstall(true)}>
                  一键安装/修复 Traefik
                </button>
              )}
            </div>
            <p className="help-text" style={{ marginTop: "0.75rem" }}>
              注意：一键安装会尝试启动一个由 forge-drop 管理的 Traefik 容器，并绑定宿主机 80/443 端口；如果端口已被占用会失败。
            </p>
          </div>
        </div>

        <div className="form-section">
          <h2>集成地址</h2>
          
          <div className="info-box">
            <div className="info-item">
              <strong>制品上传地址：</strong>
              <div className="code-row">
                <code>{settings?.artifact_upload_url}</code>
                <button type="button" className="btn-secondary" onClick={() => copy(settings?.artifact_upload_url, "制品上传地址")}>复制</button>
              </div>
            </div>
            <p className="help-text">
              在 CI 流水线中使用该地址上传制品（artifact）
            </p>
          </div>

          <div className="info-box">
            <div className="info-item">
              <strong>Forgejo Webhook 地址：</strong>
              <div className="code-row">
                <code>{settings?.forgejo_webhook_url}</code>
                <button type="button" className="btn-secondary" onClick={() => copy(settings?.forgejo_webhook_url, "Webhook 地址")}>复制</button>
              </div>
            </div>
            <p className="help-text">
              在 Forgejo 仓库设置中将该地址配置为 Webhook
            </p>
          </div>
        </div>

        {updateMutation.error && (
          <div className="error">{String(updateMutation.error)}</div>
        )}

        <div className="form-actions">
          <button type="submit" disabled={updateMutation.isPending} className="btn-primary">
            {updateMutation.isPending ? "保存中..." : "保存设置"}
          </button>
        </div>
      </form>

      {showTraefikInstall && (
        <div className="modal-overlay" onClick={() => setShowTraefikInstall(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>一键安装/修复 Traefik</h2>
            <div className="info-box">
              <p>将启动容器：<code>forge-drop-traefik</code></p>
              <p className="muted">需要：Docker 可用、80/443 端口空闲、已配置 DNS（含通配符）。</p>
            </div>
            <div className="form-group">
              <label>
                <input type="checkbox" checked={traefikStaging} onChange={(e) => setTraefikStaging(e.target.checked)} />{" "}
                使用 Let's Encrypt Staging（推荐先勾选测试）
              </label>
              <p className="help-text">Staging 不会触发正式证书的速率限制，确认无误后可关闭再安装一次。</p>
            </div>
            <div className="modal-actions">
              <button type="button" className="btn-secondary" onClick={() => setShowTraefikInstall(false)}>
                取消
              </button>
              <button
                type="button"
                className="btn-primary"
                onClick={() => installTraefikMutation.mutate()}
                disabled={installTraefikMutation.isPending}
              >
                {installTraefikMutation.isPending ? "执行中..." : "开始执行"}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="section">
        <h2>配置指南</h2>
        <div className="info-box">
          <ol>
            <li>先配置上面的基础域名和 Docker 网络</li>
            <li>为预览环境配置通配符 DNS（例如：*.example.com 指向你的服务器）</li>
            <li>配置 Traefik（Docker provider）并加入同一网络</li>
            <li>添加仓库并配置 Webhook</li>
            <li>创建应用和服务</li>
            <li>创建 API 令牌用于 CI 集成</li>
          </ol>
        </div>
      </div>
    </div>
  );
}
