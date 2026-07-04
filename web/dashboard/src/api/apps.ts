import { apiClient, type ApiTransport } from "./client";
import { arrayFrom } from "./normalize";

export type AppSettingsField = {
  key: string;
  label?: string;
  type: "boolean" | "number" | "select" | "url" | "text" | string;
  required?: boolean;
  secret?: boolean;
  secret_key?: string;
  source?: string;
  placeholder?: string;
  help?: string;
  default?: unknown;
  order?: number;
  options?: Array<{ value: unknown; label?: string }>;
};

export type AppSummary = {
  id?: string;
  app_id?: string;
  name?: string;
  version?: string;
  publisher?: string;
  description?: string;
  enabled?: boolean;
  status?: string;
  last_error?: string;
  user_access?: "admins_only" | "home_members" | string;
  public_config?: Record<string, unknown> | string;
  public_config_json?: string;
  secret_fields_set?: Record<string, boolean> | string;
  secret_fields_set_json?: string;
  settings_schema?: { fields?: AppSettingsField[] } | string;
  settings_schema_json?: string;
  updated_at?: string;
};

export type AppsListPayload = {
  apps: AppSummary[];
};

export type AppsPackagePreview = {
  staging_id: string;
  package_sha256?: string;
  replacing?: boolean;
  warnings?: string[];
  app: AppSummary;
};

export type AppsActivateInput = {
  staging_id: string;
  package_sha256?: string;
  enable: boolean;
};

export type AppsConfigInput = {
  public_config?: Record<string, unknown>;
  secrets?: Record<string, unknown>;
  enable?: boolean;
  user_access?: string;
};

export class AppsClient {
  constructor(private readonly api: ApiTransport = apiClient) {}

  async listApps(): Promise<AppsListPayload> {
    const payload = await this.api.request<Partial<AppsListPayload>>("/v1/home/apps");
    return { apps: arrayFrom<AppSummary>(payload.apps) };
  }

  previewPackage(formData: FormData) {
    return this.api.request<AppsPackagePreview>("/v1/home/apps/import/preview", {
      method: "POST",
      body: formData,
    });
  }

  activatePackage(input: AppsActivateInput) {
    return this.api.request<{ app: AppSummary }>("/v1/home/apps/import/activate", {
      method: "POST",
      body: input,
    });
  }

  saveConfig(appID: string, input: AppsConfigInput) {
    return this.api.request<{ app: AppSummary }>(`/v1/home/apps/${encodeURIComponent(appID)}/config`, {
      method: "PUT",
      body: input,
    });
  }
}

export const appsClient = new AppsClient();
