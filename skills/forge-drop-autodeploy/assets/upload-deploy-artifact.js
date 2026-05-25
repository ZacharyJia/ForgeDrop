const fs = require("fs");
const path = require("path");

function requiredEnv(name) {
  const value = process.env[name];
  if (!value) {
    console.error(`Missing required environment variable: ${name}`);
    process.exit(1);
  }
  return value;
}

async function main() {
  const deployUrl = requiredEnv("DEPLOY_URL");
  const deployToken = requiredEnv("DEPLOY_TOKEN");
  const artifactPath = requiredEnv("DEPLOY_ARTIFACT");

  if (!fs.existsSync(artifactPath)) {
    console.error(`Artifact file not found: ${artifactPath}`);
    process.exit(1);
  }

  const form = new FormData();
  form.set("app", requiredEnv("DEPLOY_APP"));
  form.set("env", requiredEnv("DEPLOY_ENV"));
  form.set("service", requiredEnv("DEPLOY_SERVICE"));
  form.set("slot", requiredEnv("DEPLOY_SLOT"));
  form.set("repo", requiredEnv("DEPLOY_REPO"));
  form.set("sha", requiredEnv("DEPLOY_SHA"));
  form.set("deploy", "1");

  if (process.env.DEPLOY_PR_NUMBER) {
    form.set("pr_number", process.env.DEPLOY_PR_NUMBER);
  }

  const artifactBuffer = fs.readFileSync(artifactPath);
  form.set(
    "artifact",
    new Blob([artifactBuffer], { type: "application/octet-stream" }),
    path.basename(artifactPath),
  );

  const response = await fetch(deployUrl, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${deployToken}`,
    },
    body: form,
  });

  const responseText = await response.text();
  fs.writeFileSync("deploy-response.json", responseText);

  console.log(`Deploy upload HTTP status: ${response.status}`);
  console.log("Deploy upload response:");
  console.log(responseText);

  if (!response.ok) {
    process.exit(1);
  }
}

main().catch((error) => {
  console.error("Deploy upload failed:");
  console.error(error.stack || error.message);
  process.exit(1);
});
