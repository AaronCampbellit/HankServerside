export type RouteDefinition = {
  path: string;
  heading: string;
  nav?: boolean;
  adminOnly?: boolean;
  publicRoute?: boolean;
};

export type NavItem = {
  href: string;
  label: string;
  adminOnly?: boolean;
};

export const appRoutes: RouteDefinition[] = [
  { path: "/", heading: "Sign in to Hank", publicRoute: true },
  { path: "/join", heading: "Join Home", publicRoute: true },
  { path: "/password-change", heading: "Change Password", publicRoute: true },
  { path: "/dashboard", heading: "Dashboard", nav: true },
  { path: "/dashboard/hank", heading: "HankAI", nav: true },
  { path: "/dashboard/home-assistant", heading: "Home Assistant", nav: true },
  { path: "/dashboard/profile-notes", heading: "Profile Notes", nav: true },
  { path: "/dashboard/file-server", heading: "File Server", nav: true },
  { path: "/dashboard/agents", heading: "Agents", nav: true },
  { path: "/dashboard/settings", heading: "Settings", nav: true },
  { path: "/dashboard/settings/home", heading: "Home & Connector", nav: true },
  { path: "/dashboard/settings/quick-links", heading: "Quick Links", nav: true },
  { path: "/dashboard/settings/people", heading: "People", nav: true },
  { path: "/dashboard/settings/connections", heading: "Connections", nav: true },
  { path: "/dashboard/settings/ai", heading: "AI & MCP", nav: true },
  { path: "/dashboard/settings/apps", heading: "Apps", nav: true, adminOnly: true },
  { path: "/dashboard/settings/attachments", heading: "Attachments", nav: true, adminOnly: true },
  { path: "/dashboard/settings/backups", heading: "Backups & Storage", nav: true, adminOnly: true },
  { path: "/dashboard/settings/recovery", heading: "Recovery", nav: true, adminOnly: true },
  { path: "/dashboard/settings/logs", heading: "Logs", nav: true, adminOnly: true },
  { path: "/dashboard/settings/join-home", heading: "Join Home", nav: true },
  { path: "/docs/deployment", heading: "Deployment", nav: true },
];

const routeByPath = new Map(appRoutes.map((route) => [route.path, route]));

export function routeForPath(pathname: string): RouteDefinition {
  return routeByPath.get(pathname) ?? { path: pathname, heading: "Not Found" };
}

export function navItemsForRoutes(routes: RouteDefinition[] = appRoutes): NavItem[] {
  return routes
    .filter((route) => route.nav)
    .map((route) => ({
      href: route.path,
      label: route.heading,
      adminOnly: route.adminOnly,
    }));
}

export function internalNavigationTarget(href: string, origin = window.location.origin): string | null {
  let url: URL;
  try {
    url = new URL(href, origin);
  } catch {
    return null;
  }
  if (url.origin !== origin || !["http:", "https:"].includes(url.protocol)) {
    return null;
  }
  return `${url.pathname}${url.search}${url.hash}`;
}
