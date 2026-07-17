import { useRef, type ClipboardEvent, type KeyboardEvent, type MouseEvent, type ReactNode } from "react";
import type { KanbanCard, KanbanColumn, NoteAttachment } from "../api/profileNotes";

export type DescriptionSelection = { start: number; end: number };

export type KanbanCardModalProps = {
  card: KanbanCard;
  title: string;
  description: string;
  columnID: string;
  columns: KanbanColumn[];
  attachments: NoteAttachment[];
  uploading: boolean;
  uploadError: string;
  onTitleChange: (value: string) => void;
  onDescriptionChange: (value: string) => void;
  onFormat: (prefix: string, suffix?: string, placeholder?: string, selection?: DescriptionSelection) => void;
  onMove: (columnID: string) => void;
  onDueDateChange: (value: string) => void;
  onColorChange: (value: string) => void;
  onUploadFiles: (files: File[], selection?: DescriptionSelection) => Promise<void>;
  onMoveLeft: () => void;
  onMoveRight: () => void;
  canMoveLeft: boolean;
  canMoveRight: boolean;
  onDelete: () => void;
  onClose: () => void;
};

const cardColors = ["default", "cyan", "blue", "green", "amber", "violet"] as const;
const focusableSelector = "textarea:not([disabled]), button:not([disabled]), input:not([disabled]), select:not([disabled]), a[href]";

function ModalIcon({ name }: { name: "close" | "upload" | "left" | "right" | "trash" }) {
  const paths: Record<string, ReactNode> = {
    close: <path d="m7 7 10 10M17 7 7 17" />,
    upload: <><path d="M12 16V5m0 0L8 9m4-4 4 4" /><path d="M5 15v4h14v-4" /></>,
    left: <path d="m14 6-6 6 6 6" />,
    right: <path d="m10 6 6 6-6 6" />,
    trash: <><path d="M5 7h14M9 7V5h6v2M8 10v8M12 10v8M16 10v8" /></>,
  };
  return <svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>;
}

function referencedAttachments(description: string, attachments: NoteAttachment[]): NoteAttachment[] {
  return attachments.filter((attachment) => description.includes(`hank-note-attachment://${attachment.id}`));
}

