# Hank Native Remote Desktop Milestone 3 Native Viewing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace both synthetic video sources with active physical-console discovery, native Windows/macOS capture, H.264 encoding, display-change handling, quality/statistics reporting, and an always-visible local indicator while preserving the Milestone 2 encrypted session contract.

**Architecture:** `DesktopSessionCoordinator` continues to own identity, authorization, keys, socket state, and termination. Each platform provides a native `DesktopHost` composed from an active-console resolver, frame source, H.264 encoder, display inventory, and indicator; those providers emit the same encrypted inner messages as the synthetic host. The browser remains provider-agnostic and reacts only to codec generation, display inventory, quality, and statistics messages.

**Tech Stack:** Swift 6, ScreenCaptureKit, CoreGraphics, VideoToolbox, AppKit; .NET 8 Windows target, Windows Graphics Capture/Direct3D 11, Media Foundation, Windows App SDK, Win32 session APIs; React/TypeScript, WebCodecs/MSE; existing Go protocol/audit layers.

## Global Constraints

- Begin only after Milestone 2 synthetic acceptance passes on browser, macOS, Windows, server opacity, reconnect, and termination.
- Capture the exact physical console currently visible locally; never create RDP, a virtual desktop, a second login, or a hidden display.
- Milestone 2 handshake, record, permission, reconnect, relay, and inner-message formats remain unchanged.
- V1 remains one operator per endpoint; local viewing and local input remain active.
- Native providers receive only session-scoped authorization and keys through `DesktopSessionCoordinator`; reusable Hank credentials remain outside provider code.
- The local indicator must be visible before the first native frame is emitted; indicator loss pauses capture and triggers the existing fail-closed lifecycle.
- Windows normal-desktop capture is implemented here; elevated applications and UAC secure desktop remain Milestone 5.
- macOS reports missing Screen Recording permission here but guided permission setup, lock transitions, and privileged helper behavior remain Milestone 5.
- Native encoders emit H.264 compatible with the existing browser decoder abstraction and force a keyframe after start, display change, encoder reset, and reconnect.
- A display/console-user change invalidates stale geometry and frames; the browser never presents an old display as live.
- No native input injection, clipboard synchronization, service/daemon installation, packaging, deployment, or push occurs in this milestone.

## Execution Status — 2026-07-21

- Checked steps below have completed implementation and portable-test evidence. Commit command snippets were intentionally not run because this execution explicitly prohibited commits, pushes, installation, and deployment.
- Portable evidence: HankServerside Go tests/build pass; browser 54-file/284-test gate and production build pass; Swift build/selftests pass; .NET agent tests and Windows-targeted native-library build pass.
- The production trust prerequisite is closed: Swift, .NET, and browser paths pin the current Home Trust Root, validate delegated operator/endpoint chains and generation, require a fresh active identity assertion, and fail closed for revoked, stale, wrong-generation, or substituted-root cases.
- Rendered desktop/phone acceptance now passes, including phone-width containment plus Fit and internal Actual Size scrolling. After explicit user approval, Screen Recording was enabled for the local HankAgent build. The live Mac path passes endpoint enrollment, durable offer/relay ordering, credential consumption, E2EE handshake, physical-console UID validation, exact 1440x900 console video, a timestamped TextEdit pattern, more than 60 seconds at roughly 29-30 fps with zero drops, and browser End Session. Physical display topology/reconnect/revocation rows remain; the connected WindowsVM is visible, but both direct and projectless handoffs are rejected because no matching saved Hankagent project exists.
- The acceptance record and exact remaining matrices are in `docs/remote-desktop/native-viewing-acceptance.md`.

---

### Task 1: Native viewing provider contract and stream-generation rules

