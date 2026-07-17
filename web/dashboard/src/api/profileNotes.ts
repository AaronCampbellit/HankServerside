import { apiClient, type ApiTransport } from "./client";
import { arrayFrom } from "./normalize";

export type ProfileNoteSummary = {
  id?: string;
  note_id?: string;
  title?: string;
  preview?: string;
  revision?: string;
  updated_at?: string;
  page_type?: "text" | "kanban" | "notebook" | string;
  parent_id?: string;
  shared?: boolean;
  mcp_excluded?: boolean;
};

export type KanbanCard = {
  id?: string;
  text?: string;
  title?: string;
  sort_order?: number;
  color?: string;
  due_date?: string;
};

export type KanbanColumn = {
  id?: string;
  title?: string;
  sort_order?: number;
  cards?: KanbanCard[];
};

export type KanbanBoard = {
  columns?: KanbanColumn[];
};

export type NoteAttachment = {
  id: string;
  note_id?: string;
  filename: string;
  content_type: string;
  size_bytes?: number;
  download_url: string;
  markdown_reference: string;
};

export type ProfileNote = ProfileNoteSummary & {
  content?: string;
  body_markdown?: string;
  body_format?: string;
  board?: KanbanBoard | null;
  attachments?: NoteAttachment[];
};

export type SaveProfileNoteInput = {
  note_id: string;
  title: string;
  body_markdown: string;
  expected_revision: string;
  page_type: string;
  parent_id: string;
  mcp_excluded?: boolean;
  board?: KanbanBoard | null;
};

export type SaveProfileNoteResponse = {
  note_id: string;
  revision: string;
  updated_at?: string;
};

export function noteID(note: ProfileNoteSummary): string {
  return note.note_id || note.id || "";
}

export class ProfileNotesClient {
  constructor(private readonly api: ApiTransport = apiClient) {}

  async listNotes(): Promise<{ notes: ProfileNoteSummary[] }> {
    const payload = await this.api.request<{ notes?: ProfileNoteSummary[] }>("/v1/me/notes");
    return { notes: arrayFrom<ProfileNoteSummary>(payload.notes) };
  }

  fetchNote(id: string) {
    return this.api.request<ProfileNote>(`/v1/me/notes/${encodeURIComponent(id)}`);
  }

  saveNote(input: SaveProfileNoteInput) {
    const body: Record<string, unknown> = {
      note_id: input.note_id,
      title: input.title,
      content: input.body_markdown,
      body_markdown: input.body_markdown,
      body_format: "markdown",
      expected_revision: input.expected_revision,
      page_type: input.page_type,
      parent_id: input.parent_id,
      mcp_excluded: Boolean(input.mcp_excluded),
    };
    if (input.page_type === "kanban") body.board = input.board || { columns: [] };
    if (input.note_id) {
      return this.api.request<SaveProfileNoteResponse>(`/v1/me/notes/${encodeURIComponent(input.note_id)}`, {
        method: "PUT",
        body,
      });
    }
    return this.api.request<SaveProfileNoteResponse>("/v1/me/notes", { method: "POST", body });
  }

  deleteNote(id: string) {
    return this.api.request<{ ok: boolean }>(`/v1/me/notes/${encodeURIComponent(id)}`, { method: "DELETE" });
  }

  uploadAttachment(noteID: string, file: File) {
    return this.api.request<NoteAttachment>(
      `/v1/me/notes/${encodeURIComponent(noteID)}/attachments?filename=${encodeURIComponent(file.name)}`,
      {
        method: "POST",
        headers: { "Content-Type": file.type || "application/octet-stream" },
        body: file,
      },
    );
  }
}

export const profileNotesClient = new ProfileNotesClient();
