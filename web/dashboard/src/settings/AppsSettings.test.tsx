import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppsSettings } from "./AppsSettings";

const bootstrapClient = vi.hoisted(() => ({ load: vi.fn() }));
const appsClient = vi.hoisted(() => ({
  listApps: vi.fn(),
  saveConfig: vi.fn(),
  previewPackage: vi.fn(),
  activatePackage: vi.fn(),
}));

vi.mock("../api/bootstrap", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/bootstrap")>()),
  bootstrapClient,
}));

vi.mock("../api/apps", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/apps")>()),
  appsClient,
}));

describe("AppsSettings search targets", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    window.history.replaceState({}, "", "/dashboard/settings/apps");
  });

  it("selects the exact app targeted by global search", async () => {
    window.history.replaceState({}, "", "/dashboard/settings/apps?app=weather");
    bootstrapClient.load.mockResolvedValue({
      permissions: { can_manage_apps: true },
    });
    appsClient.listApps.mockResolvedValue({
      apps: [
        { app_id: "notes", name: "Notes", version: "1.0.0", enabled: true },
        { app_id: "weather", name: "Weather", version: "1.0.0", enabled: true },
      ],
    });

    render(<AppsSettings />);

    expect(await screen.findByLabelText("Installed app")).toHaveValue("weather");
    expect(screen.getByText("Weather")).toBeInTheDocument();
  });
});
