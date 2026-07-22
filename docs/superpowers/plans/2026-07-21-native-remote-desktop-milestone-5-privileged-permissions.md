# Hank Native Remote Desktop Milestone 5 Privileged and Permission Behavior Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Remote Desktop authority into privileged Windows/macOS service components, authenticate their user-session hosts, support Windows elevated/UAC secure-desktop control, and provide fail-closed macOS Screen Recording/Accessibility/lock/session behavior with guided setup.

**Architecture:** A privileged Windows service and macOS launch daemon own the reusable Hank credential, endpoint identity, authorization validation, data-plane socket, epoch keys, and helper supervision. Each launches or connects to a signed host in the current physical console session through bounded authenticated local IPC; the host owns only capture, encode, normal input, clipboard, permissions, and the visible indicator. Windows adds a narrowly scoped privileged bridge for elevated and secure-desktop operations, while macOS pauses at lock/loginwindow and guides the user to grant Screen Recording and Accessibility to the signed user host.

**Tech Stack:** .NET 8 Worker Service/Windows Service, DPAPI/CNG/TPM, Win32 WTS/process/token/desktop/named-pipe APIs, Windows Graphics Capture/Desktop Duplication/SendInput; Swift 6 launch daemon and app host, Security/Keychain/Secure Enclave, NSXPCConnection, audit tokens/code-sign validation, SMAppService, ScreenCaptureKit, Accessibility; existing React viewer lifecycle states and Go audit/control plane.

## Global Constraints

- Begin only after Milestone 4 physical native-control acceptance passes on both platforms.
- Privileged services/daemons own reusable Hank credentials, endpoint private keys, session authorization, data-plane sockets, epoch keys, and termination authority.
- User-session hosts receive only session ID, current epoch material, effective permissions, display/quality configuration, and bounded commands; they never receive the reusable Hank agent token.
- Local IPC is authenticated, versioned, length-bounded, session/epoch-bound, replay-resistant, and fail-closed on peer identity, signature, code-sign, audit-token, active-session, or framing failure.
- The visible indicator remains in the physical console host and must acknowledge visibility before capture/input begins.
- Windows elevated/UAC support may run only from the privileged service/bridge and must stop immediately when the active session or desktop changes unexpectedly.
- Windows secure-attention support is enabled only when the signed service is installed with the required Windows policy; absence reports `secure_attention_unavailable` and never simulates Ctrl+Alt+Delete with ordinary input.
- macOS never controls loginwindow or bypasses privacy permissions; lock/loginwindow pauses capture and blocks input with stale-frame clearing.
- macOS Screen Recording and Accessibility are checked separately, explained separately, and opened only through explicit local user actions.
- Helper crash receives one supervised restart; repeated failure terminates. Indicator loss has the existing ten-second fail-closed window.
- No privacy screen, local-input blocking, deployment to user devices, publishing, tagging, or push occurs in this milestone.

---

### Task 1: Versioned authenticated local IPC contract

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Fixtures/desktop-ipc-v1-test-vectors.json`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKit/DesktopIPC.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKitSelftest/main.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Contracts/DesktopIPC.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/DesktopIPCContractTests.cs`

**Interfaces:**
- Consumes: desktop session ID, key epoch, permissions, host commands/events, and endpoint identity from earlier milestones.
- Produces: exact IPC frame, mutual challenge, authenticated host-connection grant, short-lived desktop-session grant, sequence, bounds, and error contracts shared by Swift/.NET.

- [x] **Step 1: Write failing cross-language IPC vector tests**

Test 16-byte header encoding, 1 MiB control/4 MiB video limits, service and host 32-byte challenges, endpoint signature, host process identity binding, renewable connection grant, session/epoch grant, exact sequence, duplicate/skipped frame, wrong peer, stale active session, and invalid signature.

- [x] **Step 2: Define the exact frame and grant**

```text
header = magic[4] "HDIP" | version u16=1 | type u16 | flags u16 | reserved u16=0 | payload_length u32
payload = canonical JSON for control or raw bytes for video
```

