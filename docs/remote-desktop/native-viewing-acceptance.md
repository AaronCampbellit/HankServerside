# Native Remote Desktop Milestone 3 Acceptance

Date: 2026-07-22

Status: **Portable gates and live single-display macOS viewing passed; the remaining physical matrices are blocked on Windows project availability and display/signing hardware.** Screen Recording is enabled for the local HankAgent build. A real browser session rendered the exact 1440x900 physical Mac console for more than 60 seconds at roughly 29-30 fps with zero dropped frames, including a timestamped TextEdit pattern. Windows handoff is blocked because the connected WindowsVM has no matching saved Hankagent project.

## Build under test

- HankServerside base commit: `6a1456d2523b23a00d9307c1c06e103f0c1908a5` plus the uncommitted Milestone 1-6 worktree
- Hankagent base commit: `60b22e1397f8e2a9f71b6f855fc60b6c556d621e` plus the uncommitted Milestone 2-6 worktree
- Local gate host: macOS 26.5.1 (25F80), arm64
- Windows target: `net8.0-windows10.0.19041.0`
- Browser: Codex in-app browser, Vite acceptance proxy at desktop and 390 px phone widths
- Deployment/install: a local ad-hoc signed debug acceptance build was installed on the physical Mac; the live demo application was not modified

The live acceptance environment used a disposable database and temporary protected identity state. It did not reuse or mutate the demo application's database.

## Portable evidence

| Surface | Result | Evidence |
| --- | --- | --- |
| HankServerside Go | Pass | `go test ./...`; `make build` |
| Browser viewer | Pass | 54 files, 284 tests; TypeScript and Vite production build |
| macOS agent | Pass | `swift build`; all `hankkit-selftest` checks |
| Windows agent | Pass | 85 .NET tests; Windows library built for `net8.0-windows10.0.19041.0` with zero warnings/errors |
| Pinned desktop trust | Pass | Operator and endpoint delegated-certificate chains terminate at the configured current Home Trust Root; revoked, stale, wrong-generation, and substituted-root cases fail closed |
| Swift production data plane | Pass | Signed raw-P-256 handshake, indicator-before-host activation, encrypted inventory/config/access unit, browser termination, malformed wire bound |
| .NET production data plane | Pass | Equivalent full bridge test, including raw browser/endpoint ECDH key compatibility |
| Generation lifecycle | Pass | Display removal/resize invalidation, stale access-unit rejection, decoder reset, reconnect host retention, first-frame keyframe contracts |
| Local indicator lifecycle | Pass | Visibility acknowledgement, one bounded restoration, second-loss fail-close, local termination, reconnect retention |
| Explicit synthetic fallback | Pass | Both agents now emit native inventory/config/access-unit event shapes, including a valid AVC decoder configuration record |
| Live enrollment response | Pass | Endpoint approval returns the installed delegated certificate chain and public SPKI; the Mac ready event matches the server identity |
| Live relay ordering | Pass | `offered` is durable before the agent command; concurrent offer/credential consumption is serialized by row locks without PostgreSQL `40001` aborts |
| Live Mac transport | Pass | Agent/browser credentials are consumed, both relay sides join, the E2EE transcript is signed and verified, and the active console UID matches the process UID |
| Live Mac native video | Pass | ScreenCaptureKit and VideoToolbox produced the exact 1440x900 primary console in the browser; a timestamped TextEdit pattern matched the physical console |
| Live Mac stability | Pass | More than 60 seconds continuously connected at about 29-30 fps, 1.3-2.7 Mbps, 0 dropped frames, and roughly 1-4 ms reported latency |
| Live Mac operator termination | Pass | Browser `End Session` closed the stream with stable reason `operator_ended` and disabled session controls |

The Home Assistant pagination test that intermittently raced its React state update was stabilized with an asynchronous assertion; its behavior and product implementation are unchanged.

Production agents require the current Home Trust Root SPKI, fingerprint, and generation in their protected identity configuration before advertising desktop readiness. Swift reads the protected Keychain identity values and Windows reads the protected stored identity values; the `HANK_DESKTOP_TRUST_ROOT_PUBLIC_KEY`, `HANK_DESKTOP_TRUST_ROOT_FINGERPRINT`, and `HANK_DESKTOP_TRUST_ROOT_GENERATION` environment variables are development fallbacks.

