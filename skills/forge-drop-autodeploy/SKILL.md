---
name: forge-drop-autodeploy
description: Use this skill when you need to configure forge-drop for a new or existing project, generate a deploy manifest, apply it through the built-in forge-dropctl CLI, and wire CI artifact uploads for automatic deployment.
---

# Forge Drop Auto Deploy

Use this skill when the user wants AI to set up automatic deployment on top of `forge-drop`.

This skill is for two kinds of work:

1. Configure `forge-drop` itself for a project by generating a declarative manifest and applying it with `forge-dropctl`.
2. Update the target project's CI so build artifacts are uploaded to `forge-drop` automatically.

## What to use

- Apply config with `go run ./cmd/forge-dropctl apply ...`
- Use the manifest example at `assets/deploy-manifest.example.json`
- Read `references/manifest-format.md` when you need field-level details
- Use `assets/forgejo-actions-upload.yml` as the starting point for CI upload logic

## Workflow

1. Inspect the target project.
2. Decide what artifact `forge-drop` should receive.
3. Decide the runtime container contract.
4. Generate a manifest JSON.
5. Apply the manifest with `forge-dropctl`.
6. Patch CI to upload the artifact to `/api/v1/artifacts/upload` or `/api/v1/artifacts/upload-batch`.
7. If useful, document the exact upload fields and env vars inside the target repo.

## Project inspection checklist

- Identify the build output: `jar`, binary, `zip`, config package, static bundle, or multiple files.
- Identify whether deployment needs one slot or multiple slots.
- Identify the runtime image and start command.
- Identify the service port if the service is externally reachable.
- Identify the repo full name used by CI uploads.

Important: `forge-drop` deploys uploaded artifacts into a runtime container. It does not build source code into an image for you. If the project currently depends on `docker build`, convert the deployment design into:

- CI builds artifact(s)
- `forge-drop` mounts artifact(s) into the container defined by `compose_template`

## Manifest rules

- Keep one top-level repo in `repos` for the common case.
- Put every named environment under `app.envs`. If omitted, `forge-dropctl` ensures `prod` and `preview`.
- Put the full desired service shape in the manifest, especially `compose_template`.
- Each slot maps one uploaded artifact to one container path.
- When a slot omits `repos`, it defaults to the first repo in `repos`.
- If a CI token is needed, set `api_token.name`.
- If the token already exists and you need the plain token again, set `api_token.rotate_if_exists=true`.

## Apply command

Use environment variables or flags for credentials:

```bash
FORGE_DROP_SERVER=http://127.0.0.1:8080 \
FORGE_DROP_USERNAME=admin \
FORGE_DROP_PASSWORD='secret123' \
go run ./cmd/forge-dropctl apply --manifest /path/to/deploy.manifest.json
```

The command returns JSON. Use it to capture created resource IDs and any newly issued `plain_token`.

## CI wiring

For single-slot services, use `/api/v1/artifacts/upload`.

Required form fields:

- `app`
- `env`
- `service`
- `slot`
- `repo`
- `artifact`

Useful optional fields:

- `sha`
- `ref`
- `deploy=0` when you want upload without immediate deployment
- `deploy_strategy=restart` for fast in-place restarts when appropriate
- `pr_number` or `change_set` for preview environments

For multi-slot services, use `/api/v1/artifacts/upload-batch` and send files as `file_<slotKey>`.

## Heuristics

- Java JAR service:
  image often `eclipse-temurin:*` runtime images
  slot path often `/app/app.jar`
  command often `sh -lc "java -jar /app/app.jar"`
- Go binary:
  image can be `debian:bookworm-slim` or another runtime with required libc
  slot path often `/app/server`
  command often `sh -lc "chmod +x /app/server && /app/server"`
- Static frontend:
  upload built assets as a directory-style artifact and mount into nginx or caddy docroot
- App plus config:
  use two slots, for example `main` and `config`

## When to read more

- Read `references/manifest-format.md` when you need exact field meanings or defaults.
- Read the repo README and `docs/USAGE.md` when the deployment design depends on preview envs, Traefik, or artifact upload semantics.
