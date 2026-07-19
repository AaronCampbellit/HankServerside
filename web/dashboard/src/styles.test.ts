import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const styles = readFileSync("src/styles.css", "utf8");

function ruleBodies(selector: string): string[] {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return [...styles.matchAll(new RegExp(`${escaped}\\s*\\{(?<body>[^}]+)\\}`, "g"))].map((match) => match.groups?.body || "");
}

function lastRuleBody(selector: string): string {
  return ruleBodies(selector).at(-1) || "";
}

describe("dashboard stylesheet", () => {
  it("keeps the expanded desktop sidebar slim", () => {
    expect(styles).toContain("--side-nav-width: 204px");
  });

  it("uses the full desktop workspace without an outer page card", () => {
    const routeCanvases = ruleBodies(".app-main > .route-cache-panel > .dashboard-page:not(.home-dashboard)");
    const notesWorkspace = lastRuleBody(".notes-guide-layout");

    expect(styles).toContain("/* Productive edge-to-edge desktop workspace */");
    expect(routeCanvases.some((body) => body.includes("min-height: calc(100vh - 56px)") && body.includes("padding: 0") && body.includes("max-width: none"))).toBe(true);
    expect(notesWorkspace).toContain("border: 0");
    expect(notesWorkspace).toContain("border-radius: 0");
  });

  it("keeps the fixed dashboard shell scrollable inside the content pane", () => {
    expect(ruleBodies(".app-content").some((body) => body.includes("height: 100%"))).toBe(true);
    expect(ruleBodies(".app-content").some((body) => body.includes("display: flex") && body.includes("flex-direction: column"))).toBe(true);
    expect(ruleBodies(".app-main").some((body) => body.includes("flex: 1 1 auto") && body.includes("overflow-y: auto"))).toBe(true);
    expect(styles).toContain("grid-template-rows: minmax(0, 1fr)");
  });

  it("keeps settings subnavigation as the grouped reference rail", () => {
    expect(ruleBodies(".settings-layout").some((body) => body.includes("grid-template-columns: 230px minmax(0, 1fr)"))).toBe(true);
    expect(ruleBodies(".settings-subnav").some((body) => body.includes("position: sticky") && body.includes("border-right: 1px solid var(--line)"))).toBe(true);
    expect(styles).toContain(".settings-subnav-group");
    expect(styles).toContain(".settings-tab-icon");
  });

  it("gives non-home dashboard routes their own content padding", () => {
    expect(styles).toContain(".app-main > .dashboard-page:not(.home-dashboard)");
    expect(styles).toContain("--route-page-x-padding: 22px");
    expect(styles).toContain("padding: 26px var(--route-page-x-padding) 40px");
    expect(ruleBodies(".home-dashboard").some((body) => body.includes("padding: 26px var(--route-page-x-padding) 40px"))).toBe(true);
    expect(ruleBodies(".settings-content").some((body) => body.includes("padding: 26px 30px 42px"))).toBe(true);
  });

  it("keeps file server table content contained inside the list pane", () => {
    expect(styles).toContain(".file-guide-table");
    expect(ruleBodies(".file-list-scroll").some((body) => body.includes("overflow: auto"))).toBe(true);
    expect(ruleBodies(".file-guide-table").some((body) => body.includes("min-width: 560px"))).toBe(true);
  });

  it("layers confirmation dialogs above the Kanban card modal", () => {
    const confirmZIndex = Number(ruleBodies(".confirm-scrim").at(0)?.match(/z-index:\s*(\d+)/)?.[1]);
    const kanbanZIndex = Number(ruleBodies(".kanban-card-modal-backdrop").at(0)?.match(/z-index:\s*(\d+)/)?.[1]);

    expect(confirmZIndex).toBeGreaterThan(kanbanZIndex);
  });

  it("keeps Kanban card editing chrome compact", () => {
    expect(ruleBodies(".kanban-card-modal").at(0)).toContain("min-height: min(520px");
    expect(ruleBodies(".kanban-card-modal-header").at(0)).toContain("padding: 12px 14px 10px 16px");
    expect(ruleBodies(".kanban-formatbar").at(0)).toContain("padding: 2px");
    expect(ruleBodies(".kanban-upload").at(0)).toContain("min-height: 44px");
  });

  it("defines the authoritative safe-area mobile shell", () => {
    expect(styles).toContain("/* Mobile responsive pass */");
    expect(styles).toContain("--mobile-bottom-nav-height: 68px");
    expect(styles).toContain("grid-template-rows: auto minmax(0, 1fr)");
    expect(styles).toContain("env(safe-area-inset-bottom)");
    expect(styles).toContain("min-height: 100dvh");
    expect(styles).toContain("max-height: calc(100dvh - 16px)");
  });

  it("defines touch-sized Notes and snapping Kanban behavior", () => {
    expect(styles).toContain("scroll-snap-type: x mandatory");
    expect(styles).toContain("grid-auto-columns: min(86vw, 340px)");
    expect(styles).toContain(".notes-toolbar .icon-button");
    expect(styles).toContain(".kanban-card-modal-scroll");
  });

  it("keeps mobile topbar actions visually subordinate to page actions", () => {
    expect(ruleBodies(".mobile-topbar-action").some((body) => body.includes("background: transparent") && body.includes("color: var(--ink)"))).toBe(true);
    expect(styles).toContain('.app-shell[data-mobile-search-open="true"] .mobile-topbar-title');
    expect(styles).toContain("visibility: hidden");
  });

  it("removes the desktop Home Assistant table minimum width on mobile", () => {
    expect(lastRuleBody(".ha-entities-table")).toContain("min-width: 0");
  });

  it("defines task-focused mobile workspaces for the primary routes", () => {
    expect(styles).toContain("/* Task-focused mobile workspaces */");
    expect(styles).toContain('.notes-guide-layout[data-mobile-pane="browser"] .notes-guide-editor');
    expect(styles).toContain('.notes-guide-layout[data-mobile-pane="editor"] .notes-guide-rail');
    expect(styles).toContain(".ha-mobile-results-footer");
    expect(styles).toContain("max-height: 76px");
    expect(styles).toContain(".file-tree-pane");
    expect(styles).toContain(".file-activity-region.is-mobile-collapsed");
    expect(styles).toContain(".home-mobile-services-toggle");
    expect(styles).toContain("min-height: calc(100dvh");
  });

  it("centers the mobile file toolbar icons inside their compact buttons", () => {
    const mobileFileAction = lastRuleBody(".file-guide-actions button");
    expect(mobileFileAction).toContain("font-size: 0");
    expect(mobileFileAction).toContain("gap: 0");
    expect(mobileFileAction).toContain("justify-content: center");
  });
});