```swift
public struct DesktopHostConnectionGrant: Codable, Sendable { public let connectionID: String; public let consoleSessionID: String; public let hostProcessIdentity: String; public let issuedAt: Date; public let expiresAt: Date; public let serviceChallenge: Data; public let hostChallenge: Data; public let endpointSignature: Data }
public struct DesktopSessionGrant: Codable, Sendable { public let connectionID: String; public let sessionID: String; public let keyEpoch: UInt32; public let permissions: [String]; public let issuedAt: Date; public let expiresAt: Date; public let endpointSignature: Data }
```

- [x] **Step 3: Implement bounded codecs and mutual proof**

Both languages length-prefix frames, reject reserved bits/non-canonical version, enforce exact next sequence in each direction, sign SHA-256 of both challenges plus connection-grant fields with the endpoint key, and derive an IPC MAC key with HKDF label `hank-desktop-v1/local-ipc`. Authenticate every post-connection frame with HMAC-SHA-256 over header, sequence, and payload. A connection grant lasts five minutes and renews only after peer/console-session revalidation; every desktop start/reconnect separately carries a signed `DesktopSessionGrant` bound to that authenticated connection, session, epoch, permissions, and the earlier of join/reconnect/hard expiry.

- [x] **Step 4: Generate and consume canonical vectors**

Generate vectors from fixed test keys/challenges only when `UPDATE_DESKTOP_FIXTURES=1`; normal Swift/.NET tests read and compare. Include positive and tamper cases.

- [x] **Step 5: Run IPC contracts (commit intentionally skipped; not authorized)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter DesktopIPCContractTests`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Fixtures/desktop-ipc-v1-test-vectors.json Sources/HankKit/DesktopIPC.swift Sources/HankKitSelftest/main.swift HankAgent-Windows/src/HankAgent.Contracts/DesktopIPC.cs HankAgent-Windows/tests/HankAgent.Tests/DesktopIPCContractTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): define authenticated host ipc"
```

### Task 2: Windows privileged service authority and credential storage

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/HankAgent.DesktopService.csproj`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/Program.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/DesktopServiceWorker.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/MachineCredentialStore.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/EndpointIdentityStore.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/DesktopControlPlane.cs`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/HankAgent.sln`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/DesktopServiceSecurityTests.cs`

**Interfaces:**
- Consumes: existing `WorkerAgent` control-plane primitives and desktop coordinator contracts.
- Produces: LocalSystem Windows service owning agent token, endpoint identity, control/data plane, and helper lifecycle.

- [x] **Step 1: Write failing credential/authority tests**

Test machine-only token access, non-exportable CNG endpoint key, TPM preference and DPAPI LocalMachine fallback, ACL restricted to SYSTEM/Administrators, no secrets in logs/errors, offer scope verification, service stop zeroization, and GUI process denied direct credential access.

- [x] **Step 2: Implement service project and stores**

Use `Microsoft.Extensions.Hosting.WindowsServices`. Store the agent token as DPAPI LocalMachine ciphertext under `%ProgramData%\Hank\Agent\agent-token.bin` with SYSTEM/Administrators ACL. Create endpoint ECDSA P-256 through CNG with Microsoft Platform Crypto Provider when available; fallback to a non-exportable machine CNG key protected by DPAPI metadata.

- [x] **Step 3: Move the single agent control plane into the service**

The service becomes the only process that connects `/ws/agent` for this agent ID. It owns the complete `WorkerAgent` control plane, `DesktopSessionCoordinator`, `DesktopDataSocket`, endpoint key, and lifecycle events. Machine-safe host actions run in the service; existing user-scoped Files and shell commands are proxied over the authenticated host IPC through a closed command allowlist. The GUI stops its direct worker connection whenever the service is configured. Router and integration tests must prove exactly one connection, no connection eviction loop, and unchanged Files/shell behavior when the interactive host is ready.

- [x] **Step 4: Add service shutdown/failure behavior**

On service stop, token/key-store failure, agent disconnect outside reconnect, or endpoint identity mismatch: close host IPC, close data plane, terminate durable session, hide indicator through host, and clear epoch material.

