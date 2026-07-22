# Hank Native Remote Desktop Milestone 6 Production Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Hank Native Remote Desktop V1 operable and releasable through adaptive quality/backpressure, complete trust recovery/rotation and audit UX, abuse/resource controls, signed packaging with upgrade/rollback behavior, and repeatable Windows/macOS native acceptance evidence.

**Architecture:** Endpoint encoders adapt from browser/relay statistics without allowing the server to inspect content. HankServerside centralizes resource budgets, metrics, audit/session history, retention, and operator workflows; the dashboard completes trust bootstrap/recovery/rotation/revocation and operational session UX. Hankagent packages the privileged authority and signed user host as one versioned unit per platform with atomic upgrade, compatibility checks, and recoverable rollback.

**Tech Stack:** Existing Go observability/maintenance/store stack, Prometheus rules, React/TypeScript/WebCrypto/IndexedDB, SwiftPM/codesign/pkgbuild/productbuild/notarytool/SMAppService, .NET 8/WinUI/WiX Toolset/MSI/Authenticode/Windows Service, and physical-device acceptance scripts/runbooks.

## Global Constraints

- Begin only after Milestone 5 privileged/permission acceptance passes on physical Windows and macOS devices.
- Preserve all `desktop.v1` wire, trust, handshake, record, permission, state, and reason-code compatibility; any incompatible change requires a new protocol version.
- Adaptive quality uses encrypted endpoint/browser statistics plus relay byte/backpressure metadata; HankServerside never parses video or encrypted inner messages.
- Resource limits are explicit, measurable, tested at/over boundaries, home/session scoped, and fail closed without affecting unrelated Hank services.
- Trust setup/recovery/rotation/reset UX never exports operator private keys, silently replaces identities, or treats password reset as cryptographic recovery.
- Offline recovery secret is generated from 256 random bits, shown once with checksum, never sent to logs/analytics, and retained only by the user.
- Audit/session UX contains metadata only and never renders or persists frame, input, key, pointer, clipboard, credential, private-key, recovery-secret, or raw ciphertext content.
- Windows/macOS package versions cover service/daemon, host, GUI, IPC, and protocol compatibility as one release unit.
- Upgrade is atomic or recoverably rolled back; an old host may not attach to an incompatible new authority or vice versa.
- Packaging/notarization/signing tests may build artifacts locally, but publishing, deployment, installation on non-test devices, tagging, pushing, and rollout require separate explicit authorization.
- V1 still excludes audio, recording, viewer file transfer, multiple operators, privacy screen, local-input blocking, Linux, and WebRTC.

---

### Task 1: Adaptive quality and backpressure controller

**Files:**
- Modify: `internal/protocol/desktop_data.go`
- Modify: `internal/protocol/desktop_data_test.go`
- Create: `web/dashboard/src/desktop/qualityController.ts`
- Create: `web/dashboard/src/desktop/qualityController.test.ts`
- Modify: `web/dashboard/src/desktop/DesktopSocket.ts`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankAgentCore/Desktop/DesktopQualityController.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKitSelftest/main.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Worker/Desktop/DesktopQualityController.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/DesktopQualityTests.cs`

**Interfaces:**
- Consumes: encrypted statistics, viewer quality request, encoder controls, and relay backpressure/byte counters.
- Produces: identical bounded adaptive state machine in browser, Swift, and .NET.

- [x] **Step 1: Write failing canonical adaptation tests**

Use a shared trace with 30-second healthy period, rising RTT, 10% drops, relay backpressure, recovery, and manual quality override. Assert no more than one downgrade per five seconds, one upgrade per 20 healthy seconds, bitrate 500 Kbit/s–20 Mbit/s, fps 10–60, scale 0.5/0.75/1.0, immediate keyframe after reconfigure, and manual cap respected.

- [x] **Step 2: Define exact quality state**

```go
type DesktopQualityLevel struct { Name string; Scale float64; FPS uint8; BitrateBPS uint32 }
var DesktopQualityLevels = []DesktopQualityLevel{{"low",.5,15,1_000_000},{"balanced",.75,30,4_000_000},{"high",1,30,8_000_000},{"ultra",1,60,20_000_000}}
```

Statistics include RTT, decoder queue, decoded/dropped frames, sender queue bytes, relay backpressure count, encoded bitrate, and current dimensions; all remain content-free.

- [x] **Step 3: Implement deterministic controller in three clients**

Endpoint owns applied encode state; browser requests a maximum level and reports decoder/network health. Downgrade on sustained queue/drop/backpressure, upgrade only after healthy hysteresis, and reset conservatively to `balanced` after reconnect. A generation increment/keyframe follows scale or encoder reconfigure.

- [ ] **Step 4: Run and commit adaptation**

Run: `go test ./internal/protocol -run DesktopQuality -count=1 && make frontend-test`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter DesktopQualityTests`

