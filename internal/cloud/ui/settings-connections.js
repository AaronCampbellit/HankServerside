const api = window.HankAPI.request;

const state = {
  user: null,
  homes: [],
  members: [],
  profiles: [],
  smbShares: [],
  editingSMBShareID: "",
  folders: [],
  editingFolderID: "",
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
  haBaseURL: document.getElementById("ha-base-url"),
  haTimeoutSeconds: document.getElementById("ha-timeout-seconds"),
  haToken: document.getElementById("ha-token"),
  haPersist: document.getElementById("ha-persist"),
  settingsRole: document.getElementById("settings-role"),
  smbForm: document.getElementById("smb-form"),
  smbSharesList: document.getElementById("smb-shares-list"),
  smbAddShareButton: document.getElementById("smb-add-share-button"),
  smbRemoveShareButton: document.getElementById("smb-remove-share-button"),
  smbName: document.getElementById("smb-name"),
  smbHost: document.getElementById("smb-host"),
  smbShare: document.getElementById("smb-share"),
  smbDomain: document.getElementById("smb-domain"),
  smbUsername: document.getElementById("smb-username"),
  smbPassword: document.getElementById("smb-password"),
  smbPersist: document.getElementById("smb-persist"),
  folderForm: document.getElementById("folder-form"),
  folderList: document.getElementById("folder-list"),
  folderAddButton: document.getElementById("folder-add-button"),
  folderRemoveButton: document.getElementById("folder-remove-button"),
  folderName: document.getElementById("folder-name"),
  folderRoot: document.getElementById("folder-root"),
  folderCreate: document.getElementById("folder-create"),
  folderPersist: document.getElementById("folder-persist"),
  toast: document.getElementById("toast"),
};


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

function normalizeHomeAssistantURL(value) {
  return String(value || "").trim().replace(/\/+$/, "");
}

function homeAssistantConfig(profile) {
  const config = profileConfig(profile);
  return {
    base_url: config.base_url || config.url || "",
    timeout_seconds: Number(config.timeout_seconds || 10) || 10,
  };
}

