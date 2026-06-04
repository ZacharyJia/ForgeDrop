# Deploy Manifest Format

`forgedrop-ctl apply` consumes a JSON manifest.

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
  "full_name": "owner/repo"
}
```

### `app`

Required application definition.

Fields:

- `app_key`: stable machine key used by upload API
- `name`: display name
- `envs`: optional named env list
- `services`: required service list

This manual-deploy skill usually keeps `envs` focused on named envs such as `prod`, `staging`, and `qa`.

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
