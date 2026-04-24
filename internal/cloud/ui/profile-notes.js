const state = {
  user: null,
  notes: [],
  selectedNoteID: "",
  currentRevision: "",
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  noteCount: document.getElementById("note-count"),
  refreshButton: document.getElementById("refresh-button"),
  newButton: document.getElementById("new-button"),
  noteList: document.getElementById("note-list"),
  noteTitle: document.getElementById("note-title"),
  noteID: document.getElementById("note-id"),
  noteContent: document.getElementById("note-content"),
  saveButton: document.getElementById("save-button"),
  deleteButton: document.getElementById("delete-button"),
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

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function clearEditor() {
  state.selectedNoteID = "";
  state.currentRevision = "";
  els.noteTitle.value = "";
  els.noteID.value = "";
  els.noteContent.value = "";
  els.deleteButton.disabled = true;
}

function renderNotes() {
  els.noteCount.textContent = `${state.notes.length} note${state.notes.length === 1 ? "" : "s"}`;
  if (!state.notes.length) {
    els.noteList.className = "card-list empty-state";
    els.noteList.textContent = "No notes yet.";
    return;
  }

  els.noteList.className = "card-list";
  els.noteList.innerHTML = "";
  state.notes.forEach((note) => {
    const card = document.createElement("article");
    card.className = "card";
    const selected = note.id === state.selectedNoteID;
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(note.title || note.id)}</div>
          <div class="meta">${escapeHTML(note.id)}</div>
        </div>
        <span class="pill">${escapeHTML(note.page_type || "text")}</span>
      </div>
      <div class="meta">Updated ${escapeHTML(formatDate(note.updated_at))}</div>
      <div class="meta">${escapeHTML(note.preview || "")}</div>
    `;
    if (selected) {
      card.style.borderColor = "rgba(196, 96, 45, 0.65)";
    }
    card.addEventListener("click", async () => {
      await loadNote(note.id);
    });
    els.noteList.appendChild(card);
  });
}

function fillEditor(note) {
  state.selectedNoteID = note.note_id;
  state.currentRevision = note.revision || "";
  els.noteTitle.value = note.title || "";
  els.noteID.value = note.note_id || "";
  els.noteContent.value = note.content || "";
  els.deleteButton.disabled = false;
  renderNotes();
}

async function loadNotes() {
  state.notes = (await api("/v1/me/notes")).notes || [];
  if (state.selectedNoteID && !state.notes.some((note) => note.id === state.selectedNoteID)) {
    clearEditor();
  }
  renderNotes();
}

async function loadNote(noteID) {
  try {
    const note = await api(`/v1/me/notes/${encodeURIComponent(noteID)}`);
    fillEditor(note);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function saveNote() {
  try {
    const noteID = els.noteID.value.trim();
    const payload = {
      note_id: noteID,
      title: els.noteTitle.value.trim(),
      content: els.noteContent.value,
      expected_revision: state.currentRevision,
      page_type: "text",
    };

    let response;
    if (state.selectedNoteID) {
      response = await api(`/v1/me/notes/${encodeURIComponent(state.selectedNoteID)}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      });
    } else {
      response = await api("/v1/me/notes", {
        method: "POST",
        body: JSON.stringify(payload),
      });
    }

    await loadNotes();
    await loadNote(response.note_id);
    showToast("Note saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function deleteNote() {
  if (!state.selectedNoteID) {
    showToast("Choose a note first.", true);
    return;
  }
  if (!window.confirm(`Delete ${els.noteTitle.value || state.selectedNoteID}?`)) {
    return;
  }
  try {
    await api(`/v1/me/notes/${encodeURIComponent(state.selectedNoteID)}`, { method: "DELETE" });
    clearEditor();
    await loadNotes();
    showToast("Note deleted.");
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
    clearEditor();
    await loadNotes();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.refreshButton.addEventListener("click", async () => {
  try {
    await loadNotes();
    showToast("Notes refreshed.");
  } catch (error) {
    showToast(error.message, true);
  }
});
els.newButton.addEventListener("click", () => {
  clearEditor();
  renderNotes();
});
els.saveButton.addEventListener("click", saveNote);
els.deleteButton.addEventListener("click", deleteNote);

hydrate();
