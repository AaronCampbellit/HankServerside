# Hank Native Remote Desktop Milestone 2 Synthetic End-to-End Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a browser-to-Hankagent Remote Desktop session that exchanges deterministic synthetic H.264 video, input, clipboard text, statistics, termination, and reconnect messages through an opaque end-to-end encrypted HankServerside relay on both macOS and Windows.

**Architecture:** Milestone 1 remains the canonical authorization, trust, session, and fixture layer. This milestone adds the first in-process binary relay, a browser viewer and cryptographic record layer, and matching Swift/.NET `DesktopSessionCoordinator` implementations backed by deterministic synthetic hosts. The server authenticates and pairs sockets but forwards only opaque ciphertext; browser and endpoint independently validate signed ephemeral handshakes and derive fresh directional keys for each epoch.

**Tech Stack:** Go 1.24, `github.com/coder/websocket`, React 19, TypeScript 6, Vite 8, Vitest/jsdom, WebCrypto, IndexedDB, WebCodecs with Media Source Extensions fallback, Swift 6/Foundation/CryptoKit, .NET 8 `ClientWebSocket`/`ECDiffieHellman`/`ECDsa`/`HKDF`/`AesGcm`, and a committed Annex-B H.264 fixture generated once with FFmpeg.

## Global Constraints

- Begin only after every Milestone 1 exit criterion in `2026-07-21-native-remote-desktop-foundation.md` passes.
- Preserve the exact `desktop.v1` JSON, transcript, certificate, recovery, HKDF, nonce, record-header, state, permission, and reason-code contracts.
- Browser and endpoint perform ECDSA P-256 verification, ephemeral ECDH P-256, HKDF-SHA-256, and AES-256-GCM; HankServerside never receives derived keys.
- Every reconnect uses fresh browser and endpoint ephemeral keys, increments the key epoch exactly once, and zeroizes the prior epoch.
- Each direction requires the exact next 64-bit sequence number; duplicate, skipped, reversed, wrong-direction, wrong-epoch, and invalid-tag records terminate the epoch.
- Browser and agent join credentials remain distinct, hashed server-side, single-use, URL-free, and delivered only through the Milestone 1 cookie/control-plane mechanisms.
- The relay accepts binary frames only, never parses an encrypted inner message, never retains payload samples, and emits payload-free counters/lifecycle metadata.
- The synthetic host may echo authorized input/clipboard state for proof, but it must not capture the physical console or inject native input.
- The synthetic H.264 asset is generated once and committed; runtime code has no FFmpeg dependency.
- Hankagent advertises desktop capabilities only when its synthetic coordinator, identity key, local indicator, and data-plane client are all ready.
- No native capture, native input injection, privileged helper, UAC, Screen Recording, Accessibility, packaging, installation, deployment, or push occurs in this milestone.

---

### Task 1: Versioned encrypted inner-message contract

**Files:**
- Create: `internal/protocol/desktop_data.go`
- Create: `internal/protocol/desktop_data_test.go`
- Create: `web/dashboard/src/desktop/protocol.ts`
- Create: `web/dashboard/src/desktop/protocol.test.ts`
- Modify: `schemas/desktop/v1/test-vectors.json`

**Interfaces:**
- Consumes: Milestone 1 `RecordHeader`, direction, epoch, and fixture encodings.
- Produces: exact pre-key data-plane frame kinds, inner type IDs, bounded binary encoders/decoders, optional/required behavior, and cross-language handshake/record vectors.

- [x] **Step 1: Add failing Go and TypeScript contract tests**

```go
func TestDesktopInnerMessageTypeCompatibility(t *testing.T) {
	want := map[uint16]string{1:"codec_config", 2:"video_access_unit", 3:"pointer_shape", 4:"display_inventory", 10:"keyboard", 11:"pointer", 12:"clipboard_offer", 13:"clipboard_text", 20:"control_mode", 21:"quality", 30:"ping", 31:"pong", 32:"statistics", 40:"secure_state", 41:"permission_state", 255:"terminate"}
	for id, name := range want { if got := DesktopInnerMessageNames[id]; got != name { t.Fatalf("type %d = %q", id, got) } }
}

func TestDesktopRequiredUnknownMessageFailsClosed(t *testing.T) {
	_, err := DecodeDesktopInnerMessage([]byte{1, 0, 0x7f, 0xff, 0, 0, 0, 0})
	if !errors.Is(err, ErrDesktopRequiredMessageUnknown) { t.Fatalf("Decode = %v", err) }
}

func TestDesktopDataPlaneFrameKindsAreStable(t *testing.T) {
	if DesktopFrameBrowserHandshake != 1 || DesktopFrameAgentHandshake != 2 || DesktopFrameEncryptedRecord != 3 { t.Fatal("data-plane frame ids changed") }
}
```

