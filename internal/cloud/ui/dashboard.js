const api = window.HankAPI.request;
const escapeHTML = window.HankUI.escapeHTML;

const state = {
  user: null,
  homes: [],
  agents: [],
  sync: null,
  storage: null,
  storageError: "",
  setup: null,
  quickLinks: [],
  canRestartAgent: false,
  agentRestartInProgress: false,
  tokensByHome: new Map(),
  refreshTimer: 0,
  quickLinksRefreshTimer: 0,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  homeList: document.getElementById("home-list"),
  homePanelTitle: document.getElementById("home-panel-title"),
  homeCount: document.getElementById("home-count"),
  agentList: document.getElementById("agent-list"),
  agentCount: document.getElementById("agent-count"),
  quickLinksCount: document.getElementById("quick-links-count"),
  quickLinksList: document.getElementById("quick-links-list"),
  setupStatus: document.getElementById("setup-status"),
  setupChecklist: document.getElementById("setup-checklist"),
  setupPanel: document.getElementById("setup-checklist-panel"),
  syncHealthPill: document.getElementById("sync-health-pill"),
  syncSummary: document.getElementById("sync-summary"),
  toast: document.getElementById("toast"),
};

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : "Never";
}

function showToast(message, isError = false) {
  window.HankUI.showToast(els.toast, message, isError);
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
  renderSetupChecklist();
}

