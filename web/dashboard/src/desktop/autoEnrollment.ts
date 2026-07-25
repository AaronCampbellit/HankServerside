import { encodeBase64URL, exactBuffer } from "./base64url";
import { IndexedDBDesktopIdentityStore } from "./identityStore";
import { desktopTrustClient, type DesktopAutoEnrollment } from "../api/desktopTrust";

const deviceIDKey = "hank-desktop-device-id";
const enrollmentInFlight = new Map<string, Promise<DesktopAutoEnrollment>>();

export function ensureBrowserDesktopIdentity(homeID: string, userID: string, sessionID: string): Promise<DesktopAutoEnrollment> {
  const scope = `${homeID}:${userID}:${sessionID}`;
  const existing = enrollmentInFlight.get(scope);
  if (existing) return existing;
  const operation = enrollBrowserDesktopIdentity(homeID, userID, sessionID).finally(() => enrollmentInFlight.delete(scope));
  enrollmentInFlight.set(scope, operation);
  return operation;
}

async function enrollBrowserDesktopIdentity(homeID: string, userID: string, sessionID: string): Promise<DesktopAutoEnrollment> {
  if (!homeID || !userID || !sessionID) throw new Error("desktop_browser_session_unavailable");
  let deviceID = localStorage.getItem(deviceIDKey) || "";
  if (!deviceID) { deviceID = `browser-${crypto.randomUUID()}`; localStorage.setItem(deviceIDKey, deviceID); }
  const store = new IndexedDBDesktopIdentityStore();
  let keyPair = await store.get(deviceID);
  let spki = await store.getPublicSPKI(deviceID);
  if (!keyPair || !spki) {
    const created = await store.create(deviceID);
    keyPair = created.keyPair;
    spki = created.spki;
  }
  const challenge = await desktopTrustClient.autoChallenge(deviceID);
  const transcript = enrollmentTranscript(homeID, userID, sessionID, "browser_operator", challenge.challenge_id, challenge.challenge, deviceID, spki);
  const signature = new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, keyPair.privateKey, exactBuffer(transcript)));
  const enrollment = await desktopTrustClient.autoEnroll({ challenge_id: challenge.challenge_id, challenge: challenge.challenge, installation_id: deviceID,
    device_id: deviceID, public_key_spki: encodeBase64URL(spki), signature: encodeBase64URL(signature) });
  window.dispatchEvent(new Event("hank-desktop-auto-enrolled"));
  return enrollment;
}

function enrollmentTranscript(homeID: string, userID: string, sessionID: string, purpose: string, challengeID: string, challenge: string, installationID: string, spki: Uint8Array): Uint8Array {
  return new TextEncoder().encode(["Hank Desktop Device Enrollment v1", homeID, userID, sessionID, purpose, challengeID, challenge, installationID, encodeBase64URL(spki)].join("\n"));
}
