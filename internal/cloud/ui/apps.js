const api = window.HankAPI.request;
const escapeHTML = window.HankUI.escapeHTML;

const state = {
  user: null,
  apps: [],
  preview: null,
  selectedApp: null,
  fileSources: [],
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  toast: document.getElementById("toast"),
  appsCount: document.getElementById("apps-count"),
  installedAppSelect: document.getElementById("installed-app-select"),
  selectedAppPanel: document.getElementById("selected-app-panel"),
  installOpenButton: document.getElementById("app-install-open-button"),
  installDialog: document.getElementById("app-install-dialog"),
  packageForm: document.getElementById("app-package-form"),
  installCancelButton: document.getElementById("app-install-cancel-button"),
  packageInput: document.getElementById("app-package-input"),
  folderInput: document.getElementById("app-folder-input"),
  preview: document.getElementById("app-preview"),
  previewStatus: document.getElementById("app-preview-status"),
  previewBody: document.getElementById("app-preview-body"),
  installButton: document.getElementById("app-install-button"),
  configPanel: document.getElementById("app-config-panel"),
  configTitle: document.getElementById("app-config-title"),
  configStatus: document.getElementById("app-config-status"),
  configForm: document.getElementById("app-config-form"),
  appEnabled: document.getElementById("app-enabled"),
  appUserAccess: document.getElementById("app-user-access"),
  publicConfig: document.getElementById("app-public-config"),
  secretFields: document.getElementById("app-secret-fields"),
};

