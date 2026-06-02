const api = window.HankAPI.request;

const els = {
  email: document.getElementById("email"),
  password: document.getElementById("password"),
  loginButton: document.getElementById("login-button"),
  registerButton: document.getElementById("register-button"),
  toast: document.getElementById("toast"),
};


function showToast(message, isError = false) {
  els.toast.hidden = false;
  els.toast.textContent = message;
  els.toast.style.background = isError ? "rgba(142, 45, 28, 0.94)" : "rgba(35, 27, 20, 0.92)";
  clearTimeout(showToast.timeoutID);
  showToast.timeoutID = window.setTimeout(() => {
    els.toast.hidden = true;
  }, 3200);
}

async function hydrate() {
  try {
    await api("/v1/me");
    window.location.replace("/dashboard");
  } catch (_) {
  }
}

async function submit(mode) {
  const email = els.email.value.trim();
  const password = els.password.value;
  if (!email || !password) {
    showToast("Email and password are required.", true);
    return;
  }
  try {
    await api(`/v1/auth/${mode}`, {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
    window.location.replace("/dashboard");
  } catch (error) {
    showToast(error.message, true);
  }
}

els.loginButton.addEventListener("click", () => submit("login"));
els.registerButton.addEventListener("click", () => submit("register"));

hydrate();
