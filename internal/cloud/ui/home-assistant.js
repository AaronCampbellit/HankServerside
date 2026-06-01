const state = {
  user: null,
  agents: [],
  appSocket: null,
  appSocketPromise: null,
  pendingRequests: new Map(),
  requestCounter: 0,
  states: [],
  dashboardEntityIDs: [],
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  agentPill: document.getElementById("homeassistant-agent-pill"),
  agentOutput: document.getElementById("homeassistant-agent-output"),
  haHealthButton: document.getElementById("ha-health-button"),
  haStatesButton: document.getElementById("ha-states-button"),
  haEntityID: document.getElementById("ha-entity-id"),
  haEntitySearch: document.getElementById("ha-entity-search"),
  haEntityResults: document.getElementById("ha-entity-results"),
  haEntityCount: document.getElementById("ha-entity-count"),
  haDashboard: document.getElementById("ha-dashboard"),
  haDashboardCount: document.getElementById("ha-dashboard-count"),
  haEntityButton: document.getElementById("ha-entity-button"),
  haServiceDomain: document.getElementById("ha-service-domain"),
  haServiceName: document.getElementById("ha-service-name"),
  haServiceBody: document.getElementById("ha-service-body"),
  haServiceButton: document.getElementById("ha-service-button"),
  haOutput: document.getElementById("ha-output"),
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

function safeJSONString(value) {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function entityName(item) {
  return item?.attributes?.friendly_name || item?.entity_id || "";
}

function entityDomain(entityID) {
  return String(entityID || "").split(".")[0] || "entity";
}

function entitySearchText(item) {
  const attrs = item.attributes || {};
  return [
    item.entity_id,
    attrs.friendly_name,
    attrs.device_class,
    attrs.area_id,
    attrs.unit_of_measurement,
    item.state,
  ].filter(Boolean).join(" ").toLowerCase();
}

function fuzzyScore(query, text) {
  const normalizedQuery = query.trim().toLowerCase();
  if (!normalizedQuery) return 1;
  const tokens = normalizedQuery.split(/\s+/).filter(Boolean);
  let score = 0;
  for (const token of tokens) {
    if (text.includes(token)) {
      score += 20 + token.length;
      continue;
    }
    let cursor = 0;
    let matched = 0;
    for (const char of token) {
      const index = text.indexOf(char, cursor);
      if (index === -1) break;
      matched += 1;
      cursor = index + 1;
    }
    if (matched < Math.ceil(token.length * 0.65)) return 0;
    score += matched;
  }
  return score;
}

function dashboardStorageKey() {
  return `hank-homeassistant-dashboard:${state.user?.id || "anonymous"}`;
}

function loadDashboard() {
  try {
    const stored = JSON.parse(window.localStorage.getItem(dashboardStorageKey()) || "[]");
    state.dashboardEntityIDs = Array.isArray(stored) ? stored.filter(Boolean) : [];
  } catch {
    state.dashboardEntityIDs = [];
  }
}

function saveDashboard() {
  window.localStorage.setItem(dashboardStorageKey(), JSON.stringify(state.dashboardEntityIDs));
}

function stateByEntityID(entityID) {
  return state.states.find((item) => item.entity_id === entityID) || null;
}

function renderEntityCard(item, options = {}) {
  const name = entityName(item);
  const domain = entityDomain(item.entity_id);
  const canToggle = ["light", "switch", "fan", "input_boolean"].includes(domain);
  const isDashboard = state.dashboardEntityIDs.includes(item.entity_id);
  return `
    <article class="tool-result-card ha-entity-card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(name)}</div>
          <div class="meta">${escapeHTML(item.entity_id)}</div>
        </div>
        <span class="pill">${escapeHTML(item.state)}</span>
      </div>
      <div class="meta">${escapeHTML(domain)}${item.attributes?.unit_of_measurement ? ` / ${escapeHTML(item.attributes.unit_of_measurement)}` : ""}</div>
      <div class="item-actions">
        <button type="button" class="secondary" data-ha-fill="${escapeHTML(item.entity_id)}">Use</button>
        ${canToggle ? `<button type="button" data-ha-toggle="${escapeHTML(item.entity_id)}">${item.state === "on" ? "Turn Off" : "Turn On"}</button>` : ""}
        <button type="button" class="${isDashboard ? "ghost" : "secondary"}" data-ha-dashboard="${escapeHTML(item.entity_id)}">${options.dashboardTile ? "Remove" : isDashboard ? "Saved" : "Add Tile"}</button>
      </div>
    </article>
  `;
}

function wireEntityActions(root) {
  root.querySelectorAll("[data-ha-fill]").forEach((button) => {
    button.addEventListener("click", () => {
      els.haEntityID.value = button.dataset.haFill;
      runHAFetchEntity();
    });
  });
  root.querySelectorAll("[data-ha-dashboard]").forEach((button) => {
    button.addEventListener("click", () => toggleDashboardEntity(button.dataset.haDashboard));
  });
  root.querySelectorAll("[data-ha-toggle]").forEach((button) => {
    button.addEventListener("click", () => toggleEntity(button.dataset.haToggle));
  });
}

function preferredAppSocketURL() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws/app`;
}

function nextRequestID() {
  state.requestCounter += 1;
  if (window.crypto?.randomUUID) return `ha-${window.crypto.randomUUID()}`;
  return `ha-${Date.now()}-${state.requestCounter}`;
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
    reject(new Error("Home Assistant websocket closed."));
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
      reject(new Error("Failed to connect Home Assistant websocket."));
    }, { once: true });
  });
  return state.appSocketPromise;
}

async function sendCommand(command, body = {}) {
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

function renderAgents() {
  const agent = state.agents[0] || null;
  const isOnline = String(agent?.status || "").toLowerCase() === "online";
  els.agentPill.textContent = agent ? (isOnline ? "Online" : "Offline") : "Not Set Up";
  els.agentPill.className = isOnline ? "status-chip" : "status-chip offline";
  if (!agent) {
    els.agentOutput.className = "card-list empty-state";
    els.agentOutput.textContent = "";
    els.agentOutput.hidden = true;
    return;
  }
  els.agentOutput.className = "card-list";
  els.agentOutput.hidden = false;
  els.agentOutput.innerHTML = `
    <article class="card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(agent.name || agent.agent_id || agent.id)}</div>
          <div class="meta">${escapeHTML(agent.home_name || "Home connector")}</div>
        </div>
        <span class="status-chip ${isOnline ? "" : "offline"}">${isOnline ? "Online" : "Offline"}</span>
      </div>
      <div class="meta">Connector ID: ${escapeHTML(agent.agent_id || agent.id || "unknown")}</div>
      <div class="meta">Last online: ${formatDate(agent.last_seen_at)}</div>
    </article>
  `;
}

function renderHAOutput(html, empty = false) {
  els.haOutput.className = empty ? "card-list empty-state" : "card-list";
  els.haOutput.hidden = false;
  els.haOutput.innerHTML = html;
}

function renderEntityResults() {
  const query = els.haEntitySearch.value.trim();
  if (!state.states.length) {
    els.haEntityResults.className = "card-list empty-state";
    els.haEntityResults.textContent = "Refresh entities to search Home Assistant.";
    els.haEntityCount.textContent = "Not Loaded";
    return;
  }
  const matches = state.states
    .map((item) => ({ item, score: fuzzyScore(query, entitySearchText(item)) }))
    .filter((entry) => entry.score > 0)
    .sort((left, right) => right.score - left.score || entityName(left.item).localeCompare(entityName(right.item)))
    .slice(0, 80)
    .map((entry) => entry.item);
  els.haEntityCount.textContent = `${matches.length} / ${state.states.length}`;
  if (!matches.length) {
    els.haEntityResults.className = "card-list empty-state";
    els.haEntityResults.textContent = "No matching entities.";
    return;
  }
  els.haEntityResults.className = "card-list";
  els.haEntityResults.innerHTML = matches.map((item) => renderEntityCard(item)).join("");
  wireEntityActions(els.haEntityResults);
}

function renderDashboard() {
  const tiles = state.dashboardEntityIDs.map(stateByEntityID).filter(Boolean);
  els.haDashboardCount.textContent = `${tiles.length} ${tiles.length === 1 ? "Tile" : "Tiles"}`;
  if (!tiles.length) {
    els.haDashboard.className = "ha-dashboard-grid empty-state";
    els.haDashboard.textContent = state.states.length ? "Add entities from the list below." : "Refresh entities to build a dashboard.";
    return;
  }
  els.haDashboard.className = "ha-dashboard-grid";
  els.haDashboard.innerHTML = tiles.map((item) => renderEntityCard(item, { dashboardTile: true })).join("");
  wireEntityActions(els.haDashboard);
}

function toggleDashboardEntity(entityID) {
  if (!entityID) return;
  if (state.dashboardEntityIDs.includes(entityID)) {
    state.dashboardEntityIDs = state.dashboardEntityIDs.filter((id) => id !== entityID);
  } else {
    state.dashboardEntityIDs = [...state.dashboardEntityIDs, entityID];
  }
  saveDashboard();
  renderDashboard();
  renderEntityResults();
}

async function toggleEntity(entityID) {
  const item = stateByEntityID(entityID);
  if (!item) return;
  const domain = entityDomain(entityID);
  const service = item.state === "on" ? "turn_off" : "turn_on";
  try {
    await sendCommand("homeassistant.call_service", { domain, service, body: { entity_id: entityID } });
    showToast(`${entityName(item)} ${service.replace("_", " ")} sent.`);
    await runHAFetchStates(false);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function runHAHealth() {
  try {
    const payload = await sendCommand("homeassistant.health");
    renderHAOutput(`<article class="tool-result-card"><div class="card-head"><div><div class="card-title">Home Assistant Check</div></div><span class="status-chip">${payload.ok ? "Working" : "Unknown"}</span></div><div class="meta">Checked at ${formatDate(payload.checked_at)}</div></article>`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function runHAFetchStates(showResults = true) {
  try {
    const payload = await sendCommand("homeassistant.fetch_states");
    const states = payload.states || [];
    state.states = states.slice().sort((left, right) => entityName(left).localeCompare(entityName(right), undefined, { sensitivity: "base" }));
    renderEntityResults();
    renderDashboard();
    if (!states.length) {
      if (showResults) renderHAOutput("No Home Assistant states returned.", true);
      return;
    }
    if (showResults) {
      renderHAOutput(`<article class="tool-result-card"><div class="card-head"><div><div class="card-title">Entities Refreshed</div><div class="meta">Use search to filter the full list.</div></div><span class="status-chip">${states.length}</span></div></article>`);
    }
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
    showToast("Enter both type and service.", true);
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

async function loadAgents() {
  const payload = await api("/v1/home/agent");
  state.agents = payload.agent ? [payload.agent] : [];
  renderAgents();
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
    loadDashboard();
    renderSession();
    await loadAgents();
    renderDashboard();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.haHealthButton.addEventListener("click", runHAHealth);
els.haStatesButton.addEventListener("click", runHAFetchStates);
els.haEntityButton.addEventListener("click", runHAFetchEntity);
els.haServiceButton.addEventListener("click", runHAServiceCall);
els.haEntitySearch.addEventListener("input", renderEntityResults);

hydrate();
