import { cleanup, createEvent, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { KanbanBoard, NoteAttachment } from "../api/profileNotes";
import { boardFromMarkdown, boardToMarkdown, KanbanEditor } from "./KanbanEditor";

const attachments: NoteAttachment[] = [{
  id: "natt-1",
  filename: "wireframe.png",
  content_type: "image/png",
  download_url: "/v1/me/notes/work/attachments/natt-1",
  markdown_reference: "![wireframe.png](hank-note-attachment://natt-1?filename=wireframe.png&scope=profile)",
}];

function workBoard(): KanbanBoard {
  return {
    columns: [
      {
        id: "inbox",
        title: "Inbox",
        sort_order: 0,
        cards: [{ id: "brief", text: "Review brief\nRead **scope** and [open Ajera](https://example.com).", sort_order: 0 }],
      },
      { id: "doing", title: "In progress", sort_order: 1, cards: [] },
      { id: "done", title: "Done", sort_order: 2, cards: [] },
    ],
  };
}

function Harness({ initial = workBoard(), onUpload = vi.fn() }: { initial?: KanbanBoard; onUpload?: (file: File) => Promise<NoteAttachment> }) {
  let board = initial;
  const change = vi.fn((next: KanbanBoard) => { board = next; rerender(); });
  const props = () => ({
    board,
    attachments,
    onChange: change,
    onUpload,
    confirmDelete: vi.fn(async () => true),
  });
  const rendered = render(<KanbanEditor {...props()} />);
  const rerender = () => rendered.rerender(<KanbanEditor {...props()} />);
  return { change };
}

describe("KanbanEditor", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });
  it("migrates legacy markdown into stable board data and back", () => {
    const board = boardFromMarkdown("# Client Work\n\n## Inbox\n- Review brief\n\n## Doing\n- Draft proposal");

    expect(board.columns?.map((column) => column.title)).toEqual(["Inbox", "Doing"]);
    expect(board.columns?.[0].cards?.[0].text).toBe("Review brief");
    expect(board.columns?.[0].id).toBeTruthy();
    expect(boardToMarkdown("Client Work", board)).toContain("## Doing\n- Draft proposal");
  });

  it("adds, edits, formats, searches, and explicitly moves a task", async () => {
    const { change } = Harness({});

    expect(screen.getByRole("link", { name: "open Ajera" })).toHaveAttribute("href", "https://example.com");
    fireEvent.click(screen.getAllByRole("button", { name: "Add task to Inbox" })[0]);
    fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "Prepare invoice" } });
    fireEvent.click(screen.getByRole("button", { name: "Create task" }));
    expect(screen.getByRole("button", { name: "Open task Prepare invoice" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open task Prepare invoice" }));
    const modal = screen.getByRole("dialog", { name: "Task details" });
    expect(modal).toHaveAttribute("aria-modal", "true");
    fireEvent.click(within(modal).getByRole("button", { name: "Edit description" }));
    fireEvent.change(within(modal).getByLabelText("Description"), { target: { value: "Send today" } });
    fireEvent.click(within(modal).getByRole("button", { name: "Bold" }));
    fireEvent.change(within(modal).getByLabelText("Due date"), { target: { value: "2026-07-18" } });
    fireEvent.click(within(modal).getByRole("button", { name: "Cyan card" }));
    fireEvent.click(within(modal).getByRole("button", { name: "Move task right" }));

    expect(screen.getByRole("heading", { name: "In progress" }).closest("section")).toHaveTextContent("Prepare invoice");
    expect(screen.getByRole("dialog", { name: "Task details" })).toBeInTheDocument();
    expect(within(modal).getByLabelText("Due date")).toHaveValue("2026-07-18");
    expect(within(modal).getByRole("button", { name: "Cyan card" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("time")).toHaveAttribute("datetime", "2026-07-18");
    expect(change).toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Search cards"), { target: { value: "invoice" } });
    expect(screen.getByRole("button", { name: "Open task Prepare invoice" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open task Review brief" })).not.toBeInTheDocument();
  });

  it("moves a dragged card without opening its editor", () => {
    Harness({});
    const open = screen.getByRole("button", { name: "Open task Review brief" });
    const card = open.closest("article");
    const target = screen.getByRole("heading", { name: "In progress" }).closest("section");
    const dataTransfer = { effectAllowed: "none", setData: vi.fn() };

    expect(card).not.toBeNull();
    expect(target).not.toBeNull();
    fireEvent.dragStart(card!, { dataTransfer });
    fireEvent.drop(target!, { dataTransfer });
    fireEvent.dragEnd(card!, { dataTransfer });

    expect(target).toHaveTextContent("Review brief");
    expect(screen.queryByRole("dialog", { name: "Task details" })).not.toBeInTheDocument();
  });

  it("allows a deliberate click after a dropped card unmounts before dragend", async () => {
    vi.useFakeTimers();
    Harness({});
    const open = screen.getByRole("button", { name: "Open task Review brief" });
    const card = open.closest("article");
    const target = screen.getByRole("heading", { name: "In progress" }).closest("section");
    const dataTransfer = { effectAllowed: "none", setData: vi.fn() };

    expect(card).not.toBeNull();
    expect(target).not.toBeNull();
    fireEvent.dragStart(card!, { dataTransfer });
    fireEvent.drop(target!, { dataTransfer });

    const moved = screen.getByRole("button", { name: "Open task Review brief" });
    fireEvent.click(moved);
    expect(screen.queryByRole("dialog", { name: "Task details" })).not.toBeInTheDocument();

    await vi.runAllTimersAsync();
    fireEvent.click(moved);
    expect(screen.getByRole("dialog", { name: "Task details" })).toBeInTheDocument();
    vi.useRealTimers();
  });

  it("suppresses the click emitted immediately after dragging", () => {
    Harness({});
    const open = screen.getByRole("button", { name: "Open task Review brief" });
    const card = open.closest("article");
    const dataTransfer = { effectAllowed: "none", setData: vi.fn() };

    expect(card).not.toBeNull();
    fireEvent.dragStart(card!, { dataTransfer });
    fireEvent.dragEnd(card!, { dataTransfer });
    fireEvent.click(open);

    expect(screen.queryByRole("dialog", { name: "Task details" })).not.toBeInTheDocument();
  });

  it("uploads and renders a screenshot in a card", async () => {
    const upload = vi.fn(async () => attachments[0]);
    Harness({ onUpload: upload });
    fireEvent.click(screen.getByRole("button", { name: "Open task Review brief" }));
    const drawer = screen.getByRole("dialog", { name: "Task details" });
    const file = new File(["image"], "wireframe.png", { type: "image/png" });

    fireEvent.change(within(drawer).getByLabelText("Add screenshot or file"), { target: { files: [file] } });

    expect(upload).toHaveBeenCalledWith(file);
    expect(await within(drawer).findByRole("img", { name: "wireframe.png" })).toHaveAttribute("src", attachments[0].download_url);
    expect(within(screen.getByRole("button", { name: "Open task Review brief" })).getByRole("img", { name: "wireframe.png" })).toHaveAttribute("src", attachments[0].download_url);
  });

  it("keeps successful pasted screenshots when a later upload fails", async () => {
    const upload = vi.fn()
      .mockResolvedValueOnce(attachments[0])
      .mockRejectedValueOnce(new Error("Second screenshot failed"));
    const { change } = Harness({ onUpload: upload });
    fireEvent.click(screen.getByRole("button", { name: "Open task Review brief" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit description" }));
    const description = screen.getByLabelText("Description") as HTMLTextAreaElement;
    description.setSelectionRange(description.value.length, description.value.length);
    const first = new File(["one"], "wireframe.png", { type: "image/png" });
    const second = new File(["two"], "second.png", { type: "image/png" });
    const paste = createEvent.paste(description, {
      bubbles: true,
      cancelable: true,
      clipboardData: {
        items: [
          { kind: "file", type: "image/png", getAsFile: () => first },
          { kind: "file", type: "image/png", getAsFile: () => second },
        ],
      },
    });

    fireEvent(description, paste);

    expect(await screen.findByRole("alert")).toHaveTextContent("Second screenshot failed");
    expect(upload).toHaveBeenNthCalledWith(1, first);
    expect(upload).toHaveBeenNthCalledWith(2, second);
    await waitFor(() => {
      const latest = change.mock.calls.at(-1)?.[0];
      const saved = latest?.columns?.flatMap((column) => column.cards || []).find((card) => card.id === "brief");
      expect(saved?.text).toContain("hank-note-attachment://natt-1");
    });
  });
});
