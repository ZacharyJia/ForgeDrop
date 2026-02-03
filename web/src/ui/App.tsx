import React, { useState } from "react";
import { BrowserRouter, Routes, Route, Link, Navigate, useNavigate } from "react-router-dom";
import { QueryClient, QueryClientProvider, useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type User } from "../api";

// Pages
import { Dashboard } from "./pages/Dashboard";
import { AppsPage } from "./pages/AppsPage";
import { AppDetailPage } from "./pages/AppDetailPage";
import { ServiceEditPage } from "./pages/ServiceEditPage";
import { ReposPage } from "./pages/ReposPage";
import { TokensPage } from "./pages/TokensPage";
import { SettingsPage } from "./pages/SettingsPage";
import { LoginPage } from "./pages/LoginPage";

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
  const { data: user } = useQuery({
    queryKey: ["me"],
    queryFn: api.getMe,
  });

  const logoutMutation = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.clear();
      navigate("/login");
    },
  });

  if (!user) {
    return <Navigate to="/login" />;
  }

  return (
    <div className="layout">
      <nav className="sidebar">
        <div className="brand">
          <h1>forge-drop</h1>
          <div className="user-info">@{user.username}</div>
        </div>
        <ul className="nav-menu">
          <li><Link to="/">Dashboard</Link></li>
          <li><Link to="/apps">Applications</Link></li>
          <li><Link to="/repos">Repositories</Link></li>
          <li><Link to="/tokens">API Tokens</Link></li>
          <li><Link to="/settings">Settings</Link></li>
        </ul>
        <button className="logout-btn" onClick={() => logoutMutation.mutate()}>
          Logout
        </button>
      </nav>
      <main className="content">
        {children}
      </main>
    </div>
  );
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<Layout><Dashboard /></Layout>} />
      <Route path="/apps" element={<Layout><AppsPage /></Layout>} />
      <Route path="/apps/:appId" element={<Layout><AppDetailPage /></Layout>} />
      <Route path="/services/:serviceId/edit" element={<Layout><ServiceEditPage /></Layout>} />
      <Route path="/repos" element={<Layout><ReposPage /></Layout>} />
      <Route path="/tokens" element={<Layout><TokensPage /></Layout>} />
      <Route path="/settings" element={<Layout><SettingsPage /></Layout>} />
    </Routes>
  );
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AppRoutes />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
