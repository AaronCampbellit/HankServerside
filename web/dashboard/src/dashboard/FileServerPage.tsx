import { useEffect, useMemo, useRef, useState } from "react";
import { connectionsClient, type ServiceProfile } from "../api/connections";
import { childPath, fileServerClient, type FileEntry, type FileOperationJob } from "../api/fileServer";
import { useToast } from "../ui/primitives";

type FileMeta = FileEntry & {
  dimensions?: string;
  dims?: string;
  owner?: string;
  type?: string;
};

type FileSource = {
  id: string;
  name: string;
  detail: string;
};

type FileDialog =
  | { kind: "folder" }
  | { kind: "delete"; items: FileMeta[] }
  | { kind: "rename"; item: FileMeta }
  | { kind: "move"; items: FileMeta[]; destinationPath: string; destinationSourceID: string };

type State =
  | { status: "loading"; path: string }
  | { status: "error"; path: string; message: string }
  | {
      status: "ready";
      path: string;
      items: FileMeta[];
      query: string;
      message: string;
      viewMode: "list" | "grid";
      selectedPaths: string[];
      previewPath: string;
      previewOpen: boolean;
      sharePickerOpen: boolean;
      menuPath: string;
      dialog: FileDialog | null;
      dialogDraft: string;
      refreshingPath?: string;
    };

type TransferJobsState =
  | { status: "loading"; jobs: FileOperationJob[]; message?: string }
  | { status: "ready"; jobs: FileOperationJob[]; message?: string }
  | { status: "error"; jobs: FileOperationJob[]; message: string };

function fileName(item: FileEntry): string {
  return item.name || item.path.split("/").filter(Boolean).pop() || "/";
}

