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
      alert("Settings saved successfully!");
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    updateMutation.mutate();
  };

  if (isLoading) {
    return <div className="loading">Loading...</div>;
  }

  return (
    <div className="settings-page">
      <h1>Settings</h1>

      <form onSubmit={handleSubmit} className="settings-form">
        <div className="form-section">
          <h2>Domain Configuration</h2>
          
          <div className="form-group">
            <label>Base Domain</label>
            <input
              type="text"
              value={formData.base_domain || ""}
              onChange={(e) => setFormData({ ...formData, base_domain: e.target.value })}
              placeholder="example.com"
            />
            <p className="help-text">
              Base domain for your deployments (e.g., example.com)
            </p>
          </div>

          <div className="form-group">
            <label>Preview Host Template</label>
            <input
              type="text"
              value={formData.preview_host_template || ""}
              onChange={(e) => setFormData({ ...formData, preview_host_template: e.target.value })}
              placeholder="pr-{app}-{repoSlug}-{pr}-{service}.{base_domain}"
            />
            <p className="help-text">
              Template for preview environment hostnames. Available variables: 
              {"{app}"}, {"{repoSlug}"}, {"{pr}"}, {"{service}"}, {"{base_domain}"}
            </p>
          </div>
        </div>

        <div className="form-section">
          <h2>Docker Configuration</h2>
          
          <div className="form-group">
            <label>Docker Network</label>
            <input
              type="text"
              value={formData.docker_network || ""}
              onChange={(e) => setFormData({ ...formData, docker_network: e.target.value })}
              placeholder="traefik"
            />
            <p className="help-text">
              Docker network name for containers (must exist and be accessible by Traefik)
            </p>
          </div>
        </div>

        <div className="form-section">
          <h2>Integration URLs</h2>
          
          <div className="info-box">
            <div className="info-item">
              <strong>Artifact Upload URL:</strong>
              <code>{settings?.artifact_upload_url}</code>
            </div>
            <p className="help-text">
              Use this URL in your CI pipeline to upload artifacts
            </p>
          </div>

          <div className="info-box">
            <div className="info-item">
              <strong>Forgejo Webhook URL:</strong>
              <code>{settings?.forgejo_webhook_url}</code>
            </div>
            <p className="help-text">
              Configure this URL as a webhook in your Forgejo repository settings
            </p>
          </div>
        </div>

        {updateMutation.error && (
          <div className="error">{String(updateMutation.error)}</div>
        )}

        <div className="form-actions">
          <button type="submit" disabled={updateMutation.isPending} className="btn-primary">
            {updateMutation.isPending ? "Saving..." : "Save Settings"}
          </button>
        </div>
      </form>

      <div className="section">
        <h2>Setup Guide</h2>
        <div className="info-box">
          <ol>
            <li>Configure the base domain and Docker network above</li>
            <li>Set up wildcard DNS for preview environments (*.example.com → your server)</li>
            <li>Configure Traefik with Docker provider and the same network</li>
            <li>Add repositories and configure webhooks</li>
            <li>Create applications and services</li>
            <li>Generate API tokens for CI integration</li>
          </ol>
        </div>
      </div>
    </div>
  );
}
