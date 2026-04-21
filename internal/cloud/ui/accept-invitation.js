const state = {
  user: null,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  token: document.getElementById("token"),
  acceptButton: document.getElementById("accept-button"),
  result: document.getElementById("result"),
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
  els.sessionMeta.textContent = `User ID ${state.user?.id || ""}`;
}

function renderResult(payload) {
  if (!payload?.home) {
    els.result.className = "card-list empty-state";
    els.result.textContent = "Enter a token to accept a pending home invitation.";
    return;
  }

  els.result.className = "card-list";
  els.result.innerHTML = `
    <article class="card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(payload.home.name)}</div>
          <div class="meta">${escapeHTML(payload.home.id)}</div>
        </div>
        <span class="pill">Joined</span>
      </div>
      <div class="meta">Created ${escapeHTML(formatDate(payload.home.created_at))}</div>
      <div class="meta">Use the dashboard tabs to manage this home now.</div>
    </article>
  `;
}

async function acceptInvitation() {
  const token = els.token.value.trim();
  if (!token) {
    showToast("Enter an invitation token.", true);
    return;
  }
  try {
    const payload = await api("/v1/home/invitations/accept", {
      method: "POST",
      body: JSON.stringify({ token }),
    });
    renderResult(payload);
    showToast("Invitation accepted.");
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
    const token = new URLSearchParams(window.location.search).get("token");
    if (token) {
      els.token.value = token;
    }
    renderResult(null);
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.acceptButton.addEventListener("click", acceptInvitation);
els.token.addEventListener("keydown", async (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    await acceptInvitation();
  }
});

hydrate();
