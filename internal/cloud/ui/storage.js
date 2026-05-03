const state = {
  user: null,
  status: null,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  checksumPill: document.getElementById("checksum-pill"),
  checksumOutput: document.getElementById("checksum-output"),
  backupPill: document.getElementById("backup-pill"),
  configForm: document.getElementById("config-form"),
  targetPath: document.getElementById("target-path"),
  fullCron: document.getElementById("full-cron"),
  diffCron: document.getElementById("diff-cron"),
  checksumSeconds: document.getElementById("checksum-seconds"),
  retentionFull: document.getElementById("retention-full"),
  amcheckCron: document.getElementById("amcheck-cron"),
  restoreVerificationCron: document.getElementById("restore-verification-cron"),
  refreshButton: document.getElementById("refresh-button"),
  backupDiffButton: document.getElementById("backup-diff-button"),
  backupFullButton: document.getElementById("backup-full-button"),
  restoreTestButton: document.getElementById("restore-test-button"),
  backupCount: document.getElementById("backup-count"),
  backupOutput: document.getElementById("backup-output"),
  restoreForm: document.getElementById("restore-form"),
  restoreLabel: document.getElementById("restore-label"),
  restoreConfirmation: document.getElementById("restore-confirmation"),
  failureCount: document.getElementById("failure-count"),
  failureOutput: document.getElementById("failure-output"),
  eventsOutput: document.getElementById("events-output"),
  toast: document.getElementById("toast"),
};

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
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
  const restore = status.restore || {};
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
  els.fullCron.value = schedule.full_backup_cron || "0 2 * * 0";
  els.diffCron.value = schedule.differential_backup_cron || "0 2 * * 1-6";
  els.checksumSeconds.value = schedule.checksum_interval_seconds || 900;
  els.retentionFull.value = schedule.retention_full || 2;
  els.amcheckCron.value = schedule.amcheck_cron || "30 3 * * 0";
  els.restoreVerificationCron.value = schedule.restore_verification_cron || "0 4 * * 0";
  els.restoreConfirmation.placeholder = config.restore?.confirmation_phrase || "RESTORE HANK DATABASE";

  renderBackups(backup.backups || [], restore);
  renderEvents(status.failures || [], els.failureOutput, true);
  renderEvents(status.events || [], els.eventsOutput, false);
  els.failureCount.textContent = `${(status.failures || []).length} failure${(status.failures || []).length === 1 ? "" : "s"}`;
}

function renderBackups(backups, restore) {
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
  if (restore?.pending_intents?.length) {
    showToast(`${restore.pending_intents.length} storage task pending.`);
  }
}

function renderEvents(events, target, failuresOnly) {
  if (!events.length) {
    target.className = "card-list empty-state";
    target.textContent = failuresOnly ? "No backup or checksum failures reported." : "No storage events reported.";
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

async function loadStatus() {
  state.status = await api("/v1/home/storage/status");
  renderStatus();
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
      full_backup_cron: els.fullCron.value.trim(),
      differential_backup_cron: els.diffCron.value.trim(),
      checksum_interval_seconds: Number(els.checksumSeconds.value || 900),
      retention_full: Number(els.retentionFull.value || 2),
      amcheck_cron: els.amcheckCron.value.trim(),
      restore_verification_cron: els.restoreVerificationCron.value.trim(),
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
    await api("/v1/home/storage/restore-primary", {
      method: "POST",
      body: JSON.stringify({
        backup_label: els.restoreLabel.value,
        confirmation: els.restoreConfirmation.value,
      }),
    });
    els.restoreConfirmation.value = "";
    await loadStatus();
    showToast("Primary restore requested.");
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
    await loadStatus();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.refreshButton.addEventListener("click", () => loadStatus().then(() => showToast("Storage status refreshed.")).catch((error) => showToast(error.message, true)));
els.configForm.addEventListener("submit", saveConfig);
els.backupDiffButton.addEventListener("click", () => requestBackup("diff"));
els.backupFullButton.addEventListener("click", () => requestBackup("full"));
els.restoreTestButton.addEventListener("click", requestRestoreTest);
els.restoreForm.addEventListener("submit", requestPrimaryRestore);

hydrate();