- [x] **Step 5: Run Windows service authority tests (commit intentionally skipped; not authorized)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter DesktopServiceSecurityTests`

Expected: PASS without installing the service.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add HankAgent-Windows/src/HankAgent.DesktopService HankAgent-Windows/HankAgent.sln HankAgent-Windows/tests/HankAgent.Tests/DesktopServiceSecurityTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): add windows service authority"
```

### Task 3: Windows active-session host and authenticated named-pipe IPC

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopHost/HankAgent.DesktopHost.csproj`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopHost/Program.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopHost/DesktopHostRuntime.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/ActiveSessionHostSupervisor.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/DesktopNamedPipeServer.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopHost/DesktopNamedPipeClient.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/WindowsDesktopHostIPCTests.cs`

**Interfaces:**
- Consumes: Task 1 IPC and Task 2 service authority.
- Produces: signed interactive host launched into the active console with mutually authenticated named-pipe transport.

- [x] **Step 1: Write failing peer/session/supervision tests**

Cover pipe ACL, client PID discovery, executable path/hash/signature, expected signer subject/thumbprint configuration, active WTS session, challenge signature, wrong session, wrong executable, relaunch on console-user switch, one crash restart, repeated failure termination, and pipe disconnect.

- [x] **Step 2: Implement active-session launch**

Resolve active console with WTS, acquire user token with `WTSQueryUserToken`, create an environment block, and launch `HankAgent.DesktopHost.exe` with `CreateProcessAsUser` into `winsta0\default`. Pass only pipe name and one-time bootstrap nonce through command arguments; never pass credentials, keys, permissions, or session IDs on the command line.

- [x] **Step 3: Implement named-pipe peer validation and IPC proof**

Create a per-session pipe with SYSTEM plus active-user SID ACL. Server obtains client PID, verifies session ID, canonical executable path, Authenticode signer, and expected file hash from installed manifest before completing Task 1 mutual proof. Host validates server PID belongs to the configured signed service image running as SYSTEM.

- [x] **Step 4: Connect native host providers and indicator**

The host owns Windows capture/encoder/input/clipboard/indicator providers plus the existing user-scoped Files/shell adapters. It starts only after a valid grant, rejects commands for another session/epoch, and stops all providers before closing IPC. Non-desktop proxy frames use distinct allowlisted IPC types, preserve existing file-policy/shell-enable checks, and never expose the service's Hank credential or endpoint key.

- [x] **Step 5: Run Windows host IPC tests (commit intentionally skipped; not authorized)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter WindowsDesktopHostIPCTests`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add HankAgent-Windows/src/HankAgent.DesktopHost HankAgent-Windows/src/HankAgent.DesktopService HankAgent-Windows/tests/HankAgent.Tests/WindowsDesktopHostIPCTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): authenticate windows desktop host"
```

### Task 4: Windows elevated applications, secure desktop, and secure attention

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/PrivilegedDesktopBridge.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/SecureDesktopMonitor.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.DesktopService/SecureAttentionProvider.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/WindowsPrivilegedDesktopTests.cs`

**Interfaces:**
- Consumes: effective `desktop.elevate`/`desktop.secure_desktop`, active-session supervisor, and native providers.
- Produces: explicit default/elevated/secure-desktop provider transitions and policy-gated Ctrl+Alt+Delete.

- [x] **Step 1: Write failing transition and permission tests**

Test normal-to-UAC transition, secure desktop detection, required permissions, stale-frame clearing, privileged input denial without grant, unexpected desktop switch, return to default desktop, active-session change, unavailable secure capture, and secure-attention policy absent/present.

- [x] **Step 2: Implement secure desktop monitor**

Use `OpenInputDesktop`, `GetUserObjectInformation`, WTS session notifications, and desktop-name comparison. Treat `Default`, `Winlogon`, and unknown desktops as separate provider generations. Unknown or inaccessible desktop pauses capture/input and emits `secure_desktop_unavailable`.

- [x] **Step 3: Implement privileged capture/input bridge**

Run only inside the LocalSystem service/approved privileged process. Bind every operation to active session, desktop name, session ID, key epoch, permission set, and current generation. Never allow the ordinary host pipe to request arbitrary process launch or Win32 calls.

- [x] **Step 4: Implement policy-gated secure attention**

Call the supported Windows secure-attention mechanism only when service identity, installation policy, and `desktop.secure_desktop` are confirmed. Otherwise return `secure_attention_unavailable`; never emulate Ctrl+Alt+Delete via three `SendInput` events.

- [x] **Step 5: Run privileged Windows behavior tests (commit intentionally skipped; not authorized)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter WindowsPrivilegedDesktopTests`

