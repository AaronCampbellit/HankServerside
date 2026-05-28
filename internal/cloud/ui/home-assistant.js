const state = {
  user: null,
  agents: [],
  appSocket: null,
  appSocketPromise: null,
  pendingRequests: new Map(),
  requestCounter: 0,
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
    renderSession();
    await loadAgents();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.haHealthButton.addEventListener("click", runHAHealth);
els.haStatesButton.addEventListener("click", runHAFetchStates);
els.haEntityButton.addEventListener("click", runHAFetchEntity);
els.haServiceButton.addEventListener("click", runHAServiceCall);

hydrate();
