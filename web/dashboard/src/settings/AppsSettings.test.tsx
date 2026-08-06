import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppsSettings } from "./AppsSettings";

const bootstrapClient = vi.hoisted(() => ({ load: vi.fn() }));
const appsClient = vi.hoisted(() => ({
  listApps: vi.fn(),
  saveConfig: vi.fn(),
  previewPackage: vi.fn(),
  activatePackage: vi.fn(),
}));
const loadPrimaryFileTargets = vi.hoisted(() => vi.fn());

vi.mock("../api/bootstrap", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/bootstrap")>()),
  bootstrapClient,
}));

vi.mock("../api/apps", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/apps")>()),
  appsClient,
}));

vi.mock("../dashboard/fileServerTargets", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../dashboard/fileServerTargets")>()),
  loadPrimaryFileTargets,
}));

describe("AppsSettings search targets", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    loadPrimaryFileTargets.mockResolvedValue([]);
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

  it("renders independent file sources and clears only the changed source path", async () => {
    bootstrapClient.load.mockResolvedValue({ permissions: { can_manage_apps: true } });
    loadPrimaryFileTargets.mockResolvedValue([
      { key: "primary:movies", sourceID: "movies", agentID: "", name: "Movies share", detail: "//nas/movies", kind: "smb" },
      { key: "primary:tv", sourceID: "tv", agentID: "", name: "TV share", detail: "//nas/tv", kind: "smb" },
    ]);
    appsClient.listApps.mockResolvedValue({
      apps: [{
        app_id: "gramaton",
        name: "Gramaton",
        enabled: true,
        public_config: {
          movie_source_id: "movies",
          movie_destination_path: "/Library/Movies",
          tv_source_id: "tv",
          tv_destination_path: "/Library/TV",
        },
        settings_schema: {
          fields: [
            { key: "movie_source_id", label: "Movie source", type: "select", source: "file_sources", required: true },
            { key: "movie_destination_path", label: "Movie destination", type: "path", source_field: "movie_source_id", required: true },
            { key: "tv_source_id", label: "TV source", type: "select", source: "file_sources", required: true },
            { key: "tv_destination_path", label: "TV destination", type: "path", source_field: "tv_source_id", required: true },
          ],
        },
      }],
    });

    render(<AppsSettings />);
    fireEvent.click(await screen.findByRole("button", { name: "Configure Gramaton" }));

    expect(screen.getByLabelText("Movie source")).toHaveValue("movies");
    expect(screen.getByLabelText("TV source")).toHaveValue("tv");
    expect(screen.getByRole("button", { name: "Choose Movie destination folder" })).toHaveTextContent("/Library/Movies");
    expect(screen.getByRole("button", { name: "Choose TV destination folder" })).toHaveTextContent("/Library/TV");

    fireEvent.change(screen.getByLabelText("Movie source"), { target: { value: "tv" } });

    expect(screen.getByRole("button", { name: "Choose Movie destination folder" })).toHaveTextContent("Choose folder");
    expect(screen.getByRole("button", { name: "Choose TV destination folder" })).toHaveTextContent("/Library/TV");
    expect(screen.getByRole("button", { name: "Save app configuration" })).toBeDisabled();
  });

  it("renders static selects and keeps unbound paths as text fields", async () => {
    bootstrapClient.load.mockResolvedValue({ permissions: { can_manage_apps: true } });
    appsClient.listApps.mockResolvedValue({
      apps: [{
        app_id: "sample",
        name: "Sample",
        settings_schema: {
          fields: [
            { key: "quality", label: "Quality", type: "select", options: [{ value: "hd", label: "HD" }, { value: "uhd", label: "4K" }] },
            { key: "cache_path", label: "Cache path", type: "path" },
          ],
        },
      }],
    });

    render(<AppsSettings />);
    fireEvent.click(await screen.findByRole("button", { name: "Configure Sample" }));

    expect(screen.getByLabelText("Quality")).toHaveDisplayValue("HD");
    expect(screen.getByLabelText("Cache path")).toHaveAttribute("type", "text");
    fireEvent.change(screen.getByLabelText("Quality"), { target: { value: "uhd" } });
    fireEvent.click(screen.getByRole("button", { name: "Save app configuration" }));

    await waitFor(() => expect(appsClient.saveConfig).toHaveBeenCalledWith("sample", expect.objectContaining({
      public_config: expect.objectContaining({ quality: "uhd" }),
    })));
  });
});
