const AUTOSAVE_DELAY_MS = 700;
const HISTORY_LIMIT = 20;
const SELECTED_NOTE_STORAGE_KEY = "hank.remote.profileNotes.selectedNoteID";
const DRAFT_HISTORY_KEY = "__draft__";

const state = {
  user: null,
  notes: [],
  selectedNoteID: "",
  currentRevision: "",
  appSocket: null,
  appSocketPromise: null,
  pendingRequests: new Map(),
  requestCounter: 0,
  liveRefreshPending: false,
  reconnectTimer: null,
  autosaveTimer: null,
  isDirty: false,
  isSaving: false,
  lastSavedHash: "",
  suppressInput: false,
  historyRestore: false,
  noteHistories: new Map(),
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  noteCount: document.getElementById("note-count"),
  noteSearch: document.getElementById("note-search"),
  refreshButton: document.getElementById("refresh-button"),
  newButton: document.getElementById("new-button"),
  noteTabs: document.getElementById("note-tabs") || document.getElementById("note-list"),
  noteTitle: document.getElementById("note-title"),
  noteContent: document.getElementById("note-content"),
  noteInline: document.getElementById("note-inline-layer"),
  saveState: document.getElementById("save-state"),
  lastSaved: document.getElementById("last-saved"),
  deleteButton: document.getElementById("delete-button"),
  toast: document.getElementById("toast"),
  formatButtons: Array.from(document.querySelectorAll("[data-format]")),
};

function logLive(message, detail = {}) {
  console.info("[Hank Remote Notes]", message, detail);
}

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
  }, 3200);
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

function formatModifiedTime(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const now = new Date();
  const isToday = date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();
  return isToday ? date.toLocaleTimeString() : date.toLocaleString();
}

function startupParams() {
  return new URLSearchParams(window.location.search);
}

function requestedNoteID() {
  return startupParams().get("note_id") || "";
}

function requestedSearchText() {
  return startupParams().get("search") || "";
}

function rememberedNoteID() {
  try {
    return window.localStorage.getItem(SELECTED_NOTE_STORAGE_KEY) || "";
  } catch (_) {
    return "";
  }
}

function rememberNoteID(noteID) {
  try {
    if (noteID) {
      window.localStorage.setItem(SELECTED_NOTE_STORAGE_KEY, noteID);
    } else {
      window.localStorage.removeItem(SELECTED_NOTE_STORAGE_KEY);
    }
  } catch (_) {
  }
}

