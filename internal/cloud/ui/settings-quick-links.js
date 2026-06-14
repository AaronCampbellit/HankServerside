const api = window.HankAPI.request;
const escapeHTML = window.HankUI.escapeHTML;

const state = {
  user: null,
  quickLinks: [],
  quickLinksCanEdit: false,
  quickLinksEditMode: false,
  quickLinksRefreshTimer: 0,
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  quickLinksCount: document.getElementById("quick-links-count"),
  quickLinksRefresh: document.getElementById("quick-links-refresh"),
  quickLinkAdd: document.getElementById("quick-link-add"),
  quickLinksEditMode: document.getElementById("quick-links-edit-mode"),
  quickLinkForm: document.getElementById("quick-link-form"),
  quickLinkID: document.getElementById("quick-link-id"),
  quickLinkTitle: document.getElementById("quick-link-title"),
  quickLinkURL: document.getElementById("quick-link-url"),
  quickLinkDescription: document.getElementById("quick-link-description"),
  quickLinkHealth: document.getElementById("quick-link-health"),
  quickLinkCancel: document.getElementById("quick-link-cancel"),
  quickLinksList: document.getElementById("quick-links-list"),
  toast: document.getElementById("toast"),
};

function showToast(message, isError = false) {
  window.HankUI.showToast(els.toast, message, isError);
}

