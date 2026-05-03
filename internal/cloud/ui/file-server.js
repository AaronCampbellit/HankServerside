const state = {
  user: null,
  homes: [],
  members: [],
  profiles: [],
  currentFilesPath: "/",
  lastTransfer: null,
  dragDepth: 0,
  appSocket: null,
  appSocketPromise: null,
  pendingRequests: new Map(),
  requestCounter: 0,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  homeSelect: document.getElementById("home-select"),
  sourceStatus: document.getElementById("source-status"),
  sourceSummary: document.getElementById("source-summary"),
  transferOutput: document.getElementById("transfer-output"),
  pathStatus: document.getElementById("path-status"),
  filesPath: document.getElementById("files-path"),
  fileBreadcrumbs: document.getElementById("file-breadcrumbs"),
  filesNewDirectory: document.getElementById("files-new-directory"),
  filesUpload: document.getElementById("files-upload"),
  filesRefreshButton: document.getElementById("files-refresh-button"),
  filesUpButton: document.getElementById("files-up-button"),
  filesCreateDirectoryButton: document.getElementById("files-create-directory-button"),
  filesUploadButton: document.getElementById("files-upload-button"),
  filesOutput: document.getElementById("files-output"),
  fileExplorerPanel: document.querySelector(".file-explorer-panel"),
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

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function selectedHomeID() {
  return els.homeSelect.value;
}

function selectedHome() {
  return state.homes.find((home) => home.id === selectedHomeID()) || null;
}

function selectedHomeOrThrow() {
  const home = selectedHome();
  if (!home) throw new Error("Choose a home first.");
  return home;
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

function parseConfigJSON(value) {
  if (!value) return {};
  if (typeof value === "object") return value;
  try {
    return JSON.parse(value);
  } catch {
    return {};
  }
}

function fileConfig() {
  const profile = profileByType("smb");
  const cfg = parseConfigJSON(profile?.public_config_json);
  const host = cfg.host || cfg.smb_host || "";
  const share = cfg.share || cfg.smb_share || "";
  const username = cfg.username || cfg.smb_username || "";
  const domain = cfg.domain || cfg.smb_domain || "";
  return {
    profile,
    host,
    share,
    username,
    domain,
    root: cfg.root || "",
    smbEnabled: Boolean(cfg.smb_enabled ?? (host && share)),
    localRootEnabled: Boolean(cfg.local_root_enabled),
    passwordSet: Boolean(cfg.smb_password_set),
  };
}

function normalizeSMBHostInput(value) {
  let host = String(value || "").trim().replaceAll("\\", "/");
  if (!host) return "";

  try {
    const parsed = new URL(host);
    if (parsed.protocol === "http:" || parsed.protocol === "https:") {
      return parsed.hostname;
    }
    if (parsed.host) {
      return parsed.host;
    }
  } catch (_) {
  }

  host = host.replace(/^smb:\/\//i, "").replace(/^cifs:\/\//i, "").replace(/^\/+/, "");
  const slashIndex = host.indexOf("/");
  return slashIndex >= 0 ? host.slice(0, slashIndex).trim() : host.trim();
}

function preferredAppSocketURL() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws/app`;
}

function nextRequestID() {
  state.requestCounter += 1;
  if (window.crypto?.randomUUID) return `files-${window.crypto.randomUUID()}`;
  return `files-${Date.now()}-${state.requestCounter}`;
}

function closeAppSocket() {
  if (state.appSocket) {
    try {
      state.appSocket.close();
    } catch (_) {
    }
  }
  state.appSocket = null;
  state.appSocketPromise = null;
  for (const { reject } of state.pendingRequests.values()) {
    reject(new Error("File server connection closed."));
  }
  state.pendingRequests.clear();
}

function handleSocketMessage(event) {
  let envelope;
  try {
    envelope = JSON.parse(event.data);
  } catch (_) {
    return;
  }
  const pending = state.pendingRequests.get(envelope.request_id);
  if (!pending) return;
  state.pendingRequests.delete(envelope.request_id);
  if (envelope.type === "app.error" || envelope.error) {
    pending.reject(new Error(envelope.error?.message || "The home connector did not return a result."));
    return;
  }
  pending.resolve(envelope.payload ?? null);
}

async function ensureAppSocket() {
  if (state.appSocket && state.appSocket.readyState === WebSocket.OPEN) {
    return state.appSocket;
  }
  if (state.appSocketPromise) {
    return state.appSocketPromise;
  }
  state.appSocketPromise = new Promise((resolve, reject) => {
    const socket = new WebSocket(preferredAppSocketURL());
    state.appSocket = socket;
    socket.addEventListener("open", () => {
      state.appSocketPromise = null;
      resolve(socket);
    }, { once: true });
    socket.addEventListener("message", handleSocketMessage);
    socket.addEventListener("close", () => {
      if (state.appSocket === socket) closeAppSocket();
    });
    socket.addEventListener("error", () => {
      if (state.appSocket === socket) closeAppSocket();
      reject(new Error("Failed to connect to the home connector."));
    }, { once: true });
  });
  return state.appSocketPromise;
}

async function sendCommand(command, body = {}) {
  selectedHomeOrThrow();
  const socket = await ensureAppSocket();
  const requestID = nextRequestID();
  const envelope = {
    version: "v1",
    type: "app.command",
    request_id: requestID,
    timestamp: new Date().toISOString(),
    payload: { command, body },
  };
  return new Promise((resolve, reject) => {
    state.pendingRequests.set(requestID, { resolve, reject });
    try {
      socket.send(JSON.stringify(envelope));
    } catch (error) {
      state.pendingRequests.delete(requestID);
      reject(error);
    }
  });
}

function normalizePath(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed || trimmed === "/") return "/";
  const normalized = `/${trimmed.replace(/^\/+/, "").replace(/\/+/g, "/")}`;
  return normalized.endsWith("/") && normalized !== "/" ? normalized.slice(0, -1) : normalized;
}

function joinPath(base, child) {
  const normalizedBase = normalizePath(base);
  const normalizedChild = String(child || "").trim().replace(/^\/+/, "");
  if (!normalizedChild) return normalizedBase;
  return normalizedBase === "/" ? `/${normalizedChild}` : `${normalizedBase}/${normalizedChild}`;
}

function parentPath(path) {
  const normalized = normalizePath(path);
  if (normalized === "/") return "/";
  const parts = normalized.split("/").filter(Boolean);
  parts.pop();
  return parts.length ? `/${parts.join("/")}` : "/";
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

function renderSourceSummary() {
  const config = fileConfig();
  const profile = config.profile;
  const sourceName = config.smbEnabled
    ? `${config.host}/${config.share}`
    : config.localRootEnabled
      ? "Home connector files"
      : "Not connected";

  els.sourceStatus.textContent = profile?.status || "Not set up";
  if (profile?.status && profile.status !== "healthy") {
    els.sourceStatus.classList.add("offline");
  } else {
    els.sourceStatus.classList.remove("offline");
  }

  if (!profile) {
    els.sourceSummary.className = "card-list empty-state";
    els.sourceSummary.textContent = "No file server has been saved for this home yet.";
    return;
  }

  els.sourceSummary.className = "card-list";
  els.sourceSummary.innerHTML = `
    <article class="card split-card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(sourceName)}</div>
          <div class="meta">Updated ${escapeHTML(formatDate(profile.updated_at))}</div>
        </div>
        <span class="status-chip ${profile.status === "healthy" ? "" : "offline"}">${escapeHTML(profile.status || "unknown")}</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Server</div><div>${escapeHTML(config.host || "Local connector volume")}</div></div>
        <div class="kv-row"><div class="kv-label">Share</div><div>${escapeHTML(config.share || config.root || "/srv/hank/files")}</div></div>
        <div class="kv-row"><div class="kv-label">Username</div><div>${escapeHTML(config.username || "Not set")}</div></div>
        <div class="kv-row"><div class="kv-label">Password</div><div>${config.passwordSet ? "Saved" : "Not saved"}</div></div>
        <div class="kv-row"><div class="kv-label">Last Error</div><div>${escapeHTML(profile.last_error || "None")}</div></div>
      </div>
    </article>
  `;
}

function renderSettings() {
  const config = fileConfig();
  els.smbHost.value = config.host;
  els.smbShare.value = config.share;
  els.smbDomain.value = config.domain;
  els.smbUsername.value = config.username;
  els.smbPassword.value = "";

  const admin = isAdmin();
  els.settingsRole.textContent = admin ? "Admin" : "View Only";
  els.smbForm.querySelectorAll("input, button").forEach((element) => {
    element.disabled = !admin;
  });
}

function renderBreadcrumbs() {
  const normalized = normalizePath(state.currentFilesPath);
  els.pathStatus.textContent = normalized;
  els.filesPath.value = normalized;
  els.fileBreadcrumbs.innerHTML = "";

  const root = document.createElement("button");
  root.type = "button";
  root.textContent = "Home";
  root.addEventListener("click", () => browseFiles("/"));
  els.fileBreadcrumbs.appendChild(root);

  const parts = normalized.split("/").filter(Boolean);
  let current = "";
  parts.forEach((part) => {
    current = `${current}/${part}`;
    const separator = document.createElement("span");
    separator.textContent = "/";
    els.fileBreadcrumbs.appendChild(separator);

    const button = document.createElement("button");
    button.type = "button";
    button.textContent = part;
    const target = current;
    button.addEventListener("click", () => browseFiles(target));
    els.fileBreadcrumbs.appendChild(button);
  });
}

function renderFiles(items) {
  renderBreadcrumbs();
  if (!items.length) {
    els.filesOutput.className = "file-list empty-state";
    els.filesOutput.textContent = "This folder is empty.";
    return;
  }
  els.filesOutput.className = "file-list";
  els.filesOutput.innerHTML = "";
  items.forEach((item) => {
    const row = document.createElement("article");
    row.className = `file-row${item.is_directory ? " directory" : ""}`;
    row.innerHTML = `
      <button type="button" class="file-main">
        <span class="file-icon" aria-hidden="true">${item.is_directory ? "Folder" : "File"}</span>
        <span>
          <strong>${escapeHTML(item.name || item.path || "Untitled")}</strong>
          <span class="meta">${escapeHTML(item.path || "")}</span>
        </span>
      </button>
      <div class="file-meta">
        <span>${item.is_directory ? "Folder" : escapeHTML(formatBytes(item.size))}</span>
        <span>${escapeHTML(formatDate(item.modified_at))}</span>
      </div>
      <div class="item-actions"></div>
    `;
    row.querySelector(".file-main").addEventListener("click", () => {
      if (item.is_directory) {
        browseFiles(item.path);
      } else {
        downloadFile(item.path);
      }
    });
    const actions = row.querySelector(".item-actions");
    if (item.is_directory) {
      const open = document.createElement("button");
      open.type = "button";
      open.textContent = "Open";
      open.addEventListener("click", () => browseFiles(item.path));
      actions.appendChild(open);
      installFolderDropTarget(row, item.path);
    } else {
      const download = document.createElement("button");
      download.type = "button";
      download.textContent = "Download";
      download.addEventListener("click", () => downloadFile(item.path));
      actions.appendChild(download);
    }
    const rename = document.createElement("button");
    rename.type = "button";
    rename.className = "secondary";
    rename.textContent = "Rename";
    rename.addEventListener("click", () => renameFileItem(item));
    actions.appendChild(rename);

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "ghost";
    remove.textContent = "Delete";
    remove.addEventListener("click", () => deleteFileItem(item));
    actions.appendChild(remove);
    els.filesOutput.appendChild(row);
  });
}

function filesFromDragEvent(event) {
  return Array.from(event.dataTransfer?.files || []).filter((file) => file && file.name);
}

function dragHasFiles(event) {
  return Array.from(event.dataTransfer?.types || []).includes("Files");
}

function setExplorerDragActive(isActive) {
  els.fileExplorerPanel?.classList.toggle("drag-active", isActive);
}

function installExplorerDropTarget() {
  if (!els.fileExplorerPanel) return;

  els.fileExplorerPanel.addEventListener("dragenter", (event) => {
    if (!filesFromDragEvent(event).length && !dragHasFiles(event)) return;
    event.preventDefault();
    state.dragDepth += 1;
    setExplorerDragActive(true);
  });

  els.fileExplorerPanel.addEventListener("dragover", (event) => {
    if (!dragHasFiles(event)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
  });

  els.fileExplorerPanel.addEventListener("dragleave", (event) => {
    if (!dragHasFiles(event)) return;
    state.dragDepth = Math.max(0, state.dragDepth - 1);
    if (state.dragDepth === 0) {
      setExplorerDragActive(false);
    }
  });

  els.fileExplorerPanel.addEventListener("drop", async (event) => {
    const files = filesFromDragEvent(event);
    if (!files.length) return;
    event.preventDefault();
    state.dragDepth = 0;
    setExplorerDragActive(false);
    await uploadFiles(files, state.currentFilesPath);
  });
}

function installFolderDropTarget(row, path) {
  row.addEventListener("dragenter", (event) => {
    if (!dragHasFiles(event)) return;
    event.preventDefault();
    row.classList.add("drop-target");
  });
  row.addEventListener("dragover", (event) => {
    if (!dragHasFiles(event)) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = "copy";
    row.classList.add("drop-target");
  });
  row.addEventListener("dragleave", () => {
    row.classList.remove("drop-target");
  });
  row.addEventListener("drop", async (event) => {
    const files = filesFromDragEvent(event);
    if (!files.length) return;
    event.preventDefault();
    event.stopPropagation();
    row.classList.remove("drop-target");
    state.dragDepth = 0;
    setExplorerDragActive(false);
    await uploadFiles(files, path);
  });
}

function renderLastTransfer() {
  if (!state.lastTransfer) {
    els.transferOutput.className = "card-list empty-state";
    els.transferOutput.textContent = "No file transfers yet.";
    return;
  }
  const transfer = state.lastTransfer;
  els.transferOutput.className = "card-list";
  els.transferOutput.innerHTML = `
    <article class="card split-card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(transfer.operation)} · ${escapeHTML(transfer.path)}</div>
          <div class="meta">${escapeHTML(transfer.transfer_id || "")}</div>
        </div>
        <span class="pill">${escapeHTML(transfer.method || "")}</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Result</div><div>${escapeHTML(transfer.result || "pending")}</div></div>
        <div class="kv-row"><div class="kv-label">Created</div><div>${escapeHTML(formatDate(transfer.created_at))}</div></div>
        <div class="kv-row"><div class="kv-label">Expires</div><div>${escapeHTML(formatDate(transfer.expires_at))}</div></div>
      </div>
    </article>
  `;
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
    renderSettings();
    return;
  }
  state.members = (await api("/v1/home/members")).members || [];
  renderSettings();
}

async function loadProfiles() {
  const home = selectedHome();
  if (!home) {
    state.profiles = [];
    renderSourceSummary();
    renderSettings();
    return;
  }
  state.profiles = (await api("/v1/home/service-profiles")).profiles || [];
  renderSourceSummary();
  renderSettings();
}

async function browseFiles(path = state.currentFilesPath) {
  try {
    const normalized = normalizePath(path);
    els.filesOutput.className = "file-list empty-state";
    els.filesOutput.textContent = "Loading folder.";
    const payload = await sendCommand("files.list", { path: normalized });
    state.currentFilesPath = normalized;
    renderFiles(payload.items || []);
  } catch (error) {
    renderBreadcrumbs();
    els.filesOutput.className = "file-list empty-state";
    els.filesOutput.textContent = "Could not load this folder.";
    showToast(error.message, true);
  }
}

async function createDirectory() {
  const name = els.filesNewDirectory.value.trim();
  if (!name) {
    showToast("Enter a folder name first.", true);
    return;
  }
  try {
    await sendCommand("files.create_directory", { path: joinPath(state.currentFilesPath, name) });
    els.filesNewDirectory.value = "";
    await browseFiles(state.currentFilesPath);
    showToast("Folder created.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function renameFileItem(item) {
  const targetName = window.prompt("Rename item to:", item.name);
  if (!targetName || targetName === item.name) return;
  try {
    await sendCommand("files.rename", { from: item.path, to: joinPath(parentPath(item.path), targetName) });
    await browseFiles(state.currentFilesPath);
    showToast("Item renamed.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function deleteFileItem(item) {
  if (!window.confirm(`Delete ${item.path}?`)) return;
  try {
    await sendCommand("files.delete", { path: item.path, is_directory: Boolean(item.is_directory) });
    await browseFiles(state.currentFilesPath);
    showToast("Item deleted.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function downloadFile(path) {
  try {
    selectedHomeOrThrow();
    const setup = await api("/v1/home/files/downloads", {
      method: "POST",
      body: JSON.stringify({ path }),
    });
    const response = await fetch(setup.url);
    if (!response.ok) throw new Error(await response.text());
    const blob = await response.blob();
    const href = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = href;
    link.download = path.split("/").pop() || "download";
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(href);
    state.lastTransfer = {
      ...setup,
      result: `downloaded ${formatBytes(blob.size)}`,
      completed_at: new Date().toISOString(),
    };
    renderLastTransfer();
    showToast(`Downloaded ${link.download}.`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function uploadOneFile(file, targetFolder) {
  const setup = await api("/v1/home/files/uploads", {
    method: "POST",
    body: JSON.stringify({ path: joinPath(targetFolder, file.name) }),
  });
  const response = await fetch(setup.url, {
    method: "PUT",
    headers: { "Content-Type": "application/octet-stream" },
    body: file,
  });
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload.error || payload.message || response.statusText);
  }
  state.lastTransfer = {
    ...setup,
    path: joinPath(targetFolder, file.name),
    result: `uploaded ${formatBytes(payload.size)}`,
    completed_at: new Date().toISOString(),
  };
}

async function uploadFiles(files, targetFolder = state.currentFilesPath) {
  const selectedFiles = Array.from(files || []).filter((file) => file && file.name);
  if (!selectedFiles.length) {
    showToast("Choose a file to upload first.", true);
    return;
  }
  try {
    selectedHomeOrThrow();
    for (const file of selectedFiles) {
      await uploadOneFile(file, targetFolder);
    }
    els.filesUpload.value = "";
    renderLastTransfer();
    await browseFiles(state.currentFilesPath);
    showToast(selectedFiles.length === 1 ? `Uploaded ${selectedFiles[0].name}.` : `Uploaded ${selectedFiles.length} files.`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function uploadFile() {
  await uploadFiles(els.filesUpload.files, state.currentFilesPath);
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
  const password = els.smbPassword.value;
  if (password) {
    payload.secrets = { password };
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
    renderLastTransfer();
    renderBreadcrumbs();
    if (selectedHome()) {
      await browseFiles("/");
    }
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.homeSelect.addEventListener("change", async () => {
  syncURL(selectedHomeID());
  closeAppSocket();
  await loadMembers();
  await loadProfiles();
  await browseFiles("/");
});
els.filesRefreshButton.addEventListener("click", () => browseFiles(els.filesPath.value));
els.filesPath.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    browseFiles(els.filesPath.value);
  }
});
els.filesUpButton.addEventListener("click", () => browseFiles(parentPath(state.currentFilesPath)));
els.filesCreateDirectoryButton.addEventListener("click", createDirectory);
els.filesUploadButton.addEventListener("click", uploadFile);
els.smbForm.addEventListener("submit", saveSMBSettings);
installExplorerDropTarget();

hydrate();
