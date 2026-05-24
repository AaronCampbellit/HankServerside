const state = {
  user: null,
  status: null,
  assistant: null,
  settings: null,
  media: null,
  statusTimer: null,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  openAILinkPill: document.getElementById("openai-link-pill"),
  openAIConfigPill: document.getElementById("openai-config-pill"),
  openAIAccountOutput: document.getElementById("openai-account-output"),
  openAIConfigOutput: document.getElementById("openai-config-output"),
  linkOpenAIButton: document.getElementById("link-openai-button"),
  refreshButton: document.getElementById("refresh-button"),
  assistantSettingsPill: document.getElementById("assistant-settings-pill"),
  assistantSettingsForm: document.getElementById("assistant-settings-form"),
  settingsTabButtons: Array.from(document.querySelectorAll("[data-settings-tab]")),
  settingsPanels: Array.from(document.querySelectorAll("[data-settings-panel]")),
  assistantHarnessOutput: document.getElementById("assistant-harness-output"),
  assistantToolsOutput: document.getElementById("assistant-tools-output"),
  mediaWorkflowPill: document.getElementById("media-workflow-pill"),
  mediaWorkflowEnabled: document.getElementById("media-workflow-enabled"),
  mediaGramatonBaseURL: document.getElementById("media-gramaton-base-url"),
  mediaGramatonUsername: document.getElementById("media-gramaton-username"),
  mediaGramatonPassword: document.getElementById("media-gramaton-password"),
  mediaDestinationPath: document.getElementById("media-destination-path"),
  mediaWorkflowMeta: document.getElementById("media-workflow-meta"),
  saveMediaSettingsButton: document.getElementById("save-media-settings-button"),
  refreshMediaSettingsButton: document.getElementById("refresh-media-settings-button"),
  mediaJobsOutput: document.getElementById("media-jobs-output"),
  harnessProfileNotesEnabled: document.getElementById("harness-profile-notes-enabled"),
  harnessHomeNotesEnabled: document.getElementById("harness-home-notes-enabled"),
  harnessFilesEnabled: document.getElementById("harness-files-enabled"),
  harnessCalendarEnabled: document.getElementById("harness-calendar-enabled"),
  harnessHomeAssistantEnabled: document.getElementById("harness-homeassistant-enabled"),
  harnessProjectDocsEnabled: document.getElementById("harness-project-docs-enabled"),
  harnessConversationsEnabled: document.getElementById("harness-conversations-enabled"),
  harnessSystemPrompt: document.getElementById("harness-system-prompt"),
  resetAssistantPromptButton: document.getElementById("reset-assistant-prompt-button"),
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
  return String(value == null ? "" : value)
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
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
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
  const assistant = state.assistant || {};
  const missing = status.missing || [];
  const isDeviceCode = status.auth_mode === "device_code";
  const isChatGPT = status.auth_provider === "chatgpt_codex" || isDeviceCode;
  const pending = status.pending || null;
  const pendingState = pending?.state || "";
  const accountLabel = isChatGPT ? "ChatGPT/Codex" : "OpenAI";

  els.openAILinkPill.textContent = status.linked ? "Linked" : pendingState === "pending" ? "Pending" : "Not Linked";
  els.openAILinkPill.className = status.linked ? "status-chip" : "status-chip offline";

  const accountRows = [
    ["Account", status.linked ? `${accountLabel} is linked.` : `No ${accountLabel} account is linked yet.`],
    ["Link Type", isDeviceCode ? "Device code" : "Browser redirect"],
    ["Plan", status.chatgpt_plan_type || "Not reported"],
    ["Last Linked", formatDate(status.updated_at)],
    ["Expires", formatDate(status.expires_at)],
    ["Scope", status.scope || status.scopes || "Not reported"],
  ];
  if (pendingState) {
    accountRows.push(["Pending State", pendingState]);
  }
  if (pending?.user_code) {
    accountRows.push(["Device Code", pending.user_code]);
  }
  if (pending?.verification_url) {
    accountRows.push(["Open", pending.verification_url]);
  }
  els.openAIAccountOutput.innerHTML = renderKV(accountRows);

  els.openAIConfigPill.textContent = status.configured ? "Ready" : "Needs Setup";
  els.openAIConfigPill.className = status.configured ? "status-chip" : "status-chip offline";
  els.linkOpenAIButton.disabled = !status.configured;
  els.linkOpenAIButton.textContent = status.linked ? `Relink ${accountLabel}` : `Link ${accountLabel}`;

  if (status.configured) {
    els.openAIConfigOutput.className = "card-list";
    els.openAIConfigOutput.innerHTML = `
      <article class="card">
        <div class="card-title">${escapeHTML(accountLabel)} linking is configured.</div>
        <div class="meta">${isDeviceCode ? "Device-code authorization is enabled." : `Redirect URL: ${escapeHTML(status.redirect_uri || "Not shown")}`}</div>
        <div class="meta">${isDeviceCode ? "Experimental ChatGPT/Codex provider." : `Scopes: ${escapeHTML(status.scopes || "openid profile email")}`}</div>
      </article>
      ${pendingState === "pending" ? `
      <article class="card">
        <div class="card-title">Finish ChatGPT/Codex link</div>
        <div class="meta">Code: ${escapeHTML(pending.user_code || "Not shown")}</div>
        <div class="meta">Open: ${escapeHTML(pending.verification_url || "Not shown")}</div>
        <div class="meta">Expires: ${escapeHTML(formatDate(pending.expires_at))}</div>
      </article>
      ` : ""}
      <article class="card">
        <div class="card-title">HankAI provider: ${escapeHTML(assistant.provider || "local")}</div>
        <div class="meta">Chat model: ${escapeHTML(assistant.chat_model || "local fallback")}</div>
        <div class="meta">Embedding model: ${escapeHTML(assistant.embedding_model || "local fallback")}</div>
        <div class="meta">Vector store: ${escapeHTML(assistant.vector_store || "postgres")}</div>
        <div class="meta">Vector mode: ${escapeHTML(assistant.index?.vector_mode || "json_fallback")}</div>
      </article>
    `;
    return;
  }

  els.openAIConfigOutput.className = "card-list";
  els.openAIConfigOutput.innerHTML = `
    <article class="card">
      <div class="card-title">HankAI provider: ${escapeHTML(assistant.provider || "local")}</div>
      <div class="meta">Chat: ${assistant.chat_configured ? "Configured" : "Using local fallback until Ollama or OpenAI is configured."}</div>
      <div class="meta">Embeddings: ${assistant.embedding_configured ? "Configured" : "Using local fallback embeddings."}</div>
      <div class="meta">Vector store: ${escapeHTML(assistant.vector_store || "postgres")}</div>
      <div class="meta">Vector mode: ${escapeHTML(assistant.index?.vector_mode || "json_fallback")}</div>
    </article>
    <article class="card">
      <div class="card-title">Add these to <code>.env.cloud</code>, then restart the cloud service.</div>
      <div class="meta">${missing.length ? missing.map(escapeHTML).join(", ") : `${escapeHTML(accountLabel)} settings are missing.`}</div>
    </article>
  `;
}

