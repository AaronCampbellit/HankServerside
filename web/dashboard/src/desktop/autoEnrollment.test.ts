import { describe, expect, it } from "vitest";
import { exactBuffer, encodeBase64URL } from "./base64url";
import { p256DERToP1363 } from "./crypto";
import { signBrowserEnrollment } from "./autoEnrollment";

describe("browser desktop auto-enrollment", () => {
  it("uses the DER ECDSA format accepted by the server", async () => {
    const identity = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, false, ["sign", "verify"]);
    const transcript = new TextEncoder().encode("Hank Desktop Device Enrollment v1");
    const signature = await signBrowserEnrollment(identity.privateKey, transcript);

    expect(signature[0]).toBe(0x30);
    expect(encodeBase64URL(signature)).not.toBe(encodeBase64URL(p256DERToP1363(signature)));
    await expect(crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, identity.publicKey,
      exactBuffer(p256DERToP1363(signature)), exactBuffer(transcript))).resolves.toBe(true);
  });
});
