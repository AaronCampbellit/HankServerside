import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Shell } from "./ui/Shell";
import { primaryNav } from "./ui/navConfig";
import { SettingsLayout } from "./settings/SettingsLayout";
import { authClient } from "./api/auth";
import { ConfirmDialogProvider, ToastProvider } from "./ui/primitives";
import { bootstrapClient, type BootstrapState } from "./api/bootstrap";
import { ApiError } from "./api/client";
import { JoinPage } from "./auth/JoinPage";
import { LoginPage } from "./auth/LoginPage";
import { PasswordChangePage } from "./auth/PasswordChangePage";
import { redirectTo } from "./browser/navigation";
import { DashboardHome } from "./dashboard/DashboardHome";
import { DeploymentGuide } from "./dashboard/DeploymentGuide";
import { FileServerPage } from "./dashboard/FileServerPage";
import { HankAIPage } from "./dashboard/HankAIPage";
import { HomeAssistantPage } from "./dashboard/HomeAssistantPage";
import { ProfileNotesPage } from "./dashboard/ProfileNotesPage";
import { AssistantSettings } from "./settings/AssistantSettings";
import { AppsSettings } from "./settings/AppsSettings";
import { BackupsSettings } from "./settings/BackupsSettings";
import { ConnectionsSettings } from "./settings/ConnectionsSettings";
import { HomeSettings } from "./settings/HomeSettings";
import { JoinHomeSettings } from "./settings/JoinHomeSettings";
import { LogsSettings } from "./settings/LogsSettings";
import { PeopleSettings } from "./settings/PeopleSettings";
import { QuickLinksSettings } from "./settings/QuickLinksSettings";
import { RecoverySettings } from "./settings/RecoverySettings";
import { routeForPath, type RouteDefinition } from "./router";

function RouteStub({ route }: { route: RouteDefinition }) {
  return (
    <section className="route-panel" aria-labelledby="route-title">
      <div>
        <p className="eyebrow">Hank Remote</p>
        <h1 id="route-title">{route.heading}</h1>
      </div>
    </section>
  );
}

function PublicRoute({ route }: { route: RouteDefinition }) {
  if (route.path === "/") return <LoginPage />;
  if (route.path === "/join") return <JoinPage />;
  if (route.path === "/password-change") return <PasswordChangePage />;
  return <RouteStub route={route} />;
}

function currentPathname(): string {
  return window.location.pathname;
}

function initialMountedRoutePaths(): string[] {
  const route = routeForPath(currentPathname());
  return route.publicRoute ? [] : [route.path];
}

function appendMountedRoutePath(paths: string[], path: string): string[] {
  return paths.includes(path) ? paths : [...paths, path];
}

// Resolve the page element for a route path (settings pages included).
function pageForRoute(route: RouteDefinition): ReactNode {
  switch (route.path) {
    case "/dashboard": return <DashboardHome />;
    case "/dashboard/hank": return <HankAIPage />;
    case "/docs/deployment": return <DeploymentGuide />;
    case "/dashboard/home-assistant": return <HomeAssistantPage />;
    case "/dashboard/file-server": return <FileServerPage />;
    case "/dashboard/profile-notes": return <ProfileNotesPage />;
    case "/dashboard/settings": return <HomeSettings />;
    case "/dashboard/settings/home": return <HomeSettings />;
    case "/dashboard/settings/people": return <PeopleSettings />;
    case "/dashboard/settings/ai": return <AssistantSettings />;
    case "/dashboard/settings/apps": return <AppsSettings />;
    case "/dashboard/settings/backups": return <BackupsSettings />;
    case "/dashboard/settings/connections": return <ConnectionsSettings />;
    case "/dashboard/settings/join-home": return <JoinHomeSettings />;
    case "/dashboard/settings/logs": return <LogsSettings />;
    case "/dashboard/settings/quick-links": return <QuickLinksSettings />;
    case "/dashboard/settings/recovery": return <RecoverySettings />;
    default: return <RouteStub route={route} />;
  }
}

