const TEXT_PREVIEW_LIMIT = 1024 * 1024;
const RICH_PREVIEW_LIMIT = 25 * 1024 * 1024;
const FILE_DRAG_TYPE = "application/x-hank-file-paths";

const imageExtensions = new Set(["avif", "bmp", "gif", "heic", "heif", "jpeg", "jpg", "png", "svg", "webp"]);
const textExtensions = new Set(["cfg", "conf", "css", "csv", "go", "htm", "html", "js", "json", "log", "md", "ps1", "py", "rb", "rs", "sh", "sql", "swift", "toml", "ts", "txt", "xml", "yaml", "yml"]);

const state = {
  user: null,
  homes: [],
  profiles: [],
  activeSourceID: "",
  currentFilesPath: "/",
  currentItems: [],
  visibleItems: [],
  selectedItems: new Map(),
  activeItemPath: "",
  targetFilesPath: "",
  targetIsDirectory: false,
  searchMode: false,
  searchQuery: "",
  sortKey: "name",
  sortDirection: "asc",
  viewMode: "list",
  lastTransfer: null,
  fileJobs: [],
  dragDepth: 0,
  previewObjectURL: "",
  previewRequestID: 0,
  renameItem: null,
  moveItems: [],
  moveDialogPath: "/",
  moveSourceID: "",
  moveDestinationSourceID: "",
  deleteItems: [],
  confirmResolver: null,
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
  sourceSelect: document.getElementById("source-select"),
  sourceStatus: document.getElementById("source-status"),
  sourceSummary: document.getElementById("source-summary"),
  transferOutput: document.getElementById("transfer-output"),
  fileJobsOutput: document.getElementById("file-jobs-output"),
  fileJobsRefreshButton: document.getElementById("file-jobs-refresh-button"),
  pathStatus: document.getElementById("path-status"),
  filesSearch: document.getElementById("files-search"),
  filesSearchButton: document.getElementById("files-search-button"),
  filesSearchClearButton: document.getElementById("files-search-clear-button"),
  filesPath: document.getElementById("files-path"),
  fileBreadcrumbs: document.getElementById("file-breadcrumbs"),
  fileTree: document.getElementById("file-tree"),
  filesUpload: document.getElementById("files-upload"),
  filesRefreshCurrentButton: document.getElementById("files-refresh-current-button"),
  filesNewFolderButton: document.getElementById("files-new-folder-button"),
  filesUploadButton: document.getElementById("files-upload-button"),
  filesOutput: document.getElementById("files-output"),
  filesSort: document.getElementById("files-sort"),
  filesSelectAll: document.getElementById("files-select-all"),
  filesListViewButton: document.getElementById("files-list-view-button"),
  filesGridViewButton: document.getElementById("files-grid-view-button"),
  selectionBar: document.getElementById("file-selection-bar"),
  selectionCount: document.getElementById("file-selection-count"),
  downloadSelectedButton: document.getElementById("files-download-selected-button"),
  moveSelectedButton: document.getElementById("files-move-selected-button"),
  renameSelectedButton: document.getElementById("files-rename-selected-button"),
  deleteSelectedButton: document.getElementById("files-delete-selected-button"),
  clearSelectionButton: document.getElementById("files-clear-selection-button"),
  fileExplorerPanel: document.querySelector(".file-explorer-panel"),
  previewTitle: document.getElementById("file-preview-title"),
  previewMeta: document.getElementById("file-preview-meta"),
  previewBody: document.getElementById("file-preview-body"),
  previewActions: document.getElementById("file-preview-actions"),
  newFolderDialog: document.getElementById("new-folder-dialog"),
  newFolderForm: document.getElementById("new-folder-form"),
  newFolderName: document.getElementById("new-folder-name"),
  newFolderLocation: document.getElementById("new-folder-location"),
  newFolderCancelButton: document.getElementById("new-folder-cancel-button"),
  renameDialog: document.getElementById("rename-dialog"),
  renameForm: document.getElementById("rename-form"),
  renameName: document.getElementById("rename-name"),
  renamePath: document.getElementById("rename-path"),
  renameCancelButton: document.getElementById("rename-cancel-button"),
  moveDialog: document.getElementById("move-dialog"),
  moveForm: document.getElementById("move-form"),
  movePath: document.getElementById("move-path"),
  moveOpenButton: document.getElementById("move-open-button"),
  moveSourceSelect: document.getElementById("move-source-select"),
  moveBreadcrumbs: document.getElementById("move-breadcrumbs"),
  moveFolders: document.getElementById("move-folders"),
  moveTargetLabel: document.getElementById("move-target-label"),
  moveConfirmButton: document.getElementById("move-confirm-button"),
  moveCancelButton: document.getElementById("move-cancel-button"),
  deleteDialog: document.getElementById("delete-dialog"),
  deleteForm: document.getElementById("delete-form"),
  deleteBody: document.getElementById("delete-body"),
  deleteCancelButton: document.getElementById("delete-cancel-button"),
  confirmDialog: document.getElementById("confirm-dialog"),
  confirmForm: document.getElementById("confirm-form"),
  confirmTitle: document.getElementById("confirm-title"),
  confirmBody: document.getElementById("confirm-body"),
  confirmContinueButton: document.getElementById("confirm-continue-button"),
  confirmCancelButton: document.getElementById("confirm-cancel-button"),
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

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
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

function itemPath(item) {
  return normalizePath(item?.path || item?.name || "");
}

function rawItemPath(item) {
  return item?.path || itemPath(item);
}

function itemName(item) {
  return item?.name || itemPath(item).split("/").pop() || "Untitled";
}

function extensionForItem(item) {
  const name = itemName(item).toLowerCase();
  const index = name.lastIndexOf(".");
  return index > 0 ? name.slice(index + 1) : "";
}

function fileTypeLabel(item) {
  if (item?.is_directory) return "Folder";
  const extension = extensionForItem(item);
  return extension ? extension.toUpperCase() : "File";
}

function iconLabel(item) {
  if (item?.is_directory) return "DIR";
  const extension = extensionForItem(item);
  if (!extension) return "FILE";
  if (imageExtensions.has(extension)) return "IMG";
  if (extension === "pdf") return "PDF";
  if (textExtensions.has(extension)) return "TXT";
  return extension.slice(0, 4).toUpperCase();
}

function selectedHomeID() {
  return els.homeSelect.value;
}

function selectedSourceID() {
  const config = fileConfig();
  const sourceID = els.sourceSelect?.value || state.activeSourceID || "";
  return config.sources.some((source) => source.id === sourceID) ? sourceID : "";
}

function startupParams() {
  return new URLSearchParams(window.location.search);
}

function requestedFilesPath() {
  return normalizePath(startupParams().get("path") || "/");
}

function requestedPathIsDirectory() {
  return startupParams().get("directory") === "true";
}

function requestedSourceID() {
  return startupParams().get("source_id") || "";
}

function selectedHome() {
  return state.homes.find((home) => home.id === selectedHomeID()) || null;
}

function selectedHomeOrThrow() {
  const home = selectedHome();
  if (!home) throw new Error("Choose a home first.");
  return home;
}

function syncURL(homeID, sourceID = state.activeSourceID) {
  const url = new URL(window.location.href);
  if (homeID) {
    url.searchParams.set("home_id", homeID);
  } else {
    url.searchParams.delete("home_id");
  }
  if (sourceID) {
    url.searchParams.set("source_id", sourceID);
  } else {
    url.searchParams.delete("source_id");
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
  const sources = fileSourcesFromConfig(cfg);
  const requested = requestedSourceID();
  const active = sources.find((source) => source.id === state.activeSourceID)
    || sources.find((source) => source.id === requested)
    || sources.find((source) => source.id === cfg.active_source_id)
    || sources[0]
    || null;
  state.activeSourceID = active?.id || "";
  return {
    profile,
    sources,
    active,
    host: active?.host || "",
    share: active?.share || "",
    username: active?.username || "",
    domain: active?.domain || "",
    root: active?.root || cfg.root || "",
    smbEnabled: Boolean(active?.smbEnabled),
    localRootEnabled: Boolean(active?.localRootEnabled),
    passwordSet: Boolean(active?.passwordSet),
  };
}

function fileSourcesFromConfig(cfg) {
  const sources = [];
  const rawSources = Array.isArray(cfg.file_sources) ? cfg.file_sources
    : Array.isArray(cfg.sources) ? cfg.sources
      : [];
  rawSources.forEach((entry, index) => {
    const source = normalizeSourceEntry(entry, index);
    if (source?.type === "smb") sources.push(source);
  });

  const rawShares = Array.isArray(cfg.shares) ? cfg.shares : [];
  rawShares.forEach((entry, index) => {
    const source = normalizeShareEntry(entry, index);
    if (source && !sources.some((candidate) => candidate.id === source.id)) {
      sources.push(source);
    }
  });

  const host = cfg.host || cfg.smb_host || "";
  const share = cfg.share || cfg.smb_share || "";
  if (host || share) {
    const source = normalizeShareEntry({
      id: cfg.source_id || cfg.id || "",
      name: cfg.name || share || "SMB share",
      host,
      share,
      username: cfg.username || cfg.smb_username || "",
      domain: cfg.domain || cfg.smb_domain || "",
      enabled: cfg.smb_enabled,
      password_set: cfg.smb_password_set,
    }, sources.length);
    if (source && !sources.some((candidate) => candidate.id === source.id)) {
      sources.unshift(source);
    }
  }

  return sources;
}

function normalizeSourceEntry(entry, index) {
  if (!entry) return null;
  const type = entry.type || (entry.smb_host || entry.host || entry.smb_share || entry.share ? "smb" : "local");
  if (type === "local") {
    return null;
  }
  return normalizeShareEntry({
    id: entry.id || entry.source_id,
    name: entry.name,
    host: entry.host || entry.smb_host,
    share: entry.share || entry.smb_share,
    username: entry.username || entry.smb_username,
    domain: entry.domain || entry.smb_domain,
    enabled: entry.enabled ?? entry.smb_enabled,
    password_set: entry.password_set ?? entry.smb_password_set,
  }, index);
}

function normalizeShareEntry(entry, index) {
  const host = entry?.host || "";
  const share = entry?.share || "";
  const id = cleanSourceID(entry?.id || entry?.source_id || entry?.name || share || `smb-${index + 1}`);
  if (!id && !host && !share) return null;
  return {
    id: id || `smb-${index + 1}`,
    name: entry?.name || share || host || "SMB share",
    type: "smb",
    host,
    share,
    username: entry?.username || "",
    domain: entry?.domain || "",
    smbEnabled: Boolean(entry?.enabled ?? (host && share)),
    localRootEnabled: false,
    passwordSet: Boolean(entry?.password_set),
  };
}

function cleanSourceID(value) {
  return String(value || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_.-]+/g, "-")
    .replace(/^-+|-+$/g, "");
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

function withSourceID(body = {}) {
  const sourceID = selectedSourceID();
  if (!sourceID) {
    throw new Error("No SMB share is configured for this home.");
  }
  return withExplicitSourceID(sourceID, body);
}

function withExplicitSourceID(sourceID, body = {}) {
  if (!sourceID) {
    throw new Error("No SMB share is configured for this home.");
  }
  return { ...body, source_id: sourceID };
}

async function sendFileCommand(command, body = {}) {
  return sendCommand(command, withSourceID(body));
}

async function sendFileCommandForSource(sourceID, command, body = {}) {
  return sendCommand(command, withExplicitSourceID(sourceID, body));
}

function openDialog(dialog) {
  if (!dialog) return;
  if (!dialog.open) dialog.showModal();
}

function closeDialog(dialog) {
  if (dialog?.open) dialog.close();
}

function renderPathStatus() {
  if (state.searchMode) {
    els.pathStatus.textContent = `Search: ${state.searchQuery}`;
    return;
  }
  els.pathStatus.textContent = normalizePath(state.currentFilesPath);
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
  const active = config.active;
  const sourceName = active ? sourceLabel(active) : "Not connected";

  renderSourceSelect(config.sources);

  els.sourceStatus.textContent = profile?.status || "Not set up";
  els.sourceStatus.classList.toggle("offline", Boolean(profile?.status && profile.status !== "healthy"));

  if (!profile || !config.sources.length) {
    els.sourceSummary.className = "card-list empty-state";
    els.sourceSummary.textContent = "No file server shares have been saved for this home yet.";
    return;
  }

  const visibleSources = active ? [active] : config.sources;
  els.sourceSummary.className = "card-list";
  els.sourceSummary.innerHTML = visibleSources.map((source) => `
    <article class="card split-card ${source.id === active?.id ? "selected" : ""}">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(sourceLabel(source))}</div>
          <div class="meta">Active share - Updated ${escapeHTML(formatDate(profile.updated_at))}</div>
        </div>
        <span class="status-chip ${profile.status === "healthy" && sourceEnabled(source) ? "" : "offline"}">${escapeHTML(sourceEnabled(source) ? profile.status || "unknown" : "not configured")}</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Server</div><div>${escapeHTML(source.host || "Not set")}</div></div>
        <div class="kv-row"><div class="kv-label">Share</div><div>${escapeHTML(source.share || "Not set")}</div></div>
        <div class="kv-row"><div class="kv-label">Username</div><div>${escapeHTML(source.username || "Not set")}</div></div>
        <div class="kv-row"><div class="kv-label">Password</div><div>${source.passwordSet ? "Saved" : "Not saved"}</div></div>
        <div class="kv-row"><div class="kv-label">Last Error</div><div>${escapeHTML(profile.last_error || "None")}</div></div>
      </div>
      <div class="item-actions">
        <a class="ops-card manage-link" href="/dashboard/settings#connections">Edit Settings</a>
      </div>
    </article>
  `).join("");
  els.sourceSummary.querySelectorAll("[data-source-id]").forEach((button) => {
    button.addEventListener("click", () => switchSource(button.dataset.sourceId));
  });
}

function renderSourceSelect(sources) {
  if (!els.sourceSelect) return;
  els.sourceSelect.innerHTML = "";
  if (!sources.length) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No shares";
    els.sourceSelect.appendChild(option);
    els.sourceSelect.disabled = true;
    return;
  }
  sources.forEach((source) => {
    const option = document.createElement("option");
    option.value = source.id;
    option.textContent = sourceLabel(source);
    option.selected = source.id === state.activeSourceID;
    els.sourceSelect.appendChild(option);
  });
  els.sourceSelect.disabled = false;
  els.sourceSelect.value = state.activeSourceID || sources[0].id;
}

function sourceEnabled(source) {
  return Boolean(source?.smbEnabled);
}

function sourceLabel(source) {
  if (!source) return "Not connected";
  const target = [source.host, source.share].filter(Boolean).join("/");
  return source.name && source.name !== source.share ? `${source.name} (${target || source.id})` : target || source.name || source.id;
}

async function switchSource(sourceID) {
  if (!sourceID || sourceID === state.activeSourceID) return;
  state.activeSourceID = sourceID;
  syncURL(selectedHomeID(), state.activeSourceID);
  resetSearch();
  clearSelection();
  renderSourceSummary();
  await browseFiles("/");
}

function renderBreadcrumbs() {
  const normalized = normalizePath(state.currentFilesPath);
  els.filesPath.value = normalized;
  els.fileBreadcrumbs.innerHTML = "";

  const root = document.createElement("button");
  root.type = "button";
  root.textContent = "Main";
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

function renderFolderTree() {
  if (!els.fileTree) return;

  const currentPath = normalizePath(state.currentFilesPath);
  els.fileTree.className = "file-tree";
  els.fileTree.innerHTML = "";

  els.fileTree.appendChild(folderTreeButton("Main", "/", currentPath === "/", 0));

  const parts = currentPath.split("/").filter(Boolean);
  let branchPath = "";
  parts.forEach((part, index) => {
    branchPath = `${branchPath}/${part}`;
    els.fileTree.appendChild(folderTreeButton(part, branchPath, branchPath === currentPath, index + 1));
  });

  const childFolders = sortedItems(state.currentItems.filter((item) => item.is_directory));
  if (state.searchMode) {
    const searchFolders = childFolders.filter((item) => itemPath(item) !== currentPath);
    if (!searchFolders.length) {
      const empty = document.createElement("div");
      empty.className = "file-tree-empty";
      empty.textContent = "No matching folders.";
      els.fileTree.appendChild(empty);
      return;
    }
    searchFolders.forEach((folder) => {
      els.fileTree.appendChild(folderTreeButton(itemName(folder), rawItemPath(folder), false, 1));
    });
    return;
  }

  childFolders.forEach((folder) => {
    els.fileTree.appendChild(folderTreeButton(itemName(folder), rawItemPath(folder), false, parts.length + 1));
  });
}

function folderTreeButton(label, path, isActive, depth) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `file-tree-item${isActive ? " active" : ""}`;
  button.style.setProperty("--tree-depth", String(Math.min(Number(depth || 0), 6)));
  button.dataset.filePath = normalizePath(path);
  button.innerHTML = `
    <span class="file-tree-caret" aria-hidden="true">${normalizePath(path) === "/" ? "" : ">"}</span>
    <span class="file-tree-icon" aria-hidden="true">DIR</span>
    <span class="file-tree-label">${escapeHTML(label)}</span>
  `;
  button.addEventListener("click", () => browseFiles(path));
  installFolderDropTarget(button, path);
  return button;
}

function sortedItems(items) {
  const copy = [...items];
  copy.sort((left, right) => {
    if (left.is_directory !== right.is_directory) {
      return left.is_directory ? -1 : 1;
    }

    let result = 0;
    if (state.sortKey === "modified") {
      result = new Date(left.modified_at || 0) - new Date(right.modified_at || 0);
    } else if (state.sortKey === "size") {
      result = Number(left.size || 0) - Number(right.size || 0);
    } else if (state.sortKey === "type") {
      result = fileTypeLabel(left).localeCompare(fileTypeLabel(right));
    } else {
      result = itemName(left).localeCompare(itemName(right), undefined, { sensitivity: "base" });
    }
    if (result === 0) {
      result = itemPath(left).localeCompare(itemPath(right), undefined, { sensitivity: "base" });
    }
    return state.sortDirection === "desc" ? -result : result;
  });
  return copy;
}

function applySortAndRender() {
  state.visibleItems = sortedItems(state.currentItems);
  renderPathStatus();
  renderBreadcrumbs();
  renderFolderTree();
  renderFiles();
  renderSelectionState();
  renderPreview();
}

function renderFiles() {
  if (!state.visibleItems.length) {
    els.filesOutput.className = "file-list empty-state";
    els.filesOutput.textContent = state.searchMode ? "No matching files were found." : "This folder is empty.";
    return;
  }

  els.filesOutput.className = state.viewMode === "grid" ? "file-grid" : "file-table";
  els.filesOutput.innerHTML = "";

  if (state.viewMode === "list") {
    els.filesOutput.appendChild(renderListHeader());
  }

  state.visibleItems.forEach((item) => {
    const element = state.viewMode === "grid" ? renderGridItem(item) : renderListItem(item);
    els.filesOutput.appendChild(element);
  });
}

function renderListHeader() {
  const header = document.createElement("div");
  header.className = "file-table-head";
  header.setAttribute("role", "row");
  header.innerHTML = `
    <span></span>
    ${renderSortHeader("Name", "name")}
    ${renderSortHeader("Last modified", "modified")}
    ${renderSortHeader("Type", "type")}
    ${renderSortHeader("Size", "size")}
  `;
  header.querySelectorAll("[data-sort-key]").forEach((button) => {
    button.addEventListener("click", () => setSort(button.dataset.sortKey));
  });
  return header;
}

function renderSortHeader(label, key) {
  const active = state.sortKey === key;
  const marker = active ? (state.sortDirection === "asc" ? "up" : "down") : "";
  return `
    <button type="button" class="${active ? "active" : ""}" data-sort-key="${escapeHTML(key)}" aria-sort="${active ? state.sortDirection === "asc" ? "ascending" : "descending" : "none"}">
      <span>${escapeHTML(label)}</span>
      <span class="sort-marker" aria-hidden="true">${marker}</span>
    </button>
  `;
}

function renderListItem(item) {
  const key = itemPath(item);
  const row = document.createElement("article");
  row.className = `file-row${item.is_directory ? " directory" : ""}${state.selectedItems.has(key) ? " selected" : ""}${key === normalizePath(state.targetFilesPath) ? " highlighted" : ""}`;
  row.dataset.filePath = key;
  row.draggable = true;
  row.innerHTML = `
    <label class="file-check">
      <input type="checkbox" ${state.selectedItems.has(key) ? "checked" : ""} aria-label="Select ${escapeHTML(itemName(item))}">
    </label>
    <button type="button" class="file-main">
      <span class="file-icon" aria-hidden="true">${escapeHTML(iconLabel(item))}</span>
      <span>
        <strong>${escapeHTML(itemName(item))}</strong>
        <span class="meta">${escapeHTML(state.searchMode ? itemPath(item) : parentPath(itemPath(item)))}</span>
      </span>
    </button>
    <span class="file-table-cell">${escapeHTML(formatDate(item.modified_at))}</span>
    <span class="file-table-cell">${escapeHTML(fileTypeLabel(item))}</span>
    <span class="file-table-cell file-size-cell">${item.is_directory ? "-" : escapeHTML(formatBytes(item.size))}</span>
  `;
  wireFileElement(row, item);
  return row;
}

function renderGridItem(item) {
  const key = itemPath(item);
  const card = document.createElement("article");
  card.className = `file-row file-card${item.is_directory ? " directory" : ""}${state.selectedItems.has(key) ? " selected" : ""}${key === normalizePath(state.targetFilesPath) ? " highlighted" : ""}`;
  card.dataset.filePath = key;
  card.draggable = true;
  card.innerHTML = `
    <div class="file-card-top">
      <label class="file-check">
        <input type="checkbox" ${state.selectedItems.has(key) ? "checked" : ""} aria-label="Select ${escapeHTML(itemName(item))}">
      </label>
      <span class="file-icon" aria-hidden="true">${escapeHTML(iconLabel(item))}</span>
    </div>
    <button type="button" class="file-main">
      <span>
        <strong>${escapeHTML(itemName(item))}</strong>
        <span class="meta">${escapeHTML(itemPath(item))}</span>
      </span>
    </button>
    <div class="file-meta">
      <span>${escapeHTML(fileTypeLabel(item))}${item.is_directory ? "" : ` - ${escapeHTML(formatBytes(item.size))}`}</span>
      <span>${escapeHTML(formatDate(item.modified_at))}</span>
    </div>
    <div class="item-actions"></div>
  `;
  wireFileElement(card, item);
  renderItemActions(card.querySelector(".item-actions"), item);
  return card;
}

function renderItemActions(container, item) {
  container.innerHTML = "";
  if (item.is_directory) {
    container.appendChild(actionButton("Open", () => browseFiles(rawItemPath(item))));
  } else {
    container.appendChild(actionButton("Preview", () => selectOnlyItem(item)));
    container.appendChild(actionButton("Download", () => downloadFile(rawItemPath(item))));
  }
  container.appendChild(actionButton("Move", () => openMoveDialog([item]), "secondary"));
  container.appendChild(actionButton("Rename", () => openRenameDialog(item), "secondary"));
  container.appendChild(actionButton("Delete", () => openDeleteDialog([item]), "ghost"));
}

function actionButton(label, onClick, className = "") {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = label;
  if (className) button.className = className;
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    onClick();
  });
  return button;
}

