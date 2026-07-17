import { useEffect, useMemo, useRef, useState, type MouseEvent as ReactMouseEvent } from "react";
import {
  type KanbanBoard,
  type NoteAttachment,
  noteID,
  profileNotesClient,
  type ProfileNote,
  type ProfileNoteSummary,
} from "../api/profileNotes";
import { useConfirmDialog, useToast } from "../ui/primitives";
import { boardFromMarkdown, boardToMarkdown, KanbanEditor } from "./KanbanEditor";

type Editor = {
  instanceKey: string;
  noteID: string;
  title: string;
  body: string;
  revision: string;
  pageType: string;
  parentID: string;
  mcpExcluded: boolean;
  board: KanbanBoard | null;
  attachments: NoteAttachment[];
  updatedAt: string;
  shared: boolean;
};

type ReadyState = {
  status: "ready";
  notes: ProfileNoteSummary[];
  selectedID: string;
  editor: Editor;
  query: string;
  selectedNotebookID: string;
  message: string;
  railOpen: boolean;
  notebookDialogOpen: boolean;
  notebookDraft: string;
  moveDialogNoteID: string;
  moveDialogTargetID: string;
  savedEditor: Editor;
  saving: boolean;
};

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | ReadyState;

const emptyEditor: Editor = {
  instanceKey: "empty",
  noteID: "",
  title: "",
  body: "",
  revision: "",
  pageType: "text",
  parentID: "",
  mcpExcluded: false,
  board: null,
  attachments: [],
  updatedAt: "",
  shared: false,
};

const ROOT_NOTEBOOK_FILTER = "__root__";
const NOTE_AUTOSAVE_DELAY_MS = 750;
const NOTE_HISTORY_GROUP_DELAY_MS = 750;
const NOTE_HISTORY_LIMIT = 50;

type HistoryActionKind = "typing" | "deletion" | "paste" | "formatting";

type HistoryEntry = {
  body: string;
  action: HistoryActionKind;
};

type HistoryState = {
  past: HistoryEntry[];
  future: HistoryEntry[];
};

type PendingSave = {
  editor: Editor;
  background: boolean;
};

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "Profile notes could not be loaded.";
}

function noteTitle(note: ProfileNoteSummary): string {
  return note.title?.trim() || noteID(note) || "Untitled";
}

function isNotebook(note: ProfileNoteSummary | Editor): boolean {
  return noteKind(note) === "notebook";
}

function editorFromNote(note: ProfileNote): Editor {
  const id = noteID(note);
  return {
    instanceKey: id || "empty",
    noteID: id,
    title: note.title || "",
    body: note.body_markdown || note.content || "",
    revision: note.revision || "",
    pageType: note.page_type || "text",
    parentID: note.parent_id || "",
    mcpExcluded: Boolean(note.mcp_excluded),
    board: note.board || null,
    attachments: note.attachments || [],
    updatedAt: note.updated_at || "",
    shared: Boolean(note.shared),
  };
}

function editorChanged(left: Editor, right: Editor): boolean {
  return left.noteID !== right.noteID
    || left.title !== right.title
    || left.body !== right.body
    || left.revision !== right.revision
    || left.pageType !== right.pageType
    || left.parentID !== right.parentID
    || left.mcpExcluded !== right.mcpExcluded
    || JSON.stringify(left.board) !== JSON.stringify(right.board);
}

function sortNotes(notes: ProfileNoteSummary[]): ProfileNoteSummary[] {
  return [...notes].sort((left, right) => String(right.updated_at || "").localeCompare(String(left.updated_at || "")));
}

function noteKind(note: ProfileNoteSummary | Editor): string {
  const pageType = "pageType" in note ? note.pageType : note.page_type;
  if (pageType === "notebook") return "notebook";
  if (pageType === "kanban") return "kanban";
  return "text";
}

function noteTag(note: ProfileNoteSummary): string {
  const title = noteTitle(note).toLowerCase();
  if (note.page_type === "kanban") return "board";
  if (note.page_type === "notebook") return "notebook";
  if (title.includes("grocery")) return "shopping";
  if (title.includes("wifi") || title.includes("password")) return "private";
  if (title.includes("trip")) return "travel";
  return "note";
}

function notebooksFrom(notes: ProfileNoteSummary[]): ProfileNoteSummary[] {
  return notes
    .filter(isNotebook)
    .sort((left, right) => noteTitle(left).localeCompare(noteTitle(right)));
}

function notebookTitle(notes: ProfileNoteSummary[], notebookID?: string): string {
  if (!notebookID) return "";
  const notebook = notes.find((note) => noteID(note) === notebookID);
  return notebook ? noteTitle(notebook) : "";
}

function notebookChildCount(notes: ProfileNoteSummary[], notebookID: string): number {
  return notes.filter((note) => note.parent_id === notebookID).length;
}

function noteOwnMcpExcluded(note: ProfileNoteSummary | Editor): boolean {
  return "mcpExcluded" in note ? note.mcpExcluded : Boolean(note.mcp_excluded);
}

function parentNotebook(notes: ProfileNoteSummary[], notebookID?: string): ProfileNoteSummary | undefined {
  if (!notebookID) return undefined;
  return notes.find((note) => noteID(note) === notebookID);
}

function noteInheritedMcpExcluded(note: ProfileNoteSummary | Editor, notes: ProfileNoteSummary[]): boolean {
  if (isNotebook(note) || noteOwnMcpExcluded(note)) return false;
  const notebookID = "parentID" in note ? note.parentID : note.parent_id;
  return Boolean(parentNotebook(notes, notebookID)?.mcp_excluded);
}

function noteEffectiveMcpExcluded(note: ProfileNoteSummary | Editor, notes: ProfileNoteSummary[]): boolean {
  return noteOwnMcpExcluded(note) || noteInheritedMcpExcluded(note, notes);
}

function activeNotebookForNewNote(state: ReadyState): string {
  if (state.selectedNotebookID && state.selectedNotebookID !== ROOT_NOTEBOOK_FILTER) return state.selectedNotebookID;
  if (state.editor.pageType === "notebook" && state.editor.noteID) return state.editor.noteID;
  return "";
}

