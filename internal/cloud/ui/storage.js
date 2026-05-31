const state = {
  user: null,
  status: null,
  auditEvents: [],
  queryTelemetry: [],
  queryTelemetryError: "",
  refreshTimer: null,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  checksumPill: document.getElementById("checksum-pill"),
  checksumOutput: document.getElementById("checksum-output"),
  backupPill: document.getElementById("backup-pill"),
  settingsSummary: document.getElementById("settings-summary"),
  backupScheduleSummary: document.getElementById("backup-schedule-summary"),
  healthScheduleSummary: document.getElementById("health-schedule-summary"),
  configForm: document.getElementById("config-form"),
  targetPath: document.getElementById("target-path"),
  fullDay: document.getElementById("full-day"),
  fullTime: document.getElementById("full-time"),
  fullCron: document.getElementById("full-cron"),
  diffDays: document.getElementById("diff-days"),
  diffTime: document.getElementById("diff-time"),
  diffCron: document.getElementById("diff-cron"),
  checksumMinutes: document.getElementById("checksum-minutes"),
  retentionFull: document.getElementById("retention-full"),
  amcheckDay: document.getElementById("amcheck-day"),
  amcheckTime: document.getElementById("amcheck-time"),
  amcheckCron: document.getElementById("amcheck-cron"),
  restoreVerificationDay: document.getElementById("restore-verification-day"),
  restoreVerificationTime: document.getElementById("restore-verification-time"),
  restoreVerificationCron: document.getElementById("restore-verification-cron"),
  advancedSchedule: document.getElementById("advanced-schedule"),
  refreshButton: document.getElementById("refresh-button"),
  backupDiffButton: document.getElementById("backup-diff-button"),
  backupFullButton: document.getElementById("backup-full-button"),
  restoreTestButton: document.getElementById("restore-test-button"),
  backupCount: document.getElementById("backup-count"),
  backupOutput: document.getElementById("backup-output"),
  taskPill: document.getElementById("task-pill"),
  taskOutput: document.getElementById("task-output"),
  restoreForm: document.getElementById("restore-form"),
  restoreLabel: document.getElementById("restore-label"),
  restoreConfirmation: document.getElementById("restore-confirmation"),
  logCount: document.getElementById("log-count"),
  logOutput: document.getElementById("log-output"),
  clearLogsButton: document.getElementById("clear-logs-button"),
  auditEventType: document.getElementById("audit-event-type"),
  auditSeverity: document.getElementById("audit-severity"),
  auditTargetType: document.getElementById("audit-target-type"),
  auditRefreshButton: document.getElementById("audit-refresh-button"),
  auditCount: document.getElementById("audit-count"),
  auditOutput: document.getElementById("audit-output"),
  queryRefreshButton: document.getElementById("query-refresh-button"),
  queryCount: document.getElementById("query-count"),
  queryOutput: document.getElementById("query-output"),
  toast: document.getElementById("toast"),
};

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  const csrf = document.cookie.split("; ").find((part) => part.startsWith("hank_remote_csrf="))?.split("=")[1];
  if (csrf && !headers.has("X-Hank-CSRF-Token")) {
    headers.set("X-Hank-CSRF-Token", decodeURIComponent(csrf));
  }
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(path, { ...options, headers });
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

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : "Never";
}

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

const dayNames = {
  "*": "Every day",
  "0": "Sunday",
  "1": "Monday",
  "2": "Tuesday",
  "3": "Wednesday",
  "4": "Thursday",
  "5": "Friday",
  "6": "Saturday",
  "7": "Sunday",
  "1-5": "Monday-Friday",
  "1-6": "Monday-Saturday",
};

function padTimePart(value) {
  return String(Number(value || 0)).padStart(2, "0");
}

function formatTimeForInput(hour, minute) {
  return `${padTimePart(hour)}:${padTimePart(minute)}`;
}