function renderAssistantSettings() {
  const payload = state.settings || {};
  const settings = payload.settings || {};
  const defaults = payload.defaults || {};
  const sources = payload.sources || [];
  const tools = payload.tools || [];
  const enabledSources = sources.filter((source) => source.enabled);

  els.assistantSettingsPill.textContent = enabledSources.length ? `${enabledSources.length} sources` : "No sources";
  els.assistantSettingsPill.className = enabledSources.length ? "status-chip" : "status-chip offline";

  els.harnessProfileNotesEnabled.checked = settings.profile_notes_enabled !== false;
  els.harnessHomeNotesEnabled.checked = settings.home_notes_enabled !== false;
  els.harnessFilesEnabled.checked = settings.files_enabled !== false;
  els.harnessCalendarEnabled.checked = settings.calendar_enabled !== false;
  els.harnessHomeAssistantEnabled.checked = settings.homeassistant_enabled !== false;
  els.harnessProjectDocsEnabled.checked = settings.project_docs_enabled !== false;
  els.harnessConversationsEnabled.checked = settings.conversations_enabled !== false;
  els.harnessSystemPrompt.value = settings.system_prompt || defaults.system_prompt || "";
  renderToolSettings(tools);
  renderMediaWorkflowSettings();
  const index = state.assistant?.index || {};

  els.assistantHarnessOutput.className = "card-list";
  els.assistantHarnessOutput.innerHTML = `
    <article class="card">
      <div class="card-title">Current provider: ${escapeHTML(state.assistant?.provider || "local")}</div>
      <div class="meta">Chat model: ${escapeHTML(state.assistant?.chat_model || "local fallback")}</div>
      <div class="meta">Embeddings: ${escapeHTML(state.assistant?.embedding_model || "local-hash")}</div>
      <div class="meta">Vector mode: ${escapeHTML(index.vector_mode || "json_fallback")}</div>
      <div class="meta">Context sent per request: ${escapeHTML(settings.max_context_items || defaults.max_context_items || 20)} items</div>
    </article>
    <article class="card">
      <div class="card-title">Indexed memory</div>
      <div class="meta">Chunks: ${escapeHTML(index.chunk_count || 0)} (${escapeHTML(index.embedded_chunk_count || 0)} embedded)</div>
      <div class="meta">Files: ${escapeHTML(index.file_count || 0)} (${escapeHTML(index.embedded_file_count || 0)} embedded)</div>
      <div class="meta">Past conversations: ${escapeHTML(index.conversation_count || 0)}</div>
    </article>
    <article class="card">
      <div class="card-title">Allowed sources</div>
      <div class="meta">${enabledSources.length ? enabledSources.map((source) => escapeHTML(source.label)).join(", ") : "None"}</div>
      <div class="meta">Changes apply to the next HankAI message. Tokens stay on Hank Remote.</div>
    </article>
    <article class="card">
      <div class="card-title">External model boundary</div>
      <div class="meta">When ChatGPT/Codex is the provider, enabled Hank context and the prompt are sent to the linked Codex backend for chat responses.</div>
    </article>
  `;
}

