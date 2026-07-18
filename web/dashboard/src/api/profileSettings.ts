import { apiClient, type ApiTransport } from "./client";

export type ProfileSettings = Record<string, unknown>;

export type ProfileSettingsResponse = {
  revision: number;
  settings: ProfileSettings;
};

export class ProfileSettingsClient {
  constructor(private readonly api: ApiTransport = apiClient) {}

  load() {
    return this.api.request<ProfileSettingsResponse>("/v1/me/profile");
  }

  save(expectedRevision: number, settings: ProfileSettings) {
    return this.api.request<ProfileSettingsResponse>("/v1/me/profile", {
      method: "PUT",
      body: { expected_revision: expectedRevision, settings },
    });
  }
}

export function mergeDefaultKanbanBoard(settings: ProfileSettings, boardID: string): ProfileSettings {
  const next = { ...settings };
  if (boardID) next.kanban_default_board_id = boardID;
  else delete next.kanban_default_board_id;
  return next;
}

export const profileSettingsClient = new ProfileSettingsClient();
