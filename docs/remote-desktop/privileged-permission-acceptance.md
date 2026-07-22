# Privileged and Permission Acceptance — Milestone 5

Date: 2026-07-22
Scope: portable privileged-boundary implementation and validation on the current uncommitted HankServerside and Hankagent worktrees.

## Result

Portable Milestone 5 remediation is complete. The Windows secure-desktop path now uses a dedicated signed LocalSystem bridge launched into the active WTS session, the browser synchronously clears and bars video at secure transitions, the Windows authoritative read/renewal pump cannot await ordinary event delivery, and macOS keeps the daemon barred until it authenticates a host-consumed final acknowledgement and `host_resumed`. Deterministic tests cover each repaired boundary.

Physical privileged acceptance is blocked and is not claimed. This milestone did not install a Windows service or macOS daemon, change privacy settings, trigger UAC/SAS, switch console users, crash installed helpers, or deploy uncommitted binaries.

## Portable Evidence

- IPC contract: canonical 16-byte `HDIP` v1 header, 1 MiB control and 4 MiB video bounds, 32-byte mutual challenges, five-minute renewable connection grants, session/epoch/permission grants, P-256 endpoint signatures, HKDF label `hank-desktop-v1/local-ipc`, HMAC-SHA-256 frames, and duplicate/skipped/tampered rejection in Swift and .NET.
- Windows authority: DPAPI LocalMachine credential encryption, SYSTEM/Administrators-only DACL, TPM-preferred non-exportable CNG P-256 identity, exactly one agent connection owned by the service, GUI direct-worker suppression when service authority is configured, a pipe-backed native host factory, closed host-command allowlist, streamed Files/shell proxy parity, and ordered fail-closed teardown/zeroization.
- Windows host boundary: WTS active-console resolution, user-token launch into `winsta0\\default`, pipe name plus one-time nonce as the only command arguments, SYSTEM/active-user pipe ACL, client/server PID inspection, exact session/path/hash/signer/thumbprint/SID checks, endpoint-signed mutual-challenge and session grants, exact-sequence HMAC-protected `HDIP` command/event frames, atomic connection renewal with a fresh same-session/epoch grant, one established-pipe reconnect/regrant, and fail-closed repeated failure. Renewal/control/reply traffic is isolated from a bounded drop-oldest ordinary event channel; secure-transition suppression drains stale ordinary frames without blocking renewal.
- Windows privileged desktop: the service duplicates its LocalSystem primary token, sets `TokenSessionId` to the active WTS session, and launches the signed bridge with `CreateProcessAsUser` on `winsta0\\Winlogon`. Its SYSTEM-only pipe receives only a one-time pipe name and nonce. Both peers validate the exact signed image, hash, signer, thumbprint, SYSTEM SID, and active session before endpoint-signed `HDIP` grants. The bridge attaches the process window station and thread desktop before creating capture/input resources, and accepts only capture events plus keyboard, pointer, held-input release, bounded control/quality/ping, and stop operations bound to the live session, desktop, grant, epoch, and generation. The service emits and drains `clear_video` at each secure boundary before accepting fresh bridge media. Direct Session 0 desktop access is no longer used. Secure attention remains gated and is never synthesized through ordinary input events.
- macOS authority: root-only data-protection Keychain access group, signed-daemon entitlement boundary, Secure Enclave-preferred non-exportable P-256 `SecKey`, one agent control plane in the daemon, injected non-exportable endpoint signing, no reusable credential in host grants, and epoch zeroization.
- macOS host boundary: peer validation, signed grants, bounded framing, and supervision are present. During renewal both sides install the new context while barred; the daemon sends its authenticated final acknowledgement but remains barred, the host validates it, unbars, and sends authenticated `host_resumed`, and only then does the daemon unbar. FIFO queues cover traffic in both directions. Final-ack loss, missing `host_resumed`, invalid resume, and timeout deactivate authentication and invalidate the XPC connection fail closed.
- macOS permission/session behavior: Screen Recording and Accessibility are emitted independently before native host startup and on live loss/restoration; local UI owns prompts/settings links. Lock/permission loss clears capture, input, and indicator. Same-user recovery restores or reactivates the indicator, waits for visibility, then starts a fresh stream generation; timeout/failure terminates.
- Browser/server: precise UAC, secure-attention, privacy-permission, lock, user-switch, helper, and indicator overlays block video/input as required. A `clear_video` secure transition synchronously resets WebCodecs/MSE, clears the canvas, and rejects video access units until a fresh secure-context codec configuration arrives. The browser explains local actions but exposes no remote Settings action. Privileged audit events accept only platform, state, permission name, reason code, session, epoch, and duration.

