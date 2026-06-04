---
name: forgedrop-deploy
description: "Use this skill when you need AI to set up or operate a forge-drop app for local manual deployment only: create the app resources, create named envs, and upload built artifacts from a local machine without CI, PR preview comments, or Forgejo Actions."
---

# Forge Drop Manual Deploy

Use this skill for one specific workflow only:

1. create or inspect the forge-drop app resources
2. create named envs such as `prod`, `staging`, or `qa`
3. upload a locally built artifact with `forgedrop-ctl artifacts upload`
4. repeat that local upload flow for later manual updates

Do not expand this skill into CI automation, PR preview comments, or Forgejo Actions unless the user explicitly asks.

## Scope

This skill is for "create the app and deploy or update it by hand from a local machine".

Expected outcome:

1. forge-drop resources for the app exist
2. the operator has a working `forgedrop-ctl` profile with server URL and admin token
3. named envs needed for manual deployment exist
4. the operator can upload the built artifact directly from local disk

## What to use

- Inspect current apps with `./bin/forgedrop-ctl apps`
- Export an existing app with `./bin/forgedrop-ctl export --app ...` when the app may already exist
- Create or update resources with `./bin/forgedrop-ctl apply ...`
- Start from `assets/deploy-manifest.example.json`
- Read `references/manual-deploy-flow.md` for the exact local upload flow
- Read `references/manifest-format.md` when manifest fields need confirmation

## Default assumptions

- deployment is started from a local terminal, not CI
- one service receives one main artifact through `/api/v1/artifacts/upload`
- the operator can use an admin token through `forgedrop-ctl`
- named envs such as `prod`, `staging`, or `qa` are the main target
- `preview` is optional and should not be the default recommendation

If the target repo already has a real build command, keep that build command and only wire forge-drop around the produced artifact.

## Exact workflow to implement

1. inspect the target repo and identify the real build output
2. decide the app key, service key, slot path, runtime image, and command
3. generate or update a manifest for forge-drop
4. run `forgedrop-ctl apply`
5. if needed, create an extra named env through manifest or `forgedrop-ctl artifacts upload --create-env`
6. build the artifact locally
7. upload the artifact with `forgedrop-ctl artifacts upload`
8. for later updates, rebuild locally and run the same upload command again

## Manual upload contract to follow

Use `forgedrop-ctl artifacts upload` as the default path.

Required flags:

- `--app`
- `--env`
- `--service`
- `--slot`
- `--repo`
- `--file`

Common optional flags:

- `--create-env` when the named env does not exist yet
- `--deploy=false` when the operator wants to upload first and deploy later
- `--sha` and `--ref` when the local git state should be recorded on the snapshot
- `--deploy-strategy restart|recreate` when a specific deploy mode is needed

Prefer the CLI over raw `curl` unless the user explicitly asks for the HTTP request.

## Manifest guidance

Keep the manifest aligned to this workflow:

- one repo in `repos`
- `app.envs` should include the named envs the operator will use most often
- one service in the common case
- one slot named `main` in the common case
- include `settings.base_domain`
- include `settings.named_host_template`
- include `settings.preview_host_template` only when preview envs are actually needed
- omit `api_token` by default because this workflow does not depend on CI tokens

## forgedrop-ctl auth model

Use an admin token with `forgedrop-ctl`.

Default local files:

- `default` profile: `~/.forgedrop/config.json` + `~/.forgedrop/auth.json`
- named profile: `~/.forgedrop/profiles/<name>/config.json` + `auth.json`
- active profile marker: `~/.forgedrop/active-profile`

Expected shapes:

```json
{"server":"https://deploy.example.com"}
```

```json
{"token":"fd_admin_token_here"}
```

If the user mentions multiple ForgeDrop instances, prefer `forgedrop-ctl profile use <name>` or per-command `--profile <name>`.

## What to avoid

- do not add Forgejo Actions or GitHub Actions by default
- do not add PR preview comment scripts
- do not create CI tokens unless the user later asks for CI
- do not make preview envs the main deployment path
- do not turn this skill into a generic forge-drop administration guide

## When to read more

- Read `references/manual-deploy-flow.md` when the user wants the exact app-create plus local-upload sequence
- Read `references/manifest-format.md` when manifest field names need confirmation
- Read the repo README and `docs/USAGE.md` only when runtime/container choices depend on forge-drop behavior
