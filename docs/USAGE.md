# forge-drop 使用指南

本指南描述如何从 0 到 1 配置 forge-drop，并把“CI 上传制品 -> 生成快照 ->（可选）部署 -> 查看状态/日志”的全流程跑通。

## 核心概念

- App：一个应用（例如一个后端系统）
- Service：一个服务（容器 / Compose 项目内的一组容器配置）
- Slot：挂载点。每个 slot 绑定一个 repo，并指定容器内路径（例如 `/app/app.jar`）
- Env：环境。命名环境如 `prod`/`staging`；预览环境为 `preview`（按 PR）
- Artifact：一次上传的文件（JAR 或任意文件）
- Snapshot：一次版本指针。Env 的 `current_snapshot` 表示“期望运行的版本（desired）”

提示：forge-drop 支持“上传后不自动部署”。此时会更新 desired，但不会触发容器更新；你可以在 Web UI 中手动点击部署。

## 部署前置条件

1. 服务器已安装 Docker，并可运行 `docker compose`
2. 反向代理（推荐 Traefik，Docker provider）
3. DNS 通配解析：`*.yourdomain.com` 指向服务器 IP（用于 preview 子域名）
4. Traefik 与业务容器处于同一 Docker network（例如 `traefik`）

可选：在管理台 Settings 页使用“一键安装/修复 Traefik”来启动一个由 forge-drop 管理的 Traefik 容器（会绑定宿主机 80/443）。

命名环境（prod/staging/dev）的 URL 默认按 `{app}-{service}-{env}.{base_domain}` 计算，可在设置里调整 `named_host_template`。

推荐：在 DNS-01（Aliyun）模式下启用通配符证书（`*.{base_domain}`），避免为每个子域名重复签发。

## Preview 模板与 PR 预览

- 系统会创建一个命名环境 `preview` 作为模板环境（可用于共享预览，也用于 PR 预览继承）。
- 当 CI 上传使用 `env=preview` 并携带 `pr_number` 时，会自动创建/更新一个 PR 专属 preview 环境，并继承模板环境的当前快照。
- 如果模板环境 `preview` 的快照有更新，可在 PR 环境详情页点击“同步 Preview 快照”，将模板最新快照同步并立即应用到当前 PR 环境。

## 管理台配置流程（建议顺序）

1. Settings：配置 `base_domain`、`preview_host_template`、`docker_network`
2. Repos：添加 `owner/repo` 并设置 webhook secret
3. Apps：创建 App
4. Services：为 App 创建 Service，并在服务编辑页配置 Docker Compose 模板 + 默认部署策略
5. Slots：为 Service 创建 Slot（repo + container path）
6. Envs：创建命名环境（至少创建 `prod`）
7. Tokens：创建 API Token 供 CI 上传制品

## Compose 模板

在服务编辑页可以查看模板示例。模板为 YAML + Go template。

常用变量：

- `{{.Artifacts}}`：slot_key -> 主机文件路径
- `{{.SlotPaths}}`：slot_key -> 容器内挂载路径
- `{{.Host}}`：该 env 的访问域名（preview/prod）
- `{{.EnvName}}`：环境名（prod/staging/preview）
- `{{.Network}}`：Docker network

## CI 上传制品

接口：`POST /api/v1/artifacts/upload`（multipart/form-data），需要 `Authorization: Bearer <token>`。

字段：

- `app` / `service` / `slot`
- `env`：`prod` / `staging` / `preview`
- `repo`：`owner/repo`（必须匹配 slot 绑定的 repo）
- `pr_number`：当 `env=preview` 必填
- `deploy`：可选；`1`（默认）上传后自动部署；`0` 仅创建快照并更新当前版本，等待手动部署
- 文件字段：`artifact=@xxx.jar`

示例：

```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -F "app=my-app" \
  -F "env=prod" \
  -F "service=api" \
  -F "slot=main" \
  -F "repo=owner/repo" \
  -F "sha=$GIT_SHA" \
  -F "deploy=0" \
  -F "artifact=@build/app.jar" \
  http://your-server:8080/api/v1/artifacts/upload
```

## 批量上传（一次生成同一个快照）

如果你的服务包含多个 slot（例如 jar + 配置包 + 静态资源），建议使用批量上传，避免多次上传导致版本不一致。

接口：`POST /api/v1/artifacts/upload-batch`

- 与单文件上传相同的字段：`app/env/service/repo/sha/ref/pr_number/deploy`
- 文件字段使用 `file_<slotKey>`，其中 `<slotKey>` 为服务下 slot 的 `slot_key`

示例：

```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -F "app=my-app" \
  -F "env=prod" \
  -F "service=api" \
  -F "repo=owner/repo" \
  -F "deploy=1" \
  -F "file_main=@build/app.jar" \
  -F "file_config=@build/config.zip" \
  http://your-server:8080/api/v1/artifacts/upload-batch
```

## 手动上传 / 手动部署

用于验证全流程：

1. 进入服务详情页，创建 Slot
2. 进入环境详情页（prod/staging）
3. 展开目标服务，选择要更新的槽位文件
4. 关闭“上传后自动部署”可进入“待部署”状态
5. 点击“部署当前环境”或“部署服务”触发更新

## 状态与日志

在环境详情页可查看：

- 当前快照（desired）
- `docker compose ps` 输出
- `docker compose logs`（tail 可调）

## 常见问题

- 部署失败：优先在环境详情页查看日志与 ps；其次在服务器用 `docker compose -f <composeFile> -p <project> logs`
- `compose file not found`：该 env+service 尚未部署（或 runtime 被清理）；点击一次“部署当前版本”即可
- 访问不到域名：检查 DNS、Traefik network、模板路由 labels
- `repo not allowed for this slot`：slot 绑定 repo 与 CI 传入 repo 不一致
