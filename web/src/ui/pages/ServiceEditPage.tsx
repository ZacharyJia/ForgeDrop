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
    queryKey: ["compose-template-example"],
    queryFn: api.getComposeTemplateExample,
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
    return <div className="loading">Loading...</div>;
  }

  if (!data) {
    return <div className="error">Service not found</div>;
  }

  const { service } = data;

  return (
    <div className="service-edit-page">
      <div className="page-header">
        <h1>Edit Service: {service.name}</h1>
        <button onClick={() => navigate(-1)} className="btn-secondary">
          ← Back
        </button>
      </div>

      <form onSubmit={handleSubmit} className="service-form">
        <div className="form-section">
          <h2>Basic Information</h2>
          
          <div className="form-group">
            <label>Service Name</label>
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
              {" "}Enabled
            </label>
          </div>
        </div>

        <div className="form-section">
          <h2>Deployment Mode</h2>
          
          <div className="form-group">
            <label className="radio-label">
              <input
                type="radio"
                checked={!formData.use_compose}
                onChange={() => setFormData({ ...formData, use_compose: false })}
              />
              {" "}Docker API (Manual Configuration)
            </label>
            <p className="help-text">
              Configure container settings manually. Suitable for simple single-container services.
            </p>
          </div>

          <div className="form-group">
            <label className="radio-label">
              <input
                type="radio"
                checked={formData.use_compose ?? false}
                onChange={() => setFormData({ ...formData, use_compose: true })}
              />
              {" "}Docker Compose (Template-based)
            </label>
            <p className="help-text">
              Use Docker Compose templates for full control. Supports multi-container services,
              resource limits, health checks, and all Compose features.
            </p>
          </div>
        </div>

        {!formData.use_compose ? (
          <div className="form-section">
            <h2>Docker API Configuration</h2>
            
            <div className="form-group">
              <label>Docker Image</label>
              <input
                type="text"
                value={formData.image || ""}
                onChange={(e) => setFormData({ ...formData, image: e.target.value })}
                placeholder="eclipse-temurin:17-jre"
              />
            </div>

            <div className="form-group">
              <label>Command</label>
              <input
                type="text"
                value={formData.command || ""}
                onChange={(e) => setFormData({ ...formData, command: e.target.value })}
                placeholder="java -jar /app/app.jar"
              />
            </div>

            <div className="form-group">
              <label>Container Port</label>
              <input
                type="number"
                value={formData.container_port || 8080}
                onChange={(e) => setFormData({ ...formData, container_port: parseInt(e.target.value) })}
              />
            </div>

            <div className="form-group">
              <label>Run User (UID:GID)</label>
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
            <h2>Docker Compose Template</h2>
            
            <div className="form-group">
              <div className="label-with-action">
                <label>Compose Template (Go template syntax)</label>
                <button
                  type="button"
                  className="btn-link"
                  onClick={() => setShowExample(!showExample)}
                >
                  {showExample ? "Hide" : "Show"} Example
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
                Use Go template syntax with variables like {`{{.Artifacts}}`}, {`{{.Host}}`}, {`{{.EnvName}}`}, etc.
                See example above for all available variables.
              </p>
            </div>
          </div>
        )}

        <div className="form-section">
          <h2>Routing Configuration</h2>
          
          <div className="form-group">
            <label>Production Host (optional)</label>
            <input
              type="text"
              value={formData.prod_host || ""}
              onChange={(e) => setFormData({ ...formData, prod_host: e.target.value })}
              placeholder="app.example.com"
            />
            <p className="help-text">
              Custom domain for production environment. Leave empty to use default.
            </p>
          </div>

          <div className="form-group">
            <label>Traefik Entrypoints</label>
            <input
              type="text"
              value={formData.traefik_entrypoints || "websecure"}
              onChange={(e) => setFormData({ ...formData, traefik_entrypoints: e.target.value })}
            />
            <p className="help-text">
              Comma-separated list of Traefik entrypoints (e.g., "web,websecure")
            </p>
          </div>
        </div>

        {updateMutation.error && (
          <div className="error">{String(updateMutation.error)}</div>
        )}

        <div className="form-actions">
          <button type="button" onClick={() => navigate(-1)} className="btn-secondary">
            Cancel
          </button>
          <button type="submit" disabled={updateMutation.isPending} className="btn-primary">
            {updateMutation.isPending ? "Saving..." : "Save Changes"}
          </button>
        </div>
      </form>
    </div>
  );
}
