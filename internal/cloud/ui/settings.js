const api = window.HankAPI.request;

const state = {
  user: null,
  canViewBackups: false,
  activeTab: "",
  paneLoadID: 0,
};

const settingsTabs = Array.from(document.querySelectorAll("[data-settings-page-tab]"));
const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  toast: document.getElementById("toast"),
  paneRoot: document.getElementById("settings-pane-root"),
  paneLoading: document.getElementById("settings-pane-loading"),
};

const tabAliases = {
  links: "quick-links",
  quicklinks: "quick-links",
  backup: "backups",
  storage: "backups",
  openai: "ai",
  assistant: "ai",
  invitations: "join-home",
  invite: "join-home",
};
const paneDefinitions = {
  home: {
    path: "/dashboard?pane=1&embedded=1",
    script: "/assets/dashboard.js?v=20260601-health-panel",
  },
  "quick-links": {
    path: "/dashboard?pane=1&embedded=1#quick-links-panel",
    script: "/assets/dashboard.js?v=20260601-health-panel",
    afterMount: () => document.getElementById("quick-links-panel")?.scrollIntoView({ block: "start" }),
  },
  people: {
    path: "/dashboard/settings/people-pane?embedded=1",
    script: "/assets/home-users.js",
  },
  connections: {
    path: "/dashboard/settings/connections-pane?embedded=1",
    script: "/assets/settings-connections.js?v=20260601-hermes",
  },
  ai: {
    path: "/dashboard/settings/ai-pane?embedded=1",
    script: "/assets/assistant-settings.js",
  },
  backups: {
    path: "/dashboard/settings/backups-pane?embedded=1",
    script: "/assets/storage.js",
  },
  "join-home": {
    path: "/dashboard/settings/join-home-pane?embedded=1",
    script: "/assets/accept-invitation.js",
  },
};

function showToast(message, isError = false) {
  window.HankUI?.toast(message, { error: isError, target: els.toast });
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function normalizeTab(value) {
  const cleaned = String(value || "").replace(/^#/, "").trim() || "home";
  return tabAliases[cleaned] || cleaned;
}

function tabIsAllowed(tab) {
  return tab !== "backups" || state.canViewBackups;
}

function frameURL(frame, tab) {
  const raw = paneDefinitions[tab]?.path || paneDefinitions.home.path;
  const url = new URL(raw, window.location.origin);
  const current = new URLSearchParams(window.location.search);
  const token = current.get("token");
  const mediaJob = current.get("media_job");
  if (tab === "join-home" && token) {
    url.searchParams.set("token", token);
  }
  if (tab === "ai" && mediaJob) {
    url.searchParams.set("media_job", mediaJob);
    url.searchParams.set("settings_tab", "tools");
  }
  return url.toString();
}

function extractMain(html) {
  const doc = new DOMParser().parseFromString(html, "text/html");
  const main = doc.querySelector("main");
  if (!main) {
    throw new Error("Settings pane did not include a main content area.");
  }
  return {
    className: main.className,
    html: main.innerHTML,
  };
}

async function importPaneScript(script, loadID) {
  const url = new URL(script, window.location.origin);
  url.searchParams.set("settings_mount", String(loadID));
  await import(url.toString());
}

async function loadNativePane(tab) {
  const definition = paneDefinitions[tab] || paneDefinitions.home;
  const loadID = ++state.paneLoadID;
  els.paneLoading.hidden = false;
  els.paneLoading.textContent = `Loading ${tab.replace("-", " ")} settings.`;
  els.paneRoot.hidden = true;
  els.paneRoot.innerHTML = "";
  try {
    const response = await fetch(frameURL(null, tab), { credentials: "same-origin" });
    if (!response.ok) {
      throw new Error(response.statusText || "Settings pane could not be loaded.");
    }
    const html = await response.text();
    if (loadID !== state.paneLoadID) return;
    const main = extractMain(html);
    els.paneRoot.className = `settings-mounted-pane ${main.className || ""}`.trim();
    els.paneRoot.innerHTML = main.html;
    els.paneRoot.hidden = false;
    els.paneLoading.hidden = true;
    await importPaneScript(definition.script, loadID);
    if (loadID !== state.paneLoadID) return;
    definition.afterMount?.();
  } catch (error) {
    if (loadID !== state.paneLoadID) return;
    els.paneRoot.hidden = true;
    els.paneLoading.hidden = false;
    els.paneLoading.textContent = error.message || "Settings pane could not be loaded.";
    showToast(els.paneLoading.textContent, true);
  }
}

function setActiveTab(tab, updateHash = true) {
  const normalized = normalizeTab(tab);
  const allowed = tabIsAllowed(normalized) ? normalized : "home";
  state.activeTab = allowed;
  settingsTabs.forEach((button) => {
    const active = button.dataset.settingsPageTab === allowed;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
  loadNativePane(allowed);
  if (updateHash && window.location.hash !== `#${allowed}`) {
    window.history.replaceState({}, "", `${window.location.pathname}${window.location.search}#${allowed}`);
  }
}

function applyAdminVisibility(isVisible) {
  state.canViewBackups = isVisible;
  document.querySelectorAll('[data-admin-only="true"]').forEach((element) => {
    element.hidden = !isVisible;
  });
  if (!isVisible && state.activeTab === "backups") {
    setActiveTab("home");
  }
}

async function detectAdminAccess() {
  applyAdminVisibility(false);
  try {
    const membersPayload = await api("/v1/home/members");
    const members = membersPayload.members || [];
    const current = members.find((member) => member.user_id === state.user?.id);
    applyAdminVisibility(current?.role === "admin");
  } catch (_) {
    applyAdminVisibility(false);
  }
}

async function chooseInitialTab() {
  let requested = normalizeTab(window.location.hash);
  if (!window.location.hash) {
    try {
      await api("/v1/home");
    } catch (_) {
      requested = "join-home";
    }
  }
  setActiveTab(requested, Boolean(window.location.hash));
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
    await detectAdminAccess();
    await chooseInitialTab();
  } catch (_) {
    window.location.replace("/");
  }
}

settingsTabs.forEach((button) => {
  button.addEventListener("click", () => setActiveTab(button.dataset.settingsPageTab));
});

window.addEventListener("hashchange", () => setActiveTab(window.location.hash, false));

window.addEventListener("message", (event) => {
  if (event.origin !== window.location.origin || event.data?.type !== "hank-dashboard:navigate") {
    return;
  }
  const url = new URL(event.data.href, window.location.origin);
  if (url.pathname === "/dashboard/settings") {
    setActiveTab(url.hash, true);
    return;
  }
  window.parent.postMessage(event.data, window.location.origin);
});

els.logoutButton.addEventListener("click", logout);

hydrate();