export function KanbanCardModal(props: KanbanCardModalProps) {
  const {
    card, title, description, columnID, columns, attachments, uploading, uploadError,
    onTitleChange, onDescriptionChange, onFormat, onMove, onDueDateChange, onColorChange,
    onUploadFiles, onMoveLeft, onMoveRight, canMoveLeft, canMoveRight, onDelete, onClose,
  } = props;
  const dialogRef = useRef<HTMLElement>(null);
  const descriptionRef = useRef<HTMLTextAreaElement>(null);
  const cardAttachments = referencedAttachments(description, attachments);
  const currentColumn = columns.find((column) => column.id === columnID);

  function handleDialogKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>(focusableSelector) || []);
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  function handleBackdropMouseDown(event: MouseEvent<HTMLDivElement>) {
    if (event.target === event.currentTarget) onClose();
  }

  function formatDescription(prefix: string, suffix = prefix, placeholder = "text") {
    const input = descriptionRef.current;
    const start = input?.selectionStart ?? description.length;
    const end = input?.selectionEnd ?? start;
    const selectedText = description.slice(start, end) || placeholder;
    onFormat(prefix, suffix, placeholder, { start, end });
    requestAnimationFrame(() => {
      input?.focus();
      input?.setSelectionRange(start + prefix.length, start + prefix.length + selectedText.length);
    });
  }

  function handleDescriptionPaste(event: ClipboardEvent<HTMLTextAreaElement>) {
    const files = Array.from(event.clipboardData.items)
      .filter((item) => item.kind === "file" && item.type.startsWith("image/"))
      .map((item) => item.getAsFile())
      .filter((file): file is File => Boolean(file));
    if (!files.length) return;
    event.preventDefault();
    const input = event.currentTarget;
    const selection = { start: input.selectionStart, end: input.selectionEnd };
    void onUploadFiles(files, selection).then(() => {
      requestAnimationFrame(() => {
        const current = descriptionRef.current;
        if (!current) return;
        current.focus();
        current.setSelectionRange(current.value.length, current.value.length);
      });
    });
  }

  return (
    <div className="kanban-card-modal-backdrop" data-testid="kanban-card-modal-backdrop" onMouseDown={handleBackdropMouseDown}>
      <article
        ref={dialogRef}
        className={`kanban-card-modal color-${card.color || "default"}`}
        role="dialog"
        aria-modal="true"
        aria-label="Task details"
        onKeyDown={handleDialogKeyDown}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="kanban-card-modal-header">
          <label className="kanban-detail-title">
            <span>Task in {currentColumn?.title || "board"}</span>
            <textarea autoFocus aria-label="Task title" rows={2} value={title} onChange={(event) => onTitleChange(event.target.value)} />
          </label>
          <button type="button" aria-label="Close task details" onClick={onClose}><ModalIcon name="close" /></button>
        </header>

        <div className="kanban-card-modal-scroll">
          <section className="kanban-detail-section">
            <h3>Description</h3>
            <div className="kanban-formatbar" aria-label="Description formatting">
              <button type="button" aria-label="Bold" onClick={() => formatDescription("**", "**", "bold text")}><strong>B</strong></button>
              <button type="button" aria-label="Italic" onClick={() => formatDescription("_", "_", "italic text")}><em>I</em></button>
              <button type="button" aria-label="Bulleted list" onClick={() => formatDescription("- ", "", "list item")}>• List</button>
              <button type="button" aria-label="Link" onClick={() => formatDescription("[", "](https://example.com)", "link title")}>Link</button>
            </div>
            <label>
              <span className="visually-hidden">Description</span>
              <textarea
                ref={descriptionRef}
                aria-label="Description"
                rows={9}
                value={description}
                onChange={(event) => onDescriptionChange(event.target.value)}
                onPaste={handleDescriptionPaste}
                placeholder="Add context, links, checklists, or notes…"
              />
            </label>
          </section>

          <div className="kanban-detail-grid">
            <label>
              <span>Column</span>
              <select value={columnID} onChange={(event) => onMove(event.target.value)}>
                {columns.map((column) => <option key={column.id} value={column.id}>{column.title}</option>)}
              </select>
            </label>
            <label>
              <span>Due date</span>
              <input aria-label="Due date" type="date" value={card.due_date || ""} onChange={(event) => onDueDateChange(event.target.value)} />
            </label>
          </div>

          <section className="kanban-detail-section">
            <h3>Card color</h3>
            <div className="kanban-color-picker">
              {cardColors.map((color) => (
                <button
                  className={`color-${color}`}
                  type="button"
                  key={color}
                  aria-label={`${color === "default" ? "Default" : color[0].toUpperCase() + color.slice(1)} card`}
                  aria-pressed={(card.color || "default") === color}
                  onClick={() => onColorChange(color === "default" ? "" : color)}
                />
              ))}
            </div>
          </section>

          <section className="kanban-detail-section">
            <h3>Links &amp; files</h3>
            <label
              className={`kanban-upload${uploading ? " is-uploading" : ""}`}
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault();
                void onUploadFiles(Array.from(event.dataTransfer.files));
              }}
            >
              <ModalIcon name="upload" />
              <span>{uploading ? "Uploading…" : "Drop screenshots here or choose files"}</span>
              <input aria-label="Add screenshot or file" type="file" multiple disabled={uploading} onChange={(event) => void onUploadFiles(Array.from(event.target.files || []))} />
            </label>
            {uploadError ? <p className="kanban-upload-error" role="alert">{uploadError}</p> : null}
            {cardAttachments.some((attachment) => attachment.content_type.startsWith("image/")) ? (
              <div className="kanban-card-modal-media">
                {cardAttachments.filter((attachment) => attachment.content_type.startsWith("image/")).map((attachment) => (
                  <a key={attachment.id} href={attachment.download_url} target="_blank" rel="noopener noreferrer">
                    <img src={attachment.download_url} alt={attachment.filename} />
                  </a>
                ))}
              </div>
            ) : null}
            <div className="kanban-attachment-list">
              {cardAttachments.map((attachment) => (
                <a key={attachment.id} href={attachment.download_url} target="_blank" rel="noopener noreferrer">{attachment.filename}</a>
              ))}
            </div>
          </section>
        </div>

        <footer className="kanban-card-modal-footer">
          <div>
            <button type="button" aria-label="Move task left" disabled={!canMoveLeft} onClick={onMoveLeft}><ModalIcon name="left" /> Move left</button>
            <button type="button" aria-label="Move task right" disabled={!canMoveRight} onClick={onMoveRight}>Move right <ModalIcon name="right" /></button>
          </div>
          <button className="danger" type="button" aria-label="Delete task" onClick={onDelete}><ModalIcon name="trash" /> Delete task</button>
        </footer>
      </article>
    </div>
  );
}
