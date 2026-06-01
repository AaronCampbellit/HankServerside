const state = {
  user: null,
  homes: [],
  agents: [],
  sync: null,
  storage: null,
  tokensByHome: new Map(),
  refreshTimer: 0,
  lastAgentEnvFile: "",
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  homeForm: document.getElementById("home-form"),
  homeName: document.getElementById("home-name"),
  homeList: document.getElementById("home-list"),
  homeCount: document.getElementById("home-count"),
  agentList: document.getElementById("agent-list"),
  agentCount: document.getElementById("agent-count"),
  setupStatus: document.getElementById("setup-status"),
  setupChecklist: document.getElementById("setup-checklist"),
  tokenForm: document.getElementById("token-form"),
  tokenHome: document.getElementById("token-home"),
  tokenAgentID: document.getElementById("token-agent-id"),
  tokenName: document.getElementById("token-name"),
  tokenExpiry: document.getElementById("token-expiry"),
  tokenCreated: document.getElementById("token-created"),
  tokenList: document.getElementById("token-list"),
  syncHealthPill: document.getElementById("sync-health-pill"),
  syncSummary: document.getElementById("sync-summary"),
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

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : "Never";
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

function dotenvValue(value) {
  return `"${String(value || "").replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

function selectedHome() {
  return state.homes.find((home) => home.id === els.tokenHome.value) || state.homes[0] || null;
}

function agentIDFromHomeName(homeName) {
  const slug = String(homeName || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || "home-agent";
}

function agentDisplayNameFromHomeName(homeName) {
  const name = String(homeName || "").trim();
  return name ? `${name} Agent` : "Home Agent";
}

function agentEnvFile(payload, home) {
  return [
    "HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent",
    `HANK_REMOTE_AGENT_ID=${dotenvValue(payload.agent_id || agentIDFromHomeName(home?.name))}`,
    `HANK_REMOTE_AGENT_TOKEN=${dotenvValue(payload.token)}`,
    `HANK_REMOTE_AGENT_HOME_NAME=${dotenvValue(home?.name || "Home")}`,
    "HANK_REMOTE_AGENT_CONFIG_PATH=/app/.env.agent",
    "",
    "# Optional Home Assistant connection. Configure later from Settings > Connections.",
    "HANK_REMOTE_HA_BASE_URL=",
    "HANK_REMOTE_HA_TOKEN=",
    "HANK_REMOTE_HA_TIMEOUT_SECONDS=10",
    "",
    "# Optional SMB connection. Leave blank to use the Docker files volume.",
    "HANK_REMOTE_SMB_HOST=",
    "HANK_REMOTE_SMB_SHARE=",
    "HANK_REMOTE_SMB_USERNAME=",
    "HANK_REMOTE_SMB_PASSWORD=",
    "HANK_REMOTE_SMB_DOMAIN=",
    "HANK_REMOTE_SMB_SHARES_JSON=",
    "",
    "HANK_REMOTE_AGENT_FILES_ROOT=/srv/hank/files",
    "HANK_REMOTE_AGENT_NOTES_ROOT=/srv/hank/notes",
    "",
    "# Optional media download workflow. Credentials can be edited later from Settings > AI.",
    "HANK_REMOTE_MEDIA_GRAMATON_ENABLED=false",
    "HANK_REMOTE_MEDIA_GRAMATON_BASE_URL=https://gramaton.io",
    "HANK_REMOTE_MEDIA_GRAMATON_USERNAME=",
    "HANK_REMOTE_MEDIA_GRAMATON_PASSWORD=",
    "HANK_REMOTE_MEDIA_DESTINATION_PATH=",
    "HANK_REMOTE_MEDIA_REQUIRE_CONFIRMATION=true",
  ].join("\n");
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
  renderSetupChecklist();
}

function renderHomes() {
  els.homeCount.textContent = `${state.homes.length} home${state.homes.length === 1 ? "" : "s"}`;
  els.tokenHome.innerHTML = "";
  if (!state.homes.length) {
    els.homeList.className = "card-list empty-state";
    els.homeList.textContent = "No home has been created yet.";
    const option = document.createElement("option");
    option.textContent = "No home yet";
    option.value = "";
    els.tokenHome.appendChild(option);
    renderSetupChecklist();
    return;
  }

  els.homeName.value = state.homes[0]?.name || "";
  els.homeList.className = "card-list";
  els.homeList.innerHTML = "";
  state.homes.forEach((home, index) => {
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(home.name)}</div>
          <div class="meta">This is the home the Hank app connects to.</div>
        </div>
        <span class="pill">Created ${formatDate(home.created_at)}</span>
      </div>
      <div class="item-actions">
        <a class="ops-card manage-link" href="/dashboard/settings#people">Manage People</a>
        <a class="ops-card manage-link" href="/dashboard/settings#connections">Connections</a>
      </div>
    `;
    els.homeList.appendChild(card);
    const option = document.createElement("option");
    option.value = home.id;
    option.textContent = home.name;
    if (index === 0) option.selected = true;
    els.tokenHome.appendChild(option);
  });
  syncTokenDefaults();
  renderSetupChecklist();
}

