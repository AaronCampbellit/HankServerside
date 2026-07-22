import { apiClient, type ApiTransport } from "./client";
import type { DesktopTrustSnapshot } from "./desktop";

export interface DesktopIdentityRequest { identity_id: string; device_id?: string; public_key_spki?: string; certificate?: string; capabilities?: string[]; expires_at?: string; confirmation?: string }
export class DesktopTrustClient {
  constructor(private readonly api: ApiTransport = apiClient) {}
  snapshot(): Promise<DesktopTrustSnapshot> { return this.api.request("/v1/home/desktop-trust"); }
  bootstrap(body: Record<string, unknown>): Promise<unknown> { return this.api.request("/v1/home/desktop-trust/operator-devices", { method: "POST", body: { ...body, confirmation: "create desktop trust" } }); }
  approveOperator(body: DesktopIdentityRequest): Promise<unknown> { return this.api.request("/v1/home/desktop-trust/operator-devices", { method: "POST", body: { ...body } }); }
  approveEndpoint(agentID: string, body: Record<string, unknown>): Promise<unknown> { return this.api.request(`/v1/home/desktop-trust/endpoints/${encodeURIComponent(agentID)}/approve`, { method: "POST", body }); }
  revokeOperator(deviceID: string, reason = "administrator_revoked"): Promise<unknown> { return this.api.request(`/v1/home/desktop-trust/operator-devices/${encodeURIComponent(deviceID)}/revoke`, { method: "POST", body: { reason, confirmation: "revoke desktop identity" } }); }
  revokeEndpoint(agentID: string, reason = "administrator_revoked"): Promise<unknown> { return this.api.request(`/v1/home/desktop-trust/endpoints/${encodeURIComponent(agentID)}/revoke`, { method: "POST", body: { reason, confirmation: "revoke desktop identity" } }); }
  recoveryChallenge(generation: number, operator: Partial<DesktopIdentityRequest>): Promise<{ challenge: string; expires_in: number }> { return this.api.request("/v1/home/desktop-trust/recovery", { method: "POST", body: { generation, operator, challenge: "", root_signature: "", confirmation: "recover desktop trust" } }); }
  recover(body: Record<string, unknown>): Promise<unknown> { return this.api.request("/v1/home/desktop-trust/recovery", { method: "POST", body: { ...body, confirmation: "recover desktop trust" } }); }
  rotate(body: Record<string, unknown>): Promise<unknown> { return this.api.request("/v1/home/desktop-trust/rotate", { method: "POST", body: { ...body, confirmation: "rotate desktop trust" } }); }
  reset(body: Record<string, unknown>): Promise<unknown> { return this.api.request("/v1/home/desktop-trust/reset", { method: "POST", body: { ...body, confirmation: "reset desktop trust" } }); }
}
export const desktopTrustClient = new DesktopTrustClient();
