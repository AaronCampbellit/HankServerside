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
      href: "/dashboard/settings#home",
      label: "Home Settings",
      detail: "Rename the home, review the connector, and create setup files.",
      keywords: ["home name", "connector", "agent", "setup file", "token", "settings"],
      searchOnly: true,
    },
    {
      href: "/dashboard/settings#quick-links",
      label: "Quick Links",
      detail: "Add, reorder, and refresh shared homepage links.",
      keywords: ["quick links", "links", "homepage links", "home links", "add link", "refresh links", "settings"],
      searchOnly: true,
    },
    {
      href: "/dashboard/settings#people",
      label: "People Settings",
      detail: "Invite people, review members, and manage access.",
      keywords: ["people", "users", "members", "invite", "invitation", "role", "admin", "access"],
      searchOnly: true,
    },
    {
      href: "/dashboard/settings#connections",
      label: "Connection Settings",
      detail: "Save Home Assistant and file server settings for the connector.",
      keywords: ["connections", "home assistant", "ha token", "settings", "file server", "smb", "nas", "credentials", "share"],
      searchOnly: true,
    },
    {
      href: "/dashboard/settings#ai",
      label: "AI Settings",
      detail: "OpenAI account linking and HankAI setup.",
      keywords: ["ai", "assistant", "openai", "chatgpt", "oauth", "link openai", "relink", "subscription", "settings", "tools", "workflows", "media downloads"],
      searchOnly: true,
    },
    {
      href: "/dashboard/settings#backups",
      label: "Backup Settings",
      detail: "Backups, restore tests, checksum checks, and backup schedules.",
      keywords: ["storage", "backup", "backups", "restore", "checksum", "schedule", "encrypted", "retention"],
      adminOnly: true,
      searchOnly: true,
    },
    {
      href: "/dashboard/settings#recovery",
      label: "Recovery Settings",
      detail: "Export redacted settings and import rebuild profiles.",
      keywords: ["recovery", "settings export", "settings import", "rebuild", "redacted", "configuration bundle"],
      adminOnly: true,
      searchOnly: true,
    },
    {
      href: "/dashboard/settings#join-home",
      label: "Join Home",
      detail: "Use an invite code to join a home.",
      keywords: ["join", "invite code", "accept invitation", "home invite"],
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
  const preloaded = new Set();
  let canViewAdmin = false;
  let dashboardFrame = null;

  function isEmbeddedDashboard() {
    return new URLSearchParams(window.location.search).get("embedded") === "1";
  }

  function embeddedURL(href) {
    const url = new URL(href, window.location.origin);
    url.searchParams.set("embedded", "1");
    return url;
  }

  function visibleURL(href) {
    const url = new URL(href, window.location.origin);
    url.searchParams.delete("embedded");
    return url;
  }

  function setAdminOnlyVisible(isVisible) {
    canViewAdmin = isVisible;
    document.querySelectorAll(adminSelector).forEach((element) => {
      element.hidden = !isVisible;
    });
    renderSideNav();
    preloadDashboardPages();
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

  function installEmbeddedMode() {
    if (!isEmbeddedDashboard()) {
      return false;
    }
    document.documentElement.classList.add("embedded-dashboard");
    document.body.classList.add("embedded-dashboard");
    document.addEventListener("click", (event) => {
      const link = event.target.closest("a[href]");
      if (!link || event.defaultPrevented || event.button !== 0) {
        return;
      }
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || (link.target && link.target !== "_self")) {
        return;
      }
      const url = new URL(link.href, window.location.origin);
      if (url.origin !== window.location.origin || !url.pathname.startsWith("/dashboard")) {
        return;
      }
      event.preventDefault();
      window.parent.postMessage({ type: "hank-dashboard:navigate", href: visibleURL(url).toString() }, window.location.origin);
    });
    return true;
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

  function currentFrameSource() {
    return embeddedURL(window.location.href).toString();
  }

  function ensureDashboardFrame() {
    const shell = document.querySelector(".dashboard-shell");
    const main = shell?.querySelector("main");
    if (!shell || !main || dashboardFrame) {
      return dashboardFrame;
    }
    main.classList.add("dashboard-frame-main");
    main.classList.remove("grid", "notes-workspace", "hank-chat-layout", "file-server-grid");
    main.innerHTML = "";
    dashboardFrame = document.createElement("iframe");
    dashboardFrame.className = "dashboard-content-frame";
    dashboardFrame.title = "Dashboard content";
    dashboardFrame.allow = "clipboard-read; clipboard-write";
    dashboardFrame.src = currentFrameSource();
    dashboardFrame.addEventListener("load", () => {
      document.body.classList.remove("dashboard-navigating");
      try {
        const framedURL = visibleURL(dashboardFrame.contentWindow.location.href);
        setActiveShellPage(framedURL);
      } catch (_) {
      }
    });
    main.appendChild(dashboardFrame);
    return dashboardFrame;
  }

  function navigateDashboardFrame(href, pushHistory = true) {
    const frame = ensureDashboardFrame();
    if (!frame) {
      window.location.assign(href);
      return;
    }
    const url = visibleURL(href);
    document.body.classList.add("dashboard-navigating");
    setActiveShellPage(url);
    frame.src = embeddedURL(url).toString();
    if (pushHistory) {
      window.history.pushState({ dashboardURL: url.toString() }, "", url.pathname + url.search + url.hash);
    }
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
      visibleNavPages().forEach((page) => preloadPage(page.href));
    };

    if ("requestIdleCallback" in window) {
      window.requestIdleCallback(run, { timeout: 1800 });
      return;
    }
    window.setTimeout(run, 250);
  }

  function shouldSmoothNavigate(event, link) {
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
    return url.pathname !== window.location.pathname || url.search !== window.location.search;
  }

  function installSmoothNavigation() {
    document.addEventListener("click", (event) => {
      const link = event.target.closest("a[href]");
      if (!shouldSmoothNavigate(event, link)) {
        return;
      }
      event.preventDefault();
      navigateDashboardFrame(link.href);
    });

    window.addEventListener("message", (event) => {
      if (event.origin !== window.location.origin || event.data?.type !== "hank-dashboard:navigate") {
        return;
      }
      navigateDashboardFrame(event.data.href);
    });

    window.addEventListener("popstate", () => {
      navigateDashboardFrame(window.location.href, false);
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
    if (installEmbeddedMode()) {
      collapseMobileDetails();
      applyAdminVisibility();
      return;
    }
    installDashboardShell();
    installSmoothNavigation();
    ensureDashboardFrame();
    collapseMobileDetails();
    applyAdminVisibility();
    preloadDashboardPages();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }
})();
