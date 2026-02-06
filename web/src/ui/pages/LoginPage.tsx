import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { useToast } from "../toast";

export function LoginPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();
  const setupStatusQuery = useQuery({
    queryKey: ["setupStatus"],
    queryFn: api.getSetupStatus,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const loginMutation = useMutation({
    mutationFn: () => api.login(username, password),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["me"] });
      navigate("/");
    },
    onError: (e) => toast.error(String(e), "登录失败"),
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    loginMutation.mutate();
  };

  return (
    <div className="login-page">
      <div className="login-card">
        <h1>forge-drop</h1>
        <p className="subtitle">自托管部署平台</p>

        {setupStatusQuery.data?.allowed && (
          <div className="info-box login-setup-tip">
            <div className="info-item"><strong>首次使用：</strong>系统尚未初始化</div>
            <Link to="/setup" className="btn-secondary">创建管理员账号</Link>
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label>用户名</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              autoFocus
            />
          </div>
          <div className="form-group">
            <label>密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          {loginMutation.error && (
            <div className="error">{String(loginMutation.error)}</div>
          )}
          <button type="submit" disabled={loginMutation.isPending}>
            {loginMutation.isPending ? "登录中..." : "登录"}
          </button>
        </form>
      </div>
    </div>
  );
}
