import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Shell } from "./Shell";

describe("Shell", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("loads and renders notification feed items", async () => {
    vi.stubGlobal("fetch", vi.fn<typeof fetch>(async (input) => {
      if (String(input) === "/v1/home/notifications") {
        return new Response(JSON.stringify({
          notifications: [{
            id: "agent-offline",
            title: "Connector offline",
            body: "Kitchen Mac has not checked in.",
            tone: "warning",
            url: "/dashboard/settings/home",
            created_at: "2026-06-28T00:00:00Z",
          }],
        }), { headers: { "Content-Type": "application/json" } });
      }
      return new Response("not found", { status: 404 });
    }));

    render(
      <Shell
        navItems={[{ href: "/dashboard", label: "Home", group: "Main" }]}
        currentPath="/dashboard"
        onNavigate={vi.fn()}
        onLogout={vi.fn()}
      >
        <div>Dashboard content</div>
      </Shell>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Notifications" }));

    expect(await screen.findByRole("dialog", { name: "Notifications" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("Connector offline")).toBeInTheDocument());
    expect(screen.getByText("Kitchen Mac has not checked in.")).toBeInTheDocument();
    expect(screen.queryByText("No new notifications.")).not.toBeInTheDocument();
  });

  it("closes the notifications popover when clicking outside it", async () => {
    vi.stubGlobal("fetch", vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ notifications: [] }), { headers: { "Content-Type": "application/json" } })));

    render(
      <Shell
        navItems={[{ href: "/dashboard", label: "Home", group: "Main" }]}
        currentPath="/dashboard"
        onNavigate={vi.fn()}
        onLogout={vi.fn()}
      >
        <div>Dashboard content</div>
      </Shell>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Notifications" }));
    expect(await screen.findByRole("dialog", { name: "Notifications" })).toBeInTheDocument();

    fireEvent.pointerDown(screen.getByText("Dashboard content"));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Notifications" })).not.toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Notifications" }));
    expect(await screen.findByRole("dialog", { name: "Notifications" })).toBeInTheDocument();

    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Notifications" })).not.toBeInTheDocument());
  });

  it("renders the reference sidebar status and user footer", () => {
    render(
      <Shell
        navItems={[{ href: "/dashboard", label: "Home", group: "Main" }]}
        currentPath="/dashboard"
        onNavigate={vi.fn()}
        onLogout={vi.fn()}
      >
        <div>Dashboard content</div>
      </Shell>,
    );

    const footer = screen.getByLabelText("Session status");
    expect(footer).toHaveClass("nav-footer");
    expect(within(footer).getByText("Connector online")).toBeInTheDocument();
    expect(within(footer).getByText("AD")).toBeInTheDocument();
    expect(within(footer).getByText("admin")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out" })).toHaveClass("nav-footer-signout");
  });

  it("orders mobile header actions as search, notifications, then menu", () => {
    render(
      <Shell
        navItems={[{ href: "/dashboard", label: "Home", group: "Main" }]}
        currentPath="/dashboard"
        onNavigate={vi.fn()}
        onLogout={vi.fn()}
      >
        <div>Dashboard content</div>
      </Shell>,
    );

    const actions = within(screen.getByRole("banner"))
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label"));

    expect(actions).toEqual(["Open search", "Notifications", "Open menu"]);
  });

  it("partitions daily mobile routes from the overflow menu", () => {
    render(
      <Shell
        navItems={[
          { href: "/dashboard", label: "Home" },
          { href: "/dashboard/hank", label: "Hank" },
          { href: "/dashboard/profile-notes", label: "Notes" },
          { href: "/dashboard/home-assistant", label: "Home Assistant" },
          { href: "/dashboard/file-server", label: "File Server" },
          { href: "/dashboard/agents", label: "Agents" },
          { href: "/dashboard/settings", label: "Settings" },
          { href: "/docs/deployment", label: "Setup Guide" },
        ]}
        currentPath="/dashboard/profile-notes"
        onNavigate={vi.fn()}
        onLogout={vi.fn()}
      >
        <div>Notes content</div>
      </Shell>,
    );

    const primary = screen.getByRole("navigation", { name: "Mobile primary" });
    expect(within(primary).getByRole("link", { name: "Notes" })).toHaveAttribute("aria-current", "page");
    expect(within(primary).getAllByRole("link")).toHaveLength(5);

    fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
    const menu = screen.getByRole("dialog", { name: "Mobile menu" });
    expect(within(menu).getByRole("link", { name: "Agents" })).toBeInTheDocument();
    expect(within(menu).getByRole("link", { name: "Settings" })).toBeInTheDocument();
    expect(within(menu).getByRole("link", { name: "Setup Guide" })).toBeInTheDocument();
  });

  it("dismisses mobile overlays with Escape and restores focus", () => {
    render(
      <Shell
        navItems={[{ href: "/dashboard", label: "Home" }]}
        currentPath="/dashboard"
        onNavigate={vi.fn()}
        onLogout={vi.fn()}
      >
        <div>Home</div>
      </Shell>,
    );

    const menuButton = screen.getByRole("button", { name: "Open menu" });
    fireEvent.click(menuButton);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Mobile menu" })).not.toBeInTheDocument();
    expect(menuButton).toHaveFocus();

    const searchButton = screen.getByRole("button", { name: "Open search" });
    fireEvent.click(searchButton);
    expect(screen.getByRole("button", { name: "Close search" })).toBeInTheDocument();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(searchButton).toHaveFocus();
  });
});
