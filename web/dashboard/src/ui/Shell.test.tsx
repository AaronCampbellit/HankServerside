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
});
