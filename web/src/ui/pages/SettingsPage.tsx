import React, { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type Settings } from "../../api";

export function SettingsPage() {
  const queryClient = useQueryClient();
  const { data: settings, isLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: api.getSettings,
  });

  const [formData, setFormData] = useState<Partial<Settings>>({});

  useEffect(() => {
    if (settings) {
      setFormData(settings);
    }
  }, [settings]);

  const updateMutation = useMutation({
    mutationFn: () => api.updateSettings(formData),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings"] });
      alert("设置已保存");
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    updateMutation.mutate();
  };

  if (isLoading) {
    return <div className="loading">加载中...</div>;
  }

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
        </div>

        <div className="form-section">
          <h2>集成地址</h2>
          
          <div className="info-box">
            <div className="info-item">
              <strong>制品上传地址：</strong>
              <code>{settings?.artifact_upload_url}</code>
            </div>
            <p className="help-text">
              在 CI 流水线中使用该地址上传制品（artifact）
            </p>
          </div>

          <div className="info-box">
            <div className="info-item">
              <strong>Forgejo Webhook 地址：</strong>
              <code>{settings?.forgejo_webhook_url}</code>
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
