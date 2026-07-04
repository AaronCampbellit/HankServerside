import { describe, expect, it, vi } from "vitest";
import { ProfileNotesClient } from "./profileNotes";
import type { ApiTransport } from "./client";

describe("ProfileNotesClient", () => {
  it("lists fetches saves and deletes profile notes", async () => {
    const request = vi.fn(async <T>() => ({}) as T);
    const client = new ProfileNotesClient({ request: request as unknown as ApiTransport["request"] });

    await client.listNotes();
    await client.fetchNote("daily.md");
    await client.saveNote({
      note_id: "",
      title: "Daily",
      body_markdown: "Remember milk",
      expected_revision: "",
      page_type: "text",
      parent_id: "",
    });
    await client.saveNote({
      note_id: "daily.md",
      title: "Daily",
      body_markdown: "Remember eggs",
      expected_revision: "2",
      page_type: "text",
      parent_id: "",
    });
    await client.deleteNote("daily.md");

    expect(request).toHaveBeenNthCalledWith(1, "/v1/me/notes");
    expect(request).toHaveBeenNthCalledWith(2, "/v1/me/notes/daily.md");
    expect(request).toHaveBeenNthCalledWith(3, "/v1/me/notes", {
      method: "POST",
      body: {
        note_id: "",
        title: "Daily",
        content: "Remember milk",
        body_markdown: "Remember milk",
        body_format: "markdown",
        expected_revision: "",
        page_type: "text",
        parent_id: "",
      },
    });
    expect(request).toHaveBeenNthCalledWith(4, "/v1/me/notes/daily.md", {
      method: "PUT",
      body: {
        note_id: "daily.md",
        title: "Daily",
        content: "Remember eggs",
        body_markdown: "Remember eggs",
        body_format: "markdown",
        expected_revision: "2",
        page_type: "text",
        parent_id: "",
      },
    });
    expect(request).toHaveBeenNthCalledWith(5, "/v1/me/notes/daily.md", { method: "DELETE" });
  });
});
