import React from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../../api";

export function Dashboard() {
  const { data: apps, isLoading } = useQuery({
    queryKey: ["apps"],
    queryFn: api.listApps,
  });

  const { data: repos } = useQuery({
    queryKey: ["repos"],
    queryFn: api.listRepos,
  });

  const { data: tokens } = useQuery({
    queryKey: ["tokens"],
    queryFn: api.listTokens,
  });

  if (isLoading) {
    return <div className="loading">Loading...</div>;
  }

  return (
    <div className="dashboard">
      <h1>Dashboard</h1>
      
      <div className="stats-grid">
        <div className="stat-card">
          <h3>Applications</h3>
          <div className="stat-value">{apps?.length || 0}</div>
          <Link to="/apps" className="stat-link">Manage →</Link>
        </div>
        
        <div className="stat-card">
          <h3>Repositories</h3>
          <div className="stat-value">{repos?.length || 0}</div>
          <Link to="/repos" className="stat-link">Manage →</Link>
        </div>
        
        <div className="stat-card">
          <h3>API Tokens</h3>
          <div className="stat-value">{tokens?.length || 0}</div>
          <Link to="/tokens" className="stat-link">Manage →</Link>
        </div>
      </div>

      <div className="quick-start">
        <h2>Quick Start</h2>
        <ol>
          <li>Configure <Link to="/settings">global settings</Link> (domain, network)</li>
          <li>Add a <Link to="/repos">repository</Link> and configure webhook</li>
          <li>Create an <Link to="/apps">application</Link> and configure services</li>
          <li>Generate an <Link to="/tokens">API token</Link> for CI uploads</li>
          <li>Upload artifacts from your CI pipeline</li>
        </ol>
      </div>

      {apps && apps.length > 0 && (
        <div className="recent-apps">
          <h2>Recent Applications</h2>
          <div className="app-list">
            {apps.slice(0, 5).map((app) => (
              <Link key={app.id} to={`/apps/${app.id}`} className="app-item">
                <div className="app-name">{app.name}</div>
                <div className="app-key">{app.app_key}</div>
              </Link>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