Expected: PASS; physical UAC acceptance runs in Task 9.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add HankAgent-Windows/src/HankAgent.DesktopService HankAgent-Windows/tests/HankAgent.Tests/WindowsPrivilegedDesktopTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): bridge windows secure desktop"
```

### Task 5: macOS privileged daemon authority and endpoint identity

**Files:**
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Package.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankDesktopDaemon/main.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankDesktopDaemon/DesktopDaemon.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankDesktopDaemon/DaemonCredentialStore.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankDesktopDaemon/DaemonEndpointIdentity.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankDesktopDaemon/DesktopControlPlane.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Tests/DesktopDaemonSelftest/main.swift`

**Interfaces:**
- Consumes: existing Swift coordinator/control plane and Task 1 IPC.
- Produces: root-owned daemon authority with Keychain/Secure Enclave endpoint identity.

- [x] **Step 1: Write failing daemon authority tests**

Test root-only agent token access, Keychain access control, Secure Enclave preference and Keychain P-256 fallback, non-exportable identity, control-plane scope, no secrets in logs, daemon stop zeroization, and GUI denied reusable token access.

- [x] **Step 2: Add daemon executable and root-owned stores**

Add SwiftPM executable `HankDesktopDaemon`. Store the agent credential in a root-owned Keychain item with daemon designated-requirement access. Create endpoint signing key in Secure Enclave when supported and a non-exportable Keychain P-256 key otherwise.

- [x] **Step 3: Move the single agent control plane to the daemon**

Daemon becomes the only process that connects `/ws/agent` for this agent ID and owns the complete control plane, desktop coordinator/data plane, endpoint key, and lifecycle events. Existing user-scoped Files/shell adapters execute in the signed console host through an allowlisted XPC proxy that preserves file-policy and shell-enable checks. The GUI stops its direct worker connection whenever daemon authority is configured. Tests prove one connection, no eviction loop, no reusable credential over XPC, and unchanged Files/shell behavior when the host is ready.

- [x] **Step 4: Run daemon authority tests (commit intentionally skipped; not authorized)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run desktop-daemon-selftest`

Expected: PASS without installing the daemon.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Package.swift Sources/HankDesktopDaemon Tests/DesktopDaemonSelftest
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): add mac daemon authority"
```

### Task 6: macOS authenticated XPC host and supervision

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankDesktopDaemon/DesktopXPCListener.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankDesktopDaemon/MacHostSupervisor.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgent/DesktopXPCClient.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKit/DesktopXPCProtocol.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgent/AppDelegate.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Tests/DesktopDaemonSelftest/main.swift`

**Interfaces:**
- Consumes: Task 1 IPC frames/grants and Task 5 daemon.
- Produces: code-sign/audit-token authenticated XPC between daemon and current console host.

- [x] **Step 1: Write failing XPC peer/supervision tests**

Cover audit token UID/PID, console UID, bundle ID, Team ID/designated requirement, signed executable path, wrong/replayed challenge, console-user switch, host crash/restart, repeated failure, disconnect, and daemon impersonation.

- [x] **Step 2: Implement XPC peer validation**

Daemon listener extracts connection audit token, verifies effective UID equals physical console UID, validates code signature/designated requirement through Security APIs, and completes mutual Task 1 proof. App client validates daemon code requirement and root ownership.

- [x] **Step 3: Connect host providers and lifecycle**

The signed HankAgent app process owns ScreenCaptureKit, VideoToolbox, CGEvent, NSPasteboard, indicator, and existing user-scoped Files/shell adapters. Console-user change revokes the grant, stops providers, and requires a newly authenticated host connection. XPC rejects command families outside the explicit desktop/files/shell allowlist.

- [x] **Step 4: Implement bounded supervision**

Daemon requests launch through supported login-item/app activation mechanisms, allows one restart per session, and terminates on repeated failure. No arbitrary executable path or arguments cross XPC.

- [x] **Step 5: Run XPC host tests (commit intentionally skipped; not authorized)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run desktop-daemon-selftest && swift run hankkit-selftest`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Sources/HankDesktopDaemon Sources/HankAgent/DesktopXPCClient.swift Sources/HankAgent/AppDelegate.swift Sources/HankKit/DesktopXPCProtocol.swift Tests/DesktopDaemonSelftest/main.swift
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): authenticate mac desktop host"
```

### Task 7: macOS permission readiness, guided setup, and lock behavior

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgent/DesktopPermissionController.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgent/DesktopPermissionView.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/MacSessionStateMonitor.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgent/Modules/SettingsModule.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKitSelftest/main.swift`

