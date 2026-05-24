const state = {
  user: null,
  sessions: [],
  selectedSessionID: "",
  pendingRun: null,
  isSending: false,
  draftAttachments: [],
  submittedAttachments: new Map(),
  appSocket: null,
  appSocketPromise: null,
  pendingRequests: new Map(),
  requestCounter: 0,
  logsVisible: false,
  traceEvents: [],
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  assistantStatus: document.getElementById("assistant-status"),
  sessionList: document.getElementById("session-list"),
  newSessionButton: document.getElementById("new-session-button"),
  deleteSessionButton: document.getElementById("delete-session-button"),
  conversationTitle: document.getElementById("conversation-title"),
  runState: document.getElementById("run-state"),
  logButton: document.getElementById("log-button"),
  logPanel: document.getElementById("log-panel"),
  logMeta: document.getElementById("log-meta"),
  logList: document.getElementById("log-list"),
  refreshLogsButton: document.getElementById("refresh-logs-button"),
  copyLogsButton: document.getElementById("copy-logs-button"),
  clearLogsButton: document.getElementById("clear-logs-button"),
  messageList: document.getElementById("message-list"),
  confirmationCard: document.getElementById("confirmation-card"),
  confirmationMessage: document.getElementById("confirmation-message"),
  confirmButton: document.getElementById("confirm-button"),
  cancelButton: document.getElementById("cancel-button"),
  attachmentTray: document.getElementById("attachment-tray"),
  attachmentInput: document.getElementById("attachment-input"),
  attachButton: document.getElementById("attach-button"),
  messageForm: document.getElementById("message-form"),
  messageInput: document.getElementById("message-input"),
  sendButton: document.getElementById("send-button"),
  toast: document.getElementById("toast"),
};

const maxChatAttachmentBytes = 100 * 1024 * 1024;

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

function formatBytes(bytes) {
  const value = Number(bytes) || 0;
  if (value < 1024) return `${value} B`;
  const units = ["KB", "MB", "GB"];
  let size = value / 1024;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(size >= 10 ? 0 : 1)} ${units[unitIndex]}`;
}

function makeID(prefix) {
  if (window.crypto?.randomUUID) {
    return `${prefix}-${window.crypto.randomUUID()}`;
  }
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function attachmentKind(contentType) {
  const type = String(contentType || "").toLowerCase();
  if (type.startsWith("image/")) return "image";
  if (type.includes("pdf")) return "pdf";
  return "document";
}

async function sha256Hex(file) {
  if (!window.crypto?.subtle) {
    return "";
  }
  const buffer = await file.arrayBuffer();
  const digest = await window.crypto.subtle.digest("SHA-256", buffer);
  return Array.from(new Uint8Array(digest)).map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

function renderAttachmentTray() {
  if (!state.draftAttachments.length) {
    els.attachmentTray.hidden = true;
    els.attachmentTray.innerHTML = "";
    return;
  }
  els.attachmentTray.hidden = false;
  els.attachmentTray.innerHTML = state.draftAttachments.map((attachment) => `
    <div class="hank-attachment-chip">
      <span class="file-icon" aria-hidden="true">${attachment.kind === "image" ? "IMG" : "DOC"}</span>
      <span>
        <strong>${escapeHTML(attachment.filename)}</strong>
        <span>${escapeHTML(formatBytes(attachment.sizeBytes))}</span>
      </span>
      <button class="hank-attachment-remove ghost" type="button" data-attachment-id="${escapeHTML(attachment.id)}" aria-label="Remove ${escapeHTML(attachment.filename)}">Remove</button>
    </div>
  `).join("");
}

async function addAttachmentsFromFiles(files) {
  const selectedFiles = Array.from(files || []).filter((file) => file && file.name);
  if (!selectedFiles.length) {
    return;
  }
  for (const file of selectedFiles) {
    if (file.size > maxChatAttachmentBytes) {
      showToast(`${file.name} is larger than the 100 MB chat upload limit.`, true);
      continue;
    }
    const contentType = file.type || "application/octet-stream";
    state.draftAttachments.push({
      id: makeID("hank-upload"),
      clientAttachmentID: makeID("client-attachment"),
      file,
      filename: file.name || "Attachment",
      contentType,
      kind: attachmentKind(contentType),
      sizeBytes: file.size,
      checksumSHA256: await sha256Hex(file),
    });
  }
  els.attachmentInput.value = "";
  renderAttachmentTray();
}

function removeDraftAttachment(attachmentID) {
  state.draftAttachments = state.draftAttachments.filter((attachment) => attachment.id !== attachmentID);
  renderAttachmentTray();
}

function attachmentUploadPayload(attachment) {
  return {
    client_attachment_id: attachment.clientAttachmentID,
    filename: attachment.filename,
    content_type: attachment.contentType,
    size_bytes: attachment.sizeBytes,
    checksum_sha256: attachment.checksumSHA256,
    kind: attachment.kind,
  };
}

function attachmentOnlyMessageText(attachments) {
  if (attachments.length === 1) {
    return `Uploaded ${attachments[0].filename}.`;
  }
  return `Uploaded ${attachments.length} attachments.`;
}

function preferredAppSocketURL() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws/app`;
}

