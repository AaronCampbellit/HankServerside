(() => {
  const navAPI = window.HankAPI.request;
  const adminSelector = '[data-admin-only="true"]';
  const dashboardPages = [
    {
      href: "/dashboard",
      label: "Home",
      mobileLabel: "Home",
      detail: "Home name, connector setup, server checks, and quick links.",
      keywords: ["home", "home name", "connector", "agent", "setup file", "token", "health", "server"],
      group: "Home",
    },
    {
      href: "/dashboard/hank",
      label: "Hank",
      mobileLabel: "Hank",
      detail: "Ask HankAI about notes, calendars, Home Assistant, and files.",
      keywords: ["hank", "hankai", "ai", "assistant", "chat", "notes", "calendar", "home assistant", "files", "smb"],
      group: "Tools",
    },
    {
      href: "/dashboard/home-assistant",
      label: "Home Assistant",
      mobileLabel: "HA",
      detail: "Run Home Assistant health, state, and service commands.",
      keywords: ["home assistant", "ha", "states", "service", "lights", "sensors", "tools"],
      group: "Tools",
    },
    {
      href: "/dashboard/file-server",
      label: "File Server",
      mobileLabel: "Files",
      detail: "Browse, upload, download, and manage home files.",
      keywords: ["file server", "files", "file share", "smb", "nas", "transfers", "upload", "download", "file moves"],
      group: "Tools",
    },
    {
      href: "/dashboard/profile-notes",
      label: "Notes",
      mobileLabel: "Notes",
      detail: "Write, search, and autosave profile notes.",
      keywords: ["notes", "my notes", "documents", "editor", "autosave", "formatting", "search notes"],
      group: "Tools",
    },
    {
      href: "/dashboard/settings",
      label: "Settings",
      mobileLabel: "Settings",
      detail: "Home, people, connections, AI, backups, recovery, and invitations.",
      keywords: ["settings", "home settings", "people", "connections", "ai", "backups", "recovery", "settings export", "import settings", "join home", "invite"],
      group: "Settings",
    },
    {
      href: "/dashboard/settings/home",
      label: "Home Settings",
      detail: "Rename the home, review the connector, and create setup files.",
      keywords: ["home name", "connector", "agent", "setup file", "token", "settings"],
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/quick-links",
      label: "Quick Links",
      detail: "Add, reorder, and refresh shared homepage links.",
      keywords: ["quick links", "links", "homepage links", "home links", "add link", "refresh links", "settings"],
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/people",
      label: "People Settings",
      detail: "Invite people, review members, and manage access.",
      keywords: ["people", "users", "members", "invite", "invitation", "role", "admin", "access"],
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/connections",
      label: "Connection Settings",
      detail: "Save Home Assistant and file server settings for the connector.",
      keywords: ["connections", "home assistant", "ha token", "settings", "file server", "smb", "nas", "credentials", "share"],
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/ai",
      label: "AI Settings",
      detail: "OpenAI account linking and HankAI setup.",
      keywords: ["ai", "assistant", "openai", "chatgpt", "oauth", "link openai", "relink", "subscription", "settings", "tools", "workflows", "media downloads"],
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/apps",
      label: "App Settings",
      mobileLabel: "Apps",
      detail: "Import, configure, enable, and disable installed Hank agent apps.",
      keywords: ["apps", "packages", "install", "configure", "ydownload", "hermes", "app packages", "install app", "import app", "hankapp", "workflows", "tools"],
      adminOnly: true,
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/backups",
      label: "Backup Settings",
      mobileLabel: "Backups",
      detail: "Configure backups, retention, restore tests, and storage health.",
      keywords: ["backup", "backups", "restore", "storage", "pgbackrest", "checksum", "schedule", "encrypted", "retention"],
      adminOnly: true,
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/recovery",
      label: "Recovery Settings",
      mobileLabel: "Recovery",
      detail: "Export and import redacted settings bundles.",
      keywords: ["recovery", "restore", "export", "import", "redacted settings", "settings export", "settings import", "rebuild", "configuration bundle"],
      adminOnly: true,
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/logs",
      label: "Log Settings",
      mobileLabel: "Logs",
      detail: "Review login, resource access, transfer, package, and security events.",
      keywords: ["logs", "audit", "events", "login", "failed actions", "downloads", "uploads", "installs", "security"],
      adminOnly: true,
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/dashboard/settings/join-home",
      label: "Join Home",
      detail: "Use an invite code to join a home.",
      keywords: ["join", "invite code", "accept invitation", "home invite"],
      group: "Settings",
      searchOnly: true,
    },
    {
      href: "/docs/deployment",
      label: "Setup Guide",
      mobileLabel: "Setup",
      detail: "One-server setup, .env.cloud, .env.agent, Docker, and first checks.",
      keywords: ["setup", "deployment", "guide", "docker", "env cloud", "env agent", "port", "18080", "first checks"],
      group: "Support",
    },
  ];
  let canViewAdmin = false;

  function setAdminOnlyVisible(isVisible) {
    canViewAdmin = isVisible;
    document.querySelectorAll(adminSelector).forEach((element) => {
      element.hidden = !isVisible;
    });
    renderSideNav();
  }

  function visiblePages() {
    return dashboardPages.filter((page) => !page.adminOnly || canViewAdmin);
  }

  function visibleNavPages() {
    return visiblePages().filter((page) => !page.searchOnly);
  }

  function normalizedPath(href) {
    try {
      return new URL(href, window.location.origin).pathname;
    } catch (_) {
      return href;
    }
  }

  function isActivePage(page) {
    return normalizedPath(page.href) === window.location.pathname;
  }

  function pageForURL(url) {
    return dashboardPages.find((page) => normalizedPath(page.href) === url.pathname) || null;
  }

  function setHeroForPage(page) {
    if (!page) return;
    const hero = document.querySelector(".dashboard-shell > .hero");
    if (!hero) return;
    const title = hero.querySelector("h1");
    const lede = hero.querySelector(".lede");
    if (title) {
      title.textContent = page.label;
    }
    if (lede) {
      lede.textContent = page.detail;
    }
    document.title = `Hank Remote ${page.label}`;
  }

  function setActiveShellPage(url) {
    document.querySelectorAll(".dashboard-shell .tab-link").forEach((link) => {
      const isActive = normalizedPath(link.href) === url.pathname;
      link.classList.toggle("active", isActive);
      if (isActive) {
        link.setAttribute("aria-current", "page");
      } else {
        link.removeAttribute("aria-current");
      }
    });
    const page = pageForURL(url);
    setHeroForPage(page);
  }

  function installDashboardShell() {
    const shell = document.querySelector(".shell");
    if (!shell || document.querySelector(".auth-grid")) {
      return;
    }

    let nav = shell.querySelector(".tab-strip");
    const main = shell.querySelector("main");
    if (!nav && main) {
      nav = document.createElement("nav");
      nav.className = "tab-strip";
      nav.setAttribute("aria-label", "Dashboard Sections");
      shell.insertBefore(nav, main);
    }
    if (!nav) {
      return;
    }

    shell.classList.add("dashboard-shell");
    nav.classList.add("side-nav");
    renderSideNav();
  }

  function renderSideNav() {
    const nav = document.querySelector(".dashboard-shell .tab-strip");
    if (!nav) {
      return;
    }

    nav.innerHTML = "";

    const header = document.createElement("div");
    header.className = "sidebar-brand";
    header.innerHTML = `
      <img class="sidebar-brand-icon" src="/assets/hank-icon-192.png" alt="" aria-hidden="true">
      <div>
        <div class="sidebar-title">Hank Remote</div>
        <div class="sidebar-subtitle">Tools and Settings</div>
      </div>
    `;
    nav.appendChild(header);

    const searchShell = document.createElement("div");
    searchShell.className = "sidebar-search-shell";
    searchShell.innerHTML = `
      <input id="dashboard-settings-search" type="search" placeholder="Search" autocomplete="off" aria-label="Search">
      <div id="dashboard-settings-results" class="sidebar-search-results" hidden></div>
    `;
    nav.appendChild(searchShell);

    const list = document.createElement("div");
    list.className = "sidebar-nav-list";
    ["Home", "Tools", "Settings", "Support"].forEach((group) => {
      const pages = visibleNavPages().filter((page) => (page.group || "Tools") === group);
      if (!pages.length) return;
      pages.forEach((page) => {
        const link = document.createElement("a");
        link.className = `tab-link${isActivePage(page) ? " active" : ""}`;
        link.href = page.href;
        link.title = page.label;
        link.setAttribute("aria-label", page.label);
        const label = document.createElement("span");
        label.className = "tab-link-label";
        label.textContent = page.label;
        const mobileLabel = document.createElement("span");
        mobileLabel.className = "tab-link-mobile-label";
        mobileLabel.textContent = page.mobileLabel || page.label;
        link.append(label, mobileLabel);
        if (isActivePage(page)) {
          link.setAttribute("aria-current", "page");
        }
        if (page.adminOnly) {
          link.dataset.adminOnly = "true";
        }
        list.appendChild(link);
      });
    });
    nav.appendChild(list);

    installSettingsSearch(
      searchShell.querySelector("#dashboard-settings-search"),
      searchShell.querySelector("#dashboard-settings-results"),
    );
  }

  function collapseMobileDetails() {
    if (!window.matchMedia("(max-width: 820px)").matches) {
      return;
    }
    document.querySelectorAll("details.collapsible-panel[open]").forEach((details) => {
      if (details.dataset.mobileKeepOpen === "true") {
        return;
      }
      details.open = false;
    });
  }

  function scorePage(page, queryTerms) {
    const haystack = [
      page.label,
      page.detail,
      ...(page.keywords || []),
    ].join(" ").toLowerCase();
    const label = page.label.toLowerCase();
    let score = 0;

    queryTerms.forEach((term) => {
      if (!term) {
        return;
      }
      if (label === term) {
        score += 12;
      } else if (label.includes(term)) {
        score += 8;
      }
      if (haystack.includes(term)) {
        score += 4;
      }
    });

    return score;
  }

  function matchingPages(query) {
    const terms = query.toLowerCase().split(/\s+/).map((term) => term.trim()).filter(Boolean);
    if (!terms.length) {
      return [];
    }

    return visiblePages()
      .map((page) => ({ page, score: scorePage(page, terms) }))
      .filter((item) => item.score > 0)
      .sort((left, right) => right.score - left.score)
      .map((item) => item.page);
  }

  function renderSearchResults(results, matches) {
    results.innerHTML = "";
    if (!matches.length) {
      const empty = document.createElement("div");
      empty.className = "sidebar-search-empty";
      empty.textContent = "No matching settings.";
      results.appendChild(empty);
      results.hidden = false;
      return;
    }

    matches.slice(0, 5).forEach((page) => {
      const result = document.createElement("a");
      result.className = "sidebar-search-result";
      result.href = page.href;
      result.innerHTML = `
        <strong>${page.label}</strong>
        <span>${page.detail}</span>
      `;
      results.appendChild(result);
    });
    results.hidden = false;
  }

  function installSettingsSearch(input, results) {
    if (!input || !results) {
      return;
    }

    input.addEventListener("input", () => {
      const query = input.value.trim();
      if (!query) {
        results.hidden = true;
        results.innerHTML = "";
        return;
      }
      renderSearchResults(results, matchingPages(query));
    });

    input.addEventListener("keydown", (event) => {
      if (event.key !== "Enter") {
        return;
      }
      const first = matchingPages(input.value.trim())[0];
      if (!first) {
        return;
      }
      event.preventDefault();
      window.location.href = first.href;
    });
  }

  function shouldUpdateRouteState(event, link) {
    if (!link || event.defaultPrevented || event.button !== 0) {
      return false;
    }
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return false;
    }
    if (link.target && link.target !== "_self") {
      return false;
    }
    const url = new URL(link.href, window.location.origin);
    if (url.origin !== window.location.origin) {
      return false;
    }
    if (!url.pathname.startsWith("/dashboard")) {
      return false;
    }
    return true;
  }

  function installRouteStateSync() {
    document.addEventListener("click", (event) => {
      const link = event.target.closest("a[href]");
      if (!shouldUpdateRouteState(event, link)) {
        return;
      }
      setActiveShellPage(new URL(link.href, window.location.origin));
    });
  }

  async function applyAdminVisibility() {
    setAdminOnlyVisible(false);
    try {
      const me = await navAPI("/v1/me");
      const membersPayload = await navAPI("/v1/home/members");
      const members = membersPayload.members || [];
      const current = members.find((member) => member.user_id === me.user?.id);
      setAdminOnlyVisible(current?.role === "admin");
    } catch (_) {
      setAdminOnlyVisible(false);
    }
  }

  function start() {
    installDashboardShell();
    installRouteStateSync();
    setActiveShellPage(new URL(window.location.href));
    collapseMobileDetails();
    applyAdminVisibility();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }
})();