function preferredAppSocketURL() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws/app`;
}

function nextRequestID() {
  state.requestCounter += 1;
  if (window.crypto?.randomUUID) return `notes-${window.crypto.randomUUID()}`;
  return `notes-${Date.now()}-${state.requestCounter}`;
}

function closeAppSocket(scheduleReconnect = true) {
  logLive("closing live notes socket", { scheduleReconnect });
  if (state.appSocket) {
    try {
      state.appSocket.close();
    } catch (_) {
    }
  }
  state.appSocket = null;
  state.appSocketPromise = null;
  for (const { reject } of state.pendingRequests.values()) {
    reject(new Error("Live notes connection closed."));
  }
  state.pendingRequests.clear();
  if (scheduleReconnect && !document.hidden && !state.reconnectTimer) {
    logLive("scheduling live notes reconnect");
    state.reconnectTimer = window.setTimeout(() => {
      state.reconnectTimer = null;
      connectLiveNotes().catch(() => {});
    }, 1500);
  }
}

function handleSocketMessage(event) {
  let envelope;
  try {
    envelope = JSON.parse(event.data);
  } catch (_) {
    return;
  }

  const pending = state.pendingRequests.get(envelope.request_id);
  if (pending) {
    state.pendingRequests.delete(envelope.request_id);
    if (envelope.type === "app.error" || envelope.error) {
      pending.reject(new Error(envelope.error?.message || "Live notes command failed."));
      return;
    }
    pending.resolve(envelope.payload ?? null);
    return;
  }

  if (envelope.type !== "app.event") {
    return;
  }
  const appEvent = envelope.payload || {};
  logLive("received live notes event", { event: appEvent.event, topic: appEvent.topic });
  if (appEvent.topic === "notes.profile" && ["notes.changed", "notes.deleted"].includes(appEvent.event)) {
    scheduleLiveRefresh();
  }
}

function sendSocketCommand(command, body = {}) {
  const socket = state.appSocket;
  if (!socket || socket.readyState !== WebSocket.OPEN) {
    return Promise.reject(new Error("Live notes connection is not open."));
  }
  logLive("sending live notes command", { command, topics: body.topics || [] });
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

async function connectLiveNotes() {
  if (state.appSocket?.readyState === WebSocket.OPEN) {
    return state.appSocket;
  }
  if (state.appSocketPromise) {
    return state.appSocketPromise;
  }

  state.appSocketPromise = new Promise((resolve, reject) => {
    logLive("opening live notes socket", { url: preferredAppSocketURL() });
    const socket = new WebSocket(preferredAppSocketURL());
    state.appSocket = socket;
    socket.addEventListener("open", async () => {
      state.appSocketPromise = null;
      try {
        logLive("live notes socket opened");
        await sendSocketCommand("app.subscribe", { topics: ["notes.profile"] });
        logLive("subscribed live notes topics", { topics: ["notes.profile"] });
        resolve(socket);
      } catch (error) {
        logLive("live notes subscribe failed", { error: error.message });
        reject(error);
      }
    }, { once: true });
    socket.addEventListener("message", handleSocketMessage);
    socket.addEventListener("close", () => {
      logLive("live notes socket closed");
      if (state.appSocket === socket) {
        closeAppSocket();
      }
    });
    socket.addEventListener("error", () => {
      logLive("live notes socket error");
      if (state.appSocket === socket) {
        closeAppSocket();
      }
      reject(new Error("Live notes connection failed."));
    }, { once: true });
  });
  return state.appSocketPromise;
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function normalizedTitle() {
  return els.noteTitle.value.trim() || "Untitled";
}

function currentMarkdown() {
  return els.noteContent.value.replace(/\r\n/g, "\n");
}

function setEditorValue(value) {
  els.noteContent.value = value || "";
  renderEditorExtras();
}

function currentEditorHash() {
  return `${state.selectedNoteID}\n${normalizedTitle()}\n${currentMarkdown()}`;
}

function activeHistoryKey() {
  return state.selectedNoteID || DRAFT_HISTORY_KEY;
}

function currentEditorSnapshot() {
  return {
    title: els.noteTitle.value,
    content: els.noteContent.value,
    selectionStart: els.noteContent.selectionStart || 0,
    selectionEnd: els.noteContent.selectionEnd || 0,
  };
}

function sameEditorSnapshot(left, right) {
  return Boolean(left && right && left.title === right.title && left.content === right.content);
}

function ensureNoteHistory(noteID = activeHistoryKey()) {
  const key = noteID || DRAFT_HISTORY_KEY;
  if (!state.noteHistories.has(key)) {
    state.noteHistories.set(key, { undo: [], redo: [], current: null });
  }
  return state.noteHistories.get(key);
}

function updateHistoryButtons() {
  const history = ensureNoteHistory();
  for (const button of els.formatButtons) {
    if (button.dataset.format === "undo") {
      button.disabled = history.undo.length === 0;
    } else if (button.dataset.format === "redo") {
      button.disabled = history.redo.length === 0;
    }
  }
}

function trimHistoryStack(stack) {
  while (stack.length > HISTORY_LIMIT) {
    stack.shift();
  }
}

function setHistoryCurrent(noteID = activeHistoryKey()) {
  const history = ensureNoteHistory(noteID);
  history.current = currentEditorSnapshot();
  updateHistoryButtons();
}

function recordEditorHistoryChange() {
  if (state.suppressInput || state.historyRestore) {
    return;
  }
  const history = ensureNoteHistory();
  const next = currentEditorSnapshot();
  if (!history.current) {
    history.current = next;
    updateHistoryButtons();
    return;
  }
  if (sameEditorSnapshot(history.current, next)) {
    return;
  }
  history.undo.push(history.current);
  trimHistoryStack(history.undo);
  history.redo = [];
  history.current = next;
  updateHistoryButtons();
}

function restoreEditorSnapshot(snapshot) {
  if (!snapshot) {
    return;
  }
  state.historyRestore = true;
  els.noteTitle.value = snapshot.title || "";
  els.noteContent.value = snapshot.content || "";
  const selectionStart = Math.min(snapshot.selectionStart || 0, els.noteContent.value.length);
  const selectionEnd = Math.min(snapshot.selectionEnd || selectionStart, els.noteContent.value.length);
  els.noteContent.focus();
  els.noteContent.setSelectionRange(selectionStart, selectionEnd);
  state.historyRestore = false;
  renderEditorExtras();
  markDirty();
}

function undoEditor() {
  const history = ensureNoteHistory();
  if (!history.undo.length) {
    return;
  }
  const current = currentEditorSnapshot();
  const snapshot = history.undo.pop();
  history.redo.push(current);
  trimHistoryStack(history.redo);
  history.current = snapshot;
  restoreEditorSnapshot(snapshot);
  updateHistoryButtons();
}

function redoEditor() {
  const history = ensureNoteHistory();
  if (!history.redo.length) {
    return;
  }
  const current = currentEditorSnapshot();
  const snapshot = history.redo.pop();
  history.undo.push(current);
  trimHistoryStack(history.undo);
  history.current = snapshot;
  restoreEditorSnapshot(snapshot);
  updateHistoryButtons();
}

function moveNoteHistory(fromNoteID, toNoteID) {
  if (!fromNoteID || !toNoteID || fromNoteID === toNoteID || !state.noteHistories.has(fromNoteID)) {
    return;
  }
  state.noteHistories.set(toNoteID, state.noteHistories.get(fromNoteID));
  state.noteHistories.delete(fromNoteID);
  updateHistoryButtons();
}

function deleteNoteHistory(noteID) {
  if (noteID) {
    state.noteHistories.delete(noteID);
  }
}

function previewFromMarkdown(markdown) {
  return markdown
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/^\s*[-*]\s+\[[ xX]\]\s+/gm, "")
    .replace(/^\s*[-*]\s+/gm, "")
    .replace(/^\s*\d+\.\s+/gm, "")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 96);
}

function setSaveState(label, mode = "") {
  els.saveState.textContent = label;
  els.saveState.className = `save-state${mode ? ` ${mode}` : ""}`;
}

function setLastSaved(value) {
  const modified = formatModifiedTime(value);
  els.lastSaved.textContent = modified ? `Modified last ${modified}` : "Not modified yet";
}

function editorHasFocus() {
  return [els.noteTitle, els.noteContent].includes(document.activeElement);
}

function clearEditor() {
  window.clearTimeout(state.autosaveTimer);
  state.selectedNoteID = "";
  state.currentRevision = "";
  state.isDirty = false;
  state.isSaving = false;
  state.suppressInput = true;
  els.noteTitle.value = "";
  setEditorValue("");
  state.suppressInput = false;
  state.lastSavedHash = currentEditorHash();
  els.deleteButton.disabled = true;
  setHistoryCurrent(DRAFT_HISTORY_KEY);
  renderEditorExtras();
  setSaveState("Saved");
  setLastSaved("");
  renderNotes();
}

function filteredNotes() {
  const query = els.noteSearch.value.trim().toLowerCase();
  const notes = sortNotesByRecency(state.notes);
  if (!query) {
    return notes;
  }
  return notes.filter((note) => {
    const haystack = [
      note.title,
      note.id,
      note.preview,
      ...(note.tags || []),
    ].join(" ").toLowerCase();
    return haystack.includes(query);
  });
}

function noteUpdatedAt(note) {
  const timestamp = Date.parse(note?.updated_at || "");
  return Number.isNaN(timestamp) ? 0 : timestamp;
}

function sortNotesByRecency(notes) {
  return [...notes].sort((left, right) => {
    const rightUpdated = noteUpdatedAt(right);
    const leftUpdated = noteUpdatedAt(left);
    if (rightUpdated === leftUpdated) {
      return String(left.title || left.id || "").localeCompare(String(right.title || right.id || ""));
    }
    return rightUpdated - leftUpdated;
  });
}

function renderNotes() {
  els.noteCount.textContent = `${state.notes.length} note${state.notes.length === 1 ? "" : "s"}`;
  const notes = filteredNotes();
  if (!state.notes.length) {
    els.noteTabs.className = "note-tabs empty-state";
    els.noteTabs.textContent = "No notes yet.";
    return;
  }
  if (!notes.length) {
    els.noteTabs.className = "note-tabs empty-state";
    els.noteTabs.textContent = "No matching notes.";
    return;
  }

  els.noteTabs.className = "note-tabs";
  els.noteTabs.innerHTML = "";
  notes.forEach((note) => {
    const tab = document.createElement("button");
    tab.type = "button";
    tab.className = `note-tab${note.id === state.selectedNoteID ? " active" : ""}`;
    tab.innerHTML = `
      <span class="note-tab-title">${escapeHTML(note.title || note.id)}</span>
      <span class="note-tab-meta">${escapeHTML(formatDate(note.updated_at))}</span>
    `;
    tab.addEventListener("click", () => {
      selectNote(note.id).catch((error) => showToast(error.message, true));
    });
    els.noteTabs.appendChild(tab);
  });
}

function updateSelectedSummaryDraft() {
  if (!state.selectedNoteID) {
    return;
  }
  const note = state.notes.find((item) => item.id === state.selectedNoteID);
  if (!note) {
    return;
  }
  note.title = normalizedTitle();
  note.preview = previewFromMarkdown(currentMarkdown());
  note.updated_at = new Date().toISOString();
  renderNotes();
}

function fillEditor(note) {
  window.clearTimeout(state.autosaveTimer);
  state.selectedNoteID = note.note_id;
  rememberNoteID(state.selectedNoteID);
  state.currentRevision = note.revision || "";
  state.isDirty = false;
  state.isSaving = false;
  state.suppressInput = true;
  els.noteTitle.value = note.title || "";
  setEditorValue(note.body_markdown || note.content || "");
  state.suppressInput = false;
  state.lastSavedHash = currentEditorHash();
  els.deleteButton.disabled = false;
  setHistoryCurrent(state.selectedNoteID);
  renderEditorExtras();
  setSaveState("Saved");
  setLastSaved(note.updated_at);
  renderNotes();
}

function noteIdentifier(note) {
  return note?.id || note?.note_id || "";
}

function findListedNote(noteID) {
  return state.notes.find((note) => noteIdentifier(note) === noteID || note.note_id === noteID || note.id === noteID) || null;
}

async function loadNotes() {
  logLive("loading profile notes list");
  state.notes = sortNotesByRecency((await api("/v1/me/notes")).notes || []);
  if (state.selectedNoteID && !state.notes.some((note) => note.id === state.selectedNoteID)) {
    clearEditor();
  }
  renderNotes();
}

async function loadNote(noteID) {
  logLive("loading profile note", { noteID });
  const note = await api(`/v1/me/notes/${encodeURIComponent(noteID)}`);
  fillEditor(note);
}

async function selectNote(noteID) {
  if (state.selectedNoteID === noteID && !state.isDirty) {
    return;
  }
  if (state.isDirty) {
    await saveNote({ force: true });
  }
  await loadNote(noteID);
}

async function openRequestedNote() {
  const noteID = requestedNoteID();
  const searchText = requestedSearchText();
  if (searchText) {
    els.noteSearch.value = searchText;
    renderNotes();
  }
  if (!noteID) {
    return false;
  }
  const listed = findListedNote(noteID);
  try {
    await loadNote(noteID);
    return true;
  } catch (error) {
    if (listed) {
      await loadNote(noteIdentifier(listed));
      return true;
    }
    showToast("That note is not available in My Notes yet.", true);
    return false;
  }
}

function markDirty() {
  if (state.suppressInput) {
    return;
  }
  const hasDraft = Boolean(state.selectedNoteID || els.noteTitle.value.trim() || els.noteContent.value.trim());
  if (!hasDraft) {
    return;
  }
  state.isDirty = true;
  setSaveState("Editing", "dirty");
  updateSelectedSummaryDraft();
  scheduleAutosave();
}

function scheduleAutosave() {
  window.clearTimeout(state.autosaveTimer);
  state.autosaveTimer = window.setTimeout(() => {
    saveNote().catch((error) => showToast(error.message, true));
  }, AUTOSAVE_DELAY_MS);
}

async function saveNote(options = {}) {
  const { force = false } = options;
  if (state.isSaving) {
    scheduleAutosave();
    return false;
  }
  const nextHash = currentEditorHash();
  if (!force && (!state.isDirty || nextHash === state.lastSavedHash)) {
    return true;
  }
  if (!state.selectedNoteID && !els.noteTitle.value.trim() && !els.noteContent.value.trim()) {
    return true;
  }

  state.isSaving = true;
  setSaveState("Saving", "saving");
  const previousNoteID = state.selectedNoteID;

  const payload = {
    note_id: state.selectedNoteID,
    title: normalizedTitle(),
    content: currentMarkdown(),
    body_markdown: currentMarkdown(),
    body_format: "markdown",
    expected_revision: state.currentRevision,
    page_type: "text",
  };

  try {
    let response;
    if (state.selectedNoteID) {
      logLive("autosaving existing profile note", { noteID: state.selectedNoteID });
      response = await api(`/v1/me/notes/${encodeURIComponent(state.selectedNoteID)}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      });
    } else {
      logLive("autosaving new profile note");
      response = await api("/v1/me/notes", {
        method: "POST",
        body: JSON.stringify(payload),
      });
    }

    state.selectedNoteID = response.note_id || state.selectedNoteID;
    if (!previousNoteID && state.selectedNoteID) {
      moveNoteHistory(DRAFT_HISTORY_KEY, state.selectedNoteID);
    }
    state.currentRevision = response.revision || "";
    state.isDirty = false;
    state.lastSavedHash = currentEditorHash();
    setHistoryCurrent(state.selectedNoteID);
    renderEditorExtras();
    setSaveState("Saved");
    setLastSaved(response.updated_at || new Date().toISOString());
    await loadNotes();
    return true;
  } catch (error) {
    setSaveState("Not saved", "error");
    if (String(error.message || "").includes("note_conflict") && state.selectedNoteID) {
      showToast("This note changed elsewhere. Reloading the latest version.", true);
      state.isDirty = false;
      await loadNote(state.selectedNoteID);
      return false;
    }
    showToast(error.message, true);
    return false;
  } finally {
    state.isSaving = false;
  }
}

