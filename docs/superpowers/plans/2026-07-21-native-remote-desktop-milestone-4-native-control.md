# Hank Native Remote Desktop Milestone 4 Native Control Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add permission-bounded native pointer, physical keyboard, text clipboard, monitor selection, view-only/control switching, and named special-key paths to the proven native viewing session on Windows and macOS.

**Architecture:** The browser normalizes pointer coordinates against the currently displayed physical monitor, emits physical keyboard codes with explicit modifier state, and performs clipboard access only from explicit user gestures. The encrypted record layer carries input directly to `DesktopSessionCoordinator`, which rechecks current epoch, granted permission, control mode, focus lease, display generation, and platform readiness before calling native providers. Platform providers inject normal interactive input and text clipboard operations; privileged/UAC/lock-screen paths remain unavailable until Milestone 5.

**Tech Stack:** React/TypeScript DOM KeyboardEvent/PointerEvent/Clipboard APIs; Swift 6 CoreGraphics `CGEvent` and AppKit `NSPasteboard`; .NET 8 Win32 `SendInput`, keyboard scan codes, pointer APIs, and Windows clipboard; existing encrypted inner protocol and audit events.

**Execution status (2026-07-22):** All portable implementation and validation steps, including independent review remediation, are complete. Commit steps were not performed because no commit authorization was given. Milestone 4 is not physically accepted: the Windows/macOS matrix remains blocked on human access described in `docs/remote-desktop/native-control-acceptance.md`; no privacy setting, installation, deployment, push, or live device state was changed.

## Global Constraints

- Begin only after Milestone 3 native viewing acceptance passes on physical Windows and macOS consoles.
- Local pointer and keyboard input remain enabled and coexist with remote input.
- View-only mode must prevent every native input and clipboard write even if stale encrypted messages arrive.
- Input requires the current session, current key epoch, active state, granted permission, enabled control mode, viewer focus lease, current display ID/generation, and platform readiness.
- Pointer coordinates are normalized to the selected physical display and clamped at the endpoint against current geometry.
- Keyboard messages represent physical keys, scan/code values, location, repeat, and explicit modifiers; text insertion never substitutes for physical key events.
- Clipboard is text-only, bounded to 1 MiB, independently permissioned by direction, user-gesture initiated in the browser, memory-only, and absent from logs/audit/server persistence.
- Clipboard failure is non-fatal and cannot expand view/control permissions.
- Special keys use a closed named allowlist; privileged secure-attention/UAC paths return `privileged_control_unavailable` until Milestone 5.
- Monitor switching cannot reuse coordinates or frames from a prior display generation.
- No service/daemon, secure-desktop control, local-input blocking, privacy screen, file transfer, installation, deployment, or push occurs in this milestone.

## Independent Review Remediation

- [x] Permit current-generation display selection after browser focus teardown without granting input authority.
- [x] Require current control permission/mode/focus for browser-to-agent clipboard writes in UI and both endpoint authorizers.
- [x] Carry the 1 MiB clipboard contract through bounded type-specific browser/Go/Swift/.NET/relay framing while retaining the 256 KiB generic control limit.
- [x] Use live host control readiness, including macOS Accessibility trust, for control-enable authorization.
- [x] Keep clipboard provider failure non-fatal and acknowledge failure without ending the session.
- [x] Normalize fit-mode pointer input against the rendered display rectangle rather than letterbox bars.
- [x] Require a second explicit user gesture before writing asynchronously received remote text to the browser clipboard.
- [x] Encode Windows XBUTTON1/XBUTTON2 mouse data on down/up and held-state release.
- [x] Bound finite wheel deltas to ±1,000,000 across Go, TypeScript, Swift, .NET, native providers, and canonical fixtures so unsafe values fail before integer conversion.
- [x] Preserve the last normalized pointer location when blur, visibility loss, or control-disable synthesizes held-button releases.
- [x] Serialize browser encrypted record construction and WebSocket writes per connection, poison the queue after failure, and reject stale work across close/reconnect.
- [x] Consume fire-and-forget viewer send rejections and preserve inbound record order with a separate receive queue.
- [x] Revalidate each inbound job's captured socket, generation, record layer, and queue after decrypt and before Pong so stale old-epoch messages cannot dispatch into a replacement session.

---

### Task 1: Input, clipboard, control-mode, and special-key semantics

