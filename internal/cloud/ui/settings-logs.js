const api = window.HankAPI.request;
const escapeHTML = window.HankUI.escapeHTML;

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  form: document.getElementById("log-filter-form"),
  eventType: document.getElementById("log-event-type"),
  severity: document.getElementById("log-severity"),
  targetType: document.getElementById("log-target-type"),
  sort: document.getElementById("log-sort"),
  order: document.getElementById("log-order"),
  limit: document.getElementById("log-limit"),
  count: document.getElementById("log-count"),
  output: document.getElementById("log-output"),
  toast: document.getElementById("toast"),
};

const state = {
  user: null,
  events: [],
};

function showToast(message, isError = false) {
  window.HankUI.showToast(els.toast, message, isError);
}

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : "Never";
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function renderMetadata(metadata) {
  const entries = Object.entries(metadata || {});
  if (!entries.length) return "";
  return `<dl class="kv-grid compact">${entries.map(([key, value]) => `
    <div>
      <dt>${escapeHTML(key)}</dt>
      <dd>${escapeHTML(typeof value === "object" ? JSON.stringify(value) : String(value))}</dd>
    </div>
  `).join("")}</dl>`;
}

function renderLogs() {
  const events = state.events || [];
  els.count.textContent = `${events.length} event${events.length === 1 ? "" : "s"}`;
  if (!events.length) {
    els.output.className = "card-list empty-state";
    els.output.textContent = "No logs match these filters.";
    return;
  }
  els.output.className = "card-list";
  els.output.innerHTML = events.map((event) => `
    <article class="card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(event.event_type || "audit event")}</div>
          <div class="meta">${escapeHTML(formatDate(event.occurred_at))} · ${escapeHTML(event.target_type || "system")}</div>
          <div class="quick-link-status-text">${escapeHTML(event.helper_text || "Review metadata for context.")}</div>
        </div>
        <span class="status-chip ${event.severity === "critical" ? "revoked" : event.severity === "warning" ? "offline" : ""}">${escapeHTML(event.severity || "info")}</span>
      </div>
      <div class="token-meta">Actor: ${escapeHTML(event.actor_user_id || event.actor_agent_id || "system")}</div>
      <div class="token-meta">Target: ${escapeHTML(event.target_id || "none")}</div>
      <div class="token-meta">Request: ${escapeHTML(event.request_id || "none")}</div>
      ${renderMetadata(event.metadata || {})}
    </article>
  `).join("");
}

async function loadLogs() {
  const params = new URLSearchParams();
  if (els.eventType.value.trim()) params.set("event_type", els.eventType.value.trim());
  if (els.severity.value) params.set("severity", els.severity.value);
  if (els.targetType.value.trim()) params.set("target_type", els.targetType.value.trim());
  params.set("sort", els.sort.value || "occurred_at");
  params.set("order", els.order.value || "desc");
  params.set("limit", els.limit.value || "100");
  const payload = await api(`/v1/home/audit-events?${params.toString()}`);
  state.events = payload.events || [];
  renderLogs();
}

async function start() {
  try {
    const me = await api("/v1/me");
    state.user = me.user;
    renderSession();
    await loadLogs();
  } catch (error) {
    showToast(error.message, true);
  }
}

els.form.addEventListener("submit", (event) => {
  event.preventDefault();
  loadLogs().catch((error) => showToast(error.message, true));
});

els.logoutButton.addEventListener("click", async () => {
  try {
    await api("/v1/auth/logout", { method: "POST" });
    window.location.href = "/";
  } catch (error) {
    showToast(error.message, true);
  }
});

start();