**Files:**
- Modify: `internal/protocol/desktop_data.go`
- Modify: `internal/protocol/desktop_data_test.go`
- Modify: `web/dashboard/src/desktop/protocol.ts`
- Modify: `web/dashboard/src/desktop/protocol.test.ts`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/DesktopSessionCoordinator.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopSessionCoordinator.cs`

**Interfaces:**
- Consumes: Milestone 2 `DesktopHost`, codec configuration, access-unit, display inventory, quality, statistics, and permission messages.
- Produces: exact display descriptors, stream-generation monotonicity, native provider readiness, and keyframe reset rules.

- [x] **Step 1: Write failing cross-language provider-contract tests**

```go
func TestDesktopDisplayDescriptorRequiresStableIDAndPositiveGeometry(t *testing.T) {
	valid := DesktopDisplayDescriptor{ID:"display-1", Name:"Primary", X:0, Y:0, Width:1920, Height:1080, Scale:1, Primary:true}
	if err := valid.Validate(); err != nil { t.Fatalf("Validate: %v", err) }
	valid.Width = 0
	if err := valid.Validate(); err == nil { t.Fatal("zero-width display accepted") }
}

func TestDesktopStreamGenerationMustIncreaseOnGeometryChange(t *testing.T) {
	state := DesktopStreamState{Generation:4, DisplayID:"display-1", Width:1920, Height:1080}
	if err := state.ApplyConfig(DesktopCodecConfig{Generation:4, DisplayID:"display-1", Width:1280, Height:720}); err == nil { t.Fatal("geometry changed without generation increment") }
}
```

- [x] **Step 2: Run and verify the tests fail**

Run: `go test ./internal/protocol -run 'DesktopDisplay|DesktopStream' -count=1`

Expected: FAIL because native display/stream validation is incomplete.

- [x] **Step 3: Define the portable provider interfaces**

```swift
public protocol DesktopConsoleResolver: Sendable { func activeConsole() async throws -> DesktopConsole }
public protocol DesktopFrameSource: Sendable { func displays() async throws -> [DesktopDisplay]; func frames(displayID: String) async throws -> AsyncStream<DesktopFrame> }
public protocol DesktopH264Encoder: Sendable { func configure(_ configuration: DesktopEncodeConfiguration) async throws -> DesktopCodecConfiguration; func encode(_ frame: DesktopFrame, forceKeyframe: Bool) async throws -> DesktopAccessUnit; func reset() async }
public protocol DesktopSessionIndicator: Sendable { func show(session: DesktopIndicatorSession) async throws; func hide() async; var terminationRequests: AsyncStream<Void> { get } }
```

Define matching C# interfaces `IDesktopConsoleResolver`, `IDesktopFrameSource`, `IDesktopH264Encoder`, and `IDesktopSessionIndicator`. Both coordinators select synthetic or native host through an injected `DesktopHostFactory`; production selects native only when readiness succeeds.

- [x] **Step 4: Enforce generation/keyframe/state semantics**

Require monotonically increasing generation on display selection, size/rotation change, encoder reconfigure, provider reset, and reconnect. The first access unit for every generation must be a keyframe preceded by codec configuration. Permission loss or console mismatch emits state and no frame.

- [x] **Step 5: Run the provider contract (commit intentionally skipped)**

Run: `go test ./internal/protocol -count=1 && cd web/dashboard && npm test -- --run src/desktop/protocol.test.ts`

Expected: PASS.

```bash
git add internal/protocol/desktop_data.go internal/protocol/desktop_data_test.go web/dashboard/src/desktop/protocol.ts web/dashboard/src/desktop/protocol.test.ts
git commit -m "feat(remote-desktop): define native viewing providers"
```

### Task 2: Windows active-console discovery and frame capture

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/WindowsConsoleResolver.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/WindowsDisplayInventory.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/WindowsGraphicsCaptureSource.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/WindowsDesktopInterop.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/WindowsDesktopCaptureTests.cs`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/HankAgent.Windows.csproj`

**Interfaces:**
- Consumes: Task 1 provider contracts.
- Produces: active-console identity, stable physical-monitor inventory, and BGRA/D3D11 native frames.

- [x] **Step 1: Write failing resolver/inventory/frame-lifetime tests**

```csharp
[Fact]
public void ResolverRejectsProcessOutsideActiveConsole()
{
    var resolver = new WindowsConsoleResolver(new FakeWindowsSessionApi(activeSession: 3, processSession: 2));
    Assert.Throws<DesktopConsoleMismatchException>(() => resolver.Resolve());
}

