import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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
    const deleteButton = screen.getByRole("button", { name: "Delete task" });
    const dialog = screen.getByRole("dialog", { name: "Task details" });

    deleteButton.focus();
    fireEvent.keyDown(dialog, { key: "Tab" });
    expect(title).toHaveFocus();

    title.focus();
    fireEvent.keyDown(dialog, { key: "Tab", shiftKey: true });
    expect(deleteButton).toHaveFocus();
  });
});