**Interfaces:**
- Consumes: Screen Recording/Accessibility status and console session notifications.
- Produces: precise readiness states, explicit local setup actions, lock/loginwindow pause, and recovery without stale frames.

- [x] **Step 1: Write failing permission/lock transition tests**

Test screen-recording missing, accessibility missing, both missing, local Open Settings actions, denial retained, grant detection, permission loss during active session, screen lock, fast-user switch, unlock same user, new console user, and no stale frame/input.

- [x] **Step 2: Implement separate permission checks and actions**

Use `CGPreflightScreenCaptureAccess`/`CGRequestScreenCaptureAccess` and `AXIsProcessTrustedWithOptions`. Show why each permission is needed, current status, and an explicit Open System Settings action using the supported privacy pane URL. Never trigger prompts from a remote request without local UI interaction.

- [x] **Step 3: Implement lock/session monitoring**

Observe workspace session resign/activate, screen sleep/wake, and `CGSessionCopyCurrentDictionary`. Lock/loginwindow clears the browser frame, pauses capture, blocks input, and emits `console_locked`; same-user unlock revalidates permissions and creates a new stream generation. User switch revokes XPC grant and terminates unless a new eligible host is established within reconnect window.

- [x] **Step 4: Run permission UX tests (commit intentionally skipped; not authorized)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Sources/HankAgent/DesktopPermissionController.swift Sources/HankAgent/DesktopPermissionView.swift Sources/HankAgent/Modules/SettingsModule.swift Sources/HankAgentCore/Desktop/MacSessionStateMonitor.swift Sources/HankKitSelftest/main.swift
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): guide mac permissions safely"
```

### Task 8: Browser privileged/readiness states and server audit

**Files:**
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.tsx`
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.test.tsx`
- Create: `web/dashboard/src/desktop/readiness.ts`
- Create: `web/dashboard/src/desktop/readiness.test.ts`
- Modify: `internal/cloud/desktop_control.go`
- Modify: `internal/cloud/desktop_control_test.go`

**Interfaces:**
- Consumes: permission, secure-state, helper, console, and service lifecycle events.
- Produces: precise viewer overlays/actions and metadata-only privileged/security audit.

- [x] **Step 1: Write failing state-mapping and redaction tests**

Test Windows UAC entered/exited/unavailable, secure attention unavailable, macOS Screen Recording/Accessibility required, console locked, user switched, helper restarting/failed, indicator lost, and permission restored. Audit tests forbid window titles, process names, frame/input/clipboard content, pipe/XPC payloads, and raw peer identity tokens.

- [x] **Step 2: Implement stable readiness mapping**

```ts
export type DesktopReadinessOverlay = { tone:"info"|"warning"|"error"; title:string; detail:string; blocksVideo:boolean; blocksInput:boolean; localAction?:"open_screen_recording"|"open_accessibility" };
export function readinessOverlay(state: DesktopPermissionState): DesktopReadinessOverlay | null;
```

Remote browser can explain a required local action but cannot open settings on the remote Mac; local action events appear only in Hankagent UI.

- [x] **Step 3: Audit privileged transitions without content**

Add `desktop.secure_desktop.entered/exited/unavailable`, `desktop.permission.required/granted/lost`, `desktop.console.locked/switched`, `desktop.helper.restarted/failed`, and `desktop.indicator.lost/restored`. Metadata contains platform, state, permission name, reason, session, epoch, and duration only.

- [x] **Step 4: Run browser/server readiness tests (commit intentionally skipped; not authorized)**

Run: `go test ./internal/cloud -run 'Desktop.*Permission|Desktop.*Secure|Desktop.*Helper' -count=1 && make frontend-test && make frontend-check && make frontend-build`

Expected: PASS.

```bash
git add internal/cloud/desktop_control.go internal/cloud/desktop_control_test.go web/dashboard/src/desktop/readiness.ts web/dashboard/src/desktop/readiness.test.ts web/dashboard/src/desktop/DesktopViewerPage.tsx web/dashboard/src/desktop/DesktopViewerPage.test.tsx
git commit -m "feat(remote-desktop): surface privileged readiness"
```

### Task 9: Privileged and permission acceptance gate

**Files:**
- Create: `docs/remote-desktop/privileged-permission-acceptance.md`
- Modify: `docs/security.md`

**Interfaces:**
- Consumes: all Milestone 5 tasks.
- Produces: physical-device evidence for privileged IPC, Windows UAC, macOS permissions/lock, and fail-closed supervision.

- [x] **Step 1: Run all portable security gates**

Run: `go test ./... && make build && make frontend-test && make frontend-check && make frontend-build`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest && swift run desktop-daemon-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj`