function cleanSourceID(value) {
  return String(value || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_.-]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function shareIDFromParts(share, existingIDs = new Set()) {
  const base = cleanSourceID(share.id || share.name || share.share || share.host || "smb");
  let candidate = base || "smb";
  let index = 2;
  while (existingIDs.has(candidate)) {
    candidate = `${base || "smb"}-${index}`;
    index += 1;
  }
  return candidate;
}

function normalizeShare(entry, index) {
  const host = entry?.host || entry?.smb_host || "";
  const share = entry?.share || entry?.smb_share || "";
  const id = cleanSourceID(entry?.id || entry?.source_id || entry?.name || share || `smb-${index + 1}`);
  if (!id && !host && !share) return null;
  return {
    id: id || `smb-${index + 1}`,
    name: entry?.name || share || host || `SMB Share ${index + 1}`,
    host,
    share,
    domain: entry?.domain || entry?.smb_domain || "",
    username: entry?.username || entry?.smb_username || "",
    password_set: Boolean(entry?.password_set ?? entry?.smb_password_set),
  };
}

function sharesFromConfig(config) {
  const shares = [];
  const rawShares = Array.isArray(config.shares) ? config.shares : [];
  rawShares.forEach((entry, index) => {
    const share = normalizeShare(entry, index);
    if (share) shares.push(share);
  });

  const rawSources = Array.isArray(config.file_sources) ? config.file_sources
    : Array.isArray(config.sources) ? config.sources
      : [];
  rawSources.forEach((entry, index) => {
    if (entry.type && entry.type !== "smb") return;
    const share = normalizeShare(entry, shares.length + index);
    if (share && !shares.some((candidate) => candidate.id === share.id)) {
      shares.push(share);
    }
  });

  const host = config.host || config.smb_host || "";
  const shareName = config.share || config.smb_share || "";
  if ((host || shareName) && !shares.length) {
    const share = normalizeShare({
      id: config.active_source_id || "smb",
      name: config.name || shareName || "SMB Share",
      host,
      share: shareName,
      domain: config.domain || config.smb_domain || "",
      username: config.username || config.smb_username || "",
      password_set: config.smb_password_set,
    }, 0);
    if (share) shares.push(share);
  }

  return shares;
}

function currentSMBShare() {
  return state.smbShares.find((share) => share.id === state.editingSMBShareID) || null;
}

function shareLabel(share) {
  if (!share) return "New Share";
  const target = [share.host, share.share].filter(Boolean).join("/");
  return share.name && share.name !== share.share ? `${share.name} (${target || share.id})` : target || share.name || share.id;
}

function normalizeFolder(entry, index) {
  const root = String(entry?.root || "").trim();
  const id = cleanSourceID(entry?.id || entry?.source_id || entry?.name || `folder-${index + 1}`);
  if (!id && !root) return null;
  return {
    id: id || `folder-${index + 1}`,
    name: entry?.name || root || `Host Folder ${index + 1}`,
    root,
  };
}

function foldersFromConfig(config) {
  const folders = [];
  const rawFolders = Array.isArray(config.folders) ? config.folders : [];
  rawFolders.forEach((entry, index) => {
    const folder = normalizeFolder(entry, index);
    if (folder) folders.push(folder);
  });

  if (!folders.length) {
    const rawSources = Array.isArray(config.file_sources) ? config.file_sources
      : Array.isArray(config.sources) ? config.sources
        : [];
    rawSources.forEach((entry, index) => {
      if (entry.type !== "local") return;
      const folder = normalizeFolder({
        id: entry.id || entry.source_id,
        name: entry.name,
        root: entry.root,
      }, folders.length + index);
      if (folder && !folders.some((candidate) => candidate.id === folder.id)) {
        folders.push(folder);
      }
    });
  }

  return folders;
}

function currentFolder() {
  return state.folders.find((folder) => folder.id === state.editingFolderID) || null;
}

function folderLabel(folder) {
  if (!folder) return "New Folder";
  return folder.name && folder.name !== folder.root ? `${folder.name} (${folder.root || folder.id})` : folder.root || folder.name || folder.id;
}

function folderIDFromParts(folder, existingIDs = new Set()) {
  const base = cleanSourceID(folder.id || folder.name || folder.root || "folder");
  let candidate = base || "folder";
  let index = 2;
  while (existingIDs.has(candidate)) {
    candidate = `${base || "folder"}-${index}`;
    index += 1;
  }
  return candidate;
}

function validHostPath(value) {
  return String(value || "").trim().startsWith("/");
}

// fileServerPublicConfig builds the public config for the shared "smb" service
// profile, carrying both network shares and host folders so a save of one does
// not drop the other.
function fileServerPublicConfig(shares, folders) {
  const publicShares = (shares || []).map((share) => ({
    id: share.id,
    name: share.name,
    host: share.host,
    share: share.share,
    domain: share.domain,
    username: share.username,
  }));
  const publicFolders = (folders || []).map((folder) => ({
    id: folder.id,
    name: folder.name,
    type: "local",
    root: folder.root,
    create: Boolean(folder.create),
  }));
  const primary = publicShares[0] || {};
  return {
    active_source_id: primary.id || publicFolders[0]?.id || "",
    host: primary.host || "",
    share: primary.share || "",
    domain: primary.domain || "",
    username: primary.username || "",
    shares: publicShares,
    folders: publicFolders,
  };
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
  els.hermesForm?.querySelectorAll("input, textarea, button").forEach((element) => {
    element.disabled = !admin;
  });
  els.smbForm.querySelectorAll("input, button").forEach((element) => {
    element.disabled = !admin;
  });
  els.smbAddShareButton.disabled = !admin;
  els.smbRemoveShareButton.disabled = !admin || !currentSMBShare();
  els.folderForm.querySelectorAll("input, button").forEach((element) => {
    element.disabled = !admin;
  });
  els.folderAddButton.disabled = !admin;
  els.folderRemoveButton.disabled = !admin || !currentFolder();

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
  const haConfig = homeAssistantConfig(homeAssistant);
  els.haBaseURL.value = haConfig.base_url;
  els.haTimeoutSeconds.value = haConfig.timeout_seconds;
  els.haToken.value = "";
  state.smbShares = sharesFromConfig(smbConfig);
  if (!state.smbShares.some((share) => share.id === state.editingSMBShareID)) {
    state.editingSMBShareID = state.smbShares[0]?.id || "";
  }
  renderSMBSharesList();
  fillSMBForm(currentSMBShare());

  state.folders = foldersFromConfig(smbConfig);
  if (!state.folders.some((folder) => folder.id === state.editingFolderID)) {
    state.editingFolderID = state.folders[0]?.id || "";
  }
  renderFolderList();
  fillFolderForm(currentFolder());
}

function renderSMBSharesList() {
  const admin = isAdmin();
  if (!state.smbShares.length) {
    els.smbSharesList.className = "card-list empty-state";
    els.smbSharesList.textContent = "No file server shares saved.";
    els.smbRemoveShareButton.disabled = true;
    return;
  }

  els.smbSharesList.className = "card-list";
  els.smbSharesList.innerHTML = "";
  state.smbShares.forEach((share) => {
    const card = document.createElement("article");
    card.className = `card split-card ${share.id === state.editingSMBShareID ? "selected" : ""}`;
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(shareLabel(share))}</div>
          <div class="meta">${share.id === state.editingSMBShareID ? "Editing" : "Saved share"}</div>
        </div>
        <span class="status-chip ${share.password_set ? "" : "offline"}">${share.password_set ? "password saved" : "password needed"}</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Server</div><div>${escapeHTML(share.host || "Not set")}</div></div>
        <div class="kv-row"><div class="kv-label">Share</div><div>${escapeHTML(share.share || "Not set")}</div></div>
        <div class="kv-row"><div class="kv-label">Username</div><div>${escapeHTML(share.username || "Not set")}</div></div>
      </div>
      <div class="item-actions">
        <button type="button" class="secondary" data-smb-share-id="${escapeHTML(share.id)}" ${admin ? "" : "disabled"}>${share.id === state.editingSMBShareID ? "Selected" : "Edit"}</button>
      </div>
    `;
    els.smbSharesList.appendChild(card);
  });
  els.smbSharesList.querySelectorAll("[data-smb-share-id]").forEach((button) => {
    button.addEventListener("click", () => selectSMBShare(button.dataset.smbShareId));
  });
  els.smbRemoveShareButton.disabled = !admin || !currentSMBShare();
}

function fillSMBForm(share) {
  els.smbName.value = share?.name || "";
  els.smbHost.value = share?.host || "";
  els.smbShare.value = share?.share || "";
  els.smbDomain.value = share?.domain || "";
  els.smbUsername.value = share?.username || "";
  els.smbPassword.value = "";
}

function selectSMBShare(id) {
  state.editingSMBShareID = id || "";
  renderSMBSharesList();
  fillSMBForm(currentSMBShare());
}

function addSMBShare() {
  state.editingSMBShareID = "";
  fillSMBForm(null);
  renderSMBSharesList();
  els.smbName.focus();
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
    const baseURL = normalizeHomeAssistantURL(els.haBaseURL.value);
    const timeoutSeconds = Number.parseInt(els.haTimeoutSeconds.value, 10) || 10;
    if (!baseURL) {
      showToast("Home Assistant address is required.", true);
      return;
    }
    const payload = {
      public_config: {
        base_url: baseURL,
        timeout_seconds: timeoutSeconds,
      },
      persist: els.haPersist.checked,
    };
    if (els.haToken.value.trim()) {
      payload.secrets = { token: els.haToken.value.trim() };
    }
    await api("/v1/home/service-profiles/homeassistant", {
      method: "PUT",
      body: JSON.stringify(payload),
    });
    els.haToken.value = "";
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
  const shareDraft = {
    id: state.editingSMBShareID,
    name: els.smbName.value.trim(),
    host: normalizeSMBHostInput(els.smbHost.value),
    share: els.smbShare.value.trim(),
    domain: els.smbDomain.value.trim(),
    username: els.smbUsername.value.trim(),
  };
  if (!shareDraft.host || !shareDraft.share) {
    showToast("Server address and share name are required.", true);
    return;
  }
  const existingIDs = new Set(state.smbShares.map((share) => share.id).filter((id) => id !== state.editingSMBShareID));
  shareDraft.id = state.editingSMBShareID || shareIDFromParts(shareDraft, existingIDs);
  shareDraft.name = shareDraft.name || shareDraft.share || shareDraft.id;
  els.smbHost.value = shareDraft.host;

  const existingShare = currentSMBShare() || {};
  const nextShares = state.smbShares.filter((share) => share.id !== state.editingSMBShareID && share.id !== shareDraft.id);
  nextShares.push({
    ...existingShare,
    ...shareDraft,
    password_set: Boolean(els.smbPassword.value || existingShare.password_set),
  });
  nextShares.sort((left, right) => shareLabel(left).localeCompare(shareLabel(right), undefined, { sensitivity: "base" }));

  const publicConfig = fileServerPublicConfig(nextShares, state.folders);
  const payload = {
    public_config: publicConfig,
    persist: els.smbPersist.checked,
  };
  if (els.smbPassword.value) {
    payload.secrets = { shares: [{ id: shareDraft.id, password: els.smbPassword.value }] };
  }
  try {
    await api("/v1/home/service-profiles/smb", {
      method: "PUT",
      body: JSON.stringify(payload),
    });
    state.editingSMBShareID = shareDraft.id;
    els.smbPassword.value = "";
    await loadProfiles();
    showToast("File server share saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

function renderFolderList() {
  const admin = isAdmin();
  if (!state.folders.length) {
    els.folderList.className = "card-list empty-state";
    els.folderList.textContent = "No host folders saved.";
    els.folderRemoveButton.disabled = true;
    return;
  }

  els.folderList.className = "card-list";
  els.folderList.innerHTML = "";
  state.folders.forEach((folder) => {
    const card = document.createElement("article");
    card.className = `card split-card ${folder.id === state.editingFolderID ? "selected" : ""}`;
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(folderLabel(folder))}</div>
          <div class="meta">${folder.id === state.editingFolderID ? "Editing" : "Saved folder"}</div>
        </div>
        <span class="status-chip">host folder</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Host Path</div><div>${escapeHTML(folder.root || "Not set")}</div></div>
      </div>
      <div class="item-actions">
        <button type="button" data-folder-id="${escapeHTML(folder.id)}" class="secondary" ${admin ? "" : "disabled"}>${folder.id === state.editingFolderID ? "Selected" : "Edit"}</button>
      </div>
    `;
    els.folderList.appendChild(card);
  });
  els.folderList.querySelectorAll("[data-folder-id]").forEach((button) => {
    button.addEventListener("click", () => selectFolder(button.dataset.folderId));
  });
  els.folderRemoveButton.disabled = !admin || !currentFolder();
}