```ts
it("matches the canonical inner type ids", () => {
  expect(DesktopMessageType.VideoAccessUnit).toBe(2);
  expect(DesktopMessageType.Keyboard).toBe(10);
  expect(DesktopMessageType.Terminate).toBe(255);
});
```

- [x] **Step 2: Run the focused tests and verify the red state**

Run: `go test ./internal/protocol -run DesktopInner -count=1`

Run: `cd web/dashboard && npm test -- --run src/desktop/protocol.test.ts`

Expected: both fail because the inner protocol does not exist.

- [x] **Step 3: Implement pre-key framing plus the fixed eight-byte inner header and bounds**

```go
type DesktopInnerHeader struct { Version byte; Flags byte; Type uint16; PayloadLength uint32 }
type DesktopDataFrameHeader struct { Kind byte; PayloadLength uint32 }
const DesktopFrameBrowserHandshake byte = 1
const DesktopFrameAgentHandshake byte = 2
const DesktopFrameEncryptedRecord byte = 3
const DesktopInnerVersion byte = 1
const DesktopInnerOptional byte = 1
const DesktopMaxControlPayload = 256 << 10
const DesktopMaxVideoPayload = 4 << 20

func (h DesktopInnerHeader) MarshalBinary() ([]byte, error) {
	limit := uint32(DesktopMaxControlPayload)
	if h.Type == DesktopMessageVideoAccessUnit { limit = DesktopMaxVideoPayload }
	if h.Version != DesktopInnerVersion || h.PayloadLength > limit { return nil, ErrDesktopInnerBounds }
	out := make([]byte, 8); out[0], out[1] = h.Version, h.Flags
	binary.BigEndian.PutUint16(out[2:4], h.Type); binary.BigEndian.PutUint32(out[4:8], h.PayloadLength)
	return out, nil
}
```

The pre-key header is 12 bytes: magic `HDV1` (four bytes), kind (one byte), three reserved zero bytes, and big-endian payload length (four bytes). Browser handshake must be the first browser frame, agent handshake the first agent response, and only encrypted-record frames are accepted after signature/key activation. The relay forwards all three kinds opaquely and does not parse them. Mirror both headers in TypeScript with `DataView`; define concrete payload types for codec configuration, access unit, display inventory, input, clipboard, mode, quality, ping/pong, statistics, state, and termination. Use UTF-8 JSON only inside the encrypted body for Milestone 2 control payloads; video access-unit payloads remain binary.

- [x] **Step 4: Extend canonical vectors with valid/invalid inner records**

Add concrete vectors for browser handshake frame, endpoint handshake frame, premature encrypted frame, duplicate handshake, codec configuration, one keyframe, input echo, clipboard echo, ping/pong, termination, unknown optional, unknown required, oversized payload, wrong epoch, duplicate sequence, and bad tag. The Go fixture test writes only under `UPDATE_DESKTOP_FIXTURES=1`; normal tests read and compare.

- [x] **Step 5: Run the shared inner contract checks**

Run: `go test ./internal/protocol ./internal/desktopcrypto -count=1 && cd web/dashboard && npm test -- --run src/desktop/protocol.test.ts`

Expected: PASS.

```bash
git add internal/protocol/desktop_data.go internal/protocol/desktop_data_test.go schemas/desktop/v1/test-vectors.json web/dashboard/src/desktop/protocol.ts web/dashboard/src/desktop/protocol.test.ts
git commit -m "feat(remote-desktop): define encrypted data protocol"
```

### Task 2: Browser cryptography, identity storage, and epoch state