**Files:**
- Modify: `internal/protocol/desktop_data.go`
- Modify: `internal/protocol/desktop_data_test.go`
- Modify: `web/dashboard/src/desktop/protocol.ts`
- Modify: `web/dashboard/src/desktop/protocol.test.ts`
- Modify: `schemas/desktop/v1/test-vectors.json`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKit/DesktopProtocol.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Contracts/DesktopProtocol.cs`

**Interfaces:**
- Consumes: Milestone 2 inner-message envelope and Milestone 3 display descriptors/generations.
- Produces: exact bounded `DesktopPointerEvent`, `DesktopKeyboardEvent`, `DesktopClipboardMessage`, `DesktopControlMode`, `DesktopDisplaySelection`, and `DesktopSpecialKey` contracts.

- [x] **Step 1: Add failing semantic validation tests**

```go
func TestDesktopPointerRequiresNormalizedCoordinatesAndCurrentDisplay(t *testing.T) {
	event := DesktopPointerEvent{DisplayID:"display-1", Generation:3, Kind:"move", X:0.5, Y:0.25}
	if err := event.Validate(); err != nil { t.Fatalf("Validate: %v", err) }
	event.X = 1.01
	if err := event.Validate(); err == nil { t.Fatal("out-of-range coordinate accepted") }
}

func TestDesktopClipboardAndSpecialKeyBounds(t *testing.T) {
	if err := (DesktopClipboardText{Direction:"browser_to_agent", Text:strings.Repeat("x", (1<<20)+1)}).Validate(); err == nil { t.Fatal("oversized clipboard accepted") }
	if err := (DesktopSpecialKey{Name:"arbitrary_command"}).Validate(); err == nil { t.Fatal("unknown special key accepted") }
}
```

- [x] **Step 2: Run and verify contract tests fail**

Run: `go test ./internal/protocol -run 'DesktopPointer|DesktopKeyboard|DesktopClipboard|DesktopSpecial' -count=1`

Expected: FAIL because semantic validators are absent.

- [x] **Step 3: Define exact message fields and bounds**

```go
type DesktopPointerEvent struct { DisplayID string `json:"display_id"`; Generation uint32 `json:"generation"`; Kind string `json:"kind"`; X float64 `json:"x"`; Y float64 `json:"y"`; Button int8 `json:"button"`; Buttons uint16 `json:"buttons"`; WheelX float64 `json:"wheel_x,omitempty"`; WheelY float64 `json:"wheel_y,omitempty"`; EventUnixMS int64 `json:"event_unix_ms"` }
type DesktopKeyboardEvent struct { Code string `json:"code"`; ScanCode uint32 `json:"scan_code"`; Location uint8 `json:"location"`; Down bool `json:"down"`; Repeat bool `json:"repeat"`; Shift bool `json:"shift"`; Control bool `json:"control"`; Alt bool `json:"alt"`; Meta bool `json:"meta"`; EventUnixMS int64 `json:"event_unix_ms"` }
type DesktopClipboardText struct { Direction string `json:"direction"`; Text string `json:"text"` }
type DesktopControlMode struct { Enabled bool `json:"enabled"`; FocusLease uint64 `json:"focus_lease"` }
type DesktopDisplaySelection struct { DisplayID string `json:"display_id"`; Generation uint32 `json:"generation"` }
type DesktopSpecialKey struct { Name string `json:"name"` }
```

Pointer `button` is `-1` when no button changed and `0` through `4` for the DOM primary/auxiliary buttons. Closed special-key names are `alt_tab`, `windows_l`, `ctrl_alt_delete`, `command_space`, `command_option_escape`, and `command_control_q`.

- [x] **Step 4: Add canonical positive/negative vectors in all languages**

Add pointer edges 0/1, high-DPI display generation, left/right modifiers, key repeat, wheel, clipboard directions/limits, control lease, monitor selection, permitted special names, unknown name, stale generation, and oversized text. Swift/.NET/TypeScript tests must consume the same JSON bytes.

- [x] **Step 5: Run the contract gates**

Run: `go test ./internal/protocol ./internal/desktopcrypto -count=1 && cd web/dashboard && npm test -- --run src/desktop/protocol.test.ts`

Expected: PASS.

```bash
git add internal/protocol/desktop_data.go internal/protocol/desktop_data_test.go schemas/desktop/v1/test-vectors.json web/dashboard/src/desktop/protocol.ts web/dashboard/src/desktop/protocol.test.ts
git commit -m "feat(remote-desktop): define native control messages"
```

- [ ] **Commit Task 1 changes** — not performed; commit authorization was not given.

### Task 2: Browser input focus lease and physical event mapping

**Files:**
- Create: `web/dashboard/src/desktop/inputController.ts`
- Create: `web/dashboard/src/desktop/inputController.test.ts`
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.tsx`
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.test.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: selected display/generation and encrypted `DesktopSocket.send`.
- Produces: `DesktopInputController`, explicit focus lease, normalized pointer/keyboard events, control toggle, and visible focus state.

