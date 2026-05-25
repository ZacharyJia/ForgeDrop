# CI Preview Comment Flow

This reference describes the only default CI flow this skill should optimize for.

## Canonical shape

Follow this structure:

1. `package` job builds one artifact
2. `deploy-preview` job runs on pull requests
3. `deploy-master-prod` job runs on pushes to `master`
4. preview deploy writes `deploy-response.json`
5. PR comment script reads `service_url` from that file

## Upload script environment variables

Required:

- `DEPLOY_URL`
- `DEPLOY_TOKEN`
- `DEPLOY_APP`
- `DEPLOY_ENV`
- `DEPLOY_SERVICE`
- `DEPLOY_SLOT`
- `DEPLOY_REPO`
- `DEPLOY_SHA`
- `DEPLOY_ARTIFACT`

Optional for this default flow:

- `DEPLOY_PR_NUMBER`

Use:

- `DEPLOY_ENV=preview` and `DEPLOY_PR_NUMBER=<pr>` for PR previews
- `DEPLOY_ENV=prod` for prod deploys

## PR comment script environment variables

Required:

- `FORGEJO_TOKEN`

Expected from Forgejo Actions runtime:

- `FORGEJO_REPOSITORY`
- `FORGEJO_EVENT_PATH`
- `FORGEJO_SERVER_URL`
- `FORGEJO_RUN_NUMBER`

## Workflow expectations

The workflow should look like this:

1. build artifact once
2. upload artifact through `node ./scripts/upload-deploy-artifact.js`
3. for PRs, run `node ./scripts/update-pr-preview-comment.js`
4. for `master` pushes, skip PR comment logic

## Organization-level assumptions

Assume CI can already access organization-level credentials and variables.

The workflow should consume those values, not try to configure the Git system itself.

## Output expectations

After setup, the target repo should behave like this:

- opening or updating a PR creates or refreshes a preview deployment
- the PR comment shows the preview URL
- pushing to `master` deploys production using the same built artifact