**Files:**
- Create: `web/dashboard/src/desktop/base64url.ts`
- Create: `web/dashboard/src/desktop/crypto.ts`
- Create: `web/dashboard/src/desktop/crypto.test.ts`
- Create: `web/dashboard/src/desktop/identityStore.ts`
- Create: `web/dashboard/src/desktop/identityStore.test.ts`
- Create: `web/dashboard/src/desktop/recordLayer.ts`
- Create: `web/dashboard/src/desktop/recordLayer.test.ts`

**Interfaces:**
- Consumes: canonical fixtures and inner protocol from Task 1.
- Produces: `DesktopIdentityStore`, `createBrowserHandshake`, `completeBrowserHandshake`, and `DesktopRecordLayer`.

- [x] **Step 1: Write failing vector, non-exportability, replay, and zeroization tests**

```ts
it("derives and decrypts the canonical browser epoch", async () => {
  const epoch = await completeBrowserHandshake(canonicalHandshakeInput());
  expect(rawBase64URL(epoch.browserToAgentKey)).toBe(fixture.valid_initial_join.hkdf.browser_to_agent_key_base64url);
});

it("rejects duplicate sequence and zeroizes replaced epoch", async () => {
  const layer = await DesktopRecordLayer.fromFixture(fixture);
  await layer.decrypt(fixture.valid_initial_join.record.bytes);
  await expect(layer.decrypt(fixture.valid_initial_join.record.bytes)).rejects.toThrow("desktop_sequence_mismatch");
  layer.replaceEpoch(2, replacementKeys());
  expect(layer.debugKeyState()).toEqual({ epoch: 2, priorEpochPresent: false });
});
```

- [x] **Step 2: Run and verify the browser crypto tests fail**

Run: `cd web/dashboard && npm test -- --run src/desktop/crypto.test.ts src/desktop/identityStore.test.ts src/desktop/recordLayer.test.ts`

Expected: FAIL because the modules are absent.

- [x] **Step 3: Implement non-exportable IndexedDB identity storage**

```ts
export interface DesktopIdentityStore {
  get(deviceID: string): Promise<CryptoKeyPair | null>;
  create(deviceID: string): Promise<{ keyPair: CryptoKeyPair; spki: Uint8Array }>;
  remove(deviceID: string): Promise<void>;
}

const algorithm: EcKeyGenParams = { name: "ECDSA", namedCurve: "P-256" };
const keyPair = await crypto.subtle.generateKey(algorithm, false, ["sign", "verify"]);
```

Add `fake-indexeddb` as a dev dependency and load it only from the test setup. Store the structured-cloneable non-exportable private `CryptoKey` and public key in IndexedDB database `hank-desktop-v1`, object store `operator-identities`, keyed by device ID. Export SPKI from the public key only. Tests reopen the database, recover the structured-cloned key, sign successfully, and prove `subtle.exportKey("pkcs8", privateKey)` rejects.

- [x] **Step 4: Implement signed ECDH handshake and record layer**

`createBrowserHandshake` generates a fresh non-exportable P-256 ECDH pair, encodes the Milestone 1 transcript, and signs its SHA-256 digest with the operator ECDSA key. `completeBrowserHandshake` verifies the endpoint certificate/signature, derives four HKDF outputs, and returns `CryptoKey` objects plus nonce prefixes. `DesktopRecordLayer.encrypt/decrypt` authenticates the outer header as AES-GCM AAD and increments exact sequence counters only after success.

- [x] **Step 5: Run browser security primitive checks**

Run: `cd web/dashboard && npm test -- --run src/desktop/crypto.test.ts src/desktop/identityStore.test.ts src/desktop/recordLayer.test.ts && npm run typecheck`

Expected: PASS.

```bash
git add web/dashboard/package.json web/dashboard/package-lock.json web/dashboard/src/desktop/base64url.ts web/dashboard/src/desktop/crypto.ts web/dashboard/src/desktop/crypto.test.ts web/dashboard/src/desktop/identityStore.ts web/dashboard/src/desktop/identityStore.test.ts web/dashboard/src/desktop/recordLayer.ts web/dashboard/src/desktop/recordLayer.test.ts
git commit -m "feat(remote-desktop): add browser encrypted epochs"
```

### Task 3: In-process opaque binary relay

