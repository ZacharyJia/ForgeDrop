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
    return <div className="loading">加载中...</div>;
  }

  return (
    <div className="dashboard">
      <div className="page-header">
        <div>
          <h1>概览</h1>
          <p className="section-desc">集中查看部署规模、配置入口与最近应用状态。</p>
        </div>
        <Link to="/apps" className="btn-primary">进入应用管理</Link>
      </div>
      
      <div className="stats-grid">
        <div className="stat-card">
          <h3>应用</h3>
          <div className="stat-value">{apps?.length || 0}</div>
          <Link to="/apps" className="stat-link">管理 →</Link>
        </div>
        
        <div className="stat-card">
          <h3>仓库</h3>
          <div className="stat-value">{repos?.length || 0}</div>
          <Link to="/repos" className="stat-link">管理 →</Link>
        </div>
        
        <div className="stat-card">
          <h3>API 令牌</h3>
          <div className="stat-value">{tokens?.length || 0}</div>
          <Link to="/tokens" className="stat-link">管理 →</Link>
        </div>
      </div>

      <div className="quick-start">
        <h2>快速开始</h2>
        <ol>
          <li>配置 <Link to="/settings">全局设置</Link>（域名、网络等）</li>
          <li>添加 <Link to="/repos">仓库</Link> 并配置 Webhook</li>
          <li>创建 <Link to="/apps">应用</Link> 并配置服务</li>
          <li>生成 <Link to="/tokens">API 令牌</Link> 供 CI 上传制品</li>
          <li>从 CI 流水线上传制品（artifact）</li>
        </ol>
        <div className="dashboard-cta-row">
          <Link to="/docs" className="btn-secondary">查看详细文档</Link>
        </div>
      </div>

      {apps && apps.length > 0 && (
        <div className="recent-apps">
          <h2>最近的应用</h2>
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
