(function () {
  function csrfToken() {
    return document.cookie
      .split("; ")
      .find((part) => part.startsWith("hank_remote_csrf="))
      ?.split("=")[1];
  }

  async function request(path, options = {}) {
    const headers = new Headers(options.headers || {});
    const csrf = csrfToken();
    if (csrf && !headers.has("X-Hank-CSRF-Token")) {
      headers.set("X-Hank-CSRF-Token", decodeURIComponent(csrf));
    }
    if (!headers.has("Content-Type") && options.body && !(options.body instanceof Blob)) {
      headers.set("Content-Type", "application/json");
    }
    const response = await fetch(path, { credentials: "same-origin", ...options, headers });
    const contentType = response.headers.get("Content-Type") || "";
    const payload = contentType.includes("application/json") ? await response.json() : await response.text();
    if (!response.ok) {
      const message = typeof payload === "string" ? payload : payload.error || payload.message || response.statusText;
      throw new Error(message);
    }
    return payload;
  }

  window.HankAPI = { request, csrfToken };
})();