function renderAgents() {
  const online = state.agents.filter((agent) => String(agent.status || "").toLowerCase() === "online").length;
  els.agentCount.textContent = `${online} online`;
  if (!state.agents.length) {
    els.agentList.className = "card-list empty-state";
    els.agentList.textContent = "The home connector has not been set up yet.";
    renderSetupChecklist();
    return;
  }
  els.agentList.className = "card-list";
  els.agentList.innerHTML = "";
  state.agents.forEach((agent) => {
    const isOnline = String(agent.status || "").toLowerCase() === "online";
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(agent.name || agent.agent_id || agent.id)}</div>
          <div class="meta">${escapeHTML(agent.home_name || "Home connector")}</div>
        </div>
        <span class="status-chip ${isOnline ? "" : "offline"}">${isOnline ? "Online" : "Offline"}</span>
      </div>
      <div class="meta">Connector ID: ${escapeHTML(agent.agent_id || agent.id || "unknown")}</div>
      <div class="meta">Last online: ${formatDate(agent.last_seen_at)}</div>
    `;
    els.agentList.appendChild(card);
  });
  renderSetupChecklist();
}

function renderTokens(homeID) {
  const tokens = state.tokensByHome.get(homeID) || [];
  if (!homeID) {
    els.tokenList.className = "card-list empty-state";
    els.tokenList.textContent = "Choose a home to see setup files.";
    renderSetupChecklist();
    return;
  }
  if (!tokens.length) {
    els.tokenList.className = "card-list empty-state";
    els.tokenList.textContent = "No setup files have been created for this home yet.";
    renderSetupChecklist();
    return;
  }
  els.tokenList.className = "card-list";
  els.tokenList.innerHTML = "";
  tokens.forEach((token) => {
    const revoked = Boolean(token.revoked_at);
    const wrapper = document.createElement("article");
    wrapper.className = "card";
    wrapper.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(token.agent_id)}</div>
          <div class="meta">Used by the home connector.</div>
        </div>
        <span class="status-chip ${revoked ? "revoked" : ""}">${revoked ? "Disabled" : "Active"}</span>
      </div>
      <div class="token-meta">Created: ${formatDate(token.created_at)}</div>
      <div class="token-meta">Expires: ${formatDate(token.expires_at)}</div>
      <div class="token-meta">Disabled: ${formatDate(token.revoked_at)}</div>
    `;
    if (!revoked) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "danger-link";
      button.textContent = "Disable setup file";
      button.addEventListener("click", () => revokeToken(homeID, token.id));
      wrapper.appendChild(button);
    } else {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "danger-link";
      button.textContent = "Remove setup file";
      button.addEventListener("click", () => removeToken(homeID, token.id));
      wrapper.appendChild(button);
    }
    els.tokenList.appendChild(wrapper);
  });
  renderSetupChecklist();
}

function renderSync() {
  const notes = state.sync?.notes || {};
  const profiles = state.sync?.profiles || {};
  const hasError = notes.last_error || Object.values(profiles).some((profile) => profile.last_error);
  els.syncHealthPill.textContent = hasError ? "Needs Review" : "Ready";
  els.syncHealthPill.className = hasError ? "status-chip offline" : "status-chip";
  const profileCount = Object.keys(profiles).length;
  els.syncSummary.innerHTML = [
    ["Notes", notes.status || "unknown"],
    ["Last Successful Sync", formatDate(notes.last_successful_sync_at)],
    ["Saved Connections", profileCount],
    ["Last Error", notes.last_error || "None"],
  ].map(([label, value]) => `
    <div class="kv-row">
      <div class="kv-label">${escapeHTML(label)}</div>
      <div>${escapeHTML(value)}</div>
    </div>
  `).join("");
  renderSetupChecklist();
}

