---
name: forge-drop-autodeploy
description: "Use this skill when you need AI to set up forge-drop automatic deployment for a repository with this fixed pattern: apply forge-drop config, add a Forgejo Actions workflow, upload one built artifact, deploy PR previews, and update the PR comment with the preview URL."
---

# Forge Drop Auto Deploy

Use this skill for one specific integration pattern only.

The default integration pattern is:

1. `forgedrop-ctl apply` creates or updates forge-drop resources.
2. The repo uses Forgejo/Gitea Actions.
3. CI builds one deployment artifact.
4. PR builds upload that artifact to `env=preview`.
5. CI updates the PR comment using `service_url` from `deploy-response.json`.
6. Pushes to `master` upload the same artifact to `env=prod`.

Do not expand this skill into other deployment styles unless the user explicitly asks.

## Scope

This skill is for "make this repository follow the standard forge-drop CI deployment pattern".

Expected repo-side outcome:

1. forge-drop resources for the repo exist
2. the repo has a Forgejo workflow for build + preview deploy + prod deploy
3. the repo has an upload script
4. the repo has a PR preview comment script

## What to use

- Apply config with `./bin/forgedrop-ctl apply ...`
- Use `assets/deploy-manifest.example.json` as the manifest starting point
- Read `references/manifest-format.md` only when manifest fields need clarification
- Use `assets/forgejo-actions-autodeploy.yml` as the workflow starting point
- Use `assets/upload-deploy-artifact.js` as the upload script starting point
- Use `assets/update-pr-preview-comment.js` as the PR comment script starting point
- Read `references/ci-preview-comment-flow.md` for the exact CI flow

## Default assumptions

- Git hosting is Forgejo
- CI is Forgejo Actions
- Git-system credentials and variables are already provided at the organization level
- Deployment is CI-driven only
- No webhook setup is required for this workflow
- One service receives one main artifact through `/api/v1/artifacts/upload`

If the target repo already has working build steps, keep those build steps and only adapt the deployment parts around them.

## Exact workflow to implement

1. Inspect the repo and find the real build output.
2. Decide the runtime image, command, app key, service key, and slot path.
3. Generate a manifest for forge-drop.
4. Run `forgedrop-ctl apply`.
5. Capture the returned `plain_token` if a token was created or rotated.
6. Add or update a Forgejo Actions workflow with three jobs:
   `package`
   `deploy-preview`
   `deploy-master-prod`
7. Add or update `scripts/upload-deploy-artifact.js`.
8. Add or update `scripts/update-pr-preview-comment.js`.
9. Make PR builds upload to `preview` with `pr_number`.
10. Make `master` pushes upload to `prod`.

## CI pattern to follow

Match the yandun-mes-v2 shape closely:
Match this fixed shape closely:

- trigger on `push` to `master`
- optionally keep `issue-*` push builds if the repo uses issue branches
- trigger on `pull_request`
- build once in `package`
- reuse the built artifact in deploy jobs
- preview deploy job runs only for pull requests
- prod deploy job runs only for `push` on `master`

## Upload contract to follow

Use `/api/v1/artifacts/upload` with these fields:

- `app`
- `env`
- `service`
- `slot`
- `repo`
- `sha`
- `artifact`

For preview deploys, also send:

- `pr_number`

Persist the raw response to `deploy-response.json`. The PR comment step must read `service_url` from that file.

## PR comment contract to follow

Use Forgejo issue comments for pull requests.

The script should:

1. read the PR number from `FORGEJO_EVENT_PATH`
2. read `service_url` from `deploy-response.json`
3. find an existing bot comment by prefix
4. update that comment if found
5. create a new one otherwise

## Manifest guidance

Keep the manifest aligned to this workflow:

- one repo in `repos`
- `app.envs` should include `prod` and `preview`
- one service in the common case
- one slot named `main` in the common case
- include `settings.base_domain`
- include `settings.named_host_template`
- include `settings.preview_host_template`
- include `api_token.name` when CI needs a token

Do not introduce webhook-oriented fields into the default path.

## What to avoid

- do not make webhook setup part of the default workflow
- do not introduce upload-batch as the main path
- do not introduce change-set preview flow as the main path
- do not broaden this skill to GitHub Actions or other CI providers by default
- do not turn this skill into a generic forge-drop operations guide

## When to read more

- Read `references/ci-preview-comment-flow.md` for the exact deployment and comment sequence
- Read `references/manifest-format.md` when manifest field names need confirmation
- Read the repo README and `docs/USAGE.md` only when runtime/container decisions depend on forge-drop behavior
