const api = window.HankAPI.request;

const state = {
  user: null,
  bundle: null,
  preview: null,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  exportButton: document.getElementById("export-button"),
  importForm: document.getElementById("import-form"),
  bundleFile: document.getElementById("bundle-file"),
  applyButton: document.getElementById("apply-button"),
  previewState: document.getElementById("preview-state"),
  changeCount: document.getElementById("change-count"),
  changesList: document.getElementById("changes-list"),
  secretCount: document.getElementById("secret-count"),
  secretsList: document.getElementById("secrets-list"),
  toast: document.getElementById("toast"),
};

function showToast(message, isError = false) {
  els.toast.hidden = false;
  els.toast.textContent = message;
  els.toast.style.background = isError ? "rgba(142, 45, 28, 0.94)" : "rgba(35, 27, 20, 0.92)";
  clearTimeout(showToast.timeoutID);
  showToast.timeoutID = window.setTimeout(() => {
    els.toast.hidden = true;
  }, 3600);
}

function escapeHTML(value) {
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Admin recovery tools are available.";
}

function downloadJSON(filename, payload) {
  const blob = new Blob([JSON.stringify(payload, null, 2), "\n"], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

async function exportBundle() {
  try {
    const bundle = await api("/v1/home/recovery/export");
    downloadJSON("hank-recovery-settings.json", bundle);
    showToast("Recovery bundle exported.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function readBundleFile() {
  const file = els.bundleFile.files?.[0];
  if (!file) {
    throw new Error("Choose a recovery bundle first.");
  }
  if (file.size > 1024 * 1024) {
    throw new Error("Recovery bundle is too large.");
  }
  const text = await file.text();
  try {
    return JSON.parse(text);
  } catch (_) {
    throw new Error("Recovery bundle must be valid JSON.");
  }
}

function renderChanges(changes = []) {
  els.changeCount.textContent = `${changes.length} change${changes.length === 1 ? "" : "s"}`;
  if (!changes.length) {
    els.changesList.className = "card-list empty-state";
    els.changesList.textContent = "No changes found in this bundle.";
    return;
  }
  els.changesList.className = "card-list";
  els.changesList.innerHTML = changes.map((change) => `
    <article class="card split-card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(change.target || change.area)}</div>
          <div class="meta">${escapeHTML(change.area)}</div>
        </div>
        <span class="status-chip">${escapeHTML(change.action || "update")}</span>
      </div>
    </article>
  `).join("");
}

function renderSecrets(secrets = []) {
  els.secretCount.textContent = `${secrets.length} secret${secrets.length === 1 ? "" : "s"}`;
  if (!secrets.length) {
    els.secretsList.className = "card-list empty-state";
    els.secretsList.textContent = "No missing connection secrets were reported.";
    return;
  }
  els.secretsList.className = "card-list";
  els.secretsList.innerHTML = secrets.map((secret) => `
    <article class="card split-card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(secret.label || secret.id)}</div>
          <div class="meta">${escapeHTML(secret.service_type || "connection")}</div>
        </div>
        <span class="status-chip offline">required</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Field</div><div>${escapeHTML(secret.target?.field || secret.kind || "secret")}</div></div>
        <div class="kv-row"><div class="kv-label">ID</div><div>${escapeHTML(secret.id)}</div></div>
      </div>
    </article>
  `).join("");
}

function renderPreview(preview) {
  state.preview = preview;
  els.previewState.textContent = preview?.valid ? "Ready" : "Invalid";
  els.applyButton.disabled = !preview?.valid;
  renderChanges(preview?.changes || []);
  renderSecrets(preview?.required_secrets || []);
}

async function previewImport(event) {
  event.preventDefault();
  try {
    state.bundle = await readBundleFile();
    const preview = await api("/v1/home/recovery/import/preview", {
      method: "POST",
      body: JSON.stringify(state.bundle),
    });
    renderPreview(preview);
    showToast("Recovery bundle previewed.");
  } catch (error) {
    state.bundle = null;
    renderPreview(null);
    showToast(error.message, true);
  }
}

async function applyImport() {
  if (!state.bundle || !state.preview?.valid) {
    showToast("Preview a valid recovery bundle first.", true);
    return;
  }
  try {
    const result = await api("/v1/home/recovery/import/apply", {
      method: "POST",
      body: JSON.stringify({ bundle: state.bundle, confirm: true }),
    });
    renderChanges(result.changes || []);
    renderSecrets(result.required_secrets || []);
    els.previewState.textContent = "Applied";
    showToast("Non-secret recovery settings applied.");
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
    await api("/v1/home/recovery/export");
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton?.addEventListener("click", logout);
els.exportButton?.addEventListener("click", exportBundle);
els.importForm?.addEventListener("submit", previewImport);
els.applyButton?.addEventListener("click", applyImport);
hydrate();