Expected: PASS.

```bash
git add internal/protocol/desktop_data.go internal/protocol/desktop_data_test.go web/dashboard/src/desktop/qualityController.ts web/dashboard/src/desktop/qualityController.test.ts web/dashboard/src/desktop/DesktopSocket.ts
git commit -m "feat(remote-desktop): adapt encrypted stream quality"
```

### Task 2: Server resource budgets, metrics, alerts, and retention

**Files:**
- Modify: `internal/cloud/desktop_relay.go`
- Modify: `internal/cloud/desktop_relay_test.go`
- Modify: `internal/observability/metrics.go`
- Modify: `internal/observability/metrics_test.go`
- Modify: `internal/store/desktop_sessions.go`
- Modify: `internal/store/desktop_sessions_test.go`
- Modify: `internal/store/production_state.go`
- Modify: `internal/maintenance/lifecycle.go`
- Modify: `ops/prometheus/alerts.yml`
- Modify: `scripts/metrics-assert.sh`

**Interfaces:**
- Consumes: relay lifecycle/byte/backpressure metadata and durable sessions/events.
- Produces: production budgets, Prometheus series/alerts, and tested retention.

- [x] **Step 1: Write failing boundary and isolation tests**

Test 32 process sessions, one live session per agent, four sessions per home, 4 MiB frame, 50 MiB/s direction, 30-second idle, eight-hour hard limit, 60-second join, 90-second reconnect, slow-consumer queue cap 16 MiB, and rejection isolation from app/agent WebSockets and Files transfers.

- [x] **Step 2: Add payload-free metrics**

```text
hank_desktop_sessions{state,platform}
hank_desktop_join_total{side,outcome}
hank_desktop_reconnect_total{outcome}
hank_desktop_terminated_total{reason}
hank_desktop_relay_bytes_total{direction}
hank_desktop_relay_backpressure_total{direction}
hank_desktop_readiness{platform,check}
```

Do not include home, user, device, agent, session, IP, user-agent, display, clipboard, key, or reason text outside fixed labels.

- [x] **Step 3: Implement queue/rate/session budgets**

Use bounded per-direction queues and token buckets; reject new sessions before credential issuance when limits are exhausted. Backpressure first signals endpoint adaptation, then terminates `slow_consumer` if queue remains over cap for ten seconds.

- [x] **Step 4: Finalize retention**

Consumed/revoked credentials: 24 hours; detailed session events: 180 days; terminal session aggregate metadata: 365 days; live sessions never pruned. Pruning is transactional, counted, and content-free.

- [x] **Step 5: Add operational alerts and run tests**

Alert on repeated join-auth failures, reconnect failure ratio, relay backpressure/slow consumers, active sessions near capacity, readiness loss, and abnormal termination rate. Avoid per-user alert labels.

Run: `go test ./internal/cloud ./internal/observability ./internal/store ./internal/maintenance -run Desktop -count=1 && scripts/metrics-assert.sh`

Expected: PASS with PostgreSQL configured.

- [ ] **Step 6: Commit server hardening**

```bash
git add internal/cloud/desktop_relay.go internal/cloud/desktop_relay_test.go internal/observability/metrics.go internal/observability/metrics_test.go internal/store/desktop_sessions.go internal/store/desktop_sessions_test.go internal/store/production_state.go internal/maintenance/lifecycle.go ops/prometheus/alerts.yml scripts/metrics-assert.sh
git commit -m "feat(remote-desktop): enforce production budgets"
```

### Task 3: Browser trust bootstrap, device approval, offline recovery, rotation, and reset UX

**Files:**
- Create: `web/dashboard/src/api/desktopTrust.ts`
- Create: `web/dashboard/src/api/desktopTrust.test.ts`
- Create: `web/dashboard/src/desktop/trust/recoveryCode.ts`
- Create: `web/dashboard/src/desktop/trust/recoveryCode.test.ts`
- Create: `web/dashboard/src/desktop/trust/DesktopTrustSettings.tsx`
- Create: `web/dashboard/src/desktop/trust/DesktopTrustSettings.test.tsx`
- Create: `web/dashboard/src/desktop/trust/DesktopDeviceApproval.tsx`
- Create: `web/dashboard/src/desktop/trust/DesktopDeviceApproval.test.tsx`
- Modify: `web/dashboard/src/settings/RecoverySettings.tsx`
- Modify: `web/dashboard/src/settings/SettingsLayout.tsx`
- Modify: `web/dashboard/src/router.ts`

