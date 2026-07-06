import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfirmDialogProvider, ToastProvider } from "../ui/primitives";
import { ProfileNotesPage } from "./ProfileNotesPage";

const profileNotesClient = vi.hoisted(() => ({
  listNotes: vi.fn(),
  fetchNote: vi.fn(),
  saveNote: vi.fn(),
  deleteNote: vi.fn(),
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
    expect(body).toHaveDisplayValue(/# Taco Night/);
    expect(body).toHaveDisplayValue(/Tortillas/);
    expect(body).toHaveDisplayValue(/Maya is bringing dessert/);
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
    expect(screen.getByLabelText("Note body")).toHaveDisplayValue(/# Roof Warranty/);
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

    fireEvent.change(body, { target: { value: "Remember milk\nCall Sam" } });
    fireEvent.click(screen.getByRole("button", { name: "Save note" }));

    expect(profileNotesClient.saveNote).toHaveBeenCalledWith({
      note_id: "daily",
      title: "Daily Notes",
      body_markdown: "Remember milk\nCall Sam",
      expected_revision: "1",
      page_type: "text",
      parent_id: "",
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

  it("applies text tools to the selection and supports undo/redo", async () => {
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

    const body = await screen.findByLabelText("Note body") as HTMLTextAreaElement;
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();

    body.setSelectionRange(0, 8);
    fireEvent.click(screen.getByRole("button", { name: "Bold" }));
    expect(body).toHaveDisplayValue("**Remember** milk");

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(body).toHaveDisplayValue("Remember milk");

    fireEvent.click(screen.getByRole("button", { name: "Redo" }));
    expect(body).toHaveDisplayValue("**Remember** milk");

    body.setSelectionRange(0, body.value.length);
    fireEvent.click(screen.getByRole("button", { name: "Bulleted list" }));
    expect(body).toHaveDisplayValue("- **Remember** milk");

    fireEvent.click(screen.getByRole("button", { name: "Heading" }));
    expect(body).toHaveDisplayValue("# **Remember** milk");
  });

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
    }));
  });
});