function renderHomes() {
  els.homePanelTitle.textContent = "Current Home";
  els.homeCount.textContent = `${state.homes.length} home${state.homes.length === 1 ? "" : "s"}`;
  if (!state.homes.length) {
    els.homeList.className = "card-list empty-state";
    els.homeList.textContent = "No home has been created yet.";
    renderSetupChecklist();
    return;
  }

  els.homeList.className = "card-list";
  els.homeList.innerHTML = "";
  state.homes.forEach((home) => {
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
  });
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
      <div class="item-actions">
        <button type="button" class="secondary" data-agent-action="restart" ${!isOnline || !state.canRestartAgent || state.agentRestartInProgress ? "disabled" : ""}>${state.agentRestartInProgress ? "Restarting..." : "Restart Connector"}</button>
      </div>
    `;
    els.agentList.appendChild(card);
  });
  renderSetupChecklist();
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

function quickLinkHost(value) {
  try {
    return new URL(value).host;
  } catch (_) {
    return value || "";
  }
}

function quickLinkStatusInfo(link) {
  const status = String(link.status || "unchecked").toLowerCase();
  if (!link.health_check_enabled || status === "disabled") {
    return { className: "disabled", label: "Not checked", detail: "Status checks off" };
  }
  if (status === "up") {
    return { className: "up", label: "Up", detail: "Up" };
  }
  if (status === "down") {
    return { className: "down", label: "Review", detail: link.last_error || "Review" };
  }
  return { className: "unchecked", label: "Unchecked", detail: "Waiting for first check" };
}

function renderQuickLinks() {
  els.quickLinksCount.textContent = `${state.quickLinks.length} link${state.quickLinks.length === 1 ? "" : "s"}`;
  if (!state.quickLinks.length) {
    els.quickLinksList.className = "quick-links-grid empty-state";
    els.quickLinksList.innerHTML = `
      <div class="quick-links-empty-copy">
        <strong>No quick links saved.</strong>
        <span>Add shared links from Settings.</span>
      </div>
    `;
    return;
  }

  els.quickLinksList.className = "quick-links-grid";
  els.quickLinksList.innerHTML = state.quickLinks.map((link) => {
    const status = quickLinkStatusInfo(link);
    const description = link.description || quickLinkHost(link.url);
    return `
      <article class="quick-link-card ${escapeHTML(status.className)}">
        <a class="quick-link-main" href="${escapeHTML(link.url)}" target="_blank" rel="noreferrer">
          <span class="quick-link-status-dot" title="${escapeHTML(status.label)}"></span>
          <span class="quick-link-copy">
            <span class="quick-link-title">${escapeHTML(link.title || quickLinkHost(link.url))}</span>
            <span class="meta">${escapeHTML(description)}</span>
            <span class="quick-link-status-text">${escapeHTML(status.detail)}</span>
          </span>
        </a>
      </article>
    `;
  }).join("");
}

function renderSync() {
  const notes = state.sync?.notes || {};
  const profiles = state.sync?.profiles || {};
  const profileValues = Object.values(profiles);
  const agentOnline = state.agents.some((agent) => String(agent.status || "").toLowerCase() === "online");
  const backup = state.storage?.backup || {};
  const restore = state.storage?.restore || {};
  const checksum = state.storage?.checksum || {};
  const failures = state.storage?.failures || [];
  const activeTasks = (state.storage?.tasks || []).filter((task) => ["queued", "running"].includes(String(task.status || "").toLowerCase()));
  const hasStorageFailure = Boolean(state.storageError || backup.failure_count || restore.last_failed_at || checksum.corruption_detected || checksum.failure_count || failures.length);
  const notesHealthy = String(notes.status || "").toLowerCase() === "healthy" && !notes.last_error;
  const connectionErrors = profileValues.filter((profile) => profile.last_error);
  const profileCount = profileValues.length;
  const backupsVerified = Boolean(backup.last_successful_at && restore.last_test_at);
  const healthItems = [
    {
      title: "Cloud",
      status: state.user ? "healthy" : "needs_attention",
      value: state.user ? "Signed in" : "Sign in required",
      detail: state.user ? "Admin account session is active." : "The dashboard could not confirm an active Hank Remote account.",
      issue: state.user ? "" : "Sign in again, then reload the dashboard.",
    },
    {
      title: "Connector",
      status: agentOnline ? "healthy" : "needs_attention",
      value: agentOnline ? "Online" : "Offline",
      detail: agentOnline ? "The home connector is connected to Hank Remote." : "The home connector is not currently online.",
      issue: agentOnline ? "" : "Start or restart the agent container, then check the connector logs if it stays offline.",
    },
    {
      title: "Notes",
      status: notesHealthy ? "healthy" : notes.last_error ? "needs_attention" : "warning",
      value: notes.status || "Unknown",
      detail: notes.last_successful_sync_at ? `Last successful sync: ${formatDate(notes.last_successful_sync_at)}.` : "No successful notes sync has been recorded yet.",
      issue: notes.last_error || (!notes.last_successful_sync_at ? "Open Notes or check the agent if sync does not start." : ""),
    },
    {
      title: "Connections",
      status: connectionErrors.length ? "needs_attention" : profileCount ? "healthy" : "warning",
      value: `${profileCount} saved`,
      detail: profileCount ? "Saved connection profiles are available for the connector." : "No Home Assistant or file server connection profile is saved yet.",
      issue: connectionErrors.length ? connectionErrors.map((profile) => `${profile.service_type || "Connection"}: ${profile.last_error}`).join(" ") : (!profileCount ? "Add Home Assistant or file server settings in Settings > Connections." : ""),
    },
    {
      title: "Backups",
      status: hasStorageFailure ? "needs_attention" : backupsVerified ? "healthy" : "warning",
      value: backupsVerified ? "Verified" : "Not verified",
      detail: backup.last_successful_at ? `Last backup: ${formatDate(backup.last_successful_at)}. Restore test: ${formatDate(restore.last_test_at)}.` : "No successful database backup has been recorded yet.",
      issue: hasStorageFailure ? (state.storageError || failures[0]?.message || "Backup, restore, or checksum failures need review.") : (!backupsVerified ? "Run a backup and restore test from Settings > Backups." : ""),
    },
    {
      title: "Database",
      status: state.storageError ? "needs_attention" : activeTasks.length ? "warning" : "healthy",
      value: state.storageError ? "Unavailable" : activeTasks.length ? "Working" : "Ready",
      detail: activeTasks.length ? activeTasks.map((task) => task.step || task.message || task.operation).join(" ") : "Storage operations are reachable.",
      issue: state.storageError || "",
    },
  ];
  const hasIssue = healthItems.some((item) => item.status === "needs_attention");
  const hasWarning = healthItems.some((item) => item.status === "warning");
  els.syncHealthPill.textContent = hasIssue ? "Needs Review" : hasWarning ? "Watch" : "Healthy";
  els.syncHealthPill.className = hasIssue ? "status-chip offline" : hasWarning ? "status-chip warning" : "status-chip";
  els.syncSummary.innerHTML = healthItems.map((item) => `
    <details class="health-check ${item.status}">
      <summary>
        <span class="health-check-title">${escapeHTML(item.title)}</span>
        <span class="health-check-value">${escapeHTML(item.value)}</span>
        <span class="status-chip ${item.status === "needs_attention" ? "offline" : item.status === "warning" ? "warning" : ""}">${item.status === "healthy" ? "Healthy" : item.status === "warning" ? "Watch" : "Review"}</span>
      </summary>
      <div class="health-check-detail">
        <p>${escapeHTML(item.detail)}</p>
        ${item.issue ? `<p class="health-check-issue">${escapeHTML(item.issue)}</p>` : `<p>No issue found.</p>`}
      </div>
    </details>
  `).join("");
  renderSetupChecklist();
}

function renderSetupChecklist() {
  if (!els.setupChecklist || !els.setupStatus) {
    return;
  }
  if (els.setupPanel) {
    els.setupPanel.hidden = state.setup?.first_setup_visible === false;
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
      href: "/dashboard/settings/home",
      action: "Create",
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
      href: "/dashboard/settings/connections",
      action: "Open",
    },
    {
      title: "Backups",
      detail: backupsVerified ? "Backup and restore test complete" : "Run the first backup and restore test",
      done: backupsVerified,
      href: "/dashboard/settings/backups",
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

function syncQuickLinksRefresh() {
  if (state.quickLinksRefreshTimer) {
    window.clearInterval(state.quickLinksRefreshTimer);
    state.quickLinksRefreshTimer = 0;
  }
  state.quickLinksRefreshTimer = window.setInterval(() => {
    refreshQuickLinks().catch(() => {});
  }, 60000);
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

async function loadQuickLinks() {
  try {
    const payload = await api("/v1/home/quick-links");
    state.quickLinks = payload.links || [];
  } catch (_) {
    state.quickLinks = [];
  }
  renderQuickLinks();
}

async function refreshQuickLinks() {
  const payload = await api("/v1/home/quick-links/checks", { method: "POST", body: JSON.stringify({}) });
  state.quickLinks = payload.links || [];
  renderQuickLinks();
  return payload;
}

async function loadTokens(homeID) {
  if (!homeID) {
    state.tokensByHome.clear();
    renderSetupChecklist();
    return;
  }
  try {
    state.tokensByHome.set(homeID, (await api("/v1/home/agent/tokens")).tokens || []);
  } catch (_) {
    state.tokensByHome.set(homeID, []);
  }
  renderSetupChecklist();
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
    state.storageError = "";
  } catch (error) {
    state.storage = null;
    state.storageError = error.message || "Storage status could not be loaded.";
  }
  renderSync();
  renderSetupChecklist();
}

async function loadSetupStatus() {
  try {
    state.setup = await api("/v1/home/setup-status");
  } catch (_) {
    state.setup = null;
  }
  renderSetupChecklist();
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
    await Promise.all([loadHomes(), loadAgents(), loadQuickLinks(), loadSync(), loadStorageStatus(), loadSetupStatus()]);
    await loadTokens(state.homes[0]?.id || "");
    syncAutoRefresh();
    syncQuickLinksRefresh();
    refreshQuickLinks().catch((error) => loadQuickLinks().catch(() => {
      showToast(error.message || "Quick link status checks could not be refreshed.", true);
    }));
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.agentList.addEventListener("click", (event) => {
  const button = event.target.closest("[data-agent-action]");
  if (!button) return;
  if (button.dataset.agentAction === "restart") {
    restartAgent().catch((error) => showToast(error.message, true));
  }
});

hydrate();
