import React from "react";
import { Link } from "react-router-dom";

export function DocsPage() {
  return (
    <div className="docs-page">
      <div className="page-header">
        <div>
          <h1>使用文档</h1>
          <div className="muted">从 0 到 1 配置 forge-drop，并把 CI 制品部署跑通</div>
        </div>
        <a className="btn-secondary" href="#troubleshooting">故障排查</a>
      </div>

      <div className="docs-grid">
        <aside className="doc-toc">
          <div className="doc-toc-title">目录</div>
          <a href="#concepts">核心概念</a>
          <a href="#prereq">部署前置条件</a>
          <a href="#setup">管理台配置流程</a>
          <a href="#compose">Compose 模板</a>
          <a href="#ci">CI 上传制品</a>
          <a href="#manual">手动上传/手动部署</a>
          <a href="#preview">Preview 环境</a>
          <a href="#status">状态与日志</a>
          <a href="#troubleshooting">常见问题</a>
        </aside>

        <div className="doc-content">
          <section className="doc-section" id="concepts">
            <h2>核心概念</h2>
            <div className="info-box">
              <p>
                forge-drop 的核心是：<strong>CI 只负责构建制品</strong>，forge-drop 负责接收制品、管理版本（快照）并驱动容器更新。
              </p>
              <ul className="doc-list">
                <li><strong>App</strong>：一个应用（例如一个后端系统）。</li>
                <li><strong>Service</strong>：一个服务（容器/Compose 项目内的一组容器配置）。</li>
                <li><strong>Slot</strong>：挂载点。每个 slot 绑定一个 repo，并指定容器内路径（例如 <code>/app/app.jar</code>）。</li>
                <li><strong>Env</strong>：环境。命名环境如 <code>prod</code>/<code>staging</code>；预览环境为 <code>preview</code>（按 PR）。</li>
                <li><strong>Artifact</strong>：一次上传的文件（JAR 或任意文件）。</li>
                <li><strong>Snapshot</strong>：一次“版本指针”。Env 有一个 <code>current_snapshot</code>，表示“期望运行的版本（desired）”。</li>
              </ul>
              <p className="muted">
                重要：你可以上传后不自动部署（只更新 desired），然后在合适的时间点手动点击部署。
              </p>
            </div>
          </section>

          <section className="doc-section" id="prereq">
            <h2>部署前置条件</h2>
            <div className="info-box">
              <ol>
                <li>服务器上有 Docker，并可运行 <code>docker compose</code></li>
                <li>你有一个反向代理（推荐 Traefik，Docker provider）</li>
                <li>DNS 通配解析：<code>*.yourdomain.com</code> 指向服务器 IP（用于 preview 子域名）</li>
                <li>Traefik 与 forge-drop 创建的业务容器加入同一 Docker network（例如 <code>traefik</code>）</li>
              </ol>
            </div>
          </section>

          <section className="doc-section" id="setup">
            <h2>管理台配置流程（建议顺序）</h2>
            <div className="info-box">
              <ol>
                <li>
                  <strong>设置</strong>：进入 <Link to="/settings">设置</Link>，配置 <code>base_domain</code>、<code>preview_host_template</code>、<code>docker_network</code>
                </li>
                <li>
                  <strong>仓库</strong>：进入 <Link to="/repos">仓库</Link>，添加 <code>owner/repo</code> 并生成 webhook secret（用于 PR 关闭自动清理 preview）
                </li>
                <li>
                  <strong>应用</strong>：进入 <Link to="/apps">应用</Link> 创建 App
                </li>
                <li>
                  <strong>服务</strong>：在 App 详情里创建 Service，然后进入“编辑配置”配置 Compose 模板
                </li>
                <li>
                  <strong>槽位</strong>：进入服务详情页创建 Slot（每个 slot 绑定 repo + 容器内挂载路径）
                </li>
                <li>
                  <strong>环境</strong>：在 App 详情里创建命名环境（至少创建 <code>prod</code>）
                </li>
                <li>
                  <strong>API 令牌</strong>：进入 <Link to="/tokens">API 令牌</Link> 创建 token，给 CI 上传制品使用
                </li>
              </ol>
            </div>
          </section>

          <section className="doc-section" id="compose">
            <h2>Compose 模板（关键）</h2>
            <div className="info-box">
              <p>在服务编辑页里可以查看模板示例。模板是 YAML + Go template。</p>
              <p className="muted">常用变量：</p>
              <ul className="doc-list">
                <li><code>{"{{.Artifacts}}"}</code>：slot_key -&gt; 主机文件路径</li>
                <li><code>{"{{.SlotPaths}}"}</code>：slot_key -&gt; 容器内挂载路径</li>
                <li><code>{"{{.Host}}"}</code>：该 env 的访问域名（preview/prod）</li>
                <li><code>{"{{.EnvName}}"}</code>：环境名（prod/staging/preview）</li>
                <li><code>{"{{.Network}}"}</code>：Docker network</li>
              </ul>
            </div>
          </section>

          <section className="doc-section" id="ci">
            <h2>CI 上传制品（推荐）</h2>
            <div className="info-box">
              <p>接口：<code>POST /api/v1/artifacts/upload</code>（multipart/form-data，Bearer Token）。</p>
              <p className="muted">字段说明：</p>
              <ul className="doc-list">
                <li><code>app</code>/<code>service</code>/<code>slot</code>：管理台配置</li>
                <li><code>env</code>：<code>prod</code>/<code>staging</code>/<code>preview</code></li>
                <li><code>repo</code>：<code>owner/repo</code>（必须匹配 slot 绑定的 repo）</li>
                <li><code>pr_number</code>：当 <code>env=preview</code> 必填</li>
                <li><code>deploy</code>：可选，<code>1</code>（默认）自动部署；<code>0</code> 仅更新版本，等待手动部署</li>
                <li>文件字段：<code>artifact=@xxx.jar</code></li>
              </ul>
              <pre>
<code>{`curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -F "app=my-app" \
  -F "env=prod" \
  -F "service=api" \
  -F "slot=main" \
  -F "repo=owner/repo" \
  -F "sha=$GIT_SHA" \
  -F "deploy=0" \
  -F "artifact=@build/app.jar" \
  http://your-server:8080/api/v1/artifacts/upload`}</code>
              </pre>
            </div>
          </section>

          <section className="doc-section" id="manual">
            <h2>手动上传 / 手动部署（用于验证全流程）</h2>
            <div className="info-box">
              <ol>
                <li>进入某个服务详情页</li>
                <li>先创建 Slot（挂载路径）</li>
                <li>选择命名环境（prod/staging）</li>
                <li>选择需要更新的槽位文件</li>
                <li>按需关闭“上传后自动部署”，即可进入“待部署”状态</li>
                <li>点击“部署当前版本”触发更新</li>
              </ol>
            </div>
          </section>

          <section className="doc-section" id="preview">
            <h2>Preview 环境（PR 预览）</h2>
            <div className="info-box">
              <p>
                当 CI 上传时使用 <code>env=preview</code> 并携带 <code>pr_number</code>，系统会为该 PR 创建/更新独立的预览环境。
              </p>
              <p>
                当 Forgejo webhook 收到 <code>pull_request closed</code> 后，会自动清理对应 preview 的 runtime 目录与 compose project。
              </p>
              <p className="muted">提示：你仍然需要 DNS 通配解析与 Traefik 路由规则才能访问 preview URL。</p>
            </div>
          </section>

          <section className="doc-section" id="status">
            <h2>状态与日志</h2>
            <div className="info-box">
              <p>在服务详情页选择目标环境后，可以看到：</p>
              <ul className="doc-list">
                <li><strong>当前快照（desired）</strong>：Env 的 current_snapshot</li>
                <li><strong>docker compose ps</strong>：容器运行状态（best-effort）</li>
                <li><strong>查看日志</strong>：拉取 compose logs（tail 可调）</li>
              </ul>
            </div>
          </section>

          <section className="doc-section" id="troubleshooting">
            <h2>常见问题</h2>
            <div className="info-box">
              <ul className="doc-list">
                <li><strong>部署失败（docker compose up）</strong>：先在服务详情页查看日志与 ps 输出；其次到服务器执行 <code>docker compose -f &lt;composeFile&gt; -p &lt;project&gt; logs</code></li>
                <li><strong>compose file not found</strong>：说明这个 env+service 还没部署过（或者 runtime 被清理）；点击一次“部署当前版本”即可生成</li>
                <li><strong>访问不到域名</strong>：检查 DNS 是否正确、Traefik 是否同 network、Service 模板里是否配置了路由 label</li>
                <li><strong>repo not allowed for this slot</strong>：slot 绑定的 repo 与 CI 传入 repo 不一致</li>
              </ul>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