function renderToolSettings(tools) {
  const fallbackTools = [
    {
      label: "Files",
      enabled: els.harnessFilesEnabled.checked,
      status: els.harnessFilesEnabled.checked ? "Ready" : "Off",
      description: "Search file names and route approved file work through the home agent.",
    },
    {
      label: "Media Downloads",
      enabled: false,
      status: els.harnessFilesEnabled.checked ? "Agent setup needed" : "Files off",
      description: "Search authorized media sources, prepare a confirmed download plan, and save approved files to the configured Media destination.",
      requirements: ["Files enabled", "Media source enabled on the home agent", "Agent file backend pointed at the Media share"],
    },
  ];
  const cards = (tools.length ? tools : fallbackTools).map((tool) => {
    const statusClass = tool.enabled ? "status-chip" : "status-chip offline";
    const requirements = Array.isArray(tool.requirements) && tool.requirements.length
      ? `<div class="meta">${tool.requirements.map(escapeHTML).join(" &middot; ")}</div>`
      : "";
    return `
      <article class="tool-setting-card">
        <div class="tool-setting-head">
          <div class="card-title">${escapeHTML(tool.label)}</div>
          <span class="${statusClass}">${escapeHTML(tool.status || (tool.enabled ? "Ready" : "Off"))}</span>
        </div>
        <div class="meta">${escapeHTML(tool.description || "")}</div>
        ${requirements}
      </article>
    `;
  }).join("");
  els.assistantToolsOutput.innerHTML = cards || `
    <article class="tool-setting-card">
      <div class="tool-setting-head">
        <div class="card-title">No tools configured</div>
        <span class="status-chip offline">Off</span>
      </div>
      <div class="meta">Enable Hank sources before using agent-backed workflows.</div>
    </article>
  `;
}

