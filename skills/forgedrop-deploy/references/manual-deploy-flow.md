# Manual Deploy Flow

This reference describes the default forge-drop workflow when no CI is involved.

## Canonical shape

Follow this sequence:

1. configure a `forgedrop-ctl` profile with server URL and admin token
2. inspect the current app list with `forgedrop-ctl apps`
3. apply a manifest that creates the repo, app, envs, service, and slot
4. build the real artifact locally
5. upload it with `forgedrop-ctl artifacts upload`
6. repeat that upload command for later manual updates

## Profile setup

Typical setup:

```bash
./bin/forgedrop-ctl profile set prod \
  --server https://deploy.example.com \
  --token fd_admin_token_here \
  --activate
```

## Apply phase

Typical apply flow:

```bash
./bin/forgedrop-ctl apps
./bin/forgedrop-ctl apply --manifest ./deploy/forgedrop-manifest.json
```

If the app already exists, export first:

```bash
./bin/forgedrop-ctl export --app demo-service --out ./deploy/forgedrop-manifest.json
```

## Upload phase

Typical upload flow:

```bash
./bin/forgedrop-ctl artifacts upload \
  --profile prod \
  --app demo-service \
  --env prod \
  --service api \
  --slot main \
  --repo acme/demo-service \
  --file ./dist/app.bin
```

If the env does not exist yet:

```bash
./bin/forgedrop-ctl artifacts upload \
  --profile prod \
  --app demo-service \
  --env qa \
  --service api \
  --slot main \
  --repo acme/demo-service \
  --file ./dist/app.bin \
  --create-env
```

If the operator wants to upload first and deploy later:

```bash
./bin/forgedrop-ctl artifacts upload \
  --profile prod \
  --app demo-service \
  --env prod \
  --service api \
  --slot main \
  --repo acme/demo-service \
  --file ./dist/app.bin \
  --deploy=false
```

## Output expectations

After setup, the target repo should behave like this:

- the app resources already exist in forge-drop
- the operator can deploy from a local terminal without CI
- later updates are just rebuild plus another `artifacts upload`