**Files:**
- Modify: `internal/cloud/desktop_relay.go`
- Modify: `internal/cloud/desktop_relay_test.go`
- Create: `internal/cloud/desktop_websocket.go`
- Create: `internal/cloud/desktop_websocket_test.go`
- Modify: `internal/cloud/server.go`

**Interfaces:**
- Consumes: Milestone 1 relay interface, hashed credential consumption, durable session state, and lifecycle sink.
- Produces: browser/agent WebSocket endpoints and `inProcessDesktopRelay`.

- [x] **Step 1: Write failing pairing, opacity, and limit tests**

```go
func TestDesktopRelayForwardsOpaqueBinaryWithoutRetention(t *testing.T) {
	relay := newInProcessDesktopRelay(testDesktopRelayLimits(), recordRelayLifecycle)
	browser, agent := pairedFakeRelayEndpoints()
	mustJoinDesktopRelay(t, relay, "desk_1", "browser", 1, browser)
	mustJoinDesktopRelay(t, relay, "desk_1", "agent", 1, agent)
	payload := []byte("opaque-ciphertext-not-plaintext")
	browser.Receive <- payload
	if got := <-agent.Sent; !bytes.Equal(got, payload) { t.Fatalf("forwarded = %x", got) }
	if bytes.Contains(relay.debugMetadata("desk_1"), payload) { t.Fatal("relay retained payload") }
}
```

- [x] **Step 2: Run and verify the relay tests fail**

Run: `go test ./internal/cloud -run 'TestDesktopRelay|TestDesktop.*WebSocket' -count=1`

Expected: FAIL because no production relay or WebSocket handlers exist.

- [x] **Step 3: Implement exact authentication and pairing**

Register `/ws/desktop/browser/{sessionID}` and `/ws/desktop/agent/{sessionID}`. Browser consumes only `hank_desktop_join` from the path-limited cookie and enforces same-origin. Agent consumes only bearer auth and requires `X-Hank-Agent-ID` to match the durable session. Both calls consume side/session/epoch-bound credentials before `websocket.Accept`; reused, swapped, expired, wrong-session, or wrong-epoch credentials return 401 without revealing session existence.

- [x] **Step 4: Implement bounded bidirectional forwarding**

Use one read/write pump per side, binary message enforcement, 4 MiB frame cap, token-bucket 50 MiB/s per direction, 30-second idle timer, eight-hour hard stop, and 32 active sessions. A limit, revoke, hard expiry, or terminal state closes both sides. A transport loss transitions active sessions to reconnecting but does not reuse credentials.

- [x] **Step 5: Prove restart/reconnect and content opacity**

Add `httptest.Server` tests for credential consumption races, one side joining first, duplicate side, text frame, frame/rate/idle limits, server revoke, 90-second reconnect, new epoch, and old-epoch rejection. Seed known plaintext inside an encrypted record and assert it is absent from store rows, audit rows, relay snapshots, and captured logs.

- [x] **Step 6: Run the relay checks**

Run: `go test ./internal/cloud -run 'TestDesktopRelay|TestDesktop.*WebSocket' -count=1`

Expected: PASS.

```bash
git add internal/cloud/desktop_relay.go internal/cloud/desktop_relay_test.go internal/cloud/desktop_websocket.go internal/cloud/desktop_websocket_test.go internal/cloud/server.go
git commit -m "feat(remote-desktop): relay opaque binary sessions"
```

### Task 4: Browser viewer shell, socket, and synthetic H.264 playback