function renderSetupChecklist() {
  if (!els.setupChecklist || !els.setupStatus) {
    return;
  }
  const home = state.homes[0] || null;
  const tokens = home ? (state.tokensByHome.get(home.id) || []) : [];
  const hasActiveToken = tokens.some((token) => !token.revoked_at);
  const agentOnline = state.agents.some((agent) => String(agent.status || "").toLowerCase() === "online");
  const profiles = state.sync?.profiles || {};
  const hasConnectionProfile = Object.values(profiles).some((profile) => Boolean(profile?.updated_at || profile?.last_backup_at || profile?.status));
  const backup = state.storage?.backup || {};
  const restore = state.storage?.restore || {};
  const backupsVerified = Boolean(backup.last_successful_at && restore.last_test_at);
  const items = [
    {
      title: "Cloud",
      detail: state.user ? "Admin account active" : "Register the first admin",
      done: Boolean(state.user),
      href: "/",
      action: "Open",
    },
    {
      title: "Setup File",
      detail: hasActiveToken ? "Connector file issued" : "Create the connector file",
      done: hasActiveToken,
      href: "#",
      action: "Create",
      onClick: () => document.querySelector(".setup-file-panel")?.setAttribute("open", ""),
    },
    {
      title: "Connector",
      detail: agentOnline ? "Home connector online" : "Start the agent container",
      done: agentOnline,
      href: "#",
      action: "Check",
      onClick: () => document.getElementById("agent-list")?.scrollIntoView({ behavior: "smooth", block: "center" }),
    },
    {
      title: "Connections",
      detail: hasConnectionProfile ? "Saved connection profile present" : "Add Home Assistant or SMB",
      done: hasConnectionProfile,
      href: "/dashboard/settings#connections",
      action: "Open",
    },
    {
      title: "Backups",
      detail: backupsVerified ? "Backup and restore test complete" : "Run the first backup and restore test",
      done: backupsVerified,
      href: "/dashboard/settings#backups",
      action: "Open",
    },
  ];
  const doneCount = items.filter((item) => item.done).length;
  const nextIndex = items.findIndex((item) => !item.done);
  els.setupStatus.textContent = `${doneCount}/${items.length} done`;
  els.setupStatus.className = doneCount >= 4 ? "pill" : "pill offline";
  els.setupChecklist.innerHTML = items.map((item, index) => `
    <div class="setup-checklist-item">
      <span class="status-chip ${item.done ? "" : "offline"}">${item.done ? "Done" : index === nextIndex ? "Next" : "Open"}</span>
      <div>
        <div class="setup-checklist-title">${escapeHTML(item.title)}</div>
        <div class="meta">${escapeHTML(item.detail)}</div>
      </div>
      <a class="ops-card manage-link" href="${escapeHTML(item.href)}" data-setup-index="${index}">${escapeHTML(item.action)}</a>
    </div>
  `).join("");
  els.setupChecklist.querySelectorAll("[data-setup-index]").forEach((link) => {
    const item = items[Number.parseInt(link.dataset.setupIndex || "0", 10)];
    if (!item?.onClick) {
      return;
    }
    link.addEventListener("click", (event) => {
      event.preventDefault();
      item.onClick();
    });
  });
}

function syncTokenDefaults() {
  const home = selectedHome();
  const homeName = home ? home.name : "";
  els.tokenAgentID.value = agentIDFromHomeName(homeName);
  els.tokenName.value = agentDisplayNameFromHomeName(homeName);
}

