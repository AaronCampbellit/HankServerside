import { type MouseEvent, type ReactNode, useEffect, useRef, useState } from "react";
import { internalNavigationTarget } from "../router";
import type { GroupedNavItem } from "./navConfig";
import { searchClient, type SearchResult } from "../api/search";
import { notificationsClient, type NotificationItem } from "../api/notifications";

/* Minimal dependency-free icon set keyed by route path. */
function NavIcon({ href }: { href: string }) {
  const p =
    href === "/dashboard" ? "M3 10.5 12 3l9 7.5V21H3z" :
    href === "/dashboard/hank" ? "M12 3l2.5 5.5L20 11l-5.5 2.5L12 19l-2.5-5.5L4 11l5.5-2.5z" :
    href === "/dashboard/home-assistant" ? "M9 21h6m-3-4v4M7 13a5 5 0 1 1 10 0c0 2-2 3-2.5 4h-5C9 16 7 15 7 13z" :
    href === "/dashboard/profile-notes" ? "M6 3h9l3 3v15H6zM9 8h6M9 12h6M9 16h4" :
    href === "/dashboard/file-server" ? "M3 7h6l2 2h10v11H3z" :
    href.startsWith("/dashboard/settings") ? "M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6zM4 12h2m12 0h2M12 4v2m0 12v2" :
    href === "/docs/deployment" ? "M5 4h14v16H5zM8 8h8M8 12h8M8 16h5" :
    "M5 12h14"; // fallback
  return (
    <svg className="tab-link-icon" viewBox="0 0 24 24" width="18" height="18"
      fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={p} />
    </svg>
  );
}

const RESULT_GLYPH: Record<string, string> = {
  page: "↵", note: "✎", quick_link: "↗", app: "▦", member: "@", file: "▤", homeassistant: "⌂",
};

const COLLAPSE_KEY = "hank.nav.collapsed";

function initialsFor(value: string): string {
  const local = value.includes("@") ? value.split("@")[0] : value;
  const parts = local.split(/[._\-\s]+/).filter(Boolean);
  const letters = parts.length > 1 ? `${parts[0][0]}${parts[1][0]}` : local.slice(0, 2);
  return (letters || "HK").toUpperCase();
}

function footerNameFor(value: string): string {
  if (!value.includes("@")) return value;
  const local = value.split("@")[0] || value;
  return local.replace(/[._-]+/g, " ");
}

