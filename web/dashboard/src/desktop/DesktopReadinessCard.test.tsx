import {render, screen} from "@testing-library/react";
import {describe, expect, it} from "vitest";
import {DesktopReadinessCard} from "./DesktopReadinessCard";

describe("DesktopReadinessCard", () => {
  it("renders authoritative checks without inferring trust from capabilities", () => {
    const readiness = {
      agent_id: "agent",
      online: true,
      platform: "macos",
      trusted: false,
      capabilities: ["desktop.session.open", "desktop.view"],
      active_session: false,
      reported_at: null,
      checks: {capture: "required", control: "ready", daemon: "ready", host: "ready", service: "ready", indicator: "ready"},
    };
    render(<DesktopReadinessCard readiness={readiness} />);
    expect(screen.getByText("Identity approval required")).toBeInTheDocument();
    expect(screen.getByText("capture: required")).toBeInTheDocument();
    expect(screen.queryByText("Identity trusted")).not.toBeInTheDocument();
  });
});
