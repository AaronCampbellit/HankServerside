(function () {
  const settingsPages = [
    { href: "/dashboard/settings/home", label: "Home" },
    { href: "/dashboard/settings/quick-links", label: "Quick Links" },
    { href: "/dashboard/settings/people", label: "People" },
    { href: "/dashboard/settings/connections", label: "Connections" },
    { href: "/dashboard/settings/ai", label: "AI" },
    { href: "/dashboard/settings/apps", label: "Apps", adminOnly: true },
    { href: "/dashboard/settings/backups", label: "Backups", adminOnly: true },
    { href: "/dashboard/settings/recovery", label: "Recovery", adminOnly: true },
    { href: "/dashboard/settings/join-home", label: "Join Home" },
  ];
  const adminRoutes = ["/dashboard/settings/apps", "/dashboard/settings/backups", "/dashboard/settings/recovery"];

  function currentPath() {
    if (window.location.pathname === "/dashboard/settings") {
      return "/dashboard/settings/home";
    }
    return window.location.pathname;
  }

  function renderSettingsNav(canViewAdmin) {
    const host = document.querySelector("[data-settings-nav]");
    if (!host) return;
    host.innerHTML = "";
    host.className = "settings-page-tabs";
    host.setAttribute("role", "tablist");
    host.setAttribute("aria-label", "Settings sections");
    const path = currentPath();
    settingsPages
      .filter((page) => !page.adminOnly || canViewAdmin)
      .forEach((page) => {
        const link = document.createElement("a");
        const active = page.href === path;
        link.href = page.href;
        link.className = `settings-page-tab${active ? " active" : ""}`;
        link.setAttribute("role", "tab");
        link.setAttribute("aria-selected", active ? "true" : "false");
        if (active) {
          link.setAttribute("aria-current", "page");
        }
        link.textContent = page.label;
        host.appendChild(link);
      });
  }

  async function canViewAdminSettings() {
    try {
      const me = await window.HankAPI.request("/v1/me");
      const membersPayload = await window.HankAPI.request("/v1/home/members");
      const members = membersPayload.members || [];
      const current = members.find((member) => member.user_id === me.user?.id);
      return current?.role === "admin";
    } catch (_) {
      return false;
    }
  }

  async function start() {
    const canViewAdmin = await canViewAdminSettings();
    renderSettingsNav(canViewAdmin);
    if (!canViewAdmin && adminRoutes.includes(window.location.pathname)) {
      window.location.replace("/dashboard/settings/home");
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }
})();