export function Shell({
  navItems,
  currentPath,
  onNavigate,
  onPrefetch,
  children,
  onLogout,
  userEmail,
  userRole,
  connectorOnline = true,
}: {
  navItems: GroupedNavItem[];
  currentPath: string;
  onNavigate: (href: string) => void;
  onPrefetch?: (href: string) => void;
  onLogout: () => void;
  children: ReactNode;
  userEmail?: string;
  userRole?: string;
  connectorOnline?: boolean;
}) {
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    try { return localStorage.getItem(COLLAPSE_KEY) === "1"; } catch { return false; }
  });
  const [notifOpen, setNotifOpen] = useState(false);
  const [notifications, setNotifications] = useState<NotificationItem[]>([]);
  const [notificationsStatus, setNotificationsStatus] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const notifButtonRef = useRef<HTMLButtonElement>(null);
  const notifPopoverRef = useRef<HTMLDivElement>(null);

  // --- global search state ---
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searchOpen, setSearchOpen] = useState(false);
  const [activeIdx, setActiveIdx] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    try { localStorage.setItem(COLLAPSE_KEY, collapsed ? "1" : "0"); } catch { /* ignore */ }
  }, [collapsed]);

  // debounced search
  useEffect(() => {
    const q = query.trim();
    if (!q) { setResults([]); return; }
    const controller = new AbortController();
    const handle = window.setTimeout(() => {
      searchClient.search(q, controller.signal)
        .then((items) => { setResults(items); setActiveIdx(0); })
        .catch(() => { /* aborted or failed — leave prior results */ });
    }, 180);
    return () => { controller.abort(); window.clearTimeout(handle); };
  }, [query]);

  useEffect(() => {
    if (!notifOpen) return;
    const controller = new AbortController();
    setNotificationsStatus("loading");
    notificationsClient.list(controller.signal)
      .then((items) => {
        setNotifications(items);
        setNotificationsStatus("ready");
      })
      .catch(() => {
        setNotificationsStatus("error");
      });
    return () => controller.abort();
  }, [notifOpen]);

  // Close the notifications popover when clicking anywhere outside it.
  useEffect(() => {
    if (!notifOpen) return;
    function onPointerDown(event: PointerEvent) {
      const target = event.target as Node;
      if (notifPopoverRef.current?.contains(target) || notifButtonRef.current?.contains(target)) return;
      setNotifOpen(false);
    }
    function onEscape(event: KeyboardEvent) {
      if (event.key === "Escape") setNotifOpen(false);
    }
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onEscape);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onEscape);
    };
  }, [notifOpen]);

  // ⌘K / Ctrl-K focuses search
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        searchInputRef.current?.focus();
        setSearchOpen(true);
      }
      if (e.key === "Escape") setSearchOpen(false);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  function chooseResult(result: SearchResult) {
    setSearchOpen(false);
    setQuery("");
    setResults([]);
    if (result.external) {
      window.open(result.url, "_blank", "noopener");
    } else {
      onNavigate(result.url);
    }
  }

  function onSearchKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (!results.length) return;
    if (e.key === "ArrowDown") { e.preventDefault(); setActiveIdx((i) => (i + 1) % results.length); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setActiveIdx((i) => (i - 1 + results.length) % results.length); }
    else if (e.key === "Enter") { e.preventDefault(); chooseResult(results[activeIdx]); }
  }

  function handleClick(event: MouseEvent<HTMLDivElement>) {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.altKey || event.ctrlKey || event.shiftKey) {
      return;
    }
    const anchor = (event.target as HTMLElement).closest("a");
    if (!anchor || anchor.target || anchor.hasAttribute("download")) {
      return;
    }
    const target = internalNavigationTarget(anchor.href);
    if (!target) {
      return;
    }
    event.preventDefault();
    onNavigate(target);
  }

  const settingsActive = currentPath.startsWith("/dashboard/settings");
  function isActive(href: string) {
    if (href === currentPath) return true;
    if (href === "/dashboard/settings" && settingsActive) return true;
    return false;
  }
  const current = navItems.find((item) => isActive(item.href));
  const crumbLabel = current && current.href !== "/dashboard" ? current.label : null;
  const showResults = searchOpen && query.trim().length > 0;
  const footerName = footerNameFor(userEmail || "Aaron D.");
  const roleLabel = userRole || "admin";
  const initials = initialsFor(footerName);

  return (
    <div className="app-shell" data-nav-collapsed={collapsed ? "true" : "false"} onClick={handleClick}>
      <nav className="app-nav" aria-label="Main">
        <div className="sidebar-brand">
          <img className="sidebar-brand-icon" src="/assets/hank-icon-192.png" alt="" />
          <div>
            <div className="sidebar-title">Hank Remote</div>
            <div className="sidebar-subtitle">home server</div>
          </div>
          <button
            type="button"
            className="nav-collapse-btn"
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            onClick={(e) => { e.preventDefault(); setCollapsed((v) => !v); }}
          >
            {collapsed ? "›" : "‹"}
          </button>
        </div>

        {navItems.map((item) => {
          return (
            <div key={item.href} style={{ display: "contents" }}>
              <a
                className="tab-link"
                href={item.href}
                title={item.label}
                aria-current={isActive(item.href) ? "page" : undefined}
                onFocus={() => onPrefetch?.(item.href)}
                onMouseEnter={() => onPrefetch?.(item.href)}
                onTouchStart={() => onPrefetch?.(item.href)}
              >
                <NavIcon href={item.href} />
                <span className="tab-link-label">{item.label}</span>
              </a>
            </div>
          );
        })}

        <div className="nav-footer" aria-label="Session status">
          <div className="nav-footer-status">
            <span className={`status-dot ${connectorOnline ? "ok" : "warn"}`} aria-hidden="true" />
            <span>{connectorOnline ? "Connector online" : "Connector offline"}</span>
            <small>{connectorOnline ? "2m" : "now"}</small>
          </div>
          <div className="nav-footer-rule" />
          <div className="nav-footer-user">
            <span className="nav-footer-avatar" aria-hidden="true">{initials}</span>
            <span className="nav-footer-text">
              <strong title={userEmail}>{footerName}</strong>
              <small>{roleLabel}</small>
            </span>
            <button className="nav-footer-signout" type="button" onClick={onLogout} title="Sign out" aria-label="Sign out">
              <svg className="tab-link-icon" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M15 12H4m0 0 3.5-3.5M4 12l3.5 3.5M14 5h4a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-4" />
              </svg>
            </button>
          </div>
        </div>
      </nav>

      <div className="app-content">
        <header className="app-topbar">
          <nav className="topbar-crumbs" aria-label="Breadcrumb">
            <a href="/dashboard">Home</a>
            {crumbLabel ? (
              <>
                <span aria-hidden="true">›</span>
                <span className="crumb-current">{crumbLabel}</span>
              </>
            ) : null}
          </nav>
          <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 10 }}>
            <div className="topbar-search-wrap">
              <label className="topbar-search">
                <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
                  <circle cx="11" cy="11" r="7" /><path d="m20 20-3-3" strokeLinecap="round" />
                </svg>
                <input
                  ref={searchInputRef}
                  type="search"
                  placeholder="Search Hank..."
                  aria-label="Search"
                  value={query}
                  onChange={(e) => { setQuery(e.target.value); setSearchOpen(true); setNotifOpen(false); }}
                  onFocus={() => { setSearchOpen(true); setNotifOpen(false); }}
                  onBlur={() => window.setTimeout(() => setSearchOpen(false), 120)}
                  onKeyDown={onSearchKeyDown}
                />
                <kbd>⌘K</kbd>
              </label>
              {showResults ? (
                <div className="search-results" role="listbox">
                  {results.length === 0 ? (
                    <div className="search-empty">No matches for “{query.trim()}”.</div>
                  ) : (
                    results.map((result, idx) => (
                      <button
                        type="button"
                        role="option"
                        aria-selected={idx === activeIdx}
                        key={`${result.type}:${result.url}:${idx}`}
                        className={`search-result${idx === activeIdx ? " is-active" : ""}`}
                        onMouseEnter={() => setActiveIdx(idx)}
                        onMouseDown={(e) => { e.preventDefault(); chooseResult(result); }}
                      >
                        <span className="search-result-glyph" aria-hidden="true">{RESULT_GLYPH[result.type] ?? "•"}</span>
                        <span className="search-result-text">
                          <span className="search-result-title">{result.title}</span>
                          {result.subtitle ? <span className="search-result-sub">{result.subtitle}</span> : null}
                        </span>
                        <span className="search-result-kind">{result.type.replace("_", " ")}</span>
                      </button>
                    ))
                  )}
                </div>
              ) : null}
            </div>
            <span className="operational-pill">
              <span className="status-dot ok" aria-hidden="true" />
              Operational
            </span>
            <button
              ref={notifButtonRef}
              type="button"
              className={`topbar-icon-btn${notifications.length ? " has-notifications" : ""}`}
              aria-label="Notifications"
              aria-expanded={notifOpen}
              onClick={(e) => { e.preventDefault(); setSearchOpen(false); setNotifOpen((v) => !v); }}
            >
              <svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M6 9a6 6 0 0 1 12 0c0 5 2 6 2 6H4s2-1 2-6M10 20a2 2 0 0 0 4 0" />
              </svg>
              {notifications.length ? <span className="dot" aria-hidden="true" /> : null}
            </button>
            {notifOpen ? (
              <div className="notif-popover" role="dialog" aria-label="Notifications" ref={notifPopoverRef}>
                <div className="notif-popover-header">
                  <strong>Notifications</strong>
                  {notifications.length ? <span>{notifications.length}</span> : null}
                </div>
                {notificationsStatus === "loading" ? (
                  <div className="notif-empty"><span className="spinner" aria-hidden="true" />Loading notifications...</div>
                ) : notificationsStatus === "error" ? (
                  <div className="notif-empty">Notifications could not be loaded.</div>
                ) : notifications.length === 0 ? (
                  <div className="notif-empty">No new notifications.</div>
                ) : (
                  <div className="notif-list">
                    {notifications.map((item) => (
                      <a className={`notif-item tone-${item.tone || "info"}`} href={item.url || "/dashboard"} key={item.id} onClick={() => setNotifOpen(false)}>
                        <span className="notif-dot" aria-hidden="true" />
                        <span>
                          <strong>{item.title}</strong>
                          {item.body ? <small>{item.body}</small> : null}
                        </span>
                      </a>
                    ))}
                  </div>
                )}
              </div>
            ) : null}
          </div>
        </header>
        <main className="app-main">{children}</main>
      </div>
    </div>
  );
}
