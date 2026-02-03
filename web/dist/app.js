const el = (tag, attrs = {}, ...children) => {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") node.className = v;
    else if (k === "onclick") node.onclick = v;
    else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
    else if (v !== undefined && v !== null) node.setAttribute(k, v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined) continue;
    node.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return node;
};

const state = {
  me: null,
  tab: "apps",
  apps: [],
  repos: [],
  tokens: [],
  settings: null,
  selectedApp: null,
  selectedEnv: null,
};

const whoEl = document.getElementById("whoami");
const logoutBtn = document.getElementById("logoutBtn");
logoutBtn.onclick = async () => {
  await fetch("/api/v1/auth/logout", { method: "POST" });
  state.me = null;
  render();
};

async function apiFetch(path, opts = {}) {
  const headers = { ...(opts.headers || {}) };
  if (opts.json) headers["Content-Type"] = "application/json";
  const res = await fetch(path, { ...opts, headers });
  const text = await res.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }
  if (!res.ok) {
    const msg = data?.error || `${res.status} ${res.statusText}`;
    const err = new Error(msg);
    err.status = res.status;
    err.data = data;
    throw err;
  }
  return data;
}

function notice(kind, msg) {
  return el("div", { class: `notice ${kind || ""}` }, msg);
}

function field(label, input) {
  return el("div", {}, el("label", {}, label), input);
}

async function refreshAll() {
  state.settings = await apiFetch("/api/v1/admin/settings");
  state.repos = await apiFetch("/api/v1/admin/repos");
  state.apps = await apiFetch("/api/v1/admin/apps");
  state.tokens = await apiFetch("/api/v1/admin/tokens");
}

async function refreshMe() {
  try {
    state.me = await apiFetch("/api/v1/admin/me");
  } catch (e) {
    state.me = null;
  }
}

function renderTabs() {
  const tabs = [
    ["apps", "Apps"],
    ["repos", "Repos"],
    ["tokens", "Tokens"],
    ["settings", "Settings"],
  ];
  return el(
    "div",
    { class: "tabs" },
    tabs.map(([k, name]) =>
      el(
        "div",
        {
          class: `tab ${state.tab === k ? "active" : ""}`,
          onclick: () => {
            state.tab = k;
            state.selectedApp = null;
            state.selectedEnv = null;
            render();
          },
        },
        name
      )
    )
  );
}