async function createNote() {
  if (state.isDirty) {
    await saveNote({ force: true });
  }
  clearEditor();
  els.noteTitle.value = "Untitled";
  state.isDirty = true;
  await saveNote({ force: true });
  els.noteTitle.focus();
  els.noteTitle.select();
}

async function deleteNote() {
  if (!state.selectedNoteID) {
    showToast("Choose a note first.", true);
    return;
  }
  const title = els.noteTitle.value.trim() || "this note";
  if (!window.confirm(`Delete "${title}"?\n\nThis cannot be undone.`)) {
    return;
  }
  try {
    logLive("deleting profile note", { noteID: state.selectedNoteID });
    const deletedNoteID = state.selectedNoteID;
    await api(`/v1/me/notes/${encodeURIComponent(state.selectedNoteID)}`, { method: "DELETE" });
    deleteNoteHistory(deletedNoteID);
    rememberNoteID("");
    clearEditor();
    await loadNotes();
    showToast("Note deleted.");
    if (state.notes[0]) {
      await loadNote(state.notes[0].id);
    }
  } catch (error) {
    showToast(error.message, true);
  }
}

async function logout() {
  try {
    await saveNote({ force: true });
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
    clearEditor();
    await loadNotes();
    const openedRequested = await openRequestedNote();
    if (!openedRequested) {
      const remembered = rememberedNoteID();
      const rememberedExists = remembered && state.notes.some((note) => noteIdentifier(note) === remembered);
      if (rememberedExists) {
        await loadNote(remembered);
      } else if (state.notes[0]) {
        await loadNote(state.notes[0].id);
      }
    }
  } catch (_) {
    window.location.replace("/");
  }
}