## Physical Matrix Pending Authorized Installation

| Behavior | macOS | Windows |
| --- | --- | --- |
| Wrong UID/session/binary/signature/IPC peer rejected | BLOCKED | BLOCKED |
| Signed authority and active-console host establish authenticated IPC | BLOCKED | BLOCKED |
| Standard physical-console capture/control | BLOCKED | BLOCKED |
| Elevated application and UAC secure-desktop transition | N/A | BLOCKED |
| Secure attention available with policy; explicit unavailable without it | N/A | BLOCKED |
| Screen Recording missing/granted/lost | BLOCKED | N/A |
| Accessibility missing/granted/lost | BLOCKED | N/A |
| Lock/loginwindow pause and same-user recovery with new generation | BLOCKED | N/A |
| Fast-user switch revokes prior host grant | BLOCKED | BLOCKED |
| One helper restart; repeat failure terminates | BLOCKED | BLOCKED |
| Indicator loss and authority stop terminate immediately | BLOCKED | BLOCKED |

Windows blocker: physical acceptance requires transferring this uncommitted worktree to the connected VM, producing a matching signed service/host build, installing the service and secure-attention policy, and running controlled UAC/session-switch/crash tests with a human observing the local console.

macOS blocker: the root daemon and signed host are not installed. Screen Recording was authorized by the user but still requires the local password-confirmed System Settings handoff. Revoking permissions, switching users, installing the daemon, or intentionally crashing the host require a controlled human-run acceptance window.

## Secret and Content Boundary Inspection

Static source/diff inspection and automated redaction tests found no route, audit field, process argument, host-grant field, or local-IPC metadata field that carries the reusable agent credential, endpoint private key, recovery secret, video/frame content, keyboard/pointer codes, clipboard text, pipe payload, XPC payload, raw audit token, window title, or process name.

Reusable credentials are loaded only in privileged authority code and are zeroized after worker construction or shutdown. Endpoint private keys remain non-exportable platform keys. Host bootstrap arguments contain only the bounded pipe name and one-time nonce. Host grants contain only connection/session identity, epoch material, effective permissions, display/quality configuration, and expiry. Browser/host content processing remains memory-only and server audit remains metadata-only.

Runtime log, crash-report, IPC-capture, SQL, process-memory, and installed-process-argument inspection remains part of the blocked physical matrices because no privileged components were installed or launched.

## Impact

- Security: introduces fail-closed local privilege separation, code/session-bound IPC, policy-gated Windows secure attention, separate macOS privacy readiness, and content-free privileged audit states. No reusable credential is intentionally exposed to a user host.
- Database/migrations: none. Existing desktop session events store the new bounded metadata-only event names and fields.
- Prohibited actions: no privacy screen, local-input blocking, service/daemon installation, privacy-setting change, UAC/SAS invocation, deployment, publish, tag, push, or commit occurred.

## Portable Gate Results

- Windows/.NET: 146 tests passed with no skips. The plain `net8.0` signed secure-bridge executable and all referenced Windows libraries build from macOS with zero warnings/errors. The full WinUI app target still requires Windows because its `XamlCompiler.exe` cannot execute on macOS.
- macOS/Swift: `swift build`, `desktop-daemon-selftest`, and `hankkit-selftest` passed, including deterministic symmetric renewal, both-direction FIFO traffic, in-flight drain, final-ack loss, missing-resume timeout, rollback, peer, permission, session, provider, and indicator coverage.
- Server/browser: `go test ./...`, `make build`, 58 frontend files/324 tests, `make frontend-check`, and `make frontend-build` passed. The Vite chunk-size notice and jsdom canvas/navigation/media notices were non-failing.
- Diff hygiene: `git diff --check` passed in both repositories.
