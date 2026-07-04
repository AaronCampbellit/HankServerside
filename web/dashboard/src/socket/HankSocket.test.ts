import { describe, expect, it, vi } from "vitest";
import { HankSocket } from "./HankSocket";

class FakeWebSocket extends EventTarget {
  static OPEN = 1;
  readyState = FakeWebSocket.OPEN;
  sent: string[] = [];

  constructor(public readonly url: string) {
    super();
  }

  send(message: string) {
    this.sent.push(message);
  }

  close() {
    this.dispatchEvent(new Event("close"));
  }

  receive(payload: unknown) {
    this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify(payload) }));
  }

  open() {
    this.dispatchEvent(new Event("open"));
  }
}

describe("HankSocket", () => {
  it("opens with an app ticket and sends command envelopes", async () => {
    const requests: Array<[string, unknown]> = [];
    const api = {
      async request<T>(path: string, options?: unknown) {
        requests.push([path, options]);
        return {
          ticket: "ticket",
          expires_at: "2026-06-27T00:00:00Z",
          websocket_path: "/ws/app?app_ticket=ticket",
        } as T;
      },
    };
    const sockets: FakeWebSocket[] = [];
    const hankSocket = new HankSocket(api, (url) => {
      const socket = new FakeWebSocket(url);
      sockets.push(socket);
      queueMicrotask(() => socket.open());
      return socket as unknown as WebSocket;
    });

    const response = hankSocket.sendCommand<{ ok: boolean }>("files.list", { path: "/" });
    await vi.waitFor(() => expect(sockets[0]?.sent.length).toBe(1));
    const socket = sockets[0];
    const envelope = JSON.parse(socket.sent[0] || "{}");
    expect(requests).toEqual([["/v1/ws/app-ticket", { method: "POST" }]]);
    expect(socket.url).toBe("ws://localhost:3000/ws/app?app_ticket=ticket");
    expect(envelope.type).toBe("app.command");
    expect(envelope.version).toBe("v1");
    expect(envelope.payload.command).toBe("files.list");

    socket.receive({ type: "app.response", request_id: envelope.request_id, payload: { ok: true } });
    await expect(response).resolves.toEqual({ ok: true });
  });

  it("emits app events", async () => {
    const api = {
      async request<T>() {
        return {
          ticket: "ticket",
          expires_at: "2026-06-27T00:00:00Z",
          websocket_path: "/ws/app?app_ticket=ticket",
        } as T;
      },
    };
    const sockets: FakeWebSocket[] = [];
    const hankSocket = new HankSocket(api, (url) => {
      const socket = new FakeWebSocket(url);
      sockets.push(socket);
      queueMicrotask(() => socket.open());
      return socket as unknown as WebSocket;
    });
    const listener = vi.fn();
    hankSocket.onEvent(listener);

    await hankSocket.connect();
    sockets[0].receive({ type: "app.event", payload: { topic: "files.jobs", event: "files.job_changed" } });

    expect(listener).toHaveBeenCalledWith({ topic: "files.jobs", event: "files.job_changed" });
  });
});
