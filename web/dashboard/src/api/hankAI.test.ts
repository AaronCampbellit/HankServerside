import { describe, expect, it, vi } from "vitest";
import { HankAIClient } from "./hankAI";
import type { ApiTransport } from "./client";

describe("HankAIClient", () => {
  it("loads sessions messages status and sends chat messages", async () => {
    const request = vi.fn(async <T>() => ({}) as T);
    const client = new HankAIClient({ request: request as unknown as ApiTransport["request"] });

    await client.status();
    await client.listSessions();
    await client.createSession();
    await client.listMessages("session-1");
    await client.sendMessage("session-1", "What is up?");
    await client.deleteSession("session-1");

    expect(request).toHaveBeenNthCalledWith(1, "/v1/home/assistant/status");
    expect(request).toHaveBeenNthCalledWith(2, "/v1/home/assistant/sessions");
    expect(request).toHaveBeenNthCalledWith(3, "/v1/home/assistant/sessions", { method: "POST" });
    expect(request).toHaveBeenNthCalledWith(4, "/v1/home/assistant/sessions/session-1/messages");
    expect(request).toHaveBeenNthCalledWith(5, "/v1/home/assistant/sessions/session-1/messages", {
      method: "POST",
      body: {
        content: "What is up?",
        attachments: [],
        device_context: {
          device_id: "hankserverside-dashboard",
          timezone: "UTC",
        },
      },
    });
    expect(request).toHaveBeenNthCalledWith(6, "/v1/home/assistant/sessions/session-1", { method: "DELETE" });
  });
});
