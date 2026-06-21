const api = window.HankAPI.request;

const state = {
  user: null,
  status: null,
  assistant: null,
  settings: null,
  models: null,
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
  assistantSettingsPill: document.getElementById("assistant-settings-pill"),
  assistantSettingsForm: document.getElementById("assistant-settings-form"),
  settingsSectionButtons: Array.from(document.querySelectorAll("[data-settings-section]")),
  settingsPanels: Array.from(document.querySelectorAll("[data-settings-panel]")),
  assistantHarnessOutput: document.getElementById("assistant-harness-output"),
  assistantToolsOutput: document.getElementById("assistant-tools-output"),
  harnessProfileNotesEnabled: document.getElementById("harness-profile-notes-enabled"),
  harnessHomeNotesEnabled: document.getElementById("harness-home-notes-enabled"),
  harnessFilesEnabled: document.getElementById("harness-files-enabled"),
  harnessCalendarEnabled: document.getElementById("harness-calendar-enabled"),
  harnessHomeAssistantEnabled: document.getElementById("harness-homeassistant-enabled"),
  harnessProjectDocsEnabled: document.getElementById("harness-project-docs-enabled"),
  harnessConversationsEnabled: document.getElementById("harness-conversations-enabled"),
  harnessAIProvider: document.getElementById("harness-ai-provider"),
  harnessOllamaBaseURL: document.getElementById("harness-ollama-base-url"),
  harnessChatModel: document.getElementById("harness-chat-model"),
  harnessEmbeddingModel: document.getElementById("harness-embedding-model"),
  harnessPlannerEnabled: document.getElementById("harness-planner-enabled"),
  harnessPlannerModel: document.getElementById("harness-planner-model"),
  harnessModelMeta: document.getElementById("harness-model-meta"),
  harnessPromptProfile: document.getElementById("harness-prompt-profile"),
  harnessSystemPrompt: document.getElementById("harness-system-prompt"),
  resetAssistantPromptButton: document.getElementById("reset-assistant-prompt-button"),
  toast: document.getElementById("toast"),
};


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

function uniqueValues(values) {
  const seen = new Set();
  const unique = [];
  values.forEach((value) => {
    const trimmed = String(value || "").trim();
    if (!trimmed || seen.has(trimmed)) return;
    seen.add(trimmed);
    unique.push(trimmed);
  });
  return unique;
}

function modelSourceLabel(source, provider) {
  switch (source) {
    case "chatgpt_codex":
      return "linked ChatGPT/Codex account";
    case "openai_api":
      return "OpenAI API key";
    case "ollama":
      return "Ollama";
    case "configured":
      return "configured fallback";
    default:
      return provider || "provider";
  }
}

function renderChatModelSelect(settings, defaults) {
  const selected = String(settings.chat_model || "").trim();
  const assistant = state.assistant || {};
  const modelState = state.models || {};
  const defaultModel = String(modelState.default_model || assistant.chat_model_default || defaults.chat_model || "").trim();
  const models = uniqueValues([
    ...(modelState.models || []),
    ...(assistant.chat_model_options || []),
    ...(defaults.chat_model_options || []),
    selected,
    defaultModel,
  ]);
  const defaultLabel = defaultModel ? `Provider default (${defaultModel})` : "Provider default";
  const options = [{ value: "", label: defaultLabel }].concat(models.map((model) => ({ value: model, label: model })));
  els.harnessChatModel.innerHTML = options.map((option) => (
    `<option value="${escapeHTML(option.value)}">${escapeHTML(option.label)}</option>`
  )).join("");
  els.harnessChatModel.value = selected;

  if (modelState.loading) {
    els.harnessModelMeta.textContent = "Loading available models from the active provider.";
  } else if (modelState.error) {
    els.harnessModelMeta.textContent = `Using fallback model list. ${modelState.error}`;
  } else if (modelState.source) {
    els.harnessModelMeta.textContent = `${models.length} model${models.length === 1 ? "" : "s"} from ${modelSourceLabel(modelState.source, modelState.provider)}.`;
  } else {
    els.harnessModelMeta.textContent = "Using configured provider defaults until the live model list is loaded.";
  }
}

function renderPlannerModelSelect(settings, defaults) {
  const selected = String(settings.planner_model || "").trim();
  const assistant = state.assistant || {};
  const modelState = state.models || {};
  const defaultModel = String(settings.chat_model || modelState.current_model || assistant.chat_model || defaults.chat_model || "").trim();
  const models = uniqueValues([
    ...(modelState.models || []),
    ...(assistant.chat_model_options || []),
    ...(defaults.chat_model_options || []),
    selected,
    defaultModel,
  ]);
  const defaultLabel = defaultModel ? `Reuse chat model (${defaultModel})` : "Reuse chat model";
  const options = [{ value: "", label: defaultLabel }].concat(models.map((model) => ({ value: model, label: model })));
  els.harnessPlannerModel.innerHTML = options.map((option) => (
    `<option value="${escapeHTML(option.value)}">${escapeHTML(option.label)}</option>`
  )).join("");
  els.harnessPlannerModel.value = selected;
}