[Fact]
public async Task CaptureDropsFramesAfterDisplayGenerationChanges()
{
    var source = WindowsCaptureFixture.Create();
    var first = await source.NextAsync();
    source.ChangeDisplaySize(1280, 720);
    Assert.False(source.IsCurrent(first));
}
```

- [x] **Step 2: Implement Win32 session and physical-display discovery**

Use `WTSGetActiveConsoleSessionId`, `ProcessIdToSessionId`, `EnumDisplayMonitors`, `GetMonitorInfo`, and `QueryDisplayConfig`. Stable display ID is SHA-256 of adapter LUID, target ID, and monitor device path; display name is the non-sensitive friendly name. Reject disconnected, mirroring-only, and zero-area targets.

- [x] **Step 3: Implement Windows Graphics Capture with Desktop Duplication fallback**

Create `GraphicsCaptureItem` for the selected monitor through `IGraphicsCaptureItemInterop`, a D3D11 device, and `Direct3D11CaptureFramePool`. Copy each current frame into an owned texture before returning it. On unsupported capture, use DXGI Desktop Duplication for the same monitor. Emit precise `capture_unavailable`, `display_removed`, and `console_changed` errors.

- [x] **Step 4: Run Windows capture unit tests**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter 'WindowsDesktopCaptureTests'`

Expected: portable fake tests PASS; hardware capture tests are explicitly trait-gated `NativeDesktopCapture` and run in Task 7.

- [ ] **Step 5: Commit Windows capture**

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add HankAgent-Windows/src/HankAgent.Windows/Desktop HankAgent-Windows/src/HankAgent.Windows/HankAgent.Windows.csproj HankAgent-Windows/tests/HankAgent.Tests/WindowsDesktopCaptureTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): capture windows console"
```

### Task 3: Windows Media Foundation H.264 encoder

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/MediaFoundationH264Encoder.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Windows/Desktop/SoftwareH264Encoder.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/WindowsH264EncoderTests.cs`

**Interfaces:**
- Consumes: D3D11/BGRA frames from Task 2 and `IDesktopH264Encoder`.
- Produces: baseline-compatible codec configuration and length-bounded H.264 access units.

- [x] **Step 1: Write failing encoder contract tests**

Assert first-frame keyframe, SPS/PPS extraction, monotonic PTS, no B-frames, generation reset, 640x360 and 1920x1080 dimension reporting, bitrate clamp 500 Kbit/s–20 Mbit/s, and software fallback selection.

- [x] **Step 2: Implement Media Foundation hardware selection**

Enumerate `MFT_CATEGORY_VIDEO_ENCODER` H.264 transforms, prefer hardware/D3D11-aware transforms, configure NV12 input and H.264 output, baseline/main compatible profile, no B-frames, low latency, and requested keyframes through `CODECAPI_AVEncVideoForceKeyFrame`.

- [x] **Step 3: Implement bounded software fallback**

Use the Windows software H.264 Media Foundation transform when no hardware transform accepts the format. Do not add FFmpeg to runtime. Surface `encoder_unavailable` if neither transform works.

- [x] **Step 4: Run encoder tests (commit intentionally skipped)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter 'WindowsH264EncoderTests'`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add HankAgent-Windows/src/HankAgent.Windows/Desktop HankAgent-Windows/tests/HankAgent.Tests/WindowsH264EncoderTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): encode windows h264"
```