function renderLogin() {
  const username = el("input", { placeholder: "admin" });
  const password = el("input", { type: "password", placeholder: "password" });
  const setupUsername = el("input", { placeholder: "admin" });
  const setupPassword = el("input", { type: "password", placeholder: "password" });
  const msg = el("div");
  const setupMsg = el("div");

  const loginBtn = el(
    "button",
    {
      class: "btn btn-primary",
      onclick: async () => {
        msg.replaceChildren();
        try {
          await apiFetch("/api/v1/auth/login", {
            method: "POST",
            json: true,
            body: JSON.stringify({ username: username.value, password: password.value }),
          });
          await refreshMe();
          render();
        } catch (e) {
          msg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Login"
  );

  const setupBtn = el(
    "button",
    {
      class: "btn",
      onclick: async () => {
        setupMsg.replaceChildren();
        try {
          await apiFetch("/api/v1/setup", {
            method: "POST",
            json: true,
            body: JSON.stringify({ username: setupUsername.value, password: setupPassword.value }),
          });
          setupMsg.replaceChildren(notice("ok", "Admin created. Now login."));
        } catch (e) {
          setupMsg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Create admin (first run only)"
  );

  return el(
    "div",
    { class: "grid" },
    el(
      "div",
      { class: "card" },
      el("h2", {}, "Login"),
      field("Username", username),
      field("Password", password),
      el("div", { class: "row", style: "margin-top:10px" }, loginBtn),
      msg
    ),
    el(
      "div",
      { class: "card" },
      el("h2", {}, "Setup"),
      el("div", { class: "muted", style: "margin-bottom:8px" }, "Only works when no users exist yet."),
      field("Admin username", setupUsername),
      field("Admin password", setupPassword),
      el("div", { class: "row", style: "margin-top:10px" }, setupBtn),
      setupMsg
    )
  );
}

function renderApps() {
  const msg = el("div");
  const appKey = el("input", { placeholder: "my-app" });
  const name = el("input", { placeholder: "My App" });

  const create = el(
    "button",
    {
      class: "btn btn-primary",
      onclick: async () => {
        msg.replaceChildren();
        try {
          await apiFetch("/api/v1/admin/apps", {
            method: "POST",
            json: true,
            body: JSON.stringify({ app_key: appKey.value, name: name.value }),
          });
          state.apps = await apiFetch("/api/v1/admin/apps");
          render();
        } catch (e) {
          msg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Create"
  );

  const list = el(
    "table",
    { class: "table" },
    el("thead", {}, el("tr", {}, el("th", {}, "App Key"), el("th", {}, "Name"), el("th", {}, ""))),
    el(
      "tbody",
      {},
      state.apps.map((a) =>
        el(
          "tr",
          {},
          el("td", {}, el("span", { class: "mono" }, a.app_key)),
          el("td", {}, a.name),
          el(
            "td",
            {},
            el(
              "button",
              {
                class: "btn",
                onclick: async () => {
                  state.selectedApp = await apiFetch(`/api/v1/admin/apps/${a.id}`);
                  state.selectedEnv = null;
                  render();
                },
              },
              "Open"
            )
          )
        )
      )
    )
  );

  return el(
    "div",
    { class: "grid" },
    el("div", { class: "card" }, el("h2", {}, "Create App"), field("App key", appKey), field("Name", name), el("div", { class: "row" }, create), msg),
    el("div", { class: "card" }, el("h2", {}, "Apps"), list)
  );
}

function renderSelectedApp() {
  const app = state.selectedApp;
  const back = el(
    "button",
    {
      class: "btn btn-ghost",
      onclick: async () => {
        state.selectedApp = null;
        state.selectedEnv = null;
        state.apps = await apiFetch("/api/v1/admin/apps");
        render();
      },
    },
    "← Back"
  );

  const services = app.services || [];
  const envs = app.envs || [];

  const svcKey = el("input", { placeholder: "api" });
  const svcName = el("input", { placeholder: "API" });
  const image = el("input", { placeholder: "eclipse-temurin:17-jre", value: "eclipse-temurin:17-jre" });
  const command = el("input", { placeholder: "java -jar /app/app.jar", value: "java -jar /app/app.jar" });
  const port = el("input", { placeholder: "8080", value: "8080" });
  const runUser = el("input", { placeholder: "1000:1000", value: "1000:1000" });
  const prodHost = el("input", { placeholder: "api.example.com" });
  const envJson = el("textarea", { placeholder: '{"JAVA_OPTS":"-Xms128m -Xmx512m"}' });
  const msg = el("div");

  const createSvc = el(
    "button",
    {
      class: "btn btn-primary",
      onclick: async () => {
        msg.replaceChildren();
        let env = {};
        try {
          env = envJson.value.trim() ? JSON.parse(envJson.value) : {};
        } catch {
          msg.replaceChildren(notice("error", "Env must be valid JSON object"));
          return;
        }
        try {
          await apiFetch(`/api/v1/admin/apps/${app.id}/services`, {
            method: "POST",
            json: true,
            body: JSON.stringify({
              service_key: svcKey.value,
              name: svcName.value,
              image: image.value,
              command: command.value,
              container_port: Number(port.value || 0),
              run_user: runUser.value,
              prod_host: prodHost.value,
              env,
            }),
          });
          state.selectedApp = await apiFetch(`/api/v1/admin/apps/${app.id}`);
          render();
        } catch (e) {
          msg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Create service"
  );

  const envName = el("input", { placeholder: "prod" });
  const envMsg = el("div");
  const createEnv = el(
    "button",
    {
      class: "btn",
      onclick: async () => {
        envMsg.replaceChildren();
        try {
          await apiFetch(`/api/v1/admin/apps/${app.id}/envs`, {
            method: "POST",
            json: true,
            body: JSON.stringify({ name: envName.value }),
          });
          state.selectedApp = await apiFetch(`/api/v1/admin/apps/${app.id}`);
          render();
        } catch (e) {
          envMsg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Create env"
  );

  const svcTable = el(
    "table",
    { class: "table" },
    el("thead", {}, el("tr", {}, el("th", {}, "Key"), el("th", {}, "Image"), el("th", {}, "Prod host"), el("th", {}, ""))),
    el(
      "tbody",
      {},
      services.map((s) =>
        el(
          "tr",
          {},
          el("td", {}, el("span", { class: "mono" }, s.service_key)),
          el("td", {}, el("span", { class: "mono" }, s.image)),
          el("td", {}, s.prod_host || el("span", { class: "muted" }, "—")),
          el(
            "td",
            {},
            el(
              "button",
              {
                class: "btn",
                onclick: async () => {
                  const detail = await apiFetch(`/api/v1/admin/services/${s.id}`);
                  renderServiceEditor(app, detail.service, detail.slots);
                },
              },
              "Edit"
            )
          )
        )
      )
    )
  );

  const envTable = el(
    "table",
    { class: "table" },
    el("thead", {}, el("tr", {}, el("th", {}, "Kind"), el("th", {}, "Name"), el("th", {}, "Repo"), el("th", {}, "PR"), el("th", {}, ""))),
    el(
      "tbody",
      {},
      envs.map((e) =>
        el(
          "tr",
          {},
          el("td", {}, el("span", { class: "pill" }, e.kind)),
          el("td", {}, e.name),
          el("td", {}, e.repo_full_name ? el("span", { class: "mono" }, e.repo_full_name) : el("span", { class: "muted" }, "—")),
          el("td", {}, e.pr_number || el("span", { class: "muted" }, "—")),
          el(
            "td",
            {},
            el(
              "button",
              {
                class: "btn",
                onclick: async () => {
                  state.selectedEnv = await apiFetch(`/api/v1/admin/envs/${e.id}`);
                  render();
                },
              },
              "Open"
            )
          )
        )
      )
    )
  );

  return el(
    "div",
    {},
    el("div", { class: "row", style: "margin-bottom:10px" }, back, el("div", {}, el("div", { style: "font-weight:700" }, app.app_key), el("div", { class: "muted" }, app.name))),
    el(
      "div",
      { class: "grid" },
      el("div", { class: "card" }, el("h2", {}, "Create Service"), field("Service key", svcKey), field("Name", svcName), field("Image", image), field("Command", command), field("Container port", port), field("Run user", runUser), field("Prod host", prodHost), field("Env (JSON)", envJson), el("div", { class: "row" }, createSvc), msg),
      el("div", { class: "card" }, el("h2", {}, "Services"), svcTable, el("h2", { style: "margin-top:14px" }, "Envs"), el("div", { class: "row" }, field("Name", envName), createEnv), envMsg, envTable)
    )
  );
}

function renderServiceEditor(app, service, slots) {
  const root = document.getElementById("content");
  const msg = el("div");
  const back = el("button", { class: "btn btn-ghost", onclick: () => render() }, "← Back");

  const name = el("input", { value: service.name });
  const image = el("input", { value: service.image });
  const command = el("input", { value: service.command });
  const port = el("input", { value: String(service.container_port || 8080) });
  const runUser = el("input", { value: service.run_user });
  const prodHost = el("input", { value: service.prod_host || "" });
  const entrypoints = el("input", { value: service.traefik_entrypoints || "websecure" });
  const enabled = el("select", {}, el("option", { value: "1" }, "enabled"), el("option", { value: "0" }, "disabled"));
  enabled.value = service.enabled ? "1" : "0";

  const save = el(
    "button",
    {
      class: "btn btn-primary",
      onclick: async () => {
        msg.replaceChildren();
        try {
          await apiFetch(`/api/v1/admin/services/${service.id}`, {
            method: "PUT",
            json: true,
            body: JSON.stringify({
              name: name.value,
              image: image.value,
              command: command.value,
              container_port: Number(port.value || 0),
              run_user: runUser.value,
              prod_host: prodHost.value,
              traefik_entrypoints: entrypoints.value,
              enabled: enabled.value === "1",
              env: service.env || {},
            }),
          });
          msg.replaceChildren(notice("ok", "Saved. Note: mount changes may require 'Redeploy'."));
        } catch (e) {
          msg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Save"
  );

  const slotKey = el("input", { placeholder: "main" });
  const slotName = el("input", { placeholder: "Main JAR" });
  const repoId = el("select");
  for (const r of state.repos) repoId.appendChild(el("option", { value: r.id }, r.full_name));
  const containerPath = el("input", { placeholder: "/app/app.jar", value: "/app/app.jar" });
  const createSlotMsg = el("div");
  const createSlot = el(
    "button",
    {
      class: "btn",
      onclick: async () => {
        createSlotMsg.replaceChildren();
        try {
          await apiFetch(`/api/v1/admin/services/${service.id}/slots`, {
            method: "POST",
            json: true,
            body: JSON.stringify({
              slot_key: slotKey.value,
              name: slotName.value,
              repo_id: repoId.value,
              container_path: containerPath.value,
            }),
          });
          const detail = await apiFetch(`/api/v1/admin/services/${service.id}`);
          renderServiceEditor(app, detail.service, detail.slots);
        } catch (e) {
          createSlotMsg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Add slot"
  );

  const slotsTable = el(
    "table",
    { class: "table" },
    el("thead", {}, el("tr", {}, el("th", {}, "Key"), el("th", {}, "Repo"), el("th", {}, "Container path"), el("th", {}, ""))),
    el(
      "tbody",
      {},
      (slots || []).map((sl) =>
        el(
          "tr",
          {},
          el("td", {}, el("span", { class: "mono" }, sl.slot_key)),
          el("td", {}, el("span", { class: "mono" }, sl.repo_id)),
          el("td", {}, el("span", { class: "mono" }, sl.container_path)),
          el(
            "td",
            {},
            el(
              "button",
              {
                class: "btn btn-danger",
                onclick: async () => {
                  if (!confirm("Delete slot?")) return;
                  await apiFetch(`/api/v1/admin/services/${service.id}/slots/${sl.id}`, { method: "DELETE" });
                  const detail = await apiFetch(`/api/v1/admin/services/${service.id}`);
                  renderServiceEditor(app, detail.service, detail.slots);
                },
              },
              "Delete"
            )
          )
        )
      )
    )
  );

  root.replaceChildren(
    el(
      "div",
      {},
      el("div", { class: "row", style: "margin-bottom:10px" }, back, el("div", {}, el("div", { style: "font-weight:700" }, `Service: ${service.service_key}`), el("div", { class: "muted" }, `App: ${app.app_key}`))),
      el(
        "div",
        { class: "grid" },
        el("div", { class: "card" }, el("h2", {}, "Service"), field("Name", name), field("Image", image), field("Command", command), field("Port", port), field("Run user", runUser), field("Prod host", prodHost), field("Traefik entrypoints", entrypoints), field("Enabled", enabled), el("div", { class: "row" }, save), msg),
        el("div", { class: "card" }, el("h2", {}, "Slots"), el("div", { class: "row" }, field("Slot key", slotKey), field("Name", slotName)), el("div", { class: "row" }, field("Repo", repoId), field("Container path", containerPath), createSlot), createSlotMsg, slotsTable)
      )
    )
  );
}

function renderRepos() {
  const fullName = el("input", { placeholder: "owner/repo" });
  const secret = el("input", { placeholder: "webhook secret (HMAC)" });
  const msg = el("div");
  const create = el(
    "button",
    {
      class: "btn btn-primary",
      onclick: async () => {
        msg.replaceChildren();
        try {
          await apiFetch("/api/v1/admin/repos", {
            method: "POST",
            json: true,
            body: JSON.stringify({ full_name: fullName.value, webhook_secret: secret.value }),
          });
          state.repos = await apiFetch("/api/v1/admin/repos");
          render();
        } catch (e) {
          msg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Create"
  );

  const table = el(
    "table",
    { class: "table" },
    el("thead", {}, el("tr", {}, el("th", {}, "Full name"), el("th", {}, "Slug"), el("th", {}, "Secret"))),
    el(
      "tbody",
      {},
      state.repos.map((r) =>
        el(
          "tr",
          {},
          el("td", {}, el("span", { class: "mono" }, r.full_name)),
          el("td", {}, el("span", { class: "mono" }, r.slug)),
          el("td", {}, r.webhook_secret ? el("span", { class: "pill ok" }, "set") : el("span", { class: "pill bad" }, "empty"))
        )
      )
    )
  );

  return el(
    "div",
    { class: "grid" },
    el("div", { class: "card" }, el("h2", {}, "Create Repo"), field("Full name", fullName), field("Webhook secret", secret), el("div", { class: "row" }, create), msg),
    el("div", { class: "card" }, el("h2", {}, "Repos"), table)
  );
}

function renderTokens() {
  const name = el("input", { placeholder: "ci-token" });
  const msg = el("div");
  const create = el(
    "button",
    {
      class: "btn btn-primary",
      onclick: async () => {
        msg.replaceChildren();
        try {
          const res = await apiFetch("/api/v1/admin/tokens", { method: "POST", json: true, body: JSON.stringify({ name: name.value }) });
          msg.replaceChildren(notice("ok", `Token created (copy now): ${res.token}`));
          state.tokens = await apiFetch("/api/v1/admin/tokens");
          render();
        } catch (e) {
          msg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Create"
  );
  const table = el(
    "table",
    { class: "table" },
    el("thead", {}, el("tr", {}, el("th", {}, "Name"), el("th", {}, "Prefix"), el("th", {}, "Revoked"), el("th", {}, ""))),
    el(
      "tbody",
      {},
      state.tokens.map((t) =>
        el(
          "tr",
          {},
          el("td", {}, t.name),
          el("td", {}, el("span", { class: "mono" }, t.prefix)),
          el("td", {}, t.revoked_at ? el("span", { class: "pill bad" }, "yes") : el("span", { class: "pill ok" }, "no")),
          el(
            "td",
            {},
            el(
              "button",
              {
                class: "btn btn-danger",
                onclick: async () => {
                  if (!confirm("Revoke token?")) return;
                  await apiFetch(`/api/v1/admin/tokens/${t.id}/revoke`, { method: "POST" });
                  state.tokens = await apiFetch("/api/v1/admin/tokens");
                  render();
                },
              },
              "Revoke"
            )
          )
        )
      )
    )
  );
  return el(
    "div",
    { class: "grid" },
    el("div", { class: "card" }, el("h2", {}, "Create Token"), field("Name", name), el("div", { class: "row" }, create), msg),
    el("div", { class: "card" }, el("h2", {}, "Tokens"), table)
  );
}

function renderSettings() {
  const s = state.settings || {};
  const baseDomain = el("input", { value: s.base_domain || "" });
  const tpl = el("input", { value: s.preview_host_template || "" });
  const network = el("input", { value: s.docker_network || "" });
  const msg = el("div");
  const save = el(
    "button",
    {
      class: "btn btn-primary",
      onclick: async () => {
        msg.replaceChildren();
        try {
          await apiFetch("/api/v1/admin/settings", {
            method: "PUT",
            json: true,
            body: JSON.stringify({
              base_domain: baseDomain.value,
              preview_host_template: tpl.value,
              docker_network: network.value,
            }),
          });
          state.settings = await apiFetch("/api/v1/admin/settings");
          msg.replaceChildren(notice("ok", "Saved"));
        } catch (e) {
          msg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Save"
  );
  return el(
    "div",
    { class: "grid" },
    el(
      "div",
      { class: "card" },
      el("h2", {}, "Settings"),
      field("Base domain", baseDomain),
      field("Preview host template", tpl),
      field("Docker network", network),
      el("div", { class: "row" }, save),
      msg
    ),
    el(
      "div",
      { class: "card" },
      el("h2", {}, "Endpoints"),
      el("div", { class: "muted" }, "Use these in Forgejo Actions / Webhook config:"),
      el("div", { style: "margin-top:10px" }, el("div", { class: "muted" }, "Artifact upload URL"), el("div", { class: "mono" }, s.artifact_upload_url || "")),
      el("div", { style: "margin-top:10px" }, el("div", { class: "muted" }, "Forgejo webhook URL"), el("div", { class: "mono" }, s.forgejo_webhook_url || ""))
    )
  );
}

function renderEnvDetail() {
  const env = state.selectedEnv.env;
  const app = state.selectedEnv.app;
  const services = state.selectedEnv.services || [];
  const back = el("button", { class: "btn btn-ghost", onclick: () => { state.selectedEnv = null; render(); } }, "← Back");
  const msg = el("div");
  const snapId = el("input", { placeholder: "snapshot_id" });
  const rollback = el(
    "button",
    {
      class: "btn btn-danger",
      onclick: async () => {
        msg.replaceChildren();
        try {
          await apiFetch(`/api/v1/admin/envs/${env.id}/rollback`, { method: "POST", json: true, body: JSON.stringify({ snapshot_id: snapId.value }) });
          msg.replaceChildren(notice("ok", "Rollback applied (containers restarted if Docker enabled)."));
        } catch (e) {
          msg.replaceChildren(notice("error", e.message));
        }
      },
    },
    "Rollback"
  );

  const urlList = el("div", {});
  const showUrls = async () => {
    const items = [];
    for (const svc of services) {
      items.push(el("div", {}, el("span", { class: "mono" }, svc.service_key), " → ", el("span", { class: "muted" }, "see Traefik host in service config")));
    }
    urlList.replaceChildren(...items);
  };
  showUrls();

  return el(
    "div",
    {},
    el("div", { class: "row", style: "margin-bottom:10px" }, back, el("div", {}, el("div", { style: "font-weight:700" }, `Env: ${env.kind}/${env.name}`), el("div", { class: "muted" }, `App: ${app.app_key}`))),
    el(
      "div",
      { class: "grid" },
      el(
        "div",
        { class: "card" },
        el("h2", {}, "Rollback"),
        el("div", { class: "muted", style: "margin-bottom:8px" }, "Set env current_snapshot_id then apply all services."),
        field("Snapshot ID", snapId),
        el("div", { class: "row" }, rollback),
        msg
      ),
      el("div", { class: "card" }, el("h2", {}, "Services"), urlList)
    )
  );
}

async function ensureAuthedAndLoad() {
  await refreshMe();
  if (!state.me) return;
  await refreshAll();
}

function render() {
  whoEl.textContent = state.me ? `@${state.me.username}` : "";
  logoutBtn.classList.toggle("hidden", !state.me);

  const root = document.getElementById("content");
  if (!state.me) {
    root.replaceChildren(renderLogin());
    return;
  }

  const body = [];
  body.push(renderTabs());

  if (state.selectedEnv) {
    body.push(renderEnvDetail());
    root.replaceChildren(...body);
    return;
  }

  if (state.selectedApp) {
    body.push(renderSelectedApp());
    root.replaceChildren(...body);
    return;
  }

  if (state.tab === "apps") body.push(renderApps());
  if (state.tab === "repos") body.push(renderRepos());
  if (state.tab === "tokens") body.push(renderTokens());
  if (state.tab === "settings") body.push(renderSettings());

  root.replaceChildren(...body);
}

(async () => {
  await ensureAuthedAndLoad();
  render();
})();

