const fs = require("fs");
const https = require("https");

const apiBase =
  process.env.FORGEJO_API_URL ||
  (process.env.FORGEJO_SERVER_URL
    ? `${process.env.FORGEJO_SERVER_URL}/api/v1`
    : null);
const repoFull = process.env.FORGEJO_REPOSITORY;
const eventPath = process.env.FORGEJO_EVENT_PATH;
const token = process.env.FORGEJO_TOKEN;
const runNumber = process.env.FORGEJO_RUN_NUMBER || "unknown";
const commentPrefix = "**<自动评论>ForgeDrop: 当前PR已部署至";
const legacyCommentPrefix = "<!-- mes-preview-comment -->";

if (!apiBase || !repoFull || !eventPath || !token) {
  console.error("Missing required Forgejo environment variables");
  process.exit(1);
}

let prNumber;
try {
  const eventPayload = JSON.parse(fs.readFileSync(eventPath, "utf8"));
  prNumber = eventPayload?.pull_request?.number;
} catch (error) {
  console.error("Failed to parse Forgejo event payload:", error.message);
  process.exit(1);
}

if (!prNumber) {
  console.error("Could not determine pull request number");
  process.exit(1);
}

let previewUrl;
try {
  const deployResult = JSON.parse(fs.readFileSync("deploy-response.json", "utf8"));
  previewUrl = deployResult?.service_url;
} catch (error) {
  console.error("Failed to parse deploy-response.json:", error.message);
  process.exit(1);
}

if (!previewUrl) {
  console.error("deploy-response.json does not contain service_url");
  process.exit(1);
}

const [repoOwner, repoName] = repoFull.split("/");
if (!repoOwner || !repoName) {
  console.error("FORGEJO_REPOSITORY must be in owner/repo format");
  process.exit(1);
}

const runUrl = `${process.env.FORGEJO_SERVER_URL}/${repoFull}/actions/runs/${runNumber}`;
const commentBody =
  `${commentPrefix}**：${previewUrl}\n\n` +
  `(自动更新于构建 ${runUrl})`;

const headers = {
  Authorization: `token ${token}`,
  Accept: "application/json",
  "Content-Type": "application/json",
  "User-Agent": "forge-drop-preview-commenter",
};

function request(method, apiPath, body) {
  return new Promise((resolve, reject) => {
    const url = new URL(`${apiBase}/repos/${repoOwner}/${repoName}${apiPath}`);
    const req = https.request(
      {
        method,
        hostname: url.hostname,
        port: url.port || 443,
        path: url.pathname + url.search,
        headers,
      },
      (res) => {
        let data = "";
        res.on("data", (chunk) => {
          data += chunk;
        });
        res.on("end", () => {
          if (res.statusCode >= 200 && res.statusCode < 300) {
            resolve(data ? JSON.parse(data) : null);
            return;
          }
          reject(new Error(`API request failed: ${res.statusCode} ${data}`));
        });
      },
    );

    req.on("error", reject);
    if (body) {
      req.write(JSON.stringify(body));
    }
    req.end();
  });
}

async function main() {
  const comments = await request("GET", `/issues/${prNumber}/comments`);
  const existingComment = comments.find((comment) =>
    comment.body &&
    (comment.body.startsWith(commentPrefix) ||
      comment.body.startsWith(legacyCommentPrefix)),
  );

  if (existingComment) {
    await request(
      "PATCH",
      `/issues/${prNumber}/comments/${existingComment.id}`,
      { body: commentBody },
    );
    console.log(`Updated preview comment #${existingComment.id}`);
    return;
  }

  await request("POST", `/issues/${prNumber}/comments`, { body: commentBody });
  console.log("Created preview comment");
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