**Interfaces:**
- Consumes: Milestone 1 trust/recovery/rotate/reset APIs, browser identity store, and canonical crypto.
- Produces: complete administrator security workflow without private-key export.

- [x] **Step 1: Write failing security journey tests**

Cover first-root creation, random 256-bit secret, checksum/typing confirmation, show-once behavior, encrypted envelope upload, non-exportable first operator key, approve new operator, approve endpoint with fingerprint comparison, changed identity block, revoke, recover with challenge, rotate old-to-new root, reset consequences, password reset separation, and all CSRF/confirmation failures.

- [x] **Step 2: Implement human-enterable recovery code**

Encode 32 secret bytes plus four-byte SHA-256 checksum in uppercase Crockford Base32 grouped as six-character blocks. Decoder strips spaces/hyphens, rejects ambiguous/invalid characters, requires exact length/checksum, and returns 32 bytes. The secret remains in component-local memory until envelope encryption completes, then buffers/state are cleared.

- [x] **Step 3: Implement bootstrap and approval UI**

Generate the operator key non-exportable and issue the first operator certificate with exactly `operator.approve`, `endpoint.approve`, `trust.recover`, and `trust.rotate`. Generate the root key extractable only for the bootstrap ceremony, export its PKCS#8 bytes, encrypt those bytes into the recovery envelope, immediately import the same PKCS#8 as a non-exportable root signing key for IndexedDB, clear every byte buffer, and drop the extractable key reference before submitting atomic bootstrap. Then show the recovery code once with Print/Copy and exact re-entry confirmation. Device/endpoint approval shows home, device/agent, platform, fingerprint, capabilities, expiry, and signer before confirm.

- [x] **Step 4: Implement recovery/rotation/reset**

Recovery decrypts the server envelope locally, proves root possession over the single-use challenge, and enrolls a new non-exportable operator identity. Rotation creates new root/recovery envelope/replacement operator, signs the rotation proof with old root, and warns that endpoints re-enroll. Reset requires typing `reset desktop trust`, enumerates revoked identities/sessions, and never uses a password-reset result as proof.

- [ ] **Step 5: Run and commit trust UX**

Run: `make frontend-test && make frontend-check && make frontend-build`

Expected: PASS.

```bash
git add web/dashboard/src/api/desktopTrust.ts web/dashboard/src/api/desktopTrust.test.ts web/dashboard/src/desktop/trust web/dashboard/src/settings/RecoverySettings.tsx web/dashboard/src/settings/SettingsLayout.tsx web/dashboard/src/router.ts
git commit -m "feat(remote-desktop): complete trust recovery ux"
```

### Task 4: Session history, audit, readiness, and operator UX

