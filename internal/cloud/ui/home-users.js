const api = window.HankAPI.request;

const state = {
  user: null,
  homes: [],
  members: [],
  invitations: [],
};

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
  homeSelect: document.getElementById("home-select"),
  homeRole: document.getElementById("home-role"),
  homeMeta: document.getElementById("home-meta"),
  inviteForm: document.getElementById("invite-form"),
  inviteEmail: document.getElementById("invite-email"),
  inviteRole: document.getElementById("invite-role"),
  inviteOutput: document.getElementById("invite-output"),
  inviteDisabled: document.getElementById("invite-disabled"),
  memberCount: document.getElementById("member-count"),
  memberList: document.getElementById("member-list"),
  invitationCount: document.getElementById("invitation-count"),
  invitationList: document.getElementById("invitation-list"),
  toast: document.getElementById("toast"),
};


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

function selectedHome() {
  return state.homes.find((home) => home.id === selectedHomeID()) || null;
}

function currentMembership() {
  return state.members.find((member) => member.user_id === state.user?.id) || null;
}

function isOwner() {
  return currentMembership()?.role === "admin";
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
  els.sessionMeta.textContent = "Hank Remote account is active.";
}

function renderHomes() {
  els.homeSelect.innerHTML = "";
  if (!state.homes.length) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No home yet";
    els.homeSelect.appendChild(option);
    els.homeRole.textContent = "No home";
    els.homeMeta.textContent = "Create the home from Home first.";
    return;
  }

  const requestedHomeID = new URLSearchParams(window.location.search).get("home_id");
  let selected = state.homes.find((home) => home.id === requestedHomeID)?.id || state.homes[0].id;
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

function renderMembers() {
  const owner = isOwner();
  els.memberCount.textContent = `${state.members.length} member${state.members.length === 1 ? "" : "s"}`;
  const membership = currentMembership();
  els.homeRole.textContent = membership ? `Your role: ${membership.role}` : "No access";
  const home = selectedHome();
  els.homeMeta.textContent = home ? `${home.name}` : "Pick a home to see who has access.";
  els.inviteForm.hidden = !owner;
  els.inviteDisabled.hidden = owner;

  if (!state.members.length) {
    els.memberList.className = "card-list empty-state";
    els.memberList.textContent = "No people have access yet.";
    return;
  }

  els.memberList.className = "card-list";
  els.memberList.innerHTML = "";
  state.members.forEach((member) => {
    const card = document.createElement("article");
    card.className = "card";
    const isSelf = member.user_id === state.user?.id;
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(member.email)}</div>
          <div class="meta">${escapeHTML(member.user_id)}</div>
        </div>
        <span class="pill">${escapeHTML(member.role)}</span>
      </div>
      <div class="meta">Joined ${formatDate(member.created_at)}</div>
    `;
    if (owner && !isSelf) {
      const actions = document.createElement("div");
      actions.className = "item-actions";
      const removeButton = document.createElement("button");
      removeButton.type = "button";
      removeButton.className = "ghost";
      removeButton.textContent = "Remove Person";
      removeButton.addEventListener("click", () => removeMember(member));
      actions.appendChild(removeButton);
      card.appendChild(actions);
    }
    els.memberList.appendChild(card);
  });
}

function renderInvitations() {
  els.invitationCount.textContent = `${state.invitations.length} pending`;
  if (!isOwner()) {
    els.invitationList.className = "card-list empty-state";
    els.invitationList.textContent = "Only admins can see pending invites.";
    return;
  }
  if (!state.invitations.length) {
    els.invitationList.className = "card-list empty-state";
    els.invitationList.textContent = "No pending invites.";
    return;
  }

  els.invitationList.className = "card-list";
  els.invitationList.innerHTML = "";
  state.invitations.forEach((invitation) => {
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      <div class="card-head">
        <div>
          <div class="card-title">${escapeHTML(invitation.email)}</div>
          <div class="meta">${escapeHTML(invitation.id)}</div>
        </div>
        <span class="pill">${escapeHTML(invitation.role)}</span>
      </div>
      <div class="meta">Created ${formatDate(invitation.created_at)}</div>
      <div class="meta">Expires ${formatDate(invitation.expires_at)}</div>
    `;
    const actions = document.createElement("div");
    actions.className = "item-actions";
    const revokeButton = document.createElement("button");
    revokeButton.type = "button";
    revokeButton.className = "ghost";
    revokeButton.textContent = "Cancel Invite";
    revokeButton.addEventListener("click", () => revokeInvitation(invitation));
    actions.appendChild(revokeButton);
    card.appendChild(actions);
    els.invitationList.appendChild(card);
  });
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

async function loadMembers() {
  const home = selectedHome();
  if (!home) {
    state.members = [];
    renderMembers();
    return;
  }
  state.members = (await api("/v1/home/members")).members || [];
  renderMembers();
}

async function loadInvitations() {
  const home = selectedHome();
  if (!home) {
    state.invitations = [];
    renderInvitations();
    return;
  }
  if (!isOwner()) {
    state.invitations = [];
    renderInvitations();
    return;
  }
  state.invitations = (await api("/v1/home/members/invitations")).invitations || [];
  renderInvitations();
}

async function refreshHomeUsers() {
  await loadMembers();
  await loadInvitations();
}

async function createInvitation(event) {
  event.preventDefault();
  const home = selectedHome();
  if (!home) {
    showToast("Choose a home first.", true);
    return;
  }
  try {
    const payload = await api("/v1/home/members/invitations", {
      method: "POST",
      body: JSON.stringify({
        email: els.inviteEmail.value.trim(),
      }),
    });
    els.inviteEmail.value = "";
    els.inviteRole.value = "member";
    els.inviteOutput.hidden = false;
    els.inviteOutput.innerHTML = `<strong>Invite created for ${escapeHTML(payload.email)}</strong><div class="token-meta">This code is only shown once. Share it with this person so they can join.</div><code>${escapeHTML(payload.token)}</code>`;
    await loadInvitations();
    showToast("Invite created.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function revokeInvitation(invitation) {
  const home = selectedHome();
  if (!home) return;
  if (!window.confirm(`Cancel the invite for ${invitation.email}?`)) return;
  try {
    await api(`/v1/home/members/invitations/${encodeURIComponent(invitation.id)}`, {
      method: "DELETE",
    });
    await loadInvitations();
    showToast("Invite cancelled.");
  } catch (error) {
    showToast(error.message, true);
  }
}

async function removeMember(member) {
  const home = selectedHome();
  if (!home) return;
  if (!window.confirm(`Remove ${member.email} from ${home.name}?`)) return;
  try {
    await api(`/v1/home/members/${encodeURIComponent(member.user_id)}`, {
      method: "DELETE",
    });
    await refreshHomeUsers();
    showToast("Person removed from home.");
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
    await refreshHomeUsers();
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);
els.homeSelect.addEventListener("change", async () => {
  els.inviteOutput.hidden = true;
  syncURL(selectedHomeID());
  await refreshHomeUsers();
});
els.inviteForm.addEventListener("submit", createInvitation);

hydrate();