**Files:**
- Create: `schemas/desktop/v1/synthetic-desktop-640x360.h264`
- Create: `schemas/desktop/v1/synthetic-desktop-640x360.json`
- Create: `web/dashboard/src/api/desktop.ts`
- Create: `web/dashboard/src/api/desktop.test.ts`
- Create: `web/dashboard/src/desktop/DesktopSocket.ts`
- Create: `web/dashboard/src/desktop/DesktopSocket.test.ts`
- Create: `web/dashboard/src/desktop/DesktopDecoder.ts`
- Create: `web/dashboard/src/desktop/DesktopDecoder.test.ts`
- Create: `web/dashboard/src/desktop/fmp4.ts`
- Create: `web/dashboard/src/desktop/fmp4.test.ts`
- Create: `web/dashboard/src/desktop/DesktopViewerPage.tsx`
- Create: `web/dashboard/src/desktop/DesktopViewerPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/AgentsPage.tsx`
- Modify: `web/dashboard/src/router.ts`
- Modify: `web/dashboard/src/App.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: browser crypto/record modules and Milestone 1 session API.
- Produces: dedicated viewer route, synthetic video playback, status/input/clipboard echo, reconnect, and termination UI.

- [x] **Step 1: Generate the deterministic synthetic clip**

Run:

```bash
ffmpeg -hide_banner -loglevel error -f lavfi -i testsrc2=size=640x360:rate=30 -t 2 -an -c:v libx264 -profile:v baseline -level 3.1 -pix_fmt yuv420p -g 30 -keyint_min 30 -sc_threshold 0 -bf 0 -f h264 schemas/desktop/v1/synthetic-desktop-640x360.h264
shasum -a 256 schemas/desktop/v1/synthetic-desktop-640x360.h264
```

Record codec `avc1.42c01f`, dimensions, frame rate, duration, access-unit offsets, keyframe indexes, and the observed SHA-256 in the adjacent JSON. Runtime tests assert the committed hash; runtime code never invokes FFmpeg.

- [x] **Step 2: Write failing API/socket/decoder/viewer tests**

Test session creation, secure-cookie response handling, binary socket URL construction without credentials, handshake completion, record dispatch, WebCodecs preference, AVCDecoderConfigurationRecord creation, Annex-B access-unit parsing, fragmented-MP4 init/media segments for MSE, unsupported-browser refusal before session creation, reconnect overlay, latency display, and End Session.

- [x] **Step 3: Implement API and socket state machine**

```ts
export type DesktopViewerState = "idle"|"authorizing"|"joining"|"active"|"reconnecting"|"ended"|"error";
export interface DesktopSocketCallbacks { onMessage(message: DesktopInnerMessage): void; onState(state: DesktopViewerState, reason?: string): void; }
export class DesktopSocket { start(session: DesktopSessionAuthorization): Promise<void>; reconnect(): Promise<void>; send(message: DesktopInnerMessage): Promise<void>; close(reason: string): Promise<void>; }
```

The socket performs the public signed handshake, activates `DesktopRecordLayer` only after both signatures verify, accepts binary records only, responds to ping, and requests fresh authorization/keys after disconnect.

- [x] **Step 4: Implement decoder abstraction and viewer route**

`DesktopDecoder` exposes `configure`, `decode`, `reset`, and `close`. Use `VideoDecoder.isConfigSupported` first, convert SPS/PPS into `description`/AVCDecoderConfigurationRecord, and convert access units into `EncodedVideoChunk`. The MSE adapter in `fmp4.ts` converts the same SPS/PPS and Annex-B access units into an initialization segment plus one bounded fragmented-MP4 media segment per generation/keyframe group; it never feeds raw Annex-B directly to `SourceBuffer`. The route `/dashboard/agents/{agentID}/desktop` is admin-only and reachable only for online agents advertising `desktop.session.open` and `desktop.view`. UI includes display canvas/video, connection/latency status, view/control toggle, clipboard buttons, quality indicator, synthetic badge, and immediate End Session.

- [x] **Step 5: Run accessibility and responsive tests**

At 390 CSS pixels, assert `scrollWidth === clientWidth`; every toolbar action is keyboard reachable; reconnect/permission/error states use live regions; fullscreen has a button fallback; synthetic state is visibly labeled.

- [x] **Step 6: Run the browser slice checks**

Run: `make frontend-test && make frontend-check && make frontend-build`

Expected: PASS.

```bash
git add schemas/desktop/v1/synthetic-desktop-640x360.h264 schemas/desktop/v1/synthetic-desktop-640x360.json web/dashboard/src/api/desktop.ts web/dashboard/src/api/desktop.test.ts web/dashboard/src/desktop web/dashboard/src/dashboard/AgentsPage.tsx web/dashboard/src/router.ts web/dashboard/src/App.tsx web/dashboard/src/styles.css
git commit -m "feat(remote-desktop): add synthetic browser viewer"
```

### Task 5: macOS synthetic coordinator and encrypted data plane

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/DesktopSessionCoordinator.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/DesktopRecordLayer.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/SyntheticDesktopHost.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/DesktopDataSocket.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/DesktopIndicator.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Fixtures/synthetic-desktop-640x360.h264`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Fixtures/synthetic-desktop-640x360.json`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/WorkerAgent.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKitSelftest/main.swift`