function renderProviderSelect(settings, defaults) {
  const selected = String(settings.ai_provider || "").trim();
  const rawProviders = defaults.provider_options || [
    { key: "", label: "Configured default" },
    { key: "auto", label: "Auto" },
    { key: "ollama", label: "Local Ollama" },
    { key: "chatgpt_codex", label: "Linked ChatGPT/Codex" },
    { key: "openai", label: "OpenAI API key" },
  ];
  const providers = rawProviders.map((provider) => {
    const key = String(provider.key ?? provider.value ?? "");
    const fallbackLabel = key || "Configured default";
    return { key, label: String(provider.label ?? provider.name ?? fallbackLabel) };
  });
  els.harnessAIProvider.innerHTML = providers.map((provider) => (
    `<option value="${escapeHTML(provider.key)}">${escapeHTML(provider.label)}</option>`
  )).join("");
  if (selected && !providers.some((provider) => provider.key === selected)) {
    els.harnessAIProvider.insertAdjacentHTML("beforeend", `<option value="${escapeHTML(selected)}">${escapeHTML(selected)}</option>`);
  }
  els.harnessAIProvider.value = selected;
}

function renderEmbeddingModelSelect(settings, defaults) {
  const selected = String(settings.embedding_model || "").trim();
  const assistant = state.assistant || {};
  const defaultModel = String(assistant.embedding_model || defaults.embedding_model || "").trim();
  const models = uniqueValues([
    ...(defaults.embedding_model_options || []),
    selected,
    defaultModel,
  ]);
  const defaultLabel = defaultModel ? `Provider default (${defaultModel})` : "Provider default";
  const options = [{ value: "", label: defaultLabel }].concat(models.map((model) => ({ value: model, label: model })));
  els.harnessEmbeddingModel.innerHTML = options.map((option) => (
    `<option value="${escapeHTML(option.value)}">${escapeHTML(option.label)}</option>`
  )).join("");
  els.harnessEmbeddingModel.value = selected;
}

function renderPromptProfileSelect(settings, defaults) {
  const selected = String(settings.prompt_profile || "chatgpt").trim();
  const profiles = defaults.prompt_profiles || [];
  els.harnessPromptProfile.innerHTML = profiles.map((profile) => (
    `<option value="${escapeHTML(profile.key)}">${escapeHTML(profile.label)}</option>`
  )).join("");
  els.harnessPromptProfile.value = profiles.some((profile) => profile.key === selected) ? selected : "custom";
}

