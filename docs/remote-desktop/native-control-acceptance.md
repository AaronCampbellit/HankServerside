# Native Control Acceptance — Milestone 4

Date: 2026-07-22
Scope: portable native-control implementation and validation on the current uncommitted HankServerside and Hankagent worktrees.

## Result

Portable acceptance passes after independent review remediation. Browser control remains inert until the endpoint acknowledges the current focus lease; endpoint authorization fails closed on session, epoch, permission, control, focus, display generation, clipboard bound, allowlist, and live platform readiness; Windows and macOS provider translations pass fake-sink/provider tests. Audit events retain only bounded outcome metadata and never input or clipboard content.

Milestone 4 is not physically accepted. The portable result proves contract, authorization, translation, failure-continuity, and rendered browser behavior, but not real input injection on both target consoles.

Physical Windows/macOS control acceptance is blocked and is not claimed:

- macOS native viewing cannot reach the control matrix until a human grants Screen Recording in System Settings. This milestone did not change macOS privacy settings.
- The connected Windows VM does not have a saved Codex remote project/worktree containing these uncommitted Hankagent changes. No commit, push, package transfer, installation, or deployment was authorized for this milestone, so the Windows binary was not installed or exercised on that VM.

Required human follow-up: grant Screen Recording to the local HankAgent build on macOS, and make this exact Hankagent worktree available to the connected Windows VM (or authorize a commit/package deployment). Then run the matrix below without enabling service mode, secure desktop, input blocking, or privacy-screen behavior.

## Portable Evidence

Independent review blockers were resolved as follows:

- Display selection is a view operation with current-generation validation, so browser focus teardown cannot make monitor switching impossible.
- Browser-to-agent clipboard writes now require control permission, enabled control, and current focus in both browser UI and endpoint authorization; stale/view-only writes fail closed.
- Clipboard records have a type-specific bounded JSON allowance for the 1 MiB UTF-8 text contract, including worst-case JSON escaping, while generic control remains limited to 256 KiB. The opaque relay frame bound was raised to match.
- Coordinator authorization queries the active host's live control readiness. macOS uses current Accessibility trust and denies control enable with `platform_not_ready` when unavailable.
- Clipboard provider failures produce a negative acknowledgement without terminating the desktop session.
- Fit-mode pointer mapping uses the actual rendered display rectangle, excluding letterbox bars.
- Remote clipboard read is two-step: the endpoint response becomes memory-only “ready to copy” state, and a second explicit click invokes the browser clipboard write within user activation.
- Windows XBUTTON1/XBUTTON2 down and release events carry the required mouse data.
- Wheel deltas are finite and bounded to ±1,000,000 in the shared Go, TypeScript, Swift, and .NET contracts. Both native providers reject larger values before integer conversion, and the canonical fixtures exercise the accepted boundary and rejected overflow.
- Browser teardown releases held buttons at the last normalized pointer location, so blur and control-disable cannot jump the remote cursor to the display origin.
- Browser encrypted sends use one connection-scoped FIFO. Blur release bursts are encrypted and written in strict call order with unique monotonic sequences; the first encryption or wire failure poisons that queue, and close/reconnect prevents stale work from reaching either socket.
- Fire-and-forget input, clipboard, display, quality, and special-key sends consume rejections and move the viewer to an error state without an unhandled promise rejection. Unmount suppresses late state changes.
- Incoming encrypted WebSocket records use a separate connection-scoped FIFO, preserving receive sequence while allowing Pong replies to use the independent outbound queue without deadlock.
- Every inbound job remains bound to its captured socket, generation, record layer, and queue after asynchronous decryption. Delayed clipboard, terminate, and ping records from a closed or replaced connection are dropped before decode/callback, and stale ping cannot enqueue Pong into the new epoch.

HankServerside:

- `go test ./...` — pass.
- `make build` — pass.
- `make frontend-test` — 57 files and 311 tests pass.
- `make frontend-check` — 311/311 tests plus TypeScript/Vite production build pass.
- `make frontend-build` — pass; Vite reports only the existing large-chunk advisory.
- Focused native-control tests — 23 browser tests and focused protocol/cloud relay/audit tests pass.

Hankagent:

- `swift build && swift run hankkit-selftest` — pass, including authorization, canonical contract, macOS input, clipboard, special-key, and held-state cases.
- `.NET test tests/HankAgent.Tests/HankAgent.Tests.csproj` — 111/111 pass.
- Windows-target `HankAgent.Windows.csproj` build for `net8.0-windows10.0.19041.0` — pass with zero warnings and errors.
- Focused Windows authorization/data-plane/provider tests — 24/24 pass.

## Physical Matrix Pending Human Access

| Behavior | macOS | Windows |
| --- | --- | --- |
| View-only rejects pointer, keyboard, and clipboard writes | BLOCKED | BLOCKED |
| Normal move, click, wheel, physical keys, and modifiers | BLOCKED | BLOCKED |
| Local pointer and keyboard coexist with remote input | BLOCKED | BLOCKED |
| Blur/reconnect releases held state and requires a new focus lease | BLOCKED | BLOCKED |
| Clipboard read/write permissions remain independent | BLOCKED | BLOCKED |
| Monitor switch advances generation and geometry | BLOCKED | BLOCKED |
| Platform normal special keys work | BLOCKED | BLOCKED |
| Privileged special path reports `privileged_control_unavailable` | BLOCKED | BLOCKED |

Portable fake-provider coverage exists for every row, but it is not a substitute for the required physical console proof.

## Safety Impact

- Security: the encrypted endpoint acknowledgement is authoritative; stale or unauthorized input is denied before native providers. Browser encryption and wire writes are strictly serialized per connection, so concurrent UI events cannot reuse an AES-GCM nonce; a failed queue cannot advance later records, and reconnect cannot emit stale encrypted work. Receive decryption is independently ordered and revalidates its captured socket, generation, record layer, and queue after every asynchronous decrypt and before Pong, preventing stale clipboard/control/terminate callbacks or cross-epoch replies. Wheel values are finite and bounded before native integer conversion, and synthesized held-button releases preserve the last valid pointer location while still failing closed. Remote clipboard writes require current control/focus, provider failure is non-fatal, and remote reads require a second explicit copy gesture. Clipboard remains text-only, independently permissioned, limited to 1 MiB, memory-only, and content-free in audit/log data. Live macOS Accessibility readiness is enforced before control enable. Special keys use a closed allowlist; secure attention remains unavailable.
- Database/migrations: none. Audit uses the existing desktop session event store and adds no schema or migration.
- Prohibited actions: no service/daemon installation, privacy setting change, secure-desktop control, local-input blocking, privacy screen, deployment, commit, push, or live-state mutation occurred.
