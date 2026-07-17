import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type DragEvent,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import type { KanbanBoard, KanbanCard, KanbanColumn, NoteAttachment } from "../api/profileNotes";
import { KanbanCardModal, type DescriptionSelection } from "./KanbanCardModal";

type CardLocation = { columnID: string; cardID: string };

type KanbanEditorProps = {
  board: KanbanBoard;
  attachments?: NoteAttachment[];
  onChange: (board: KanbanBoard) => void;
  onUpload: (file: File) => Promise<NoteAttachment>;
  confirmDelete: (message: string) => Promise<boolean>;
};

const defaultColumnTitles = ["Inbox", "In progress", "Done"];
let fallbackID = 0;

function uniqueID(prefix: string): string {
  const uuid = globalThis.crypto?.randomUUID?.();
  if (uuid) return `${prefix}-${uuid}`;
  fallbackID += 1;
  return `${prefix}-${fallbackID}`;
}

function ordered<T extends { sort_order?: number }>(items: T[] | undefined): T[] {
  return [...(items || [])].sort((left, right) => (left.sort_order ?? 0) - (right.sort_order ?? 0));
}

function normalizeBoard(board: KanbanBoard): KanbanBoard {
  return {
    columns: ordered(board.columns).map((column, columnIndex) => ({
      ...column,
      id: column.id || uniqueID("column"),
      title: column.title?.trim() || "Untitled column",
      sort_order: columnIndex,
      cards: ordered(column.cards).map((card, cardIndex) => ({
        ...card,
        id: card.id || uniqueID("card"),
        text: card.text || card.title || "Untitled task",
        sort_order: cardIndex,
      })),
    })),
  };
}

export function boardFromMarkdown(body: string): KanbanBoard {
  const columns: KanbanColumn[] = [];
  let current: KanbanColumn | null = null;
  for (const rawLine of body.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (line.startsWith("## ")) {
      current = {
        id: uniqueID("column"),
        title: line.replace(/^##\s+/, "").trim() || "Untitled column",
        sort_order: columns.length,
        cards: [],
      };
      columns.push(current);
      continue;
    }
    if (!current || !/^[-*]\s+/.test(line)) continue;
    const text = line.replace(/^[-*]\s+/, "").trim();
    if (!text) continue;
    current.cards?.push({ id: uniqueID("card"), text, sort_order: current.cards.length });
  }
  if (!columns.length) {
    return {
      columns: defaultColumnTitles.map((title, index) => ({
        id: uniqueID("column"),
        title,
        sort_order: index,
        cards: [],
      })),
    };
  }
  return normalizeBoard({ columns });
}

export function boardToMarkdown(title: string, board: KanbanBoard): string {
  const lines = [`# ${title.trim() || "Board"}`, ""];
  for (const column of ordered(board.columns)) {
    lines.push(`## ${column.title?.trim() || "Untitled column"}`);
    for (const card of ordered(column.cards)) {
      const cardTitle = cardTitleAndDescription(card).title;
      if (cardTitle) lines.push(`- ${cardTitle}`);
    }
    lines.push("");
  }
  return lines.join("\n").trim();
}

function cardTitleAndDescription(card: KanbanCard): { title: string; description: string } {
  const lines = String(card.text || card.title || "").split(/\r?\n/);
  const titleIndex = lines.findIndex((line) => line.trim());
  if (titleIndex < 0) return { title: "Untitled task", description: "" };
  return {
    title: lines[titleIndex].trim(),
    description: lines.slice(titleIndex + 1).join("\n").trim(),
  };
}

function cardText(title: string, description: string): string {
  return [title.trim() || "Untitled task", description.trim()].filter(Boolean).join("\n");
}

function safeHref(value: string): string {
  const href = value.trim();
  return /^(https?:|mailto:|\/|#)/i.test(href) ? href : "";
}

function attachmentForReference(reference: string, attachments: NoteAttachment[]): NoteAttachment | undefined {
  const match = /^hank-note-attachment:\/\/(.+)$/i.exec(reference.trim());
  if (!match) return undefined;
  const attachmentID = match[1].split(/[?#]/, 1)[0];
  return attachments.find((attachment) => attachment.id === attachmentID);
}

function renderInline(value: string, attachments: NoteAttachment[], keyPrefix: string): ReactNode[] {
  const tokenPattern = /(!\[([^\]]*)\]\(([^)]+)\)|\[([^\]]+)\]\(([^)]+)\)|\*\*([^*\n]+)\*\*|_([^_\n]+)_)/g;
  const nodes: ReactNode[] = [];
  let cursor = 0;
  let tokenIndex = 0;
  for (const match of value.matchAll(tokenPattern)) {
    const index = match.index || 0;
    if (index > cursor) nodes.push(value.slice(cursor, index));
    const key = `${keyPrefix}-${tokenIndex}`;
    if (match[2] !== undefined) {
      const attachment = attachmentForReference(match[3], attachments);
      const src = attachment?.download_url || safeHref(match[3]);
      if (src) nodes.push(<img className="kanban-card-image" key={key} src={src} alt={match[2] || attachment?.filename || "Task attachment"} />);
      else nodes.push(match[2] || "Attachment");
    } else if (match[4] !== undefined) {
      const href = safeHref(match[5]);
      nodes.push(href ? <a key={key} href={href} target="_blank" rel="noopener noreferrer" onClick={(event) => event.stopPropagation()}>{match[4]}</a> : match[4]);
    } else if (match[6] !== undefined) {
      nodes.push(<strong key={key}>{match[6]}</strong>);
    } else if (match[7] !== undefined) {
      nodes.push(<em key={key}>{match[7]}</em>);
    }
    cursor = index + match[0].length;
    tokenIndex += 1;
  }
  if (cursor < value.length) nodes.push(value.slice(cursor));
  return nodes;
}