function nextRequestID() {
  state.requestCounter += 1;
  if (window.crypto?.randomUUID) return `hank-${window.crypto.randomUUID()}`;
  return `hank-${Date.now()}-${state.requestCounter}`;
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

function normalizeFilePath(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed || trimmed === "/") return "";
  return trimmed.replaceAll("\\", "/").split("/").filter(Boolean).join("/");
}

function joinFilePath(base, child) {
  const normalizedBase = normalizeFilePath(base);
  const normalizedChild = String(child || "").trim().replace(/^\/+/, "");
  if (!normalizedChild) return normalizedBase;
  return normalizedBase ? `${normalizedBase}/${normalizedChild}` : normalizedChild;
}

function splitName(name) {
  const value = String(name || "Attachment").trim() || "Attachment";
  const dotIndex = value.lastIndexOf(".");
  if (dotIndex <= 0 || dotIndex === value.length - 1) {
    return [value, ""];
  }
  return [value.slice(0, dotIndex), value.slice(dotIndex + 1)];
}

function uniqueCopyName(originalName, existingNames) {
  if (!existingNames.has(originalName)) {
    return originalName;
  }
  const [baseName, ext] = splitName(originalName);
  let counter = 1;
  while (true) {
    const candidateBase = counter === 1 ? `${baseName} copy` : `${baseName} copy ${counter}`;
    const candidate = ext ? `${candidateBase}.${ext}` : candidateBase;
    if (!existingNames.has(candidate)) {
      return candidate;
    }
    counter += 1;
  }
}

function stringValue(value, fallback = "") {
  return typeof value === "string" ? value.trim() : fallback;
}

function stringArrayValue(value) {
  return Array.isArray(value) ? value.map((item) => String(item || "").trim()).filter(Boolean) : [];
}

async function parseFetchPayload(response) {
  const contentType = response.headers.get("Content-Type") || "";
  const payload = contentType.includes("application/json") ? await response.json() : await response.text();
  if (!response.ok) {
    const message = typeof payload === "string" ? payload.trim() : payload.error || payload.message;
    throw new Error(message || response.statusText);
  }
  return payload;
}

async function uploadNoteAttachment(scope, noteID, attachment) {
  const basePath = scope === "home"
    ? `/v1/home/notes/${encodeURIComponent(noteID)}/attachments`
    : `/v1/me/notes/${encodeURIComponent(noteID)}/attachments`;
  const response = await fetch(basePath, {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": attachment.contentType || "application/octet-stream",
      "X-Hank-Filename": attachment.filename,
    },
    body: attachment.file,
  });
  return parseFetchPayload(response);
}

