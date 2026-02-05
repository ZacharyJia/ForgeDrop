import React, { useMemo, useState } from "react";
import { BrowserRouter, Routes, Route, NavLink, Navigate, useLocation, useNavigate } from "react-router-dom";
import { QueryClient, QueryClientProvider, useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";

import { ToastProvider, useToast } from "./toast";

// Pages
import { Dashboard } from "./pages/Dashboard";
import { AppsPage } from "./pages/AppsPage";
import { AppDetailPage } from "./pages/AppDetailPage";
import { ServiceEditPage } from "./pages/ServiceEditPage";
import { ServiceDetailPage } from "./pages/ServiceDetailPage";
import { EnvDetailPage } from "./pages/EnvDetailPage";
import { ReposPage } from "./pages/ReposPage";
import { TokensPage } from "./pages/TokensPage";
import { SettingsPage } from "./pages/SettingsPage";
import { LoginPage } from "./pages/LoginPage";
import { DocsPage } from "./pages/DocsPage";
import { SetupPage } from "./pages/SetupPage";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: false,
    },
  },
});

function Layout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const loc = useLocation();
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: api.getMe,
  });

  const title = useMemo(() => {
    const path = loc.pathname;
    if (path === "/") return "概览";
    if (path.startsWith("/apps")) return "应用";
    if (path.startsWith("/repos")) return "仓库";
    if (path.startsWith("/tokens")) return "API 令牌";
    if (path.startsWith("/settings")) return "设置";
    if (path.startsWith("/docs")) return "文档";
    if (path.startsWith("/services")) return "服务";
    return "forge-drop";
  }, [loc.pathname]);

  const logoutMutation = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.clear();
      navigate("/login");
    },
    onError: (e) => {
      toast.error(String(e), "退出失败");
    },
  });

  if (meQuery.isPending) {
    return <div className="loading">加载中...</div>;
  }

  if (meQuery.isError || !meQuery.data) {
    return <Navigate to="/login" replace />;
  }

  const user = meQuery.data;

  return (
    <div className="layout">
      {mobileNavOpen && <div className="backdrop" onClick={() => setMobileNavOpen(false)} />}

      <nav className={`sidebar ${mobileNavOpen ? "sidebar-open" : ""}`}>
        <div className="brand">
          <h1>forge-drop</h1>
          <div className="user-info">@{user.username}</div>
        </div>
        <ul className="nav-menu">
          <li>
            <NavLink
              to="/"
              end
              className={({ isActive }) => (isActive ? "active" : undefined)}
              onClick={() => setMobileNavOpen(false)}
            >
              概览
            </NavLink>
          </li>
          <li>
            <NavLink
              to="/apps"
              className={({ isActive }) => (isActive ? "active" : undefined)}
              onClick={() => setMobileNavOpen(false)}
            >
              应用
            </NavLink>
          </li>
          <li>
            <NavLink
              to="/repos"
              className={({ isActive }) => (isActive ? "active" : undefined)}
              onClick={() => setMobileNavOpen(false)}
            >
              仓库
            </NavLink>
          </li>
          <li>
            <NavLink
              to="/tokens"
              className={({ isActive }) => (isActive ? "active" : undefined)}
              onClick={() => setMobileNavOpen(false)}
            >
              API 令牌
            </NavLink>
          </li>
          <li>
            <NavLink
              to="/settings"
              className={({ isActive }) => (isActive ? "active" : undefined)}
              onClick={() => setMobileNavOpen(false)}
            >
              设置
            </NavLink>
          </li>
          <li>
            <NavLink
              to="/docs"
              className={({ isActive }) => (isActive ? "active" : undefined)}
              onClick={() => setMobileNavOpen(false)}
            >
              文档
            </NavLink>
          </li>
        </ul>
        <button className="logout-btn" onClick={() => logoutMutation.mutate()} disabled={logoutMutation.isPending}>
          退出登录
        </button>
      </nav>
      <main className="content">
        <header className="topbar">
          <button className="nav-toggle" onClick={() => setMobileNavOpen(true)} aria-label="Menu">
            ☰
          </button>
          <div className="topbar-title">{title}</div>
          <div className="topbar-spacer" />
        </header>
        {children}
      </main>
    </div>
  );
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/setup" element={<SetupPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<Layout><Dashboard /></Layout>} />
      <Route path="/apps" element={<Layout><AppsPage /></Layout>} />
      <Route path="/apps/:appId" element={<Layout><AppDetailPage /></Layout>} />
      <Route path="/envs/:envId" element={<Layout><EnvDetailPage /></Layout>} />
      <Route path="/services/:serviceId" element={<Layout><ServiceDetailPage /></Layout>} />
      <Route path="/services/:serviceId/edit" element={<Layout><ServiceEditPage /></Layout>} />
      <Route path="/repos" element={<Layout><ReposPage /></Layout>} />
      <Route path="/tokens" element={<Layout><TokensPage /></Layout>} />
      <Route path="/settings" element={<Layout><SettingsPage /></Layout>} />
      <Route path="/docs" element={<Layout><DocsPage /></Layout>} />
    </Routes>
  );
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <BrowserRouter>
          <AppRoutes />
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}
