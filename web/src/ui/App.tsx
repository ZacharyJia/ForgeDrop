import React, { useEffect, useMemo, useState } from "react";
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

type ThemeMode = "light" | "dark";

const THEME_STORAGE_KEY = "forge-drop-theme";

function getInitialTheme(): ThemeMode {
  if (typeof window === "undefined") return "light";
  const attrTheme = document.documentElement.getAttribute("data-theme");
  if (attrTheme === "dark" || attrTheme === "light") {
    return attrTheme;
  }
  const saved = window.localStorage.getItem(THEME_STORAGE_KEY);
  return saved === "dark" ? "dark" : "light";
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: false,
    },
  },
});

function ThemeSwitch({
  theme,
  onToggleTheme,
  className,
}: {
  theme: ThemeMode;
  onToggleTheme: () => void;
  className?: string;
}) {
  const nextLabel = theme === "dark" ? "亮色" : "暗色";

  const icon = theme === "dark" ? (
    <svg viewBox="0 0 24 24" aria-hidden="true" className="theme-switch-thumb-icon theme-switch-thumb-icon-moon">
      <path d="M21 12.79A9 9 0 1 1 11.21 3a7 7 0 0 0 9.79 9.79Z" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" aria-hidden="true" className="theme-switch-thumb-icon theme-switch-thumb-icon-sun">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
    </svg>
  );

  return (
    <button
      className={["theme-switch", theme === "dark" ? "is-dark" : "is-light", className || ""].join(" ").trim()}
      type="button"
      onClick={onToggleTheme}
      aria-label={`切换到${nextLabel}模式`}
      title={`切换到${nextLabel}模式`}
      aria-pressed={theme === "dark"}
    >
      <span className="theme-switch-track" aria-hidden="true">
        <span className="theme-switch-thumb">{icon}</span>
      </span>
    </button>
  );
}

function Layout({
  children,
  theme,
  onToggleTheme,
}: {
  children: React.ReactNode;
  theme: ThemeMode;
  onToggleTheme: () => void;
}) {
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
    if (path.startsWith("/tokens")) return "访问令牌";
    if (path.startsWith("/settings")) return "设置";
    if (path.startsWith("/docs")) return "文档";
    if (path.startsWith("/services")) return "服务";
    return "forge-drop";
  }, [loc.pathname]);

  useEffect(() => {
    setMobileNavOpen(false);
  }, [loc.pathname]);

  useEffect(() => {
    if (!mobileNavOpen) return;
    const onKeyDown = (ev: KeyboardEvent) => {
      if (ev.key === "Escape") {
        setMobileNavOpen(false);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [mobileNavOpen]);

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
          <div className="brand-top">
            <h1>forge-drop</h1>
            <ThemeSwitch theme={theme} onToggleTheme={onToggleTheme} className="brand-theme-switch" />
          </div>
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
              访问令牌
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
        <div className="sidebar-actions">
          <button className="logout-btn" onClick={() => logoutMutation.mutate()} disabled={logoutMutation.isPending}>
            退出登录
          </button>
        </div>
      </nav>
      <main className="content">
        <header className="topbar">
          <button className="nav-toggle" onClick={() => setMobileNavOpen(true)} aria-label="Menu">
            ☰
          </button>
          <div className="topbar-title-wrap">
            <div className="topbar-title">{title}</div>
            <div className="topbar-user">@{user.username}</div>
          </div>
          <div className="topbar-spacer" />
          <ThemeSwitch theme={theme} onToggleTheme={onToggleTheme} className="topbar-theme-switch" />
        </header>
        {children}
      </main>
    </div>
  );
}

function AppRoutes({ theme, onToggleTheme }: { theme: ThemeMode; onToggleTheme: () => void }) {
  return (
    <Routes>
      <Route path="/setup" element={<SetupPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><Dashboard /></Layout>} />
      <Route path="/apps" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><AppsPage /></Layout>} />
      <Route path="/apps/:appId" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><AppDetailPage /></Layout>} />
      <Route path="/envs/:envId" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><EnvDetailPage /></Layout>} />
      <Route path="/services/:serviceId" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><ServiceDetailPage /></Layout>} />
      <Route path="/services/:serviceId/edit" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><ServiceEditPage /></Layout>} />
      <Route path="/repos" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><ReposPage /></Layout>} />
      <Route path="/tokens" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><TokensPage /></Layout>} />
      <Route path="/settings" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><SettingsPage /></Layout>} />
      <Route path="/docs" element={<Layout theme={theme} onToggleTheme={onToggleTheme}><DocsPage /></Layout>} />
    </Routes>
  );
}

export function App() {
  const [theme, setTheme] = useState<ThemeMode>(getInitialTheme);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  }, [theme]);

  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <BrowserRouter>
          <AppRoutes
            theme={theme}
            onToggleTheme={() => setTheme((prev) => (prev === "dark" ? "light" : "dark"))}
          />
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}
