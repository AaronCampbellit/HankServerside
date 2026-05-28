const state = {
  user: null,
  homes: [],
  members: [],
  profiles: [],
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  homeSelect: document.getElementById("home-select"),
  homeRole: document.getElementById("home-role"),
  homeMeta: document.getElementById("home-meta"),
  profilesSummary: document.getElementById("profiles-summary"),
  haForm: document.getElementById("ha-form"),
  haPublicConfig: document.getElementById("ha-public-config"),
  haSecrets: document.getElementById("ha-secrets"),
  haPersist: document.getElementById("ha-persist"),
  settingsRole: document.getElementById("settings-role"),
  smbForm: document.getElementById("smb-form"),
  smbHost: document.getElementById("smb-host"),
  smbShare: document.getElementById("smb-share"),
  smbDomain: document.getElementById("smb-domain"),
  smbUsername: document.getElementById("smb-username"),
  smbPassword: document.getElementById("smb-password"),
  smbPersist: document.getElementById("smb-persist"),
  toast: document.getElementById("toast"),
};

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  const csrf = document.cookie.split("; ").find((part) => part.startsWith("hank_remote_csrf="))?.split("=")[1];
  if (csrf && !headers.has("X-Hank-CSRF-Token")) {
    headers.set("X-Hank-CSRF-Token", decodeURIComponent(csrf));
  }
  if (!headers.has("Content-Type") && options.body && !(options.body instanceof Blob)) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(path, { ...options, headers });
  const contentType = response.headers.get("Content-Type") || "";
  const isJSON = contentType.includes("application/json");
  const payload = isJSON ? await response.json() : await response.text();
  if (!response.ok) {
    const message = typeof payload === "string" ? payload : payload.error || payload.message || response.statusText;
    throw new Error(message);
  }
  return payload;
}

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
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : "Never";
}

function prettyJSON(value) {
  if (value === null || value === undefined || value === "") {
    return "";
  }
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value, null, 2);
}

function parseOptionalJSON(value, fieldName) {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return null;
  }
  try {
    return JSON.parse(trimmed);
  } catch {
    throw new Error(`${fieldName} must be valid JSON.`);
  }
}

