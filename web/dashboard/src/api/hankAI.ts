import { apiClient, type ApiTransport } from "./client";
import { arrayFrom } from "./normalize";

export type HankAISession = {
  id: string;
  title?: string;
  last_message_at?: string;
};

export type HankAIMessage = {
  id?: string;
  role: "user" | "assistant" | string;
  text?: string;
  content?: string;
  created_at?: string;
};

export type HankAIRun = {
  id: string;
  state: string;
  assistant_message?: HankAIMessage;
};

export class HankAIClient {
  constructor(private readonly api: ApiTransport = apiClient) {}

  status() {
    return this.api.request<Record<string, unknown>>("/v1/home/assistant/status");
  }

  async listSessions(): Promise<{ sessions: HankAISession[] }> {
    const payload = await this.api.request<{ sessions?: HankAISession[] }>("/v1/home/assistant/sessions");
    return { sessions: arrayFrom<HankAISession>(payload.sessions) };
  }

  createSession() {
    return this.api.request<HankAISession>("/v1/home/assistant/sessions", { method: "POST" });
  }

  async listMessages(sessionID: string): Promise<{ messages: HankAIMessage[] }> {
    const payload = await this.api.request<{ messages?: HankAIMessage[] }>(`/v1/home/assistant/sessions/${encodeURIComponent(sessionID)}/messages`);
    return { messages: arrayFrom<HankAIMessage>(payload.messages) };
  }

  sendMessage(sessionID: string, content: string) {
    return this.api.request<HankAIRun>(`/v1/home/assistant/sessions/${encodeURIComponent(sessionID)}/messages`, {
      method: "POST",
      body: {
        content,
        attachments: [],
        device_context: {
          device_id: "hankserverside-dashboard",
          timezone: "UTC",
        },
      },
    });
  }

  deleteSession(sessionID: string) {
    return this.api.request<{ ok: boolean }>(`/v1/home/assistant/sessions/${encodeURIComponent(sessionID)}`, {
      method: "DELETE",
    });
  }
}

export const hankAIClient = new HankAIClient();