function renderMediaWorkflowSettings() {
  const payload = state.media || {};
  const settings = payload.settings || {};
  const online = payload.online === true;
  const canEdit = payload.can_edit === true;
  const hasPassword = settings.has_password === true;
  const enabled = settings.enabled === true;
  const fieldsDisabled = !canEdit;

  els.mediaWorkflowPill.textContent = !canEdit ? "Admin Only" : online ? (enabled ? "Enabled" : "Configured Off") : "Agent Offline";
  els.mediaWorkflowPill.className = online && enabled ? "status-chip" : "status-chip offline";
  els.mediaWorkflowEnabled.checked = enabled;
  els.mediaGramatonBaseURL.value = settings.base_url || "https://gramaton.io";
  els.mediaGramatonUsername.value = settings.username || "";
  els.mediaDestinationPath.value = settings.destination_path || "";
  els.mediaGramatonPassword.placeholder = hasPassword ? "Leave unchanged" : "Required to enable";
  if (!canEdit) {
    els.mediaWorkflowMeta.textContent = "Only Home admins can change media workflow settings.";
  } else if (!online) {
    els.mediaWorkflowMeta.textContent = `${payload.error || "The home agent is offline."} You can prepare these fields, but saving requires the updated home agent to be online.`;
  } else {
    els.mediaWorkflowMeta.textContent = `Destination resolves under ${settings.destination_path ? `Media root/${settings.destination_path}` : "Media root"}. 1080p is preferred and 720p is used only as fallback.`;
  }

  [
    els.mediaWorkflowEnabled,
    els.mediaGramatonBaseURL,
    els.mediaGramatonUsername,
    els.mediaGramatonPassword,
    els.mediaDestinationPath,
    els.saveMediaSettingsButton,
  ].forEach((element) => {
    element.disabled = fieldsDisabled;
  });
  els.refreshMediaSettingsButton.disabled = !online;
  renderMediaJobs(payload.jobs || []);
}

function renderMediaJobs(jobs) {
  if (!state.media?.online) {
    els.mediaJobsOutput.className = "card-list empty-state";
    els.mediaJobsOutput.textContent = state.media?.error || "The home agent is offline.";
    return;
  }
  if (!jobs.length) {
    els.mediaJobsOutput.className = "card-list empty-state";
    els.mediaJobsOutput.textContent = "No media jobs reported.";
    return;
  }
  els.mediaJobsOutput.className = "card-list";
  els.mediaJobsOutput.innerHTML = jobs.map((job) => {
    const status = String(job.status || "unknown");
    const active = status === "queued" || status === "running";
    const statusClass = active || status === "completed" ? "status-chip" : "status-chip offline";
    const current = job.current_file ? `<div class="meta">Current: ${escapeHTML(job.current_file)} (${formatBytes(job.bytes_written)})</div>` : "";
    return `
      <article class="card media-job-card">
        <div class="card-head">
          <div>
            <div class="card-title">${escapeHTML(job.title || job.job_id || "Media job")}</div>
            <div class="media-job-meta">
              <span>${escapeHTML(job.completed_count || 0)}/${escapeHTML(job.total_count || 0)} complete</span>
              <span>${escapeHTML(job.skipped_count || 0)} skipped</span>
              <span>${escapeHTML(job.failed_count || 0)} failed</span>
            </div>
          </div>
          <span class="${statusClass}">${escapeHTML(status)}</span>
        </div>
        ${current}
        ${job.error_message ? `<div class="meta">${escapeHTML(job.error_message)}</div>` : ""}
        ${active ? `<div class="actions wrap"><button type="button" class="secondary" data-cancel-media-job="${escapeHTML(job.job_id)}">Cancel Job</button></div>` : ""}
      </article>
    `;
  }).join("");
}