function showToast(message, isError = false) {
  window.HankUI.showToast(els.toast, message, isError);
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

function appUserAccess(app) {
  return app.user_access === "home_members" ? "home_members" : "admins_only";
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

function appSettingsSchema(app) {
  const schema = app.settings_schema_json ?? app.settings_schema ?? {};
  if (typeof schema === "string") {
    try {
      return JSON.parse(schema || "{}") || {};
    } catch {
      return {};
    }
  }
  return schema || {};
}

function sortedSettingsFields(app) {
  const fields = appSettingsSchema(app).fields || [];
  return Array.isArray(fields)
    ? fields.slice().sort((a, b) => Number(a.order || 0) - Number(b.order || 0) || String(a.key || "").localeCompare(String(b.key || "")))
    : [];
}

function parseConfigValue(app) {
  try {
    return JSON.parse(appPublicConfig(app) || "{}") || {};
  } catch {
    return {};
  }
}

function configDefault(field) {
  if (!Object.prototype.hasOwnProperty.call(field, "default")) return undefined;
  return field.default;
}

function fieldValue(config, field) {
  const key = field.key || "";
  if (Object.prototype.hasOwnProperty.call(config, key)) return config[key];
  return configDefault(field);
}

function fileSourceOptions(currentValue = "") {
  const options = new Map();
  state.fileSources.forEach((source) => {
    const id = String(source.id || source.source_id || "").trim();
    if (!id) return;
    options.set(id, source.label || source.name || source.share || id);
  });
  if (currentValue && !options.has(currentValue)) {
    options.set(currentValue, currentValue);
  }
  return Array.from(options.entries()).map(([value, label]) => ({ value, label }));
}

function fieldOptions(field, currentValue = "") {
  if (field.source === "file_sources") {
    return fileSourceOptions(currentValue);
  }
  return Array.isArray(field.options) ? field.options : [];
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function renderApps() {
  els.appsCount.textContent = `${state.apps.length} app${state.apps.length === 1 ? "" : "s"}`;
  renderAppSelector();
  renderSelectedAppPanel();
}

function renderAppSelector() {
  if (!state.apps.length) {
    els.installedAppSelect.innerHTML = `<option value="">No apps installed</option>`;
    els.installedAppSelect.disabled = true;
    state.selectedApp = null;
    return;
  }
  els.installedAppSelect.disabled = false;
  if (!state.selectedApp || !state.apps.some((app) => appID(app) === appID(state.selectedApp))) {
    state.selectedApp = state.apps[0];
  }
  const selectedID = appID(state.selectedApp);
  els.installedAppSelect.innerHTML = state.apps.map((app) => {
    const id = appID(app);
    const label = `${appName(app)} (${id})`;
    return `<option value="${escapeHTML(id)}" ${id === selectedID ? "selected" : ""}>${escapeHTML(label)}</option>`;
  }).join("");
}

function renderSelectedAppPanel() {
  const app = state.selectedApp;
  if (!app) {
    els.selectedAppPanel.className = "stack empty-state";
    els.selectedAppPanel.textContent = "No apps installed.";
    els.configPanel.hidden = true;
    return;
  }
  els.selectedAppPanel.className = "stack";
  const id = appID(app);
  els.selectedAppPanel.innerHTML = `
    <article class="card split-card">
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
    </article>
  `;
}

function renderLegacyAppsList() {
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
    els.selectedAppPanel.appendChild(card);
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
  renderAppSelector();
  renderSelectedAppPanel();
  els.configPanel.hidden = !app;
  if (!app) return;
  els.configTitle.setAttribute("tabindex", "-1");
  els.configTitle.textContent = `${appName(app)} Configuration`;
  els.configStatus.textContent = appEnabled(app) ? "Enabled" : "Disabled";
  els.appEnabled.checked = appEnabled(app);
  if (els.appUserAccess) {
    els.appUserAccess.value = appUserAccess(app);
  }
  const config = parseConfigValue(app);
  els.publicConfig.value = JSON.stringify(config, null, 2);
  els.secretFields.innerHTML = "";
  const settingsFields = sortedSettingsFields(app);
  const rawConfigField = els.publicConfig.closest(".setting-field");
  if (rawConfigField) {
    rawConfigField.hidden = settingsFields.length > 0;
    rawConfigField.style.display = settingsFields.length > 0 ? "none" : "";
  }
  if (settingsFields.length) {
    renderSettingsFields(app, config, settingsFields);
  } else {
    renderLegacySecretFields(app);
  }
  revealConfigPanel();
}

function renderLegacySecretFields(app) {
  const fields = appSecretFields(app);
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

function renderSettingsFields(app, config, fields) {
  const secretState = appSecretFields(app);
  fields.forEach((field) => {
    if (!field?.key) return;
    const key = String(field.key);
    const value = fieldValue(config, field);
    const label = document.createElement("label");
    label.className = field.type === "boolean" ? "toggle-field" : "setting-field";
    const labelText = field.label || key;
    const help = field.help ? `<span class="meta">${escapeHTML(field.help)}</span>` : "";
    if (field.type === "boolean") {
      label.innerHTML = `
        <input type="checkbox" data-config-field="${escapeHTML(key)}" ${Boolean(value) ? "checked" : ""}>
        <span>${escapeHTML(labelText)}</span>
        ${help}
      `;
    } else if (field.type === "select") {
      const options = fieldOptions(field, String(value || ""));
      label.innerHTML = `
        <span>${escapeHTML(labelText)}</span>
        <select data-config-field="${escapeHTML(key)}" ${field.required ? "required" : ""}>
          <option value="">Default</option>
          ${options.map((option) => `<option value="${escapeHTML(option.value)}" data-option-type="${escapeHTML(typeof option.value)}" ${String(option.value) === String(value || "") ? "selected" : ""}>${escapeHTML(option.label || option.value)}</option>`).join("")}
        </select>
        ${help}
      `;
    } else {
      const isSecret = Boolean(field.secret);
      const inputType = isSecret ? "password" : field.type === "number" ? "number" : field.type === "url" ? "url" : "text";
      const attr = isSecret ? `data-secret-field="${escapeHTML(field.secret_key || key)}"` : `data-config-field="${escapeHTML(key)}"`;
      const setLabel = isSecret ? ` ${secretState[field.secret_key || key] ? "(set)" : "(not set)"}` : "";
      label.innerHTML = `
        <span>${escapeHTML(labelText)}${escapeHTML(setLabel)}</span>
        <input type="${inputType}" ${attr} ${field.required && !isSecret ? "required" : ""} placeholder="${escapeHTML(field.placeholder || "")}" autocomplete="${isSecret ? "new-password" : "off"}" value="${isSecret ? "" : escapeHTML(value ?? "")}">
        ${help}
      `;
    }
    els.secretFields.appendChild(label);
  });
}

function revealConfigPanel() {
  window.requestAnimationFrame(() => {
    els.configPanel.scrollIntoView({ behavior: "smooth", block: "start" });
    els.configTitle.focus({ preventScroll: true });
    scrollContainingFramesIntoView();
  });
}

function scrollContainingFramesIntoView() {
  let currentWindow = window;
  for (let depth = 0; depth < 3; depth += 1) {
    try {
      const frame = currentWindow.frameElement;
      if (!frame) return;
      frame.scrollIntoView({ behavior: "smooth", block: "nearest" });
      currentWindow = currentWindow.parent;
    } catch (_) {
      return;
    }
  }
}

async function loadApps() {
  const payload = await api("/v1/home/apps");
  const selectedID = state.selectedApp ? appID(state.selectedApp) : "";
  state.apps = payload.apps || [];
  if (selectedID) {
    state.selectedApp = state.apps.find((app) => appID(app) === selectedID) || null;
  }
  renderApps();
  if (state.selectedApp) {
    selectApp(state.apps.find((app) => appID(app) === appID(state.selectedApp)) || null);
  }
}

async function loadFileSources() {
  try {
    const payload = await api("/v1/home/service-profiles");
    const profiles = payload.profiles || [];
    const smb = profiles.find((profile) => profile.service_type === "smb");
    const config = parseProfileConfig(smb);
    const rawSources = Array.isArray(config.file_sources) ? config.file_sources
      : Array.isArray(config.sources) ? config.sources
        : [];
    state.fileSources = rawSources.map((source, index) => normalizeFileSource(source, index)).filter(Boolean);
  } catch (_) {
    state.fileSources = [];
  }
}

function parseProfileConfig(profile) {
  if (!profile) return {};
  const raw = profile.public_config_json ?? profile.public_config ?? {};
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw || "{}") || {};
    } catch {
      return {};
    }
  }
  return raw || {};
}

function normalizeFileSource(source, index) {
  const id = String(source?.id || source?.source_id || source?.name || source?.share || `source_${index + 1}`).trim();
  if (!id) return null;
  const label = source?.label || source?.name || source?.share || id;
  return { ...source, id, label };
}

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n += 1) {
    let c = n;
    for (let k = 0; k < 8; k += 1) {
      c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    }
    table[n] = c >>> 0;
  }
  return table;
})();