function wireFileElement(element, item) {
  const checkbox = element.querySelector(".file-check input");
  const main = element.querySelector(".file-main");
  checkbox.addEventListener("click", (event) => {
    event.stopPropagation();
    setItemSelected(item, checkbox.checked);
  });
  main.addEventListener("click", (event) => {
    if (event.metaKey || event.ctrlKey) {
      setItemSelected(item, !state.selectedItems.has(itemPath(item)));
      return;
    }
    selectOnlyItem(item);
  });
  element.addEventListener("click", (event) => {
    if (event.target.closest("button, input, label, a")) return;
    if (event.metaKey || event.ctrlKey) {
      setItemSelected(item, !state.selectedItems.has(itemPath(item)));
      return;
    }
    selectOnlyItem(item);
  });
  main.addEventListener("dblclick", (event) => {
    event.preventDefault();
    openItem(item);
  });
  element.addEventListener("dblclick", (event) => {
    if (event.target.closest("button, input, label")) return;
    openItem(item);
  });
  installFileDrag(element, item);
  if (item.is_directory) {
    installFolderDropTarget(element, rawItemPath(item));
  }
}

function updateRenderedSelection() {
  document.querySelectorAll("[data-file-path]").forEach((element) => {
    const selected = state.selectedItems.has(element.dataset.filePath);
    element.classList.toggle("selected", selected);
    const checkbox = element.querySelector(".file-check input");
    if (checkbox) checkbox.checked = selected;
  });
}