**Interfaces:**
- Consumes: Milestone 1 Swift contract/fixture types and committed H.264 fixture.
- Produces: `DesktopHost`, `DesktopSessionCoordinator`, `DesktopDataSocket`, and synthetic capability advertisement.

- [x] **Step 1: Add failing portable coordinator tests to the self-test target**

Test offer scope/signature validation, endpoint signed handshake, canonical key derivation, exact sequence enforcement, synthetic codec/access-unit emission, authorized input/clipboard echo, indicator-required activation, termination, 90-second reconnect, and prior-key disposal.

- [x] **Step 2: Define the portable host and coordinator contracts**

```swift
public protocol DesktopHost: Sendable {
    func start(_ authorization: DesktopHostAuthorization) async throws -> AsyncStream<DesktopHostEvent>
    func apply(_ command: DesktopHostCommand) async throws
    func stop(reason: String) async
}
public actor DesktopSessionCoordinator {
    public func offer(_ offer: DesktopSessionOffer) async throws
    public func reconnect(_ offer: DesktopSessionOffer) async throws
    public func terminate(reason: String) async
}
```

- [x] **Step 3: Implement cryptography, socket, and synthetic host**

Copy the two committed synthetic assets from HankServerside into Hankagent `Fixtures/` and assert their SHA-256 values match the canonical metadata before build. Use CryptoKit `P256.KeyAgreement`, `P256.Signing`, HKDF SHA-256, and `AES.GCM`. `SyntheticDesktopHost` replays the committed Annex-B access units at 30 fps, emits display `synthetic-1`, echoes bounded input and clipboard messages as statistics only, and never calls ScreenCaptureKit, CGEvent, or NSPasteboard.

- [x] **Step 4: Integrate control commands and guarded capabilities**

`WorkerAgent` routes `desktop.session.offer`, `activate`, and `close` to the coordinator and sends lifecycle events through the existing authenticated agent socket. Advertise `desktop.status`, `desktop.session.open`, `desktop.session.close`, `desktop.view`, `desktop.control`, and clipboard capabilities only when identity storage, fixture load, indicator, and coordinator initialization pass.

- [x] **Step 5: Run macOS synthetic checks**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Sources/HankAgentCore/DesktopSessionCoordinator.swift Sources/HankAgentCore/DesktopRecordLayer.swift Sources/HankAgentCore/SyntheticDesktopHost.swift Sources/HankAgentCore/DesktopDataSocket.swift Sources/HankAgentCore/DesktopIndicator.swift Sources/HankAgentCore/WorkerAgent.swift Sources/HankKitSelftest/main.swift Fixtures/synthetic-desktop-640x360.h264 Fixtures/synthetic-desktop-640x360.json
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): add mac synthetic sessions"
```

### Task 6: Windows synthetic coordinator and encrypted data plane

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopSessionCoordinator.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopRecordLayer.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/SyntheticDesktopHost.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopDataSocket.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.App/Services/DesktopIndicatorService.cs`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/WorkerAgent.cs`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/WorkerCommandDispatcher.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/DesktopSyntheticTests.cs`

**Interfaces:**
- Consumes: Milestone 1 .NET contracts and committed H.264 fixture.
- Produces: .NET equivalents of the Swift host/coordinator/socket interfaces and conditional capabilities.

- [x] **Step 1: Write failing xUnit coordinator and fixture tests**

Cover the same offer, handshake, fixture, sequence, echo, indicator, termination, reconnect, and zeroization cases as Task 5. Assert no `desktop.*` capability is present when any readiness dependency fails.

- [x] **Step 2: Implement the coordinator contracts**

```csharp
public interface IDesktopHost { IAsyncEnumerable<DesktopHostEvent> StartAsync(DesktopHostAuthorization authorization, CancellationToken cancellationToken); Task ApplyAsync(DesktopHostCommand command, CancellationToken cancellationToken); Task StopAsync(string reason); }
public sealed class DesktopSessionCoordinator { public Task OfferAsync(DesktopSessionOffer offer, CancellationToken cancellationToken); public Task ReconnectAsync(DesktopSessionOffer offer, CancellationToken cancellationToken); public Task TerminateAsync(string reason); }
```