function noteMatchesQuery(note: ProfileNoteSummary, notes: ProfileNoteSummary[], query: string): boolean {
  if (!query) return true;
  return [note.title, note.preview, note.note_id, note.parent_id, notebookTitle(notes, note.parent_id)]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function noteIconName(note: ProfileNoteSummary | Editor): string {
  const kind = noteKind(note);
  if (kind === "notebook") return "book";
  if (kind === "kanban") return "kanban";
  return "note";
}

function updatedLabel(note?: ProfileNoteSummary): string {
  if (!note?.updated_at) return "No timestamp";
  const date = new Date(note.updated_at);
  if (Number.isNaN(date.getTime())) return "No timestamp";
  const minutes = Math.max(1, Math.round((Date.now() - date.getTime()) / 60000));
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

/* Note rows hide their actions off-canvas; touch users swipe them in, mouse users hover. */
function finePointer(): boolean {
  return Boolean(window.matchMedia?.("(hover: hover) and (pointer: fine)").matches);
}

function revealRowActions(event: ReactMouseEvent<HTMLDivElement>) {
  if (!finePointer()) return;
  event.currentTarget.scrollTo({ left: event.currentTarget.scrollWidth, behavior: "smooth" });
}

function hideRowActions(event: ReactMouseEvent<HTMLDivElement>) {
  if (!finePointer()) return;
  event.currentTarget.scrollTo({ left: 0, behavior: "smooth" });
}

type BodyMutation = { body: string; selectionStart: number; selectionEnd: number };

export function wrapSelection(body: string, start: number, end: number, prefix: string, suffix: string, placeholder: string): BodyMutation {
  const selected = body.slice(start, end) || placeholder;
  const next = `${body.slice(0, start)}${prefix}${selected}${suffix}${body.slice(end)}`;
  return {
    body: next,
    selectionStart: start + prefix.length,
    selectionEnd: start + prefix.length + selected.length,
  };
}

export function stripLinePrefix(line: string): string {
  return line.replace(/^\s*(#{1,6}\s+|[-*]\s+|\d+\.\s+)/, "");
}

export function prefixLines(body: string, start: number, end: number, prefixFor: (line: string, index: number) => string): BodyMutation {
  const blockStart = body.lastIndexOf("\n", Math.max(0, start - 1)) + 1;
  const newlineAfter = body.indexOf("\n", end);
  const blockEnd = newlineAfter === -1 ? body.length : newlineAfter;
  const block = body.slice(blockStart, blockEnd);
  const nextBlock = block.split("\n").map((line, index) => prefixFor(line, index)).join("\n");
  return {
    body: `${body.slice(0, blockStart)}${nextBlock}${body.slice(blockEnd)}`,
    selectionStart: blockStart,
    selectionEnd: blockStart + nextBlock.length,
  };
}

function escapeHTML(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function renderInlineMarkdown(value: string): string {
  return escapeHTML(value)
    .replace(/&lt;u&gt;([\s\S]*?)&lt;\/u&gt;/g, "<u>$1</u>")
    .replace(/\*\*([^*\n]+)\*\*/g, "<strong>$1</strong>")
    .replace(/(^|[^\w])_([^_\n]+)_/g, "$1<em>$2</em>")
    .replace(/\[([^\]\n]+)\]\(([^)\n]+)\)/g, (_match, text: string, href: string) => {
      const normalizedHref = href.replace(/&amp;/g, "&").trim();
      if (!/^(https?:|mailto:|\/|#)/i.test(normalizedHref)) return text;
      return `<a href="${href}" rel="noreferrer">${text}</a>`;
    });
}

function markdownToHTML(body: string): string {
  const lines = body.split(/\r?\n/);
  const html: string[] = [];
  let list: "ul" | "ol" | "" = "";
  const closeList = () => {
    if (!list) return;
    html.push(`</${list}>`);
    list = "";
  };

  lines.forEach((line) => {
    const heading = /^(#{1,3})\s+(.*)$/.exec(line);
    if (heading) {
      closeList();
      const level = heading[1].length;
      html.push(`<h${level}>${renderInlineMarkdown(heading[2]) || "<br>"}</h${level}>`);
      return;
    }
    const unordered = /^\s*[-*]\s+(.*)$/.exec(line);
    if (unordered) {
      if (list !== "ul") {
        closeList();
        html.push("<ul>");
        list = "ul";
      }
      html.push(`<li>${renderInlineMarkdown(unordered[1]) || "<br>"}</li>`);
      return;
    }
    const ordered = /^\s*\d+\.\s+(.*)$/.exec(line);
    if (ordered) {
      if (list !== "ol") {
        closeList();
        html.push("<ol>");
        list = "ol";
      }
      html.push(`<li>${renderInlineMarkdown(ordered[1]) || "<br>"}</li>`);
      return;
    }
    closeList();
    html.push(`<p>${line ? renderInlineMarkdown(line) : "<br>"}</p>`);
  });
  closeList();
  return html.join("");
}

function inlineHTMLToMarkdown(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent || "";
  if (node.nodeType !== Node.ELEMENT_NODE) return "";
  const element = node as HTMLElement;
  const inner = Array.from(element.childNodes).map(inlineHTMLToMarkdown).join("");
  switch (element.tagName) {
    case "B":
    case "STRONG":
      return `**${inner}**`;
    case "I":
    case "EM":
      return `_${inner}_`;
    case "U":
      return `<u>${inner}</u>`;
    case "A":
      return `[${inner}](${element.getAttribute("href") || ""})`;
    case "BR":
      return "\n";
    default:
      return inner;
  }
}

const BLOCK_TAGS = new Set(["H1", "H2", "H3", "UL", "OL", "P", "DIV", "BLOCKQUOTE"]);

/* Browsers nest blocks freely inside contentEditable (e.g. a <ul> inside a <p>),
   so block conversion has to recurse instead of assuming a flat structure. */
function walkBlockToMarkdown(node: Node, lines: string[]): void {
  if (node.nodeType === Node.TEXT_NODE) {
    if ((node.textContent || "").trim()) lines.push(node.textContent || "");
    return;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) return;
  const element = node as HTMLElement;
  const inline = () => Array.from(element.childNodes).map(inlineHTMLToMarkdown).join("").trimEnd();
  switch (element.tagName) {
    case "H1":
      lines.push(`# ${inline()}`.trimEnd());
      return;
    case "H2":
      lines.push(`## ${inline()}`.trimEnd());
      return;
    case "H3":
      lines.push(`### ${inline()}`.trimEnd());
      return;
    case "UL":
    case "OL":
      Array.from(element.children).forEach((child, index) => {
        const item = Array.from(child.childNodes).map(inlineHTMLToMarkdown).join("").trimEnd();
        lines.push(element.tagName === "OL" ? `${index + 1}. ${item}`.trimEnd() : `- ${item}`.trimEnd());
      });
      return;
    case "P":
    case "DIV":
    case "BLOCKQUOTE": {
      const hasBlockChild = Array.from(element.children).some((child) => BLOCK_TAGS.has(child.tagName));
      if (!hasBlockChild) {
        lines.push(inline());
        return;
      }
      let run: Node[] = [];
      const flush = () => {
        if (!run.length) return;
        const text = run.map(inlineHTMLToMarkdown).join("").trimEnd();
        if (text) lines.push(text);
        run = [];
      };
      Array.from(element.childNodes).forEach((child) => {
        if (child.nodeType === Node.ELEMENT_NODE && BLOCK_TAGS.has((child as HTMLElement).tagName)) {
          flush();
          walkBlockToMarkdown(child, lines);
        } else {
          run.push(child);
        }
      });
      flush();
      return;
    }
    default:
      lines.push(inlineHTMLToMarkdown(element));
  }
}

function htmlToMarkdown(root: HTMLElement): string {
  const lines: string[] = [];
  Array.from(root.childNodes).forEach((node) => walkBlockToMarkdown(node, lines));
  return lines.join("\n").replace(/\n{3,}/g, "\n\n").trimEnd();
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
      {name === "panel" ? <><rect x="4" y="4" width="16" height="16" rx="3" {...common} /><path d="M9 4v16" {...common} /></> : null}
      {name === "plus" ? <><path d="M12 5v14M5 12h14" {...common} /></> : null}
      {name === "book-plus" ? <><path d="M5 5.5A2.5 2.5 0 0 1 7.5 3H19v15H7.5A2.5 2.5 0 0 0 5 20.5z" {...common} /><path d="M9 7h5M15 10v5M12.5 12.5h5" {...common} /></> : null}
      {name === "search" ? <><circle cx="11" cy="11" r="6.5" {...common} /><path d="m16.5 16.5 3.5 3.5" {...common} /></> : null}
      {name === "note" ? <><path d="M7 4h10l2 2v14H7z" {...common} /><path d="M10 9h6M10 13h6M10 17h4" {...common} /></> : null}
      {name === "kanban" ? <><rect x="4" y="5" width="16" height="14" rx="2" {...common} /><path d="M9 5v14M15 5v14" {...common} /></> : null}
      {name === "book" ? <><path d="M5 5.5A2.5 2.5 0 0 1 7.5 3H19v15H7.5A2.5 2.5 0 0 0 5 20.5z" {...common} /><path d="M8 7h7M8 11h7" {...common} /></> : null}
      {name === "trash" ? <><path d="M5 7h14M9 7V5h6v2M8 10v8M12 10v8M16 10v8" {...common} /></> : null}
      {name === "undo" ? <><path d="M9 7 5 11l4 4" {...common} /><path d="M5 11h8a5 5 0 0 1 5 5v1" {...common} /></> : null}
      {name === "redo" ? <><path d="m15 7 4 4-4 4" {...common} /><path d="M19 11h-8a5 5 0 0 0-5 5v1" {...common} /></> : null}
      {name === "list" ? <><path d="M8 7h12M8 12h12M8 17h12" {...common} /><path d="M4 7h.01M4 12h.01M4 17h.01" {...common} /></> : null}
      {name === "ordered" ? <><path d="M10 7h10M10 12h10M10 17h10" {...common} /><path d="M4 6h1v3M4 17h3M4 14h2a1 1 0 0 1 0 2H4" {...common} /></> : null}
      {name === "tag" ? <><path d="M4 12V5h7l9 9-6 6z" {...common} /><circle cx="8" cy="8" r="1" fill="currentColor" /></> : null}
      {name === "link" ? <><path d="M10 13a4 4 0 0 0 5.5.5l2-2a4 4 0 0 0-5.5-5.5l-1 1" {...common} /><path d="M14 11a4 4 0 0 0-5.5-.5l-2 2a4 4 0 0 0 5.5 5.5l1-1" {...common} /></> : null}
      {name === "lock" ? <><rect x="6" y="11" width="12" height="9" rx="2" {...common} /><path d="M9 11V8a3 3 0 0 1 6 0v3" {...common} /></> : null}
      {name === "unlock" ? <><rect x="6" y="11" width="12" height="9" rx="2" {...common} /><path d="M15 11V8a3 3 0 1 0-6 0" {...common} /></> : null}
      {name === "x" ? <><path d="M7 7l10 10M17 7 7 17" {...common} /></> : null}
      {name === "check" ? <path d="m5 12 4 4L19 6" {...common} /> : null}
    </svg>
  );
}

export function ProfileNotesPage() {
  const [state, setState] = useState<State>({ status: "loading" });
  const [history, setHistory] = useState<HistoryState>({ past: [], future: [] });
  const bodyInputRef = useRef<HTMLDivElement>(null);
  const lastRenderedBodyRef = useRef("");
  const lastHistoryActionRef = useRef<{ action: HistoryActionKind; at: number } | null>(null);
  const richCommandPendingRef = useRef(false);
  const pendingSelectionRef = useRef<{ start: number; end: number } | null>(null);
  const savingRef = useRef(false);
  const activeSaveRef = useRef<PendingSave | null>(null);
  const pendingSaveRef = useRef<PendingSave[]>([]);
  const autosaveTimerRef = useRef<number | null>(null);
  const latestEditorRef = useRef<Editor>(emptyEditor);
  const latestSavedEditorRef = useRef<Editor>(emptyEditor);
  const draftSequenceRef = useRef(0);

  // Restore the caret/selection after a toolbar action once React has committed the new body.
  useEffect(() => {
    const pending = pendingSelectionRef.current;
    const node = bodyInputRef.current;
    if (!pending || !node) return;
    pendingSelectionRef.current = null;
    node.focus();
  });

  useEffect(() => {
    if (state.status !== "ready") return;
    latestEditorRef.current = state.editor;
    latestSavedEditorRef.current = state.savedEditor;
    if (state.editor.pageType !== "text") return;
    const node = bodyInputRef.current;
    if (!node || lastRenderedBodyRef.current === state.editor.body) return;
    node.innerHTML = markdownToHTML(state.editor.body);
    lastRenderedBodyRef.current = state.editor.body;
  }, [state]);

  useEffect(() => () => {
    if (autosaveTimerRef.current !== null) window.clearTimeout(autosaveTimerRef.current);
    const editor = latestEditorRef.current;
    if (editorHasSavableContent(editor) && editorChanged(editor, latestSavedEditorRef.current)) {
      void saveNote(editor, true);
    }
  }, []);
  const dialog = useConfirmDialog();
  const { showToast } = useToast();

  async function load(message = "") {
    try {
      const payload = await profileNotesClient.listNotes();
      const notes = sortNotes(payload.notes || []);
      const currentSelected = state.status === "ready" ? state.selectedID : "";
      const selectedID = currentSelected && notes.some((note) => noteID(note) === currentSelected)
        ? currentSelected
        : noteID(notes[0] || {});
      const editor = selectedID ? editorFromNote(await profileNotesClient.fetchNote(selectedID)) : emptyEditor;
      setState((current) => ({
        status: "ready",
        notes,
        selectedID,
        editor,
        query: current.status === "ready" ? current.query : "",
        selectedNotebookID: current.status === "ready" ? current.selectedNotebookID : "",
        message,
        railOpen: current.status === "ready" ? current.railOpen : true,
        notebookDialogOpen: current.status === "ready" ? current.notebookDialogOpen : false,
        notebookDraft: current.status === "ready" ? current.notebookDraft : "",
        moveDialogNoteID: current.status === "ready" ? current.moveDialogNoteID : "",
        moveDialogTargetID: current.status === "ready" ? current.moveDialogTargetID : "",
        savedEditor: editor,
        saving: false,
      }));
    } catch (error) {
      setState({ status: "error", message: errorMessage(error) });
    }
  }

  useEffect(() => {
    void load();
    // Initial load only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const visibleNotes = useMemo(() => {
    if (state.status !== "ready") return [];
    const query = state.query.trim().toLowerCase();
    let notes = state.notes;
    if (state.selectedNotebookID === ROOT_NOTEBOOK_FILTER) {
      notes = notes.filter((note) => !note.parent_id && !isNotebook(note));
    } else if (state.selectedNotebookID) {
      notes = notes.filter((note) => note.parent_id === state.selectedNotebookID);
    } else {
      notes = notes.filter((note) => !isNotebook(note));
    }
    return notes.filter((note) => noteMatchesQuery(note, state.notes, query));
  }, [state]);

  if (state.status === "loading") {
    return (
      <section className="dashboard-page notes-guide-page" aria-labelledby="route-title">
        <h1 id="route-title">Loading notes</h1>
        <p className="loading-state"><span className="spinner" aria-hidden="true" />Loading notes...</p>
      </section>
    );
  }

  if (state.status === "error") {
    return (
      <section className="dashboard-page notes-guide-page" aria-labelledby="route-title">
        <h1 id="route-title">Notes</h1>
        <p className="error-state">{state.message}</p>
      </section>
    );
  }

  const readyState = state;
  const selectedSummary = readyState.notes.find((note) => noteID(note) === readyState.selectedID);
  const notebookItems = notebooksFrom(readyState.notes);
  const visibleNotebookItems = notebookItems.filter((note) => noteMatchesQuery(note, readyState.notes, readyState.query.trim().toLowerCase()));
  const editorInheritedExclusion = noteInheritedMcpExcluded(readyState.editor, readyState.notes);
  const editorEffectiveExclusion = noteEffectiveMcpExcluded(readyState.editor, readyState.notes);

  function setReady(next: Partial<ReadyState>) {
    setState((current) => current.status === "ready" ? { ...current, ...next } : current);
  }

  function clearAutosaveTimer() {
    if (autosaveTimerRef.current === null) return;
    window.clearTimeout(autosaveTimerRef.current);
    autosaveTimerRef.current = null;
  }

  function editorHasSavableContent(editor: Editor): boolean {
    return Boolean(editor.noteID || editor.title.trim() || editor.body.trim() || editor.pageType !== "text");
  }

  function scheduleAutosave(editor: Editor) {
    latestEditorRef.current = editor;
    clearAutosaveTimer();
    if (!editorHasSavableContent(editor) || !editorChanged(editor, readyState.savedEditor)) return;
    autosaveTimerRef.current = window.setTimeout(() => {
      autosaveTimerRef.current = null;
      void saveNote(latestEditorRef.current, true);
    }, NOTE_AUTOSAVE_DELAY_MS);
  }

  function updateEditor(editor: Editor, autosave = true) {
    latestEditorRef.current = editor;
    setReady({ editor });
    if (autosave) scheduleAutosave(editor);
  }

  function flushAutosave() {
    clearAutosaveTimer();
    const editor = latestEditorRef.current;
    if (!editorHasSavableContent(editor) || !editorChanged(editor, readyState.savedEditor)) return;
    void saveNote(editor, true);
  }

  function newDraftEditor(parentID = ""): Editor {
    draftSequenceRef.current++;
    return { ...emptyEditor, instanceKey: `draft-${draftSequenceRef.current}`, parentID };
  }

  function resetHistory() {
    setHistory({ past: [], future: [] });
    lastHistoryActionRef.current = null;
  }

  async function selectNote(id: string) {
    flushAutosave();
    try {
      const note = await profileNotesClient.fetchNote(id);
      resetHistory();
      const editor = editorFromNote(note);
      latestEditorRef.current = editor;
      setReady({ selectedID: id, editor, savedEditor: editor, message: "" });
    } catch (error) {
      setReady({ message: errorMessage(error) });
    }
  }

  function newNote() {
    flushAutosave();
    const parentID = activeNotebookForNewNote(readyState);
    resetHistory();
    const editor = newDraftEditor(parentID);
    latestEditorRef.current = editor;
    setReady({
      selectedID: "",
      editor,
      savedEditor: { ...emptyEditor, instanceKey: editor.instanceKey },
      message: "",
    });
  }

  function newNoteInNotebook(parentID: string) {
    flushAutosave();
    resetHistory();
    const editor = newDraftEditor(parentID);
    latestEditorRef.current = editor;
    setReady({
      selectedID: "",
      selectedNotebookID: parentID,
      editor,
      savedEditor: { ...emptyEditor, instanceKey: editor.instanceKey },
      message: "",
    });
  }

  function openNotebookDialog() {
    setReady({ notebookDialogOpen: true, notebookDraft: "" });
  }

  async function createNotebook() {
    flushAutosave();
    const title = readyState.notebookDraft.trim() || "Untitled notebook";
    try {
      const response = await profileNotesClient.saveNote({
        note_id: "",
        title,
        body_markdown: "",
        expected_revision: "",
        page_type: "notebook",
        parent_id: "",
        mcp_excluded: false,
      });
      const savedID = response.note_id;
      const summary: ProfileNoteSummary = {
        note_id: savedID,
        title,
        preview: "Notebook",
        revision: response.revision,
        updated_at: response.updated_at,
        page_type: "notebook",
        mcp_excluded: false,
      };
      setReady({
        notes: sortNotes([summary, ...readyState.notes.filter((note) => noteID(note) !== savedID)]),
        selectedID: savedID,
        selectedNotebookID: "",
        editor: { ...emptyEditor, instanceKey: savedID, noteID: savedID, title, revision: response.revision || "", pageType: "notebook", updatedAt: response.updated_at || "" },
        savedEditor: { ...emptyEditor, instanceKey: savedID, noteID: savedID, title, revision: response.revision || "", pageType: "notebook", updatedAt: response.updated_at || "" },
        notebookDialogOpen: false,
        notebookDraft: "",
        message: "Notebook created.",
      });
      showToast("Notebook created.");
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  function recordBodyChange(previousBody: string, action: HistoryActionKind) {
    const now = Date.now();
    const lastAction = lastHistoryActionRef.current;
    const canCoalesce = (action === "typing" || action === "deletion")
      && lastAction?.action === action
      && now - lastAction.at < NOTE_HISTORY_GROUP_DELAY_MS;
    setHistory((current) => {
      if (canCoalesce && current.past.length) {
        return current.future.length ? { past: current.past, future: [] } : current;
      }
      return {
        past: [...current.past, { body: previousBody, action }].slice(-NOTE_HISTORY_LIMIT),
        future: [],
      };
    });
    lastHistoryActionRef.current = { action, at: now };
  }

  function syncBodyFromEditor(action: HistoryActionKind) {
    const element = bodyInputRef.current;
    if (!element) return;
    const nextBody = htmlToMarkdown(element);
    if (nextBody === readyState.editor.body) return;
    recordBodyChange(readyState.editor.body, action);
    lastRenderedBodyRef.current = nextBody;
    updateEditor({ ...readyState.editor, body: nextBody });
  }

  function inputActionKind(inputType: string): HistoryActionKind {
    if (inputType.startsWith("delete")) return "deletion";
    if (inputType === "insertFromPaste" || inputType === "insertFromDrop") return "paste";
    if (inputType.startsWith("format")) return "formatting";
    return "typing";
  }

  function syncInputFromEditor(inputType: string) {
    const action = richCommandPendingRef.current ? "formatting" : inputActionKind(inputType || "insertText");
    richCommandPendingRef.current = false;
    syncBodyFromEditor(action);
  }

  function undoBody() {
    if (!history.past.length) return;
    const previous = history.past[history.past.length - 1];
    setHistory({
      past: history.past.slice(0, -1),
      future: [{ body: readyState.editor.body, action: previous.action }, ...history.future].slice(0, NOTE_HISTORY_LIMIT),
    });
    lastHistoryActionRef.current = null;
    updateEditor({ ...readyState.editor, body: previous.body });
  }

  function redoBody() {
    if (!history.future.length) return;
    const next = history.future[0];
    setHistory({
      past: [...history.past, { body: readyState.editor.body, action: next.action }].slice(-NOTE_HISTORY_LIMIT),
      future: history.future.slice(1),
    });
    lastHistoryActionRef.current = null;
    updateEditor({ ...readyState.editor, body: next.body });
  }

  function mutateBody(mutator: (body: string, start: number, end: number) => { body: string; selectionStart: number; selectionEnd: number }) {
    const body = readyState.editor.body;
    const start = body.length;
    const end = body.length;
    const result = mutator(body, start, end);
    recordBodyChange(body, "formatting");
    pendingSelectionRef.current = { start: result.selectionStart, end: result.selectionEnd };
    updateEditor({ ...readyState.editor, body: result.body });
  }

  function selectionInEditor(): boolean {
    const element = bodyInputRef.current;
    const selection = window.getSelection?.();
    if (!element || !selection || !selection.rangeCount) return false;
    const range = selection.getRangeAt(0);
    return element.contains(range.commonAncestorContainer);
  }

  function runRichCommand(command: string, value?: string): boolean {
    const element = bodyInputRef.current;
    if (!element || typeof document.execCommand !== "function") return false;
    element.focus();
    // If the selection lives outside the note, park the caret at the end so the
    // command always applies to the note instead of silently doing nothing.
    if (!selectionInEditor()) {
      const selection = window.getSelection?.();
      if (selection) {
        const range = document.createRange();
        range.selectNodeContents(element);
        range.collapse(false);
        selection.removeAllRanges();
        selection.addRange(range);
      }
    }
    let effectiveCommand = command;
    let effectiveValue = value;
    if (command === "createLink" && (window.getSelection?.()?.isCollapsed ?? true)) {
      // createLink needs a selection; with a bare caret insert a ready-made link instead.
      effectiveCommand = "insertHTML";
      effectiveValue = `<a href="${value}" rel="noreferrer">Link</a>`;
    }
    if (command === "formatBlock" && typeof document.queryCommandValue === "function") {
      const currentBlock = String(document.queryCommandValue("formatBlock") || "").toLowerCase();
      if (currentBlock === value) effectiveValue = "p";
    }
    richCommandPendingRef.current = true;
    document.execCommand(effectiveCommand, false, effectiveValue);
    // Browsers normally emit an input event for execCommand. The fallback keeps
    // the editor in sync in environments that do not, without recording twice.
    if (richCommandPendingRef.current) {
      richCommandPendingRef.current = false;
      syncBodyFromEditor("formatting");
    }
    return true;
  }

  function applyEditorAction(action: string) {
    if (action === "kanban") {
      const board = readyState.editor.board || boardFromMarkdown(readyState.editor.body);
      updateEditor({
        ...readyState.editor,
        pageType: "kanban",
        board,
        body: boardToMarkdown(readyState.editor.title, board),
      });
      return;
    }
    if (action === "text") {
      updateEditor({
        ...readyState.editor,
        pageType: "text",
        body: readyState.editor.board ? boardToMarkdown(readyState.editor.title, readyState.editor.board) : readyState.editor.body,
        board: null,
      });
      return;
    }
    if (action === "undo") { undoBody(); return; }
    if (action === "redo") { redoBody(); return; }
    const wrappers: Record<string, { prefix: string; suffix: string; placeholder: string }> = {
      bold: { prefix: "**", suffix: "**", placeholder: "bold text" },
      italic: { prefix: "_", suffix: "_", placeholder: "italic text" },
      underline: { prefix: "<u>", suffix: "</u>", placeholder: "underlined text" },
      tag: { prefix: "#", suffix: "", placeholder: "tag" },
    };
    const headingLevels: Record<string, string> = { heading: "# ", large: "## ", small: "### " };
    if (wrappers[action]) {
      const commandForAction: Record<string, string> = { bold: "bold", italic: "italic", underline: "underline" };
      if (commandForAction[action] && runRichCommand(commandForAction[action])) return;
      const { prefix, suffix, placeholder } = wrappers[action];
      mutateBody((body, start, end) => wrapSelection(body, start, end, prefix, suffix, placeholder));
      return;
    }
    if (headingLevels[action]) {
      const tagForAction: Record<string, string> = { heading: "h1", large: "h2", small: "h3" };
      if (runRichCommand("formatBlock", tagForAction[action])) return;
      mutateBody((body, start, end) => prefixLines(body, start, end, (line) => `${headingLevels[action]}${stripLinePrefix(line)}`));
      return;
    }
    if (action === "bullets") {
      if (runRichCommand("insertUnorderedList")) return;
      mutateBody((body, start, end) => prefixLines(body, start, end, (line) => `- ${stripLinePrefix(line)}`));
      return;
    }
    if (action === "numbers") {
      if (runRichCommand("insertOrderedList")) return;
      mutateBody((body, start, end) => prefixLines(body, start, end, (line, index) => `${index + 1}. ${stripLinePrefix(line)}`));
      return;
    }
    if (action === "link") {
      if (runRichCommand("createLink", "https://example.com")) return;
      mutateBody((body, start, end) => {
        const selected = body.slice(start, end) || "Link";
        const url = "https://example.com";
        const next = `${body.slice(0, start)}[${selected}](${url})${body.slice(end)}`;
        const urlStart = start + selected.length + 3;
        return { body: next, selectionStart: urlStart, selectionEnd: urlStart + url.length };
      });
    }
  }

  async function saveNote(editor = readyState.editor, background = false): Promise<Editor | null> {
    if (!background) clearAutosaveTimer();
    if (savingRef.current) {
      const active = activeSaveRef.current;
      if (active?.editor.instanceKey === editor.instanceKey && !editorChanged(active.editor, editor)) return null;
      const pendingIndex = pendingSaveRef.current.findIndex((pending) => pending.editor.instanceKey === editor.instanceKey);
      const pending = pendingIndex >= 0 ? pendingSaveRef.current[pendingIndex] : null;
      const nextPending = {
        editor,
        background: pending?.background === false ? false : background,
      };
      if (pendingIndex >= 0) pendingSaveRef.current[pendingIndex] = nextPending;
      else pendingSaveRef.current.push(nextPending);
      return null;
    }
    savingRef.current = true;
    activeSaveRef.current = { editor, background };
    setReady({ saving: true });
    let queuedSave: PendingSave | null = null;
    let savedResult: Editor | null = null;
    try {
      const response = await profileNotesClient.saveNote({
        note_id: editor.noteID,
        title: editor.title.trim() || "Untitled",
        body_markdown: editor.body,
        expected_revision: editor.revision,
        page_type: editor.pageType,
        parent_id: editor.parentID,
        mcp_excluded: editor.mcpExcluded,
        board: editor.pageType === "kanban" ? editor.board : undefined,
      });
      const savedEditor = {
        ...editor,
        noteID: response.note_id || editor.noteID,
        revision: response.revision || editor.revision,
        updatedAt: response.updated_at || editor.updatedAt,
      };
      savedResult = savedEditor;
      if (latestEditorRef.current.instanceKey === editor.instanceKey) {
        latestEditorRef.current = {
          ...latestEditorRef.current,
          noteID: savedEditor.noteID,
          revision: savedEditor.revision,
          updatedAt: savedEditor.updatedAt,
        };
        latestSavedEditorRef.current = savedEditor;
      }
      const savedID = savedEditor.noteID;
      setState((current) => {
        if (current.status !== "ready") return current;
        const matchesCurrentEditor = current.editor.instanceKey === editor.instanceKey;
        const nextEditor = matchesCurrentEditor
          ? { ...current.editor, noteID: savedID, revision: savedEditor.revision, updatedAt: savedEditor.updatedAt }
          : current.editor;
        return {
          ...current,
          selectedID: matchesCurrentEditor ? savedID : current.selectedID,
          notes: sortNotes([
            {
              note_id: savedID,
              title: editor.title.trim() || "Untitled",
              preview: editor.pageType === "notebook" ? "Notebook" : editor.pageType === "kanban" ? "Kanban board" : editor.body.replace(/\s+/g, " ").trim().slice(0, 96),
              revision: savedEditor.revision,
              updated_at: response.updated_at,
              page_type: editor.pageType,
              parent_id: editor.pageType === "notebook" ? "" : editor.parentID,
              mcp_excluded: editor.mcpExcluded,
            },
            ...current.notes.filter((note) => noteID(note) !== savedID && noteID(note) !== editor.noteID),
          ]),
          editor: nextEditor,
          savedEditor: matchesCurrentEditor ? savedEditor : current.savedEditor,
          saving: false,
          message: background ? current.message : "Note saved.",
        };
      });
      pendingSaveRef.current = pendingSaveRef.current.map((pending) => pending.editor.instanceKey === editor.instanceKey
        ? { ...pending, editor: { ...pending.editor, noteID: savedID, revision: savedEditor.revision, updatedAt: savedEditor.updatedAt } }
        : pending);
      queuedSave = pendingSaveRef.current.shift() || null;
      if (queuedSave && queuedSave.editor.instanceKey === editor.instanceKey) {
        queuedSave = { ...queuedSave, editor: { ...queuedSave.editor, noteID: savedID, revision: savedEditor.revision, updatedAt: savedEditor.updatedAt } };
      }
      if (!background) showToast("Note saved.");
    } catch (error) {
      pendingSaveRef.current = pendingSaveRef.current.filter((pending) => pending.editor.instanceKey !== editor.instanceKey);
      queuedSave = pendingSaveRef.current.shift() || null;
      setReady({ saving: false });
      showToast(errorMessage(error), "error");
    } finally {
      activeSaveRef.current = null;
      savingRef.current = false;
      if (queuedSave) void saveNote(queuedSave.editor, queuedSave.background);
    }
    return savedResult;
  }

  async function uploadKanbanAttachment(file: File): Promise<NoteAttachment> {
    if (savingRef.current) {
      await new Promise<void>((resolve, reject) => {
        const startedAt = Date.now();
        const waitForSave = () => {
          if (!savingRef.current) { resolve(); return; }
          if (Date.now() - startedAt > 10_000) { reject(new Error("The board is still saving. Try the upload again.")); return; }
          window.setTimeout(waitForSave, 40);
        };
        waitForSave();
      });
    }
    let editor = latestEditorRef.current;
    if (!editor.noteID || editorChanged(editor, latestSavedEditorRef.current)) {
      const saved = await saveNote(editor, true);
      if (saved) editor = saved;
    }
    if (!editor.noteID) throw new Error("Save the board before adding a file.");
    const attachment = await profileNotesClient.uploadAttachment(editor.noteID, file);
    const attachments = [...editor.attachments.filter((item) => item.id !== attachment.id), attachment];
    latestEditorRef.current = { ...editor, attachments };
    latestSavedEditorRef.current = { ...latestSavedEditorRef.current, attachments };
    setState((current) => {
      if (current.status !== "ready" || current.editor.instanceKey !== editor.instanceKey) return current;
      return {
        ...current,
        editor: { ...current.editor, attachments },
        savedEditor: { ...current.savedEditor, attachments },
      };
    });
    return attachment;
  }

  async function deleteNote() {
    if (!readyState.editor.noteID) return;
    await deleteNoteByID(readyState.editor.noteID, readyState.editor.title || readyState.editor.noteID);
  }

  async function deleteNoteByID(id: string, title: string) {
    const confirmed = await dialog.confirm({
      title: "Delete note",
      message: `Delete "${title}"? This can't be undone.`,
      confirmLabel: "Delete",
      tone: "danger",
    });
    if (!confirmed) return;
    try {
      await profileNotesClient.deleteNote(id);
      const selectedDeleted = readyState.editor.noteID === id;
      setReady({
        notes: readyState.notes.filter((note) => noteID(note) !== id),
        selectedID: selectedDeleted ? "" : readyState.selectedID,
        editor: selectedDeleted ? emptyEditor : readyState.editor,
        message: "",
      });
      showToast("Note deleted.");
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  function openMoveDialog(note: ProfileNoteSummary) {
    setReady({ moveDialogNoteID: noteID(note), moveDialogTargetID: note.parent_id || "" });
  }

  async function moveNote() {
    if (!readyState.moveDialogNoteID) return;
    try {
      const note = await profileNotesClient.fetchNote(readyState.moveDialogNoteID);
      const id = noteID(note);
      const response = await profileNotesClient.saveNote({
        note_id: id,
        title: note.title?.trim() || "Untitled",
        body_markdown: note.body_markdown || note.content || "",
        expected_revision: note.revision || "",
        page_type: note.page_type || "text",
        parent_id: readyState.moveDialogTargetID,
        mcp_excluded: Boolean(note.mcp_excluded),
      });
      const movedID = response.note_id || id;
      const movedTitle = note.title?.trim() || "Untitled";
      setReady({
        notes: sortNotes([
          {
            note_id: movedID,
            title: movedTitle,
            preview: note.preview || (note.body_markdown || note.content || "").replace(/\s+/g, " ").trim().slice(0, 96),
            revision: response.revision || note.revision,
            updated_at: response.updated_at || note.updated_at,
            page_type: note.page_type || "text",
            parent_id: readyState.moveDialogTargetID,
            mcp_excluded: Boolean(note.mcp_excluded),
          },
          ...readyState.notes.filter((summary) => noteID(summary) !== movedID),
        ]),
        editor: readyState.editor.noteID === movedID ? { ...readyState.editor, parentID: readyState.moveDialogTargetID, revision: response.revision || readyState.editor.revision } : readyState.editor,
        savedEditor: readyState.editor.noteID === movedID ? { ...readyState.savedEditor, parentID: readyState.moveDialogTargetID, revision: response.revision || readyState.savedEditor.revision } : readyState.savedEditor,
        moveDialogNoteID: "",
        moveDialogTargetID: "",
        message: "Note moved.",
      });
      showToast("Note moved.");
    } catch (error) {
      showToast(errorMessage(error), "error");
    }
  }

  return (
    <section className="dashboard-page notes-guide-page" aria-labelledby="route-title">
      {state.message ? <p className="notice-state">{state.message}</p> : null}

      <div className={`notes-guide-layout${state.railOpen ? "" : " rail-closed"}`}>
        {state.railOpen ? (
          <aside className="notes-guide-rail" aria-label="Notes list">
            <header className="notes-rail-head">
              <div>
                <p className="eyebrow">Hank Remote</p>
                <h1 id="route-title">Notes</h1>
              </div>
              <div className="notes-rail-actions">
                <button className="icon-button" type="button" aria-label="Collapse notes rail" title="Collapse notes rail" onClick={() => setReady({ railOpen: false })}><Icon name="panel" /></button>
                <button className="icon-button" type="button" aria-label="New notebook" title="New notebook" onClick={openNotebookDialog}><Icon name="book-plus" /></button>
              </div>
            </header>
            <button className="notes-new-note" type="button" aria-label="New note" onClick={newNote}><Icon name="plus" />New note</button>
            <label className="notes-search">
              <Icon name="search" />
              <span className="visually-hidden">Search notes</span>
              <input type="search" placeholder="Search notes" value={state.query} onChange={(event) => setReady({ query: event.target.value })} />
            </label>
            <label className="notes-notebook-filter">
              <span>Notebook filter</span>
              <select
                aria-label="Notebook filter"
                value={state.selectedNotebookID}
                onChange={(event) => setReady({ selectedNotebookID: event.target.value })}
              >
                <option value="">All Notes</option>
                <option value={ROOT_NOTEBOOK_FILTER}>No Notebook</option>
                {notebookItems.map((note) => (
                  <option key={noteID(note)} value={noteID(note)}>{noteTitle(note)}</option>
                ))}
              </select>
            </label>
            <section className="notes-notebooks-section" aria-labelledby="notebooks-heading">
              <div className="notes-section-head">
                <h2 id="notebooks-heading">Notebooks</h2>
                <span>{visibleNotebookItems.length}</span>
              </div>
              {visibleNotebookItems.length ? (
                <div className="notes-notebook-list">
                  {visibleNotebookItems.map((note) => {
                    const id = noteID(note);
                    const title = noteTitle(note);
                    const childCount = notebookChildCount(state.notes, id);
                    const excluded = noteEffectiveMcpExcluded(note, state.notes);
                    return (
                      <button
                        aria-label={`Open notebook ${title}`}
                        className={`${id === state.selectedID ? "notes-notebook-item active" : "notes-notebook-item"}${excluded ? " is-mcp-excluded" : ""}`}
                        key={id}
                        onClick={() => void selectNote(id)}
                        type="button"
                      >
                        <span className="notes-guide-icon" aria-hidden="true"><Icon name="book" /></span>
                        <span>
                          <strong>{title}{excluded ? <span className="notes-lock-indicator" aria-hidden="true"><Icon name="lock" /></span> : null}</strong>
                          <small>{childCount} {childCount === 1 ? "page" : "pages"}</small>
                        </span>
                      </button>
                    );
                  })}
                </div>
              ) : (
                <p className="notes-notebook-empty">No notebooks yet.</p>
              )}
            </section>
            {visibleNotes.length ? (
              <div className="notes-guide-list" aria-label="Note cards">
                {visibleNotes.map((note) => {
                  const id = noteID(note);
                  const title = noteTitle(note);
                  const parentTitle = notebookTitle(state.notes, note.parent_id);
                  const excluded = noteEffectiveMcpExcluded(note, state.notes);
                  return (
                    <div className="notes-guide-row" key={id} onMouseEnter={revealRowActions} onMouseLeave={hideRowActions}>
                      <button
                        aria-label={title}
                        className={`${id === state.selectedID ? "notes-guide-item active" : "notes-guide-item"}${excluded ? " is-mcp-excluded" : ""}`}
                        onClick={() => void selectNote(id)}
                        type="button"
                      >
                        <span className="notes-guide-icon" aria-hidden="true"><Icon name={noteIconName(note)} /></span>
                        <span className="notes-guide-copy">
                          <strong>{title}{excluded ? <span className="notes-lock-indicator" aria-hidden="true"><Icon name="lock" /></span> : null}</strong>
                          <span>{note.preview || "No preview"}</span>
                          <span className="notes-tag-row"><em className="notes-tag">{parentTitle || noteTag(note)}</em><small>{updatedLabel(note)}</small></span>
                        </span>
                      </button>
                      <div className="notes-row-actions" aria-label={`Actions for ${title}`}>
                        <button className="icon-button" type="button" aria-label={`Move ${title}`} title="Move note" onClick={() => openMoveDialog(note)}><Icon name="book" /></button>
                        <button className="icon-button danger" type="button" aria-label={`Delete ${title}`} title="Delete note" onClick={() => void deleteNoteByID(id, title)}><Icon name="trash" /></button>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <p className="empty-state" aria-label="Note cards">No notes found.</p>
            )}
          </aside>
        ) : (
          <aside className="notes-rail-collapsed" aria-label="Notes rail">
            <button className="icon-button" type="button" aria-label="Expand notes rail" title="Expand notes rail" onClick={() => setReady({ railOpen: true })}><Icon name="panel" /></button>
            <button className="icon-button" type="button" aria-label="New notebook" title="New notebook" onClick={openNotebookDialog}><Icon name="book-plus" /></button>
            <button className="icon-button" type="button" aria-label="New note" title="New note" onClick={newNote}><Icon name="plus" /></button>
          </aside>
        )}

        <section className="notes-guide-editor" aria-label="Note editor">
          <header className="notes-editor-header">
            <span className="notes-title-kind" aria-hidden="true"><Icon name={noteIconName(state.editor)} /></span>
            <label className="visually-hidden" htmlFor="noteTitle">Note title</label>
            <input
              id="noteTitle"
              className="notes-title-input"
              value={state.editor.title}
              placeholder="Untitled"
              onBlur={flushAutosave}
              onChange={(event) => updateEditor({ ...state.editor, title: event.target.value })}
            />
            <label className="notes-editor-notebook">
              <span>Notebook</span>
              <select
                aria-label="Notebook"
                disabled={state.editor.pageType === "notebook"}
                value={state.editor.pageType === "notebook" ? "" : state.editor.parentID}
                onChange={(event) => {
                  const editor = { ...state.editor, parentID: event.target.value };
                  updateEditor(editor, false);
                  void saveNote(editor, true);
                }}
              >
                <option value="">No Notebook</option>
                {notebookItems.map((note) => (
                  <option key={noteID(note)} value={noteID(note)}>{noteTitle(note)}</option>
                ))}
              </select>
            </label>
            <button className="notes-save-pill" type="button" aria-label="Save note" onClick={() => void saveNote()}>
              <span>{state.saving ? "Saving…" : state.editor.noteID && !editorChanged(state.editor, state.savedEditor) ? "Saved" : "Unsaved"}</span>
              <small>{selectedSummary ? updatedLabel(selectedSummary) : "Not saved"}</small>
            </button>
            <button className="icon-button danger" disabled={!state.editor.noteID} type="button" aria-label="Delete note" title="Delete note" onClick={() => void deleteNote()}><Icon name="trash" /></button>
          </header>

          <div className="notes-toolbar" aria-label="Editor tools">
            <ToolbarButton
              label={state.editor.mcpExcluded ? "Include in MCP" : "Exclude from MCP"}
              icon={state.editor.mcpExcluded ? "unlock" : "lock"}
              pressed={state.editor.mcpExcluded}
              onClick={() => {
                const editor = { ...state.editor, mcpExcluded: !state.editor.mcpExcluded };
                updateEditor(editor, false);
                void saveNote(editor, true);
              }}
            />
            {state.editor.pageType === "notebook" ? null : (
              <>
                <span className="notes-toolbar-separator" aria-hidden="true" />
                <ToolbarButton label="Undo" icon="undo" disabled={!history.past.length} onClick={() => applyEditorAction("undo")} />
                <ToolbarButton label="Redo" icon="redo" disabled={!history.future.length} onClick={() => applyEditorAction("redo")} />
                <span className="notes-toolbar-separator" aria-hidden="true" />
                <ToolbarButton label="Bold" text="B" onClick={() => applyEditorAction("bold")} />
                <ToolbarButton label="Italic" text="I" onClick={() => applyEditorAction("italic")} />
                <ToolbarButton label="Underline" text="U" onClick={() => applyEditorAction("underline")} />
                <ToolbarButton label="Smaller heading" text="A-" onClick={() => applyEditorAction("small")} />
                <ToolbarButton label="Heading" text="H" onClick={() => applyEditorAction("heading")} />
                <ToolbarButton label="Larger heading" text="A+" onClick={() => applyEditorAction("large")} />
                <ToolbarButton label="Bulleted list" icon="list" onClick={() => applyEditorAction("bullets")} />
                <ToolbarButton label="Numbered list" icon="ordered" onClick={() => applyEditorAction("numbers")} />
                <span className="notes-toolbar-separator" aria-hidden="true" />
                <ToolbarButton label="Text page" icon="note" pressed={state.editor.pageType === "text"} onClick={() => applyEditorAction("text")} />
                <ToolbarButton label="Kanban page" icon="kanban" pressed={state.editor.pageType === "kanban"} onClick={() => applyEditorAction("kanban")} />
                <ToolbarButton label="Tag" icon="tag" onClick={() => applyEditorAction("tag")} />
                <ToolbarButton label="Link" icon="link" onClick={() => applyEditorAction("link")} />
              </>
            )}
          </div>

          <div className={`notes-content-scroll${state.editor.pageType === "kanban" ? " is-kanban" : ""}`}>
            {editorEffectiveExclusion ? (
              <p className={`notes-mcp-state${editorInheritedExclusion ? " inherited" : ""}`}>
                {editorInheritedExclusion ? "Excluded because its notebook is locked" : "Excluded from MCP"}
              </p>
            ) : null}
            {state.editor.pageType === "kanban" ? (
              <KanbanEditor
                board={state.editor.board || boardFromMarkdown(state.editor.body)}
                attachments={state.editor.attachments}
                onChange={(board) => {
                  const editor = latestEditorRef.current;
                  updateEditor({
                    ...editor,
                    board,
                    body: boardToMarkdown(editor.title, board),
                  });
                }}
                onUpload={uploadKanbanAttachment}
                confirmDelete={(message) => dialog.confirm({
                  title: "Delete from board",
                  message,
                  confirmLabel: "Delete",
                  tone: "danger",
                })}
              />
            ) : state.editor.pageType === "notebook" ? (
              <NotebookEditor
                notes={state.notes}
                parentID={state.editor.noteID}
                title={state.editor.title || "Notebook"}
                onOpenNote={(id) => void selectNote(id)}
                onNewNote={newNoteInNotebook}
              />
            ) : (
              <>
                <label className="visually-hidden" htmlFor="noteBody">Note body</label>
                <div
                  id="noteBody"
                  ref={bodyInputRef}
                  className="notes-body-input"
                  contentEditable
                  data-placeholder="Start writing here."
                  role="textbox"
                  aria-label="Note body"
                  aria-multiline="true"
                  suppressContentEditableWarning
                  onInput={(event) => syncInputFromEditor((event.nativeEvent as InputEvent).inputType)}
                />
              </>
            )}
          </div>
        </section>
      </div>

      {state.notebookDialogOpen ? (
        <NotebookDialog
          draft={state.notebookDraft}
          onDraft={(notebookDraft) => setReady({ notebookDraft })}
          onClose={() => setReady({ notebookDialogOpen: false })}
          onCreate={createNotebook}
        />
      ) : null}
      {state.moveDialogNoteID ? (
        <MoveNoteDialog
          notebooks={notebookItems}
          targetID={state.moveDialogTargetID}
          onTarget={(moveDialogTargetID) => setReady({ moveDialogTargetID })}
          onClose={() => setReady({ moveDialogNoteID: "", moveDialogTargetID: "" })}
          onMove={moveNote}
        />
      ) : null}
    </section>
  );
}

function ToolbarButton({
  label,
  icon,
  text,
  pressed,
  disabled,
  onClick,
}: {
  label: string;
  icon?: string;
  text?: string;
  pressed?: boolean;
  disabled?: boolean;
  onClick?: () => void;
}) {
  return (
    <button
      className="icon-button"
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={pressed}
      disabled={disabled}
      // Keep focus and the text selection inside the note while using the toolbar.
      onMouseDown={(event) => event.preventDefault()}
      onClick={onClick}
    >
      {icon ? <Icon name={icon} /> : <span>{text}</span>}
    </button>
  );
}

function NotebookEditor({
  notes,
  parentID,
  title,
  onOpenNote,
  onNewNote,
}: {
  notes: ProfileNoteSummary[];
  parentID: string;
  title: string;
  onOpenNote: (id: string) => void;
  onNewNote: (parentID: string) => void;
}) {
  const childNotes = notes.filter((note) => note.parent_id && note.parent_id === parentID && noteID(note));
  return (
    <div className="notebook-surface" aria-label="Notebook pages">
      <header className="notebook-hero">
        <span className="notebook-hero-icon" aria-hidden="true"><Icon name="book" /></span>
        <div>
          <h2>{title || "Notebook"}</h2>
          <p>{childNotes.length} {childNotes.length === 1 ? "page" : "pages"}</p>
        </div>
        <button
          className="notebook-new-note"
          disabled={!parentID}
          type="button"
          aria-label={`New note in ${title || "Notebook"}`}
          onClick={() => onNewNote(parentID)}
        >
          <Icon name="plus" />
          New note
        </button>
      </header>
      {childNotes.length ? (
        <div className="notebook-grid">
          {childNotes.map((note) => {
            const id = noteID(note);
            const title = noteTitle(note);
            const excluded = noteEffectiveMcpExcluded(note, notes);
            return (
              <button
                aria-label={`Open ${title}`}
                className={`notebook-card${excluded ? " is-mcp-excluded" : ""}`}
                key={id}
                onClick={() => onOpenNote(id)}
                type="button"
              >
                <span className="note-kind" aria-hidden="true"><Icon name="note" /></span>
                <strong>{title}{excluded ? <span className="notes-lock-indicator" aria-hidden="true"><Icon name="lock" /></span> : null}</strong>
                <span>{note.preview || "No preview"}</span>
              </button>
            );
          })}
        </div>
      ) : <p className="empty-state">No pages in this notebook yet.</p>}
    </div>
  );
}

function NotebookDialog({
  draft,
  onDraft,
  onClose,
  onCreate,
}: {
  draft: string;
  onDraft: (value: string) => void;
  onClose: () => void;
  onCreate: () => void;
}) {
  return (
    <div className="guide-dialog-scrim" role="presentation" onClick={onClose}>
      <section className="guide-dialog notebook-dialog" role="dialog" aria-modal="true" aria-label="New notebook" onClick={(event) => event.stopPropagation()}>
        <header>
          <span className="guide-dialog-icon" aria-hidden="true"><Icon name="book-plus" /></span>
          <h2>New notebook</h2>
          <button className="file-icon-action" type="button" aria-label="Close dialog" onClick={onClose}><Icon name="x" /></button>
        </header>
        <label className="guide-dialog-field">
          <span>Notebook name</span>
          <input autoFocus placeholder="Notebook" value={draft} onChange={(event) => onDraft(event.target.value)} />
        </label>
        <footer>
          <button className="secondary" type="button" onClick={onClose}>Cancel</button>
          <button type="button" onClick={onCreate}>Create notebook</button>
        </footer>
      </section>
    </div>
  );
}

function MoveNoteDialog({
  notebooks,
  targetID,
  onTarget,
  onClose,
  onMove,
}: {
  notebooks: ProfileNoteSummary[];
  targetID: string;
  onTarget: (value: string) => void;
  onClose: () => void;
  onMove: () => void;
}) {
  return (
    <div className="guide-dialog-scrim" role="presentation" onClick={onClose}>
      <section className="guide-dialog notebook-dialog" role="dialog" aria-modal="true" aria-label="Move note" onClick={(event) => event.stopPropagation()}>
        <header>
          <span className="guide-dialog-icon" aria-hidden="true"><Icon name="book" /></span>
          <h2>Move note</h2>
          <button className="file-icon-action" type="button" aria-label="Close dialog" onClick={onClose}><Icon name="x" /></button>
        </header>
        <label className="guide-dialog-field">
          <span>Move to notebook</span>
          <select autoFocus aria-label="Move to notebook" value={targetID} onChange={(event) => onTarget(event.target.value)}>
            <option value="">No Notebook</option>
            {notebooks.map((note) => (
              <option key={noteID(note)} value={noteID(note)}>{noteTitle(note)}</option>
            ))}
          </select>
        </label>
        <footer>
          <button className="secondary" type="button" onClick={onClose}>Cancel</button>
          <button type="button" onClick={onMove}>Move note</button>
        </footer>
      </section>
    </div>
  );
}
