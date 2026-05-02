(() => {
  const adminSelector = '[data-admin-only="true"]';
  const dashboardPages = [
    {
      href: "/dashboard",
      label: "Overview",
      detail: "Home name, connector setup, server checks, Home Assistant, files, and notes.",
      keywords: ["home", "home name", "connector", "agent", "setup file", "token", "health", "home assistant", "files", "notes"],
    },
    {
      href: "/dashboard/home-users",
      label: "People",
      detail: "Invite people, review members, and manage access.",
      keywords: ["people", "users", "members", "invite", "invitation", "role", "admin", "access"],
    },
    {
      href: "/dashboard/service-profiles",
      label: "Connections",
      detail: "Save Home Assistant and file share settings for the home connector.",
      keywords: ["connections", "home assistant", "ha token", "file share", "smb", "nas", "credentials", "settings"],
    },
    {
      href: "/dashboard/sync-status",
      label: "Status",
      detail: "Check notes, saved connections, and sync health.",
      keywords: ["status", "sync", "health", "saved connections", "notes", "profile backup"],
    },
    {
      href: "/dashboard/storage",
      label: "Storage",
      detail: "Backups, restore tests, checksum checks, and backup schedules.",
      keywords: ["storage", "backup", "backups", "restore", "checksum", "schedule", "encrypted", "retention"],
      adminOnly: true,
    },
    {
      href: "/dashboard/profile-notes",
      label: "My Notes",
      detail: "Write, search, and autosave profile notes.",
      keywords: ["notes", "my notes", "documents", "editor", "autosave", "formatting", "search notes"],
    },
    {
      href: "/dashboard/file-transfers",
      label: "Transfers",
      detail: "Upload and download files through Hank Remote.",
      keywords: ["transfers", "upload", "download", "files", "file moves"],
    },
    {
      href: "/dashboard/accept-invitation",
      label: "Join Home",
      detail: "Use an invite code to join a home.",
      keywords: ["join", "invite code", "accept invitation", "home invite"],
    },
    {
      href: "/docs/deployment",
      label: "Setup Guide",
      detail: "One-server setup, .env.cloud, .env.agent, Docker, and first checks.",
      keywords: ["setup", "deployment", "guide", "docker", "env cloud", "env agent", "port", "18080", "first checks"],
    },
  ];
  const preloaded = new Set();
  let canViewAdmin = false;

  function setAdminOnlyVisible(isVisible) {
    canViewAdmin = isVisible;
    document.querySelectorAll(adminSelector).forEach((element) => {
      element.hidden = !isVisible;
    });
    renderSideNav();
    preloadDashboardPages();
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

  function visiblePages() {
    return dashboardPages.filter((page) => !page.adminOnly || canViewAdmin);
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
        <div class="sidebar-subtitle">Server Settings</div>
      </div>
    `;
    nav.appendChild(header);

    const searchShell = document.createElement("div");
    searchShell.className = "sidebar-search-shell";
    searchShell.innerHTML = `
      <label for="dashboard-settings-search">
        <span>Search Settings</span>
        <input id="dashboard-settings-search" type="search" placeholder="Search settings" autocomplete="off">
      </label>
      <div id="dashboard-settings-results" class="sidebar-search-results" hidden></div>
    `;
    nav.appendChild(searchShell);

    const list = document.createElement("div");
    list.className = "sidebar-nav-list";
    visiblePages().forEach((page) => {
      const link = document.createElement("a");
      link.className = `tab-link${isActivePage(page) ? " active" : ""}`;
      link.href = page.href;
      link.textContent = page.label;
      if (isActivePage(page)) {
        link.setAttribute("aria-current", "page");
      }
      if (page.adminOnly) {
        link.dataset.adminOnly = "true";
      }
      list.appendChild(link);
    });
    nav.appendChild(list);

    const activePage = dashboardPages.find(isActivePage);
    const footer = document.createElement("div");
    footer.className = "sidebar-context";
    footer.textContent = activePage?.detail || "Manage Hank Remote from one place.";
    nav.appendChild(footer);

    installSettingsSearch(
      searchShell.querySelector("#dashboard-settings-search"),
      searchShell.querySelector("#dashboard-settings-results"),
    );
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

  function preloadAsset(url) {
    if (preloaded.has(url)) {
      return;
    }
    preloaded.add(url);
    fetch(url, { credentials: "same-origin", cache: "force-cache" }).catch(() => {});
  }

  function preloadPageAssets(html) {
    const doc = new DOMParser().parseFromString(html, "text/html");
    doc.querySelectorAll('script[src], link[rel="stylesheet"][href]').forEach((element) => {
      const url = element.getAttribute("src") || element.getAttribute("href");
      if (url) {
        preloadAsset(new URL(url, window.location.origin).toString());
      }
    });
  }

  function preloadPage(href) {
    const url = new URL(href, window.location.origin);
    const key = url.toString();
    if (preloaded.has(key) || url.pathname === window.location.pathname) {
      return;
    }
    preloaded.add(key);

    const link = document.createElement("link");
    link.rel = "prefetch";
    link.href = url.pathname;
    document.head.appendChild(link);

    fetch(url.pathname, { credentials: "same-origin", cache: "force-cache" })
      .then((response) => response.ok ? response.text() : "")
      .then((html) => {
        if (html) {
          preloadPageAssets(html);
        }
      })
      .catch(() => {});
  }

  function preloadDashboardPages() {
    const run = () => {
      visiblePages().forEach((page) => preloadPage(page.href));
    };

    if ("requestIdleCallback" in window) {
      window.requestIdleCallback(run, { timeout: 1800 });
      return;
    }
    window.setTimeout(run, 250);
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

  function start() {
    installDashboardShell();
    applyAdminVisibility();
    preloadDashboardPages();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }
})();
