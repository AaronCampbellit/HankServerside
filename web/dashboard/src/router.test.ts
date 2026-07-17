import { describe, expect, it } from "vitest";
import { appRoutes, internalNavigationTarget, navItemsForRoutes, routeForPath } from "./router";

describe("router", () => {
  it("resolves known routes and returns a not-found route for unknown paths", () => {
    expect(routeForPath("/dashboard/settings").heading).toBe("Settings");
    expect(routeForPath("/missing").heading).toBe("Not Found");
    expect(routeForPath("/missing").publicRoute).toBeUndefined();
  });

  it("derives shell navigation from route metadata", () => {
    const navItems = navItemsForRoutes(appRoutes);

    expect(navItems.some((item) => item.href === "/dashboard/settings/apps" && item.adminOnly)).toBe(true);
    expect(navItems.some((item) => item.href === "/dashboard/settings/attachments" && item.adminOnly)).toBe(true);
    expect(navItems.some((item) => item.href === "/join")).toBe(false);
  });

  it("intercepts same-origin app links and leaves non-app links alone", () => {
    expect(internalNavigationTarget("/dashboard/settings", "https://hank.local")).toBe("/dashboard/settings");
    expect(internalNavigationTarget("https://hank.local/dashboard?tab=home#top", "https://hank.local")).toBe("/dashboard?tab=home#top");
    expect(internalNavigationTarget("https://example.com/dashboard", "https://hank.local")).toBeNull();
    expect(internalNavigationTarget("mailto:hank@example.com", "https://hank.local")).toBeNull();
  });
});