function visibleSelectionStats() {
  const visibleKeys = state.visibleItems.map(itemPath);
  const selectedVisible = visibleKeys.filter((path) => state.selectedItems.has(path));
  return { visibleKeys, selectedVisible };
}

function renderSelectionState() {
  const items = selectedItems();
  const count = items.length;
  els.selectionBar.hidden = count === 0;
  els.selectionCount.textContent = count === 1 ? "1 selected" : `${count} selected`;
  els.renameSelectedButton.disabled = count !== 1;
  els.moveSelectedButton.disabled = count === 0;
  els.deleteSelectedButton.disabled = count === 0;
  els.downloadSelectedButton.disabled = !items.some((item) => !item.is_directory);

  const { visibleKeys, selectedVisible } = visibleSelectionStats();
  els.filesSelectAll.indeterminate = selectedVisible.length > 0 && selectedVisible.length < visibleKeys.length;
  els.filesSelectAll.checked = visibleKeys.length > 0 && selectedVisible.length === visibleKeys.length;
  updateRenderedSelection();
}

function clearSelection() {
  state.selectedItems.clear();
  state.activeItemPath = "";
  renderSelectionState();
  renderPreview();
}

function setItemSelected(item, selected) {
  const key = itemPath(item);
  if (selected) {
    state.selectedItems.set(key, item);
    state.activeItemPath = key;
  } else {
    state.selectedItems.delete(key);
    if (state.activeItemPath === key) {
      state.activeItemPath = selectedItems()[0] ? itemPath(selectedItems()[0]) : "";
    }
  }
  renderSelectionState();
  renderPreview();
}

