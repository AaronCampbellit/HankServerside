const state = {
  user: null,
  homes: [],
  sync: null,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  homeSelect: document.getElementById("home-select"),
  refreshButton: document.getElementById("refresh-button"),
  profileCount: document.getElementById("profile-count"),
  profilesOutput: document.getElementById("profiles-output"),
  notesStatus: document.getElementById("notes-status"),
  notesOutput: document.getElementById("notes-output"),
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

function selectedHomeID() {
  return els.homeSelect.value;
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

function renderSync() {
  const profiles = state.sync?.profiles || {};
  const notes = state.sync?.notes || {};
  const entries = Object.entries(profiles);

  els.profileCount.textContent = `${entries.length} saved`;
  els.notesStatus.textContent = notes.status || "Unknown";

  if (!entries.length) {
    els.profilesOutput.className = "card-list empty-state";
    els.profilesOutput.textContent = "No saved connections have reported status yet.";
  } else {
    els.profilesOutput.className = "card-list";
    els.profilesOutput.innerHTML = "";
    for (const [serviceType, profile] of entries) {
      const card = document.createElement("article");
      card.className = "card";
      card.innerHTML = `
        <div class="card-head">
          <div>
            <div class="card-title">${escapeHTML(serviceType)}</div>
            <div class="meta">Updated ${escapeHTML(formatDate(profile.updated_at))}</div>
          </div>
          <span class="status-chip ${profile.status === "healthy" ? "" : "offline"}">${escapeHTML(profile.status || "unknown")}</span>
        </div>
        <div class="meta">Saved version: ${escapeHTML(profile.applied_version)}</div>
        <div class="meta">Private version: ${escapeHTML(profile.secret_version)}</div>
        <div class="meta">Last backup: ${escapeHTML(formatDate(profile.last_backup_at))}</div>
        <div class="meta">Last error: ${escapeHTML(profile.last_error || "None")}</div>
      `;
      els.profilesOutput.appendChild(card);
    }
  }

  const rows = [
    ["Status", notes.status || "unknown"],
    ["Last Manifest", formatDate(notes.last_manifest_at)],
    ["Last Pull", formatDate(notes.last_pull_at)],
    ["Last Push", formatDate(notes.last_push_at)],
    ["Last Successful Sync", formatDate(notes.last_successful_sync_at)],
    ["Last Successful Backup", formatDate(notes.last_successful_backup_at)],
    ["Pending Pull Count", notes.pending_pull_count ?? 0],
    ["Pending Push Count", notes.pending_push_count ?? 0],
    ["Last Error", notes.last_error || "None"],
  ];
  els.notesOutput.innerHTML = rows.map(([label, value]) => `
    <div class="kv-row">
      <div class="kv-label">${escapeHTML(label)}</div>
      <div>${escapeHTML(value)}</div>
    </div>
  `).join("");
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

async function loadSync() {
  const homeID = selectedHomeID();
  if (!homeID) {
    state.sync = null;
    renderSync();
    return;
  }
  state.sync = await api("/v1/home/sync");
  renderSync();
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
    await loadSync();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.homeSelect.addEventListener("change", async () => {
  syncURL(selectedHomeID());
  await loadSync();
});
els.refreshButton.addEventListener("click", async () => {
  try {
    await loadSync();
    showToast("Status refreshed.");
  } catch (error) {
    showToast(error.message, true);
  }
});

hydrate();
