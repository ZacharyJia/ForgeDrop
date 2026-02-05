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

  const [configureInfra, setConfigureInfra] = useState(true);
  const [baseDomain, setBaseDomain] = useState("");
  const [dockerNetwork, setDockerNetwork] = useState("traefik");
  const [acmeEmail, setAcmeEmail] = useState("");
  const [aliAccessKey, setAliAccessKey] = useState("");
  const [aliSecretKey, setAliSecretKey] = useState("");
  const [aliRegion, setAliRegion] = useState("cn-hangzhou");
  const [staging, setStaging] = useState(true);
  const [enableDashboard, setEnableDashboard] = useState(false);
  const [dashboardHost, setDashboardHost] = useState("");

  const setupMutation = useMutation({
    mutationFn: async () => {
      if (!username.trim() || !password) throw new Error("请输入用户名与密码");
      if (password.length < 6) throw new Error("密码至少 6 位");
      if (password !== password2) throw new Error("两次输入的密码不一致");

		if (configureInfra) {
			if (!baseDomain.trim()) throw new Error("请填写基础域名（用于预览域名计算）");
			if (!acmeEmail.trim()) throw new Error("请填写 ACME 邮箱");
			if (!aliAccessKey.trim() || !aliSecretKey.trim()) throw new Error("请填写阿里云 DNS 的 AccessKey/SecretKey");
			if (enableDashboard && dashboardHost.trim() && !dashboardHost.includes(".")) {
				throw new Error("Dashboard 域名必须是完整域名（例如 traefik.example.com）");
			}
		}

      await api.setup(username.trim(), password);
      // After creating the first admin, log in immediately for a smooth first-run.
      await api.login(username.trim(), password);

		if (configureInfra) {
			// Persist basic settings + Traefik ACME mode.
			await api.updateSettings({
				base_domain: baseDomain.trim(),
				named_host_template: "{app}-{service}-{env}.{base_domain}",
				docker_network: dockerNetwork.trim() || "traefik",
				traefik_acme_email: acmeEmail.trim(),
				traefik_acme_mode: "dns-alidns",
				traefik_alicloud_region_id: aliRegion.trim() || "cn-hangzhou",
			});
			await api.setTraefikAliyunCredentials(aliAccessKey.trim(), aliSecretKey.trim());
			await api.installTraefik({
				staging,
				enable_dashboard: enableDashboard,
				dashboard_host: dashboardHost.trim() || undefined,
			});
		}
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

			<details className="info-box" open>
				<summary style={{ cursor: "pointer" }}>
					<strong>基础设施初始化（可选）</strong>
					<span className="muted">（内网推荐：DNS-01 阿里云）</span>
				</summary>
				<div style={{ marginTop: "0.75rem" }}>
					<div className="form-group">
						<label>
							<input type="checkbox" checked={configureInfra} onChange={(e) => setConfigureInfra(e.target.checked)} />{" "}
							同时配置并安装 Traefik（DNS Challenge）
						</label>
						<p className="help-text">会启动 Traefik 容器并绑定 80/443 端口；需要域名托管在阿里云 DNS。</p>
					</div>

					{configureInfra && (
						<>
							<div className="form-group">
								<label>基础域名</label>
								<input type="text" value={baseDomain} onChange={(e) => setBaseDomain(e.target.value)} placeholder="example.com" />
								<p className="help-text">用于生成预览域名（preview）等。</p>
							</div>

							<div className="form-group">
								<label>Docker Network</label>
								<input type="text" value={dockerNetwork} onChange={(e) => setDockerNetwork(e.target.value)} placeholder="traefik" />
							</div>

							<div className="form-group">
								<label>ACME 邮箱</label>
								<input type="text" value={acmeEmail} onChange={(e) => setAcmeEmail(e.target.value)} placeholder="admin@example.com" />
							</div>

							<div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" }}>
								<div className="form-group">
									<label>ALICLOUD_ACCESS_KEY</label>
									<input type="text" value={aliAccessKey} onChange={(e) => setAliAccessKey(e.target.value)} placeholder="access key id" />
								</div>
								<div className="form-group">
									<label>ALICLOUD_SECRET_KEY</label>
									<input type="password" value={aliSecretKey} onChange={(e) => setAliSecretKey(e.target.value)} placeholder="access key secret" />
								</div>
							</div>

							<div className="form-group">
								<label>ALICLOUD_REGION_ID</label>
								<input type="text" value={aliRegion} onChange={(e) => setAliRegion(e.target.value)} placeholder="cn-hangzhou" />
							</div>

							<div className="form-group">
								<label>
									<input type="checkbox" checked={staging} onChange={(e) => setStaging(e.target.checked)} />{" "}
									使用 Let's Encrypt Staging
								</label>
								<p className="help-text">推荐先勾选测试，确认无误后可在设置页关闭后再安装一次。</p>
							</div>

							<div className="form-group">
								<label>
									<input type="checkbox" checked={enableDashboard} onChange={(e) => setEnableDashboard(e.target.checked)} />{" "}
									启用 Traefik Dashboard
								</label>
								<p className="help-text">建议仅在内网使用；当前不自动加认证。</p>
							</div>

							{enableDashboard && (
								<div className="form-group">
									<label>Dashboard 域名（可选）</label>
									<input type="text" value={dashboardHost} onChange={(e) => setDashboardHost(e.target.value)} placeholder={baseDomain ? `traefik.${baseDomain}` : "traefik.example.com"} />
									<p className="help-text">为空则默认使用 <code>traefik.&lt;base_domain&gt;</code>。</p>
								</div>
							)}
						</>
					)}
				</div>
			</details>

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