Expected: PASS.

- [ ] **Step 2: Run Windows physical privileged matrix**

In a controlled Windows VM/physical console, run service and signed host; prove wrong user/session/binary/pipe client rejection, standard desktop, elevated app control, UAC secure-desktop transition, stale-frame clearing, secure attention when policy permits, explicit unavailable state when it does not, fast-user switch, host crash/restart, repeated failure termination, indicator failure, and immediate service-stop termination.

- [ ] **Step 3: Run macOS physical permission/session matrix**

Run daemon and signed host; prove wrong UID/binary/signature/XPC rejection, each permission missing/granted/lost, guided local settings actions, lock/loginwindow pause, same-user unlock recovery with new generation, console-user switch, host crash/restart, repeated failure, indicator loss, and daemon-stop termination.

- [x] **Step 4: Inspect portable secret/content boundaries (installed-runtime inspection remains in Steps 2–3)**

Search logs, crash reports, IPC captures, SQL, audit, process arguments, environment, and user-host memory diagnostics for the agent token, endpoint private key, epoch keys, recovery secret, frame markers, input codes, and clipboard markers. Only the privileged authority may access reusable credentials/endpoint key; content must not appear outside browser/host processing.

- [x] **Step 5: Complete acceptance and security documentation (commit intentionally skipped; not authorized)**

```bash
git add docs/remote-desktop/privileged-permission-acceptance.md docs/security.md
git commit -m "test(remote-desktop): prove privileged session boundary"
```

## Milestone 5 Exit Criteria

- [x] Windows service and macOS daemon exclusively own reusable credentials, endpoint keys, session authorization, data-plane sockets, and epoch keys.
- [x] Signed active-console hosts authenticate through bounded session/epoch IPC and receive only session-scoped material.
- [x] Wrong peer, session, console user, signature, code identity, challenge, frame, or sequence fails closed in portable tests; installed wrong-peer checks remain in the physical matrices.
- [ ] Windows controls elevated applications and supported UAC secure-desktop flows; secure attention is policy-gated and never simulated unsafely.
- [x] macOS separately guides Screen Recording and Accessibility, pauses at lock/loginwindow, and recovers safely without stale frames.
- [x] Helper/indicator crash behavior performs one bounded restart and terminates on repeated failure.
- [ ] Portable and physical security matrices pass with content/secret exclusion evidence.
- [x] No privacy screen, local-input blocking, deployment, publish, tag, or push occurred.
