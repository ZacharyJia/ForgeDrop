import React, { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useNavigate } from "react-router-dom";
import { api, type Service } from "../../api";

export function ServiceEditPage() {
  const { serviceId } = useParams<{ serviceId: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["service", serviceId],
    queryFn: () => api.getService(serviceId!),
    enabled: !!serviceId,
  });

  const { data: templateExample } = useQuery({
    queryKey: ["compose-template-example", serviceId],
    queryFn: () => api.getComposeTemplateExample(serviceId!),
    enabled: !!serviceId,
  });

  const [formData, setFormData] = useState<Partial<Service>>({});
  const [showExample, setShowExample] = useState(false);

  useEffect(() => {
    if (data?.service) {
      setFormData(data.service);
    }
  }, [data]);

  const updateMutation = useMutation({
    mutationFn: () => api.updateService(serviceId!, formData),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["service", serviceId] });
      alert("Service updated successfully!");
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    updateMutation.mutate();
  };

  if (isLoading) {
    return <div className="loading">加载中...</div>;
  }

  if (!data) {
    return <div className="error">未找到服务</div>;
  }

  const { service } = data;

  return (
    <div className="service-edit-page">
      <div className="page-header">
        <h1>编辑服务：{service.name}</h1>
        <button onClick={() => navigate(-1)} className="btn-secondary">
          ← 返回
        </button>
      </div>

      <form onSubmit={handleSubmit} className="service-form">
        <div className="form-section">
          <h2>基本信息</h2>
          
          <div className="form-group">
            <label>服务名称</label>
            <input
              type="text"
              value={formData.name || ""}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              required
            />
          </div>

          <div className="form-group">
            <label>
              <input
                type="checkbox"
                checked={formData.enabled ?? true}
                onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
              />
              {" "}启用
            </label>
          </div>
        </div>

        <div className="form-section">
          <h2>部署模式</h2>
          
          <div className="form-group">
            <label className="radio-label">
              <input
                type="radio"
                checked={!formData.use_compose}
                onChange={() => setFormData({ ...formData, use_compose: false })}
              />
              {" "}Docker API（手动配置）
            </label>
            <p className="help-text">
              手动配置容器参数，适合简单的单容器服务。
            </p>
          </div>

          <div className="form-group">
            <label className="radio-label">
              <input
                type="radio"
                checked={formData.use_compose ?? false}
                onChange={() => setFormData({ ...formData, use_compose: true })}
              />
              {" "}Docker Compose（模板）
            </label>
            <p className="help-text">
              使用 Docker Compose 模板获得完全控制能力：支持多容器、资源限制、健康检查等 Compose 特性。
            </p>
          </div>
        </div>

        {!formData.use_compose ? (
          <div className="form-section">
            <h2>Docker API 配置</h2>
            
            <div className="form-group">
              <label>镜像</label>
              <input
                type="text"
                value={formData.image || ""}
                onChange={(e) => setFormData({ ...formData, image: e.target.value })}
                placeholder="eclipse-temurin:17-jre"
              />
            </div>

            <div className="form-group">
              <label>启动命令</label>
              <input
                type="text"
                value={formData.command || ""}
                onChange={(e) => setFormData({ ...formData, command: e.target.value })}
                placeholder="java -jar /app/app.jar"
              />
            </div>

            <div className="form-group">
              <label>容器端口</label>
              <input
                type="number"
                value={formData.container_port || 8080}
                onChange={(e) => setFormData({ ...formData, container_port: parseInt(e.target.value) })}
              />
            </div>

            <div className="form-group">
              <label>运行用户（UID:GID）</label>
              <input
                type="text"
                value={formData.run_user || ""}
                onChange={(e) => setFormData({ ...formData, run_user: e.target.value })}
                placeholder="1000:1000"
              />
            </div>
          </div>
        ) : (
          <div className="form-section">
            <h2>Docker Compose 模板</h2>
            
            <div className="form-group">
              <div className="label-with-action">
                <label>Compose 模板（Go template 语法）</label>
                <button
                  type="button"
                  className="btn-link"
                  onClick={() => setShowExample(!showExample)}
                >
                  {showExample ? "隐藏" : "查看"} 示例
                </button>
              </div>
              
              {showExample && templateExample && (
                <div className="example-box">
                  <pre>{templateExample.example}</pre>
                </div>
              )}

              <textarea
                value={formData.compose_template || ""}
                onChange={(e) => setFormData({ ...formData, compose_template: e.target.value })}
                rows={20}
                placeholder="services:\n  app:\n    image: your-image\n    ..."
                className="code-textarea"
              />
              <p className="help-text">
                使用 Go template 语法，可使用变量如 {`{{.Artifacts}}`}、{`{{.Host}}`}、{`{{.EnvName}}`} 等。
                变量列表可查看上方示例。
              </p>
            </div>
          </div>
        )}

        <div className="form-section">
          <h2>路由配置</h2>
          
          <div className="form-group">
            <label>生产域名（可选）</label>
            <input
              type="text"
              value={formData.prod_host || ""}
              onChange={(e) => setFormData({ ...formData, prod_host: e.target.value })}
              placeholder="app.example.com"
            />
            <p className="help-text">生产环境自定义域名；留空则使用默认规则。</p>
          </div>

          <div className="form-group">
            <label>Traefik 入口点（Entrypoints）</label>
            <input
              type="text"
              value={formData.traefik_entrypoints || "websecure"}
              onChange={(e) => setFormData({ ...formData, traefik_entrypoints: e.target.value })}
            />
            <p className="help-text">用逗号分隔（例如："web,websecure"）。</p>
          </div>
        </div>

        {updateMutation.error && (
          <div className="error">{String(updateMutation.error)}</div>
        )}

        <div className="form-actions">
          <button type="button" onClick={() => navigate(-1)} className="btn-secondary">
            取消
          </button>
          <button type="submit" disabled={updateMutation.isPending} className="btn-primary">
            {updateMutation.isPending ? "保存中..." : "保存"}
          </button>
        </div>
      </form>
    </div>
  );
}