export function App() {
  const [pathname, setPathname] = useState(currentPathname);
  const [bootstrap, setBootstrap] = useState<BootstrapState | null>(null);
  const [mountedRoutePaths, setMountedRoutePaths] = useState<string[]>(initialMountedRoutePaths);
  const route = routeForPath(pathname);

  useEffect(() => {
    function handlePopState() {
      setPathname(currentPathname());
    }
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  useEffect(() => {
    if (route.publicRoute) {
      setBootstrap(null);
      return;
    }
    if (bootstrap) return;
    let alive = true;
    bootstrapClient.load()
      .then((payload) => {
        if (payload.user.password_change_required && currentPathname() !== "/password-change") {
          redirectTo("/password-change");
          return;
        }
        if (alive) setBootstrap(payload);
      })
      .catch((error) => {
        if (alive && error instanceof ApiError && error.status === 401) redirectTo("/?expired=1");
        if (alive) setBootstrap(null);
      });
    return () => {
      alive = false;
    };
  }, [bootstrap, route.publicRoute]);

  useEffect(() => {
    if (route.publicRoute) {
      setMountedRoutePaths([]);
      return;
    }
    setMountedRoutePaths((paths) => appendMountedRoutePath(paths, route.path));
  }, [route.path, route.publicRoute]);

  function navigateTo(href: string) {
    const url = new URL(href, window.location.origin);
    const nextPath = url.pathname;
    if (nextPath === pathname && url.search === window.location.search && url.hash === window.location.hash) {
      return;
    }
    window.history.pushState({}, "", `${url.pathname}${url.search}${url.hash}`);
    setPathname(nextPath);
  }

  function prefetchRoute(href: string) {
    let url: URL;
    try {
      url = new URL(href, window.location.origin);
    } catch {
      return;
    }
    const targetRoute = routeForPath(url.pathname);
    if (targetRoute.publicRoute) return;
    setMountedRoutePaths((paths) => appendMountedRoutePath(paths, targetRoute.path));
  }

  const isAdmin = Boolean(bootstrap?.permissions?.is_admin);
  const visibleNavItems = useMemo(() => primaryNav, []);

  async function logout() {
    try {
      await authClient.logout();
    } finally {
      redirectTo("/");
    }
  }

  function renderCachedRoute(path: string) {
    const cachedRoute = routeForPath(path);
    const page = pageForRoute(cachedRoute);
    return cachedRoute.path.startsWith("/dashboard/settings") ? (
      <SettingsLayout currentPath={cachedRoute.path} isAdmin={isAdmin} onPrefetch={prefetchRoute}>
        {page}
      </SettingsLayout>
    ) : page;
  }

  const visibleRoutePaths = route.publicRoute ? [] : appendMountedRoutePath(mountedRoutePaths, route.path);
  const cachedRouteContent = (
    <>
      {visibleRoutePaths.map((path) => {
        const page = renderCachedRoute(path);
        const hidden = path !== route.path;
        return (
          <div aria-hidden={hidden ? "true" : undefined} className="route-cache-panel" hidden={hidden} key={path}>
            {page}
          </div>
        );
      })}
    </>
  );

  const content = route.publicRoute ? (
    <main className="auth-surface">
      <PublicRoute route={route} />
    </main>
  ) : (
    <Shell
      navItems={visibleNavItems}
      currentPath={route.path}
      onNavigate={navigateTo}
      onPrefetch={prefetchRoute}
      onLogout={() => void logout()}
      userEmail={bootstrap?.user.email}
      userRole={bootstrap?.membership?.role}
      connectorOnline={bootstrap?.agent?.status?.toLowerCase() === "online"}
    >
      {cachedRouteContent}
    </Shell>
  );

  return (
    <ToastProvider>
      <ConfirmDialogProvider>
        {content}
      </ConfirmDialogProvider>
    </ToastProvider>
  );
}