function promptProfilePrompt(profileKey) {
  const profiles = state.settings?.defaults?.prompt_profiles || [];
  const profile = profiles.find((item) => item.key === profileKey);
  return profile?.prompt || "";
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
        <div class="meta">Vector mode: ${escapeHTML(assistant.index?.vector_mode || "unavailable")}</div>
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
      <div class="meta">Vector mode: ${escapeHTML(assistant.index?.vector_mode || "unavailable")}</div>
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
  renderProviderSelect(settings, defaults);
  els.harnessOllamaBaseURL.value = settings.ollama_base_url || defaults.ollama_base_url || "";
  renderChatModelSelect(settings, defaults);
  renderEmbeddingModelSelect(settings, defaults);
  els.harnessPlannerEnabled.checked = settings.planner_enabled !== false;
  renderPlannerModelSelect(settings, defaults);
  renderPromptProfileSelect(settings, defaults);
  els.harnessSystemPrompt.value = settings.system_prompt || defaults.system_prompt || "";
  renderToolSettings(tools);
  renderMediaWorkflowSettings();
  const index = state.assistant?.index || {};

  els.assistantHarnessOutput.className = "card-list";
  els.assistantHarnessOutput.innerHTML = `
    <article class="card">
      <div class="card-title">Current provider: ${escapeHTML(state.assistant?.provider || "local")}</div>
      <div class="meta">Chat model: ${escapeHTML(state.assistant?.chat_model || "local fallback")}</div>
      <div class="meta">Provider override: ${escapeHTML(settings.ai_provider || "Configured default")}</div>
      <div class="meta">Ollama URL: ${escapeHTML(settings.ollama_base_url || defaults.ollama_base_url || "Not configured")}</div>
      <div class="meta">Model override: ${escapeHTML(settings.chat_model || "Provider default")}</div>
      <div class="meta">Embeddings: ${escapeHTML(state.assistant?.embedding_model || "local-hash")}</div>
      <div class="meta">Embedding override: ${escapeHTML(settings.embedding_model || "Provider default")}</div>
      <div class="meta">Planner: ${settings.planner_enabled === false ? "Off" : escapeHTML(settings.planner_model || settings.chat_model || "Chat model")}</div>
      <div class="meta">Prompt profile: ${escapeHTML(settings.prompt_profile || "chatgpt")}</div>
      <div class="meta">Vector mode: ${escapeHTML(index.vector_mode || "unavailable")}</div>
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

function setSettingsSection(nextSection, options = {}) {
  els.settingsSectionButtons.forEach((button) => {
    const active = button.dataset.settingsSection === nextSection;
    button.classList.toggle("active", active);
  });
  const panel = els.settingsPanels.find((item) => item.dataset.settingsPanel === nextSection);
  if (panel && options.scroll !== false) {
    panel.scrollIntoView({ block: "start", behavior: "smooth" });
  }
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
    ai_provider: els.harnessAIProvider.value.trim(),
    ollama_base_url: els.harnessOllamaBaseURL.value.trim(),
    chat_model: els.harnessChatModel.value.trim(),
    embedding_model: els.harnessEmbeddingModel.value.trim(),
    planner_enabled: els.harnessPlannerEnabled.checked,
    planner_model: els.harnessPlannerModel.value.trim(),
    prompt_profile: els.harnessPromptProfile.value.trim(),
    system_prompt: els.harnessSystemPrompt.value,
  };
}

async function loadStatus() {
  clearTimeout(state.statusTimer);
  const wasLinked = state.status?.linked === true;
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
  if (status.linked && (!wasLinked || state.models?.error)) {
    loadModelOptions().catch((error) => showToast(error.message, true));
  }
  if (status.pending?.state === "pending") {
    const waitSeconds = Math.max(Number(status.pending.poll_after_seconds || 3), 2);
    state.statusTimer = window.setTimeout(() => {
      loadStatus().catch((error) => showToast(error.message, true));
    }, waitSeconds * 1000);
  }
}

async function loadModelOptions() {
  if (state.models?.loading) {
    return;
  }
  state.models = { loading: true };
  if (state.settings) {
    renderAssistantSettings();
  }
  try {
    state.models = await api("/v1/home/assistant/models");
  } catch (error) {
    state.models = { models: [], source: "configured", error: error.message };
  }
  if (state.settings) {
    renderAssistantSettings();
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
    state.models = null;
    await loadModelOptions();
    showToast("HankAI settings saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function linkOpenAI() {
  try {
    const payload = await api("/v1/oauth/openai/start");
    if (payload.auth_mode === "device_code" && payload.verification_url && payload.user_code) {
      showToast(`Enter code ${payload.user_code} to finish linking.`);
      window.open(payload.verification_url, "_blank", "noopener");
      await loadStatus();
      loadModelOptions().catch((error) => showToast(error.message, true));
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
    const params = new URLSearchParams(window.location.search);
    if (params.get("settings_tab")) {
      setSettingsSection(params.get("settings_tab"));
    }
    await loadStatus();
    loadModelOptions().catch((error) => showToast(error.message, true));
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.linkOpenAIButton.addEventListener("click", linkOpenAI);
els.assistantSettingsForm.addEventListener("submit", saveAssistantSettings);
els.settingsSectionButtons.forEach((button) => {
  button.addEventListener("click", () => setSettingsSection(button.dataset.settingsSection));
});
els.harnessAIProvider.addEventListener("change", () => {
  const provider = els.harnessAIProvider.value;
  if (provider === "ollama" && els.harnessPromptProfile.value !== "local") {
    els.harnessPromptProfile.value = "local";
    const prompt = promptProfilePrompt("local");
    if (prompt) els.harnessSystemPrompt.value = prompt;
  } else if ((provider === "chatgpt_codex" || provider === "openai") && els.harnessPromptProfile.value !== "chatgpt") {
    els.harnessPromptProfile.value = "chatgpt";
    const prompt = promptProfilePrompt("chatgpt");
    if (prompt) els.harnessSystemPrompt.value = prompt;
  }
});
els.harnessPromptProfile.addEventListener("change", () => {
  const profile = els.harnessPromptProfile.value;
  const prompt = promptProfilePrompt(profile);
  if (prompt) {
    els.harnessSystemPrompt.value = prompt;
  }
});
els.harnessSystemPrompt.addEventListener("input", () => {
  const current = els.harnessSystemPrompt.value.trim();
  const selected = els.harnessPromptProfile.value;
  const selectedPrompt = promptProfilePrompt(selected).trim();
  if (selected !== "custom" && selectedPrompt && current !== selectedPrompt) {
    els.harnessPromptProfile.value = "custom";
  }
});
els.resetAssistantPromptButton.addEventListener("click", () => {
  const profile = els.harnessPromptProfile.value === "local" ? "local" : "chatgpt";
  const defaultPrompt = promptProfilePrompt(profile) || state.settings?.defaults?.system_prompt || "";
  if (defaultPrompt) {
    els.harnessSystemPrompt.value = defaultPrompt;
    els.harnessPromptProfile.value = profile;
  }
});

hydrate();
