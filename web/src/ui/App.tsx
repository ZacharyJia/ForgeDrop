import React, { useEffect, useState } from "react";

type Me = { id: string; username: string };

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  const txt = await res.text();
  const data = txt ? JSON.parse(txt) : null;
  if (!res.ok) throw new Error(data?.error || `${res.status} ${res.statusText}`);
  return data as T;
}

export function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [err, setErr] = useState<string>("");

  useEffect(() => {
    apiFetch<Me>("/api/v1/admin/me")
      .then(setMe)
      .catch(() => setMe(null));
  }, []);

  return (
    <div className="wrap">
      <header className="top">
        <div className="brand">forge-drop</div>
        <div className="muted">{me ? `@${me.username}` : "not logged in"}</div>
      </header>
      <main className="card">
        <h1>Web UI</h1>
        <p className="muted">
          当前仓库内置了一个可直接运行的最小管理台（见 <code>web/dist</code>）。
          这里是 React+Vite 的 UI 骨架，方便后续扩展成完整后台。
        </p>
        {err ? <div className="err">{err}</div> : null}
        <div className="muted">
          若要用 React UI 覆盖内置 UI：在 <code>web/</code> 目录执行 <code>npm i</code> 然后 <code>npm run build</code>。
        </div>
      </main>
    </div>
  );
}