### Task 4: macOS active-console capture and VideoToolbox encoding

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/MacConsoleResolver.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/ScreenCaptureKitFrameSource.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/VideoToolboxH264Encoder.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/MacDesktopHost.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Package.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKitSelftest/main.swift`

**Interfaces:**
- Consumes: Task 1 provider contracts.
- Produces: macOS active-console discovery, display inventory, BGRA frames, and H.264 access units.

- [x] **Step 1: Add failing fake ScreenCaptureKit/VideoToolbox tests**

Cover console UID mismatch, stable `CGDirectDisplayID`, primary display, display add/remove, permission denied, first keyframe, SPS/PPS, generation increment, PTS monotonicity, hardware preference, and software fallback.

- [x] **Step 2: Implement active-console/display discovery**

Resolve `kCGSessionOnConsoleKey` and `kCGSessionUserIDKey` through `CGSessionCopyCurrentDictionary`; require the current effective UID to own the console. Build inventory from `SCShareableContent.current.displays`, keyed by `displayID`, with CoreGraphics bounds/scale/primary metadata.

- [x] **Step 3: Implement ScreenCaptureKit frames**

Create one `SCStream` for the selected display, exclude audio, configure BGRA pixel format, 30 fps minimum interval, bounded queue depth, and cursor inclusion. Copy or retain each `CVPixelBuffer` only through encode completion. Display removal or permission loss stops emission immediately.

- [x] **Step 4: Implement VideoToolbox encoder**

Use `VTCompressionSessionCreate` with H.264, real-time mode, no frame reordering, 1-second keyframe interval, requested bitrate, and hardware acceleration preference. Extract SPS/PPS from `CMVideoFormatDescription`; convert AVCC NAL units into the existing access-unit format. Permit VideoToolbox software encoding when hardware is unavailable.

- [x] **Step 5: Run macOS viewing providers (commit intentionally skipped)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Expected: PASS; permission-dependent native acceptance runs in Task 7.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Package.swift Sources/HankAgentCore/Desktop Sources/HankKitSelftest/main.swift
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): capture and encode mac console"
```

### Task 5: Persistent local indicator and native-host activation

**Files:**
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgent/AppDelegate.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgent/DesktopIndicatorController.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.App/Services/TrayIconService.cs`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.App/Services/DesktopIndicatorService.cs`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopSessionCoordinator.cs`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/DesktopSessionCoordinator.swift`

**Interfaces:**
- Consumes: native hosts from Tasks 2–4 and coordinator indicator contract.
- Produces: visible Remote Desktop indicator, local End Session, and fail-closed activation.

- [x] **Step 1: Write indicator lifecycle tests**

Assert activation waits for visible indicator acknowledgement, local termination closes session, disappearance pauses frames, one restart is attempted, failure after ten seconds terminates, reconnect retains the indicator, and no ordinary setting hides it.

- [x] **Step 2: Implement macOS status-item indicator**

During active/reconnecting state, change the existing status item to a persistent remote-access badge and menu containing session status and `End Remote Session`. The controller acknowledges visibility only after AppKit creates the item on the main actor.

- [x] **Step 3: Implement Windows tray indicator**

Extend the existing tray service with an always-present Remote Desktop state, tooltip, visible menu item, and local End Session callback. Coordinator activation requires tray acknowledgement from the interactive process.

- [x] **Step 4: Select native hosts only after readiness**

Production host factory chooses native providers when console, capture, encoder, and indicator are ready. Synthetic provider remains available only under explicit development/test configuration and is labeled in lifecycle metadata.

- [x] **Step 5: Run indicator integration (commit intentionally skipped)**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj`

Expected: PASS.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Sources/HankAgent/AppDelegate.swift Sources/HankAgent/DesktopIndicatorController.swift Sources/HankAgentCore/DesktopSessionCoordinator.swift HankAgent-Windows/src/HankAgent.App/Services HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopSessionCoordinator.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): require local session indicator"
```

### Task 6: Browser display inventory, scaling, quality, and statistics

**Files:**
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.tsx`
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.test.tsx`
- Create: `web/dashboard/src/desktop/displayStore.ts`
- Create: `web/dashboard/src/desktop/displayStore.test.ts`
- Modify: `web/dashboard/src/desktop/DesktopDecoder.ts`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: native display/codec/quality/statistics messages.
- Produces: current-display inventory, fit/actual scaling, generation-safe decoder reset, and operator-visible health; interactive monitor switching remains Milestone 4.

- [x] **Step 1: Write failing display/generation/UI tests**

Test inventory replacement, current-display removal fallback selected by the endpoint, decoder reset on generation change, stale access-unit rejection, fit/actual scale, fullscreen, requested/applied quality distinction, fps/bitrate/dropped-frame/latency display, disabled monitor-switch presentation, and phone containment.

- [x] **Step 2: Implement deterministic display state**

```ts
export type DisplayState = { inventory: DesktopDisplay[]; selectedID: string | null; generation: number; mode: "fit"|"actual" };
export function applyDisplayInventory(state: DisplayState, inventory: DesktopDisplay[]): DisplayState;
export function applyCodecConfiguration(state: DisplayState, config: DesktopCodecConfig): DisplayState;
```