- [x] **Step 3: Implement .NET crypto/socket/synthetic host**

Use `ECDiffieHellman`, `ECDsa`, `HKDF.DeriveKey`, `AesGcm`, and `ClientWebSocket`. Replay the committed clip at 30 fps; never call Graphics Capture, `SendInput`, or the Windows clipboard.

- [x] **Step 4: Integrate control routing and readiness advertisement**

Inject the coordinator into `WorkerAgent`, intercept `desktop.session.*` before `WorkerCommandDispatcher`, emit lifecycle events, and add capabilities only after all readiness checks pass. Existing files/shell behavior remains unchanged.

- [x] **Step 5: Run Windows synthetic checks**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add HankAgent-Windows/src/HankAgent.Worker HankAgent-Windows/src/HankAgent.App/Services/DesktopIndicatorService.cs HankAgent-Windows/tests/HankAgent.Tests/DesktopSyntheticTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): add windows synthetic sessions"
```

### Task 7: Cross-repository synthetic acceptance gate

**Files:**
- Create: `internal/cloud/desktop_e2e_test.go`
- Create: `docs/remote-desktop/synthetic-acceptance.md`
- Modify: `docs/api.md`
- Modify: `docs/security.md`

**Interfaces:**
- Consumes: all Milestone 2 tasks.
- Produces: repeatable end-to-end evidence and the Milestone 2 exit decision.

- [x] **Step 1: Add a process-level synthetic harness**

Start a test cloud, synthetic agent adapter, and browser-protocol client; bootstrap fixed test trust identities; create a session; join both sides; verify signed handshake; decrypt codec/video; send input/clipboard; exchange statistics; force disconnect/reconnect; and terminate from browser, server, and endpoint in separate subtests.

- [x] **Step 2: Add server-opacity and content-exclusion assertions**

Use known markers `synthetic-screen-secret`, `synthetic-keystroke-secret`, and `synthetic-clipboard-secret`. Assert none appears in SQL rows, audit metadata, logs, relay snapshots, HTTP responses, or control-plane bodies; only the browser and agent test clients recover them.

- [x] **Step 3: Run complete gates**

The complete local Go/build/frontend, Swift, and .NET gates pass. The PostgreSQL-backed exit row was run from an isolated copy of the Milestone 2 source against the Hank demo database network using Go 1.26; the desktop subset passed three consecutive runs and the full `internal/store` package passed without skips. This validation found and fixed PostgreSQL `TEXT[]` scanning, serializable-conflict mapping, and existing-schema migration idempotency before the gate was closed. The deployed demo application was not modified.

Run: `go test ./... && make build && make frontend-test && make frontend-check && make frontend-build`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj`

Expected: all PASS; PostgreSQL-backed tests require `HANK_REMOTE_TEST_DATABASE_URL` and skipped database tests do not satisfy the exit gate.

- [x] **Step 4: Record the acceptance matrix and documentation**

Document macOS synthetic, Windows synthetic, WebCodecs, MSE fallback, reconnect, all three termination actors, content opacity, and audit redaction with command output references.

```bash
git add internal/cloud/desktop_e2e_test.go docs/remote-desktop/synthetic-acceptance.md docs/api.md docs/security.md
git commit -m "test(remote-desktop): prove synthetic encrypted slice"
```

## Milestone 2 Exit Criteria

- [x] Browser, Swift, and .NET pass the same handshake, record, message, tamper, reconnect, and termination vectors.
- [x] Both synthetic endpoint implementations advertise the same guarded capabilities and produce the same H.264 fixture stream.
- [x] The browser decodes synthetic H.264 through WebCodecs and the supported MSE fallback.
- [x] Input and clipboard messages are encrypted, permission-checked, echoed only by the synthetic host, and absent from server state.
- [x] Reconnect uses fresh credentials, epoch, ephemeral keys, nonce prefixes, and sequence counters within 90 seconds.
- [x] Browser, server, and endpoint termination close both sockets and dispose keys.
- [x] Hank Server cannot decrypt or otherwise recover known video, input, or clipboard plaintext.
- [x] No physical capture, native input, privileged helper, installation, deployment, or push occurred.
