const api = window.HankAPI.request;

const state = {
  user: null,
  apps: [],
  preview: null,
  selectedApp: null,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  toast: document.getElementById("toast"),
  appsCount: document.getElementById("apps-count"),
  appsList: document.getElementById("apps-list"),
  importForm: document.getElementById("app-import-form"),
  packageInput: document.getElementById("app-package-input"),
  preview: document.getElementById("app-preview"),
  previewStatus: document.getElementById("app-preview-status"),
  previewBody: document.getElementById("app-preview-body"),
  installButton: document.getElementById("app-install-button"),
  configPanel: document.getElementById("app-config-panel"),
  configTitle: document.getElementById("app-config-title"),
  configStatus: document.getElementById("app-config-status"),
  configForm: document.getElementById("app-config-form"),
  appEnabled: document.getElementById("app-enabled"),
  publicConfig: document.getElementById("app-public-config"),
  secretFields: document.getElementById("app-secret-fields"),
};

function showToast(message, isError = false) {
  els.toast.hidden = false;
  els.toast.textContent = message;
  els.toast.style.background = isError ? "rgba(142, 45, 28, 0.94)" : "rgba(35, 27, 20, 0.92)";
  clearTimeout(showToast.timeoutID);
  showToast.timeoutID = window.setTimeout(() => {
    els.toast.hidden = true;
  }, 3400);
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function appID(app) {
  return app.app_id || app.id || "";
}

function appName(app) {
  return app.name || appID(app) || "App";
}

function appVersion(app) {
  return app.version || "unknown";
}

function appEnabled(app) {
  return Boolean(app.enabled);
}

function appStatus(app) {
  return app.status || "unknown";
}

function appPublicConfig(app) {
  const config = app.public_config_json ?? app.public_config ?? {};
  if (typeof config === "string") {
    return config.trim() || "{}";
  }
  return JSON.stringify(config || {}, null, 2);
}

function appSecretFields(app) {
  const fields = app.secret_fields_set_json ?? app.secret_fields_set ?? {};
  if (typeof fields === "string") {
    try {
      return JSON.parse(fields || "{}") || {};
    } catch {
      return {};
    }
  }
  return fields || {};
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function renderApps() {
  els.appsCount.textContent = `${state.apps.length} app${state.apps.length === 1 ? "" : "s"}`;
  if (!state.apps.length) {
    els.appsList.className = "card-list empty-state";
    els.appsList.textContent = "No apps installed.";
    return;
  }
  els.appsList.className = "card-list";
  els.appsList.innerHTML = "";
  state.apps.forEach((app) => {
    const id = appID(app);
    const card = document.createElement("article");
    card.className = "card split-card";
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(appName(app))}</div>
          <div class="meta">${escapeHTML(id)} · ${escapeHTML(appVersion(app))}</div>
        </div>
        <span class="status-chip ${appEnabled(app) ? "" : "offline"}">${escapeHTML(appEnabled(app) ? "enabled" : "disabled")}</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Status</div><div>${escapeHTML(appStatus(app))}</div></div>
        <div class="kv-row"><div class="kv-label">Updated</div><div>${escapeHTML(app.updated_at ? new Date(app.updated_at).toLocaleString() : "Unknown")}</div></div>
        <div class="kv-row"><div class="kv-label">Last Error</div><div>${escapeHTML(app.last_error || "None")}</div></div>
      </div>
      <div class="actions wrap compact">
        <button type="button" class="secondary" data-action="configure" data-app-id="${escapeHTML(id)}">Configure</button>
        <button type="button" class="ghost" data-action="toggle" data-app-id="${escapeHTML(id)}">${appEnabled(app) ? "Disable" : "Enable"}</button>
      </div>
    `;
    els.appsList.appendChild(card);
  });
}

function renderPreview() {
  const preview = state.preview;
  els.preview.hidden = !preview;
  els.installButton.disabled = !preview?.staging_id;
  if (!preview) {
    els.previewBody.innerHTML = "";
    return;
  }
  const app = preview.app || {};
  const slashCommands = app.slash_commands || [];
  const commands = app.commands || [];
  els.previewStatus.textContent = preview.replacing ? "Replace" : "New";
  els.previewBody.innerHTML = `
    <div class="kv-grid">
      <div class="kv-row"><div class="kv-label">Name</div><div>${escapeHTML(app.name)}</div></div>
      <div class="kv-row"><div class="kv-label">ID</div><div>${escapeHTML(app.id)}</div></div>
      <div class="kv-row"><div class="kv-label">Version</div><div>${escapeHTML(app.version)}</div></div>
      <div class="kv-row"><div class="kv-label">Publisher</div><div>${escapeHTML(app.publisher || "Unknown")}</div></div>
    </div>
    <div class="app-command-list">
      ${commands.map((command) => `<span class="pill">${escapeHTML(command.id)} · ${escapeHTML(command.mode || "request")}</span>`).join("") || '<span class="meta">No commands reported.</span>'}
    </div>
    <div class="app-permission-list">
      ${slashCommands.map((command) => `<span class="pill">${escapeHTML(command.command)} → ${escapeHTML(command.command_id)}</span>`).join("") || '<span class="meta">No slash commands reported.</span>'}
    </div>
  `;
}

function selectApp(app) {
  state.selectedApp = app;
  els.configPanel.hidden = !app;
  if (!app) return;
  els.configTitle.textContent = `${appName(app)} Configuration`;
  els.configStatus.textContent = appEnabled(app) ? "Enabled" : "Disabled";
  els.appEnabled.checked = appEnabled(app);
  els.publicConfig.value = appPublicConfig(app);
  const fields = appSecretFields(app);
  els.secretFields.innerHTML = "";
  Object.keys(fields).sort().forEach((field) => {
    const label = document.createElement("label");
    label.className = "setting-field";
    label.innerHTML = `
      <span>${escapeHTML(field)} ${fields[field] ? "(set)" : "(not set)"}</span>
      <input type="password" autocomplete="new-password" data-secret-field="${escapeHTML(field)}">
    `;
    els.secretFields.appendChild(label);
  });
}

async function loadApps() {
  const payload = await api("/v1/home/apps");
  state.apps = payload.apps || [];
  renderApps();
  if (state.selectedApp) {
    selectApp(state.apps.find((app) => appID(app) === appID(state.selectedApp)) || null);
  }
}

async function previewPackage(event) {
  event.preventDefault();
  const file = els.packageInput.files?.[0];
  if (!file) {
    showToast("Choose a .hankapp package.", true);
    return;
  }
  const formData = new FormData();
  formData.set("package", file);
  const headers = new Headers();
  const csrf = window.HankAPI.csrfToken?.();
  if (csrf) {
    headers.set("X-Hank-CSRF-Token", decodeURIComponent(csrf));
  }
  try {
    const response = await fetch("/v1/home/apps/import/preview", {
      method: "POST",
      credentials: "same-origin",
      headers,
      body: formData,
    });
    const contentType = response.headers.get("Content-Type") || "";
    const payload = contentType.includes("application/json") ? await response.json() : await response.text();
    if (!response.ok) {
      throw new Error(typeof payload === "string" ? payload : payload.error || payload.message || response.statusText);
    }
    state.preview = payload;
    renderPreview();
    showToast("Package preview ready.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function installPreview() {
  if (!state.preview?.staging_id) return;
  try {
    await api("/v1/home/apps/import/activate", {
      method: "POST",
      body: JSON.stringify({ staging_id: state.preview.staging_id, enable: false }),
    });
    state.preview = null;
    els.packageInput.value = "";
    renderPreview();
    await loadApps();
    showToast("App installed.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function saveSelectedApp(event) {
  event.preventDefault();
  const app = state.selectedApp;
  if (!app) return;
  let publicConfig = {};
  try {
    publicConfig = els.publicConfig.value.trim() ? JSON.parse(els.publicConfig.value) : {};
  } catch {
    showToast("Public config must be valid JSON.", true);
    return;
  }
  const secrets = {};
  els.secretFields.querySelectorAll("[data-secret-field]").forEach((input) => {
    const value = input.value.trim();
    if (value) {
      secrets[input.dataset.secretField] = value;
    }
  });
  try {
    const payload = { public_config: publicConfig, enable: els.appEnabled.checked };
    if (Object.keys(secrets).length) {
      payload.secrets = secrets;
    }
    await api(`/v1/home/apps/${encodeURIComponent(appID(app))}/config`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
    els.secretFields.querySelectorAll("[data-secret-field]").forEach((input) => {
      input.value = "";
    });
    await loadApps();
    showToast("App saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function toggleApp(app) {
  try {
    await api(`/v1/home/apps/${encodeURIComponent(appID(app))}/config`, {
      method: "PUT",
      body: JSON.stringify({ enable: !appEnabled(app) }),
    });
    await loadApps();
    showToast(appEnabled(app) ? "App disabled." : "App enabled.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function logout() {
  try {
    await api("/v1/auth/logout", { method: "POST" });
  } catch (_) {
  }
  window.location.replace("/");
}

async function hydrate() {
  try {
    const me = await api("/v1/me");
    state.user = me.user;
    renderSession();
    await loadApps();
  } catch (_) {
    window.location.replace("/");
  }
}

els.importForm.addEventListener("submit", previewPackage);
els.installButton.addEventListener("click", installPreview);
els.configForm.addEventListener("submit", saveSelectedApp);
els.logoutButton.addEventListener("click", logout);
els.appsList.addEventListener("click", (event) => {
  const button = event.target.closest("button[data-action]");
  if (!button) return;
  const app = state.apps.find((item) => appID(item) === button.dataset.appId);
  if (!app) return;
  if (button.dataset.action === "configure") {
    selectApp(app);
  } else if (button.dataset.action === "toggle") {
    toggleApp(app);
  }
});

hydrate();
