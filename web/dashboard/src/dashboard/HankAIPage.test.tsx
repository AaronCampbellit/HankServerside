import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConfirmDialogProvider } from "../ui/primitives";
import { HankAIPage } from "./HankAIPage";

const hankAIClient = vi.hoisted(() => ({
  status: vi.fn(),
  listSessions: vi.fn(),
  listMessages: vi.fn(),
  createSession: vi.fn(),
  sendMessage: vi.fn(),
  deleteSession: vi.fn(),
}));

const appsClient = vi.hoisted(() => ({
  listApps: vi.fn(),
}));

vi.mock("../api/hankAI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/hankAI")>();
  return {
    ...actual,
    hankAIClient,
  };
});

vi.mock("../api/apps", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/apps")>();
  return {
    ...actual,
    appsClient,
  };
});

describe("HankAIPage", () => {
  beforeEach(() => {
    appsClient.listApps.mockResolvedValue({ apps: [] });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("does not show synthetic workflow logs or fake attachments", async () => {
    hankAIClient.status.mockResolvedValue({ provider: "gpt-4o", ready: true });
    hankAIClient.listSessions.mockResolvedValue({
      sessions: [{ id: "s1", title: "Weekend grocery plan", last_message_at: "now" }],
    });
    hankAIClient.listMessages.mockResolvedValue({
      messages: [{ role: "assistant", text: "Storage looks good for the backup." }],
    });

    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

    expect(await screen.findByRole("button", { name: "Weekend grocery plan" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Workflow logs" })).not.toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: "Workflow logs" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Attach a file" })).not.toBeInTheDocument();
    expect(screen.queryByText("lease-2026.pdf")).not.toBeInTheDocument();
  });

  it("shows live conversation messages without canned tool review cards", async () => {
    hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
    hankAIClient.listSessions.mockResolvedValue({
      sessions: [
        { id: "s1", title: "Weekend grocery plan", last_message_at: "now" },
        { id: "s2", title: "Lower thermostat at night", last_message_at: "yesterday" },
        { id: "s3", title: "Find the lease PDF", last_message_at: "Monday" },
      ],
    });
    hankAIClient.listMessages.mockResolvedValue({
      messages: [
        { role: "user", text: "Can you add taco night groceries to the shared list?" },
        { role: "assistant", text: "I found the shared grocery list and staged the taco night items." },
      ],
    });

    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

    expect(await screen.findByRole("button", { name: "Weekend grocery plan" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lower thermostat at night" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Find the lease PDF" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Hank" })).toBeInTheDocument();
    expect(screen.getByText("Can you add taco night groceries to the shared list?")).toBeInTheDocument();
    expect(screen.getByText("I found the shared grocery list and staged the taco night items.")).toBeInTheDocument();
    expect(screen.queryByText("Search shared grocery list")).not.toBeInTheDocument();
    expect(screen.queryByText("Append 6 grocery items")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
  });

  it("deletes a conversation from the list after confirmation", async () => {
    hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
    hankAIClient.listSessions.mockResolvedValue({
      sessions: [
        { id: "s1", title: "Keep this chat", last_message_at: "now" },
        { id: "s2", title: "Delete this chat", last_message_at: "yesterday" },
      ],
    });
    hankAIClient.listMessages.mockResolvedValue({ messages: [] });
    hankAIClient.deleteSession.mockResolvedValue({ ok: true });

    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

    fireEvent.click(await screen.findByRole("button", { name: "Delete Delete this chat" }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));

    await waitFor(() => expect(hankAIClient.deleteSession).toHaveBeenCalledWith("s2"));
    expect(screen.queryByRole("button", { name: "Delete this chat" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Keep this chat" })).toBeInTheDocument();
  });

  it("loads the next conversation when deleting the selected chat", async () => {
    hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
    hankAIClient.listSessions.mockResolvedValue({
      sessions: [
        { id: "s1", title: "Selected chat", last_message_at: "now" },
        { id: "s2", title: "Next chat", last_message_at: "yesterday" },
      ],
    });
    hankAIClient.listMessages.mockImplementation(async (id: string) => ({
      messages: id === "s1" ? [{ role: "assistant", text: "Selected message" }] : [{ role: "assistant", text: "Next message" }],
    }));
    hankAIClient.deleteSession.mockResolvedValue({ ok: true });

    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

    fireEvent.click(await screen.findByRole("button", { name: "Delete Selected chat" }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));

    await waitFor(() => expect(hankAIClient.listMessages).toHaveBeenCalledWith("s2"));
    expect(await screen.findByText("Next message")).toBeInTheDocument();
  });

  it("keeps a deleted conversation removed when fallback message loading fails", async () => {
    hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
    hankAIClient.listSessions.mockResolvedValue({
      sessions: [
        { id: "s1", title: "Selected chat", last_message_at: "now" },
        { id: "s2", title: "Next chat", last_message_at: "yesterday" },
      ],
    });
    hankAIClient.listMessages
      .mockResolvedValueOnce({ messages: [{ role: "assistant", text: "Selected message" }] })
      .mockRejectedValueOnce(new Error("message load failed"));
    hankAIClient.deleteSession.mockResolvedValue({ ok: true });

    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

    fireEvent.click(await screen.findByRole("button", { name: "Delete Selected chat" }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));

    await waitFor(() => expect(screen.queryByRole("button", { name: "Selected chat" })).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Next chat" })).toBeInTheDocument();
    expect(screen.getByText("message load failed")).toBeInTheDocument();
  });

  it("shows enabled installed app slash commands and inserts the selected token", async () => {
    hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
    hankAIClient.listSessions.mockResolvedValue({ sessions: [] });
    appsClient.listApps.mockResolvedValue({
      apps: [{
        id: "gramaton",
        name: "Gramaton",
        enabled: true,
        slash_commands: [{
          command: "/gramaton",
          command_id: "search",
          description: "Search for a movie or TV show on Gramaton.",
        }],
      }],
    });

    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

    const composer = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(composer, { target: { value: "/gra" } });
    fireEvent.mouseDown(screen.getByRole("option", { name: /gramaton/i }));

    expect(composer).toHaveValue("/gramaton ");
    expect(appsClient.listApps).toHaveBeenCalledTimes(1);
  });

  it("excludes disabled installed app slash commands", async () => {
    hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
    hankAIClient.listSessions.mockResolvedValue({ sessions: [] });
    appsClient.listApps.mockResolvedValue({
      apps: [{
        id: "gramaton",
        name: "Gramaton",
        enabled: false,
        slash_commands: [{ command: "/gramaton", command_id: "search" }],
      }],
    });

    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

    const composer = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(composer, { target: { value: "/" } });
    expect(screen.queryByRole("option", { name: /gramaton/i })).not.toBeInTheDocument();
  });

  it("keeps built-in commands when installed app discovery fails", async () => {
    hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
    hankAIClient.listSessions.mockResolvedValue({ sessions: [] });
    appsClient.listApps.mockRejectedValue(new Error("apps unavailable"));

    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

    const composer = await screen.findByRole("textbox", { name: "Message" });
    fireEvent.change(composer, { target: { value: "/fi" } });
    expect(screen.getByRole("option", { name: /files/i })).toBeInTheDocument();
  });
});