async function refreshLiveNotes() {
  const selectedNoteID = state.selectedNoteID;
  await loadNotes();
  const selectedStillExists = selectedNoteID && state.notes.some((note) => note.id === selectedNoteID);
  if (!selectedStillExists) {
    return;
  }
  if (document.hidden || editorHasFocus() || state.isDirty || state.isSaving) {
    logLive("deferring live notes refresh", {
      hidden: document.hidden,
      editorFocused: editorHasFocus(),
      dirty: state.isDirty,
      saving: state.isSaving,
    });
    state.liveRefreshPending = true;
    return;
  }
  state.liveRefreshPending = false;
  logLive("refreshing selected note from live event");
  await loadNote(selectedNoteID);
}

function scheduleLiveRefresh() {
  window.clearTimeout(scheduleLiveRefresh.timeoutID);
  scheduleLiveRefresh.timeoutID = window.setTimeout(() => {
    refreshLiveNotes().catch(() => {});
  }, 150);
}

function replaceText(start, end, replacement, selectionStart, selectionEnd) {
  els.noteContent.setRangeText(replacement, start, end, "preserve");
  els.noteContent.focus();
  els.noteContent.setSelectionRange(selectionStart, selectionEnd);
  recordEditorHistoryChange();
  renderEditorExtras();
  markDirty();
}

