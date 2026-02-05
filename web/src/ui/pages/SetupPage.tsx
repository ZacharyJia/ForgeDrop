import React, { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { useToast } from "../toast";

export function SetupPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const statusQuery = useQuery({
    queryKey: ["setupStatus"],
    queryFn: api.getSetupStatus,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [password2, setPassword2] = useState("");

  const setupMutation = useMutation({
    mutationFn: async () => {
      if (!username.trim() || !password) throw new Error("请输入用户名与密码");
      if (password.length < 6) throw new Error("密码至少 6 位");
      if (password !== password2) throw new Error("两次输入的密码不一致");

      await api.setup(username.trim(), password);
      // After creating the first admin, log in immediately for a smooth first-run.
      await api.login(username.trim(), password);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["me"] });
      toast.success("初始化完成，已登录");
      navigate("/");
    },
    onError: (e) => toast.error(String(e), "初始化失败"),
  });

  useEffect(() => {
    if (statusQuery.data && !statusQuery.data.allowed) {
      // Already initialized; keep UX obvious.
      toast.info("系统已初始化，请直接登录");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusQuery.data?.allowed]);

  return (
    <div className="login-page">
      <div className="login-card">
        <h1>forge-drop</h1>
        <p className="subtitle">首次初始化（创建管理员账号）</p>

        {statusQuery.isPending ? (
          <div className="loading">检查初始化状态...</div>
        ) : statusQuery.isError ? (
          <div className="error">{String(statusQuery.error)}</div>
        ) : statusQuery.data && !statusQuery.data.allowed ? (
          <div>
            <div className="info-box">
              <div className="info-item"><strong>状态：</strong>已初始化（user_count={statusQuery.data.user_count}）</div>
              <div className="muted">如忘记密码，请删除数据目录重新初始化（仅适用于测试环境）。</div>
            </div>
            <Link className="btn-primary" to="/login">去登录</Link>
          </div>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              setupMutation.mutate();
            }}
          >
            <div className="form-group">
              <label>管理员用户名</label>
              <input type="text" value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus />
            </div>

            <div className="form-group">
              <label>密码</label>
              <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </div>

            <div className="form-group">
              <label>确认密码</label>
              <input type="password" value={password2} onChange={(e) => setPassword2(e.target.value)} required />
            </div>

            {setupMutation.error && <div className="error">{String(setupMutation.error)}</div>}
            <button type="submit" className="btn-primary" disabled={setupMutation.isPending}>
              {setupMutation.isPending ? "初始化中..." : "创建并登录"}
            </button>

            <div style={{ marginTop: "1rem" }}>
              <Link to="/login" className="btn-link">返回登录</Link>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
