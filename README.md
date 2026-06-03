# forge-drop

一个自托管的 Go 部署平台：CI 上传制品（JAR/任意文件）即触发部署；支持多仓库、多服务（多容器）与单服务多制品（多挂载文件）；提供 Web 管理台（内置静态资源，无需额外 UI 服务）。

## 快速开始（本地）

1. （推荐）构建并运行（包含内置 Web UI）：

```bash
scripts/build.sh --install
./bin/forge-drop --addr :8080 --data-dir ./data
```

2. 或者直接运行（如果你尚未构建 `web/dist`，会看到“Web UI is not built”的提示页）：

```bash
go run ./cmd/forge-drop --addr :8080 --data-dir ./data
```

3. 打开管理台：`http://localhost:8080/`
    - 首次启动会进入 **Setup** 页面创建 admin 账号（只允许一次）。
    - 若本机没有 Docker（或不想连接 Docker），可设置：`FORGE_DROP_NO_DOCKER=1`

> 管理台内置了使用文档页面：登录后访问 `http://localhost:8080/docs`。

> 管理台 UI 由 `web/` 构建产出到 `web/dist`，并在 Go 二进制中 embed（无需额外 UI 服务）。建议使用 `scripts/build.sh --install` 一次性构建前后端。

## AI Skill 与声明式配置

仓库根目录内置了可直接分发的 skill：

- `skills/forge-drop-autodeploy/`

这个 skill 配合内置 CLI `forgedrop-ctl` 使用，目标是让 AI 可以直接为新项目或存量项目生成自动部署配置，而不是手工点很多管理台表单。

典型流程：

1. AI 分析项目，决定制品类型、运行镜像、启动命令和 slot
2. AI 生成一份 deploy manifest JSON
3. AI 如需先发现可用 app，可先调用 `forgedrop-ctl apps`
4. AI 如需先读取现有配置，可先调用 `forgedrop-ctl export --app <app_key>` 导出 manifest，再基于它调整
5. AI 调用 `forgedrop-ctl apply` 自动创建/更新 repo、app、env、service、slot、token
6. AI 再去修改目标项目的 CI，把制品上传到 forge-drop

示例命令：

```bash
./bin/forgedrop-ctl profile set default \
  --server http://127.0.0.1:8080 \
  --token fd_admin_token_here \
  --activate

./bin/forgedrop-ctl apps
./bin/forgedrop-ctl apply --manifest ./skills/forge-drop-autodeploy/assets/deploy-manifest.example.json
./bin/forgedrop-ctl export --app demo --out ./deploy-manifest.json
```

如果你需要同时管理多个 ForgeDrop 实例，可以为每个实例准备一个 profile：

```bash
./bin/forgedrop-ctl profile set prod \
  --server https://deploy-prod.example.com \
  --token fd_prod_token

./bin/forgedrop-ctl profile set staging \
  --server https://deploy-staging.example.com \
  --token fd_staging_token

./bin/forgedrop-ctl profile use staging
./bin/forgedrop-ctl apps
./bin/forgedrop-ctl apply --manifest ./deploy/staging.json

./bin/forgedrop-ctl apps --profile prod
```

profile 文件布局：

- `default`: `~/.forgedrop/config.json` + `~/.forgedrop/auth.json`
- 命名 profile: `~/.forgedrop/profiles/<name>/config.json` + `auth.json`
- 当前 profile: `~/.forgedrop/active-profile`

如果你希望把实例里公开的 skill 直接安装到本地 Agent，也可以直接用 CLI：

```bash
./bin/forgedrop-ctl skill list --profile prod
./bin/forgedrop-ctl skill install forge-drop-autodeploy
./bin/forgedrop-ctl skill install forge-drop-autodeploy --target codex
```

说明：

- 不传 `--target` 时，交互式终端会提示你选择安装到 `~/.agents/skills` 还是 `~/.codex/skills`
- 如果目标位置已经安装过同名 skill，会先比对内容；相同则直接返回 `up_to_date`
- 如果内容不同，会询问是否覆盖；脚本场景可显式传 `--force`
- 安装完成后，重启对应 Agent / Codex 进程，让新 skill 生效

相关文件：

- skill 入口：`skills/forge-drop-autodeploy/SKILL.md`
- manifest 示例：`skills/forge-drop-autodeploy/assets/deploy-manifest.example.json`
- CI 示例：`skills/forge-drop-autodeploy/assets/forgejo-actions-autodeploy.yml`
- 公开读取：`GET /agents/skill` 或 `GET /agents/skill/forge-drop-autodeploy`

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
- `pr_number`：用于 PR preview 环境
- `change_set`：用于 change-set preview 环境（与 `pr_number` 同时传时，`change_set` 优先）
- `env_kind`：可选；当 `env=preview` 且希望直接更新命名模板环境时传 `named`
- `deploy`：可选，`1`（默认）表示上传后自动部署；`0` 表示仅创建快照并更新当前版本，等待手动部署
- 文件字段：`artifact=@xxx.jar`

`env=preview` 的行为规则：
- 默认（不传 `env_kind`）：必须提供 `pr_number` 或 `change_set`，会创建/更新 repo-scoped preview 子环境
- 传 `env_kind=named`：会直接更新命名环境 `preview`（模板环境）；此时不允许再传 `pr_number` / `change_set`

批量上传：`POST /api/v1/artifacts/upload-batch`
- 字段规则与单文件上传一致（含 `change_set` / `env_kind`）
- 文件字段使用 `file_<slotKey>=@...`，例如：`file_main=@app.jar`、`file_config=@config.zip`

下载制品（管理台登录态）：
- `GET /api/v1/admin/artifacts/:artifactID/download`
- 环境详情页“槽位与文件版本”卡片可直接下载当前生效版本

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