function formatSize(item: FileEntry): string {
  if (item.is_directory) return "—";
  const size = item.size || 0;
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function formatBytes(value: number | null | undefined): string {
  const bytes = Number(value || 0);
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function formatModified(item: FileEntry): string {
  if (!item.modified_at) return "Unknown";
  const date = new Date(item.modified_at);
  if (Number.isNaN(date.getTime())) return "Unknown";
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatJobTime(value?: string | null): string {
  if (!value) return "Just now";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Just now";
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function fileType(item: FileMeta): string {
  if (item.is_directory) return "Folder";
  if (item.type) return item.type;
  const ext = fileName(item).split(".").pop()?.toLowerCase() || "";
  if (["png", "jpg", "jpeg", "gif", "webp", "svg", "heic"].includes(ext)) return ext === "jpg" || ext === "jpeg" ? "JPEG image" : "Image";
  if (["mp4", "mov", "mkv", "avi", "webm"].includes(ext)) return "Video";
  if (["mp3", "wav", "flac", "aac", "m4a"].includes(ext)) return "Audio";
  if (["zip", "tar", "gz", "7z", "rar"].includes(ext)) return "Archive";
  if (["pdf", "doc", "docx", "txt", "md", "rtf"].includes(ext)) return "Document";
  return "File";
}

function fileIcon(item: FileMeta): string {
  if (item.is_directory) return "folder";
  const ext = fileName(item).split(".").pop()?.toLowerCase() || "";
  if (["png", "jpg", "jpeg", "gif", "webp", "svg", "heic"].includes(ext)) return "image";
  if (["mp4", "mov", "mkv", "avi", "webm"].includes(ext)) return "video";
  if (["mp3", "wav", "flac", "aac", "m4a"].includes(ext)) return "audio";
  if (["zip", "tar", "gz", "7z", "rar"].includes(ext)) return "archive";
  return "file";
}

function fileIconTone(item: FileMeta): string {
  switch (fileIcon(item)) {
    case "folder": return "var(--brand-dark)";
    case "image": return "#6fd3a8";
    case "video": return "#c98ad8";
    case "audio": return "#f0bd6b";
    case "archive": return "#f08a8a";
    default: return "var(--muted)";
  }
}

function jobDisplayPath(job: FileOperationJob): string {
  if (job.operation === "move") return job.to_path || job.from_path || job.path || "/";
  return job.path || job.from_path || job.to_path || "/";
}

function jobTitle(job: FileOperationJob): string {
  const raw = jobDisplayPath(job).split("/").filter(Boolean).pop() || jobDisplayPath(job);
  const name = raw || "file";
  switch (job.operation) {
    case "download": return `Download ${name}`;
    case "upload": return `Upload ${name}`;
    case "move": return `Move ${name}`;
    case "copy": return `Copy ${name}`;
    case "delete": return `Delete ${name}`;
    default: return `${job.operation || "File job"} ${name}`;
  }
}

function jobDetail(job: FileOperationJob): string {
  const status = job.status.replaceAll("_", " ");
  if (job.operation === "move" && job.from_path && job.to_path) return `${status} from ${job.from_path} to ${job.to_path}`;
  return `${status} ${jobDisplayPath(job)}`;
}

function jobProgress(job: FileOperationJob): number {
  const total = Number(job.bytes_total || job.files_total || 0);
  const done = Number(job.bytes_total ? job.bytes_done || 0 : job.files_done || 0);
  if (total <= 0) {
    return isTerminalJob(job.status) ? 100 : 0;
  }
  return Math.max(0, Math.min(100, Math.round((done / total) * 100)));
}

function isTerminalJob(status: string): boolean {
  return ["completed", "failed", "cancelled", "rollback_required", "rolled_back"].includes(status);
}

function fileDimensions(item: FileMeta): string {
  if (item.dimensions) return item.dimensions;
  if (item.dims) return item.dims;
  return "—";
}

function isImageFile(item: FileMeta): boolean {
  return fileIcon(item) === "image";
}

function isVideoFile(item: FileMeta): boolean {
  return fileIcon(item) === "video";
}

function isAudioFile(item: FileMeta): boolean {
  return fileIcon(item) === "audio";
}

function isPDFFile(item: FileMeta): boolean {
  return fileName(item).toLowerCase().endsWith(".pdf");
}

function parentPath(path: string): string {
  const parts = path.split("/").filter(Boolean);
  parts.pop();
  return parts.length ? `/${parts.join("/")}` : "/";
}

function previewURL(item: FileMeta, sourceID: string): string {
  const params = new URLSearchParams();
  if (sourceID) params.set("source_id", sourceID);
  params.set("path", item.path);
  return `/v1/home/files/preview?${params.toString()}`;
}

function startBrowserDownload(url: string, filename: string) {
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.rel = "noopener";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

function crumbs(path: string): Array<{ label: string; path: string }> {
  const parts = path.split("/").filter(Boolean);
  const out: Array<{ label: string; path: string }> = [{ label: "Home", path: "/" }];
  let acc = "";
  for (const part of parts) {
    acc += `/${part}`;
    out.push({ label: part, path: acc });
  }
  return out;
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "Files could not be loaded.";
}

function initialPathFromLocation(): string {
  const raw = new URLSearchParams(window.location.search).get("path") || "/";
  const trimmed = raw.trim();
  return trimmed.startsWith("/") ? trimmed : `/${trimmed}`;
}

function firstString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function recordArray(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.map(asRecord).filter((record) => Object.keys(record).length > 0) : [];
}

function parsePublicConfig(profile: ServiceProfile): Record<string, unknown> {
  const profileWithConfig = profile as ServiceProfile & { public_config?: unknown };
  if (profileWithConfig.public_config && typeof profileWithConfig.public_config === "object") return asRecord(profileWithConfig.public_config);
  if (!profile.public_config_json) return {};
  try {
    return asRecord(JSON.parse(profile.public_config_json));
  } catch {
    return {};
  }
}

function fileSourceFromRecord(record: Record<string, unknown>): FileSource | null {
  const host = firstString(record.host, record.smb_host);
  const share = firstString(record.share, record.smb_share);
  const type = firstString(record.type, record.kind, record.service_type);
  if (type === "local" || (!host && !share)) return null;
  const id = firstString(record.id, record.source_id, record.key, share, record.name);
  if (!id) return null;
  return {
    id,
    name: firstString(record.label, record.name, share, id),
    detail: host && share ? `//${host}/${share}` : firstString(record.path, record.root, "SMB source"),
  };
}

async function loadFileSources(): Promise<{ sources: FileSource[]; activeSourceID: string }> {
  try {
    const payload = await connectionsClient.listProfiles();
    const profile = payload.profiles.find((candidate) => candidate.service_type === "smb");
    if (!profile) return { sources: [], activeSourceID: "" };
    const config = parsePublicConfig(profile);
    const sources = new Map<string, FileSource>();
    for (const record of [...recordArray(config.shares), ...recordArray(config.sources), ...recordArray(config.file_sources)]) {
      const source = fileSourceFromRecord(record);
      if (source && !sources.has(source.id)) sources.set(source.id, source);
    }
    const sourceList = Array.from(sources.values());
    const activeSourceID = firstString(config.active_source_id, config.source_id);
    return {
      sources: sourceList,
      activeSourceID: sourceList.some((source) => source.id === activeSourceID) ? activeSourceID : sourceList[0]?.id || "",
    };
  } catch {
    return { sources: [], activeSourceID: "" };
  }
}

function Icon({ name }: { name: string }) {
  const common = {
    fill: "none",
    stroke: "currentColor",
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    strokeWidth: 1.8,
  };
  return (
    <svg className="ui-icon" viewBox="0 0 24 24" aria-hidden="true">
      {name === "hard-drive" ? <><rect x="4" y="5" width="16" height="14" rx="3" {...common} /><path d="M7 15h.01M17 15h.01" {...common} /></> : null}
      {name === "folder" ? <><path d="M3.5 7.5h6l1.6 2H20.5v7.5a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" {...common} /><path d="M3.5 9.5h17" {...common} /></> : null}
      {name === "folder-plus" ? <><path d="M3.5 7.5h6l1.6 2H20.5v7.5a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" {...common} /><path d="M12 13v4M10 15h4" {...common} /></> : null}
      {name === "upload" ? <><path d="M12 16V5" {...common} /><path d="m7.5 9.5 4.5-4.5 4.5 4.5" {...common} /><path d="M5 18.5h14" {...common} /></> : null}
      {name === "search" ? <><circle cx="11" cy="11" r="6.5" {...common} /><path d="m16.5 16.5 3.5 3.5" {...common} /></> : null}
      {name === "list" ? <><path d="M8 7h12M8 12h12M8 17h12" {...common} /><path d="M4 7h.01M4 12h.01M4 17h.01" {...common} /></> : null}
      {name === "grid" ? <><rect x="4" y="4" width="6" height="6" rx="1.5" {...common} /><rect x="14" y="4" width="6" height="6" rx="1.5" {...common} /><rect x="4" y="14" width="6" height="6" rx="1.5" {...common} /><rect x="14" y="14" width="6" height="6" rx="1.5" {...common} /></> : null}
      {name === "image" ? <><rect x="4" y="5" width="16" height="14" rx="2" {...common} /><path d="m7 16 3.2-3.2 2.3 2.3 2.8-3.1L20 17" {...common} /><circle cx="8.5" cy="8.5" r="1" fill="currentColor" /></> : null}
      {name === "video" ? <><rect x="4" y="6" width="12" height="12" rx="2" {...common} /><path d="m16 10 4-2v8l-4-2z" {...common} /></> : null}
      {name === "audio" ? <><path d="M9 18V7l9-2v11" {...common} /><circle cx="7" cy="18" r="2" {...common} /><circle cx="16" cy="16" r="2" {...common} /></> : null}
      {name === "archive" ? <><rect x="6" y="4" width="12" height="16" rx="2" {...common} /><path d="M10 4v4h4V4M10 13h4" {...common} /></> : null}
      {name === "file" ? <><path d="M7 4h7l4 4v12H7z" {...common} /><path d="M14 4v5h4" {...common} /></> : null}
      {name === "dots" ? <><circle cx="6" cy="12" r="1" fill="currentColor" /><circle cx="12" cy="12" r="1" fill="currentColor" /><circle cx="18" cy="12" r="1" fill="currentColor" /></> : null}
      {name === "download" ? <><path d="M12 4v11" {...common} /><path d="m7.5 10.5 4.5 4.5 4.5-4.5" {...common} /><path d="M5 19h14" {...common} /></> : null}
      {name === "move" ? <><path d="M5 12h14" {...common} /><path d="m14 7 5 5-5 5" {...common} /></> : null}
      {name === "trash" ? <><path d="M5 7h14M9 7V5h6v2M8 10v8M12 10v8M16 10v8" {...common} /></> : null}
      {name === "pencil" ? <><path d="M4 20h4l11-11a2.1 2.1 0 0 0-3-3L5 17z" {...common} /><path d="m14 8 2 2" {...common} /></> : null}
      {name === "x" ? <><path d="M7 7l10 10M17 7 7 17" {...common} /></> : null}
      {name === "check" ? <path d="m5 12 4 4L19 6" {...common} /> : null}
      {name === "minus" ? <path d="M6 12h12" {...common} /> : null}
    </svg>
  );
}

export function FileServerPage() {
  const [state, setState] = useState<State>({ status: "loading", path: initialPathFromLocation() });
  const [transferJobs, setTransferJobs] = useState<TransferJobsState>({ status: "loading", jobs: [] });
  const [sources, setSources] = useState<FileSource[]>([]);
  const [activeSourceID, setActiveSourceID] = useState("");
  const uploadInputRef = useRef<HTMLInputElement>(null);
  const { showToast } = useToast();

  async function load(path = state.path, message = "", sourceID = activeSourceID) {
    setState((current) => current.status === "ready"
      ? { ...current, refreshingPath: path, message, sharePickerOpen: false, menuPath: "" }
      : { status: "loading", path });
    try {
      let nextSources = sources;
      let nextSourceID = sourceID;
      if (!nextSources.length) {
        const loadedSources = await loadFileSources();
        nextSources = loadedSources.sources;
        nextSourceID = nextSourceID || loadedSources.activeSourceID;
        setSources(nextSources);
      }
      setActiveSourceID(nextSourceID);
      const payload = await fileServerClient.list(path, nextSourceID || undefined);
      const items = (payload.items || payload.entries || []) as FileMeta[];
      const defaultPreview = items.find((item) => !item.is_directory)?.path || items[0]?.path || "";
      setState((current) => ({
        status: "ready",
        path: payload.path || path,
        items,
        query: current.status === "ready" ? current.query : "",
        message,
        viewMode: current.status === "ready" ? current.viewMode : "list",
        selectedPaths: current.status === "ready" ? current.selectedPaths.filter((selectedPath) => items.some((item) => item.path === selectedPath)) : [],
        previewPath: current.status === "ready" && items.some((item) => item.path === current.previewPath) ? current.previewPath : defaultPreview,
        previewOpen: current.status === "ready" ? current.previewOpen : true,
        sharePickerOpen: false,
        menuPath: "",
        dialog: null,
        dialogDraft: "",
        refreshingPath: "",
      }));
    } catch (error) {
      setState((current) => current.status === "ready"
        ? { ...current, refreshingPath: "", message: errorMessage(error) }
        : { status: "error", path, message: errorMessage(error) });
    }
  }

  async function loadTransferJobs() {
    setTransferJobs((current) => ({ status: current.jobs.length ? "ready" : "loading", jobs: current.jobs }));
    try {
      const jobs = await fileServerClient.listJobs(20);
      setTransferJobs({ status: "ready", jobs });
    } catch (error) {
      setTransferJobs((current) => ({ status: "error", jobs: current.jobs, message: errorMessage(error) }));
    }
  }

  useEffect(() => {
    void load(initialPathFromLocation());
    void loadTransferJobs();
    // Initial load only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    let active = true;
    void fileServerClient.subscribeToJobs().catch(() => undefined);
    const unsubscribe = fileServerClient.onJobsChanged(() => {
      if (active) void loadTransferJobs();
    });
    return () => {
      active = false;
      unsubscribe();
    };
    // Job subscription only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const visibleItems = useMemo(() => {
    if (state.status !== "ready") return [];
    const query = state.query.trim().toLowerCase();
    const items = [...state.items].sort((left, right) => Number(Boolean(right.is_directory)) - Number(Boolean(left.is_directory)) || fileName(left).localeCompare(fileName(right)));
    if (!query) return items;
    return items.filter((item) => [fileName(item), item.path, fileType(item)].join(" ").toLowerCase().includes(query));
  }, [state]);

  if (state.status === "loading") {
    return (
      <section className="dashboard-page file-server-page" aria-labelledby="route-title">
        <h1 id="route-title">Loading files</h1>
        <p className="loading-state"><span className="spinner" aria-hidden="true" />Loading {state.path}...</p>
      </section>
    );
  }

  if (state.status === "error") {
    return (
      <section className="dashboard-page file-server-page" aria-labelledby="route-title">
        <h1 id="route-title">File Server</h1>
        <p className="error-state">{state.message}</p>
      </section>
    );
  }

  const readyState = state;
  const isRefreshing = Boolean(readyState.refreshingPath);
  const sourceOptions = sources.length ? sources : [{ id: "", name: "Home files", detail: "Home connector files" }];
  const activeSource = sourceOptions.find((source) => source.id === activeSourceID) || sourceOptions[0];
  const commandSourceID = activeSource?.id || "";

  function setReady(next: Partial<Extract<State, { status: "ready" }>>) {
    setState((current) => current.status === "ready" ? { ...current, ...next } : current);
  }

  async function createFolder() {
    const name = readyState.dialogDraft.trim();
    if (!name) return;
    try {
      await fileServerClient.createDirectory(childPath(readyState.path, name), commandSourceID || undefined);
      await load(readyState.path, "", commandSourceID);
      showToast("Folder created.");
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  async function deleteItems(items: FileMeta[]) {
    if (!items.length) return;
    try {
      for (const item of items) {
        await fileServerClient.deleteItem(item.path, Boolean(item.is_directory), commandSourceID || undefined);
      }
      await load(readyState.path, "", commandSourceID);
      showToast(items.length === 1 ? "Moved to trash." : `${items.length} items moved to trash.`);
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  async function uploadFiles(files: FileList | null) {
    const selectedFiles = Array.from(files || []).filter((file) => file.name);
    if (!selectedFiles.length) return;
    try {
      for (const file of selectedFiles) {
        await fileServerClient.uploadFile(file, readyState.path, commandSourceID || undefined);
      }
      if (uploadInputRef.current) uploadInputRef.current.value = "";
      await load(readyState.path, "", commandSourceID);
      showToast(selectedFiles.length === 1 ? `Uploaded ${selectedFiles[0].name}.` : `Uploaded ${selectedFiles.length} files.`);
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  async function downloadItems(items: FileMeta[]) {
    const files = items.filter((item) => !item.is_directory);
    if (!files.length) {
      showToast("Select one or more files to download.", "error");
      return;
    }
    try {
      for (const item of files) {
        const setup = await fileServerClient.setupDownload(item.path, commandSourceID || undefined);
        if (setup.url) startBrowserDownload(setup.url, fileName(item));
      }
      showToast(files.length === 1 ? `Started download for ${fileName(files[0])}.` : `Started ${files.length} downloads.`);
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  async function renameItem(item: FileMeta, name: string) {
    const cleanName = name.trim();
    if (!cleanName || cleanName.includes("/")) {
      showToast("Use a file name without slashes.", "error");
      return;
    }
    const targetPath = childPath(parentPath(item.path), cleanName);
    try {
      await fileServerClient.rename(item.path, targetPath, commandSourceID || undefined);
      await load(readyState.path, "", commandSourceID);
      showToast("Item renamed.");
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  async function moveItems(items: FileMeta[], destinationPath: string, destinationSourceID: string) {
    const cleanDestination = destinationPath.trim().startsWith("/") ? destinationPath.trim() : `/${destinationPath.trim()}`;
    if (!cleanDestination || !items.length) return;
    try {
      for (const item of items) {
        await fileServerClient.move(
          item.path,
          childPath(cleanDestination, fileName(item)),
          Boolean(item.is_directory),
          commandSourceID || undefined,
          destinationSourceID || commandSourceID || undefined,
        );
      }
      await load(readyState.path, "", commandSourceID);
      showToast(items.length === 1 ? "Move queued." : `${items.length} moves queued.`);
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  const pathCrumbs = crumbs(readyState.path);
  const folderItems = visibleItems.filter((item) => item.is_directory);
  const selectedItems = visibleItems.filter((item) => readyState.selectedPaths.includes(item.path));
  const selectedItem = visibleItems.find((item) => item.path === readyState.previewPath) || visibleItems.find((item) => !item.is_directory) || visibleItems[0];
  const selectedCount = readyState.selectedPaths.length;
  const previewItem = selectedItem;

  function selectItem(item: FileMeta) {
    const nextSelected = readyState.selectedPaths.includes(item.path)
      ? readyState.selectedPaths.filter((path: string) => path !== item.path)
      : [...readyState.selectedPaths, item.path];
    setReady({ selectedPaths: nextSelected, previewPath: item.path, previewOpen: true });
  }

  function selectAllVisible() {
    const visiblePaths = visibleItems.map((item) => item.path);
    const allSelected = visiblePaths.length > 0 && visiblePaths.every((path) => readyState.selectedPaths.includes(path));
    setReady({ selectedPaths: allSelected ? [] : visiblePaths });
  }

  function openItem(item: FileMeta) {
    if (item.is_directory) {
      void load(item.path, "", commandSourceID);
      return;
    }
    setReady({ previewPath: item.path, previewOpen: true });
  }

  function openFolderDialog() {
    setReady({
      dialog: { kind: "folder" },
      dialogDraft: "",
      menuPath: "",
    });
  }

  function openDeleteDialog(items: FileMeta[]) {
    if (!items.length) return;
    setReady({
      dialog: { kind: "delete", items },
      dialogDraft: "",
      menuPath: "",
    });
  }

  function openRenameDialog(item: FileMeta) {
    setReady({
      dialog: { kind: "rename", item },
      dialogDraft: fileName(item),
      menuPath: "",
    });
  }

  function openMoveDialog(items: FileMeta[]) {
    if (!items.length) return;
    setReady({
      dialog: { kind: "move", items, destinationPath: readyState.path, destinationSourceID: commandSourceID },
      dialogDraft: readyState.path,
      menuPath: "",
    });
  }

  function rowFor(item: FileMeta) {
    const name = fileName(item);
    const selected = readyState.selectedPaths.includes(item.path);
    return (
      <div className={`file-guide-row${selected ? " selected" : ""}`} key={item.path} onClick={() => openItem(item)} role="row">
        <button className="file-check" type="button" aria-label={`Select ${name}`} aria-pressed={selected} onClick={(event) => { event.stopPropagation(); selectItem(item); }}>
          {selected ? <Icon name="check" /> : null}
        </button>
        <span className="file-guide-name" role="cell">
          <span className="file-guide-glyph" style={{ color: fileIconTone(item) }} aria-hidden="true"><Icon name={fileIcon(item)} /></span>
          <button className="file-name-button" type="button" onClick={(event) => { event.stopPropagation(); openItem(item); }}>{name}</button>
        </span>
        <span className="file-guide-mono" role="cell">{formatSize(item)}</span>
        <span className="file-guide-muted" role="cell">{fileType(item)}</span>
        <span className="file-guide-mono" role="cell">{formatModified(item)}</span>
        <span className="file-guide-menu-cell" role="cell">
          <button className="file-menu-button" type="button" aria-label={`More actions for ${name}`} onClick={(event) => { event.stopPropagation(); setReady({ menuPath: readyState.menuPath === item.path ? "" : item.path }); }}>
            <Icon name="dots" />
          </button>
        </span>
      </div>
    );
  }

  return (
    <section className="dashboard-page file-server-page file-guide-surface" aria-labelledby="route-title">
      <header className="file-guide-header">
        <h1 id="route-title">File Server</h1>
        <div className="file-share-wrap">
          <button className="file-share-button" type="button" aria-expanded={readyState.sharePickerOpen} onClick={() => setReady({ sharePickerOpen: !readyState.sharePickerOpen })}>
            <Icon name="hard-drive" />
            <span className="visually-hidden">Active share</span>
            <span>{activeSource.name}</span>
            <span className="file-caret" aria-hidden="true">⌄</span>
          </button>
          {readyState.sharePickerOpen ? (
            <div className="file-share-menu" role="menu" aria-label="File shares">
              {sourceOptions.map((source) => (
                <button
                  key={source.id || "default"}
                  role="menuitem"
                  type="button"
                  onClick={() => {
                    setReady({ sharePickerOpen: false });
                    void load("/", "", source.id);
                  }}
                >
                  <Icon name="hard-drive" />
                  <span><strong>{source.name}</strong><small>{source.detail}</small></span>
                  {source.id === commandSourceID ? <Icon name="check" /> : null}
                </button>
              ))}
            </div>
          ) : null}
        </div>
        <div className="file-guide-actions">
          <button className="secondary" type="button" onClick={openFolderDialog}><Icon name="folder-plus" />New folder</button>
          <button type="button" onClick={() => uploadInputRef.current?.click()}><Icon name="upload" />Upload</button>
          <input ref={uploadInputRef} className="visually-hidden" type="file" multiple aria-label="Choose files to upload" onChange={(event) => void uploadFiles(event.currentTarget.files)} />
        </div>
      </header>

      {readyState.message ? <p className="notice-state">{readyState.message}</p> : null}

      <div className="file-guide-tools">
        <nav className="file-guide-crumbs" aria-label="Path">
          {pathCrumbs.map((crumb, index) => (
            <span className="file-crumb" key={crumb.path}>
              {index === 0 ? <Icon name="folder" /> : <span className="file-crumb-sep">/</span>}
              {index === pathCrumbs.length - 1 ? (
                <span className="file-crumb-current">{crumb.label}</span>
              ) : (
                <button type="button" className="file-crumb-link" onClick={() => void load(crumb.path, "", commandSourceID)}>{crumb.label}</button>
              )}
            </span>
          ))}
        </nav>
        <label className="file-search">
          <Icon name="search" />
          <span className="visually-hidden">Search files</span>
          <input type="search" placeholder="Search files" value={readyState.query} onChange={(event) => setReady({ query: event.target.value })} />
        </label>
        <div className="file-view-toggle" aria-label="View mode">
          <button type="button" aria-label="List" aria-current={readyState.viewMode === "list" ? "true" : undefined} onClick={() => setReady({ viewMode: "list" })}><Icon name="list" /></button>
          <button type="button" aria-label="Grid" aria-current={readyState.viewMode === "grid" ? "true" : undefined} onClick={() => setReady({ viewMode: "grid" })}><Icon name="grid" /></button>
        </div>
      </div>

      <div className={`file-guide-panes${readyState.previewOpen ? "" : " preview-closed"}`}>
        <aside className="file-tree-pane" aria-labelledby="file-folders-heading">
          <h2 id="file-folders-heading" className="file-pane-label">Folders</h2>
          {readyState.path !== "/" ? (
            <button className="file-tree-item" type="button" onClick={() => void load(parentPath(readyState.path), "", commandSourceID)}><Icon name="folder" />Parent folder</button>
          ) : null}
          {folderItems.length ? (
            <div className="file-tree-dynamic" aria-label="Current folder children">
              {folderItems.map((item) => {
                const name = fileName(item);
                return <button aria-label={`Open ${name}`} className="file-tree-item" key={item.path} type="button" onClick={() => void load(item.path, "", commandSourceID)}><Icon name="folder" />{name}</button>;
              })}
            </div>
          ) : null}
        </aside>

        <section className={`file-list-pane${isRefreshing ? " is-refreshing" : ""}`} aria-label="Files" aria-busy={isRefreshing}>
          <h2 className="visually-hidden">File list</h2>
          {isRefreshing ? (
            <p className="file-refresh-status"><span className="spinner" aria-hidden="true" />Opening {readyState.refreshingPath}</p>
          ) : null}
          <div className="file-list-scroll">
            {visibleItems.length && readyState.viewMode === "list" ? (
              <div className="file-guide-table" role="table" aria-label="Files">
                <div className="file-guide-head" role="row">
                  <button className="file-check" type="button" aria-label="Select all visible files" onClick={selectAllVisible}>
                    {selectedCount === 0 ? null : selectedCount === visibleItems.length ? <Icon name="check" /> : <Icon name="minus" />}
                  </button>
                  <span role="columnheader">Name</span>
                  <span role="columnheader">Size</span>
                  <span role="columnheader">Type</span>
                  <span role="columnheader">Modified</span>
                  <span />
                </div>
                <div>{visibleItems.map((item) => rowFor(item))}</div>
              </div>
            ) : visibleItems.length && readyState.viewMode === "grid" ? (
              <div className="file-guide-card-grid" aria-label="File grid">
                {visibleItems.map((item) => {
                  const name = fileName(item);
                  const selected = readyState.selectedPaths.includes(item.path);
                  return (
                    <article className={`file-guide-card${selected ? " selected" : ""}`} key={item.path}>
                      <button className="file-card-select" type="button" aria-label={`Select ${name}`} aria-pressed={selected} onClick={() => selectItem(item)}>
                        {selected ? <Icon name="check" /> : null}
                      </button>
                      <button className="file-card-menu" type="button" aria-label={`More actions for ${name}`} onClick={() => setReady({ menuPath: readyState.menuPath === item.path ? "" : item.path })}><Icon name="dots" /></button>
                      <button className="file-card-thumb" type="button" onClick={() => openItem(item)}>
                        <span style={{ color: fileIconTone(item) }}><Icon name={fileIcon(item)} /></span>
                      </button>
                      <div className="file-card-copy">
                        <strong>{name}</strong>
                        <span>{item.is_directory ? "Folder" : `${formatSize(item)} · ${fileType(item)}`}</span>
                      </div>
                    </article>
                  );
                })}
              </div>
            ) : (
              <div className="file-empty-state">
                <Icon name="search" />
                <strong>No files match your search</strong>
                <span>Try a different name or clear the search field.</span>
              </div>
            )}
          </div>
          <footer className="file-selection-bar">
            {selectedCount === 0 ? (
              <>
                <span>{visibleItems.length} {visibleItems.length === 1 ? "item" : "items"}</span>
              </>
            ) : (
              <>
                <span className="file-selection-count">{selectedCount} selected</span>
                <div className="file-selection-actions">
                  <button type="button" aria-label="Download selected" onClick={() => void downloadItems(selectedItems)}><Icon name="download" />Download</button>
                  <button className="secondary" type="button" aria-label="Move selected" onClick={() => openMoveDialog(selectedItems)}><Icon name="move" />Move</button>
                  <button className="danger-link" type="button" aria-label="Delete selected" onClick={() => openDeleteDialog(selectedItems)}><Icon name="trash" />Delete</button>
                  <button className="file-icon-action" type="button" aria-label="Clear selection" onClick={() => setReady({ selectedPaths: [] })}><Icon name="x" /></button>
                </div>
              </>
            )}
          </footer>
        </section>

        {readyState.previewOpen && previewItem ? (
          <aside className="file-preview-panel" aria-label="File preview">
            <h2 className="visually-hidden">Preview</h2>
            <div className="file-preview-media">
              {isImageFile(previewItem) ? (
                <img src={previewURL(previewItem, commandSourceID)} alt={`Preview ${fileName(previewItem)}`} />
              ) : isVideoFile(previewItem) ? (
                <video src={previewURL(previewItem, commandSourceID)} controls preload="metadata" aria-label={`Preview ${fileName(previewItem)}`} />
              ) : isAudioFile(previewItem) ? (
                <audio src={previewURL(previewItem, commandSourceID)} controls preload="metadata" aria-label={`Preview ${fileName(previewItem)}`} />
              ) : isPDFFile(previewItem) ? (
                <iframe src={previewURL(previewItem, commandSourceID)} title={`Preview ${fileName(previewItem)}`} />
              ) : (
                <span style={{ color: fileIconTone(previewItem) }}><Icon name={fileIcon(previewItem)} /></span>
              )}
              <em>{previewItem.is_directory ? "folder" : "preview"}</em>
              <button className="file-preview-close" type="button" aria-label="Close preview" title="Close preview" onClick={() => setReady({ previewOpen: false })}><Icon name="x" /></button>
            </div>
            <div className="file-preview-info">
              <strong>{fileName(previewItem)}</strong>
              <code>{previewItem.path}</code>
              <dl>
                <div><dt>Size</dt><dd>{formatSize(previewItem)}</dd></div>
                <div><dt>Type</dt><dd>{fileType(previewItem)}</dd></div>
                {isImageFile(previewItem) ? <div><dt>Dimensions</dt><dd>{fileDimensions(previewItem)}</dd></div> : null}
                <div><dt>Modified</dt><dd>{formatModified(previewItem)}</dd></div>
                <div><dt>Owner</dt><dd>{previewItem.owner || "Home agent"}</dd></div>
              </dl>
            </div>
            <div className="file-preview-actions">
              {!previewItem.is_directory ? (
                <button type="button" aria-label="Download preview" onClick={() => void downloadItems([previewItem])}><Icon name="download" />Download</button>
              ) : (
                <button type="button" aria-label={`Open ${fileName(previewItem)}`} onClick={() => openItem(previewItem)}><Icon name="folder" />Open</button>
              )}
              <button className="secondary" type="button" aria-label="Rename preview" onClick={() => openRenameDialog(previewItem)}><Icon name="pencil" />Rename</button>
              <button className="secondary" type="button" aria-label="Move preview" onClick={() => openMoveDialog([previewItem])}><Icon name="move" />Move</button>
              <button className="danger-link" type="button" aria-label={`Delete ${fileName(previewItem)}`} onClick={() => openDeleteDialog([previewItem])}><Icon name="trash" />Delete</button>
            </div>
          </aside>
        ) : null}
      </div>

      <TransferJobsPanel state={transferJobs} onRefresh={() => void loadTransferJobs()} />

      {readyState.menuPath ? (
        <FileActionMenu
          item={visibleItems.find((item) => item.path === readyState.menuPath)}
          onClose={() => setReady({ menuPath: "" })}
          onDownload={(item) => void downloadItems([item])}
          onDelete={(item) => openDeleteDialog([item])}
          onMove={(item) => openMoveDialog([item])}
          onOpen={(item) => setReady({ previewPath: item.path, previewOpen: true, menuPath: "" })}
          onRename={openRenameDialog}
        />
      ) : null}
      {readyState.dialog ? (
        <FileDialogCard
          dialog={readyState.dialog}
          draft={readyState.dialogDraft}
          onDraft={(dialogDraft) => setReady({ dialogDraft })}
          onClose={() => setReady({ dialog: null, dialogDraft: "" })}
          onCreate={() => void createFolder()}
          onDelete={(items) => void deleteItems(items)}
          onMove={(items, destinationPath, destinationSourceID) => void moveItems(items, destinationPath, destinationSourceID)}
          onMoveSource={(destinationSourceID) => setReady({
            dialog: readyState.dialog?.kind === "move" ? { ...readyState.dialog, destinationSourceID } : readyState.dialog,
          })}
          onRename={(item, name) => void renameItem(item, name)}
          sources={sourceOptions}
        />
      ) : null}
    </section>
  );
}

function TransferJobsPanel({ state, onRefresh }: { state: TransferJobsState; onRefresh: () => void }) {
  const activeJobs = state.jobs.filter((job) => !isTerminalJob(job.status));
  const recentJobs = state.jobs.filter((job) => isTerminalJob(job.status)).slice(0, 8);
  const visibleJobs = [...activeJobs, ...recentJobs].slice(0, 10);
  return (
    <section className="file-transfers-panel" aria-labelledby="file-transfers-heading">
      <header>
        <div>
          <h2 id="file-transfers-heading">Transfers</h2>
          <p>{activeJobs.length ? `${activeJobs.length} active` : "No active transfers"}</p>
        </div>
        <button className="secondary" type="button" onClick={onRefresh}>Refresh</button>
      </header>
      {state.status === "loading" && !visibleJobs.length ? (
        <p className="file-transfer-empty"><span className="spinner" aria-hidden="true" />Loading transfers...</p>
      ) : null}
      {state.status === "error" ? <p className="error-state">{state.message}</p> : null}
      {state.status !== "loading" && !visibleJobs.length ? (
        <p className="file-transfer-empty">Downloads, uploads, and moves will appear here.</p>
      ) : null}
      {visibleJobs.length ? (
        <div className="file-transfer-list">
          {visibleJobs.map((job) => <TransferJobRow key={job.id} job={job} />)}
        </div>
      ) : null}
    </section>
  );
}

function TransferJobRow({ job }: { job: FileOperationJob }) {
  const progress = jobProgress(job);
  const updated = job.completed_at || job.updated_at || job.created_at;
  const bytesTotal = Number(job.bytes_total || 0);
  const bytesDone = Number(job.bytes_done || 0);
  return (
    <article className={`file-transfer-job status-${job.status.replaceAll("_", "-")}`}>
      <span className="file-transfer-icon" aria-hidden="true"><Icon name={job.operation === "upload" ? "upload" : job.operation === "move" ? "move" : "download"} /></span>
      <div className="file-transfer-main">
        <div className="file-transfer-title">
          <strong>{jobTitle(job)}</strong>
          <span>{formatJobTime(updated)}</span>
        </div>
        <p>{jobDetail(job)}</p>
        {job.error_message ? <p className="file-transfer-error">{job.error_message}</p> : null}
        <div className="file-transfer-progress" aria-label={`${jobTitle(job)} progress`}>
          <span style={{ width: `${progress}%` }} />
        </div>
        <small>{bytesTotal > 0 ? `${formatBytes(bytesDone)} of ${formatBytes(bytesTotal)}` : `${progress}%`}</small>
      </div>
      <span className="status-pill">{job.status.replaceAll("_", " ")}</span>
    </article>
  );
}

function FileActionMenu({
  item,
  onClose,
  onDownload,
  onDelete,
  onMove,
  onOpen,
  onRename,
}: {
  item?: FileMeta;
  onClose: () => void;
  onDownload: (item: FileMeta) => void;
  onDelete: (item: FileMeta) => void;
  onMove: (item: FileMeta) => void;
  onOpen: (item: FileMeta) => void;
  onRename: (item: FileMeta) => void;
}) {
  if (!item) return null;
  return (
    <div className="file-context-menu" role="menu" aria-label="File actions">
      <button role="menuitem" type="button" onClick={() => { onOpen(item); onClose(); }}><Icon name="file" />Open</button>
      {!item.is_directory ? <button role="menuitem" type="button" onClick={() => { onDownload(item); onClose(); }}><Icon name="download" />Download</button> : null}
      <button role="menuitem" type="button" onClick={() => { onRename(item); onClose(); }}><Icon name="pencil" />Rename</button>
      <button role="menuitem" type="button" onClick={() => { onMove(item); onClose(); }}><Icon name="move" />Move</button>
      <button role="menuitem" type="button" className="danger" onClick={() => onDelete(item)}><Icon name="trash" />Delete</button>
    </div>
  );
}

function FileDialogCard({
  dialog,
  draft,
  onDraft,
  onClose,
  onCreate,
  onDelete,
  onMove,
  onMoveSource,
  onRename,
  sources,
}: {
  dialog: FileDialog;
  draft: string;
  onDraft: (value: string) => void;
  onClose: () => void;
  onCreate: () => void;
  onDelete: (items: FileMeta[]) => void;
  onMove: (items: FileMeta[], destinationPath: string, destinationSourceID: string) => void;
  onMoveSource: (destinationSourceID: string) => void;
  onRename: (item: FileMeta, name: string) => void;
  sources: FileSource[];
}) {
  const title = dialog.kind === "folder"
    ? "New folder"
    : dialog.kind === "rename"
      ? "Rename"
      : dialog.kind === "move"
        ? dialog.items.length === 1 ? `Move "${fileName(dialog.items[0])}"` : `Move ${dialog.items.length} items`
        : dialog.items.length === 1
          ? `Delete "${fileName(dialog.items[0])}"?`
          : `Delete ${dialog.items.length} items?`;
  const icon = dialog.kind === "folder" ? "folder-plus" : dialog.kind === "rename" ? "pencil" : dialog.kind === "move" ? "move" : "trash";
  return (
    <div className="guide-dialog-scrim" role="presentation" onClick={onClose}>
      <section className="guide-dialog" role="dialog" aria-modal="true" aria-label={title} onClick={(event) => event.stopPropagation()}>
        <header>
          <span className="guide-dialog-icon" aria-hidden="true"><Icon name={icon} /></span>
          <h2>{title}</h2>
          <button className="file-icon-action" type="button" aria-label="Close dialog" onClick={onClose}><Icon name="x" /></button>
        </header>
        {dialog.kind === "folder" || dialog.kind === "rename" ? (
          <label className="guide-dialog-field">
            <span>{dialog.kind === "folder" ? "Folder name" : "File name"}</span>
            <input autoFocus placeholder="Untitled folder" value={draft} onChange={(event) => onDraft(event.target.value)} />
          </label>
        ) : null}
        {dialog.kind === "move" ? (
          <>
            <label className="guide-dialog-field">
              <span>Destination path</span>
              <input autoFocus value={draft} onChange={(event) => onDraft(event.target.value)} />
            </label>
            {sources.length > 1 ? (
              <label className="guide-dialog-field">
                <span>Destination share</span>
                <select value={dialog.destinationSourceID} onChange={(event) => onMoveSource(event.target.value)}>
                  {sources.map((source) => <option key={source.id || "default"} value={source.id}>{source.name}</option>)}
                </select>
              </label>
            ) : null}
            <p className="guide-dialog-copy">Move {dialog.items.length === 1 ? fileName(dialog.items[0]) : `${dialog.items.length} selected items`} into {draft || dialog.destinationPath}.</p>
          </>
        ) : null}
        {dialog.kind === "delete" ? (
          <p className="guide-dialog-copy">This will delete {dialog.items.length === 1 ? fileName(dialog.items[0]) : `${dialog.items.length} selected items`} from the active source.</p>
        ) : null}
        <footer>
          <button className="secondary" type="button" onClick={onClose}>Cancel</button>
          {dialog.kind === "folder" ? <button type="button" onClick={onCreate}>Create folder</button> : null}
          {dialog.kind === "rename" ? <button type="button" onClick={() => onRename(dialog.item, draft)}>Rename</button> : null}
          {dialog.kind === "move" ? <button type="button" onClick={() => onMove(dialog.items, draft || dialog.destinationPath, dialog.destinationSourceID)}>Move here</button> : null}
          {dialog.kind === "delete" ? <button className="danger-solid" type="button" onClick={() => onDelete(dialog.items)}>Delete</button> : null}
        </footer>
      </section>
    </div>
  );
}
