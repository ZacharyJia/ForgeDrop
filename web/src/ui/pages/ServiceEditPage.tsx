import React, { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useNavigate } from "react-router-dom";
import { api, type Service } from "../../api";
import CodeMirror from "@uiw/react-codemirror";
import { yaml } from "@codemirror/lang-yaml";
import { useToast } from "../toast";

export function ServiceEditPage() {
  const { serviceId } = useParams<{ serviceId: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();

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
      // Compose-only mode
      setFormData({ ...data.service, use_compose: true });
    }
  }, [data]);

  const updateMutation = useMutation({
    mutationFn: () =>
      api.updateService(serviceId!, {
        name: formData.name,
        enabled: formData.enabled,
        compose_template: formData.compose_template,
        deploy_strategy: formData.deploy_strategy,
        prod_host: formData.prod_host,
        traefik_entrypoints: formData.traefik_entrypoints,
        container_port: formData.container_port,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["service", serviceId] });
      toast.success("服务已更新");
    },
    onError: (e) => {
      toast.error(String(e), "更新失败");
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

            <CodeMirror
              className="compose-editor"
              value={formData.compose_template || ""}
              height="520px"
              extensions={[yaml()]}
              onChange={(value) => setFormData({ ...formData, compose_template: value })}
              basicSetup={{
                lineNumbers: true,
                foldGutter: true,
                highlightActiveLine: true,
              }}
            />
            <p className="help-text">
              支持 YAML + Go template。你可以使用变量 {`{{.Artifacts}}`}、{`{{.SlotPaths}}`}、{`{{.Host}}`}、{`{{.EnvName}}`} 等。
            </p>
          </div>
        </div>

        <div className="form-section">
          <h2>路由配置</h2>

		  <div className="form-group">
			<label>默认部署策略</label>
			<select
			  value={formData.deploy_strategy || "recreate"}
			  onChange={(e) => setFormData({ ...formData, deploy_strategy: e.target.value as any })}
			>
			  <option value="recreate">重建部署（down + up，默认）</option>
			  <option value="restart">快速重启（restart，更快）</option>
			</select>
			<p className="help-text">用于该服务在自动部署与未显式传 strategy 时的默认行为。</p>
		  </div>

          <div className="form-group">
            <label>服务端口（供模板变量 .Port 使用）</label>
            <input
              type="number"
              min={1}
              value={formData.container_port || 8080}
              onChange={(e) => {
                const nextPort = Number.parseInt(e.target.value, 10);
                setFormData({ ...formData, container_port: Number.isNaN(nextPort) ? 8080 : nextPort });
              }}
            />
            <p className="help-text">一般是容器内部服务监听端口（例如 8080）。</p>
          </div>
          
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
            <p className="help-text">用逗号分隔（例如："web,websecure"）。该值会作为模板变量 .EntryPoints。</p>
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
