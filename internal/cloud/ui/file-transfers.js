const state = {
  user: null,
  homes: [],
  lastTransfer: null,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  homeSelect: document.getElementById("home-select"),
  transferOutput: document.getElementById("transfer-output"),
  downloadPath: document.getElementById("download-path"),
  downloadButton: document.getElementById("download-button"),
  uploadPath: document.getElementById("upload-path"),
  uploadFile: document.getElementById("upload-file"),
  uploadButton: document.getElementById("upload-button"),
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

function selectedHomeID() {
  return els.homeSelect.value;
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

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = `User ID ${state.user?.id || ""}`;
}

function renderHomes() {
  els.homeSelect.innerHTML = "";
  if (!state.homes.length) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No homes available";
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

function renderLastTransfer() {
  if (!state.lastTransfer) {
    els.transferOutput.className = "card-list empty-state";
    els.transferOutput.textContent = "Run an upload or download to see the issued transfer details.";
    return;
  }

  const transfer = state.lastTransfer;
  els.transferOutput.className = "card-list";
  els.transferOutput.innerHTML = `
    <article class="card split-card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(transfer.operation)} · ${escapeHTML(transfer.path)}</div>
          <div class="meta">${escapeHTML(transfer.transfer_id || "")}</div>
        </div>
        <span class="pill">${escapeHTML(transfer.method || transfer.result?.method || "")}</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Created</div><div>${escapeHTML(formatDate(transfer.created_at))}</div></div>
        <div class="kv-row"><div class="kv-label">Expires</div><div>${escapeHTML(formatDate(transfer.expires_at))}</div></div>
        <div class="kv-row"><div class="kv-label">Next Offset</div><div>${escapeHTML(transfer.next_offset ?? 0)}</div></div>
        <div class="kv-row"><div class="kv-label">Resume Count</div><div>${escapeHTML(transfer.resume_count ?? 0)}</div></div>
        <div class="kv-row"><div class="kv-label">Issued URL</div><div>${escapeHTML(transfer.url || "")}</div></div>
        <div class="kv-row"><div class="kv-label">Result</div><div>${escapeHTML(transfer.result || "pending")}</div></div>
      </div>
      <pre class="mono-block">${escapeHTML(JSON.stringify(transfer, null, 2))}</pre>
    </article>
  `;
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

async function startDownload() {
  const homeID = selectedHomeID();
  const remotePath = els.downloadPath.value.trim();
  if (!homeID) {
    showToast("Choose a home first.", true);
    return;
  }
  if (!remotePath) {
    showToast("Enter a remote download path.", true);
    return;
  }

  try {
    const setup = await api("/v1/home/files/downloads", {
      method: "POST",
      body: JSON.stringify({ path: remotePath }),
    });

    const response = await fetch(setup.url);
    if (!response.ok) {
      throw new Error(await response.text() || response.statusText);
    }
    const blob = await response.blob();
    const objectURL = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = objectURL;
    link.download = remotePath.split("/").pop() || "download";
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(objectURL);

    state.lastTransfer = {
      ...setup,
      result: `downloaded ${blob.size} bytes`,
      completed_at: new Date().toISOString(),
    };
    renderLastTransfer();
    showToast("Download completed.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function startUpload() {
  const homeID = selectedHomeID();
  const remotePath = els.uploadPath.value.trim();
  const file = els.uploadFile.files?.[0];
  if (!homeID) {
    showToast("Choose a home first.", true);
    return;
  }
  if (!remotePath) {
    showToast("Enter a remote upload path.", true);
    return;
  }
  if (!file) {
    showToast("Choose a file to upload.", true);
    return;
  }

  try {
    const setup = await api("/v1/home/files/uploads", {
      method: "POST",
      body: JSON.stringify({ path: remotePath }),
    });

    const response = await fetch(setup.url, {
      method: "PUT",
      body: file,
    });
    const payload = await response.json();
    if (!response.ok) {
      throw new Error(payload.error || payload.message || response.statusText);
    }

    state.lastTransfer = {
      ...setup,
      result: `uploaded ${payload.size} bytes`,
      upload_response: payload,
      completed_at: new Date().toISOString(),
    };
    renderLastTransfer();
    showToast("Upload completed.");
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
    renderLastTransfer();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.homeSelect.addEventListener("change", () => {
  syncURL(selectedHomeID());
});
els.downloadButton.addEventListener("click", startDownload);
els.uploadButton.addEventListener("click", startUpload);

hydrate();