function setSettingsTab(nextTab) {
  els.settingsTabButtons.forEach((button) => {
    const active = button.dataset.settingsTab === nextTab;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
  els.settingsPanels.forEach((panel) => {
    panel.hidden = panel.dataset.settingsPanel !== nextTab;
  });
}

function assistantSettingsFormPayload() {
  return {
    profile_notes_enabled: els.harnessProfileNotesEnabled.checked,
    home_notes_enabled: els.harnessHomeNotesEnabled.checked,
    files_enabled: els.harnessFilesEnabled.checked,
    calendar_enabled: els.harnessCalendarEnabled.checked,
    homeassistant_enabled: els.harnessHomeAssistantEnabled.checked,
    project_docs_enabled: els.harnessProjectDocsEnabled.checked,
    conversations_enabled: els.harnessConversationsEnabled.checked,
    system_prompt: els.harnessSystemPrompt.value,
  };
}

async function loadStatus() {
  const [status, assistant, settings, media] = await Promise.all([
    api("/v1/oauth/openai/status"),
    api("/v1/home/assistant/status"),
    api("/v1/home/assistant/settings"),
    api("/v1/home/assistant/media-settings").catch((error) => ({
      online: false,
      can_edit: false,
      settings: { base_url: "https://gramaton.io", preferred_quality: "1080p", require_confirmation: true },
      jobs: [],
      error: error.message,
    })),
  ]);
  state.status = status;
  state.assistant = assistant;
  state.settings = settings;
  state.media = media;
  renderStatus();
  renderAssistantSettings();
  clearTimeout(state.statusTimer);
  if (status.pending?.state === "pending") {
    const waitSeconds = Math.max(Number(status.pending.poll_after_seconds || 3), 2);
    state.statusTimer = window.setTimeout(() => {
      loadStatus().catch((error) => showToast(error.message, true));
    }, waitSeconds * 1000);
  }
}

async function saveAssistantSettings(event) {
  event.preventDefault();
  try {
    await api("/v1/home/assistant/settings", {
      method: "PUT",
      body: JSON.stringify(assistantSettingsFormPayload()),
    });
    await loadStatus();
    showToast("HankAI settings saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function saveMediaSettings() {
  try {
    if (!state.media?.online) {
      showToast("Start or redeploy the updated home agent before saving media workflow settings.", true);
      return;
    }
    const password = els.mediaGramatonPassword.value;
    const payload = await api("/v1/home/assistant/media-settings", {
      method: "PUT",
      body: JSON.stringify({
        settings: {
          enabled: els.mediaWorkflowEnabled.checked,
          base_url: els.mediaGramatonBaseURL.value,
          username: els.mediaGramatonUsername.value,
          destination_path: els.mediaDestinationPath.value,
          preferred_quality: "1080p",
          require_confirmation: true,
        },
        password,
        persist: true,
      }),
    });
    els.mediaGramatonPassword.value = "";
    state.media = { ...state.media, ...payload, jobs: state.media?.jobs || [] };
    await loadStatus();
    showToast("Media workflow settings saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function cancelMediaJob(jobID) {
  try {
    await api(`/v1/home/assistant/media-jobs/${encodeURIComponent(jobID)}/cancel`, { method: "POST" });
    await loadStatus();
    showToast("Media job cancelled.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function linkOpenAI() {
  try {
    const payload = await api("/v1/oauth/openai/start");
    if (payload.authorization_url) {
      window.location.href = payload.authorization_url;
      return;
    }
    if (payload.auth_mode === "device_code" && payload.verification_url && payload.user_code) {
      showToast(`Enter code ${payload.user_code} to finish linking.`);
      window.open(payload.verification_url, "_blank", "noopener");
      await loadStatus();
      return;
    }
    throw new Error("Link flow did not return an authorization step.");
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
els.linkOpenAIButton.addEventListener("click", linkOpenAI);
els.refreshButton.addEventListener("click", () => loadStatus().then(() => showToast("AI settings refreshed.")).catch((error) => showToast(error.message, true)));
els.assistantSettingsForm.addEventListener("submit", saveAssistantSettings);
els.saveMediaSettingsButton.addEventListener("click", saveMediaSettings);
els.refreshMediaSettingsButton.addEventListener("click", () => loadStatus().then(() => showToast("Media jobs refreshed.")).catch((error) => showToast(error.message, true)));
els.mediaJobsOutput.addEventListener("click", (event) => {
  const button = event.target.closest("[data-cancel-media-job]");
  if (!button) return;
  cancelMediaJob(button.dataset.cancelMediaJob);
});
els.settingsTabButtons.forEach((button) => {
  button.addEventListener("click", () => setSettingsTab(button.dataset.settingsTab));
});
els.resetAssistantPromptButton.addEventListener("click", () => {
  const defaultPrompt = state.settings?.defaults?.system_prompt || "";
  if (defaultPrompt) {
    els.harnessSystemPrompt.value = defaultPrompt;
  }
});

hydrate();