function selectOnlyItem(item) {
  state.selectedItems.clear();
  state.selectedItems.set(itemPath(item), item);
  state.activeItemPath = itemPath(item);
  renderSelectionState();
  renderPreview();
}

function selectAllVisible(checked) {
  if (!checked) {
    for (const item of state.visibleItems) {
      state.selectedItems.delete(itemPath(item));
    }
  } else {
    for (const item of state.visibleItems) {
      state.selectedItems.set(itemPath(item), item);
    }
    if (!state.activeItemPath && state.visibleItems[0]) {
      state.activeItemPath = itemPath(state.visibleItems[0]);
    }
  }
  if (!state.selectedItems.size) state.activeItemPath = "";
  renderSelectionState();
  renderPreview();
}

function selectedItems() {
  return Array.from(state.selectedItems.values());
}

function openItem(item) {
  if (item.is_directory) {
    browseFiles(rawItemPath(item));
    return;
  }
  selectOnlyItem(item);
}

function setFilesLoading(message) {
  els.filesOutput.className = "file-list empty-state";
  els.filesOutput.textContent = message;
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

async function loadProfiles() {
  const home = selectedHome();
  if (!home) {
    state.profiles = [];
    renderSourceSummary();
    return;
  }
  state.profiles = (await api("/v1/home/service-profiles")).profiles || [];
  renderSourceSummary();
}

function resetSearch() {
  state.searchMode = false;
  state.searchQuery = "";
  els.filesSearch.value = "";
}

async function browseFiles(path = state.currentFilesPath, options = {}) {
  try {
    if (!fileConfig().active) {
      state.currentItems = [];
      state.visibleItems = [];
      renderPathStatus();
      renderBreadcrumbs();
      renderFolderTree();
      renderSelectionState();
      renderPreview();
      setFilesLoading("No SMB share is configured for this home.");
      return;
    }
    const normalized = normalizePath(path);
    if (!options.keepSearch) resetSearch();
    setFilesLoading("Loading folder.");
    const payload = await sendFileCommand("files.list", { path: normalized });
    state.currentFilesPath = normalized;
    state.currentItems = payload.items || [];
    state.visibleItems = [];
    if (!options.keepSelection) {
      state.selectedItems.clear();
      state.activeItemPath = "";
    }
    selectRequestedTargetIfPresent();
    applySortAndRender();
  } catch (error) {
    renderPathStatus();
    renderBreadcrumbs();
    setFilesLoading("Could not load this folder.");
    showToast(error.message, true);
  }
}

function selectRequestedTargetIfPresent() {
  if (!state.targetFilesPath) return;
  const target = normalizePath(state.targetFilesPath);
  const item = state.currentItems.find((candidate) => itemPath(candidate) === target);
  if (!item) return;
  state.selectedItems.clear();
  state.selectedItems.set(itemPath(item), item);
  state.activeItemPath = itemPath(item);
}

async function searchFiles(query = els.filesSearch.value) {
  const trimmed = String(query || "").trim();
  if (!trimmed) {
    await browseFiles(state.currentFilesPath);
    return;
  }
  try {
    setFilesLoading("Searching files.");
    const payload = await sendFileCommand("files.search", { query: trimmed, limit: 100 });
    state.searchMode = true;
    state.searchQuery = trimmed;
    state.currentItems = payload.items || [];
    state.selectedItems.clear();
    state.activeItemPath = "";
    applySortAndRender();
  } catch (error) {
    setFilesLoading("Could not search files.");
    showToast(error.message, true);
  }
}

async function refreshCurrentView() {
  if (state.searchMode) {
    await searchFiles(state.searchQuery);
    return;
  }
  await browseFiles(state.currentFilesPath, { keepSelection: true });
}

function openNewFolderDialog() {
  els.newFolderName.value = "";
  els.newFolderLocation.textContent = `Create in ${normalizePath(state.currentFilesPath)}`;
  openDialog(els.newFolderDialog);
  els.newFolderName.focus();
}

function validFileName(name) {
  return Boolean(name) && !name.includes("/") && !name.includes("\\");
}

async function createDirectoryFromDialog(event) {
  event.preventDefault();
  const name = els.newFolderName.value.trim();
  if (!validFileName(name)) {
    showToast("Use a folder name without slashes.", true);
    return;
  }
  const targetPath = joinPath(state.currentFilesPath, name);
  try {
    if (await statItem(targetPath)) {
      showToast("A file or folder already exists with that name.", true);
      return;
    }
    await sendFileCommand("files.create_directory", { path: targetPath });
    closeDialog(els.newFolderDialog);
    await browseFiles(state.currentFilesPath);
    showToast("Folder created.");
  } catch (error) {
    showToast(error.message, true);
  }
}

function openRenameDialog(item) {
  state.renameItem = item;
  els.renameName.value = itemName(item);
  els.renamePath.textContent = itemPath(item);
  openDialog(els.renameDialog);
  els.renameName.focus();
  els.renameName.select();
}

async function renameCurrentItem(event) {
  event.preventDefault();
  const item = state.renameItem;
  if (!item) return;
  const targetName = els.renameName.value.trim();
  if (!validFileName(targetName)) {
    showToast("Use a name without slashes.", true);
    return;
  }
  if (targetName === itemName(item)) {
    closeDialog(els.renameDialog);
    return;
  }

  const targetPath = joinPath(parentPath(itemPath(item)), targetName);
  try {
    const collision = await statItem(targetPath);
    if (collision) {
      closeDialog(els.renameDialog);
      const approved = await confirmAction({
        title: "Replace Existing Item",
        body: `An item already exists at ${targetPath}. Continue only if you want the file server to replace it or return an error if replacement is not supported.`,
        confirmLabel: "Continue",
        danger: true,
      });
      if (!approved) return;
    }

    await sendFileCommand("files.rename", { from: rawItemPath(item), to: targetPath });
    closeDialog(els.renameDialog);
    clearSelection();
    await refreshCurrentView();
    showToast("Item renamed.");
  } catch (error) {
    showToast(error.message, true);
  }
}

function openDeleteDialog(items) {
  const targets = items.filter(Boolean);
  if (!targets.length) {
    showToast("Select an item first.", true);
    return;
  }
  state.deleteItems = targets;
  els.deleteBody.innerHTML = `
    <p>${escapeHTML(targets.length === 1 ? "This item will be deleted." : `${targets.length} items will be deleted.`)}</p>
    ${renderPathList(targets.map(itemPath))}
  `;
  openDialog(els.deleteDialog);
}

async function deleteItemsFromDialog(event) {
  event.preventDefault();
  const targets = [...state.deleteItems];
  closeDialog(els.deleteDialog);
  try {
    for (const item of targets) {
      await sendFileCommand("files.delete", { path: rawItemPath(item), is_directory: Boolean(item.is_directory) });
    }
    clearSelection();
    await refreshCurrentView();
    showToast(targets.length === 1 ? "Item deleted." : `${targets.length} items deleted.`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function statItem(path, sourceID = selectedSourceID()) {
  try {
    const payload = await sendFileCommandForSource(sourceID, "files.stat", { path: normalizePath(path) });
    return payload.item || null;
  } catch (_) {
    return null;
  }
}

async function collectCollisions(targetPaths, sourceID = selectedSourceID()) {
  const collisions = [];
  for (const targetPath of targetPaths) {
    const item = await statItem(targetPath, sourceID);
    if (item) collisions.push({ path: normalizePath(targetPath), item });
  }
  return collisions;
}

function renderPathList(paths) {
  const items = paths.slice(0, 8).map((path) => `<li>${escapeHTML(normalizePath(path))}</li>`).join("");
  const extra = paths.length > 8 ? `<li>${paths.length - 8} more</li>` : "";
  return `<ul class="dialog-path-list">${items}${extra}</ul>`;
}

function confirmAction({ title, body, confirmLabel = "Continue", danger = false }) {
  return new Promise((resolve) => {
    state.confirmResolver = resolve;
    els.confirmTitle.textContent = title;
    els.confirmBody.innerHTML = `<p>${escapeHTML(body)}</p>`;
    els.confirmContinueButton.textContent = confirmLabel;
    els.confirmContinueButton.classList.toggle("danger", danger);
    openDialog(els.confirmDialog);
  });
}

function settleConfirmDialog(approved) {
  const resolver = state.confirmResolver;
  state.confirmResolver = null;
  closeDialog(els.confirmDialog);
  if (resolver) resolver(approved);
}

async function openMoveDialog(items) {
  const targets = items.filter(Boolean);
  if (!targets.length) {
    showToast("Select an item first.", true);
    return;
  }
  const activeSourceID = selectedSourceID();
  if (!activeSourceID) {
    showToast("Choose an SMB share first.", true);
    return;
  }
  state.moveItems = targets;
  state.moveSourceID = activeSourceID;
  state.moveDestinationSourceID = activeSourceID;
  renderMoveSourceSelect();
  openDialog(els.moveDialog);
  await browseMoveFolder(state.currentFilesPath);
}

function renderMoveSourceSelect() {
  if (!els.moveSourceSelect) return;
  const config = fileConfig();
  els.moveSourceSelect.innerHTML = "";
  config.sources.forEach((source) => {
    const option = document.createElement("option");
    option.value = source.id;
    option.textContent = sourceLabel(source);
    option.selected = source.id === state.moveDestinationSourceID;
    els.moveSourceSelect.appendChild(option);
  });
  els.moveSourceSelect.disabled = config.sources.length <= 1;
  els.moveSourceSelect.value = state.moveDestinationSourceID;
}

function moveDestinationSourceID() {
  return els.moveSourceSelect?.value || state.moveDestinationSourceID || state.moveSourceID;
}

async function browseMoveFolder(path) {
  const normalized = normalizePath(path);
  const destinationSourceID = moveDestinationSourceID();
  state.moveDestinationSourceID = destinationSourceID;
  state.moveDialogPath = normalized;
  els.movePath.value = normalized;
  els.moveFolders.className = "file-list compact empty-state";
  els.moveFolders.textContent = "Loading folders.";
  renderMoveTarget();
  renderMoveBreadcrumbs();
  try {
    const payload = await sendFileCommandForSource(destinationSourceID, "files.list", { path: normalized });
    const folders = (payload.items || []).filter((item) => item.is_directory);
    renderMoveFolders(folders);
  } catch (error) {
    els.moveFolders.className = "file-list compact empty-state";
    els.moveFolders.textContent = "Could not load folders.";
    showToast(error.message, true);
  }
}

function renderMoveTarget() {
  const count = state.moveItems.length;
  const destinationSourceID = moveDestinationSourceID();
  const reason = invalidMoveDestinationReason(state.moveItems, state.moveDialogPath, destinationSourceID);
  const config = fileConfig();
  const destinationSource = config.sources.find((source) => source.id === destinationSourceID);
  const shareName = destinationSource ? sourceLabel(destinationSource) : "selected share";
  els.moveTargetLabel.textContent = reason || `Move ${count === 1 ? itemName(state.moveItems[0]) : `${count} items`} to ${shareName}:${state.moveDialogPath}`;
  els.moveConfirmButton.disabled = Boolean(reason);
}

function renderMoveBreadcrumbs() {
  els.moveBreadcrumbs.innerHTML = "";
  const root = document.createElement("button");
  root.type = "button";
  root.textContent = "Main";
  root.addEventListener("click", () => browseMoveFolder("/"));
  els.moveBreadcrumbs.appendChild(root);

  const parts = normalizePath(state.moveDialogPath).split("/").filter(Boolean);
  let current = "";
  parts.forEach((part) => {
    current = `${current}/${part}`;
    const separator = document.createElement("span");
    separator.textContent = "/";
    els.moveBreadcrumbs.appendChild(separator);
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = part;
    const target = current;
    button.addEventListener("click", () => browseMoveFolder(target));
    els.moveBreadcrumbs.appendChild(button);
  });
}

function renderMoveFolders(folders) {
  if (!folders.length) {
    els.moveFolders.className = "file-list compact empty-state";
    els.moveFolders.textContent = "No folders here.";
    return;
  }
  els.moveFolders.className = "file-list compact";
  els.moveFolders.innerHTML = "";
  folders.forEach((folder) => {
    const row = document.createElement("article");
    row.className = "file-row directory compact-row";
    row.innerHTML = `
      <button type="button" class="file-main">
        <span class="file-icon" aria-hidden="true">DIR</span>
        <span>
          <strong>${escapeHTML(itemName(folder))}</strong>
          <span class="meta">${escapeHTML(itemPath(folder))}</span>
        </span>
      </button>
      <div class="item-actions"></div>
    `;
    row.querySelector(".file-main").addEventListener("click", () => browseMoveFolder(rawItemPath(folder)));
    row.querySelector(".item-actions").appendChild(actionButton("Open", () => browseMoveFolder(rawItemPath(folder)), "secondary"));
    els.moveFolders.appendChild(row);
  });
}

function invalidMoveDestinationReason(items, destination, destinationSourceID = moveDestinationSourceID()) {
  const normalizedDestination = normalizePath(destination);
  if (!items.length) return "Select an item first.";
  if (destinationSourceID === state.moveSourceID) {
    for (const item of items) {
      const source = itemPath(item);
      if (item.is_directory && (normalizedDestination === source || normalizedDestination.startsWith(`${source}/`))) {
        return "Choose a destination outside the folder being moved.";
      }
    }
    if (items.every((item) => normalizePath(joinPath(normalizedDestination, itemName(item))) === itemPath(item))) {
      return "These items are already in this folder.";
    }
  }
  return "";
}

async function moveItemsToDestination(items, destination, options = {}) {
  const targets = items.filter(Boolean);
  if (!targets.length) {
    showToast("Select an item first.", true);
    return;
  }
  const sourceID = state.moveSourceID || selectedSourceID();
  const destinationSourceID = moveDestinationSourceID();
  const normalizedDestination = normalizePath(destination);
  const reason = invalidMoveDestinationReason(targets, normalizedDestination, destinationSourceID);
  if (reason) {
    showToast(reason, true);
    return;
  }

  const moves = targets
    .map((item) => ({ item, to: joinPath(normalizedDestination, itemName(item)) }))
    .filter((move) => normalizePath(move.to) !== itemPath(move.item));
  if (!moves.length) {
    showToast("These items are already in this folder.", true);
    return;
  }

  if (options.confirmMove) {
    const approved = await confirmAction({
      title: "Move Items",
      body: `Move ${moves.length === 1 ? itemName(moves[0].item) : `${moves.length} items`} to ${normalizedDestination}?`,
      confirmLabel: "Move",
    });
    if (!approved) return;
  }

  const collisions = await collectCollisions(moves.map((move) => move.to), destinationSourceID);
  if (collisions.length) {
    closeDialog(els.moveDialog);
    const approved = await confirmAction({
      title: "Replace Existing Items",
      body: `${collisions.length} destination path${collisions.length === 1 ? "" : "s"} already exist: ${collisions.map((entry) => entry.path).join(", ")}`,
      confirmLabel: "Continue",
      danger: true,
    });
    if (!approved) return;
  }

  try {
    for (const move of moves) {
      const job = await sendFileCommandForSource(sourceID, "files.move", {
        destination_source_id: destinationSourceID,
        from: rawItemPath(move.item),
        to: move.to,
        is_directory: Boolean(move.item.is_directory),
      });
      if (job?.job_id) {
        state.fileJobs = [{ ...job, id: job.job_id }, ...state.fileJobs.filter((entry) => entry.id !== job.job_id)];
        renderFileJobs();
      }
    }
    closeDialog(els.moveDialog);
    clearSelection();
    await loadFileJobs();
    await refreshCurrentView();
    showToast(moves.length === 1 ? "Item moved." : `${moves.length} items moved.`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function submitMoveDialog(event) {
  event.preventDefault();
  await moveItemsToDestination(state.moveItems, state.moveDialogPath);
}

function dragHasExternalFiles(event) {
  return Array.from(event.dataTransfer?.types || []).includes("Files");
}

function dragHasFileItems(event) {
  return Array.from(event.dataTransfer?.types || []).includes(FILE_DRAG_TYPE);
}

function filesFromDragEvent(event) {
  return Array.from(event.dataTransfer?.files || []).filter((file) => file && file.name);
}

function itemsFromFileDrag(event) {
  try {
    const paths = JSON.parse(event.dataTransfer.getData(FILE_DRAG_TYPE) || "[]");
    return paths
      .map((path) => state.selectedItems.get(normalizePath(path)) || state.currentItems.find((item) => itemPath(item) === normalizePath(path)))
      .filter(Boolean);
  } catch (_) {
    return [];
  }
}

function setExplorerDragActive(isActive) {
  els.fileExplorerPanel?.classList.toggle("drag-active", isActive);
}

function installExplorerDropTarget() {
  if (!els.fileExplorerPanel) return;

  els.fileExplorerPanel.addEventListener("dragenter", (event) => {
    if (!dragHasExternalFiles(event)) return;
    event.preventDefault();
    state.dragDepth += 1;
    setExplorerDragActive(true);
  });

  els.fileExplorerPanel.addEventListener("dragover", (event) => {
    if (!dragHasExternalFiles(event)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
  });

  els.fileExplorerPanel.addEventListener("dragleave", (event) => {
    if (!dragHasExternalFiles(event)) return;
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

function installFileDrag(element, item) {
  element.addEventListener("dragstart", (event) => {
    if (!state.selectedItems.has(itemPath(item))) {
      selectOnlyItem(item);
    }
    const paths = selectedItems().map(itemPath);
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData(FILE_DRAG_TYPE, JSON.stringify(paths));
    element.classList.add("dragging");
  });
  element.addEventListener("dragend", () => {
    element.classList.remove("dragging");
    document.querySelectorAll(".move-target").forEach((target) => target.classList.remove("move-target"));
  });
}

function installFolderDropTarget(row, path) {
  row.addEventListener("dragenter", (event) => {
    if (!dragHasExternalFiles(event) && !dragHasFileItems(event)) return;
    event.preventDefault();
    row.classList.add(dragHasFileItems(event) ? "move-target" : "drop-target");
  });
  row.addEventListener("dragover", (event) => {
    if (!dragHasExternalFiles(event) && !dragHasFileItems(event)) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = dragHasFileItems(event) ? "move" : "copy";
    row.classList.add(dragHasFileItems(event) ? "move-target" : "drop-target");
  });
  row.addEventListener("dragleave", () => {
    row.classList.remove("drop-target", "move-target");
  });
  row.addEventListener("drop", async (event) => {
    if (!dragHasExternalFiles(event) && !dragHasFileItems(event)) return;
    event.preventDefault();
    event.stopPropagation();
    row.classList.remove("drop-target", "move-target");
    state.dragDepth = 0;
    setExplorerDragActive(false);

    if (dragHasFileItems(event)) {
      await moveItemsToDestination(itemsFromFileDrag(event), path, { confirmMove: true });
      return;
    }

    await uploadFiles(filesFromDragEvent(event), path);
  });
}

async function fetchFileBlob(path, resultLabel) {
  selectedHomeOrThrow();
  const setup = await api("/v1/home/files/downloads", {
    method: "POST",
    body: JSON.stringify(withSourceID({ path })),
  });
  const response = await fetch(setup.url, {
    headers: { "Authorization": `Bearer ${setup.transfer_token}` },
  });
  if (!response.ok) throw new Error(await response.text());
  const blob = await response.blob();
  state.lastTransfer = {
    ...setup,
    result: resultLabel || `downloaded ${formatBytes(blob.size)}`,
    completed_at: new Date().toISOString(),
  };
  renderLastTransfer();
  return blob;
}

async function downloadFile(path, options = {}) {
  try {
    const normalized = normalizePath(path);
    const blob = await fetchFileBlob(normalized, `downloaded ${formatBytes(0)}`);
    state.lastTransfer.result = `downloaded ${formatBytes(blob.size)}`;
    renderLastTransfer();
    const href = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = href;
    link.download = normalized.split("/").pop() || "download";
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(href);
    if (options.toast !== false) showToast(`Downloaded ${link.download}.`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function downloadSelected() {
  const files = selectedItems().filter((item) => !item.is_directory);
  if (!files.length) {
    showToast("Select one or more files to download.", true);
    return;
  }
  for (const item of files) {
    await downloadFile(rawItemPath(item), { toast: files.length === 1 });
  }
  if (files.length > 1) showToast(`Started ${files.length} downloads.`);
}

function previewMimeForItem(item) {
  const extension = extensionForItem(item);
  if (imageExtensions.has(extension)) return extension === "svg" ? "image/svg+xml" : `image/${extension === "jpg" ? "jpeg" : extension}`;
  if (extension === "pdf") return "application/pdf";
  if (extension === "json") return "application/json";
  if (extension === "html" || extension === "htm") return "text/html";
  return "text/plain";
}

function cleanupPreviewURL() {
  if (state.previewObjectURL) {
    URL.revokeObjectURL(state.previewObjectURL);
    state.previewObjectURL = "";
  }
}

function renderPreview() {
  state.previewRequestID += 1;
  const requestID = state.previewRequestID;
  cleanupPreviewURL();
  els.previewActions.innerHTML = "";

  const items = selectedItems();
  if (items.length > 1) {
    els.previewTitle.textContent = `${items.length} items selected`;
    els.previewMeta.textContent = `${items.filter((item) => item.is_directory).length} folders, ${items.filter((item) => !item.is_directory).length} files`;
    els.previewBody.className = "file-preview-body";
    els.previewBody.innerHTML = renderPathList(items.map(itemPath));
    els.previewActions.appendChild(actionButton("Move", () => openMoveDialog(items), "secondary"));
    els.previewActions.appendChild(actionButton("Delete", () => openDeleteDialog(items), "ghost"));
    return;
  }

  const item = items[0];
  if (!item) {
    els.previewTitle.textContent = "Details";
    els.previewMeta.textContent = "Select a file or folder.";
    els.previewBody.className = "file-preview-body empty-state";
    els.previewBody.textContent = "No file selected.";
    return;
  }

  els.previewTitle.textContent = itemName(item);
  els.previewMeta.textContent = `${fileTypeLabel(item)} - ${itemPath(item)}`;
  renderPreviewActions(item);

  if (item.is_directory) {
    els.previewBody.className = "file-preview-body";
    els.previewBody.innerHTML = `
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Path</div><div>${escapeHTML(itemPath(item))}</div></div>
        <div class="kv-row"><div class="kv-label">Modified</div><div>${escapeHTML(formatDate(item.modified_at))}</div></div>
      </div>
    `;
    return;
  }

  const size = Number(item.size || 0);
  const extension = extensionForItem(item);
  if (imageExtensions.has(extension) && size <= RICH_PREVIEW_LIMIT) {
    loadRichPreview(item, requestID, "image");
    return;
  }
  if (extension === "pdf" && size <= RICH_PREVIEW_LIMIT) {
    loadRichPreview(item, requestID, "pdf");
    return;
  }
  if (textExtensions.has(extension) && size <= TEXT_PREVIEW_LIMIT) {
    loadTextPreview(item, requestID);
    return;
  }

  els.previewBody.className = "file-preview-body empty-state";
  els.previewBody.textContent = size > RICH_PREVIEW_LIMIT ? "This file is too large for automatic preview." : "Preview is not available for this file type.";
}

function renderPreviewActions(item) {
  els.previewActions.innerHTML = "";
  if (item.is_directory) {
    els.previewActions.appendChild(actionButton("Open", () => browseFiles(rawItemPath(item))));
  } else {
    els.previewActions.appendChild(actionButton("Download", () => downloadFile(rawItemPath(item)), "secondary"));
  }
  els.previewActions.appendChild(actionButton("Move", () => openMoveDialog([item]), "secondary"));
  els.previewActions.appendChild(actionButton("Rename", () => openRenameDialog(item), "secondary"));
  els.previewActions.appendChild(actionButton("Delete", () => openDeleteDialog([item]), "ghost"));
}

async function loadRichPreview(item, requestID, kind) {
  els.previewBody.className = "file-preview-body empty-state";
  els.previewBody.textContent = "Loading preview.";
  try {
    const blob = await fetchFileBlob(rawItemPath(item), `previewed ${itemName(item)}`);
    if (requestID !== state.previewRequestID) return;
    if (blob.size > RICH_PREVIEW_LIMIT) {
      els.previewBody.textContent = "This file is too large for automatic preview.";
      return;
    }
    const typedBlob = new Blob([blob], { type: previewMimeForItem(item) });
    state.previewObjectURL = URL.createObjectURL(typedBlob);
    els.previewBody.className = "file-preview-body";
    if (kind === "image") {
      els.previewBody.innerHTML = `<img class="file-preview-image" src="${state.previewObjectURL}" alt="">`;
      return;
    }
    els.previewBody.innerHTML = `<iframe class="file-preview-frame" src="${state.previewObjectURL}" title="${escapeHTML(itemName(item))}"></iframe>`;
  } catch (error) {
    if (requestID !== state.previewRequestID) return;
    els.previewBody.className = "file-preview-body empty-state";
    els.previewBody.textContent = error.message;
  }
}

async function loadTextPreview(item, requestID) {
  els.previewBody.className = "file-preview-body empty-state";
  els.previewBody.textContent = "Loading preview.";
  try {
    const blob = await fetchFileBlob(rawItemPath(item), `previewed ${itemName(item)}`);
    if (requestID !== state.previewRequestID) return;
    if (blob.size > TEXT_PREVIEW_LIMIT) {
      els.previewBody.textContent = "This text file is too large for automatic preview.";
      return;
    }
    const text = await blob.text();
    if (requestID !== state.previewRequestID) return;
    els.previewBody.className = "file-preview-body";
    const pre = document.createElement("pre");
    pre.className = "file-preview-text";
    pre.textContent = text;
    els.previewBody.replaceChildren(pre);
  } catch (error) {
    if (requestID !== state.previewRequestID) return;
    els.previewBody.className = "file-preview-body empty-state";
    els.previewBody.textContent = error.message;
  }
}

async function uploadOneFile(file, targetPath) {
  const setup = await api("/v1/home/files/uploads", {
    method: "POST",
    body: JSON.stringify(withSourceID({ path: targetPath, size: file.size || 0 })),
  });
  const response = await fetch(setup.url, {
    method: "PUT",
    headers: {
      "Authorization": `Bearer ${setup.transfer_token}`,
      "Content-Type": "application/octet-stream",
    },
    body: file,
  });
  const contentType = response.headers.get("Content-Type") || "";
  const payload = contentType.includes("application/json") ? await response.json() : await response.text();
  if (!response.ok) {
    const message = typeof payload === "string" ? payload.trim() : payload.error || payload.message;
    throw new Error(message || response.statusText);
  }
  if (typeof payload !== "object" || payload === null) {
    throw new Error("The file server returned an unexpected upload response.");
  }
  state.lastTransfer = {
    ...setup,
    path: targetPath,
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
    const targetPaths = selectedFiles.map((file) => joinPath(targetFolder, file.name));
    const collisions = await collectCollisions(targetPaths);
    if (collisions.length) {
      const approved = await confirmAction({
        title: "Replace Existing Files",
        body: `${collisions.length} upload target${collisions.length === 1 ? "" : "s"} already exist: ${collisions.map((entry) => entry.path).join(", ")}`,
        confirmLabel: "Upload",
        danger: true,
      });
      if (!approved) return;
    }
    for (let index = 0; index < selectedFiles.length; index += 1) {
      await uploadOneFile(selectedFiles[index], targetPaths[index]);
    }
    els.filesUpload.value = "";
    renderLastTransfer();
    await loadFileJobs();
    await browseFiles(state.currentFilesPath, { keepSelection: true });
    showToast(selectedFiles.length === 1 ? `Uploaded ${selectedFiles[0].name}.` : `Uploaded ${selectedFiles.length} files.`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function browseRequestedPath() {
  const targetPath = requestedFilesPath();
  state.targetFilesPath = targetPath === "/" ? "" : targetPath;
  state.targetIsDirectory = requestedPathIsDirectory();
  if (!state.targetFilesPath) {
    await browseFiles("/");
    return;
  }
  if (state.targetIsDirectory) {
    await browseFiles(state.targetFilesPath);
    return;
  }
  await browseFiles(parentPath(state.targetFilesPath));
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
          <div class="card-title">${escapeHTML(transfer.operation)} - ${escapeHTML(transfer.path)}</div>
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

async function loadFileJobs() {
  if (!selectedHome()) {
    state.fileJobs = [];
    renderFileJobs();
    return;
  }
  try {
    const payload = await api("/v1/home/file-jobs?limit=10");
    state.fileJobs = payload.jobs || [];
  } catch (_) {
    state.fileJobs = [];
  }
  renderFileJobs();
}

function renderFileJobs() {
  if (!state.fileJobs.length) {
    els.fileJobsOutput.className = "card-list empty-state";
    els.fileJobsOutput.textContent = "No managed file jobs yet.";
    return;
  }
  els.fileJobsOutput.className = "card-list";
  els.fileJobsOutput.innerHTML = state.fileJobs.map((job) => {
    const progress = job.files_total > 0
      ? `${job.files_done || 0}/${job.files_total} files`
      : job.bytes_total > 0
        ? `${formatBytes(job.bytes_done)} / ${formatBytes(job.bytes_total)}`
        : job.status;
    const canRetry = ["failed", "rollback_required", "cancelled"].includes(job.status);
    const canCancel = ["queued", "running"].includes(job.status);
    return `
      <article class="card split-card">
        <div class="card-head">
          <div>
            <div class="card-title">${escapeHTML(job.operation)} - ${escapeHTML(job.from_path || job.to_path || job.id)}</div>
            <div class="meta">${escapeHTML(job.id)}</div>
          </div>
          <span class="pill">${escapeHTML(job.status)}</span>
        </div>
        <div class="kv-grid">
          <div class="kv-row"><div class="kv-label">To</div><div>${escapeHTML(job.to_path || "")}</div></div>
          <div class="kv-row"><div class="kv-label">Progress</div><div>${escapeHTML(progress)}</div></div>
          <div class="kv-row"><div class="kv-label">Updated</div><div>${escapeHTML(formatDate(job.updated_at))}</div></div>
          ${job.error_message ? `<div class="kv-row"><div class="kv-label">Error</div><div>${escapeHTML(job.error_message)}</div></div>` : ""}
        </div>
        <div class="actions wrap compact">
          ${canRetry ? `<button type="button" class="secondary" data-file-job-action="retry" data-file-job-id="${escapeHTML(job.id)}">Retry</button>` : ""}
          ${canCancel ? `<button type="button" class="ghost" data-file-job-action="cancel" data-file-job-id="${escapeHTML(job.id)}">Cancel</button>` : ""}
        </div>
      </article>
    `;
  }).join("");
}

async function runFileJobAction(jobID, action) {
  if (!jobID || !action) return;
  try {
    await api(`/v1/home/file-jobs/${encodeURIComponent(jobID)}/${action}`, { method: "POST" });
    await loadFileJobs();
  } catch (error) {
    showToast(error.message, true);
  }
}

function defaultSortDirection(key) {
  return key === "modified" || key === "size" ? "desc" : "asc";
}

function syncSortControl() {
  const value = `${state.sortKey}-${state.sortDirection}`;
  if (Array.from(els.filesSort.options).some((option) => option.value === value)) {
    els.filesSort.value = value;
  }
}

function setSort(key, direction = "") {
  const nextKey = key || "name";
  let nextDirection = direction;
  if (!nextDirection) {
    nextDirection = state.sortKey === nextKey
      ? state.sortDirection === "asc" ? "desc" : "asc"
      : defaultSortDirection(nextKey);
  }
  state.sortKey = nextKey;
  state.sortDirection = nextDirection === "desc" ? "desc" : "asc";
  syncSortControl();
  applySortAndRender();
}

function setSortFromControl() {
  const [key, direction] = String(els.filesSort.value || "name-asc").split("-");
  setSort(key, direction);
}

function setViewMode(mode) {
  state.viewMode = mode === "grid" ? "grid" : "list";
  els.filesListViewButton.classList.toggle("active", state.viewMode === "list");
  els.filesGridViewButton.classList.toggle("active", state.viewMode === "grid");
  els.filesListViewButton.setAttribute("aria-pressed", String(state.viewMode === "list"));
  els.filesGridViewButton.setAttribute("aria-pressed", String(state.viewMode === "grid"));
  renderFiles();
  renderSelectionState();
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
    await loadProfiles();
    renderLastTransfer();
    await loadFileJobs();
    renderPathStatus();
    renderBreadcrumbs();
    renderPreview();
    if (selectedHome()) {
      await browseRequestedPath();
    }
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.homeSelect.addEventListener("change", async () => {
  state.activeSourceID = "";
  syncURL(selectedHomeID(), "");
  closeAppSocket();
  resetSearch();
  clearSelection();
  await loadProfiles();
  await loadFileJobs();
  await browseFiles("/");
});
els.sourceSelect?.addEventListener("change", () => switchSource(selectedSourceID()));
els.filesSearchButton.addEventListener("click", () => searchFiles());
els.filesSearchClearButton.addEventListener("click", () => browseFiles(state.currentFilesPath));
els.filesSearch.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    searchFiles();
  }
});
els.filesRefreshCurrentButton.addEventListener("click", () => refreshCurrentView());
els.filesPath.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    browseFiles(els.filesPath.value);
  }
});
els.filesNewFolderButton.addEventListener("click", openNewFolderDialog);
els.filesUploadButton.addEventListener("click", () => els.filesUpload.click());
els.filesUpload.addEventListener("change", () => uploadFiles(els.filesUpload.files, state.currentFilesPath));
els.fileJobsRefreshButton?.addEventListener("click", loadFileJobs);
els.fileJobsOutput?.addEventListener("click", (event) => {
  const button = event.target.closest("[data-file-job-action]");
  if (!button) return;
  runFileJobAction(button.dataset.fileJobId, button.dataset.fileJobAction);
});
els.filesSort.addEventListener("change", setSortFromControl);
els.filesSelectAll.addEventListener("change", () => selectAllVisible(els.filesSelectAll.checked));
els.filesListViewButton.addEventListener("click", () => setViewMode("list"));
els.filesGridViewButton.addEventListener("click", () => setViewMode("grid"));
els.clearSelectionButton.addEventListener("click", clearSelection);
els.downloadSelectedButton.addEventListener("click", downloadSelected);
els.moveSelectedButton.addEventListener("click", () => openMoveDialog(selectedItems()));
els.renameSelectedButton.addEventListener("click", () => {
  const items = selectedItems();
  if (items.length === 1) openRenameDialog(items[0]);
});
els.deleteSelectedButton.addEventListener("click", () => openDeleteDialog(selectedItems()));

els.newFolderForm.addEventListener("submit", createDirectoryFromDialog);
els.newFolderCancelButton.addEventListener("click", () => closeDialog(els.newFolderDialog));
els.renameForm.addEventListener("submit", renameCurrentItem);
els.renameCancelButton.addEventListener("click", () => closeDialog(els.renameDialog));
els.moveForm.addEventListener("submit", submitMoveDialog);
els.moveCancelButton.addEventListener("click", () => closeDialog(els.moveDialog));
els.moveOpenButton.addEventListener("click", () => browseMoveFolder(els.movePath.value));
els.moveSourceSelect?.addEventListener("change", () => browseMoveFolder(els.movePath.value));
els.movePath.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    browseMoveFolder(els.movePath.value);
  }
});
els.deleteForm.addEventListener("submit", deleteItemsFromDialog);
els.deleteCancelButton.addEventListener("click", () => closeDialog(els.deleteDialog));
els.confirmForm.addEventListener("submit", (event) => {
  event.preventDefault();
  settleConfirmDialog(true);
});
els.confirmCancelButton.addEventListener("click", () => settleConfirmDialog(false));
els.confirmDialog.addEventListener("cancel", (event) => {
  event.preventDefault();
  settleConfirmDialog(false);
});

installExplorerDropTarget();
hydrate();