function crc32(bytes) {
  let crc = 0xffffffff;
  for (let i = 0; i < bytes.length; i += 1) {
    crc = CRC_TABLE[(crc ^ bytes[i]) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

// Strip the top-level folder segment so app.json lands at the archive root,
// which is what the import endpoint requires.
function archiveEntryName(relativePath) {
  const parts = String(relativePath).split("/");
  if (parts.length <= 1) return parts[0] || "";
  return parts.slice(1).join("/");
}

function shouldSkipEntry(name) {
  if (!name) return true;
  return name.split("/").some((part) => part === ".DS_Store" || part === "__MACOSX");
}

// Build a store-only (uncompressed) ZIP. Go's archive/zip reads stored entries,
// so no client-side compression dependency is needed.
async function buildHankAppFromFiles(fileList) {
  const encoder = new TextEncoder();
  const entries = [];
  for (const file of fileList) {
    const name = archiveEntryName(file.webkitRelativePath || file.name);
    if (shouldSkipEntry(name)) continue;
    const data = new Uint8Array(await file.arrayBuffer());
    entries.push({ name: encoder.encode(name), data, crc: crc32(data) });
  }
  if (!entries.some((entry) => new TextDecoder().decode(entry.name) === "app.json")) {
    throw new Error("Folder must contain app.json at its top level.");
  }

  const chunks = [];
  const central = [];
  let offset = 0;

  for (const entry of entries) {
    const header = new Uint8Array(30 + entry.name.length);
    const dv = new DataView(header.buffer);
    dv.setUint32(0, 0x04034b50, true);
    dv.setUint16(4, 20, true); // version needed
    dv.setUint16(6, 0, true); // flags
    dv.setUint16(8, 0, true); // method: store
    dv.setUint16(10, 0, true); // mod time
    dv.setUint16(12, 0x21, true); // mod date (1980-01-01)
    dv.setUint32(14, entry.crc, true);
    dv.setUint32(18, entry.data.length, true);
    dv.setUint32(22, entry.data.length, true);
    dv.setUint16(26, entry.name.length, true);
    dv.setUint16(28, 0, true);
    header.set(entry.name, 30);
    chunks.push(header, entry.data);
    entry.offset = offset;
    offset += header.length + entry.data.length;
  }

  for (const entry of entries) {
    const record = new Uint8Array(46 + entry.name.length);
    const dv = new DataView(record.buffer);
    dv.setUint32(0, 0x02014b50, true);
    dv.setUint16(4, 20, true); // version made by
    dv.setUint16(6, 20, true); // version needed
    dv.setUint16(8, 0, true);
    dv.setUint16(10, 0, true);
    dv.setUint16(12, 0, true);
    dv.setUint16(14, 0x21, true);
    dv.setUint32(16, entry.crc, true);
    dv.setUint32(20, entry.data.length, true);
    dv.setUint32(24, entry.data.length, true);
    dv.setUint16(28, entry.name.length, true);
    dv.setUint32(42, entry.offset, true);
    record.set(entry.name, 46);
    central.push(record);
  }

  const centralSize = central.reduce((sum, record) => sum + record.length, 0);
  const end = new Uint8Array(22);
  const endView = new DataView(end.buffer);
  endView.setUint32(0, 0x06054b50, true);
  endView.setUint16(8, entries.length, true);
  endView.setUint16(10, entries.length, true);
  endView.setUint32(12, centralSize, true);
  endView.setUint32(16, offset, true);

  return new Blob([...chunks, ...central, end], { type: "application/vnd.hank.app-package" });
}

async function resolvePackageUpload() {
  const file = els.packageInput.files?.[0];
  if (file) {
    return { blob: file, filename: file.name };
  }
  const folderFiles = els.folderInput?.files;
  if (folderFiles?.length) {
    const top = (folderFiles[0].webkitRelativePath || "").split("/")[0] || "app";
    const blob = await buildHankAppFromFiles(folderFiles);
    return { blob, filename: `${top}.hankapp` };
  }
  return null;
}

async function previewPackage(event) {
  event.preventDefault();
  let upload;
  try {
    upload = await resolvePackageUpload();
  } catch (error) {
    showToast(error.message, true);
    return;
  }
  if (!upload) {
    showToast("Choose a .hankapp package or an app folder.", true);
    return;
  }
  const file = upload.blob;
  const formData = new FormData();
  formData.set("package", file, upload.filename);
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
    closeInstallDialog();
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
    if (els.folderInput) els.folderInput.value = "";
    renderPreview();
    await loadApps();
    showToast("App installed.");
  } catch (error) {
    showToast(error.message, true);
  }
}

function openInstallDialog() {
  if (!els.installDialog) return;
  if (typeof els.installDialog.showModal === "function") {
    els.installDialog.showModal();
  } else {
    els.installDialog.hidden = false;
  }
}

function closeInstallDialog() {
  if (!els.installDialog) return;
  if (typeof els.installDialog.close === "function" && els.installDialog.open) {
    els.installDialog.close();
  } else {
    els.installDialog.hidden = true;
  }
}

async function saveSelectedApp(event) {
  event.preventDefault();
  const app = state.selectedApp;
  if (!app) return;
  const settingsFields = sortedSettingsFields(app);
  const publicConfig = settingsFields.length ? collectSettingsConfig() : parseRawPublicConfig();
  if (!publicConfig) return;
  const secrets = {};
  els.secretFields.querySelectorAll("[data-secret-field]").forEach((input) => {
    const value = input.value.trim();
    if (value) {
      secrets[input.dataset.secretField] = value;
    }
  });
  try {
    const payload = { public_config: publicConfig, enable: els.appEnabled.checked, user_access: els.appUserAccess?.value || "admins_only" };
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

function parseRawPublicConfig() {
  try {
    return els.publicConfig.value.trim() ? JSON.parse(els.publicConfig.value) : {};
  } catch {
    showToast("Public config must be valid JSON.", true);
    return null;
  }
}

function collectSettingsConfig() {
  const config = {};
  els.secretFields.querySelectorAll("[data-config-field]").forEach((input) => {
    const key = input.dataset.configField;
    if (!key) return;
    if (input.type === "checkbox") {
      config[key] = input.checked;
    } else if (input.type === "number") {
      const value = input.value.trim();
      if (value !== "") config[key] = Number(value);
    } else if (input.tagName === "SELECT") {
      const value = input.value.trim();
      if (value === "") return;
      const optionType = input.selectedOptions?.[0]?.dataset?.optionType;
      config[key] = optionType === "number" ? Number(value) : value;
    } else {
      const value = input.value.trim();
      if (value !== "") config[key] = value;
    }
  });
  return config;
}

async function toggleApp(app) {
  try {
    await api(`/v1/home/apps/${encodeURIComponent(appID(app))}/config`, {
      method: "PUT",
      body: JSON.stringify({ enable: !appEnabled(app), user_access: appUserAccess(app) }),
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
    await loadFileSources();
    await loadApps();
  } catch (_) {
    window.location.replace("/");
  }
}

els.packageForm.addEventListener("submit", previewPackage);
els.installOpenButton.addEventListener("click", openInstallDialog);
els.installCancelButton.addEventListener("click", closeInstallDialog);
els.installButton.addEventListener("click", installPreview);
els.configForm.addEventListener("submit", saveSelectedApp);
els.logoutButton.addEventListener("click", logout);
els.installedAppSelect.addEventListener("change", () => {
  state.selectedApp = state.apps.find((app) => appID(app) === els.installedAppSelect.value) || null;
  renderSelectedAppPanel();
  els.configPanel.hidden = true;
});
els.selectedAppPanel.addEventListener("click", (event) => {
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