function CardDescription({ value, attachments }: { value: string; attachments: NoteAttachment[] }) {
  if (!value.trim()) return null;
  return (
    <div className="kanban-card-description">
      {value.split(/\r?\n/).filter(Boolean).slice(0, 5).map((line, index) => {
        const bullet = /^[-*]\s+/.test(line);
        const content = line.replace(/^[-*]\s+/, "");
        return bullet
          ? <p className="kanban-card-bullet" key={`${index}-${line}`}>{renderInline(content, attachments, `bullet-${index}`)}</p>
          : <p key={`${index}-${line}`}>{renderInline(content, attachments, `line-${index}`)}</p>;
      })}
    </div>
  );
}

function SmallIcon({ name }: { name: "plus" | "search" | "left" | "right" | "trash" | "close" | "grip" | "upload" }) {
  const paths: Record<string, ReactNode> = {
    plus: <path d="M12 5v14M5 12h14" />,
    search: <><circle cx="11" cy="11" r="6" /><path d="m16 16 4 4" /></>,
    left: <path d="m14 6-6 6 6 6" />,
    right: <path d="m10 6 6 6-6 6" />,
    trash: <><path d="M5 7h14M9 7V5h6v2M8 10v8M12 10v8M16 10v8" /></>,
    close: <path d="m7 7 10 10M17 7 7 17" />,
    grip: <><circle cx="9" cy="8" r=".8" fill="currentColor" /><circle cx="15" cy="8" r=".8" fill="currentColor" /><circle cx="9" cy="12" r=".8" fill="currentColor" /><circle cx="15" cy="12" r=".8" fill="currentColor" /><circle cx="9" cy="16" r=".8" fill="currentColor" /><circle cx="15" cy="16" r=".8" fill="currentColor" /></>,
    upload: <><path d="M12 16V5m0 0L8 9m4-4 4 4" /><path d="M5 15v4h14v-4" /></>,
  };
  return <svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>;
}

