import { apiClient, type ApiTransport } from "../api/client";

type AppTicketResponse = {
  ticket: string;
  expires_at: string;
  websocket_path: string;
};

type RoutedCommandEnvelope = {
  version: "v1";
  type: "app.command";
  request_id: string;
  timestamp: string;
  agent_id?: string;
  payload: {
    command: string;
    body: unknown;
  };
};

type AppEnvelope = {
  type: "app.response" | "app.error" | "app.event" | string;
  request_id?: string;
  payload?: unknown;
  error?: {
    code: string;
    message: string;
    details?: unknown;
  };
};

export type HankSocketEvent = {
  topic?: string;
  event?: string;
  body?: unknown;
};

type PendingCommand = {
  resolve: (value: unknown) => void;
  reject: (reason: Error) => void;
  timeoutID: number;
};

export class HankSocketError extends Error {
  constructor(
    public readonly code: string,
    message: string,
    public readonly details?: unknown,
  ) {
    super(message);
    this.name = "HankSocketError";
  }
}

export class HankSocket {
  private socket: WebSocket | null = null;
  private pending = new Map<string, PendingCommand>();
  private listeners = new Set<(event: HankSocketEvent) => void>();

  constructor(
    private readonly api: ApiTransport = apiClient,
    private readonly createWebSocket: (url: string) => WebSocket = (url) => new WebSocket(url),
  ) {}

  async connect(): Promise<void> {
    if (this.socket?.readyState === WebSocket.OPEN) return;
    const ticket = await this.api.request<AppTicketResponse>("/v1/ws/app-ticket", { method: "POST" });
    const socket = this.createWebSocket(this.websocketURL(ticket.websocket_path));
    this.socket = socket;
    socket.addEventListener("message", (event) => this.handleMessage(event));
    socket.addEventListener("close", () => this.rejectAll(new HankSocketError("socket_closed", "Hank socket closed.")));
    socket.addEventListener("error", () => this.rejectAll(new HankSocketError("socket_error", "Hank socket failed.")));
    await new Promise<void>((resolve, reject) => {
      socket.addEventListener("open", () => resolve(), { once: true });
      socket.addEventListener("error", () => reject(new HankSocketError("socket_open_failed", "Hank socket failed to open.")), {
        once: true,
      });
    });
  }

  close() {
    this.socket?.close();
    this.socket = null;
    this.rejectAll(new HankSocketError("socket_closed", "Hank socket closed."));
  }

  async sendCommand<T>(command: string, body: unknown = {}, options: { timeoutMs?: number; agentID?: string } = {}): Promise<T> {
    await this.connect();
    const socket = this.socket;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      throw new HankSocketError("socket_not_open", "Hank socket is not open.");
    }
    const requestID = `req_${Date.now()}_${Math.random().toString(16).slice(2)}`;
    const envelope: RoutedCommandEnvelope = {
      version: "v1",
      type: "app.command",
      request_id: requestID,
      timestamp: new Date().toISOString(),
      payload: { command, body },
    };
    if (options.agentID) {
      // Target a specific agent (blank routes to the home's primary agent).
      envelope.agent_id = options.agentID;
    }
    const timeoutMs = options.timeoutMs ?? 30000;
    return new Promise<T>((resolve, reject) => {
      const timeoutID = window.setTimeout(() => {
        this.pending.delete(requestID);
        reject(new HankSocketError("command_timeout", `${command} timed out.`));
      }, timeoutMs);
      this.pending.set(requestID, {
        resolve: (value) => resolve(value as T),
        reject,
        timeoutID,
      });
      socket.send(JSON.stringify(envelope));
    });
  }

  subscribe(topics: string[], options?: { timeoutMs?: number }) {
    return this.sendCommand("app.subscribe", { topics }, options);
  }

  onEvent(listener: (event: HankSocketEvent) => void) {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private handleMessage(message: MessageEvent) {
    const envelope = JSON.parse(String(message.data)) as AppEnvelope;
    if (envelope.type === "app.event") {
      for (const listener of this.listeners) listener(envelope.payload as HankSocketEvent);
      return;
    }
    if (!envelope.request_id) return;
    const pending = this.pending.get(envelope.request_id);
    if (!pending) return;
    this.pending.delete(envelope.request_id);
    window.clearTimeout(pending.timeoutID);
    if (envelope.type === "app.error") {
      pending.reject(
        new HankSocketError(
          envelope.error?.code || "app_error",
          envelope.error?.message || "App command failed.",
          envelope.error?.details,
        ),
      );
      return;
    }
    pending.resolve(envelope.payload);
  }

  private rejectAll(error: Error) {
    for (const pending of this.pending.values()) {
      window.clearTimeout(pending.timeoutID);
      pending.reject(error);
    }
    this.pending.clear();
  }

  private websocketURL(path: string) {
    const url = new URL(path, window.location.origin);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    return url.toString();
  }
}