The WinUI application project cannot complete its XAML compile on this macOS host because `XamlCompiler.exe` is a Windows executable. The portable application-service type check and the Windows-targeted native library build pass; the full WinUI build remains a Windows-device gate.

## Windows physical-console matrix

Result: **Blocked before dispatch.** The Codex WindowsVM connection is visible, but it has no matching saved Hankagent project. Direct handoff and a source-only projectless fallback were both rejected by the connection layer with `No matching saved project was found on WindowsVM`; no alternate device-access path was used.

- [ ] Confirm the process session equals `WTSGetActiveConsoleSessionId` with no RDP session.
- [ ] Show a timestamped local test pattern and match browser content, stable display ID, and dimensions.
- [ ] Record first-frame/keyframe time and 60 seconds of stable frames, fps, bitrate, drops, and RTT.
- [ ] Exercise display add/remove, resize/rotation, and reconnect without a stale frame.
- [ ] Confirm the tray indicator appears before the first frame, persists through reconnect, and End Remote Session closes both sides.
- [ ] Record hardware Media Foundation encoder selection.
- [ ] Disable the hardware encoder and repeat to prove the system Media Foundation software fallback.
- [ ] Complete the full WinUI Windows build.

## macOS physical-console matrix

Result: **Live single-display viewing passed.** A disposable isolated cloud/database, protected acceptance identities, Vite browser enrollment, and a local debug HankAgent build established a real native session. Certificate-chain installation, credential consumption, relay pairing, encrypted transcript verification, physical-console identity, ScreenCaptureKit capture, VideoToolbox output, 60-second stability, and operator termination all passed. The remaining topology, reconnect, encoder-selection, and permission-revocation rows require physical or disruptive device actions and were not fabricated.

- [x] Confirm `CGSessionCopyCurrentDictionary` identifies the effective user as the physical console owner.
- [x] With Screen Recording granted, match a timestamped local TextEdit pattern, stable `Main Display` identity, 1440x900 dimensions, and browser content.
- [x] Record browser connection within the bounded startup window and 60 seconds of stable frames: about 29-30 fps, 1.3-2.7 Mbps, 0 drops, and roughly 1-4 ms RTT.
- [ ] Exercise display add/remove, resize/rotation, and reconnect without a stale frame.
- [ ] Confirm the status-item indicator persists through a forced reconnect; indicator-before-capture acknowledgement and browser End Session passed, but the forced reconnect row remains unrun.
- [ ] Record VideoToolbox hardware/software encoder selection.
- [ ] Revoke Screen Recording and prove immediate frame cessation with `screen_recording_required` and no retained frame. Permission was intentionally left enabled after the user approved it.

## Browser layout matrix

Automated viewer state, controls, and containment CSS tests passed. Rendered measurements also passed: at 1280 px and 390 px, document `scrollWidth === clientWidth`; at 390 px Actual Size keeps the 640 px remote canvas inside the viewer's internal scrolling stage, while Fit constrains the stage to 388 px. Controls remained reachable at phone width. The in-app browser does not expose `requestFullscreen`, so native fullscreen remains a browser/device gate.

## Safety and impact

- Milestone 3 capture providers remain view-focused. This acceptance used the combined later-milestone worktree; remote pointer control was separately exercised only after the native video gate passed.
- Native providers receive session-scoped authorization only. Reusable worker credentials do not enter capture or encoder providers.
- Indicator, permission, console, capture, encoder, record, or transport failure stops forwarding and closes the session with a bounded reason.
- No schema or migration change was introduced by Milestone 3. The new concurrency test used automatically created and dropped PostgreSQL test databases.
- No commit, push, publish, or live-demo deployment occurred. The local Mac install and disposable acceptance database are temporary test state.

## Exit decision

Milestone 3 is **DONE WITH CONCERNS**. Portable implementation, rendered browser behavior, live enrollment, relay, E2EE handshake, Mac console ownership, exact physical-console video, 60-second stability, and operator termination pass. Full physical acceptance still requires Mac display topology/reconnect/permission-revocation and encoder-selection evidence, plus a matching saved Hankagent project (or equivalent callable project connection) on WindowsVM.