export function KanbanEditor({ board, attachments = [], onChange, onUpload, confirmDelete }: KanbanEditorProps) {
  const normalized = useMemo(() => normalizeBoard(board), [board]);
  const columns = normalized.columns || [];
  const [query, setQuery] = useState("");
  const [addingColumnID, setAddingColumnID] = useState("");
  const [taskDraft, setTaskDraft] = useState("");
  const [editingColumnID, setEditingColumnID] = useState("");
  const [selected, setSelected] = useState<CardLocation | null>(null);
  const [dragging, setDragging] = useState<CardLocation | null>(null);
  const [dropTarget, setDropTarget] = useState("");
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState("");
  const suppressOpenRef = useRef(false);
  const dragReleaseTimerRef = useRef<number | null>(null);
  const cardButtonRefs = useRef(new Map<string, HTMLButtonElement>());

  const selectedColumn = selected ? columns.find((column) => column.id === selected.columnID) : undefined;
  const selectedCard = selectedColumn?.cards?.find((card) => card.id === selected?.cardID);
  const selectedParts = selectedCard ? cardTitleAndDescription(selectedCard) : null;

  useEffect(() => {
    if (!selected || selectedCard) return;
    setSelected(null);
  }, [selected, selectedCard]);

  useEffect(() => () => {
    if (dragReleaseTimerRef.current !== null) window.clearTimeout(dragReleaseTimerRef.current);
  }, []);

  function commit(next: KanbanBoard) {
    onChange(normalizeBoard(next));
  }

  function updateColumn(columnID: string, updater: (column: KanbanColumn) => KanbanColumn) {
    commit({ columns: columns.map((column) => column.id === columnID ? updater(column) : column) });
  }

  function updateCard(location: CardLocation, updater: (card: KanbanCard) => KanbanCard) {
    updateColumn(location.columnID, (column) => ({
      ...column,
      cards: ordered(column.cards).map((card) => card.id === location.cardID ? updater(card) : card),
    }));
  }

  function addTask(columnID: string) {
    const title = taskDraft.trim();
    if (!title) return;
    const card: KanbanCard = { id: uniqueID("card"), text: title, sort_order: 0 };
    updateColumn(columnID, (column) => ({ ...column, cards: [...ordered(column.cards), card] }));
    setTaskDraft("");
    setAddingColumnID("");
  }

  function moveColumn(columnID: string, direction: -1 | 1) {
    const index = columns.findIndex((column) => column.id === columnID);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= columns.length) return;
    const next = [...columns];
    [next[index], next[target]] = [next[target], next[index]];
    commit({ columns: next });
  }

  async function deleteColumn(column: KanbanColumn) {
    const count = column.cards?.length || 0;
    if (count && !await confirmDelete(`Delete “${column.title}” and its ${count} ${count === 1 ? "task" : "tasks"}?`)) return;
    commit({ columns: columns.filter((item) => item.id !== column.id) });
  }

  function moveCard(
    location: CardLocation,
    targetColumnID: string,
    targetIndex = Number.MAX_SAFE_INTEGER,
    preserveEditor = selected?.cardID === location.cardID,
  ) {
    const sourceColumn = columns.find((column) => column.id === location.columnID);
    const card = sourceColumn?.cards?.find((item) => item.id === location.cardID);
    if (!card) return;
    const nextColumns = columns.map((column) => ({
      ...column,
      cards: ordered(column.cards).filter((item) => item.id !== location.cardID),
    }));
    const target = nextColumns.find((column) => column.id === targetColumnID);
    if (!target) return;
    const cards = [...(target.cards || [])];
    cards.splice(Math.min(targetIndex, cards.length), 0, card);
    target.cards = cards;
    commit({ columns: nextColumns });
    if (preserveEditor) setSelected({ columnID: targetColumnID, cardID: location.cardID });
  }

  function moveSelected(direction: -1 | 1) {
    if (!selected) return;
    const index = columns.findIndex((column) => column.id === selected.columnID);
    const target = columns[index + direction];
    if (target) moveCard(selected, target.id || "");
  }

  function dropCard(event: DragEvent, columnID: string, targetIndex: number) {
    event.preventDefault();
    if (dragging) moveCard(dragging, columnID, targetIndex, false);
    setDragging(null);
    setDropTarget("");
  }

  function beginCardDrag(location: CardLocation, event: DragEvent<HTMLElement>) {
    if (dragReleaseTimerRef.current !== null) window.clearTimeout(dragReleaseTimerRef.current);
    suppressOpenRef.current = true;
    setDragging(location);
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", location.cardID);
  }

  function endCardDrag() {
    setDragging(null);
    setDropTarget("");
    dragReleaseTimerRef.current = window.setTimeout(() => {
      suppressOpenRef.current = false;
      dragReleaseTimerRef.current = null;
    }, 0);
  }

  function openCard(location: CardLocation) {
    if (suppressOpenRef.current) return;
    setSelected(location);
  }

  function closeCard() {
    const cardID = selected?.cardID || "";
    setSelected(null);
    requestAnimationFrame(() => cardButtonRefs.current.get(cardID)?.focus());
  }

  function formatDescription(prefix: string, suffix = prefix, placeholder = "text", selection?: DescriptionSelection) {
    if (!selected || !selectedCard || !selectedParts) return;
    const start = selection?.start ?? selectedParts.description.length;
    const end = selection?.end ?? start;
    const selectedText = selectedParts.description.slice(start, end) || placeholder;
    const description = `${selectedParts.description.slice(0, start)}${prefix}${selectedText}${suffix}${selectedParts.description.slice(end)}`;
    updateCard(selected, (card) => ({ ...card, text: cardText(selectedParts.title, description) }));
  }

  async function upload(file?: File) {
    if (!file || !selected || !selectedCard || !selectedParts) return;
    setUploading(true);
    setUploadError("");
    try {
      const attachment = await onUpload(file);
      const reference = attachment.markdown_reference || `![${attachment.filename}](hank-note-attachment://${attachment.id})`;
      const description = [selectedParts.description, reference].filter(Boolean).join("\n\n");
      updateCard(selected, (card) => ({ ...card, text: cardText(selectedParts.title, description) }));
    } catch (error) {
      setUploadError(error instanceof Error ? error.message : "File could not be uploaded.");
    } finally {
      setUploading(false);
    }
  }

  async function uploadFiles(files: File[]) {
    for (const file of files) await upload(file);
  }

  async function deleteSelected() {
    if (!selected || !selectedCard || !selectedParts) return;
    if (!await confirmDelete(`Delete “${selectedParts.title}”?`)) return;
    updateColumn(selected.columnID, (column) => ({ ...column, cards: ordered(column.cards).filter((card) => card.id !== selected.cardID) }));
    setSelected(null);
  }

  function addColumn() {
    const column: KanbanColumn = { id: uniqueID("column"), title: "New column", sort_order: columns.length, cards: [] };
    commit({ columns: [...columns, column] });
    setEditingColumnID(column.id || "");
  }

  const normalizedQuery = query.trim().toLowerCase();

  return (
    <div className={`kanban-workspace${selectedCard ? " details-open" : ""}`}>
      <div className="kanban-main">
        <header className="kanban-workbar">
          <label className="kanban-search">
            <SmallIcon name="search" />
            <span className="visually-hidden">Search cards</span>
            <input aria-label="Search cards" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search cards" />
          </label>
          <span className="kanban-board-summary">{columns.reduce((total, column) => total + (column.cards?.length || 0), 0)} tasks · {columns.length} columns</span>
          <button className="kanban-add-column" type="button" onClick={addColumn}><SmallIcon name="plus" /> Add column</button>
        </header>

        <div className="kanban-board" aria-label="Kanban board">
          {columns.map((column, columnIndex) => {
            const cards = ordered(column.cards);
            const visibleCards = normalizedQuery
              ? cards.filter((card) => String(card.text || card.title || "").toLowerCase().includes(normalizedQuery))
              : cards;
            return (
              <section
                className={`kanban-column${dropTarget === column.id ? " is-drop-target" : ""}`}
                key={column.id}
                aria-labelledby={`kanban-column-${column.id}`}
                onDragOver={(event) => { event.preventDefault(); setDropTarget(column.id || ""); }}
                onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node)) setDropTarget(""); }}
                onDrop={(event) => dropCard(event, column.id || "", cards.length)}
              >
                <header className="kanban-column-head">
                  {editingColumnID === column.id ? (
                    <input
                      autoFocus
                      aria-label="Column title"
                      value={column.title || ""}
                      onChange={(event) => updateColumn(column.id || "", (item) => ({ ...item, title: event.target.value }))}
                      onBlur={() => setEditingColumnID("")}
                      onKeyDown={(event: KeyboardEvent<HTMLInputElement>) => { if (event.key === "Enter" || event.key === "Escape") setEditingColumnID(""); }}
                    />
                  ) : (
                    <button className="kanban-column-title" type="button" onClick={() => setEditingColumnID(column.id || "")} title="Rename column">
                      <h2 id={`kanban-column-${column.id}`}>{column.title}</h2>
                    </button>
                  )}
                  <span className="kanban-count">{cards.length}</span>
                  <div className="kanban-column-actions">
                    <button type="button" aria-label={`Move ${column.title} left`} disabled={columnIndex === 0} onClick={() => moveColumn(column.id || "", -1)}><SmallIcon name="left" /></button>
                    <button type="button" aria-label={`Move ${column.title} right`} disabled={columnIndex === columns.length - 1} onClick={() => moveColumn(column.id || "", 1)}><SmallIcon name="right" /></button>
                    <button type="button" aria-label={`Delete ${column.title}`} onClick={() => void deleteColumn(column)}><SmallIcon name="trash" /></button>
                  </div>
                </header>

                <div className="kanban-card-stack">
                  {visibleCards.map((card) => {
                    const parts = cardTitleAndDescription(card);
                    const cardIndex = cards.findIndex((item) => item.id === card.id);
                    return (
                      <article
                        className={`kanban-card color-${card.color || "default"}${dragging?.cardID === card.id ? " is-dragging" : ""}`}
                        key={card.id}
                        draggable
                        onDragStart={(event) => beginCardDrag({ columnID: column.id || "", cardID: card.id || "" }, event)}
                        onDragEnd={endCardDrag}
                        onDragOver={(event) => event.preventDefault()}
                        onDrop={(event) => { event.stopPropagation(); dropCard(event, column.id || "", cardIndex); }}
                      >
                        <button
                          ref={(node) => {
                            if (node) cardButtonRefs.current.set(card.id || "", node);
                            else cardButtonRefs.current.delete(card.id || "");
                          }}
                          className="kanban-card-open"
                          type="button"
                          aria-label={`Open task ${parts.title}`}
                          onClick={() => openCard({ columnID: column.id || "", cardID: card.id || "" })}
                        >
                          <span className="kanban-card-grip" aria-hidden="true"><SmallIcon name="grip" /></span>
                          <strong>{parts.title}</strong>
                          <CardDescription value={parts.description} attachments={attachments} />
                          {card.due_date ? <time dateTime={card.due_date}>{new Date(`${card.due_date}T12:00:00`).toLocaleDateString(undefined, { month: "short", day: "numeric" })}</time> : null}
                        </button>
                        <div className="kanban-card-move">
                          <button type="button" aria-label={`Move ${parts.title} left`} disabled={columnIndex === 0} onClick={() => moveCard({ columnID: column.id || "", cardID: card.id || "" }, columns[columnIndex - 1]?.id || "", Number.MAX_SAFE_INTEGER, false)}><SmallIcon name="left" /></button>
                          <button type="button" aria-label={`Move ${parts.title} right`} disabled={columnIndex === columns.length - 1} onClick={() => moveCard({ columnID: column.id || "", cardID: card.id || "" }, columns[columnIndex + 1]?.id || "", Number.MAX_SAFE_INTEGER, false)}><SmallIcon name="right" /></button>
                        </div>
                      </article>
                    );
                  })}
                  {!visibleCards.length ? <p className="kanban-column-empty">{normalizedQuery && cards.length ? "No matching tasks" : "Drop tasks here"}</p> : null}
                </div>

                {addingColumnID === column.id ? (
                  <form className="kanban-task-composer" onSubmit={(event) => { event.preventDefault(); addTask(column.id || ""); }}>
                    <label>
                      <span className="visually-hidden">Task title</span>
                      <textarea
                        autoFocus
                        aria-label="Task title"
                        value={taskDraft}
                        onChange={(event) => setTaskDraft(event.target.value)}
                        onKeyDown={(event) => {
                          if (event.key !== "Enter" || event.shiftKey) return;
                          event.preventDefault();
                          addTask(column.id || "");
                        }}
                        placeholder="What needs doing?"
                        rows={2}
                      />
                    </label>
                    <div>
                      <button type="submit" disabled={!taskDraft.trim()}>Create task</button>
                      <button type="button" onClick={() => { setAddingColumnID(""); setTaskDraft(""); }}>Cancel</button>
                    </div>
                  </form>
                ) : (
                  <button className="add-card-button" type="button" aria-label={`Add task to ${column.title}`} onClick={() => setAddingColumnID(column.id || "")}><SmallIcon name="plus" /> Add task</button>
                )}
              </section>
            );
          })}
          {!columns.length ? (
            <button className="kanban-empty-board" type="button" onClick={addColumn}><SmallIcon name="plus" /> Add your first column</button>
          ) : null}
        </div>
      </div>

      {selectedCard && selected && selectedParts ? (
        <KanbanCardModal
          card={selectedCard}
          title={selectedParts.title}
          description={selectedParts.description}
          columnID={selected.columnID}
          columns={columns}
          attachments={attachments}
          uploading={uploading}
          uploadError={uploadError}
          onTitleChange={(value) => updateCard(selected, (card) => ({ ...card, text: cardText(value, selectedParts.description) }))}
          onDescriptionChange={(value) => updateCard(selected, (card) => ({ ...card, text: cardText(selectedParts.title, value) }))}
          onFormat={formatDescription}
          onMove={(columnID) => moveCard(selected, columnID, Number.MAX_SAFE_INTEGER, true)}
          onDueDateChange={(value) => updateCard(selected, (card) => ({ ...card, due_date: value }))}
          onColorChange={(value) => updateCard(selected, (card) => ({ ...card, color: value }))}
          onUploadFiles={uploadFiles}
          onMoveLeft={() => moveSelected(-1)}
          onMoveRight={() => moveSelected(1)}
          canMoveLeft={columns.findIndex((column) => column.id === selected.columnID) > 0}
          canMoveRight={columns.findIndex((column) => column.id === selected.columnID) < columns.length - 1}
          onDelete={() => void deleteSelected()}
          onClose={closeCard}
        />
      ) : null}
    </div>
  );
}