async function uploadFileServerAttachment(targetPath, attachment, filename) {
  const destinationPath = joinFilePath(targetPath, filename);
  const setup = await api("/v1/home/files/uploads", {
    method: "POST",
    body: JSON.stringify({ path: destinationPath }),
  });
  const response = await fetch(setup.url, {
    method: "PUT",
    credentials: "same-origin",
    headers: { "Content-Type": "application/octet-stream" },
    body: attachment.file,
  });
  const payload = await parseFetchPayload(response);
  return { payload, path: destinationPath };
}

function removeSubmittedAttachments(attachments) {
  for (const attachment of attachments) {
    state.submittedAttachments.delete(attachment.clientAttachmentID);
  }
}

class AttachmentCommitError extends Error {
  constructor(message, result) {
    super(message);
    this.name = "AttachmentCommitError";
    this.result = result;
  }
}

function missingStagedAttachmentError(attachmentIDs, destinationKind = "") {
  return new AttachmentCommitError("The staged upload is no longer available in this browser.", {
    destination_kind: destinationKind,
    attachment_ids: attachmentIDs,
    expired_attachment_ids: attachmentIDs,
    error_code: "missing_staged_attachment",
  });
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function renderStatus(status) {
  const index = status.index || {};
  els.assistantStatus.innerHTML = [
    ["Provider", status.provider || "local"],
    ["Chat", status.chat_configured ? "Configured" : "Local fallback"],
    ["Embeddings", status.embedding_configured ? "Configured" : "Local fallback"],
    ["Vector Store", status.vector_store || "postgres"],
    ["Vector Mode", index.vector_mode || "json_fallback"],
    ["Memory", `${index.chunk_count || 0} chunks · ${index.conversation_count || 0} conversations`],
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
    <article class="card hank-session-card${session.id === state.selectedSessionID ? " active" : ""}">
      <button class="hank-session-select" type="button" data-session-id="${escapeHTML(session.id)}">
        <span class="card-title">${escapeHTML(session.title || "New Conversation")}</span>
        <span class="meta">${new Date(session.updated_at || session.last_message_at).toLocaleString()}</span>
      </button>
      <button class="hank-session-delete ghost" type="button" data-delete-session-id="${escapeHTML(session.id)}">Delete</button>
    </article>
  `).join("");
}

function renderMessages(messages = []) {
  const session = state.sessions.find((item) => item.id === state.selectedSessionID);
  els.conversationTitle.textContent = session?.title || "HankAI";
  els.deleteSessionButton.disabled = !session;
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
    <article class="card hank-result-card">
      <div class="card-title">${escapeHTML(card.title)}</div>
      <div class="meta">${escapeHTML(card.summary || "")}</div>
      <div class="hank-result-card-footer">
        <div class="pill">${escapeHTML(card.kind || "result")}</div>
        ${renderCardAction(card)}
      </div>
    </article>
  `).join("")}</div>`;
}

function renderCardAction(card) {
  const href = cardActionHref(card);
  if (!href) return "";
  return `<a class="button-link hank-result-action" href="${escapeHTML(href)}">${escapeHTML(card.action_title || "Open")}</a>`;
}

function formatTraceTime(value) {
  return value ? new Date(value).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "";
}

function renderTraceDetails(details = {}) {
  const rows = Object.entries(details || {}).filter(([_, value]) => String(value || "").trim());
  if (!rows.length) return "";
  return `<dl class="hank-log-details">${rows.map(([key, value]) => `
    <div>
      <dt>${escapeHTML(key.replaceAll("_", " "))}</dt>
      <dd>${escapeHTML(value)}</dd>
    </div>
  `).join("")}</dl>`;
}

function renderLogs(payload = {}) {
  const events = payload.events || [];
  state.traceEvents = events;
  const total = Number(payload.total || events.length);
  const sessionSuffix = state.selectedSessionID ? " for this conversation" : "";
  els.logMeta.textContent = `${events.length} of ${total} recent event(s)${sessionSuffix}.`;
  if (!events.length) {
    els.logList.className = "hank-log-list empty-state";
    els.logList.textContent = "No workflow events recorded for this view yet.";
    return;
  }
  els.logList.className = "hank-log-list";
  els.logList.innerHTML = events.slice().reverse().map((event) => {
    const level = String(event.level || "info").toLowerCase();
    return `
      <article class="hank-log-entry ${escapeHTML(level)}">
        <div class="hank-log-entry-head">
          <span>${escapeHTML(formatTraceTime(event.created_at))}</span>
          <strong>${escapeHTML(event.event || "event")}</strong>
          <span class="pill">${escapeHTML(event.scope || "assistant")}</span>
        </div>
        <p>${escapeHTML(event.summary || "")}</p>
        <div class="hank-log-ids">
          ${event.run_id ? `<span>run ${escapeHTML(event.run_id)}</span>` : ""}
          ${event.request_id ? `<span>request ${escapeHTML(event.request_id)}</span>` : ""}
          ${event.message_id ? `<span>message ${escapeHTML(event.message_id)}</span>` : ""}
        </div>
        ${renderTraceDetails(event.details || {})}
      </article>
    `;
  }).join("");
}

function traceLogText() {
  const lines = [
    "HankAI workflow logs",
    `Generated: ${new Date().toISOString()}`,
    state.selectedSessionID ? `Session: ${state.selectedSessionID}` : "Session: all recent",
    "",
  ];
  for (const event of state.traceEvents) {
    lines.push(`[${event.created_at || ""}] ${event.level || "info"} ${event.scope || "assistant"} ${event.event || "event"}`);
    if (event.summary) lines.push(`  ${event.summary}`);
    if (event.home_id) lines.push(`  home_id=${event.home_id}`);
    if (event.session_id) lines.push(`  session_id=${event.session_id}`);
    if (event.run_id) lines.push(`  run_id=${event.run_id}`);
    if (event.message_id) lines.push(`  message_id=${event.message_id}`);
    if (event.request_id) lines.push(`  request_id=${event.request_id}`);
    for (const [key, value] of Object.entries(event.details || {})) {
      lines.push(`  ${key}=${value}`);
    }
  }
  return lines.join("\n");
}

function cardActionHref(card) {
  const kind = String(card.kind || "").toLowerCase();
  if (kind === "note" && card.note_id) {
    const params = new URLSearchParams({ note_id: card.note_id });
    if (card.search_text) params.set("search", card.search_text);
    return `/dashboard/profile-notes?${params.toString()}`;
  }
  if (kind === "file" && card.path) {
    const params = new URLSearchParams({ path: card.path });
    if (card.is_directory) params.set("directory", "true");
    return `/dashboard/file-server?${params.toString()}`;
  }
  if (kind === "calendar") {
    const params = new URLSearchParams();
    if (card.event_id) params.set("event_id", card.event_id);
    if (card.target_date) params.set("date", card.target_date);
    return params.toString() ? `/dashboard?${params.toString()}` : "/dashboard";
  }
  return "";
}

async function loadSessions() {
  const payload = await api("/v1/home/assistant/sessions");
  state.sessions = payload.sessions || [];
  if (state.selectedSessionID && !state.sessions.some((session) => session.id === state.selectedSessionID)) {
    state.selectedSessionID = "";
  }
  if (!state.selectedSessionID && state.sessions[0]) {
    state.selectedSessionID = state.sessions[0].id;
  }
  renderSessions();
  if (state.selectedSessionID) {
    await loadMessages(state.selectedSessionID);
  } else {
    renderMessages([]);
  }
}

async function loadMessages(sessionID) {
  state.selectedSessionID = sessionID;
  renderSessions();
  const payload = await api(`/v1/home/assistant/sessions/${encodeURIComponent(sessionID)}/messages`);
  renderMessages(payload.messages || []);
  if (state.logsVisible) {
    await loadLogs();
  }
}

async function loadLogs() {
  if (!els.logPanel || els.logPanel.hidden) return;
  const params = new URLSearchParams({ limit: "300" });
  if (state.selectedSessionID) {
    params.set("session_id", state.selectedSessionID);
  }
  try {
    renderLogs(await api(`/v1/home/assistant/logs?${params.toString()}`));
  } catch (error) {
    els.logMeta.textContent = "Workflow logs are admin-only.";
    els.logList.className = "hank-log-list empty-state";
    els.logList.textContent = error.message;
  }
}

async function toggleLogs() {
  state.logsVisible = !state.logsVisible;
  els.logPanel.hidden = !state.logsVisible;
  els.logButton.textContent = state.logsVisible ? "Hide Logs" : "Logs";
  if (state.logsVisible) {
    await loadLogs();
  }
}

async function copyLogs() {
  await loadLogs();
  const text = traceLogText();
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      showToast("Workflow logs copied.");
      return;
    }
  } catch (_) {
  }
  if (copyTextFallback(text)) {
    showToast("Workflow logs copied.");
    return;
  }
  showToast("Clipboard is blocked. The logs are selected; press Cmd+C to copy.", true);
}

function copyTextFallback(text) {
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "readonly");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  textarea.style.top = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  let copied = false;
  try {
    copied = document.execCommand("copy");
  } catch (_) {
    copied = false;
  }
  if (copied) {
    document.body.removeChild(textarea);
  }
  return copied;
}

async function clearLogs() {
  if (!window.confirm("Clear recent HankAI workflow logs for this home?")) {
    return;
  }
  await api("/v1/home/assistant/logs", { method: "DELETE" });
  await loadLogs();
  showToast("Workflow logs cleared.");
}

async function createSession() {
  const session = await api("/v1/home/assistant/sessions", { method: "POST" });
  state.sessions.unshift(session);
  state.selectedSessionID = session.id;
  renderSessions();
  renderMessages([]);
}

async function deleteSession(sessionID = state.selectedSessionID) {
  if (!sessionID) return;
  const session = state.sessions.find((item) => item.id === sessionID);
  const title = session?.title || "this conversation";
  if (!window.confirm(`Delete ${title}?`)) {
    return;
  }
  await api(`/v1/home/assistant/sessions/${encodeURIComponent(sessionID)}`, { method: "DELETE" });
  state.sessions = state.sessions.filter((item) => item.id !== sessionID);
  if (state.selectedSessionID === sessionID) {
    state.selectedSessionID = state.sessions[0]?.id || "";
  }
  renderSessions();
  if (state.selectedSessionID) {
    await loadMessages(state.selectedSessionID);
  } else {
    renderMessages([]);
  }
  showToast("Conversation deleted.");
}

async function sendMessage(event) {
  event.preventDefault();
  if (state.isSending) return;
  const content = els.messageInput.value.trim();
  const attachmentsToSend = [...state.draftAttachments];
  if (!content && !attachmentsToSend.length) return;
  if (!state.selectedSessionID) {
    await createSession();
  }
  state.isSending = true;
  els.sendButton.disabled = true;
  els.attachButton.disabled = true;
  els.runState.textContent = "Working";
  try {
    const messageContent = content || attachmentOnlyMessageText(attachmentsToSend);
    const run = await api(`/v1/home/assistant/sessions/${encodeURIComponent(state.selectedSessionID)}/messages`, {
      method: "POST",
      body: JSON.stringify({
        content: messageContent,
        attachments: attachmentsToSend.map(attachmentUploadPayload),
        device_context: {
          device_id: "hankserverside-dashboard",
          timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
        },
      }),
    });
    els.messageInput.value = "";
    const sentIDs = new Set(attachmentsToSend.map((attachment) => attachment.id));
    state.draftAttachments = state.draftAttachments.filter((attachment) => !sentIDs.has(attachment.id));
    for (const attachment of attachmentsToSend) {
      state.submittedAttachments.set(attachment.clientAttachmentID, attachment);
    }
    renderAttachmentTray();
    await continueRun(run);
    await loadSessions();
    if (state.logsVisible) {
      await loadLogs();
    }
  } catch (error) {
    showToast(error.message, true);
  } finally {
    state.isSending = false;
    els.sendButton.disabled = false;
    els.attachButton.disabled = false;
  }
}

function handleMessageInputKeydown(event) {
  if (event.key !== "Enter" || event.shiftKey || event.isComposing) {
    return;
  }
  event.preventDefault();
  els.messageForm.requestSubmit();
}

async function executeClientToolRun(run) {
  const request = run.client_tool_request;
  if (!request || !request.tool_name) {
    throw new Error("Hank did not provide a client tool request.");
  }
  try {
    const result = await executeClientTool(request);
    return api(`/v1/home/assistant/runs/${encodeURIComponent(run.id)}/client-tool-results`, {
      method: "POST",
      body: JSON.stringify({
        tool_name: request.tool_name,
        result,
      }),
    });
  } catch (error) {
    const result = error instanceof AttachmentCommitError ? error.result : {
      destination_kind: stringValue(request.arguments?.destination_kind),
      attachment_ids: stringArrayValue(request.arguments?.attachment_ids),
    };
    return api(`/v1/home/assistant/runs/${encodeURIComponent(run.id)}/client-tool-results`, {
      method: "POST",
      body: JSON.stringify({
        tool_name: request.tool_name,
        error: error.message || "The client tool could not complete.",
        result,
      }),
    });
  }
}

async function executeClientTool(request) {
  switch (request.tool_name) {
    case "attachments.commit":
      return commitAttachments(request.arguments || {});
    default:
      throw new Error(`This action still needs the Hank iPhone app: ${request.tool_name}.`);
  }
}

async function commitAttachments(argumentsPayload) {
  const attachmentIDs = stringArrayValue(argumentsPayload.attachment_ids);
  const destinationKind = stringValue(argumentsPayload.destination_kind);
  if (!attachmentIDs.length) {
    throw new Error("Hank did not include any staged attachment IDs.");
  }
  const selectedAttachments = attachmentIDs.map((id) => state.submittedAttachments.get(id));
  if (selectedAttachments.some((attachment) => !attachment)) {
    throw missingStagedAttachmentError(attachmentIDs, destinationKind);
  }

  switch (destinationKind) {
    case "note_attachment":
      return commitNoteAttachments(argumentsPayload, selectedAttachments, attachmentIDs);
    case "smb":
      return commitFileServerAttachments(argumentsPayload, selectedAttachments, attachmentIDs);
    default:
      throw new Error("Hank did not include a valid attachment destination.");
  }
}

async function commitNoteAttachments(argumentsPayload, selectedAttachments, attachmentIDs) {
  const noteID = stringValue(argumentsPayload.note_id);
  if (!noteID) {
    throw new Error("Hank did not include the target note.");
  }
  const noteScope = stringValue(argumentsPayload.note_scope, "profile") || "profile";
  const noteTitle = stringValue(argumentsPayload.note_title, "Note") || "Note";
  const files = [];
  try {
    for (const attachment of selectedAttachments) {
      const uploaded = await uploadNoteAttachment(noteScope, noteID, attachment);
      files.push({
        client_attachment_id: attachment.clientAttachmentID,
        attachment_id: uploaded.id,
        filename: uploaded.filename || attachment.filename,
        content_type: uploaded.content_type || attachment.contentType,
        size_bytes: uploaded.size_bytes || attachment.sizeBytes,
      });
    }
  } catch (error) {
    throw new AttachmentCommitError(error.message, {
      destination_kind: "note_attachment",
      note_id: noteID,
      note_scope: noteScope,
      note_title: noteTitle,
      attachment_ids: attachmentIDs,
      files,
    });
  }
  removeSubmittedAttachments(selectedAttachments);
  return {
    destination_kind: "note_attachment",
    note_id: noteID,
    note_scope: noteScope,
    note_title: noteTitle,
    attachment_ids: attachmentIDs,
    files,
  };
}

async function commitFileServerAttachments(argumentsPayload, selectedAttachments, attachmentIDs) {
  const targetPath = normalizeFilePath(argumentsPayload.target_path);
  const files = [];
  let existingNames = new Set();
  try {
    const listing = await sendCommand("files.list", { path: targetPath });
    existingNames = new Set((listing.items || []).map((item) => item.name).filter(Boolean));
  } catch (_) {
    existingNames = new Set();
  }
  try {
    for (const attachment of selectedAttachments) {
      const targetName = uniqueCopyName(attachment.filename || "Attachment", existingNames);
      existingNames.add(targetName);
      const uploaded = await uploadFileServerAttachment(targetPath, attachment, targetName);
      files.push({
        client_attachment_id: attachment.clientAttachmentID,
        filename: targetName,
        path: uploaded.path,
        content_type: attachment.contentType,
        size_bytes: uploaded.payload?.size || attachment.sizeBytes,
      });
    }
  } catch (error) {
    throw new AttachmentCommitError(error.message, {
      destination_kind: "smb",
      target_path: targetPath,
      attachment_ids: attachmentIDs,
      files,
    });
  }
  removeSubmittedAttachments(selectedAttachments);
  return {
    destination_kind: "smb",
    target_path: targetPath,
    attachment_ids: attachmentIDs,
    files,
  };
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
      if (run.client_tool_request?.tool_name === "attachments.commit") {
        els.runState.textContent = "Uploading";
        run = await executeClientToolRun(run);
        continue;
      }
      els.runState.textContent = "Use iPhone";
      showToast("This action needs the Hank iPhone app to finish.", true);
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
    if (state.logsVisible) {
      await loadLogs();
    }
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
els.deleteSessionButton.addEventListener("click", () => deleteSession().catch((error) => showToast(error.message, true)));
els.logButton?.addEventListener("click", () => toggleLogs().catch((error) => showToast(error.message, true)));
els.refreshLogsButton?.addEventListener("click", () => loadLogs().then(() => showToast("Workflow logs refreshed.")).catch((error) => showToast(error.message, true)));
els.copyLogsButton?.addEventListener("click", () => copyLogs().catch((error) => showToast(error.message, true)));
els.clearLogsButton?.addEventListener("click", () => clearLogs().catch((error) => showToast(error.message, true)));
els.sessionList.addEventListener("click", (event) => {
  const deleteButton = event.target.closest("[data-delete-session-id]");
  if (deleteButton) {
    deleteSession(deleteButton.dataset.deleteSessionId).catch((error) => showToast(error.message, true));
    return;
  }
  const button = event.target.closest("[data-session-id]");
  if (!button) return;
  loadMessages(button.dataset.sessionId).catch((error) => showToast(error.message, true));
});
els.messageForm.addEventListener("submit", sendMessage);
els.messageInput.addEventListener("keydown", handleMessageInputKeydown);
els.attachButton.addEventListener("click", () => els.attachmentInput.click());
els.attachmentInput.addEventListener("change", () => {
  addAttachmentsFromFiles(els.attachmentInput.files).catch((error) => showToast(error.message, true));
});
els.attachmentTray.addEventListener("click", (event) => {
  const button = event.target.closest("[data-attachment-id]");
  if (!button) return;
  removeDraftAttachment(button.dataset.attachmentId);
});
els.confirmButton.addEventListener("click", () => confirmPending(true));
els.cancelButton.addEventListener("click", () => confirmPending(false));
window.addEventListener("beforeunload", closeAppSocket);

hydrate();
