(() => {
  const adminSelector = '[data-admin-only="true"]';

  function setAdminOnlyVisible(isVisible) {
    document.querySelectorAll(adminSelector).forEach((element) => {
      element.hidden = !isVisible;
    });
  }

  async function apiJSON(path) {
    const response = await fetch(path, { credentials: "same-origin" });
    const contentType = response.headers.get("Content-Type") || "";
    const payload = contentType.includes("application/json") ? await response.json() : await response.text();
    if (!response.ok) {
      const message = typeof payload === "string" ? payload : payload.error || payload.message || response.statusText;
      throw new Error(message);
    }
    return payload;
  }

  async function applyAdminVisibility() {
    setAdminOnlyVisible(false);
    try {
      const me = await apiJSON("/v1/me");
      const membersPayload = await apiJSON("/v1/home/members");
      const members = membersPayload.members || [];
      const current = members.find((member) => member.user_id === me.user?.id);
      setAdminOnlyVisible(current?.role === "admin");
    } catch (_) {
      setAdminOnlyVisible(false);
    }
  }

  document.addEventListener("DOMContentLoaded", applyAdminVisibility);
})();
