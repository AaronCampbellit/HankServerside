const state = {
  user: null,
  status: null,
  assistant: null,
  settings: null,
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
  assistantHarnessOutput: document.getElementById("assistant-harness-output"),
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
  const [status, assistant, settings] = await Promise.all([
    api("/v1/oauth/openai/status"),
    api("/v1/home/assistant/status"),
    api("/v1/home/assistant/settings"),
  ]);
  state.status = status;
  state.assistant = assistant;
  state.settings = settings;
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
els.resetAssistantPromptButton.addEventListener("click", () => {
  const defaultPrompt = state.settings?.defaults?.system_prompt || "";
  if (defaultPrompt) {
    els.harnessSystemPrompt.value = defaultPrompt;
  }
});

hydrate();
