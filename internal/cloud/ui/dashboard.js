const state = {
  user: null,
  homes: [],
  agents: [],
  tokensByHome: new Map(),
  refreshTimer: 0,
  appSocket: null,
  appSocketPromise: null,
  pendingRequests: new Map(),
  requestCounter: 0,
  currentNoteRevision: "",
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
  haHealthButton: document.getElementById("ha-health-button"),
  haStatesButton: document.getElementById("ha-states-button"),
  haEntityID: document.getElementById("ha-entity-id"),
  haEntityButton: document.getElementById("ha-entity-button"),
  haServiceDomain: document.getElementById("ha-service-domain"),
  haServiceName: document.getElementById("ha-service-name"),
  haServiceBody: document.getElementById("ha-service-body"),
  haServiceButton: document.getElementById("ha-service-button"),
  haOutput: document.getElementById("ha-output"),
  notesSearchQuery: document.getElementById("notes-search-query"),
  notesTagQuery: document.getElementById("notes-tag-query"),
  notesPageType: document.getElementById("notes-page-type"),
  notesListButton: document.getElementById("notes-list-button"),
  notesSearchButton: document.getElementById("notes-search-button"),
  notesTagsButton: document.getElementById("notes-tags-button"),
  notesTagRollupButton: document.getElementById("notes-tag-rollup-button"),
  notesNewButton: document.getElementById("notes-new-button"),
  notesResults: document.getElementById("notes-results"),
  notesTitle: document.getElementById("notes-title"),
  notesID: document.getElementById("notes-id"),
  notesContent: document.getElementById("notes-content"),
  notesBoardJSON: document.getElementById("notes-board-json"),
  notesFetchButton: document.getElementById("notes-fetch-button"),
  notesSaveButton: document.getElementById("notes-save-button"),
  notesDeleteButton: document.getElementById("notes-delete-button"),
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

function safeJSONString(value) {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function dotenvValue(value) {
  return `"${String(value || "").replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

function agentEnvFile(payload, home) {
  return [
    "HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent",
    `HANK_REMOTE_AGENT_ID=${dotenvValue(payload.agent_id || agentIDFromHomeName(home?.name))}`,
    `HANK_REMOTE_AGENT_TOKEN=${dotenvValue(payload.token)}`,
    `HANK_REMOTE_AGENT_HOME_NAME=${dotenvValue(home?.name || "Home")}`,
    "HANK_REMOTE_AGENT_CONFIG_PATH=/app/.env.agent",
    "",
    "# Optional Home Assistant connection.",
    "HANK_REMOTE_HA_BASE_URL=http://homeassistant:8123",
    "HANK_REMOTE_HA_TOKEN=replace-with-home-assistant-token",
    "HANK_REMOTE_HA_TIMEOUT_SECONDS=10",
    "",
    "# Optional SMB connection. Leave blank to use the Docker files volume.",
    "HANK_REMOTE_SMB_HOST=",
    "HANK_REMOTE_SMB_SHARE=",
    "HANK_REMOTE_SMB_USERNAME=",
    "HANK_REMOTE_SMB_PASSWORD=",
    "HANK_REMOTE_SMB_DOMAIN=",
    "",
    "HANK_REMOTE_AGENT_FILES_ROOT=/srv/hank/files",
    "HANK_REMOTE_AGENT_NOTES_ROOT=/srv/hank/notes",
    "",
    "# Optional media download workflow. Credentials can be edited later from AI Settings.",
    "HANK_REMOTE_MEDIA_GRAMATON_ENABLED=false",
    "HANK_REMOTE_MEDIA_GRAMATON_BASE_URL=https://gramaton.io",
    "HANK_REMOTE_MEDIA_GRAMATON_USERNAME=",
    "HANK_REMOTE_MEDIA_GRAMATON_PASSWORD=",
    "HANK_REMOTE_MEDIA_DESTINATION_PATH=",
  ].join("\n");
}

function selectedHome() {
  return state.homes.find((home) => home.id === els.tokenHome.value) || state.homes[0] || null;
}

function selectedHomeOrThrow() {
  const home = selectedHome();
  if (!home) throw new Error("Create or select a home first.");
  return home;
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

function preferredAppSocketURL() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws/app`;
}

function nextRequestID() {
  state.requestCounter += 1;
  if (window.crypto?.randomUUID) return `dash-${window.crypto.randomUUID()}`;
  return `dash-${Date.now()}-${state.requestCounter}`;
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
    reject(new Error("Dashboard websocket closed."));
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
      if (state.appSocket === socket) {
        closeAppSocket();
      }
    });
    socket.addEventListener("error", () => {
      if (state.appSocket === socket) {
        closeAppSocket();
      }
      reject(new Error("Failed to connect dashboard websocket."));
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
        <span class="pill">Created ${formatDate(home.created_at)}</span>
      </div>
      <div class="item-actions">
        <a class="ops-card manage-link" href="/dashboard/home-users">Manage People</a>
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
    if (!revoked) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "danger-link";
      button.textContent = "Disable setup file";
      button.addEventListener("click", () => revokeToken(homeID, token.id));
      wrapper.appendChild(button);
    }
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

function renderHAOutput(html, empty = false) {
  els.haOutput.className = empty ? "card-list empty-state" : "card-list";
  els.haOutput.innerHTML = html;
}

async function runHAHealth() {
  try {
    const payload = await sendCommand("homeassistant.health");
    renderHAOutput(`<article class="tool-result-card"><div class="card-head"><div><div class="card-title">Home Assistant Check</div></div><span class="status-chip">${payload.ok ? "Working" : "Unknown"}</span></div><div class="meta">Checked at ${formatDate(payload.checked_at)}</div></article>`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function runHAFetchStates() {
  try {
    const payload = await sendCommand("homeassistant.fetch_states");
    const states = payload.states || [];
    if (!states.length) {
      renderHAOutput("No Home Assistant states returned.", true);
      return;
    }
    renderHAOutput(states.slice(0, 100).map((item) => `<article class="tool-result-card"><div class="card-head"><div><div class="card-title">${escapeHTML(item.entity_id)}</div><div class="meta">Updated ${formatDate(item.last_updated)}</div></div><span class="pill">${escapeHTML(item.state)}</span></div><pre>${escapeHTML(safeJSONString(item.attributes || {}))}</pre></article>`).join(""));
    if (states.length > 100) showToast(`Showing first 100 of ${states.length} states.`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function runHAFetchEntity() {
  const entityID = els.haEntityID.value.trim();
  if (!entityID) {
    showToast("Enter a device ID first.", true);
    return;
  }
  try {
    const payload = await sendCommand("homeassistant.fetch_state", { entity_id: entityID });
    renderHAOutput(`<article class="tool-result-card"><div class="card-head"><div><div class="card-title">${escapeHTML(payload.state?.entity_id || entityID)}</div><div class="meta">Last changed ${formatDate(payload.state?.last_changed)}</div></div><span class="pill">${escapeHTML(payload.state?.state || "")}</span></div><pre>${escapeHTML(safeJSONString(payload.state || {}))}</pre></article>`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function runHAServiceCall() {
  const domain = els.haServiceDomain.value.trim();
  const service = els.haServiceName.value.trim();
  if (!domain || !service) {
    showToast("Enter both domain and service.", true);
    return;
  }
  let body = null;
  const raw = els.haServiceBody.value.trim();
  if (raw) {
    try {
      body = JSON.parse(raw);
    } catch (error) {
      showToast(`Command details are not valid JSON: ${error.message}`, true);
      return;
    }
  }
  try {
    const payload = await sendCommand("homeassistant.call_service", { domain, service, body });
    renderHAOutput(`<article class="tool-result-card"><div class="card-head"><div><div class="card-title">${escapeHTML(`${domain}.${service}`)}</div><div class="meta">Command completed</div></div><span class="status-chip">OK</span></div><pre>${escapeHTML(safeJSONString(payload.result || {}))}</pre></article>`);
  } catch (error) {
    showToast(error.message, true);
  }
}

function clearNoteEditor() {
  els.notesTitle.value = "";
  els.notesID.value = "";
  els.notesContent.value = "";
  els.notesBoardJSON.value = "";
  els.notesPageType.value = "text";
  state.currentNoteRevision = "";
}

function renderNotesResults(html, empty = false) {
  els.notesResults.className = empty ? "card-list empty-state" : "card-list";
  els.notesResults.innerHTML = html;
}

function setNoteEditor(note) {
  els.notesTitle.value = note.title || "";
  els.notesID.value = note.note_id || note.id || "";
  els.notesContent.value = note.content || "";
  els.notesPageType.value = note.page_type || "text";
  els.notesBoardJSON.value = note.board ? safeJSONString(note.board) : "";
  state.currentNoteRevision = note.revision || "";
}

function noteCardHTML(note, options = {}) {
  const tags = Array.isArray(note.tags) && note.tags.length ? `<div class="meta">Tags: ${note.tags.map((tag) => `#${escapeHTML(tag)}`).join(", ")}</div>` : "";
  const preview = note.preview ? `<div class="meta">${escapeHTML(note.preview)}</div>` : "";
  return `<article class="tool-result-card"><div class="card-head"><div><div class="card-title">${escapeHTML(note.title || note.note_title || note.note_id || note.id || "Untitled")}</div><div class="meta">${escapeHTML(note.note_id || note.id || "")}</div></div><span class="pill">${escapeHTML(note.page_type || "text")}</span></div>${preview}${tags}${options.extra || ""}<div class="item-actions"><button type="button" data-note-id="${escapeHTML(note.note_id || note.id || "")}" class="note-open-button">Open</button></div></article>`;
}

function bindNoteOpenButtons() {
  document.querySelectorAll(".note-open-button").forEach((button) => {
    button.addEventListener("click", () => fetchNote(button.getAttribute("data-note-id")));
  });
}

async function listNotes() {
  try {
    const payload = await sendCommand("notes.list");
    const notes = payload.notes || [];
    if (!notes.length) {
      renderNotesResults("No notes found.", true);
      return;
    }
    renderNotesResults(notes.map((note) => noteCardHTML(note)).join(""));
    bindNoteOpenButtons();
  } catch (error) {
    showToast(error.message, true);
  }
}

async function searchNotes() {
  const query = els.notesSearchQuery.value.trim();
  if (!query) {
    showToast("Enter a search query first.", true);
    return;
  }
  try {
    const payload = await sendCommand("notes.search", { query, limit: 50 });
    const results = payload.results || [];
    if (!results.length) {
      renderNotesResults(`No notes matched "${escapeHTML(query)}".`, true);
      return;
    }
    renderNotesResults(results.map((item) => noteCardHTML(item, { extra: `<div class="meta">Match at character ${item.match_location}, line ${item.line_index + 1}</div>` })).join(""));
    bindNoteOpenButtons();
  } catch (error) {
    showToast(error.message, true);
  }
}

async function listTags() {
  try {
    const payload = await sendCommand("notes.tags");
    const tags = payload.tags || [];
    if (!tags.length) {
      renderNotesResults("No tags found.", true);
      return;
    }
    renderNotesResults(tags.map((tag) => `<article class="tool-result-card"><div class="card-head"><div><div class="card-title">#${escapeHTML(tag.tag)}</div><div class="meta">${tag.count} note${tag.count === 1 ? "" : "s"}</div></div><button type="button" class="tag-rollup-button" data-tag="${escapeHTML(tag.tag)}">Open Rollup</button></div></article>`).join(""));
    document.querySelectorAll(".tag-rollup-button").forEach((button) => {
      button.addEventListener("click", () => {
        els.notesTagQuery.value = button.getAttribute("data-tag") || "";
        tagRollup();
      });
    });
  } catch (error) {
    showToast(error.message, true);
  }
}

async function tagRollup() {
  const tag = els.notesTagQuery.value.trim();
  if (!tag) {
    showToast("Enter a tag first.", true);
    return;
  }
  try {
    const payload = await sendCommand("notes.tag_rollup", { tag });
    const items = payload.items || [];
    if (!items.length) {
      renderNotesResults(`No rollup items found for #${escapeHTML(tag)}.`, true);
      return;
    }
    renderNotesResults(items.map((item) => noteCardHTML(item, { extra: `<div class="meta">Line ${item.line_index + 1}: ${escapeHTML(item.line_text || "")}</div>` })).join(""));
    bindNoteOpenButtons();
  } catch (error) {
    showToast(error.message, true);
  }
}

async function fetchNote(noteID = els.notesID.value.trim()) {
  if (!noteID) {
    showToast("Enter or choose a note ID first.", true);
    return;
  }
  try {
    const payload = await sendCommand("notes.fetch", { note_id: noteID });
    setNoteEditor(payload);
    showToast("Note loaded.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function saveNote() {
  const pageType = els.notesPageType.value;
  let board = null;
  const rawBoard = els.notesBoardJSON.value.trim();
  if (pageType === "kanban" && rawBoard) {
    try {
      board = JSON.parse(rawBoard);
    } catch (error) {
      showToast(`Board details are not valid JSON: ${error.message}`, true);
      return;
    }
  }
  try {
    const payload = await sendCommand("notes.save", {
      note_id: els.notesID.value.trim(),
      title: els.notesTitle.value.trim(),
      content: els.notesContent.value,
      expected_revision: state.currentNoteRevision,
      page_type: pageType,
      board,
    });
    els.notesID.value = payload.note_id || els.notesID.value.trim();
    els.notesPageType.value = payload.page_type || pageType;
    state.currentNoteRevision = payload.revision || "";
    await listNotes();
    showToast("Note saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function deleteNote() {
  const noteID = els.notesID.value.trim();
  if (!noteID) {
    showToast("Load or enter a note ID first.", true);
    return;
  }
  if (!window.confirm(`Delete note ${noteID}?`)) return;
  try {
    await sendCommand("notes.delete", { note_id: noteID });
    clearNoteEditor();
    await listNotes();
    showToast("Note deleted.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function loadHomes() {
  try {
    const home = await api("/v1/home");
    state.homes = home ? [home] : [];
  } catch (error) {
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
    <strong>Issued token for ${escapeHTML(payload.agent_name)}</strong>
    <div class="token-meta">Copy this setup file into <code>.env.agent</code>. It is only shown once.</div>
    <button type="button" class="secondary" data-copy-agent-env>Copy .env.agent</button>
    <pre>${escapeHTML(envFile)}</pre>
    <div class="token-meta">Then start the home connector:</div>
    <code>docker compose --env-file .env.cloud --profile agent up -d agent</code>`;
  await Promise.all([loadAgents(), loadTokens(homeID)]);
  showToast("Setup file created. Copy it into .env.agent.");
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

async function logout() {
  try {
    await api("/v1/auth/logout", { method: "POST" });
  } catch (_) {
  }
  closeAppSocket();
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
els.haHealthButton.addEventListener("click", runHAHealth);
els.haStatesButton.addEventListener("click", runHAFetchStates);
els.haEntityButton.addEventListener("click", runHAFetchEntity);
els.haServiceButton.addEventListener("click", runHAServiceCall);
els.notesListButton?.addEventListener("click", listNotes);
els.notesSearchButton?.addEventListener("click", searchNotes);
els.notesTagsButton?.addEventListener("click", listTags);
els.notesTagRollupButton?.addEventListener("click", tagRollup);
els.notesNewButton?.addEventListener("click", clearNoteEditor);
els.notesFetchButton?.addEventListener("click", () => fetchNote());
els.notesSaveButton?.addEventListener("click", saveNote);
els.notesDeleteButton?.addEventListener("click", deleteNote);

hydrate();
