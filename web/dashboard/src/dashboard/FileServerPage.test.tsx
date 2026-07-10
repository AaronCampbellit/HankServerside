import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConfirmDialogProvider, ToastProvider } from "../ui/primitives";
import { FileServerPage } from "./FileServerPage";

const fileServerClient = vi.hoisted(() => ({
  list: vi.fn(),
  search: vi.fn(),
  listJobs: vi.fn(),
  subscribeToJobs: vi.fn(),
  onJobsChanged: vi.fn(),
  stat: vi.fn(),
  createDirectory: vi.fn(),
  rename: vi.fn(),
  move: vi.fn(),
  deleteItem: vi.fn(),
  setupDownload: vi.fn(),
  uploadFile: vi.fn(),
}));

const connectionsClient = vi.hoisted(() => ({
  listProfiles: vi.fn(),
}));

vi.mock("../api/fileServer", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/fileServer")>();
  return {
    ...actual,
    fileServerClient,
  };
});

vi.mock("../api/connections", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/connections")>();
  return {
    ...actual,
    connectionsClient,
  };
});

function mockDemoShares() {
  connectionsClient.listProfiles.mockResolvedValue({
    profiles: [
      {
        home_id: "home-demo",
        service_type: "smb",
        public_config_json: JSON.stringify({
          active_source_id: "hankdemo",
          shares: [
            { id: "hankdemo", name: "Hankdemo", host: "192.168.86.137", share: "Hankdemo", username: "Hankdemo" },
            { id: "hankdemo2", name: "Hankdemo2", host: "192.168.86.137", share: "Hankdemo2", username: "Hankdemo" },
          ],
        }),
        secret_version: 1,
        applied_version: 1,
        status: "healthy",
        updated_at: "2026-07-01T12:00:00Z",
        updated_by: "admin",
      },
    ],
  });
}

function renderPage() {
  return render(
    <ToastProvider>
      <ConfirmDialogProvider>
        <FileServerPage />
      </ConfirmDialogProvider>
    </ToastProvider>,
  );
}

