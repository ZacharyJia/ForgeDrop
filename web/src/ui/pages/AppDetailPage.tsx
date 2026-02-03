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
    return <div className="loading">Loading...</div>;
  }

  if (!data) {
    return <div className="error">Application not found</div>;
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
          ← Back to Apps
        </Link>
      </div>

      <div className="section">
        <div className="section-header">
          <h2>Services</h2>
          <p className="section-desc">
            Configure services (containers) for this application. Each service can use either
            Docker Compose templates or manual Docker API configuration.
          </p>
        </div>

        {services.length === 0 ? (
          <div className="empty-state">
            <p>No services configured yet.</p>
            <p className="muted">
              Services are created automatically when you upload artifacts via the API.
              Or you can create them manually via the API.
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
                      <span className="badge badge-enabled">Enabled</span>
                    ) : (
                      <span className="badge badge-disabled">Disabled</span>
                    )}
                  </div>
                </div>

                <div className="service-body">
                  {service.use_compose ? (
                    <div className="service-info">
                      <div className="info-item">
                        <strong>Mode:</strong> Docker Compose
                      </div>
                      <div className="info-item">
                        <strong>Template:</strong> {service.compose_template ? "Configured" : "Not set"}
                      </div>
                    </div>
                  ) : (
                    <div className="service-info">
                      <div className="info-item">
                        <strong>Image:</strong> {service.image}
                      </div>
                      <div className="info-item">
                        <strong>Port:</strong> {service.container_port}
                      </div>
                      <div className="info-item">
                        <strong>Command:</strong> <code>{service.command}</code>
                      </div>
                    </div>
                  )}

                  {service.prod_host && (
                    <div className="info-item">
                      <strong>Production Host:</strong> {service.prod_host}
                    </div>
                  )}

                  <div className="service-meta">
                    Revision: {service.revision} | Updated: {new Date(service.updated_at).toLocaleString()}
                  </div>
                </div>

                <div className="service-actions">
                  <Link to={`/services/${service.id}/edit`} className="btn-secondary">
                    Edit Configuration
                  </Link>
                  <button
                    className="btn-danger-small"
                    onClick={() => {
                      if (confirm(`Delete service "${service.name}"?`)) {
                        deleteMutation.mutate(service.id);
                      }
                    }}
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="section">
        <h2>Deployment Info</h2>
        <div className="info-box">
          <p>To deploy this application:</p>
          <ol>
            <li>Configure services above</li>
            <li>Create an API token in the <Link to="/tokens">Tokens</Link> page</li>
            <li>Upload artifacts from your CI pipeline using the API</li>
          </ol>
          <p className="muted">
            See the <a href="https://github.com/your-repo/forge-drop" target="_blank">documentation</a> for CI integration examples.
          </p>
        </div>
      </div>
    </div>
  );
}