**Files:**
- Create: `web/dashboard/src/api/desktopAudit.ts`
- Create: `web/dashboard/src/api/desktopAudit.test.ts`
- Create: `web/dashboard/src/desktop/DesktopSessionHistory.tsx`
- Create: `web/dashboard/src/desktop/DesktopSessionHistory.test.tsx`
- Create: `web/dashboard/src/desktop/DesktopReadinessCard.tsx`
- Create: `web/dashboard/src/desktop/DesktopReadinessCard.test.tsx`
- Modify: `web/dashboard/src/dashboard/AgentsPage.tsx`
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.tsx`
- Modify: `web/dashboard/src/styles.css`
- Modify: `internal/cloud/desktop_session_handlers.go`
- Modify: `internal/cloud/desktop_session_handlers_test.go`

**Interfaces:**
- Consumes: session/events/readiness/metrics APIs.
- Produces: operational preflight, live status, metadata-only history, reason guidance, and safe termination.

- [x] **Step 1: Write failing end-user workflow tests**

Test capability/readiness preflight, identity trust required, one-operator conflict, active/reconnecting status, local indicator status, Windows secure support, macOS permissions, latency/quality/backpressure, terminate confirmation, paginated history, stable reason guidance, and no content fields.

- [x] **Step 2: Implement preflight and history responses**

Return fixed readiness keys, effective permissions, trusted fingerprint, platform, service/daemon/host/indicator state, and active session summary. History response includes times, duration, states, actor type, reason, epoch count, aggregate bytes, and permission names only.

- [x] **Step 3: Implement practical operator UI**

Agent detail shows Remote Desktop readiness and active operator state before opening viewer. Viewer shows applied quality, latency, reconnect deadline, hard expiry, indicator status, and End Session. History maps every stable reason to concise action; unknown internal errors remain generic.

- [ ] **Step 4: Run and commit operational UX**

Run: `go test ./internal/cloud -run Desktop -count=1 && make frontend-test && make frontend-check && make frontend-build`

Expected: PASS.

```bash
git add internal/cloud/desktop_session_handlers.go internal/cloud/desktop_session_handlers_test.go web/dashboard/src/api/desktopAudit.ts web/dashboard/src/api/desktopAudit.test.ts web/dashboard/src/desktop web/dashboard/src/dashboard/AgentsPage.tsx web/dashboard/src/styles.css
git commit -m "feat(remote-desktop): add operator session history"
```

### Task 5: Windows signed package, upgrade, and rollback

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/installer/HankAgent.Installer.wixproj`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/installer/Package.wxs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/installer/DesktopService.wxs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/scripts/build-installer.ps1`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/scripts/test-upgrade.ps1`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/scripts/build-unsigned.ps1`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/Directory.Packages.props`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/InstalledManifestTests.cs`

**Interfaces:**
- Consumes: GUI, service, host, IPC version, signer identity, and installed-file manifest.
- Produces: versioned MSI build, service lifecycle, atomic upgrade, and recoverable rollback proof.

- [x] **Step 1: Write failing installed-manifest and compatibility tests**

Require GUI/service/host version equality, IPC major equality, SHA-256 for each executable, expected signer metadata, service account/start/recovery settings, ProgramData ACL, no plaintext token, and host path matching service validation manifest.

- [x] **Step 2: Build one WiX package**

WiX installs GUI, LocalSystem service, signed host, dependencies, Start Menu entry, service recovery policy, and protected ProgramData directories. First privileged activation performs a mutually authenticated one-time migration from the current user's Windows Credential Vault into the service DPAPI store, verifies service connection, then deletes the old reusable token; failure leaves the old token intact and the GUI worker stopped to prevent duplicate connections. Major upgrade stops sessions/service, verifies no active helper, installs atomically, restarts service, and preserves DPAPI/CNG identity/token material.

- [x] **Step 3: Add upgrade/rollback test script**

Build versions N and N+1, install N in disposable Windows VM, enroll test identity, run synthetic/native smoke, upgrade to N+1, verify identity/session readiness, force an install failure, verify MSI rollback restores N, then uninstall and verify binaries/services removed while user-requested credential retention follows explicit flag.

- [ ] **Step 4: Run packaging gate**

Run: `powershell -File scripts/build-installer.ps1 -Configuration Release -UnsignedTestBuild`

Run in disposable VM: `powershell -File scripts/test-upgrade.ps1 -OldMsi artifacts\test\HankAgent-1.0.0.msi -NewMsi artifacts\test\HankAgent-1.1.0.msi`

Expected: tests/build/upgrade/rollback/uninstall PASS. Paths are supplied by the local packaging harness; no artifact is published.

- [ ] **Step 5: Commit Windows packaging**

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add HankAgent-Windows/installer HankAgent-Windows/scripts/build-installer.ps1 HankAgent-Windows/scripts/test-upgrade.ps1 HankAgent-Windows/scripts/build-unsigned.ps1 HankAgent-Windows/tests/HankAgent.Tests/InstalledManifestTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "build(remote-desktop): package windows service and host"
```

### Task 6: macOS signed package, daemon registration, upgrade, and rollback

**Files:**
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/scripts/build-app.sh`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/scripts/build-pkg.sh`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/scripts/notarize-pkg.sh`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/scripts/test-upgrade.sh`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Resources/com.hank.desktop-daemon.plist`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Resources/DesktopDaemon.entitlements`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Resources/HankAgent.entitlements`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Tests/PackagingSelftest/main.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Package.swift`

**Interfaces:**
- Consumes: signed app/daemon, SMAppService registration, entitlements, designated requirements, and Keychain continuity.
- Produces: signed/notarizable pkg, compatibility manifest, upgrade, rollback, and uninstall proof.

- [x] **Step 1: Write failing bundle/entitlement/requirement tests**