function formatTimeForDisplay(timeValue) {
  if (!timeValue) return "unknown time";
  const [hour, minute] = timeValue.split(":").map((part) => Number(part));
  if (!Number.isFinite(hour) || !Number.isFinite(minute)) return "unknown time";
  return new Date(2000, 0, 1, hour, minute).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

function parseCronSchedule(spec, fallbackSpec = "") {
  const value = String(spec || fallbackSpec || "").trim();
  const fields = value.split(/\s+/);
  if (fields.length !== 5 || fields[2] !== "*" || fields[3] !== "*") {
    return { spec: value, days: "custom", time: "", custom: true };
  }
  const minute = Number(fields[0]);
  const hour = Number(fields[1]);
  if (!Number.isInteger(minute) || !Number.isInteger(hour) || minute < 0 || minute > 59 || hour < 0 || hour > 23) {
    return { spec: value, days: "custom", time: "", custom: true };
  }
  return {
    spec: value,
    days: fields[4] === "7" ? "0" : fields[4],
    time: formatTimeForInput(hour, minute),
    custom: false,
  };
}

function selectHasValue(select, value) {
  return Array.from(select.options).some((option) => option.value === value);
}

function applyScheduleControl(select, timeInput, rawInput, spec, fallbackSpec) {
  const schedule = parseCronSchedule(spec, fallbackSpec);
  rawInput.value = schedule.spec || fallbackSpec;
  timeInput.value = schedule.time || parseCronSchedule(fallbackSpec).time;
  if (!schedule.custom && selectHasValue(select, schedule.days)) {
    select.value = schedule.days;
  } else {
    select.value = "custom";
    els.advancedSchedule.open = true;
  }
  syncRawScheduleField(select, timeInput, rawInput);
}

function syncRawScheduleField(select, timeInput, rawInput) {
  const isCustom = select.value === "custom";
  timeInput.disabled = isCustom;
  rawInput.disabled = !isCustom;
  if (isCustom) return;
  const [hour = "0", minute = "0"] = String(timeInput.value || "02:00").split(":");
  rawInput.value = `${Number(minute)} ${Number(hour)} * * ${select.value}`;
}

function scheduleFromControls(select, timeInput, rawInput, fallbackSpec) {
  if (select.value === "custom") {
    return rawInput.value.trim() || fallbackSpec;
  }
  syncRawScheduleField(select, timeInput, rawInput);
  return rawInput.value.trim();
}

function describeSchedule(select, timeInput, rawInput) {
  if (select.value === "custom") {
    return rawInput.value.trim() || "Custom";
  }
  return `${dayNames[select.value] || select.value} at ${formatTimeForDisplay(timeInput.value)}`;
}

function updateScheduleSummaries() {
  els.settingsSummary.textContent = `${describeSchedule(els.fullDay, els.fullTime, els.fullCron)} full backup. ${describeSchedule(els.diffDays, els.diffTime, els.diffCron)} diff backup.`;
  els.backupScheduleSummary.textContent = `${describeSchedule(els.fullDay, els.fullTime, els.fullCron)} / ${describeSchedule(els.diffDays, els.diffTime, els.diffCron)}`;
  els.healthScheduleSummary.textContent = `Every ${els.checksumMinutes.value || 15} minutes / ${describeSchedule(els.restoreVerificationDay, els.restoreVerificationTime, els.restoreVerificationCron)}`;
}

function bindScheduleControl(select, timeInput, rawInput) {
  const sync = () => {
    syncRawScheduleField(select, timeInput, rawInput);
    if (select.value === "custom") {
      els.advancedSchedule.open = true;
    }
    updateScheduleSummaries();
  };
  select.addEventListener("change", sync);
  timeInput.addEventListener("input", sync);
  rawInput.addEventListener("input", updateScheduleSummaries);
}

function minutesFromSeconds(seconds) {
  const value = Number(seconds || 900);
  return Math.max(1, Math.round(value / 60));
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

function renderKV(rows) {
  return rows.map(([label, value]) => `
    <div class="kv-row">
      <div class="kv-label">${escapeHTML(label)}</div>
      <div>${escapeHTML(value)}</div>
    </div>
  `).join("");
}

function renderStatus() {
  const status = state.status || {};
  const checksum = status.checksum || {};
  const backup = status.backup || {};
  const config = status.config || {};
  const schedule = config.schedule || {};
  const target = config.target || {};

  els.checksumPill.textContent = checksum.corruption_detected ? "Corruption" : checksum.enabled ? "Enabled" : "Needs Setup";
  els.checksumPill.className = checksum.corruption_detected ? "status-chip offline" : "pill";
  els.checksumOutput.innerHTML = renderKV([
    ["Checksums", checksum.enabled ? "Enabled" : "Not enabled"],
    ["Last Check", formatDate(checksum.last_check_at)],
    ["Last Deep Check", formatDate(checksum.last_amcheck_at)],
    ["Failures", checksum.failure_count || 0],
    ["Last Error", checksum.last_error || "None"],
  ]);

  els.backupPill.textContent = target.type || "posix";
  els.targetPath.value = target.path || "/var/lib/pgbackrest";
  applyScheduleControl(els.fullDay, els.fullTime, els.fullCron, schedule.full_backup_cron, "0 2 * * 0");
  applyScheduleControl(els.diffDays, els.diffTime, els.diffCron, schedule.differential_backup_cron, "0 2 * * 1-6");
  els.checksumMinutes.value = minutesFromSeconds(schedule.checksum_interval_seconds || 900);
  els.retentionFull.value = schedule.retention_full || 2;
  applyScheduleControl(els.amcheckDay, els.amcheckTime, els.amcheckCron, schedule.amcheck_cron, "30 3 * * 0");
  applyScheduleControl(els.restoreVerificationDay, els.restoreVerificationTime, els.restoreVerificationCron, schedule.restore_verification_cron, "0 4 * * 0");
  updateScheduleSummaries();
  els.restoreConfirmation.placeholder = config.restore?.confirmation_phrase || "RESTORE HANK DATABASE";

  renderBackups(backup.backups || []);
  renderTasks(status.tasks || []);
  renderEvents(status.events || [], els.logOutput);
  els.logCount.textContent = `${(status.events || []).length} log${(status.events || []).length === 1 ? "" : "s"}`;
}

function renderBackups(backups) {
  els.backupCount.textContent = `${backups.length} backup${backups.length === 1 ? "" : "s"}`;
  els.restoreLabel.innerHTML = "";
  if (!backups.length) {
    els.backupOutput.className = "card-list empty-state";
    els.backupOutput.textContent = "No backups reported yet.";
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No backup yet";
    els.restoreLabel.appendChild(option);
    return;
  }
  els.backupOutput.className = "card-list";
  els.backupOutput.innerHTML = backups.map((backup) => `
    <article class="card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(backup.label)}</div>
          <div class="meta">${escapeHTML(backup.type || "backup")} finished ${escapeHTML(formatDate(backup.stopped_at))}</div>
        </div>
        <span class="pill">${escapeHTML(formatBytes(backup.size_bytes))}</span>
      </div>
    </article>
  `).join("");
  backups.slice().reverse().forEach((backup) => {
    const option = document.createElement("option");
    option.value = backup.label;
    option.textContent = backup.label;
    els.restoreLabel.appendChild(option);
  });
}

function renderTasks(tasks) {
  const activeTasks = tasks.filter((task) => isActiveTask(task));
  setTaskControlsBusy(activeTasks.length > 0);
  if (!tasks.length) {
    els.taskPill.textContent = "Idle";
    els.taskPill.className = "pill";
    els.taskOutput.className = "card-list empty-state";
    els.taskOutput.textContent = "No backup or restore task running.";
    return;
  }
  els.taskPill.textContent = activeTasks.length ? "Running" : "Finished";
  els.taskPill.className = activeTasks.length ? "status-chip" : "pill";
  els.taskOutput.className = "card-list";
  els.taskOutput.innerHTML = tasks.map((task) => `
    <article class="card task-card ${isActiveTask(task) ? "active" : ""}">
      <div class="card-head">
        <div>
          <div class="card-title">${isActiveTask(task) ? `<span class="loading-dot" aria-hidden="true"></span>` : ""}${escapeHTML(task.message || taskLabel(task))}</div>
          <div class="meta">${escapeHTML(task.step || task.operation || "storage")} · updated ${escapeHTML(formatDate(task.updated_at))}</div>
        </div>
        <span class="status-chip ${task.status === "failed" ? "offline" : ""}">${escapeHTML(task.status || "running")}</span>
      </div>
      ${task.backup_label ? `<div class="meta">Backup: ${escapeHTML(task.backup_label)}</div>` : ""}
    </article>
  `).join("");
}

function taskLabel(task) {
  if (task.operation === "backup") {
    return `${task.backup_type === "full" ? "Full" : "Diff"} backup ${task.status || "running"}`;
  }
  if (task.operation === "restore_test") return `Restore test ${task.status || "running"}`;
  if (task.operation === "primary_restore") return `Primary restore ${task.status || "running"}`;
  return `Storage task ${task.status || "running"}`;
}

function isActiveTask(task) {
  return task?.status === "queued" || task?.status === "running";
}

function setTaskControlsBusy(isBusy) {
  els.backupDiffButton.disabled = isBusy;
  els.backupFullButton.disabled = isBusy;
  els.restoreTestButton.disabled = isBusy;
  const primaryRestoreButton = els.restoreForm.querySelector("button[type='submit']");
  if (primaryRestoreButton) primaryRestoreButton.disabled = isBusy;
}

function renderEvents(events, target) {
  if (!events.length) {
    target.className = "card-list empty-state";
    target.textContent = "No storage logs reported.";
    return;
  }
  target.className = "card-list";
  target.innerHTML = events.map((event) => `
    <article class="card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(event.message || event.operation)}</div>
          <div class="meta">${escapeHTML(event.operation)} · ${escapeHTML(formatDate(event.time))}</div>
        </div>
        <span class="status-chip ${event.severity === "error" || event.severity === "critical" ? "offline" : ""}">${escapeHTML(event.severity || "info")}</span>
      </div>
      ${event.backup_label ? `<div class="meta">Backup: ${escapeHTML(event.backup_label)}</div>` : ""}
      ${renderEventDetails(event)}
    </article>
  `).join("");
}

function renderEventDetails(event) {
  const details = event.details || {};
  const rows = [];
  if (details.hint) rows.push(["Next Check", details.hint]);
  if (details.error) rows.push(["Command", details.error]);
  if (details.output_excerpt) rows.push(["Output", details.output_excerpt]);
  if (!rows.length) return "";
  return `
    <div class="kv-grid">
      ${rows.map(([label, value]) => `
        <div class="kv-row">
          <div class="kv-label">${escapeHTML(label)}</div>
          <code class="mono-block">${escapeHTML(value)}</code>
        </div>
      `).join("")}
    </div>
  `;
}

function renderAuditEvents() {
  const events = state.auditEvents || [];
  els.auditCount.textContent = `${events.length} event${events.length === 1 ? "" : "s"}`;
  if (!events.length) {
    els.auditOutput.className = "card-list empty-state";
    els.auditOutput.textContent = "No audit events reported.";
    return;
  }
  els.auditOutput.className = "card-list";
  els.auditOutput.innerHTML = events.map((event) => `
    <article class="card">
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(event.event_type || "audit event")}</div>
          <div class="meta">${escapeHTML(event.target_type || "target")} · ${escapeHTML(formatDate(event.occurred_at))}</div>
        </div>
        <span class="status-chip ${event.severity === "critical" || event.severity === "warning" ? "offline" : ""}">${escapeHTML(event.severity || "info")}</span>
      </div>
      <div class="kv-grid">
        <div class="kv-row"><div class="kv-label">Actor</div><div>${escapeHTML(event.actor_user_id || event.actor_agent_id || "system")}</div></div>
        <div class="kv-row"><div class="kv-label">Target ID</div><div>${escapeHTML(event.target_id || "")}</div></div>
        ${renderAuditMetadata(event.metadata || {})}
      </div>
    </article>
  `).join("");
}

function renderAuditMetadata(metadata) {
  const entries = Object.entries(metadata || {});
  if (!entries.length) return "";
  return entries.map(([key, value]) => `
    <div class="kv-row">
      <div class="kv-label">${escapeHTML(key)}</div>
      <code class="mono-block">${escapeHTML(typeof value === "string" ? value : JSON.stringify(value))}</code>
    </div>
  `).join("");
}

function queryValue(row, snakeKey, goKey) {
  return row?.[snakeKey] ?? row?.[goKey] ?? "";
}

function renderQueryTelemetry() {
  const queries = state.queryTelemetry || [];
  els.queryCount.textContent = state.queryTelemetryError ? "Unavailable" : `${queries.length} quer${queries.length === 1 ? "y" : "ies"}`;
  if (state.queryTelemetryError) {
    els.queryOutput.className = "card-list empty-state";
    els.queryOutput.textContent = state.queryTelemetryError;
    return;
  }
  if (!queries.length) {
    els.queryOutput.className = "card-list empty-state";
    els.queryOutput.textContent = "No query telemetry reported.";
    return;
  }
  els.queryOutput.className = "card-list";
  els.queryOutput.innerHTML = queries.map((row) => {
    const totalMS = Number(queryValue(row, "total_exec_ms", "TotalExecMS") || 0);
    const meanMS = Number(queryValue(row, "mean_exec_ms", "MeanExecMS") || 0);
    return `
      <article class="card">
        <div class="card-head">
          <div>
            <div class="card-title">${escapeHTML(Number(queryValue(row, "calls", "Calls") || 0).toLocaleString())} calls</div>
            <div class="meta">${escapeHTML(totalMS.toFixed(1))} ms total · ${escapeHTML(meanMS.toFixed(2))} ms mean</div>
          </div>
          <span class="pill">${escapeHTML(Number(queryValue(row, "rows", "Rows") || 0).toLocaleString())} rows</span>
        </div>
        <code class="mono-block">${escapeHTML(queryValue(row, "query", "Query"))}</code>
      </article>
    `;
  }).join("");
}

async function loadStatus() {
  state.status = await api("/v1/home/storage/status");
  renderStatus();
  scheduleStatusRefresh();
}

function scheduleStatusRefresh() {
  window.clearTimeout(state.refreshTimer);
  const tasks = state.status?.tasks || [];
  if (!tasks.some((task) => isActiveTask(task))) {
    state.refreshTimer = null;
    return;
  }
  state.refreshTimer = window.setTimeout(() => {
    loadStatus().catch((error) => showToast(error.message, true));
  }, 3000);
}

async function loadAuditEvents() {
  const params = new URLSearchParams({ limit: "25" });
  if (els.auditEventType.value) params.set("event_type", els.auditEventType.value);
  if (els.auditSeverity.value) params.set("severity", els.auditSeverity.value);
  if (els.auditTargetType.value) params.set("target_type", els.auditTargetType.value);
  const payload = await api(`/v1/home/audit-events?${params.toString()}`);
  state.auditEvents = payload.events || [];
  renderAuditEvents();
}

async function loadQueryTelemetry() {
  state.queryTelemetryError = "";
  try {
    const payload = await api("/v1/home/query-telemetry?limit=20");
    state.queryTelemetry = payload.queries || [];
  } catch (error) {
    state.queryTelemetry = [];
    state.queryTelemetryError = error.message || "Query telemetry is unavailable.";
  }
  renderQueryTelemetry();
}

async function saveConfig(event) {
  event.preventDefault();
  const current = state.status?.config || {};
  const payload = {
    ...current,
    target: {
      type: "posix",
      path: els.targetPath.value.trim(),
    },
    schedule: {
      ...(current.schedule || {}),
      full_backup_cron: scheduleFromControls(els.fullDay, els.fullTime, els.fullCron, "0 2 * * 0"),
      differential_backup_cron: scheduleFromControls(els.diffDays, els.diffTime, els.diffCron, "0 2 * * 1-6"),
      checksum_interval_seconds: Number(els.checksumMinutes.value || 15) * 60,
      retention_full: Number(els.retentionFull.value || 2),
      amcheck_cron: scheduleFromControls(els.amcheckDay, els.amcheckTime, els.amcheckCron, "30 3 * * 0"),
      restore_verification_cron: scheduleFromControls(els.restoreVerificationDay, els.restoreVerificationTime, els.restoreVerificationCron, "0 4 * * 0"),
      restore_verification_enabled: true,
    },
  };
  try {
    await api("/v1/home/storage/config", { method: "PUT", body: JSON.stringify(payload) });
    await loadStatus();
    showToast("Backup settings saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function requestBackup(type) {
  try {
    await api("/v1/home/storage/backup", { method: "POST", body: JSON.stringify({ backup_type: type }) });
    await loadStatus();
    showToast(`${type === "full" ? "Full" : "Diff"} backup requested.`);
  } catch (error) {
    showToast(error.message, true);
  }
}

async function requestRestoreTest() {
  const backupLabel = els.restoreLabel.value;
  try {
    await api("/v1/home/storage/restore-test", { method: "POST", body: JSON.stringify({ backup_label: backupLabel }) });
    await loadStatus();
    showToast("Restore test requested.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function requestPrimaryRestore(event) {
  event.preventDefault();
  try {
    const tokenPayload = await api("/v1/home/storage/restore-primary", {
      method: "POST",
      body: JSON.stringify({ request_action_token: true }),
    });
    await api("/v1/home/storage/restore-primary", {
      method: "POST",
      body: JSON.stringify({
        backup_label: els.restoreLabel.value,
        confirmation: els.restoreConfirmation.value,
        admin_action_token: tokenPayload.admin_action_token,
      }),
    });
    els.restoreConfirmation.value = "";
    await loadStatus();
    showToast("Primary restore requested.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function clearLogs() {
  if (!window.confirm("Clear storage logs? Backups and settings will not be changed.")) {
    return;
  }
  try {
    await api("/v1/home/storage/events", { method: "DELETE" });
    await loadStatus();
    showToast("Storage logs cleared.");
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
    await Promise.all([loadStatus(), loadAuditEvents(), loadQueryTelemetry()]);
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.refreshButton.addEventListener("click", () => loadStatus().then(() => showToast("Storage status refreshed.")).catch((error) => showToast(error.message, true)));
els.configForm.addEventListener("submit", saveConfig);
bindScheduleControl(els.fullDay, els.fullTime, els.fullCron);
bindScheduleControl(els.diffDays, els.diffTime, els.diffCron);
bindScheduleControl(els.amcheckDay, els.amcheckTime, els.amcheckCron);
bindScheduleControl(els.restoreVerificationDay, els.restoreVerificationTime, els.restoreVerificationCron);
els.checksumMinutes.addEventListener("input", updateScheduleSummaries);
els.backupDiffButton.addEventListener("click", () => requestBackup("diff"));
els.backupFullButton.addEventListener("click", () => requestBackup("full"));
els.restoreTestButton.addEventListener("click", requestRestoreTest);
els.restoreForm.addEventListener("submit", requestPrimaryRestore);
els.clearLogsButton.addEventListener("click", clearLogs);
els.auditRefreshButton.addEventListener("click", () => loadAuditEvents().then(() => showToast("Audit filters applied.")).catch((error) => showToast(error.message, true)));
els.queryRefreshButton.addEventListener("click", () => loadQueryTelemetry().then(() => showToast("Query telemetry refreshed.")).catch((error) => showToast(error.message, true)));

hydrate();
