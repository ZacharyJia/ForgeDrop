import React, { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, Link } from "react-router-dom";
import { api } from "../../api";

export function AppDetailPage() {
  const { appId } = useParams<{ appId: string }>();
  const queryClient = useQueryClient();
  
  const { data, isLoading } = useQuery({
    queryKey: ["app", appId],
    queryFn: () => api.getApp(appId!),
    enabled: !!appId,
  });

  const deleteMutation = useMutation({
    mutationFn: api.deleteService,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app", appId] });
    },
  });

  if (isLoading) {
    return <div className="loading">加载中...</div>;
  }

  if (!data) {
    return <div className="error">未找到应用</div>;
  }

  const app = data.app;
  const services = data.services ?? [];

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
            配置该应用的服务（容器）。每个服务可选择 Docker Compose 模板或 Docker API（手动）模式。
          </p>
        </div>

        {services.length === 0 ? (
          <div className="empty-state">
            <p>暂无服务配置。</p>
            <p className="muted">
              当你通过 API 上传制品（artifact）时，会自动创建相关服务；也可以通过 API 手动创建。
            </p>
          </div>
        ) : (
          <div className="services-list">
            {services.map((service) => (
              <div key={service.id} className="service-card">
                <div className="service-header">
                  <div>
                    <h3>{service.name}</h3>
                    <div className="service-key">{service.service_key}</div>
                  </div>
                  <div className="service-badges">
                    {service.use_compose && (
                      <span className="badge badge-compose">Docker Compose</span>
                    )}
                    {!service.use_compose && (
                      <span className="badge badge-docker">Docker API</span>
                    )}
                    {service.enabled ? (
                      <span className="badge badge-enabled">已启用</span>
                    ) : (
                      <span className="badge badge-disabled">已禁用</span>
                    )}
                  </div>
                </div>

                <div className="service-body">
                  {service.use_compose ? (
                    <div className="service-info">
                      <div className="info-item">
                        <strong>模式：</strong> Docker Compose
                      </div>
                      <div className="info-item">
                        <strong>模板：</strong> {service.compose_template ? "已配置" : "未设置"}
                      </div>
                    </div>
                  ) : (
                    <div className="service-info">
                      <div className="info-item">
                        <strong>镜像：</strong> {service.image}
                      </div>
                      <div className="info-item">
                        <strong>端口：</strong> {service.container_port}
                      </div>
                      <div className="info-item">
                        <strong>启动命令：</strong> <code>{service.command}</code>
                      </div>
                    </div>
                  )}

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
        )}
      </div>

      <div className="section">
        <h2>部署说明</h2>
        <div className="info-box">
          <p>要部署该应用：</p>
          <ol>
            <li>先配置上面的服务</li>
            <li>在 <Link to="/tokens">API 令牌</Link> 页面创建令牌</li>
            <li>在 CI 流水线中通过 API 上传制品（artifact）</li>
          </ol>
          <p className="muted">
            CI 集成示例请参考文档。
          </p>
        </div>
      </div>
    </div>
  );
}