Only the currently selected display/generation reaches the decoder. A display change clears the last rendered frame and shows `Switching display…` until a new keyframe decodes.

- [x] **Step 3: Implement controls and status presentation**

Show the endpoint-selected monitor and remaining inventory read-only, plus Fit/Actual Size, fullscreen, requested quality, applied resolution/bitrate, fps, frame drops, and round-trip latency. Label monitor switching as unavailable until control support lands in Milestone 4. Disable controls while reconnecting and retain accessible buttons beside pointer-friendly controls.

- [x] **Step 4: Run browser viewing UX (commit intentionally skipped)**

Run: `make frontend-test && make frontend-check && make frontend-build`

Expected: PASS.

```bash
git add web/dashboard/src/desktop web/dashboard/src/styles.css
git commit -m "feat(remote-desktop): present native displays"
```

### Task 7: Native viewing acceptance gate

**Files:**
- Create: `docs/remote-desktop/native-viewing-acceptance.md`
- Modify: `docs/remote-desktop/synthetic-acceptance.md`

**Interfaces:**
- Consumes: all Milestone 3 tasks.
- Produces: device evidence that the browser shows the exact physical console on Windows and macOS.

- [x] **Step 1: Run portable gates**

Run: `go test ./... && make build && make frontend-test && make frontend-check && make frontend-build`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj`

Expected: PASS.

- [ ] **Step 2: Run Windows native viewing acceptance**

Blocked: the connected Codex WindowsVM has no matching saved Hankagent project. Direct project handoff and the source-only projectless fallback were both rejected by the connection layer before dispatch.

On a physical Windows console with no RDP session, show a timestamped test pattern locally, connect from the browser, and record matching display ID, dimensions, visible content, first-frame/keyframe time, 60-second frame stability, display add/remove, resize/rotation, reconnect, indicator visibility, and End Session. Repeat with hardware encoder disabled to prove software fallback.

- [ ] **Step 3: Run macOS native viewing acceptance**

Partially complete after explicit user approval enabled Screen Recording: a disposable isolated cloud/database, protected endpoint/operator identities, rendered browser binding, and locally installed debug agent established the live encrypted session. The browser matched a timestamped TextEdit pattern on the exact 1440x900 primary console and remained stable for more than 60 seconds at roughly 29-30 fps with zero drops before `End Session` produced `operator_ended`. Display add/remove/rotation, forced reconnect, encoder-selection evidence, and permission revocation/re-grant remain physical or disruptive device rows.

On a physical macOS console with Screen Recording granted, run the same content/display/reconnect/indicator matrix. Revoke Screen Recording and prove frames stop immediately with `screen_recording_required`; re-grant behavior is deferred to Milestone 5 but no stale frame may remain.

- [ ] **Step 4: Commit acceptance evidence**

The acceptance documents are written, but this step remains unchecked because no commit was authorized and the physical evidence rows remain unrun.

Document OS/build, browser, agent commit, display topology, encoder selected, commands, results, limitations, and hashes of non-sensitive diagnostic artifacts.

```bash
git add docs/remote-desktop/native-viewing-acceptance.md docs/remote-desktop/synthetic-acceptance.md
git commit -m "test(remote-desktop): prove native console viewing"
```

## Milestone 3 Exit Criteria

- [ ] Windows and macOS browser sessions show the exact active physical console, not a virtual/RDP/secondary session.
- [ ] Hardware H.264 works where available and the platform software fallback works.
- [ ] Display inventory, endpoint-selected current display, changes, generation resets, keyframes, scaling, quality, statistics, and reconnect work without stale frames.
- [ ] A visible local indicator precedes capture, persists through reconnect, and can terminate the session.
- [ ] Indicator, permission, console, capture, or encoder failure stops frames and reports a stable reason.
- [ ] No native input, clipboard, privileged helper, installation, deployment, or push occurred. Later milestones added input/clipboard/privileged-helper code in the same worktree, and the user explicitly authorized a temporary local Mac debug deployment for live acceptance; no service/daemon install, publish, or push occurred.