describe("FileServerPage", () => {
  beforeEach(() => {
    fileServerClient.listJobs.mockResolvedValue([]);
    fileServerClient.subscribeToJobs.mockResolvedValue({});
    fileServerClient.onJobsChanged.mockReturnValue(() => undefined);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    window.history.pushState({}, "", "/dashboard/file-server");
  });

  it("supports grid/list switching and a dismissible preview pane", async () => {
    mockDemoShares();
    fileServerClient.list.mockResolvedValue({
      path: "/Media/Photos",
      items: [
        { path: "/Media/Photos", name: "Photos", is_directory: true },
        { path: "/Media/Photos/sunset-beach.jpg", name: "sunset-beach.jpg", size: 4404019, modified_at: "2026-06-14T16:47:00Z" },
        { path: "/Media/Photos/birthday-party.mp4", name: "birthday-party.mp4", size: 228589568, modified_at: "2026-06-11T09:12:00Z" },
      ],
    });

    renderPage();

    expect(await screen.findByRole("button", { name: "sunset-beach.jpg" })).toBeInTheDocument();
    const gridButton = screen.getByRole("button", { name: "Grid" });
    expect(gridButton).toBeEnabled();
    fireEvent.click(gridButton);
    expect(screen.getByRole("button", { name: "List" })).toBeEnabled();
    expect(screen.getByLabelText("File grid")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Close preview" }));
    await waitFor(() => expect(screen.queryByLabelText("File preview")).not.toBeInTheDocument());
  });

  it("loads live SMB shares and sends the selected source id", async () => {
    mockDemoShares();
    fileServerClient.list.mockResolvedValue({
      path: "/Media/Photos",
      items: [
        { path: "/Media", name: "Media", is_directory: true },
        { path: "/Media/Photos", name: "Photos", is_directory: true },
        {
          path: "/Media/Photos/sunset-beach.jpg",
          name: "sunset-beach.jpg",
          size: 4404019,
          modified_at: "2026-06-14T16:47:00Z",
          owner: "Aaron D.",
          dimensions: "4032 x 3024",
        },
        { path: "/Media/Photos/reunion-clip.mp4", name: "reunion-clip.mp4", size: 228589568, modified_at: "2026-06-11T09:12:00Z" },
      ],
    });

    renderPage();

    expect(await screen.findByRole("heading", { name: "File Server" })).toBeInTheDocument();
    await waitFor(() => expect(fileServerClient.list).toHaveBeenCalledWith("/", "hankdemo"));
    expect(screen.getByRole("button", { name: /Hankdemo/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Upload" })).toBeEnabled();
    expect(screen.queryByText("nas-attic")).not.toBeInTheDocument();
    expect(screen.queryByText("backups-vault")).not.toBeInTheDocument();
    expect(screen.queryByText("1.85 GB")).not.toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Transfers" })).toBeInTheDocument();
    expect(screen.getByText("No active transfers")).toBeInTheDocument();
    expect(screen.queryByText("1 active")).not.toBeInTheDocument();
    expect(screen.queryByText("4 photos uploaded")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Hankdemo/i }));
    const shareMenu = screen.getByRole("menu", { name: "File shares" });
    expect(within(shareMenu).getByText("Hankdemo")).toBeInTheDocument();
    expect(within(shareMenu).getByText("Hankdemo2")).toBeInTheDocument();
    expect(within(shareMenu).getAllByText("//192.168.86.137/Hankdemo")).toHaveLength(1);
    expect(within(shareMenu).getAllByText("//192.168.86.137/Hankdemo2")).toHaveLength(1);

    fireEvent.click(within(shareMenu).getByRole("menuitem", { name: /Hankdemo2/i }));
    await waitFor(() => expect(fileServerClient.list).toHaveBeenLastCalledWith("/", "hankdemo2"));

    const preview = screen.getByLabelText("File preview");
    expect(within(preview).getByText("sunset-beach.jpg")).toBeInTheDocument();
    expect(within(preview).getByText("/Media/Photos/sunset-beach.jpg")).toBeInTheDocument();
    expect(within(preview).getByRole("img", { name: "Preview sunset-beach.jpg" })).toHaveAttribute("src", "/v1/home/files/preview?source_id=hankdemo2&path=%2FMedia%2FPhotos%2Fsunset-beach.jpg");
    expect(within(preview).getByText("Dimensions")).toBeInTheDocument();
    expect(within(preview).getByText("4032 x 3024")).toBeInTheDocument();
    expect(within(preview).getByText("Owner")).toBeInTheDocument();
    expect(within(preview).getByText("Aaron D.")).toBeInTheDocument();
    expect(within(preview).getByRole("button", { name: "Download preview" })).toBeInTheDocument();
    expect(within(preview).getByRole("button", { name: "Rename preview" })).toBeInTheDocument();
    expect(within(preview).getByRole("button", { name: "Move preview" })).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Select reunion-clip.mp4"));
    expect(screen.getByText("1 selected")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Download selected" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Move selected" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete selected" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Clear selection" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "More actions for sunset-beach.jpg" }));
    const menu = screen.getByRole("menu", { name: "File actions" });
    expect(within(menu).getByRole("menuitem", { name: "Open" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Rename" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Move" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Download" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Delete" })).toBeInTheDocument();
  });

  it("keeps the file pane mounted while opening folders and hides the root sidebar row", async () => {
    mockDemoShares();
    let resolveFolder!: (value: { path: string; items: Array<{ path: string; name: string; is_directory?: boolean; size?: number }> }) => void;
    const folderLoad = new Promise<{ path: string; items: Array<{ path: string; name: string; is_directory?: boolean; size?: number }> }>((resolve) => {
      resolveFolder = resolve;
    });
    fileServerClient.list
      .mockResolvedValueOnce({
        path: "/",
        items: [
          { path: "/Media", name: "Media", is_directory: true },
          { path: "/readme.txt", name: "readme.txt", size: 1200 },
        ],
      })
      .mockReturnValueOnce(folderLoad);

    renderPage();

    expect(await screen.findByRole("button", { name: "readme.txt" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Root/i })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open Media" }));

    expect(screen.queryByRole("heading", { name: "Loading files" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "readme.txt" })).toBeInTheDocument();
    expect(screen.getByText("Opening /Media")).toBeInTheDocument();
    await waitFor(() => expect(fileServerClient.list).toHaveBeenLastCalledWith("/Media", "hankdemo"));

    resolveFolder({
      path: "/Media",
      items: [{ path: "/Media/photo.jpg", name: "photo.jpg", size: 2400 }],
    });

    expect(await screen.findByRole("button", { name: "photo.jpg" })).toBeInTheDocument();
    expect(screen.queryByText("Opening /Media")).not.toBeInTheDocument();
  });

  it("searches the selected share instead of filtering only the open folder", async () => {
    mockDemoShares();
    fileServerClient.list.mockResolvedValue({
      path: "/Current",
      items: [{ path: "/Current/readme.txt", name: "readme.txt", size: 4 }],
    });
    fileServerClient.search.mockResolvedValue({
      items: [{ path: "/Archive/2024/needle.pdf", name: "needle.pdf", size: 42 }],
    });

    renderPage();

    fireEvent.change(await screen.findByLabelText("Search files"), { target: { value: "needle" } });

    await waitFor(() => expect(fileServerClient.search).toHaveBeenCalledWith("needle", "hankdemo"));
    expect(await screen.findByRole("button", { name: "needle.pdf" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "readme.txt" })).not.toBeInTheDocument();
  });

  it("clears search results before switching to another share", async () => {
    mockDemoShares();
    fileServerClient.list
      .mockResolvedValueOnce({
        path: "/",
        items: [{ path: "/needle-local.txt", name: "needle-local.txt", size: 4 }],
      })
      .mockReturnValueOnce(new Promise(() => undefined));
    fileServerClient.search.mockResolvedValue({
      items: [{ path: "/Archive/needle.pdf", name: "needle.pdf", size: 42 }],
    });

    renderPage();

    fireEvent.change(await screen.findByLabelText("Search files"), { target: { value: "needle" } });
    expect(await screen.findByRole("button", { name: "needle.pdf" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Hankdemo/i }));
    fireEvent.click(within(screen.getByRole("menu", { name: "File shares" })).getByRole("menuitem", { name: /Hankdemo2/i }));

    expect(screen.queryByRole("button", { name: "needle.pdf" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Select needle.pdf")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "needle-local.txt" })).not.toBeInTheDocument();
  });

  it("shows active and recently completed file transfer jobs", async () => {
    mockDemoShares();
    fileServerClient.list.mockResolvedValue({
      path: "/Media/Photos",
      items: [
        { path: "/Media/Photos/sunset-beach.jpg", name: "sunset-beach.jpg", size: 4404019, modified_at: "2026-06-14T16:47:00Z" },
      ],
    });
    fileServerClient.listJobs.mockResolvedValue([
      {
        id: "filejob_upload",
        operation: "upload",
        from_path: "/Media/Photos/family.jpg",
        status: "running",
        bytes_done: 512,
        bytes_total: 1024,
        updated_at: "2026-07-03T12:00:00Z",
      },
      {
        id: "filejob_move",
        operation: "move",
        from_path: "/Media/Photos/sunset-beach.jpg",
        to_path: "/Media/Archive/sunset-beach.jpg",
        status: "completed",
        files_done: 1,
        files_total: 1,
        completed_at: "2026-07-03T12:01:00Z",
      },
      {
        id: "filejob_download",
        operation: "download",
        from_path: "/Media/Photos/report.pdf",
        status: "failed",
        error_message: "agent offline",
        updated_at: "2026-07-03T12:02:00Z",
      },
    ]);

    renderPage();

    expect(await screen.findByRole("heading", { name: "Transfers" })).toBeInTheDocument();
    expect(screen.getByText("1 active")).toBeInTheDocument();
    expect(screen.getByText("Upload family.jpg")).toBeInTheDocument();
    expect(screen.getByText("512 B of 1 KB")).toBeInTheDocument();
    expect(screen.getByText("Move sunset-beach.jpg")).toBeInTheDocument();
    expect(screen.getByText("Download report.pdf")).toBeInTheDocument();
    expect(screen.getByText("agent offline")).toBeInTheDocument();
    expect(fileServerClient.subscribeToJobs).toHaveBeenCalled();
    expect(fileServerClient.onJobsChanged).toHaveBeenCalled();
  });

  it("uploads device files into the current folder and refreshes the chosen source", async () => {
    mockDemoShares();
    fileServerClient.list.mockResolvedValue({
      path: "/Media/Photos",
      items: [
        { path: "/Media/Photos/sunset-beach.jpg", name: "sunset-beach.jpg", size: 4404019, modified_at: "2026-06-14T16:47:00Z" },
      ],
    });
    fileServerClient.uploadFile.mockResolvedValue({ ok: true });
    const file = new File(["new image"], "new-photo.jpg", { type: "image/jpeg" });

    renderPage();

    expect(await screen.findByRole("button", { name: "sunset-beach.jpg" })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Choose files to upload"), { target: { files: [file] } });

    await waitFor(() => expect(fileServerClient.uploadFile).toHaveBeenCalledWith(file, "/Media/Photos", "hankdemo"));
    await waitFor(() => expect(fileServerClient.list).toHaveBeenLastCalledWith("/Media/Photos", "hankdemo"));
    expect(await screen.findByText("Uploaded new-photo.jpg.")).toBeInTheDocument();
  });

  it("renames, moves, and downloads files from the preview actions", async () => {
    mockDemoShares();
    fileServerClient.list.mockResolvedValue({
      path: "/Media/Photos",
      items: [
        { path: "/Media/Photos/sunset-beach.jpg", name: "sunset-beach.jpg", size: 4404019, modified_at: "2026-06-14T16:47:00Z" },
        { path: "/Media/Archive", name: "Archive", is_directory: true, modified_at: "2026-06-14T16:47:00Z" },
      ],
    });
    fileServerClient.rename.mockResolvedValue({ ok: true });
    fileServerClient.move.mockResolvedValue({ ok: true, job_id: "filejob_1" });
    fileServerClient.setupDownload.mockResolvedValue({ url: "/v1/file-transfers/download-1" });

    renderPage();

    const preview = await screen.findByLabelText("File preview");
    fireEvent.click(within(preview).getByRole("button", { name: "Download preview" }));
    await waitFor(() => expect(fileServerClient.setupDownload).toHaveBeenCalledWith("/Media/Photos/sunset-beach.jpg", "hankdemo"));

    fireEvent.click(within(preview).getByRole("button", { name: "Rename preview" }));
    fireEvent.change(await screen.findByLabelText("File name"), { target: { value: "sunrise.jpg" } });
    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    await waitFor(() => expect(fileServerClient.rename).toHaveBeenCalledWith("/Media/Photos/sunset-beach.jpg", "/Media/Photos/sunrise.jpg", "hankdemo"));

    fireEvent.click(within(preview).getByRole("button", { name: "Move preview" }));
    fireEvent.change(await screen.findByLabelText("Destination path"), { target: { value: "/Media/Archive" } });
    fireEvent.click(screen.getByRole("button", { name: "Move here" }));
    await waitFor(() => expect(fileServerClient.move).toHaveBeenCalledWith("/Media/Photos/sunset-beach.jpg", "/Media/Archive/sunset-beach.jpg", false, "hankdemo", "hankdemo"));
  });
});
