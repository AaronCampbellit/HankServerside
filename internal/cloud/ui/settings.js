const api = window.HankAPI.request;

const state = {
  user: null,
  canViewBackups: false,
  activeTab: "",
};

const settingsTabs = Array.from(document.querySelectorAll("[data-settings-page-tab]"));
const settingsPanels = Array.from(document.querySelectorAll("[data-settings-page-panel]"));
const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  toast: document.getElementById("toast"),
};

const tabAliases = {
  links: "quick-links",
  quicklinks: "quick-links",
  backup: "backups",
  storage: "backups",
  restore: "recovery",
  recovery: "recovery",
  openai: "ai",
  assistant: "ai",
  invitations: "join-home",
  invite: "join-home",
};
const minFrameHeight = 620;
const framePadding = 14;


function showToast(message, isError = false) {
  els.toast.hidden = false;
  els.toast.textContent = message;
  els.toast.style.background = isError ? "rgba(142, 45, 28, 0.94)" : "rgba(35, 27, 20, 0.92)";
  clearTimeout(showToast.timeoutID);
  showToast.timeoutID = window.setTimeout(() => {
    els.toast.hidden = true;
  }, 3400);
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
  return (tab !== "backups" && tab !== "recovery") || state.canViewBackups;
}

function frameURL(frame, tab) {
  const raw = frame.dataset.src || "";
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

function resizeFrame(frame) {
  try {
    const doc = frame.contentDocument;
    const win = frame.contentWindow;
    if (!doc || !win) return;
    const contentRoot = doc.querySelector(".shell") || doc.querySelector("main") || doc.body;
    if (!contentRoot) return;
    const rect = contentRoot.getBoundingClientRect();
    const bodyStyle = win.getComputedStyle(doc.body);
    const marginTop = Number.parseFloat(bodyStyle.marginTop) || 0;
    const marginBottom = Number.parseFloat(bodyStyle.marginBottom) || 0;
    const measuredHeight = Math.ceil(rect.height + marginTop + marginBottom + framePadding);
    const nextHeight = Math.max(minFrameHeight, measuredHeight);
    const currentHeight = Number.parseFloat(frame.style.height) || 0;
    if (Math.abs(currentHeight - nextHeight) > 2) {
      frame.style.height = `${nextHeight}px`;
    }
  } catch (_) {
  }
}

function scheduleFrameResize(frame) {
  if (frame.dataset.resizePending === "true") {
    return;
  }
  frame.dataset.resizePending = "true";
  window.requestAnimationFrame(() => {
    frame.dataset.resizePending = "false";
    resizeFrame(frame);
  });
}

function installFrameResizer(frame) {
  if (frame.dataset.resizerInstalled === "true") {
    scheduleFrameResize(frame);
    return;
  }
  frame.dataset.resizerInstalled = "true";
  frame.addEventListener("load", () => {
    resizeFrame(frame);
    try {
      const doc = frame.contentDocument;
      const resize = () => scheduleFrameResize(frame);
      frame._settingsPaneResizeObserver?.disconnect();
      const contentRoot = doc?.querySelector(".shell") || doc?.querySelector("main") || doc?.body;
      const nodeType = frame.contentWindow?.Node;
      const canObserve = contentRoot && (!nodeType || contentRoot instanceof nodeType);
      if (frame.contentWindow?.ResizeObserver && canObserve) {
        const observer = new frame.contentWindow.ResizeObserver(resize);
        observer.observe(contentRoot);
        frame._settingsPaneResizeObserver = observer;
      }
      frame.contentWindow?.addEventListener("resize", resize);
      window.setTimeout(resize, 150);
      window.setTimeout(resize, 600);
    } catch (_) {
    }
  });
}

function loadPanelFrame(panel, tab) {
  const frame = panel.querySelector("iframe[data-src]");
  if (!frame) return;
  installFrameResizer(frame);
  if (!frame.src) {
    frame.src = frameURL(frame, tab);
  } else {
    resizeFrame(frame);
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
  settingsPanels.forEach((panel) => {
    const active = panel.dataset.settingsPagePanel === allowed;
    panel.hidden = !active;
    if (active) {
      loadPanelFrame(panel, allowed);
    }
  });
  if (updateHash && window.location.hash !== `#${allowed}`) {
    window.history.replaceState({}, "", `${window.location.pathname}${window.location.search}#${allowed}`);
  }
}

function applyAdminVisibility(isVisible) {
  state.canViewBackups = isVisible;
  document.querySelectorAll('[data-admin-only="true"]').forEach((element) => {
    element.hidden = !isVisible;
  });
  if (!isVisible && (state.activeTab === "backups" || state.activeTab === "recovery")) {
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
