const api = window.HankAPI.request;

const els = {
  logoutButton: document.getElementById("logout-button"),
  sessionState: document.getElementById("session-state"),
  sessionMeta: document.getElementById("session-meta"),
};

function renderSession(user) {
  document.body.classList.add("signed-in");
  els.sessionState.textContent = `Signed in as ${user?.email || "unknown"}`;
  els.sessionMeta.textContent = "Hank Remote account is active.";
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
    renderSession(me.user);
  } catch (_) {
    window.location.replace("/");
  }
}

els.logoutButton.addEventListener("click", logout);

hydrate();
