const api = window.HankAPI.request;

const els = {
  form: document.getElementById("password-form"),
  currentPassword: document.getElementById("current-password"),
  newPassword: document.getElementById("new-password"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
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

async function hydrate() {
  try {
    const me = await api("/v1/me");
    els.sessionState.textContent = `Signed in as ${me.user?.email || "unknown"}`;
    if (!me.user?.password_change_required) {
      window.location.replace("/dashboard");
    }
  } catch (_) {
    window.location.replace("/");
  }
}

async function submitPassword(event) {
  event.preventDefault();
  try {
    await api("/v1/auth/change-password", {
      method: "POST",
      body: JSON.stringify({
        current_password: els.currentPassword.value,
        new_password: els.newPassword.value,
      }),
    });
    window.location.replace("/dashboard");
  } catch (error) {
    showToast(error.message, true);
  }
}

els.form.addEventListener("submit", submitPassword);
hydrate();