Require app/daemon version equality, IPC major equality, exact bundle/daemon IDs, launch daemon plist, code-sign designated requirements, required Screen Recording/Accessibility explanation strings, daemon root ownership, host/daemon mutual requirement values, and no broad entitlement unrelated to V1.

- [ ] **Step 2: Build signed app and daemon package**

`build-app.sh` embeds helper/daemon metadata and signs nested code inside-out. `build-pkg.sh` uses `pkgbuild`/`productbuild` to install app and daemon support files, register privileged service through supported `SMAppService` flow, preserve Keychain endpoint/token items, and verify signatures before activation. First privileged activation performs a mutually authenticated one-time transfer from the existing user Keychain item to the daemon-only Keychain item, proves the daemon control connection, then deletes the old reusable item; failure keeps the old item but leaves the GUI worker stopped to prevent duplicate connections.

- [x] **Step 3: Add notarization without secret leakage**

`notarize-pkg.sh` reads notary profile name from argument/Keychain, runs `notarytool submit --wait`, staples, and verifies Gatekeeper. It never prints credentials. Unsigned local test mode skips submission but still validates structure.

- [x] **Step 4: Add upgrade/rollback test**

In disposable macOS VM, install N, enroll, grant test permissions, run smoke, upgrade N+1, verify Keychain identity and permissions/readiness, force installer failure, restore N through saved signed package, verify daemon/host compatibility, then uninstall test package.

- [ ] **Step 5: Run and commit macOS packaging**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build -c release && swift run packaging-selftest && ./scripts/build-pkg.sh --unsigned-test-build`

Expected: PASS; notarization runs only with separately authorized credentials.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Package.swift scripts Resources Tests/PackagingSelftest
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "build(remote-desktop): package mac daemon and host"
```

### Task 7: Automated abuse, longevity, compatibility, and native acceptance

**Files:**
- Create: `scripts/remote-desktop-load-validation.sh`
- Create: `scripts/remote-desktop-acceptance.sh`
- Create: `docs/remote-desktop/v1-acceptance.md`
- Create: `docs/remote-desktop/v1-operations.md`
- Modify: `docs/demo-validation.md`
- Modify: `RELEASE.md`

**Interfaces:**
- Consumes: all V1 milestones and package artifacts.
- Produces: repeatable release evidence and operator runbook.

- [x] **Step 1: Add server load/abuse harness**

Exercise credential replay, wrong side/session/epoch, 33rd process session, fifth home session, duplicate operator, 4 MiB+1 frame, 50 MiB/s+1 rate, 16 MiB+1 queue, idle, reconnect expiry, hard expiry, slow consumer, revocation, server restart, malformed frames, and unrelated API/Files health during rejection.

- [x] **Step 2: Add eight-hour longevity harness**

Run native video/control/clipboard with adaptive quality for eight hours, include three brief reconnects, display changes, lock/unlock, quality degradation/recovery, and local input. Assert hard expiry terminates at authorization+8h and resources return to baseline.

- [ ] **Step 3: Run the complete 11-point native acceptance matrix**

On packaged Windows and macOS builds prove exact physical console, concurrent local/remote input, view-only, clipboard directions, fresh-key reconnect, local indicator/termination, three termination actors, relay opacity, content-free audit, Windows elevated/UAC, and macOS permission loss/recovery.

- [ ] **Step 4: Run compatibility/upgrade/rollback matrix**

Test current browser/server with current package, one supported prior package under compatible `desktop.v1`, rejected incompatible IPC major, upgrade N→N+1, rollback N+1→N, service/daemon restart, server restart, identity rotation, and trust reset/re-enrollment.

- [ ] **Step 5: Run full repository release gates**

Run: `make tidy && make fmt && go test ./... && make build && make frontend-test && make frontend-check && make frontend-build && make migrate-status && make schema-drift-check && scripts/doctor.sh`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build -c release && swift run hankkit-selftest && swift run desktop-daemon-selftest && swift run packaging-selftest`

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj -c Release && powershell -File scripts/build-installer.ps1 -Configuration Release -UnsignedTestBuild`

Expected: all PASS; PostgreSQL, Windows, macOS, signing, and physical-device gates must be recorded honestly and no skip satisfies release readiness.

- [x] **Step 6: Document operations and evidence**

Runbook covers readiness, metrics/alerts, active-session termination, identity approval/revocation/rotation/reset, recovery-code loss, permission/service/daemon/helper failure, backpressure, upgrades, rollback, log redaction, and incident evidence collection without content.

- [ ] **Step 7: Commit final V1 release evidence**