function syncAutoRefresh() {
  if (state.refreshTimer) {
    window.clearInterval(state.refreshTimer);
    state.refreshTimer = 0;
  }
  state.refreshTimer = window.setInterval(async () => {
    try {
      await loadAgents();
    } catch (_) {
    }
  }, 5000);
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

async function loadAgents() {
  const payload = await api("/v1/home/agent");
  state.agents = payload.agent ? [payload.agent] : [];
  renderAgents();
}

async function loadTokens(homeID) {
  if (!homeID) {
    renderTokens("");
    return;
  }
  state.tokensByHome.set(homeID, (await api("/v1/home/agent/tokens")).tokens || []);
  renderTokens(homeID);
}

async function loadSync() {
  try {
    state.sync = await api("/v1/home/sync");
  } catch (_) {
    state.sync = null;
  }
  renderSync();
}

async function loadStorageStatus() {
  try {
    state.storage = await api("/v1/home/storage/status");
  } catch (_) {
    state.storage = null;
  }
  renderSetupChecklist();
}

async function createHome(event) {
  event.preventDefault();
  try {
    const home = await api("/v1/home", { method: "PUT", body: JSON.stringify({ name: els.homeName.value.trim() }) });
    state.homes = home ? [home] : [];
    renderHomes();
    await loadTokens(home.id);
    showToast("Home updated.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function issueToken(event) {
  event.preventDefault();
  const homeID = els.tokenHome.value;
  if (!homeID) {
    showToast("Create a home first.", true);
    return;
  }
  const home = selectedHome();
  const payload = await api("/v1/home/agent/tokens", {
    method: "POST",
    body: JSON.stringify({
      agent_id: els.tokenAgentID.value.trim() || agentIDFromHomeName(home?.name),
      name: els.tokenName.value.trim() || agentDisplayNameFromHomeName(home?.name),
      expires_in_seconds: Number.parseInt(els.tokenExpiry.value || "0", 10) || 0,
    }),
  });
  const envFile = agentEnvFile(payload, home);
  state.lastAgentEnvFile = envFile;
  els.tokenCreated.hidden = false;
  els.tokenCreated.innerHTML = `
    <strong>Issued setup file for ${escapeHTML(payload.agent_name)}</strong>
    <div class="token-meta">Copy this into <code>.env.agent</code>. It is only shown once.</div>
    <button type="button" class="secondary" data-copy-agent-env>Copy .env.agent</button>
    <pre>${escapeHTML(envFile)}</pre>
    <div class="token-meta">Then start the home connector:</div>
    <code>pbpaste | ssh &lt;server-user&gt;@&lt;server-host&gt; 'cd /srv/hank-remote/HankServerside &amp;&amp; scripts/install-agent-env.sh'</code>
    <code>ssh &lt;server-user&gt;@&lt;server-host&gt; 'cd /srv/hank-remote/HankServerside &amp;&amp; scripts/doctor.sh'</code>`;
  await Promise.all([loadAgents(), loadTokens(homeID)]);
  showToast("Setup file created.");
}

async function revokeToken(homeID, tokenID) {
  try {
    await api(`/v1/home/agent/tokens/${encodeURIComponent(tokenID)}`, { method: "DELETE" });
    await loadTokens(homeID);
    showToast("Setup file disabled.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function removeToken(homeID, tokenID) {
  try {
    await api(`/v1/home/agent/tokens/${encodeURIComponent(tokenID)}?purge=1`, { method: "DELETE" });
    await loadTokens(homeID);
    showToast("Setup file removed.");
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
    await Promise.all([loadHomes(), loadAgents(), loadSync(), loadStorageStatus()]);
    await loadTokens(els.tokenHome.value || state.homes[0]?.id || "");
    syncAutoRefresh();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.homeForm.addEventListener("submit", createHome);
els.tokenForm.addEventListener("submit", (event) => issueToken(event).catch((error) => showToast(error.message, true)));
els.tokenCreated.addEventListener("click", async (event) => {
  if (!event.target.closest("[data-copy-agent-env]") || !state.lastAgentEnvFile) {
    return;
  }
  try {
    await navigator.clipboard.writeText(state.lastAgentEnvFile);
    showToast(".env.agent copied.");
  } catch (_) {
    showToast("Select and copy the generated .env.agent setup file.", true);
  }
});
els.tokenHome.addEventListener("change", async (event) => {
  els.tokenCreated.hidden = true;
  state.lastAgentEnvFile = "";
  const home = state.homes.find((item) => item.id === event.target.value);
  if (home) {
    els.tokenAgentID.value = agentIDFromHomeName(home.name);
    els.tokenName.value = agentDisplayNameFromHomeName(home.name);
  }
  await loadTokens(event.target.value);
});

hydrate();
