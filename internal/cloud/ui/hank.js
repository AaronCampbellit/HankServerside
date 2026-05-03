const state = {
  user: null,
  sessions: [],
  selectedSessionID: "",
  pendingRun: null,
  isSending: false,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  assistantStatus: document.getElementById("assistant-status"),
  sessionList: document.getElementById("session-list"),
  newSessionButton: document.getElementById("new-session-button"),
  conversationTitle: document.getElementById("conversation-title"),
  runState: document.getElementById("run-state"),
  messageList: document.getElementById("message-list"),
  confirmationCard: document.getElementById("confirmation-card"),
  confirmationMessage: document.getElementById("confirmation-message"),
  confirmButton: document.getElementById("confirm-button"),
  cancelButton: document.getElementById("cancel-button"),
  messageForm: document.getElementById("message-form"),
  messageInput: document.getElementById("message-input"),
  sendButton: document.getElementById("send-button"),
  toast: document.getElementById("toast"),
};

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(path, { ...options, headers, credentials: "same-origin" });
  const contentType = response.headers.get("Content-Type") || "";
  const payload = contentType.includes("application/json") ? await response.json() : await response.text();
  if (!response.ok) {
    const message = typeof payload === "string" ? payload : payload.error || payload.message || response.statusText;
    throw new Error(message);
  }
  return payload;
}

function escapeHTML(value) {
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
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

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function renderStatus(status) {
  els.assistantStatus.innerHTML = [
    ["Provider", status.provider || "local"],
    ["Chat", status.chat_configured ? "Configured" : "Local fallback"],
    ["Embeddings", status.embedding_configured ? "Configured" : "Local fallback"],
    ["Vector Store", status.vector_store || "postgres"],
  ].map(([label, value]) => `
    <div class="kv-row">
      <div class="kv-label">${escapeHTML(label)}</div>
      <div>${escapeHTML(value)}</div>
    </div>
  `).join("");
}

function renderSessions() {
  if (!state.sessions.length) {
    els.sessionList.className = "card-list empty-state";
    els.sessionList.textContent = "No conversations yet.";
    return;
  }
  els.sessionList.className = "card-list hank-session-list";
  els.sessionList.innerHTML = state.sessions.map((session) => `
    <button class="card hank-session-card${session.id === state.selectedSessionID ? " active" : ""}" type="button" data-session-id="${escapeHTML(session.id)}">
      <span class="card-title">${escapeHTML(session.title || "New Conversation")}</span>
      <span class="meta">${new Date(session.updated_at || session.last_message_at).toLocaleString()}</span>
    </button>
  `).join("");
}

function renderMessages(messages = []) {
  const session = state.sessions.find((item) => item.id === state.selectedSessionID);
  els.conversationTitle.textContent = session?.title || "HankAI";
  if (!messages.length) {
    els.messageList.className = "hank-message-list empty-state";
    els.messageList.textContent = "Ask Hank about your synced Hank data.";
    return;
  }
  els.messageList.className = "hank-message-list";
  els.messageList.innerHTML = messages.map((message) => `
    <article class="hank-message ${escapeHTML(message.role)}">
      <div class="meta">${escapeHTML(message.role)} · ${new Date(message.created_at).toLocaleString()}</div>
      <p>${escapeHTML(message.text).replaceAll("\n", "<br>")}</p>
      ${renderCards(message.cards || [])}
    </article>
  `).join("");
  els.messageList.scrollTop = els.messageList.scrollHeight;
}

function renderCards(cards) {
  if (!cards.length) return "";
  return `<div class="card-list hank-result-cards">${cards.map((card) => `
    <article class="card">
      <div class="card-title">${escapeHTML(card.title)}</div>
      <div class="meta">${escapeHTML(card.summary || "")}</div>
      <div class="pill">${escapeHTML(card.kind || "result")}</div>
    </article>
  `).join("")}</div>`;
}

async function loadSessions() {
  const payload = await api("/v1/home/assistant/sessions");
  state.sessions = payload.sessions || [];
  if (!state.selectedSessionID && state.sessions[0]) {
    state.selectedSessionID = state.sessions[0].id;
  }
  renderSessions();
  if (state.selectedSessionID) {
    await loadMessages(state.selectedSessionID);
  }
}

async function loadMessages(sessionID) {
  state.selectedSessionID = sessionID;
  renderSessions();
  const payload = await api(`/v1/home/assistant/sessions/${encodeURIComponent(sessionID)}/messages`);
  renderMessages(payload.messages || []);
}

async function createSession() {
  const session = await api("/v1/home/assistant/sessions", { method: "POST" });
  state.sessions.unshift(session);
  state.selectedSessionID = session.id;
  renderSessions();
  renderMessages([]);
}

async function sendMessage(event) {
  event.preventDefault();
  if (state.isSending) return;
  const content = els.messageInput.value.trim();
  if (!content) return;
  if (!state.selectedSessionID) {
    await createSession();
  }
  state.isSending = true;
  els.sendButton.disabled = true;
  els.runState.textContent = "Working";
  try {
    const run = await api(`/v1/home/assistant/sessions/${encodeURIComponent(state.selectedSessionID)}/messages`, {
      method: "POST",
      body: JSON.stringify({
        content,
        device_context: {
          device_id: "hankserverside-dashboard",
          timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
        },
      }),
    });
    els.messageInput.value = "";
    await continueRun(run);
    await loadSessions();
  } catch (error) {
    showToast(error.message, true);
  } finally {
    state.isSending = false;
    els.sendButton.disabled = false;
  }
}

async function continueRun(initialRun) {
  let run = initialRun.run || initialRun;
  for (let attempt = 0; attempt < 20; attempt += 1) {
    if (run.requires_confirmation) {
      state.pendingRun = run;
      els.confirmationMessage.textContent = run.assistant_message?.text || "Hank needs confirmation before continuing.";
      els.confirmationCard.hidden = false;
      els.runState.textContent = "Confirm";
      return;
    }
    if (run.requires_client_tools) {
      els.runState.textContent = "Use iPhone";
      showToast("This action needs the Hank app to finish the device-only calendar step.", true);
      return;
    }
    if (["completed", "failed", "cancelled", "canceled"].includes(String(run.state || "").toLowerCase())) {
      els.runState.textContent = run.state || "Ready";
      return;
    }
    await new Promise((resolve) => window.setTimeout(resolve, 500));
    run = await api(`/v1/home/assistant/runs/${encodeURIComponent(run.id)}`);
  }
  els.runState.textContent = "Still Working";
}

async function confirmPending(approved) {
  if (!state.pendingRun) return;
  try {
    const run = await api(`/v1/home/assistant/runs/${encodeURIComponent(state.pendingRun.id)}/confirm`, {
      method: "POST",
      body: JSON.stringify({ approved }),
    });
    state.pendingRun = null;
    els.confirmationCard.hidden = true;
    await continueRun(run);
    await loadSessions();
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
    renderStatus(await api("/v1/home/assistant/status"));
    await loadSessions();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.newSessionButton.addEventListener("click", () => createSession().catch((error) => showToast(error.message, true)));
els.sessionList.addEventListener("click", (event) => {
  const button = event.target.closest("[data-session-id]");
  if (!button) return;
  loadMessages(button.dataset.sessionId).catch((error) => showToast(error.message, true));
});
els.messageForm.addEventListener("submit", sendMessage);
els.confirmButton.addEventListener("click", () => confirmPending(true));
els.cancelButton.addEventListener("click", () => confirmPending(false));

hydrate();