- [x] **Step 1: Write failing focus, pointer, keyboard, and teardown tests**

```ts
it("sends input only with active control and focus lease", () => {
  const controller = inputFixture({ control: true, focused: false });
  controller.pointer(pointerEvent({ clientX: 50, clientY: 25 }));
  expect(controller.sent()).toEqual([]);
  controller.focus(); controller.pointer(pointerEvent({ clientX: 50, clientY: 25 }));
  expect(controller.sent()[1]).toMatchObject({ x: 0.5, y: 0.25, display_id: "display-1" });
});

it("releases remote modifiers on blur", () => {
  const controller = focusedInputFixture();
  controller.key(keyEvent({ code: "ShiftLeft", key: "Shift", type: "keydown" }));
  controller.blur();
  expect(controller.sent().at(-1)).toMatchObject({ code: "ShiftLeft", down: false });
});
```

- [x] **Step 2: Run and verify browser input tests fail**

Run: `cd web/dashboard && npm test -- --run src/desktop/inputController.test.ts src/desktop/DesktopViewerPage.test.tsx`

Expected: FAIL because the controller is absent.

- [x] **Step 3: Implement focus and coordinate mapping**

`DesktopInputController` allocates a monotonically increasing 64-bit focus lease on explicit click/focus. It ignores events when inactive, view-only, reconnecting, hidden, pointer-unlocked, or stale generation. Map coordinates from the rendered content rectangle, not the outer toolbar; clamp exactly to `[0,1]`. Coalesce move events to one per animation frame but never coalesce button or wheel events.

- [x] **Step 4: Implement physical keyboard handling**

Use `KeyboardEvent.code`, `location`, `repeat`, and modifier booleans; prevent browser default only for keys actually sent. On blur, visibility loss, mode disable, reconnect, or unmount, synthesize key-up/button-up for every remotely held key/button and invalidate the focus lease.

- [x] **Step 5: Add visible control/focus UX and run frontend gates**

Show `View only`, `Control enabled`, and `Click display to control` as distinct states. Keep keyboard-accessible Enable/Disable Control buttons and a visible focus ring.

Run: `make frontend-test && make frontend-check && make frontend-build`

Expected: PASS.

```bash
git add web/dashboard/src/desktop/inputController.ts web/dashboard/src/desktop/inputController.test.ts web/dashboard/src/desktop/DesktopViewerPage.tsx web/dashboard/src/desktop/DesktopViewerPage.test.tsx web/dashboard/src/styles.css
git commit -m "feat(remote-desktop): capture browser input safely"
```

- [ ] **Commit Task 2 changes** — not performed; commit authorization was not given.

### Task 3: Browser clipboard and monitor/special-key controls

**Files:**
- Create: `web/dashboard/src/desktop/clipboardController.ts`
- Create: `web/dashboard/src/desktop/clipboardController.test.ts`
- Create: `web/dashboard/src/desktop/specialKeys.ts`
- Create: `web/dashboard/src/desktop/specialKeys.test.ts`
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.tsx`
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.test.tsx`

**Interfaces:**
- Consumes: granted permissions, display inventory, and encrypted socket.
- Produces: explicit Copy From Remote/Paste To Remote gestures, monitor selection requests, and bounded special-key menu.

- [x] **Step 1: Write failing permission/gesture tests**

Test no automatic clipboard reads, read/write permissions independently, 1 MiB limit before send/write, unsupported clipboard API, non-fatal denial, monitor request/ack distinction, disabled privileged special keys, and no clipboard content in errors.

- [x] **Step 2: Implement explicit clipboard methods**

```ts
export class DesktopClipboardController {
  pasteToRemote(): Promise<void>;
  copyFromRemote(): Promise<void>;
  acceptRemoteText(text: string): void;
}
```

`pasteToRemote` calls `navigator.clipboard.readText()` only from the button handler and sends only with `desktop.clipboard.write`. `copyFromRemote` requests remote text, then calls `navigator.clipboard.writeText()` only while satisfying the user gesture and `desktop.clipboard.read`.

- [x] **Step 3: Implement monitor and special-key controls**

