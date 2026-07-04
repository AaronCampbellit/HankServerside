import { apiClient, type ApiTransport } from "./client";
import { arrayFrom } from "./normalize";

export type NotificationTone = "info" | "warning" | "danger" | "success" | string;

export type NotificationItem = {
  id: string;
  title: string;
  body?: string;
  tone?: NotificationTone;
  url?: string;
  created_at?: string;
};

type NotificationsResponse = {
  notifications?: NotificationItem[];
};

export class NotificationsClient {
  constructor(private readonly api: ApiTransport = apiClient) {}

  async list(signal?: AbortSignal): Promise<NotificationItem[]> {
    const payload = await this.api.request<NotificationsResponse>("/v1/home/notifications", {
      signal,
      timeoutMs: 8000,
    });
    return arrayFrom<NotificationItem>(payload?.notifications);
  }
}

export const notificationsClient = new NotificationsClient();
