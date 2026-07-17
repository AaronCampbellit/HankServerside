import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfirmDialogProvider, ToastProvider } from "../ui/primitives";
import { ProfileNotesPage } from "./ProfileNotesPage";

const profileNotesClient = vi.hoisted(() => ({
  listNotes: vi.fn(),
  fetchNote: vi.fn(),
  saveNote: vi.fn(),
  deleteNote: vi.fn(),
  uploadAttachment: vi.fn(),
}));

vi.mock("../api/profileNotes", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/profileNotes")>();
  return {
    ...actual,
    profileNotesClient,
  };
});

function renderPage() {
  return render(
    <ToastProvider>
      <ConfirmDialogProvider>
        <ProfileNotesPage />
      </ConfirmDialogProvider>
    </ToastProvider>,
  );
}

describe("ProfileNotesPage", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
    Reflect.deleteProperty(document, "execCommand");
  });

  it("renders the redesigned editor chrome and kanban/notebook states", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "grocery", title: "Grocery List", preview: "Tortillas, ground beef, salsa", page_type: "text" },
        { note_id: "projects", title: "Home Projects", preview: "Kanban · 3 columns", page_type: "kanban" },
        { note_id: "house", title: "House Notebook", preview: "4 notes", page_type: "notebook" },
      ],
    });
    profileNotesClient.fetchNote.mockImplementation(async (id: string) => ({
      note_id: id,
      title: id === "projects" ? "Home Projects" : "Grocery List",
      body_markdown: id === "projects" ? "# Home Projects\n\n## To do\n- Repaint porch\n\n## Doing\n- Order furnace filter" : "- [x] Tortillas\n- [ ] Salsa",
      revision: "1",
      page_type: id === "projects" ? "kanban" : "text",
    }));

    renderPage();

    expect(await screen.findByRole("button", { name: "Home Projects" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "New notebook" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bold" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Home Projects" }));

    expect(await screen.findByText("To do")).toBeInTheDocument();
    expect(screen.getByText("Doing")).toBeInTheDocument();
    expect(screen.queryByText("Done")).not.toBeInTheDocument();
  });

  it("autosaves interactive kanban changes with board data", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "work", title: "Client Work", preview: "Kanban", page_type: "kanban", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "work",
      title: "Client Work",
      body_markdown: "# Client Work\n\n## Inbox\n- Review brief\n\n## Done",
      revision: "1",
      page_type: "kanban",
      board: {
        columns: [
          { id: "inbox", title: "Inbox", sort_order: 0, cards: [{ id: "brief", text: "Review brief", sort_order: 0 }] },
          { id: "done", title: "Done", sort_order: 1, cards: [] },
        ],
      },
    });
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "work", revision: "2" });

    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Add task to Inbox" }));
    fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "Prepare invoice" } });
    vi.useFakeTimers();
    fireEvent.click(screen.getByRole("button", { name: "Create task" }));
    await vi.advanceTimersByTimeAsync(750);

    expect(profileNotesClient.saveNote).toHaveBeenCalledWith(expect.objectContaining({
      note_id: "work",
      page_type: "kanban",
      board: expect.objectContaining({
        columns: expect.arrayContaining([
          expect.objectContaining({ cards: expect.arrayContaining([expect.objectContaining({ text: "Prepare invoice" })]) }),
        ]),
      }),
    }));
  });

  it("autosaves the canonical attachment reference after a kanban upload", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "work", title: "Client Work", preview: "Kanban", page_type: "kanban", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "work",
      title: "Client Work",
      body_markdown: "# Client Work\n\n## Inbox\n- Review brief",
      revision: "1",
      page_type: "kanban",
      board: { columns: [{ id: "inbox", title: "Inbox", sort_order: 0, cards: [{ id: "brief", text: "Review brief", sort_order: 0 }] }] },
      attachments: [],
    });
    profileNotesClient.uploadAttachment.mockResolvedValue({
      id: "natt-1",
      filename: "wireframe.png",
      content_type: "image/png",
      download_url: "/v1/me/notes/work/attachments/natt-1",
      markdown_reference: "![wireframe.png](hank-note-attachment://natt-1?filename=wireframe.png&scope=profile)",
    });
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "work", revision: "2" });

    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Open task Review brief" }));
    vi.useFakeTimers();
    const file = new File(["image"], "wireframe.png", { type: "image/png" });
    fireEvent.change(screen.getByLabelText("Add screenshot or file"), { target: { files: [file] } });
    await vi.advanceTimersByTimeAsync(750);

    expect(profileNotesClient.uploadAttachment).toHaveBeenCalledWith("work", file);
    expect(profileNotesClient.saveNote).toHaveBeenCalledWith(expect.objectContaining({
      note_id: "work",
      board: expect.objectContaining({
        columns: [expect.objectContaining({
          cards: [expect.objectContaining({ text: expect.stringContaining("hank-note-attachment://natt-1") })],
        })],
      }),
    }));
  });

  it("matches the guide notes anatomy with rich toolbar, notebook dialog, rendered text page, and kanban controls", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "grocery", title: "Grocery List", preview: "Tortillas, ground beef, salsa", page_type: "text" },
        { note_id: "projects", title: "Home Projects", preview: "Kanban · 3 columns", page_type: "kanban" },
        { note_id: "house", title: "House Notebook", preview: "No pages yet", page_type: "notebook" },
      ],
    });
    profileNotesClient.fetchNote.mockImplementation(async (id: string) => ({
      note_id: id,
      title: id === "projects" ? "Home Projects" : id === "house" ? "House Notebook" : "Grocery List",
      body_markdown: id === "projects"
        ? "# Home Projects\n\n## To do\n- Re-caulk the bathroom\n- Repaint the porch railing\n\n## Doing\n- Clean the gutters\n\n## Done\n- Order furnace filter"
        : "# Taco Night\n- [x] Tortillas\n- [x] Ground beef\n- [ ] Salsa (medium)\n- [ ] Avocados x4\n# Notes\nMaya is bringing dessert. Pick up the order from #print-shop before 5pm.",
      revision: "1",
      page_type: id === "projects" ? "kanban" : id === "house" ? "notebook" : "text",
    }));

    renderPage();

    expect(await screen.findByRole("heading", { name: "Notes" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Collapse notes rail" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "New notebook" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "New note" })).toBeInTheDocument();

    const editorTools = screen.getByLabelText("Editor tools");
    for (const label of ["Undo", "Redo", "Bold", "Italic", "Underline", "Smaller heading", "Heading", "Larger heading", "Bulleted list", "Numbered list", "Text page", "Kanban page", "Tag", "Link"]) {
      expect(within(editorTools).getByRole("button", { name: label })).toBeInTheDocument();
    }

    const body = await screen.findByLabelText("Note body");
    expect(body).toHaveAttribute("contenteditable", "true");
    expect(body).toHaveTextContent("Taco Night");
    expect(body).toHaveTextContent("Tortillas");
    expect(body).toHaveTextContent("Maya is bringing dessert");
    expect(body).not.toHaveTextContent("# Taco Night");
    // The note is a single editable surface — no read-only rendered duplicate.
    expect(screen.queryByLabelText("Rendered note body")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Home Projects" }));
    expect(await screen.findByText("Re-caulk the bathroom")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add card" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open notebook House Notebook" }));
    expect(await screen.findByText("No pages in this notebook yet.")).toBeInTheDocument();
    expect(screen.queryByText("Paint colors")).not.toBeInTheDocument();
    expect(screen.queryByText(/shared with 2 people/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "New note" }));
    expect(screen.getByLabelText("Note title")).toHaveValue("");
    expect(screen.queryByText("# Untitled")).not.toBeInTheDocument();
    expect(screen.queryByText("2m ago")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "New notebook" }));
    const dialog = screen.getByRole("dialog", { name: "New notebook" });
    expect(within(dialog).getByLabelText("Notebook name")).toBeInTheDocument();
    expect(within(dialog).queryByLabelText("Share with all home members")).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Create notebook" })).toBeInTheDocument();
  });

  it("opens notebook child pages from the notebook section", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "house", title: "House Notebook", preview: "1 page", page_type: "notebook" },
        { note_id: "roof", title: "Roof Warranty", preview: "Expires 2027", page_type: "text", parent_id: "house" },
      ],
    });
    profileNotesClient.fetchNote.mockImplementation(async (id: string) => ({
      note_id: id,
      title: id === "house" ? "House Notebook" : "Roof Warranty",
      body_markdown: id === "house" ? "" : "# Roof Warranty\nExpires in 2027.",
      revision: "1",
      page_type: id === "house" ? "notebook" : "text",
      parent_id: id === "roof" ? "house" : "",
    }));

    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /Open Roof Warranty/i }));

    expect(await screen.findByDisplayValue("Roof Warranty")).toBeInTheDocument();
    expect(screen.getByLabelText("Note body")).toHaveTextContent("Roof Warranty");
    expect(profileNotesClient.fetchNote).toHaveBeenCalledWith("roof");
  });

  it("shows a notebook section and notebook controls even when no notebooks exist", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "daily", title: "Daily Notes", preview: "Remember milk", page_type: "text" },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "# Daily\nRemember milk",
      revision: "1",
      page_type: "text",
    });

    renderPage();

    expect(await screen.findByRole("heading", { name: "Notebooks" })).toBeInTheDocument();
    expect(screen.getByText("No notebooks yet.")).toBeInTheDocument();
    expect(screen.getByLabelText("Notebook filter")).toBeInTheDocument();
    expect(screen.getByLabelText("Notebook")).toBeInTheDocument();
  });

  it("filters notes by notebook and creates child notes from the notebook panel", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "house", title: "House Notebook", preview: "1 page", page_type: "notebook" },
        { note_id: "roof", title: "Roof Warranty", preview: "Expires 2027", page_type: "text", parent_id: "house" },
        { note_id: "daily", title: "Daily Notes", preview: "Remember milk", page_type: "text" },
      ],
    });
    profileNotesClient.fetchNote.mockImplementation(async (id: string) => ({
      note_id: id,
      title: id === "house" ? "House Notebook" : id === "roof" ? "Roof Warranty" : "Daily Notes",
      body_markdown: id === "house" ? "" : id === "roof" ? "# Roof Warranty\nExpires in 2027." : "# Daily\nRemember milk",
      revision: "1",
      page_type: id === "house" ? "notebook" : "text",
      parent_id: id === "roof" ? "house" : "",
    }));

    renderPage();

    fireEvent.change(await screen.findByLabelText("Notebook filter"), { target: { value: "house" } });
    const noteTabs = screen.getByLabelText("Note cards");
    expect(within(noteTabs).getByRole("button", { name: "Roof Warranty" })).toBeInTheDocument();
    expect(within(noteTabs).queryByRole("button", { name: /Daily Notes/i })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open notebook House Notebook" }));
    fireEvent.click(await screen.findByRole("button", { name: "New note in House Notebook" }));

    expect(screen.getByLabelText("Note title")).toHaveValue("");
    expect(screen.getByLabelText("Notebook")).toHaveValue("house");
  });

  it("lets people type into a text note and save the changed body", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "daily", title: "Daily Notes", preview: "Remember milk", page_type: "text", revision: "1" },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Remember milk",
      revision: "1",
      page_type: "text",
    });
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "daily", revision: "2", updated_at: "2026-07-03T12:00:00Z" });

    renderPage();

    const body = await screen.findByLabelText("Note body");
    expect(body).toBeVisible();
    expect(body).not.toHaveClass("visually-hidden");

    body.innerHTML = "Remember milk<div>Call Sam</div>";
    fireEvent.input(body);
    fireEvent.click(screen.getByRole("button", { name: "Save note" }));

    expect(profileNotesClient.saveNote).toHaveBeenCalledWith({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Remember milk\nCall Sam",
      expected_revision: "1",
      page_type: "text",
      parent_id: "",
      mcp_excluded: false,
    });
  });

  it("keeps note row actions off-canvas and reveals them on hover", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "daily", title: "Daily Notes", preview: "Remember milk", page_type: "text", revision: "1" },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Remember milk",
      revision: "1",
      page_type: "text",
    });
    vi.stubGlobal("matchMedia", vi.fn(() => ({ matches: true })));

    renderPage();

    const row = (await screen.findByRole("button", { name: "Daily Notes" })).closest(".notes-guide-row") as HTMLDivElement;
    const scrollTo = vi.fn();
    row.scrollTo = scrollTo as unknown as typeof row.scrollTo;

    fireEvent.mouseEnter(row);
    expect(scrollTo).toHaveBeenCalledWith({ left: row.scrollWidth, behavior: "smooth" });

    fireEvent.mouseLeave(row);
    expect(scrollTo).toHaveBeenCalledWith({ left: 0, behavior: "smooth" });
  });

  it("renders formatting while editing and stores markdown", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "daily", title: "Daily Notes", preview: "Remember milk", page_type: "text", revision: "1" },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "**Remember** milk",
      revision: "1",
      page_type: "text",
    });
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "daily", revision: "2", updated_at: "2026-07-03T12:00:00Z" });

    renderPage();

    const body = await screen.findByLabelText("Note body");
    expect(body).toHaveAttribute("contenteditable", "true");
    expect(within(body).getByText("Remember").tagName).toBe("STRONG");
    expect(body).toHaveTextContent("Remember milk");
    expect(body).not.toHaveTextContent("**Remember** milk");
    fireEvent.click(screen.getByRole("button", { name: "Save note" }));

    expect(profileNotesClient.saveNote).toHaveBeenCalledWith({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "**Remember** milk",
      expected_revision: "1",
      page_type: "text",
      parent_id: "",
      mcp_excluded: false,
    });
  });

  it("applies text tools to the editor and supports undo/redo", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "daily", title: "Daily Notes", preview: "Remember milk", page_type: "text", revision: "1" },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Remember milk",
      revision: "1",
      page_type: "text",
    });

    renderPage();

    const body = await screen.findByLabelText("Note body");
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Bold" }));
    expect(within(body).getByText("bold text").tagName).toBe("STRONG");

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(body).toHaveTextContent("Remember milk");
    expect(body).not.toHaveTextContent("bold text");

    fireEvent.click(screen.getByRole("button", { name: "Redo" }));
    expect(within(body).getByText("bold text").tagName).toBe("STRONG");

    fireEvent.click(screen.getByRole("button", { name: "Bulleted list" }));
    expect(body.querySelector("ul")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Heading" }));
    expect(body.querySelector("h1")).toBeInTheDocument();
  });

  it("undoes and redoes typing and deletion as separate action groups", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "daily", title: "Daily Notes", preview: "Original", page_type: "text", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
    });

    renderPage();

    const body = await screen.findByLabelText("Note body");
    body.innerHTML = "Original text";
    fireEvent.input(body, { inputType: "insertText" });
    body.innerHTML = "Original tex";
    fireEvent.input(body, { inputType: "deleteContentBackward" });

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(body).toHaveTextContent("Original text");

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(body).toHaveTextContent("Original");

    fireEvent.click(screen.getByRole("button", { name: "Redo" }));
    expect(body).toHaveTextContent("Original text");
    fireEvent.click(screen.getByRole("button", { name: "Redo" }));
    expect(body).toHaveTextContent("Original tex");
  });

  it("records a rich editor command as one undo action when the browser also fires input", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "daily", title: "Daily Notes", preview: "Original", page_type: "text", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
    });
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: vi.fn(() => {
        const body = document.activeElement as HTMLElement;
        body.innerHTML += "<strong>bold text</strong>";
        body.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "formatBold" }));
        return true;
      }),
    });

    renderPage();
    const body = await screen.findByLabelText("Note body");

    fireEvent.click(screen.getByRole("button", { name: "Bold" }));
    expect(body).toHaveTextContent("bold text");
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(body).toHaveTextContent("Original");
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
  });

  it("keeps exactly 50 undo and redo actions", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "daily", title: "Daily Notes", preview: "Original", page_type: "text", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
    });

    renderPage();
    await screen.findByLabelText("Note body");

    for (let index = 0; index < 55; index++) {
      fireEvent.click(screen.getByRole("button", { name: "Bold" }));
    }

    const undo = screen.getByRole("button", { name: "Undo" });
    for (let index = 0; index < 50; index++) fireEvent.click(undo);
    expect(undo).toBeDisabled();

    const redo = screen.getByRole("button", { name: "Redo" });
    for (let index = 0; index < 50; index++) fireEvent.click(redo);
    expect(redo).toBeDisabled();
  }, 15_000);

  it("moves a note from the list into a selected notebook", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "house", title: "House Notebook", preview: "No pages yet", page_type: "notebook" },
        { note_id: "daily", title: "Daily Notes", preview: "Remember milk", page_type: "text", revision: "1" },
      ],
    });
    profileNotesClient.fetchNote.mockImplementation(async (id: string) => ({
      note_id: id,
      title: id === "house" ? "House Notebook" : "Daily Notes",
      body_markdown: id === "house" ? "" : "Remember milk",
      revision: "1",
      page_type: id === "house" ? "notebook" : "text",
      parent_id: "",
    }));
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "daily", revision: "2", updated_at: "2026-07-03T12:00:00Z" });

    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Move Daily Notes" }));
    const dialog = screen.getByRole("dialog", { name: "Move note" });
    fireEvent.change(within(dialog).getByLabelText("Move to notebook"), { target: { value: "house" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Move note" }));

    expect(profileNotesClient.fetchNote).toHaveBeenCalledWith("daily");
    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenCalledWith({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Remember milk",
      expected_revision: "1",
      page_type: "text",
      parent_id: "house",
      mcp_excluded: false,
    }));
  });

  it("saves a notebook selection immediately instead of only changing the local editor state", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "family", title: "Family", preview: "Notebook", page_type: "notebook" },
        { note_id: "passwords", title: "Passwords", preview: "secret", page_type: "text", revision: "1" },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "passwords",
      title: "Passwords",
      body_markdown: "secret",
      revision: "1",
      page_type: "text",
      parent_id: "",
    });
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "passwords", revision: "2" });

    renderPage();

    fireEvent.change(await screen.findByLabelText("Notebook"), { target: { value: "family" } });

    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenCalledWith({
      note_id: "passwords",
      title: "Passwords",
      body_markdown: "secret",
      expected_revision: "1",
      page_type: "text",
      parent_id: "family",
      mcp_excluded: false,
    }));
  });

  it("toggles MCP exclusion from the editor toolbar and sends the lock state in save payloads", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "daily", title: "Daily", preview: "Original", page_type: "text", revision: "1", mcp_excluded: false },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
      parent_id: "",
      mcp_excluded: false,
    });
    profileNotesClient.saveNote
      .mockResolvedValueOnce({ note_id: "daily", revision: "2" })
      .mockResolvedValueOnce({ note_id: "daily", revision: "3" });

    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Exclude from MCP" }));

    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenNthCalledWith(1, {
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      expected_revision: "1",
      page_type: "text",
      parent_id: "",
      mcp_excluded: true,
    }));

    fireEvent.click(await screen.findByRole("button", { name: "Include in MCP" }));

    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenNthCalledWith(2, {
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      expected_revision: "2",
      page_type: "text",
      parent_id: "",
      mcp_excluded: false,
    }));
  });

  it("shows inherited MCP exclusion copy for notes inside an excluded notebook", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "roof", title: "Roof Warranty", preview: "Expires 2027", page_type: "text", parent_id: "house", mcp_excluded: false, updated_at: "2026-07-10T12:00:00Z" },
        { note_id: "house", title: "House Notebook", preview: "Notebook", page_type: "notebook", mcp_excluded: true, updated_at: "2026-07-09T12:00:00Z" },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "roof",
      title: "Roof Warranty",
      body_markdown: "# Roof Warranty\nExpires in 2027.",
      revision: "1",
      page_type: "text",
      parent_id: "house",
      mcp_excluded: false,
    });

    renderPage();

    expect(await screen.findByText("Excluded because its notebook is locked")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Exclude from MCP" })).toBeInTheDocument();
  });

  it("preserves edits made while an earlier save is still running", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "daily", title: "Daily Notes", preview: "Original", page_type: "text", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
      parent_id: "",
    });
    let resolveSave!: (value: { note_id: string; revision: string }) => void;
    profileNotesClient.saveNote.mockReturnValue(new Promise((resolve) => { resolveSave = resolve; }));

    renderPage();

    const body = await screen.findByLabelText("Note body");
    body.innerHTML = "First edit";
    fireEvent.input(body);
    fireEvent.click(screen.getByRole("button", { name: "Save note" }));
    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenCalledTimes(1));

    body.innerHTML = "Second edit";
    fireEvent.input(body);
    resolveSave({ note_id: "daily", revision: "2" });

    await waitFor(() => expect(body).toHaveTextContent("Second edit"));
    expect(body).not.toHaveTextContent("First edit");
    expect(screen.getByRole("button", { name: "Save note" })).toHaveTextContent("Unsaved");
  });

  it("queues the latest notebook selection while a save is running", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "family", title: "Family", preview: "Notebook", page_type: "notebook" },
        { note_id: "work", title: "Work", preview: "Notebook", page_type: "notebook" },
        { note_id: "daily", title: "Daily", preview: "Original", page_type: "text", revision: "1" },
      ],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
      parent_id: "",
    });
    let resolveFirst!: (value: { note_id: string; revision: string }) => void;
    profileNotesClient.saveNote
      .mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve; }))
      .mockResolvedValueOnce({ note_id: "daily", revision: "3" });

    renderPage();

    const notebook = await screen.findByLabelText("Notebook");
    fireEvent.change(notebook, { target: { value: "family" } });
    fireEvent.change(notebook, { target: { value: "work" } });
    resolveFirst({ note_id: "daily", revision: "2" });

    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenCalledTimes(2));
    expect(profileNotesClient.saveNote).toHaveBeenLastCalledWith({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      expected_revision: "2",
      page_type: "text",
      parent_id: "work",
      mcp_excluded: false,
    });
  });

  it("autosaves the latest note body after 750ms of idle typing", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "daily", title: "Daily", preview: "Original", page_type: "text", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
      parent_id: "",
    });
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "daily", revision: "2" });

    renderPage();
    const body = await screen.findByLabelText("Note body");
    vi.useFakeTimers();

    body.innerHTML = "First draft";
    fireEvent.input(body);
    await vi.advanceTimersByTimeAsync(400);
    body.innerHTML = "Latest draft";
    fireEvent.input(body);
    await vi.advanceTimersByTimeAsync(749);
    expect(profileNotesClient.saveNote).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    expect(profileNotesClient.saveNote).toHaveBeenCalledWith({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Latest draft",
      expected_revision: "1",
      page_type: "text",
      parent_id: "",
      mcp_excluded: false,
    });
    expect(screen.queryByText("Note saved.")).not.toBeInTheDocument();
  });

  it("flushes a pending autosave before opening another note", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "first", title: "First", preview: "Original", page_type: "text", revision: "1", updated_at: "2026-07-10T12:00:00Z" },
        { note_id: "second", title: "Second", preview: "Second body", page_type: "text", revision: "1", updated_at: "2026-07-09T12:00:00Z" },
      ],
    });
    profileNotesClient.fetchNote.mockImplementation(async (id: string) => ({
      note_id: id,
      title: id === "first" ? "First" : "Second",
      body_markdown: id === "first" ? "Original" : "Second body",
      revision: "1",
      page_type: "text",
      parent_id: "",
    }));
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "first", revision: "2" });

    renderPage();
    const body = await screen.findByLabelText("Note body");
    body.innerHTML = "Unsaved work";
    fireEvent.input(body);
    profileNotesClient.fetchNote.mockClear();

    fireEvent.click(screen.getByRole("button", { name: "Second" }));

    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenCalledWith({
      note_id: "first",
      title: "First",
      body_markdown: "Unsaved work",
      expected_revision: "1",
      page_type: "text",
      parent_id: "",
      mcp_excluded: false,
    }));
    expect(profileNotesClient.saveNote.mock.invocationCallOrder[0]).toBeLessThan(profileNotesClient.fetchNote.mock.invocationCallOrder[0]);
  });

  it("flushes pending edits when leaving the Notes route", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "daily", title: "Daily", preview: "Original", page_type: "text", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
      parent_id: "",
    });
    profileNotesClient.saveNote.mockResolvedValue({ note_id: "daily", revision: "2" });

    const page = renderPage();
    const body = await screen.findByLabelText("Note body");
    body.innerHTML = "Save before leaving";
    fireEvent.input(body);

    page.unmount();

    expect(profileNotesClient.saveNote).toHaveBeenCalledWith({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Save before leaving",
      expected_revision: "1",
      page_type: "text",
      parent_id: "",
      mcp_excluded: false,
    });
  });

  it("keeps queued saves for different notes while another save is running", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [
        { note_id: "first", title: "First", preview: "A", page_type: "text", revision: "1", updated_at: "2026-07-10T12:00:00Z" },
        { note_id: "second", title: "Second", preview: "B", page_type: "text", revision: "1", updated_at: "2026-07-09T12:00:00Z" },
      ],
    });
    profileNotesClient.fetchNote.mockImplementation(async (id: string) => ({
      note_id: id,
      title: id === "first" ? "First" : "Second",
      body_markdown: id === "first" ? "A" : "B",
      revision: "1",
      page_type: "text",
      parent_id: "",
    }));
    let resolveFirst!: (value: { note_id: string; revision: string }) => void;
    let resolveSecond!: (value: { note_id: string; revision: string }) => void;
    profileNotesClient.saveNote
      .mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve; }))
      .mockReturnValueOnce(new Promise((resolve) => { resolveSecond = resolve; }))
      .mockResolvedValueOnce({ note_id: "second", revision: "2" });

    renderPage();
    const firstBody = await screen.findByLabelText("Note body");
    firstBody.innerHTML = "First save";
    fireEvent.input(firstBody);
    fireEvent.click(screen.getByRole("button", { name: "Save note" }));

    firstBody.innerHTML = "Latest first";
    fireEvent.input(firstBody);
    fireEvent.click(screen.getByRole("button", { name: "Second" }));
    const secondBody = await screen.findByLabelText("Note body");
    await waitFor(() => expect(secondBody).toHaveTextContent("B"));
    secondBody.innerHTML = "Latest second";
    fireEvent.input(secondBody);
    fireEvent.click(screen.getByRole("button", { name: "Save note" }));

    resolveFirst({ note_id: "first", revision: "2" });
    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenCalledTimes(2));
    expect(profileNotesClient.saveNote).toHaveBeenNthCalledWith(2, expect.objectContaining({
      note_id: "first",
      body_markdown: "Latest first",
      expected_revision: "2",
    }));

    resolveSecond({ note_id: "first", revision: "3" });
    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenCalledTimes(3));
    expect(profileNotesClient.saveNote).toHaveBeenNthCalledWith(3, expect.objectContaining({
      note_id: "second",
      body_markdown: "Latest second",
    }));
  });

  it("flushes the current note before creating a notebook", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "daily", title: "Daily", preview: "Original", page_type: "text", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
      parent_id: "",
    });
    profileNotesClient.saveNote
      .mockResolvedValueOnce({ note_id: "daily", revision: "2" })
      .mockResolvedValueOnce({ note_id: "family", revision: "1" });

    renderPage();
    const body = await screen.findByLabelText("Note body");
    body.innerHTML = "Dirty daily note";
    fireEvent.input(body);
    fireEvent.click(screen.getByRole("button", { name: "New notebook" }));
    fireEvent.change(screen.getByLabelText("Notebook name"), { target: { value: "Family" } });
    fireEvent.click(screen.getByRole("button", { name: "Create notebook" }));

    await waitFor(() => expect(profileNotesClient.saveNote).toHaveBeenCalledTimes(2));
    expect(profileNotesClient.saveNote).toHaveBeenNthCalledWith(1, expect.objectContaining({
      note_id: "daily",
      body_markdown: "Dirty daily note",
    }));
    expect(profileNotesClient.saveNote).toHaveBeenNthCalledWith(2, expect.objectContaining({
      note_id: "",
      title: "Family",
      page_type: "notebook",
    }));
  });

  it("does not queue an identical snapshot while that snapshot is saving", async () => {
    profileNotesClient.listNotes.mockResolvedValue({
      notes: [{ note_id: "daily", title: "Daily", preview: "Original", page_type: "text", revision: "1" }],
    });
    profileNotesClient.fetchNote.mockResolvedValue({
      note_id: "daily",
      title: "Daily",
      body_markdown: "Original",
      revision: "1",
      page_type: "text",
      parent_id: "",
    });
    let resolveSave!: (value: { note_id: string; revision: string }) => void;
    profileNotesClient.saveNote.mockReturnValue(new Promise((resolve) => { resolveSave = resolve; }));

    renderPage();
    const body = await screen.findByLabelText("Note body");
    body.innerHTML = "One snapshot";
    fireEvent.input(body);
    fireEvent.click(screen.getByRole("button", { name: "Save note" }));
    fireEvent.blur(screen.getByLabelText("Note title"));
    resolveSave({ note_id: "daily", revision: "2" });

    await waitFor(() => expect(screen.getByRole("button", { name: "Save note" })).toHaveTextContent("Saved"));
    expect(profileNotesClient.saveNote).toHaveBeenCalledTimes(1);
  });
});
