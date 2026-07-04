import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HankAIPage } from "./HankAIPage";

const hankAIClient = vi.hoisted(() => ({
  status: vi.fn(),
  listSessions: vi.fn(),
  listMessages: vi.fn(),
  createSession: vi.fn(),
  sendMessage: vi.fn(),
}));

vi.mock("../api/hankAI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/hankAI")>();
  return {
    ...actual,
    hankAIClient,
  };
});

describe("HankAIPage", () => {
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

    render(<HankAIPage />);

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

    render(<HankAIPage />);

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
});