Monitor selection remains pending until endpoint acknowledgement supplies a new stream generation. Special-key menu shows only platform-applicable names; `ctrl_alt_delete` is visible but disabled with `Requires privileged control` until Milestone 5.

- [x] **Step 4: Run browser control tool gates**

Run: `make frontend-test && make frontend-check && make frontend-build`

Expected: PASS.

```bash
git add web/dashboard/src/desktop/clipboardController.ts web/dashboard/src/desktop/clipboardController.test.ts web/dashboard/src/desktop/specialKeys.ts web/dashboard/src/desktop/specialKeys.test.ts web/dashboard/src/desktop/DesktopViewerPage.tsx web/dashboard/src/desktop/DesktopViewerPage.test.tsx
git commit -m "feat(remote-desktop): add clipboard and monitor controls"
```

- [ ] **Commit Task 3 changes** — not performed; commit authorization was not given.

### Task 4: Portable endpoint input authorization gate

**Files:**
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/DesktopSessionCoordinator.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/DesktopInputAuthorizer.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKitSelftest/main.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopSessionCoordinator.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopInputAuthorizer.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/DesktopInputAuthorizationTests.cs`

**Interfaces:**
- Consumes: decrypted messages plus current session/epoch/permissions/display state.
- Produces: one fail-closed decision before any platform provider receives input or clipboard.

- [x] **Step 1: Write identical Swift/.NET authorization tables**

Cases cover inactive/reconnecting/terminal state, wrong epoch, missing view/control/clipboard permission, disabled control mode, stale/missing focus lease, stale display/generation, platform not ready, oversized clipboard, special-key allowlist, and successful normal input.

- [x] **Step 2: Implement pure authorization decisions**

```swift
public enum DesktopInputDecision: Equatable { case allow; case deny(String) }
public struct DesktopInputAuthorizer { public func authorize(_ message: DesktopHostCommand, state: DesktopAuthorizationState) -> DesktopInputDecision }
```

Create matching C# `DesktopInputDecision Authorize(DesktopHostCommand, DesktopAuthorizationState)`. It has no side effects and returns stable reason codes.

- [x] **Step 3: Gate coordinator dispatch and acknowledgements**

Only `allow` reaches `DesktopHost.apply`. Denials produce encrypted acknowledgement/reason and metadata-only agent lifecycle event; they never echo key, pointer, or clipboard payloads. A mode-disable flushes held input before acknowledgement.

- [x] **Step 4: Run portable authorization gates**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter DesktopInputAuthorizationTests`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Sources/HankAgentCore/DesktopSessionCoordinator.swift Sources/HankAgentCore/Desktop/DesktopInputAuthorizer.swift Sources/HankKitSelftest/main.swift HankAgent-Windows/src/HankAgent.Worker/Desktop HankAgent-Windows/tests/HankAgent.Tests/DesktopInputAuthorizationTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): gate native control commands"
```

- [ ] **Commit Task 4 changes** — not performed; commit authorization was not given.

### Task 5: Windows normal input and text clipboard provider

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/WindowsInputProvider.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/WindowsClipboardProvider.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/WindowsSpecialKeyProvider.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/WindowsDesktopControlTests.cs`

**Interfaces:**
- Consumes: authorized host commands and current physical display geometry.
- Produces: Win32 normal pointer/keyboard injection, text clipboard operations, and non-privileged special keys.

- [x] **Step 1: Write failing Win32 translation tests with fake API**

Test multi-monitor virtual coordinates including negative origins, absolute pointer scaling to 0–65535, left/right scan-code mapping, extended-key flags, repeat, wheel, modifier release, Unicode clipboard, size bound, unavailable secure attention, and local input remaining enabled.

- [x] **Step 2: Implement pointer and keyboard injection**

Map normalized coordinates through the selected monitor bounds, then virtual-screen coordinates for `SendInput` with `MOUSEEVENTF_ABSOLUTE|MOUSEEVENTF_VIRTUALDESK`. Use scan-code keyboard input with `KEYEVENTF_SCANCODE`, `KEYEVENTF_EXTENDEDKEY`, and `KEYEVENTF_KEYUP`; never use text injection for physical keyboard messages.

- [x] **Step 3: Implement STA clipboard operations**

Run clipboard access on a dedicated STA thread, use Unicode text only, enforce 1 MiB UTF-8 before conversion, retry a busy clipboard with bounded backoff for at most 500 ms, clear temporary buffers, and never log text.

- [x] **Step 4: Implement normal special keys and explicit privileged denial**