function fillFolderForm(folder) {
  els.folderName.value = folder?.name || "";
  els.folderRoot.value = folder?.root || "";
  els.folderCreate.checked = true;
}

function selectFolder(id) {
  state.editingFolderID = id || "";
  renderFolderList();
  fillFolderForm(currentFolder());
}

function addFolder() {
  state.editingFolderID = "";
  fillFolderForm(null);
  renderFolderList();
  els.folderName.focus();
}

async function saveFolderSettings(event) {
  event.preventDefault();
  if (!isAdmin()) {
    showToast("Only admins can change file server settings.", true);
    return;
  }
  const root = els.folderRoot.value.trim();
  if (!validHostPath(root)) {
    showToast("Enter an absolute host path that starts with /.", true);
    return;
  }
  const existingIDs = new Set(state.folders.map((folder) => folder.id).filter((id) => id !== state.editingFolderID));
  const draft = {
    id: state.editingFolderID || folderIDFromParts({ name: els.folderName.value.trim(), root }, existingIDs),
    name: els.folderName.value.trim() || root,
    root,
    create: els.folderCreate.checked,
  };

  const nextFolders = state.folders
    .filter((folder) => folder.id !== state.editingFolderID && folder.id !== draft.id)
    .concat(draft)
    .sort((left, right) => folderLabel(left).localeCompare(folderLabel(right), undefined, { sensitivity: "base" }));

  try {
    await api("/v1/home/service-profiles/smb", {
      method: "PUT",
      body: JSON.stringify({
        public_config: fileServerPublicConfig(state.smbShares, nextFolders),
        persist: els.folderPersist.checked,
      }),
    });
    state.editingFolderID = draft.id;
    await loadProfiles();
    showToast("Host folder saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function removeFolder() {
  if (!isAdmin()) {
    showToast("Only admins can change file server settings.", true);
    return;
  }
  const folder = currentFolder();
  if (!folder) return;
  const nextFolders = state.folders.filter((candidate) => candidate.id !== folder.id);
  try {
    await api("/v1/home/service-profiles/smb", {
      method: "PUT",
      body: JSON.stringify({
        public_config: fileServerPublicConfig(state.smbShares, nextFolders),
        persist: els.folderPersist.checked,
      }),
    });
    state.editingFolderID = nextFolders[0]?.id || "";
    await loadProfiles();
    showToast("Host folder removed.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function removeSMBShare() {
  if (!isAdmin()) {
    showToast("Only admins can change file server settings.", true);
    return;
  }
  const share = currentSMBShare();
  if (!share) return;
  const nextShares = state.smbShares.filter((candidate) => candidate.id !== share.id);
  try {
    await api("/v1/home/service-profiles/smb", {
      method: "PUT",
      body: JSON.stringify({
        public_config: fileServerPublicConfig(nextShares, state.folders),
        persist: els.smbPersist.checked,
      }),
    });
    state.editingSMBShareID = nextShares[0]?.id || "";
    await loadProfiles();
    showToast("File server share removed.");
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
els.smbAddShareButton.addEventListener("click", addSMBShare);
els.smbRemoveShareButton.addEventListener("click", removeSMBShare);
els.folderForm.addEventListener("submit", saveFolderSettings);
els.folderAddButton.addEventListener("click", addFolder);
els.folderRemoveButton.addEventListener("click", removeFolder);

hydrate();
