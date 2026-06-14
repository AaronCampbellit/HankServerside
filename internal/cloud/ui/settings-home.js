const api = window.HankAPI.request;
const escapeHTML = window.HankUI.escapeHTML;

const state = {
  user: null,
  homes: [],
  agents: [],
  canRestartAgent: false,
  agentRestartInProgress: false,
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
  tokenForm: document.getElementById("token-form"),
  tokenHome: document.getElementById("token-home"),
  tokenAgentID: document.getElementById("token-agent-id"),
  tokenName: document.getElementById("token-name"),
  tokenExpiry: document.getElementById("token-expiry"),
  tokenCreated: document.getElementById("token-created"),
  tokenList: document.getElementById("token-list"),
  toast: document.getElementById("toast"),
};

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : "Never";
}

function showToast(message, isError = false) {
  window.HankUI.showToast(els.toast, message, isError);
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
    "HANK_REMOTE_AGENT_FILES_ROOT=/srv/hank/files",
    "HANK_REMOTE_AGENT_NOTES_ROOT=/srv/hank/notes",
    "",
    "# Configure Home Assistant, SMB shares, and media credentials from dashboard Settings.",
  ].join("\n");
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
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
      </div>
      <div class="item-actions">
        <a class="ops-card manage-link" href="/dashboard/settings/people">Manage People</a>
        <a class="ops-card manage-link" href="/dashboard/settings/connections">Connections</a>
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
}

function renderAgents() {
  const online = state.agents.filter((agent) => String(agent.status || "").toLowerCase() === "online").length;
  els.agentCount.textContent = `${online} online`;
  if (!state.agents.length) {
    els.agentList.className = "card-list empty-state";
    els.agentList.textContent = "The home connector has not been set up yet.";
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
      <div class="item-actions">
        <button type="button" class="secondary" data-agent-action="restart" ${!isOnline || !state.canRestartAgent || state.agentRestartInProgress ? "disabled" : ""}>${state.agentRestartInProgress ? "Restarting..." : "Restart Connector"}</button>
      </div>
    `;
    els.agentList.appendChild(card);
  });
}

function renderTokens(homeID) {
  const tokens = state.tokensByHome.get(homeID) || [];
  if (!homeID) {
    els.tokenList.className = "card-list empty-state";
    els.tokenList.textContent = "Choose a home to see setup files.";
    return;
  }
  if (!tokens.length) {
    els.tokenList.className = "card-list empty-state";
    els.tokenList.textContent = "No setup files have been created for this home yet.";
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
    const button = document.createElement("button");
    button.type = "button";
    button.className = "danger-link";
    button.textContent = revoked ? "Remove setup file" : "Disable setup file";
    button.addEventListener("click", () => {
      const action = revoked ? removeToken : revokeToken;
      action(homeID, token.id).catch((error) => showToast(error.message, true));
    });
    wrapper.appendChild(button);
    els.tokenList.appendChild(wrapper);
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
  state.canRestartAgent = Boolean(payload.can_restart);
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

async function restartAgent() {
  if (state.agentRestartInProgress || !state.canRestartAgent) return;
  const agent = state.agents[0] || {};
  if (String(agent.status || "").toLowerCase() !== "online") {
    showToast("The home connector is offline. Restart it from the host, then refresh status.", true);
    return;
  }
  if (!window.confirm("Restart the home connector now? Home Assistant, files, media, and notes may be unavailable for a few seconds.")) {
    return;
  }
  state.agentRestartInProgress = true;
  renderAgents();
  try {
    await api("/v1/home/agent/restart", { method: "POST" });
    showToast("Connector restart requested.");
    window.setTimeout(() => loadAgents().catch((error) => showToast(error.message, true)), 3000);
  } catch (error) {
    showToast(error.message, true);
  } finally {
    state.agentRestartInProgress = false;
    renderAgents();
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
  await api(`/v1/home/agent/tokens/${encodeURIComponent(tokenID)}`, { method: "DELETE" });
  await loadTokens(homeID);
  showToast("Setup file disabled.");
}

async function removeToken(homeID, tokenID) {
  await api(`/v1/home/agent/tokens/${encodeURIComponent(tokenID)}?purge=1`, { method: "DELETE" });
  await loadTokens(homeID);
  showToast("Setup file removed.");
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
    await Promise.all([loadHomes(), loadAgents()]);
    await loadTokens(els.tokenHome.value || state.homes[0]?.id || "");
    syncAutoRefresh();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.homeForm.addEventListener("submit", createHome);
els.agentList.addEventListener("click", (event) => {
  const button = event.target.closest("[data-agent-action]");
  if (!button) return;
  if (button.dataset.agentAction === "restart") {
    restartAgent().catch((error) => showToast(error.message, true));
  }
});
els.tokenForm.addEventListener("submit", (event) => issueToken(event).catch((error) => showToast(error.message, true)));
els.tokenCreated.addEventListener("click", async (event) => {
  if (!event.target.closest("[data-copy-agent-env]") || !state.lastAgentEnvFile) {
    return;
  }
  if (await window.HankUI.copyText(state.lastAgentEnvFile)) {
    showToast(".env.agent copied.");
  } else {
    showToast("Clipboard is blocked. Select and copy the generated .env.agent setup file.", true);
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