function normalizeSMBHostInput(value) {
  let host = String(value || "").trim();
  host = host.replace(/^[a-z][a-z0-9+.-]*:\/\//i, "");
  host = host.replace(/^\\\\+/, "");
  host = host.replace(/^\/+/, "");
  host = host.split(/[/?#]/)[0] || host;
  return host.trim();
}

function selectedHomeID() {
  return els.homeSelect.value;
}

function selectedHome() {
  return state.homes.find((home) => home.id === selectedHomeID()) || null;
}

function currentMembership() {
  return state.members.find((member) => member.user_id === state.user?.id) || null;
}

function isAdmin() {
  return currentMembership()?.role === "admin";
}

function syncURL(homeID) {
  const url = new URL(window.location.href);
  if (homeID) {
    url.searchParams.set("home_id", homeID);
  } else {
    url.searchParams.delete("home_id");
  }
  window.history.replaceState({}, "", url);
}

function profileByType(serviceType) {
  return state.profiles.find((profile) => profile.service_type === serviceType) || null;
}

function profileConfig(profile) {
  const config = profile?.public_config_json || profile?.public_config || {};
  if (typeof config === "string") {
    try {
      return JSON.parse(config);
    } catch {
      return {};
    }
  }
  return config || {};
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function renderHomes() {
  els.homeSelect.innerHTML = "";
  if (!state.homes.length) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No home yet";
    els.homeSelect.appendChild(option);
    els.homeRole.textContent = "No home";
    els.homeMeta.textContent = "Create or join a home first.";
    return;
  }

  const requestedHomeID = new URLSearchParams(window.location.search).get("home_id");
  const selected = state.homes.find((home) => home.id === requestedHomeID)?.id || state.homes[0].id;
  state.homes.forEach((home) => {
    const option = document.createElement("option");
    option.value = home.id;
    option.textContent = home.name;
    option.selected = home.id === selected;
    els.homeSelect.appendChild(option);
  });
  els.homeSelect.value = selected;
  syncURL(selected);
}

function renderSummary() {
  const home = selectedHome();
  const membership = currentMembership();
  const admin = isAdmin();
  els.homeRole.textContent = membership ? `Your role: ${membership.role}` : "No access";
  els.homeMeta.textContent = home ? `${home.name}` : "Choose a home to see its connections.";
  els.settingsRole.textContent = admin ? "Admin" : "View Only";
  els.haForm.querySelectorAll("input, textarea, button").forEach((element) => {
    element.disabled = !admin;
  });
  els.smbForm.querySelectorAll("input, button").forEach((element) => {
    element.disabled = !admin;
  });

  if (!home) {
    els.profilesSummary.className = "card-list empty-state";
    els.profilesSummary.textContent = "Choose a home to see its connections.";
    return;
  }
  if (!state.profiles.length) {
    els.profilesSummary.className = "card-list empty-state";
    els.profilesSummary.textContent = admin
      ? "No connections saved for this home yet."
      : "No connections are visible for this home yet.";
    return;
  }

  els.profilesSummary.className = "card-list";
  els.profilesSummary.innerHTML = "";
  state.profiles.forEach((profile) => {
    const card = document.createElement("article");
    card.className = "card split-card";
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(profile.service_type)}</div>
          <div class="meta">Updated ${formatDate(profile.updated_at)}</div>
        </div>
        <span class="status-chip ${profile.status === "healthy" ? "" : "offline"}">${escapeHTML(profile.status || "unknown")}</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Applied Version</div><div>${escapeHTML(profile.applied_version)}</div></div>
        <div class="kv-row"><div class="kv-label">Secret Version</div><div>${escapeHTML(profile.secret_version)}</div></div>
        <div class="kv-row"><div class="kv-label">Last Backup</div><div>${escapeHTML(formatDate(profile.last_backup_at))}</div></div>
        <div class="kv-row"><div class="kv-label">Last Error</div><div>${escapeHTML(profile.last_error || "None")}</div></div>
      </div>
    `;
    els.profilesSummary.appendChild(card);
  });
}

function renderForms() {
  const homeAssistant = profileByType("homeassistant");
  const smb = profileByType("smb");
  const smbConfig = profileConfig(smb);
  els.haPublicConfig.value = prettyJSON(homeAssistant?.public_config_json);
  els.haSecrets.value = "";
  els.smbHost.value = smbConfig.host || "";
  els.smbShare.value = smbConfig.share || "";
  els.smbDomain.value = smbConfig.domain || "";
  els.smbUsername.value = smbConfig.username || "";
  els.smbPassword.value = "";
}

async function loadHomes() {
  try {
    const home = await api("/v1/home");
    state.homes = home ? [home] : [];
  } catch (_) {
    state.homes = [];
  }
  renderHomes();
}

async function loadMembers() {
  const home = selectedHome();
  if (!home) {
    state.members = [];
    renderSummary();
    return;
  }
  state.members = (await api("/v1/home/members")).members || [];
  renderSummary();
}

async function loadProfiles() {
  const home = selectedHome();
  if (!home) {
    state.profiles = [];
    renderSummary();
    renderForms();
    return;
  }
  state.profiles = (await api("/v1/home/service-profiles")).profiles || [];
  renderSummary();
  renderForms();
}

async function saveHomeAssistant(event) {
  event.preventDefault();
  if (!isAdmin()) {
    showToast("Only admins can change Home Assistant settings.", true);
    return;
  }
  try {
    const payload = {
      public_config: parseOptionalJSON(els.haPublicConfig.value, "Home Assistant address") ?? {},
      persist: els.haPersist.checked,
    };
    const secrets = parseOptionalJSON(els.haSecrets.value, "Home Assistant token");
    if (secrets !== null) {
      payload.secrets = secrets;
    }
    await api("/v1/home/service-profiles/homeassistant", {
      method: "PUT",
      body: JSON.stringify(payload),
    });
    els.haSecrets.value = "";
    await loadProfiles();
    showToast("Home Assistant settings saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function saveSMBSettings(event) {
  event.preventDefault();
  if (!isAdmin()) {
    showToast("Only admins can change file server settings.", true);
    return;
  }
  const publicConfig = {
    host: normalizeSMBHostInput(els.smbHost.value),
    share: els.smbShare.value.trim(),
    domain: els.smbDomain.value.trim(),
    username: els.smbUsername.value.trim(),
  };
  els.smbHost.value = publicConfig.host;
  const payload = {
    public_config: publicConfig,
    persist: els.smbPersist.checked,
  };
  if (els.smbPassword.value) {
    payload.secrets = { password: els.smbPassword.value };
  }
  try {
    await api("/v1/home/service-profiles/smb", {
      method: "PUT",
      body: JSON.stringify(payload),
    });
    els.smbPassword.value = "";
    await loadProfiles();
    showToast("File server settings saved.");
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
    await loadHomes();
    await loadMembers();
    await loadProfiles();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.homeSelect.addEventListener("change", async () => {
  syncURL(selectedHomeID());
  await loadMembers();
  await loadProfiles();
});
els.haForm.addEventListener("submit", saveHomeAssistant);
els.smbForm.addEventListener("submit", saveSMBSettings);

hydrate();