```bash
git add scripts/remote-desktop-load-validation.sh scripts/remote-desktop-acceptance.sh docs/remote-desktop/v1-acceptance.md docs/remote-desktop/v1-operations.md docs/demo-validation.md RELEASE.md
git commit -m "test(remote-desktop): complete v1 acceptance gate"
```

## Milestone 6 Exit Criteria

- [x] Adaptive quality/backpressure behaves identically across browser, Swift, and .NET test traces and remains content opaque to the server.
- [x] Resource/abuse limits, metrics, alerts, isolation, retention, and failure reasons pass boundary/load tests.
- [x] Trust bootstrap, device/endpoint approval, recovery, rotation, revocation, unexpected identity change, and reset UX pass security journeys.
- [x] Session readiness/history/audit UX is practical, complete, and content-free.
- [ ] Windows and macOS signed-package structures, service/daemon registration, compatibility, upgrade, rollback, and uninstall pass in disposable test devices.
- [ ] Eight-hour longevity and all 11 native acceptance requirements pass on packaged physical-console builds.
- [ ] Complete HankServerside/Hankagent release gates pass with no unreported database, signing, packaging, or device skips.
- [x] No publish, deployment, tag, push, or production rollout occurred without separate explicit authorization.

## Execution blockers recorded 2026-07-22

- Commit steps remain unchecked because this task did not authorize commits.
- Database migration verification passed against Hank demo's isolated disposable PostgreSQL database: migrations `000001` through `000021` applied, strict migration status passed, direct schema drift passed, and the database and tunnel were removed afterward. A separate isolated PostgreSQL test passed after 1,000 late terminal acknowledgements, proving persisted history remained bounded to the authoritative events while relay revocation still ran; its database and tunnel were also removed. The deep Compose comparison and full doctor remain unchecked locally because Docker and Docker Compose are unavailable; `scripts/doctor.sh` reported exactly those two missing-command failures while both environment files existed with mode `0600`.
- Windows portable validation now passes all 160 .NET tests with zero skips. MSI binding and disposable-VM upgrade/rollback/uninstall remain unchecked because `wix.exe` is Windows-only and rejects the macOS host; the connected Windows VM artifact/upgrade gate must run from a Windows-hosted worktree.
- Developer ID signing, notarization submission, signed-package VM installation, and rollback remain unchecked because release credentials and installation/submission authority were not granted. The local macOS Release build, three selftest products, and ad-hoc signed unsigned-test pkg structure pass.
- One-time first-transfer/re-enrollment coordinators now enforce the same ordered transaction on Windows and macOS from a zero-configuration privileged service/daemon: stop the duplicate GUI worker, atomically create provisional machine configuration and credential, generate a non-exportable endpoint identity, install the exact server-approved certificate, validate the exact readiness receipt, create the configured marker only at commit, and only then retire the legacy reusable credential. Failure stops the provisional authority, removes credential/readiness/configuration state, and restores the legacy GUI worker for a clean retry. Physical DPAPI/Keychain continuity still requires the installed-device gate.
- Contract-only load and acceptance scripts require exact `go test -list` discovery and exact `go test -json` RUN/PASS events for every mapped portable scenario. Integration/physical-only rows are recorded explicitly as not run, and every receipt is marked `native_evidence:false`; contract receipts do not satisfy native acceptance.
- Native driver receipts are structurally decoded with duplicate-key rejection and reserved-field conflict checks before canonical metadata is written. Relay pressure is delivered over the authenticated agent control plane, causes an immediate endpoint encoder downgrade, and uses a resettable statistics-window count. Reconnect relay claims carry the one authoritative server deadline so a late join cannot start a second 90-second timer.
- The Windows GUI/service named-pipe and macOS GUI/daemon XPC first-transfer conformers are wired into production startup. Both authenticate their privileged peer, transfer the reusable credential into the machine store, wait for changed endpoint approval and an actual registered server connection/readiness receipt, and only then allow legacy retirement. Physical DPAPI/Keychain continuity remains part of the device gate.
- The macOS daemon resolves its credential group from the signed daemon-only entitlement with no application-identifier, launch environment, or ad-hoc runtime fallback. The forced-upgrade rollback fixture is a dedicated component package with a verified failing postinstall rather than an unreachable script injection into a scriptless package.
- Eight-hour longevity, the eleven-point physical-console matrix, and packaged compatibility/upgrade/rollback matrices remain unchecked pending packaged physical Windows/macOS test devices and duration.
