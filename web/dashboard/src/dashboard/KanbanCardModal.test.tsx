import { cleanup, createEvent, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KanbanCardModal, type KanbanCardModalProps } from "./KanbanCardModal";

function modalProps(overrides: Partial<KanbanCardModalProps> = {}): KanbanCardModalProps {
  return {
    card: { id: "brief", text: "Review brief\nRead the scope", sort_order: 0, color: "cyan" },
    title: "Review brief",
    description: "Read the scope",
    columnID: "inbox",
    columns: [
      { id: "inbox", title: "Inbox", sort_order: 0, cards: [] },
      { id: "doing", title: "In progress", sort_order: 1, cards: [] },
    ],
    attachments: [],
    uploading: false,
    deleting: false,
    uploadError: "",
    onTitleChange: vi.fn(),
    onDescriptionChange: vi.fn(),
    onFormat: vi.fn(),
    onMove: vi.fn(),
    onDueDateChange: vi.fn(),
    onColorChange: vi.fn(),
    onUploadFiles: vi.fn(async () => undefined),
    onMoveLeft: vi.fn(),
    onMoveRight: vi.fn(),
    canMoveLeft: false,
    canMoveRight: true,
    onDelete: vi.fn(),
    onClose: vi.fn(),
    ...overrides,
  };
}

describe("KanbanCardModal", () => {
  afterEach(cleanup);

  it("opens as a modal card and focuses its title", () => {
    render(<KanbanCardModal {...modalProps()} />);

    const dialog = screen.getByRole("dialog", { name: "Task details" });
    const title = screen.getByLabelText("Task title");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveClass("color-cyan");
    expect(title).toHaveFocus();
  });

  it("closes from Escape and the backdrop but not an inside click", () => {
    const onClose = vi.fn();
    render(<KanbanCardModal {...modalProps({ onClose })} />);
    const dialog = screen.getByRole("dialog", { name: "Task details" });

    fireEvent.mouseDown(dialog);
    expect(onClose).not.toHaveBeenCalled();

    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);

    onClose.mockClear();
    fireEvent.mouseDown(screen.getByTestId("kanban-card-modal-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("wraps keyboard focus within the modal", () => {
    render(<KanbanCardModal {...modalProps()} />);
    const title = screen.getByLabelText("Task title");
    const lastControl = screen.getByLabelText("Add screenshot or file");
    const dialog = screen.getByRole("dialog", { name: "Task details" });

    lastControl.focus();
    fireEvent.keyDown(dialog, { key: "Tab" });
    expect(title).toHaveFocus();

    title.focus();
    fireEvent.keyDown(dialog, { key: "Tab", shiftKey: true });
    expect(lastControl).toHaveFocus();
  });

  it("routes pasted images through inline upload at the caret", () => {
    const onUploadFiles = vi.fn(async () => undefined);
    render(<KanbanCardModal {...modalProps({ onUploadFiles, description: "Before after" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Edit description" }));
    const description = screen.getByLabelText("Description") as HTMLTextAreaElement;
    description.setSelectionRange(7, 7);
    const image = new File(["png"], "capture.png", { type: "image/png" });
    const paste = createEvent.paste(description, {
      bubbles: true,
      cancelable: true,
      clipboardData: { items: [{ kind: "file", type: "image/png", getAsFile: () => image }] },
    });

    fireEvent(description, paste);

    expect(paste.defaultPrevented).toBe(true);
    expect(onUploadFiles).toHaveBeenCalledWith([image], { start: 7, end: 7 });
  });

  it("leaves ordinary text paste to the browser", () => {
    const onUploadFiles = vi.fn(async () => undefined);
    render(<KanbanCardModal {...modalProps({ onUploadFiles })} />);
    fireEvent.click(screen.getByRole("button", { name: "Edit description" }));
    const description = screen.getByLabelText("Description");
    const paste = createEvent.paste(description, {
      bubbles: true,
      cancelable: true,
      clipboardData: { items: [{ kind: "string", type: "text/plain", getAsFile: () => null }] },
    });

    fireEvent(description, paste);

    expect(paste.defaultPrevented).toBe(false);
    expect(onUploadFiles).not.toHaveBeenCalled();
  });

  it("renders referenced screenshots inside the description instead of exposing their markdown", () => {
    const attachment = {
      id: "natt-1",
      filename: "capture.png",
      content_type: "image/png",
      download_url: "/v1/me/notes/work/attachments/natt-1",
      markdown_reference: "![capture.png](hank-note-attachment://natt-1)",
    };
    render(<KanbanCardModal {...modalProps({ description: attachment.markdown_reference, attachments: [attachment] })} />);

    const descriptionSection = screen.getByRole("heading", { name: "Description" }).closest("section");
    expect(descriptionSection).not.toBeNull();
    expect(within(descriptionSection!).getByRole("img", { name: "capture.png" })).toHaveAttribute("src", attachment.download_url);
    expect(within(descriptionSection!).queryByText(attachment.markdown_reference)).not.toBeInTheDocument();

    fireEvent.click(within(descriptionSection!).getByRole("button", { name: "Edit description" }));
    expect(screen.getByLabelText("Description")).toHaveValue(attachment.markdown_reference);
  });

  it("opens the editor when the description surface is clicked", () => {
    render(<KanbanCardModal {...modalProps()} />);

    fireEvent.click(screen.getByTestId("kanban-description-preview"));

    expect(screen.getByLabelText("Description")).toHaveFocus();
  });

  it("keeps all task options together above the description", () => {
    render(<KanbanCardModal {...modalProps()} />);
    const options = screen.getByTestId("kanban-card-options");
    const description = screen.getByRole("heading", { name: "Description" }).closest("section");

    expect(screen.getByLabelText("Task title")).toHaveAttribute("rows", "1");
    expect(within(options).getByLabelText("Column")).toBeInTheDocument();
    expect(within(options).getByLabelText("Due date")).toBeInTheDocument();
    expect(within(options).getByRole("button", { name: "Move task right" })).toBeInTheDocument();
    expect(within(options).getByRole("button", { name: "Delete task" })).toBeInTheDocument();
    expect(options.compareDocumentPosition(description!)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });
});
