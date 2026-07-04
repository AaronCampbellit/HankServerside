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
});