function wrapSelection(before, after = before, placeholder = "text") {
  const start = els.noteContent.selectionStart;
  const end = els.noteContent.selectionEnd;
  const selected = els.noteContent.value.slice(start, end) || placeholder;
  const replacement = `${before}${selected}${after}`;
  const innerStart = start + before.length;
  replaceText(start, end, replacement, innerStart, innerStart + selected.length);
}

function selectedLineBounds() {
  const value = els.noteContent.value;
  const start = els.noteContent.selectionStart;
  const end = els.noteContent.selectionEnd;
  const lineStart = value.lastIndexOf("\n", Math.max(0, start - 1)) + 1;
  const nextBreak = value.indexOf("\n", end);
  return {
    start: lineStart,
    end: nextBreak === -1 ? value.length : nextBreak,
  };
}

function transformSelectedLines(transform) {
  const bounds = selectedLineBounds();
  const value = els.noteContent.value;
  const segment = value.slice(bounds.start, bounds.end);
  const lines = segment.split("\n");
  const replacement = lines.map(transform).join("\n");
  replaceText(bounds.start, bounds.end, replacement, bounds.start, bounds.start + replacement.length);
}

function stripLinePrefix(line) {
  return line
    .replace(/^\s*#{1,6}\s+/, "")
    .replace(/^\s*[-*]\s+\[[ xX]\]\s+/, "")
    .replace(/^\s*[-*]\s+/, "")
    .replace(/^\s*\d+\.\s+/, "");
}

function headingLevel(line) {
  const match = line.match(/^\s*(#{1,6})\s+/);
  return match ? match[1].length : 0;
}

function setHeading(line, level) {
  const text = stripLinePrefix(line);
  if (!text.trim()) {
    return level > 0 ? `${"#".repeat(level)} ` : line;
  }
  return level > 0 ? `${"#".repeat(level)} ${text}` : text;
}

function toggleHeading() {
  transformSelectedLines((line) => setHeading(line, headingLevel(line) > 0 ? 0 : 2));
}

function adjustHeadingSize(direction) {
  transformSelectedLines((line) => {
    const current = headingLevel(line);
    if (direction > 0) {
      const next = current === 0 ? 2 : Math.max(1, current - 1);
      return setHeading(line, next);
    }
    if (current === 0) {
      return line;
    }
    const next = current >= 6 ? 0 : current + 1;
    return setHeading(line, next);
  });
}

function applyLinePrefix(prefixForLine) {
  transformSelectedLines((line, index) => {
    const text = stripLinePrefix(line);
    if (!text.trim()) {
      return prefixForLine(index);
    }
    return `${prefixForLine(index)}${text}`;
  });
}

function insertAtCursor(text) {
  const start = els.noteContent.selectionStart;
  const end = els.noteContent.selectionEnd;
  replaceText(start, end, text, start + text.length, start + text.length);
}

function normalizeTag(value) {
  return String(value || "")
    .trim()
    .replace(/^#/, "")
    .replace(/\s+/g, "-")
    .replace(/[^A-Za-z0-9_-]/g, "");
}

function normalizeURL(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return "";
  }
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) {
    return trimmed;
  }
  return `https://${trimmed}`;
}

function isLikelyURL(value) {
  return /^(https?:\/\/|www\.|[A-Za-z0-9-]+\.[A-Za-z]{2,})/.test(String(value || "").trim());
}

function cleanLinkToken(value) {
  return String(value || "")
    .trim()
    .replace(/^["'`<([{]+/, "")
    .replace(/[>"'`)\]},.;!?]+$/g, "");
}

function inlineLinkHTML(label, rawURL) {
  const cleaned = cleanLinkToken(rawURL);
  if (!cleaned || !isLikelyURL(cleaned)) {
    return escapeHTML(label || rawURL);
  }
  const href = normalizeURL(cleaned);
  const text = String(label || cleaned).trim() || href;
  return `<a class="note-inline-link" href="${escapeHTML(href)}" target="_blank" rel="noopener noreferrer">${escapeHTML(text)}</a>`;
}

function renderBareLinks(text) {
  const source = String(text || "");
  const barePattern = /\b((?:https?:\/\/|www\.)[^\s<>()]+|[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9-]+)+(?:\/[^\s<>()]*)?)/g;
  let html = "";
  let lastIndex = 0;
  for (const match of source.matchAll(barePattern)) {
    const raw = match[1];
    const index = match.index || 0;
    html += escapeHTML(source.slice(lastIndex, index));
    html += inlineLinkHTML(raw, raw);
    lastIndex = index + raw.length;
  }
  html += escapeHTML(source.slice(lastIndex));
  return html;
}

function renderInlineText(text) {
  const source = String(text || "");
  const markdownLinkPattern = /\[([^\]\n]+)\]\(([^)\s]+)\)/g;
  let html = "";
  let lastIndex = 0;
  for (const match of source.matchAll(markdownLinkPattern)) {
    const index = match.index || 0;
    html += renderBareLinks(source.slice(lastIndex, index));
    html += inlineLinkHTML(match[1], match[2]);
    lastIndex = index + match[0].length;
  }
  html += renderBareLinks(source.slice(lastIndex));
  return html || "&nbsp;";
}

function renderInlineLine(line, lineIndex) {
  const checklist = String(line || "").match(/^(\s*)([-*]\s+\[)([ xX])(\]\s+)(.*)$/);
  if (!checklist) {
    return renderInlineText(line);
  }
  const checked = checklist[3].toLowerCase() === "x";
  const text = checklist[5] || "Checklist item";
  return `${escapeHTML(checklist[1])}<button type="button" class="note-check-toggle inline${checked ? " checked" : ""}" data-line-index="${lineIndex}" aria-pressed="${checked ? "true" : "false"}" title="${checked ? "Mark incomplete" : "Mark complete"}"><span class="note-check-circle" aria-hidden="true"></span></button><span class="note-check-text">${renderInlineText(text)}</span>`;
}

function renderInlineEditor() {
  const lines = currentMarkdown().split("\n");
  els.noteInline.innerHTML = lines
    .map((line, index) => `<div class="note-inline-line">${renderInlineLine(line, index)}</div>`)
    .join("");
}

function syncEditorScroll() {
  els.noteInline.scrollTop = els.noteContent.scrollTop;
  els.noteInline.scrollLeft = els.noteContent.scrollLeft;
}

function renderEditorExtras() {
  renderInlineEditor();
  syncEditorScroll();
}

function lineRangeForIndex(lines, lineIndex) {
  let start = 0;
  for (let index = 0; index < lineIndex; index += 1) {
    start += lines[index].length + 1;
  }
  return { start, end: start + lines[lineIndex].length };
}

function toggleChecklistLine(lineIndex) {
  const lines = currentMarkdown().split("\n");
  const line = lines[lineIndex] || "";
  const match = line.match(/^(\s*[-*]\s+\[)([ xX])(\]\s+)(.*)$/);
  if (!match) {
    return;
  }
  const replacement = `${match[1]}${match[2].toLowerCase() === "x" ? " " : "x"}${match[3]}${match[4]}`;
  const range = lineRangeForIndex(lines, lineIndex);
  replaceText(range.start, range.end, replacement, range.start, range.start + replacement.length);
}

function applyLink() {
  const start = els.noteContent.selectionStart;
  const end = els.noteContent.selectionEnd;
  const selected = els.noteContent.value.slice(start, end).trim();
  const initialURL = isLikelyURL(selected) ? selected : "";
  const rawURL = window.prompt("Link URL", initialURL);
  if (rawURL === null) {
    return;
  }
  const url = normalizeURL(rawURL);
  if (!url) {
    return;
  }
  const text = selected && !isLikelyURL(selected) ? selected : url;
  replaceText(start, end, `[${text}](${url})`, start, start + `[${text}](${url})`.length);
}

function applyFormat(format) {
  switch (format) {
  case "undo":
    undoEditor();
    break;
  case "redo":
    redoEditor();
    break;
  case "bold":
    wrapSelection("**");
    break;
  case "italic":
    wrapSelection("*");
    break;
  case "underline":
    wrapSelection("<u>", "</u>");
    break;
  case "decrease":
    adjustHeadingSize(-1);
    break;
  case "increase":
    adjustHeadingSize(1);
    break;
  case "heading":
    toggleHeading();
    break;
  case "bullet":
    applyLinePrefix(() => "- ");
    break;
  case "number":
    applyLinePrefix((index) => `${index + 1}. `);
    break;
  case "checklist":
    applyLinePrefix(() => "- [ ] ");
    break;
  case "tag": {
    const rawTag = window.prompt("Tag");
    const tag = normalizeTag(rawTag);
    if (tag) {
      insertAtCursor(`#${tag}`);
    }
    break;
  }
  case "link":
    applyLink();
    break;
  default:
    break;
  }
}

els.logoutButton.addEventListener("click", logout);
els.refreshButton.addEventListener("click", async () => {
  try {
    await saveNote({ force: true });
    await loadNotes();
    if (state.selectedNoteID) {
      await loadNote(state.selectedNoteID);
    }
    showToast("Notes refreshed.");
  } catch (error) {
    showToast(error.message, true);
  }
});
els.newButton.addEventListener("click", () => {
  createNote().catch((error) => showToast(error.message, true));
});
els.deleteButton.addEventListener("click", deleteNote);
els.noteSearch.addEventListener("input", renderNotes);
function handleEditorInput() {
  recordEditorHistoryChange();
  renderEditorExtras();
  markDirty();
}
els.noteTitle.addEventListener("input", handleEditorInput);
els.noteContent.addEventListener("input", handleEditorInput);
els.noteContent.addEventListener("scroll", syncEditorScroll);
els.noteContent.addEventListener("keydown", (event) => {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
    event.preventDefault();
    saveNote({ force: true }).catch((error) => showToast(error.message, true));
  }
});
els.noteInline.addEventListener("click", (event) => {
  const checklistButton = event.target.closest("[data-line-index]");
  if (!checklistButton) {
    return;
  }
  event.preventDefault();
  event.stopPropagation();
  toggleChecklistLine(Number(checklistButton.dataset.lineIndex));
});
els.formatButtons.forEach((button) => {
  button.addEventListener("mousedown", (event) => {
    event.preventDefault();
  });
  button.addEventListener("click", () => applyFormat(button.dataset.format));
});

hydrate();
connectLiveNotes().catch(() => {});
window.setInterval(() => {
  refreshLiveNotes().catch(() => {});
}, 10000);

document.addEventListener("visibilitychange", () => {
  if (document.hidden) {
    closeAppSocket(false);
    return;
  }
  connectLiveNotes().catch(() => {});
  refreshLiveNotes().catch(() => {});
});

[els.noteTitle, els.noteContent].forEach((field) => {
  field.addEventListener("blur", () => {
    if (state.isDirty) {
      saveNote({ force: true }).catch((error) => showToast(error.message, true));
      return;
    }
    if (state.liveRefreshPending) {
      refreshLiveNotes().catch(() => {});
    }
  });
});