Support `alt_tab` and `windows_l` through bounded platform calls. Return `privileged_control_unavailable` for `ctrl_alt_delete` and any secure-desktop transition.

- [x] **Step 5: Run Windows control gates**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter WindowsDesktopControlTests`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add HankAgent-Windows/src/HankAgent.Windows/Desktop HankAgent-Windows/tests/HankAgent.Tests/WindowsDesktopControlTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): control windows console"
```

- [ ] **Commit Task 5 changes** — not performed; commit authorization was not given.

### Task 6: macOS normal input and text clipboard provider

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/MacInputProvider.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/MacClipboardProvider.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/MacSpecialKeyProvider.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKitSelftest/main.swift`

**Interfaces:**
- Consumes: authorized host commands and current CoreGraphics display geometry.
- Produces: CGEvent normal pointer/keyboard injection, NSPasteboard text, and non-privileged special keys.

- [x] **Step 1: Write failing CoreGraphics translation tests with fake sink**

Test display origins/scales, normalized-to-global coordinates, mouse buttons/wheel, hardware keycode mapping, left/right modifiers, repeat, key release, Accessibility unavailable, text clipboard bounds, and normal special keys.

- [x] **Step 2: Implement CGEvent pointer/keyboard provider**

Create mouse and keyboard `CGEvent` values using physical virtual keycodes and explicit flags; post only to the HID event tap after `AXIsProcessTrusted()` succeeds. Do not use Unicode event text for physical key messages.

- [x] **Step 3: Implement NSPasteboard and special keys**

Use `.general` with `NSPasteboard.PasteboardType.string`, enforce 1 MiB UTF-8, and clear temporary buffers. Support `command_space`, `command_option_escape`, and `command_control_q`; report `accessibility_required` when events cannot be posted.

- [x] **Step 4: Run macOS control gates**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Sources/HankAgentCore/Desktop Sources/HankKitSelftest/main.swift
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): control mac console"
```

- [ ] **Commit Task 6 changes** — not performed; commit authorization was not given.

### Task 7: Control-mode audit and native acceptance gate

**Files:**
- Modify: `internal/cloud/desktop_control.go`
- Modify: `internal/cloud/desktop_control_test.go`
- Create: `docs/remote-desktop/native-control-acceptance.md`

**Interfaces:**
- Consumes: all Milestone 4 tasks.
- Produces: metadata-only control/display/clipboard audit events and physical-device proof.

- [x] **Step 1: Add metadata-only audit tests**

Audit mode enabled/disabled, display changed, clipboard direction/success/failure, and named special key; store only names/directions/reasons, never codes, scan codes, coordinates, buttons, wheel values, or clipboard text.

- [x] **Step 2: Run complete portable gates**

Run: `go test ./... && make build && make frontend-test && make frontend-check && make frontend-build`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj`

Expected: PASS.

- [ ] **Step 3: Run physical Windows and macOS control matrix** — blocked on macOS Screen Recording approval and availability of this exact uncommitted Hankagent worktree on the connected Windows VM; see the acceptance document.

On each platform prove view-only blocks pointer/keyboard/clipboard; control mode moves/clicks/types physical keys; local input still works; focus loss releases held state; clipboard directions are independent; monitor switch uses correct geometry; reconnect requires focus again; normal special keys work; privileged special paths clearly report unavailable.

- [x] **Step 4: Record audit and acceptance evidence**

```bash
git add internal/cloud/desktop_control.go internal/cloud/desktop_control_test.go docs/remote-desktop/native-control-acceptance.md
git commit -m "test(remote-desktop): prove native console control"
```

- [ ] **Commit Task 7 changes** — not performed; commit authorization was not given.

## Milestone 4 Exit Criteria

- [x] View-only mode blocks all native input and clipboard writes under stale/racing messages in portable authorization tests.
- [ ] Pointer, physical keyboard, modifiers, wheel, focus teardown, and normal special keys work on both physical platforms while local input remains enabled — portable provider tests pass; physical proof remains blocked.
- [x] Clipboard text obeys independent directions, explicit browser gestures, 1 MiB bound, memory-only handling, and content-free logs/audit.
- [x] Monitor selection changes stream generation and input geometry without stale coordinates.
- [x] Control acknowledgements reflect endpoint success; the viewer never claims an operation succeeded early.
- [x] Privileged/UAC/lock-screen paths remain unavailable with precise reasons until Milestone 5.
- [x] No service/daemon installation, privacy screen, local-input blocking, deployment, or push occurred.
