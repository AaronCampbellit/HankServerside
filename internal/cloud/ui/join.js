const api = window.HankAPI.request;

const els = {
  form: document.getElementById("join-form"),
  token: document.getElementById("token"),
  email: document.getElementById("email"),
  password: document.getElementById("password"),
  inviteState: document.getElementById("invite-state"),
  inviteMeta: document.getElementById("invite-meta"),
  invitePreview: document.getElementById("invite-preview"),
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

async function previewInvite() {
  const token = els.token.value.trim();
  if (!token) return;
  try {
    const preview = await api("/v1/auth/invitations/preview", {
      method: "POST",
      body: JSON.stringify({ token }),
    });
    els.email.value = preview.email || els.email.value;
    els.inviteState.textContent = "Invite Found";
    els.inviteMeta.textContent = `${preview.email} can join as ${preview.role}.`;
    els.invitePreview.hidden = false;
    els.invitePreview.innerHTML = `<strong>${escapeHTML(preview.email)}</strong><div class="token-meta">Role: ${escapeHTML(preview.role)}</div>`;
  } catch (error) {
    els.inviteState.textContent = "Invite Needed";
    els.inviteMeta.textContent = "Enter a valid invite code.";
    els.invitePreview.hidden = true;
    showToast(error.message, true);
  }
}

async function submitJoin(event) {
  event.preventDefault();
  try {
    await api("/v1/auth/invitations/signup", {
      method: "POST",
      body: JSON.stringify({
        token: els.token.value.trim(),
        email: els.email.value.trim(),
        password: els.password.value,
      }),
    });
    window.location.replace("/dashboard");
  } catch (error) {
    showToast(error.message, true);
  }
}

function hydrate() {
  const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ""));
  const queryParams = new URLSearchParams(window.location.search);
  const token = hashParams.get("token") || queryParams.get("token");
  if (token) {
    els.token.value = token;
    previewInvite();
  }
}

els.form.addEventListener("submit", submitJoin);
els.token.addEventListener("change", previewInvite);
hydrate();
