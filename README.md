# forge-drop

一个自托管的 Go 部署平台：CI 上传制品（JAR/任意文件）即触发部署；支持多仓库、多服务（多容器）与单服务多制品（多挂载文件）；提供 Web 管理台（内置静态资源，无需额外 UI 服务）。

## 快速开始（本地）

1. 启动（默认数据目录 `./data`）：

```bash
go run ./cmd/forge-drop --addr :8080 --data-dir ./data
```

2. 打开管理台：`http://localhost:8080/`
   - 首次启动会进入 **Setup** 页面创建 admin 账号（只允许一次）。
   - 若本机没有 Docker（或不想连接 Docker），可设置：`FORGE_DROP_NO_DOCKER=1`

> 管理台 UI 默认直接使用仓库内提交的 `web/dist`（无需 Node）。后续如果你要把 UI 扩展成完整后台，可在 `web/` 下用 React+Vite 构建并覆盖 `web/dist`。

## 与 Traefik 配合（推荐）

- 你需要自行运行 Traefik（Docker provider），并确保 `forge-drop` 创建的业务容器加入同一个 network（默认 `traefik`）。
- DNS 需要通配解析到 VPS：`*.yourdomain.com -> VPS IP`。

## 管理台配置流程（建议顺序）

1. Settings：设置 `base_domain`、`preview_host_template`、`docker_network`
2. Repos：添加 `owner/repo`，并设置 webhook secret（用于 PR 关闭时自动清理 preview）
3. Apps：创建 App
4. Services：为 App 创建一个或多个 Service（镜像/命令/端口/Prod host）
5. Slots：在 Service 下创建一个或多个 Slot，并绑定 Repo + container path（文件会以只读 bind mount 方式挂载到该路径）
6. Envs：创建命名环境（至少创建 `prod`）
7. Tokens：创建 CI Token，供 Forgejo Actions 上传制品

## CI 上传制品（Forgejo Actions）

接口：`POST /api/v1/artifacts/upload`（multipart/form-data），需要 `Authorization: Bearer <token>`。

典型字段：
- `app` / `service` / `slot`：在管理台配置
- `env`：`prod` / `staging` / `preview`
- `repo`：`owner/name`（必须匹配 slot 绑定的 repo）
- `pr_number`：当 `env=preview` 必填
- 文件字段：`artifact=@xxx.jar`

示例（curl）：

```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -F "app=my-app" \
  -F "env=prod" \
  -F "service=api" \
  -F "slot=main" \
  -F "repo=owner/repo" \
  -F "sha=$GIT_SHA" \
  -F "artifact=@build/app.jar" \
  http://your-server:8080/api/v1/artifacts/upload
```

## Forgejo Webhook（PR 关闭清理 Preview）

- URL：在管理台 Settings 页面可直接复制 `/webhooks/forgejo`
- 事件：`pull_request`
- Secret：填写你在管理台 Repos 中配置的 `webhook_secret`

当收到 `pull_request` 的 `closed` 时，会自动删除对应 repo+PR 的 preview env 相关容器与运行目录。

## 数据目录结构

- SQLite：`<data-dir>/forge-drop.db`
- Artifact Store：`<data-dir>/artifacts/<artifact_id>/<original_filename>`
- 运行目录：`<data-dir>/runtime/env-<env_id>/service-<service_id>/slots/<slot_key>/file`

## 备注

- 本项目默认使用纯 Go 的 SQLite 驱动（`modernc.org/sqlite`），便于打包单二进制。
- Docker 集成通过 Docker Engine API，需要 `forge-drop` 能访问 Docker（host 运行或容器内挂载 `/var/run/docker.sock`）。
