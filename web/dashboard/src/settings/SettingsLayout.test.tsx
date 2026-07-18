import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { SettingsLayout } from "./SettingsLayout";

describe("SettingsLayout", () => {
  afterEach(() => cleanup());

  it("renders the guide settings rail with admin sections marked", () => {
    render(
      <SettingsLayout currentPath="/dashboard/settings/apps" isAdmin>
        <section aria-label="Current settings page">Apps page</section>
      </SettingsLayout>,
    );

    const nav = screen.getByRole("navigation", { name: "Settings sections" });
    expect(nav).toHaveClass("settings-subnav");
    expect(within(nav).getByText("Admin")).toBeInTheDocument();
    expect(within(nav).getByText("Membership")).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Home" })).toHaveAttribute("href", "/dashboard/settings/home");
    expect(within(nav).getByRole("link", { name: "Quick Links" })).toHaveAttribute("href", "/dashboard/settings/quick-links");
    expect(within(nav).getByRole("link", { name: "People" })).toHaveAttribute("href", "/dashboard/settings/people");
    expect(within(nav).getByRole("link", { name: "Connections" })).toHaveAttribute("href", "/dashboard/settings/connections");
    expect(within(nav).getByRole("link", { name: "AI" })).toHaveAttribute("href", "/dashboard/settings/ai");
    const appsLink = within(nav).getByRole("link", { name: "Apps ADMIN" });
    expect(appsLink).toHaveAttribute("aria-current", "page");
    expect(appsLink.querySelector(".settings-tab-icon")).not.toBeNull();
    expect(within(nav).getByRole("link", { name: "Backups ADMIN" })).toHaveAttribute("href", "/dashboard/settings/backups");
    expect(within(nav).getByRole("link", { name: "Attachments ADMIN" })).toHaveAttribute("href", "/dashboard/settings/attachments");
    expect(within(nav).getByRole("link", { name: "Recovery ADMIN" })).toHaveAttribute("href", "/dashboard/settings/recovery");
    expect(within(nav).getByRole("link", { name: "Logs ADMIN" })).toHaveAttribute("href", "/dashboard/settings/logs");
    expect(within(nav).getByRole("link", { name: "Join Home" })).toHaveAttribute("href", "/dashboard/settings/join-home");
  });

  it("keeps admin-only guide sections hidden for members", () => {
    render(
      <SettingsLayout currentPath="/dashboard/settings/home" isAdmin={false}>
        <section aria-label="Current settings page">Home page</section>
      </SettingsLayout>,
    );

    const nav = screen.getByRole("navigation", { name: "Settings sections" });
    expect(within(nav).queryByRole("link", { name: "Apps ADMIN" })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Backups ADMIN" })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Attachments ADMIN" })).not.toBeInTheDocument();
    expect(within(nav).queryByText("Admin")).not.toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
  });

  it("treats the settings root as the Home settings tab", () => {
    render(
      <SettingsLayout currentPath="/dashboard/settings" isAdmin>
        <section aria-label="Current settings page">Home page</section>
      </SettingsLayout>,
    );

    const nav = screen.getByRole("navigation", { name: "Settings sections" });
    expect(within(nav).getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
  });

  it("renders a permission-filtered mobile section chooser", () => {
    render(
      <SettingsLayout currentPath="/dashboard/settings/people" isAdmin={false}>
        <section>People page</section>
      </SettingsLayout>,
    );

    const chooser = screen.getByRole("group", { name: "Mobile settings section" });
    expect(chooser.querySelector("summary strong")).toHaveTextContent("People");
    expect(within(chooser).getByRole("link", { name: "Home" })).toBeInTheDocument();
    expect(within(chooser).queryByRole("link", { name: "Apps ADMIN" })).not.toBeInTheDocument();
  });
});
