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

function selectedHomeID() {
  return els.homeSelect.value;
}

function selectedHome() {
  return state.homes.find((home) => home.id === selectedHomeID()) || null;
}

function currentMembership() {
  return state.members.find((member) => member.user_id === state.user?.id) || null;
}

function isOwner() {
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
  els.homeRole.textContent = membership ? `Your role: ${membership.role}` : "No access";
  els.homeMeta.textContent = home ? `${home.name}` : "Choose a home to see its connections.";

  const owner = isOwner();
  els.haForm.querySelectorAll("input, textarea, button").forEach((element) => {
    element.disabled = !owner;
  });

  if (!home) {
    els.profilesSummary.className = "card-list empty-state";
    els.profilesSummary.textContent = "Choose a home to see its connections.";
    return;
  }

  if (!state.profiles.length) {
    els.profilesSummary.className = "card-list empty-state";
    els.profilesSummary.textContent = owner
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
  els.haPublicConfig.value = prettyJSON(homeAssistant?.public_config_json);
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

async function saveProfile(serviceType) {
  const home = selectedHome();
  if (!home) {
    showToast("Choose a home first.", true);
    return;
  }

  try {
    const payload = {
      public_config: parseOptionalJSON(els.haPublicConfig.value, "Home Assistant public config") ?? {},
      persist: els.haPersist.checked,
    };
    const secrets = parseOptionalJSON(els.haSecrets.value, "Home Assistant secrets");
    if (secrets !== null) {
      payload.secrets = secrets;
    }

    await api(`/v1/home/service-profiles/${encodeURIComponent(serviceType)}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });

    els.haSecrets.value = "";
    await loadProfiles();
    showToast(`${serviceType} connection saved.`);
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
els.haForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  await saveProfile("homeassistant");
});

hydrate();