function renderSession() {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${state.user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function canEditQuickLinks(payload = {}) {
  return Boolean(payload.can_edit);
}

function quickLinkHost(value) {
  try {
    return new URL(value).host;
  } catch (_) {
    return value || "";
  }
}

function quickLinkStatusInfo(link) {
  const status = String(link.status || "unchecked").toLowerCase();
  if (!link.health_check_enabled || status === "disabled") {
    return { className: "disabled", label: "Not checked", detail: "Status checks off" };
  }
  if (status === "up") {
    return { className: "up", label: "Up", detail: "Up" };
  }
  if (status === "down") {
    return { className: "down", label: "Review", detail: link.last_error || "Review" };
  }
  return { className: "unchecked", label: "Unchecked", detail: "Waiting for first check" };
}

function renderQuickLinks() {
  els.quickLinksCount.textContent = `${state.quickLinks.length} link${state.quickLinks.length === 1 ? "" : "s"}`;
  if (!state.quickLinksCanEdit) {
    state.quickLinksEditMode = false;
  }
  els.quickLinksEditMode.hidden = !state.quickLinksCanEdit;
  els.quickLinksEditMode.setAttribute("aria-pressed", String(state.quickLinksEditMode));
  els.quickLinksEditMode.textContent = state.quickLinksEditMode ? "Done" : "Edit";
  els.quickLinksCount.hidden = !state.quickLinksEditMode;
  els.quickLinksRefresh.hidden = !state.quickLinksEditMode;
  els.quickLinkAdd.hidden = !state.quickLinksEditMode;
  if (!state.quickLinksCanEdit) {
    hideQuickLinkForm();
  } else if (!state.quickLinksEditMode && !els.quickLinkForm.hidden) {
    hideQuickLinkForm();
  }
  if (!state.quickLinks.length) {
    els.quickLinksList.className = "quick-links-grid empty-state";
    els.quickLinksList.innerHTML = `
      <div class="quick-links-empty-copy">
        <strong>No quick links saved.</strong>
        <span>${state.quickLinksCanEdit ? "Add links for dashboards, admin tools, and service pages." : "Ask an admin to add shared home links."}</span>
      </div>
    `;
    return;
  }

  els.quickLinksList.className = "quick-links-grid";
  els.quickLinksList.innerHTML = state.quickLinks.map((link, index) => {
    const status = quickLinkStatusInfo(link);
    const description = link.description || quickLinkHost(link.url);
    const adminActions = state.quickLinksCanEdit && state.quickLinksEditMode ? `
      <div class="quick-link-card-actions">
        <button type="button" class="ghost" data-quick-link-move="${index}" data-direction="-1" ${index === 0 ? "disabled" : ""}>Up</button>
        <button type="button" class="ghost" data-quick-link-move="${index}" data-direction="1" ${index === state.quickLinks.length - 1 ? "disabled" : ""}>Down</button>
        <button type="button" class="secondary" data-quick-link-edit="${escapeHTML(link.id)}">Edit</button>
        <button type="button" class="danger-link" data-quick-link-delete="${escapeHTML(link.id)}">Delete</button>
      </div>
    ` : "";
    return `
      <article class="quick-link-card ${escapeHTML(status.className)}">
        <a class="quick-link-main" href="${escapeHTML(link.url)}" target="_blank" rel="noreferrer">
          <span class="quick-link-status-dot" title="${escapeHTML(status.label)}"></span>
          <span class="quick-link-copy">
            <span class="quick-link-title">${escapeHTML(link.title || quickLinkHost(link.url))}</span>
            <span class="meta">${escapeHTML(description)}</span>
            <span class="quick-link-status-text">${escapeHTML(status.detail)}</span>
          </span>
        </a>
        ${adminActions}
      </article>
    `;
  }).join("");

  els.quickLinksList.querySelectorAll("[data-quick-link-edit]").forEach((button) => {
    button.addEventListener("click", () => editQuickLink(button.dataset.quickLinkEdit));
  });
  els.quickLinksList.querySelectorAll("[data-quick-link-delete]").forEach((button) => {
    button.addEventListener("click", () => deleteQuickLink(button.dataset.quickLinkDelete));
  });
  els.quickLinksList.querySelectorAll("[data-quick-link-move]").forEach((button) => {
    button.addEventListener("click", () => moveQuickLink(Number.parseInt(button.dataset.quickLinkMove || "0", 10), Number.parseInt(button.dataset.direction || "0", 10)));
  });
}

function showQuickLinkForm(link = null) {
  if (!state.quickLinksCanEdit) {
    return;
  }
  els.quickLinkID.value = link?.id || "";
  els.quickLinkTitle.value = link?.title || "";
  els.quickLinkURL.value = link?.url || "";
  els.quickLinkDescription.value = link?.description || "";
  els.quickLinkHealth.checked = link ? Boolean(link.health_check_enabled) : true;
  els.quickLinkForm.hidden = false;
  els.quickLinkAdd.setAttribute("aria-expanded", "true");
  els.quickLinkTitle.focus();
}

function hideQuickLinkForm() {
  els.quickLinkForm.reset();
  els.quickLinkID.value = "";
  els.quickLinkHealth.checked = true;
  els.quickLinkForm.hidden = true;
  els.quickLinkAdd.setAttribute("aria-expanded", "false");
}

function editQuickLink(linkID) {
  const link = state.quickLinks.find((item) => item.id === linkID);
  if (link) {
    showQuickLinkForm(link);
  }
}

function toggleQuickLinksEditMode() {
  if (!state.quickLinksCanEdit) {
    return;
  }
  state.quickLinksEditMode = !state.quickLinksEditMode;
  if (!state.quickLinksEditMode) {
    hideQuickLinkForm();
  }
  renderQuickLinks();
}

function toggleQuickLinkForm() {
  if (els.quickLinkForm.hidden) {
    showQuickLinkForm();
    return;
  }
  hideQuickLinkForm();
}

function syncQuickLinksRefresh() {
  if (state.quickLinksRefreshTimer) {
    window.clearInterval(state.quickLinksRefreshTimer);
    state.quickLinksRefreshTimer = 0;
  }
  state.quickLinksRefreshTimer = window.setInterval(() => {
    refreshQuickLinks().catch(() => {});
  }, 60000);
}

async function loadQuickLinks() {
  try {
    const payload = await api("/v1/home/quick-links");
    state.quickLinks = payload.links || [];
    state.quickLinksCanEdit = canEditQuickLinks(payload);
  } catch (_) {
    state.quickLinks = [];
    state.quickLinksCanEdit = false;
  }
  renderQuickLinks();
}

async function refreshQuickLinks() {
  const previousText = els.quickLinksRefresh.textContent;
  els.quickLinksRefresh.disabled = true;
  els.quickLinksRefresh.textContent = "Refreshing";
  try {
    const payload = await api("/v1/home/quick-links/checks", { method: "POST", body: JSON.stringify({}) });
    state.quickLinks = payload.links || [];
    state.quickLinksCanEdit = canEditQuickLinks(payload);
    renderQuickLinks();
    return payload;
  } finally {
    els.quickLinksRefresh.disabled = false;
    els.quickLinksRefresh.textContent = previousText;
  }
}

async function saveQuickLink(event) {
  event.preventDefault();
  if (!state.quickLinksCanEdit) {
    showToast("Only admins can change quick links.", true);
    return;
  }
  const linkID = els.quickLinkID.value.trim();
  const body = {
    title: els.quickLinkTitle.value.trim(),
    url: els.quickLinkURL.value.trim(),
    description: els.quickLinkDescription.value.trim(),
    health_check_enabled: els.quickLinkHealth.checked,
  };
  try {
    const path = linkID ? `/v1/home/quick-links/${encodeURIComponent(linkID)}` : "/v1/home/quick-links";
    await api(path, { method: linkID ? "PUT" : "POST", body: JSON.stringify(body) });
    hideQuickLinkForm();
    await loadQuickLinks();
    showToast("Quick link saved.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function deleteQuickLink(linkID) {
  if (!state.quickLinksCanEdit || !linkID) {
    return;
  }
  if (!window.confirm("Delete this quick link?")) {
    return;
  }
  try {
    await api(`/v1/home/quick-links/${encodeURIComponent(linkID)}`, { method: "DELETE" });
    await loadQuickLinks();
    showToast("Quick link deleted.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function moveQuickLink(index, direction) {
  if (!state.quickLinksCanEdit || !direction) {
    return;
  }
  const nextIndex = index + direction;
  if (index < 0 || nextIndex < 0 || index >= state.quickLinks.length || nextIndex >= state.quickLinks.length) {
    return;
  }
  const links = [...state.quickLinks];
  [links[index], links[nextIndex]] = [links[nextIndex], links[index]];
  try {
    const payload = await api("/v1/home/quick-links/order", {
      method: "PUT",
      body: JSON.stringify({ ids: links.map((link) => link.id) }),
    });
    state.quickLinks = payload.links || links;
    state.quickLinksCanEdit = canEditQuickLinks(payload);
    renderQuickLinks();
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
    await loadQuickLinks();
    syncQuickLinksRefresh();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.quickLinksRefresh.addEventListener("click", () => refreshQuickLinks()
  .then(() => showToast("Quick links refreshed."))
  .catch((error) => {
    loadQuickLinks().catch(() => {});
    showToast(error.message || "Quick link status checks could not be refreshed.", true);
  }));
els.quickLinksEditMode.addEventListener("click", toggleQuickLinksEditMode);
els.quickLinkAdd.addEventListener("click", toggleQuickLinkForm);
els.quickLinkForm.addEventListener("submit", saveQuickLink);
els.quickLinkCancel.addEventListener("click", hideQuickLinkForm);

hydrate();
