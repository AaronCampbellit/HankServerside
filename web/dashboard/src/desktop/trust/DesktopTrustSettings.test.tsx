import "fake-indexeddb/auto";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { encodeBase64URL } from "../base64url";
import { IndexedDBDesktopIdentityStore } from "../identityStore";
import { DesktopTrustSettings } from "./DesktopTrustSettings";

describe("DesktopTrustSettings", () => {
  afterEach(cleanup);

  beforeEach(async () => {
    localStorage.clear();
    await new IndexedDBDesktopIdentityStore().clearForTests();
  });

  it("shows metadata-only identities, password separation, and exact reset confirmation", async () => {
    const client = {
      snapshot: vi.fn().mockResolvedValue({
        configured: true,
        root: { generation: 1, fingerprint: "root-fp", algorithm: "P-256", public_key_spki: "spki" },
        identities: [{ identity_id: "id", identity_type: "endpoint", user_id: "", device_id: "", agent_id: "agent-1", fingerprint: "endpoint-fp", capabilities: ["desktop.view"] }],
      }),
      revokeOperator: vi.fn(),
      revokeEndpoint: vi.fn(),
      reset: vi.fn().mockResolvedValue({}),
    };
    render(<DesktopTrustSettings client={client as never} homeID="home" userID="user" />);
    expect(await screen.findByText("endpoint-fp")).toBeInTheDocument();
    expect(screen.getByText(/Password reset does not recover/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Reset confirmation"), { target: { value: "wrong" } });
    expect(screen.getByRole("button", { name: "Reset desktop trust" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Reset confirmation"), { target: { value: "reset desktop trust" } });
    await waitFor(() => expect(screen.getByRole("button", { name: "Reset desktop trust" })).toBeEnabled());
  });

  it("signs and submits a reviewed browser approval with the approved local identity", async () => {
    const store = new IndexedDBDesktopIdentityStore();
    const signerDeviceID = "browser-approved";
    await store.create(signerDeviceID);
    localStorage.setItem("hank-desktop-device-id", signerDeviceID);
    const requested = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, false, ["sign", "verify"]);
    const requestedSPKI = new Uint8Array(await crypto.subtle.exportKey("spki", requested.publicKey));
    const now = Date.now();
    const approvalRequest = {
      identity_type: "operator_device",
      identity_id: "dop_requested",
      device_id: "browser-requested",
      public_key_spki: encodeBase64URL(requestedSPKI),
      capabilities: ["operator.approve", "endpoint.approve", "trust.recover", "trust.rotate"],
      created_at: new Date(now - 1_000).toISOString(),
      expires_at: new Date(now + 60_000).toISOString(),
      platform: "browser",
    };
    const client = {
      snapshot: vi.fn().mockResolvedValue({
        configured: true,
        root: { generation: 1, fingerprint: "root-fp", algorithm: "P-256", public_key_spki: "spki" },
        identities: [{
          identity_id: "dop_approved",
          identity_type: "operator_device",
          user_id: "user",
          device_id: signerDeviceID,
          agent_id: "",
          fingerprint: "approved-fp",
          capabilities: ["operator.approve", "endpoint.approve", "trust.recover", "trust.rotate"],
        }],
      }),
      approveOperator: vi.fn().mockResolvedValue({}),
    };

    render(<DesktopTrustSettings client={client as never} homeID="home" userID="user" />);
    await screen.findByText("approved-fp");
    fireEvent.change(screen.getByLabelText("Desktop approval request"), { target: { value: JSON.stringify(approvalRequest) } });
    fireEvent.click(screen.getByRole("button", { name: "Review approval request" }));
    await screen.findByText("browser-requested");
    fireEvent.click(screen.getByRole("button", { name: "Compare fingerprint and approve" }));

    await waitFor(() => expect(client.approveOperator).toHaveBeenCalledOnce());
    expect(client.approveOperator).toHaveBeenCalledWith(expect.objectContaining({
      identity_id: "dop_requested",
      device_id: "browser-requested",
      certificate: expect.any(String),
    }));
  });

  it("allows re-enrollment when the server identity exists but this browser lost its private key", async () => {
    localStorage.setItem("hank-desktop-device-id", "browser-key-lost");
    const client = {
      snapshot: vi.fn().mockResolvedValue({
        configured: true,
        root: { generation: 1, fingerprint: "root-fp", algorithm: "P-256", public_key_spki: "spki" },
        identities: [{
          identity_id: "dop_stale",
          identity_type: "operator_device",
          user_id: "user",
          device_id: "browser-key-lost",
          agent_id: "",
          fingerprint: "stale-fingerprint",
          capabilities: ["operator.approve", "endpoint.approve", "trust.recover", "trust.rotate"],
        }],
      }),
    };

    render(<DesktopTrustSettings client={client as never} homeID="home" userID="user" />);

    await screen.findByText("stale-fingerprint");
    await waitFor(() => expect(screen.getByRole("button", { name: "Create this browser approval request" })).toBeEnabled());
  });
});
