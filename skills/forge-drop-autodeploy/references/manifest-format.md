# Deploy Manifest Format

`forge-dropctl apply` consumes a JSON manifest.

## Top-level fields

### `settings`

Optional map of forge-drop settings written through `PUT /api/v1/admin/settings`.

Common keys:

- `base_domain`
- `named_host_template`
- `preview_host_template`
- `docker_network`
- `traefik_acme_email`
- `traefik_acme_mode`
- `traefik_alicloud_region_id`
- `traefik_wildcard_enabled`
- `traefik_wildcard_include_apex`

### `repos`

Required array of repositories.

Each item:

```json
{
  "full_name": "owner/repo",
  "webhook_secret": "optional-secret"
}
```

The first repo is the default repo for slots that omit `repos`.

### `app`

Required application definition.

Fields:

- `app_key`: stable machine key used by upload API
- `name`: display name
- `envs`: optional named env list; defaults to `prod` and `preview`
- `services`: required service list

### `api_token`

Optional CI token request.

```json
{
  "name": "ci-my-app",
  "rotate_if_exists": true
}
```

When `rotate_if_exists` is `false` and the token already exists, `forge-dropctl apply` fails because forge-drop cannot reveal the previous plain token again.

## Service fields

Each `app.services[]` item supports:

- `service_key`
- `name`
- `image`
- `command`
- `container_port`
- `run_user`
- `env`
- `prod_host`
- `traefik_entrypoints`
- `compose_template`
- `deploy_strategy`
- `enabled`
- `slots`

Notes:

- `compose_template` is required.
- `deploy_strategy` may be `recreate` or `restart`.
- `enabled` defaults to `true` when omitted.

## Slot fields

Each `app.services[].slots[]` item supports:

- `slot_key`
- `name`
- `repos`
- `mount_type`
- `container_path`

Notes:

- `repos` is optional and defaults to the first top-level repo.
- `mount_type` defaults to `file`.
- `mount_type` must be `file` or `dir`.

## Minimal example

See `../assets/deploy-manifest.example.json`.
