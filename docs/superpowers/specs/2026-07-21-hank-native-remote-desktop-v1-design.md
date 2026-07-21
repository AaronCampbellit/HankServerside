# Hank Native Remote Desktop V1 Design

**Status:** Approved design, pending written-spec review
**Date:** 2026-07-21
**Owning platform:** HankServerside
**Endpoint implementation repository:** Hankagent

## Purpose

Add a native browser-based Hank Remote Desktop viewer for authorized administrators. The viewer shadows the exact physical console currently visible to the local Windows or macOS user. It does not create an RDP session, virtual desktop, secondary login, or disconnected user environment.

HankServerside owns the shared authorization, persistence, protocol, relay, audit, and browser-viewer contracts. Hankagent owns the Windows and macOS service, helper, capture, encoding, input, clipboard, permission, and local-indicator implementations.

The current product remains one self-hosted deployment for one home. V1 does not add MSP roles, multi-home routing, or SaaS assumptions. Home, operator, endpoint, and session boundaries remain explicit so a future version can extend the authorization model without replacing the wire, identity, or persistence contracts.

## V1 Scope

V1 includes:

- Windows and macOS on one shared contract and milestone sequence
- active physical-console shadowing
- one remote operator per endpoint
- unattended access for authorized Hank administrators
- no local approval prompt; the persistent local indicator is always required
- view-only and control modes
- H.264 video
- end-to-end encrypted desktop, input, and clipboard payloads
- a binary WebSocket relay hosted behind an internal HankServerside relay interface
- mouse, keyboard, special-key, and text-clipboard support
- monitor selection, scaling, fullscreen, quality, and latency status
- a 60-second initial join window
- a 90-second reconnect window with fresh credentials and keys
- an eight-hour hard session maximum
- termination by browser, server, or endpoint
- a small persistent local indicator while remote access is active
- Windows elevated applications, UAC secure desktop, and privileged input
- macOS Screen Recording and Accessibility readiness with guided setup
- complete metadata auditing without captured content
- a Home Trust Root, operator-device identities, endpoint identities, offline recovery, revocation, and rotation

V1 excludes:

- audio
- session recording
- file transfer inside the viewer
- multiple remote operators
- privacy screen or local-input blocking
- Linux
- WebRTC, STUN, TURN, or direct peer-to-peer transport
- protection against a compromised Hank Server that can replace the browser viewer JavaScript

Files continue to move through Hank's existing Files interface.

## Delivery Strategy

V1 uses a contract-first vertical slice. HankServerside and both endpoint platforms first prove the complete encrypted session using deterministic synthetic H.264 providers. Native Windows and macOS providers then replace the synthetic sources behind the same interfaces.

This prevents capture or input implementation details from defining the security, relay, browser, or persistence contracts. Windows and macOS remain on the same protocol and milestone contract even when their native work is performed sequentially.

## System Architecture

### HankServerside

HankServerside owns:

- authenticated session APIs
- administrator and permission enforcement
- home, operator, and endpoint scoping
- trust-root and identity public metadata
- encrypted recovery envelopes
- session persistence and state transitions
- single-use browser and agent join credentials
- revocation, expiration, and reconnect authorization
- agent control-plane negotiation
- the browser viewer
- the internal relay interface and its first in-process implementation
- audit records and session metrics

The relay only matches authorized sockets and forwards opaque binary ciphertext. The relay does not parse encrypted inner messages, derive session keys, decode video, inspect input, or inspect clipboard content.

### Browser viewer

The browser viewer owns:

- operator-device identity access
- ephemeral key generation and signed handshake verification
- session-key derivation and key rotation
- binary inner-message encryption and decryption
- H.264 decoder configuration and playback
- display scaling, fullscreen, and monitor selection
- view/control mode
- normalized pointer and physical keyboard events
- explicit browser clipboard gestures
- special-key commands
- connection, latency, permission, and secure-state presentation
- reconnect and termination controls

The viewer is a dedicated dashboard route opened from an online capable agent's detail page.

### Hankagent

Hankagent owns a shared `DesktopSessionCoordinator` and `DesktopHost` contract. The coordinator owns authorization validation, identity use, data-plane connectivity, helper supervision, session state, and termination. A platform Desktop Host owns only capture, encoding, input, clipboard, display state, permission state, and the local indicator.

The Desktop Host never receives a reusable Hank Agent credential.

## Threat and Trust Model

### Protected threats

V1 protects against:

- a passive relay reading desktop, input, or clipboard payloads
- a compromised or curious relay component reading those payloads
- stolen or replayed single-use join credentials
- cross-home, cross-agent, or cross-operator session confusion
- ciphertext tampering, replay, reordering, or truncation
- an untrusted local process impersonating a Desktop Host
- a revoked operator device or endpoint identity opening a new session
- plaintext private keys or recovery material stored by Hank Server

Hank Server retains authority to allow, deny, revoke, expire, or terminate a session. It sees session metadata, public keys, ciphertext sizes, timing, byte counts, source network metadata, and state transitions.

### Explicit limitation

V1 does not claim protection from a Hank Server compromise that can replace the browser viewer's JavaScript. Malicious viewer code could exfiltrate keys after the browser decrypts them. Defending that boundary requires an independently distributed trusted viewer, such as a signed native client, browser extension, or separately secured viewer origin.

## Cryptographic Design

### Algorithms

V1 uses algorithms available across browser WebCrypto, Go, Swift CryptoKit, and Windows CNG/.NET:

- ECDSA P-256 with SHA-256 for identity signatures
- ephemeral ECDH P-256 for each initial join and reconnect
- HKDF-SHA-256 for directional key derivation
- AES-256-GCM for encrypted inner messages
- SHA-256 for fingerprints, transcript digests, and credential hashes

Identity public keys use DER SubjectPublicKeyInfo encoding. Ephemeral ECDH public keys use uncompressed P-256 points. All binary values transmitted through JSON control messages use unpadded base64url.

### Home Trust Root

The first administrator device creates a Home Trust Root signing key. Hank Server stores its public key and fingerprint. The root private key is never stored in plaintext by Hank Server.

Trusted administrator devices receive operator identity certificates rooted in the Home Trust Root. Operator certificates state the home, device identifier, public key, allowed trust operations, creation time, and expiration.

Endpoint identity keys are generated locally by Hankagent. Endpoint certificates bind the public key to the home, agent ID, platform, and enrollment event. A trusted administrator device whose operator certificate includes endpoint-approval authority signs the endpoint certificate; verifiers validate that delegated signature through the operator certificate to the Home Trust Root. The endpoint private key remains in the platform credential store.

### Private-key storage

- Windows uses TPM-backed CNG keys where available and DPAPI-protected machine storage as fallback.
- macOS uses Keychain access controls and Secure Enclave-backed signing keys where available.
- browser operator keys use non-exportable WebCrypto keys when supported. Operator private keys are never transferred; each new trusted device generates a new identity and is separately approved.

### Recovery

Home Trust Root recovery uses a random 256-bit offline recovery secret shown once during setup with a human-enterable encoding and checksum. HKDF-SHA-256 derives an AES-256-GCM recovery key from that secret and the home/root generation context. The key encrypts a versioned root recovery envelope stored by Hank Server. The server cannot decrypt the envelope, and authenticated decryption detects replacement or corruption.

An existing trusted administrator device can approve a new operator device without using the recovery secret. If every trusted administrator device is lost, the offline recovery secret restores the root onto a new device. Password reset alone never resets or silently replaces the trust root.

If both trusted devices and the recovery secret are lost, the only supported path is an explicit cryptographic reset. Reset revokes all operator and endpoint certificates and requires re-enrollment. The UI must describe the consequence before confirmation.

Recovery, new-device approval, revocation, rotation, unexpected identity replacement, and cryptographic reset are audited. A changed known identity is blocked until explicitly approved.

### Session handshake

The browser generates an ephemeral ECDH key and signs a deterministic handshake transcript with its operator identity. The transcript includes:

- protocol label `Hank Desktop Handshake v1`
- home ID
- session ID
- agent ID
- operator user and operator-device IDs
- requested permissions
- browser ephemeral public key
- join and hard-expiration timestamps
- key epoch

The agent validates the operator certificate, transcript signature, home, agent, permissions, timestamps, and revocation state. It adds its ephemeral ECDH key, endpoint certificate fingerprint, and platform readiness, then signs the completed transcript with the endpoint identity.

The transcript uses a specified length-prefixed binary field sequence before SHA-256 hashing. Implementations do not sign ordinary JSON serialization.

After both signatures verify, browser and agent derive an ECDH shared secret. HKDF uses the transcript digest as salt and distinct labels for browser-to-agent and agent-to-browser AES keys and nonce prefixes. Hank Server relays public handshake material but cannot derive the shared secret.

Every reconnect increments the key epoch and performs a fresh signed ephemeral exchange. Prior epoch keys are zeroized after the new epoch activates.

### Encrypted record layer

Every data-plane record has a small outer header containing protocol version, key epoch, direction, sequence number, and ciphertext length. The outer header is authenticated as AES-GCM associated data. The encrypted body contains the inner message type and payload.

Each direction uses a unique nonce prefix plus a monotonically increasing 64-bit sequence. Because each WebSocket direction is ordered, the receiver requires the exact next sequence number. Duplicate, skipped, reversed, or authenticated-invalid records fail closed. Sequence state never survives a key epoch.

## Authorization and Permissions

Only authenticated home administrators may create Remote Desktop sessions in V1. The server enforces home membership, administrator role, target agent ownership, agent online state, advertised capabilities, identity validity, and the one-operator limit.

Permissions remain distinct even when enabled together for internal administrators:

- `desktop.view`
- `desktop.control`
- `desktop.clipboard.read`
- `desktop.clipboard.write`
- `desktop.elevate`
- `desktop.secure_desktop`
- `desktop.unattended`

`desktop.record` exists only as a reserved future permission and is never granted or implemented in V1.

Cookie-authenticated browser writes require CSRF protection. Browser and agent join credentials are different, short-lived, single-use, randomly generated secrets stored only as server-side hashes. Credentials never appear in URLs.

The browser join credential is delivered as a Secure, HttpOnly, SameSite, path-limited cookie. The agent credential is delivered through the authenticated control plane and used in a data-plane authorization header.

## Session API

The V1 HTTP surface is:

```text
POST /v1/agents/{agentID}/desktop-sessions
GET  /v1/desktop-sessions/{sessionID}
POST /v1/desktop-sessions/{sessionID}/reconnect
POST /v1/desktop-sessions/{sessionID}/terminate
GET  /v1/desktop-sessions/{sessionID}/events
```

Trust administration is home-scoped:

```text
GET  /v1/home/desktop-trust
POST /v1/home/desktop-trust/operator-devices
POST /v1/home/desktop-trust/operator-devices/{deviceID}/revoke
POST /v1/home/desktop-trust/endpoints/{agentID}/approve
POST /v1/home/desktop-trust/endpoints/{agentID}/revoke
POST /v1/home/desktop-trust/recovery
POST /v1/home/desktop-trust/reset
```

Route handlers use the established Hank authentication, singleton-home membership, administrator, CSRF, confirmation, and audit patterns.

## Persistent Model

Versioned migrations add narrowly scoped tables:

### `desktop_trust_roots`

Stores home ID, root public key, fingerprint, encrypted recovery envelope, algorithm/version, creation time, rotation time, and reset generation.

### `desktop_identities`

Stores identity ID, home ID, identity type (`operator_device` or `endpoint`), user ID or agent ID, public key, certificate, fingerprint, capabilities, creation/expiration time, revocation time, and trust-root generation.

### `desktop_sessions`

Stores session ID, home ID, agent ID, operator user/device IDs, requested/effective permissions, state, key epoch, request/join/active/reconnect/expiration/termination timestamps, termination reason, source metadata hashes, and aggregate byte counts.

### `desktop_join_credentials`

Stores session ID, side, credential hash, expiration, key epoch, consumed time, and revocation time. Plaintext credentials are never persisted.

### `desktop_session_events`

Stores ordered metadata-only session events with event type, actor, time, severity, reason code, and redacted structured metadata.

Foreign keys preserve home, user, agent, identity, and session ownership. Important session-state, side, identity-type, and permission constraints are enforced in the database. Content payloads have no persistence column.

## Session State Machine

Durable states are:

```text
requested -> offered -> agent_ready -> joining -> active
active -> reconnecting -> active
requested/offered/agent_ready/joining/reconnecting -> denied|failed|expired|terminated
active -> expired|terminated|failed
```

Only the owning server transition functions update state. Terminal states cannot return to a live state.

The initial join window is 60 seconds. A data-plane interruption enters `reconnecting` for at most 90 seconds. The hard session maximum is eight hours from authorization and is not extended by reconnect. A reconnect preserves the authorization session but requires new browser and agent credentials, new ephemeral keys, and a new key epoch.

Server restart may restore durable authorization and reconnect eligibility. It never restores keys, sequence counters, plaintext credentials, sockets, video, input, or clipboard state.

## Control Plane

The existing Hank Agent WebSocket remains the control plane. It carries low-frequency signed handshake material, authorization metadata, readiness, lifecycle, and termination—not video frames or high-frequency input.

Agent capabilities include:

```text
desktop.status
desktop.session.open
desktop.session.close
desktop.view
desktop.control
desktop.multi_monitor
desktop.clipboard.read
desktop.clipboard.write
desktop.secure_desktop
```

Control commands include:

```text
desktop.status
desktop.session.offer
desktop.session.activate
desktop.session.close
desktop.session.set_control
desktop.session.set_display
desktop.session.set_quality
```

Asynchronous events include:

```text
desktop.session.ready
desktop.session.connected
desktop.session.disconnected
desktop.display.changed
desktop.permission.required
desktop.secure_desktop.entered
desktop.secure_desktop.exited
desktop.session.stats
desktop.session.error
desktop.session.terminated
```

Protocol messages are versioned and validated in HankServerside before routing. The agent independently validates session identity, scope, permissions, timestamps, and signatures.

## Data Plane and Relay

The first data plane uses dedicated binary WebSockets:

```text
Browser -- TLS WebSocket -- Hank relay -- TLS WebSocket -- Hankagent
```

Browser and agent use separate endpoints and credentials. The relay interface owns:

- credential validation and consumption
- side and key-epoch matching
- one connection per side
- join timeout
- connection and session limits
- frame-size, bandwidth, idle, and duration limits
- bidirectional opaque forwarding
- byte and lifecycle metrics
- closing both sides on revocation, expiration, protocol-limit failure, or disconnect outside the reconnect window

The first implementation runs inside the current Hank Server process. The interface allows a future `hank-relay` service without changing browser, agent, database, handshake, or encrypted record formats.

## Inner Data-Plane Protocol

Encrypted inner message families are:

- codec configuration
- encoded video access unit
- pointer position and shape
- display inventory and selected display
- keyboard event
- pointer button, movement, and wheel event
- clipboard offer and text payload
- control-mode change
- quality request and applied quality
- ping, pong, and statistics
- secure-state and permission state
- graceful termination

All messages carry a version and bounded payload length. Unknown optional message types may be ignored only when their envelope marks them optional. Unknown required message types terminate the epoch.

### Video

V1 uses H.264 with explicit codec configuration messages followed by length-prefixed encoded access units. The protocol identifies codec, profile, level, coded dimensions, display dimensions, rotation, keyframe status, presentation timestamp, and stream generation.

The browser uses WebCodecs through a decoder abstraction when available and a Media Source Extensions H.264 fallback through the same abstraction on supported browsers. If neither decoder path is available, the viewer reports the browser as unsupported before creating a session.

Synthetic providers produce deterministic moving test content and known frame hashes/markers. Native providers use platform hardware encoding where practical and a software fallback where required.

### Input

Pointer coordinates are normalized to the selected display. Messages carry display ID, coordinates, buttons, wheel deltas, and event time. The endpoint maps them to the current physical display geometry.

Keyboard messages use physical key identifiers, scan/code values, location, repeat state, and explicit modifier state. Text insertion is not substituted for physical key events. Special keys use named bounded commands and require the corresponding permission.

Input is accepted only while control mode is active, the viewer is focused, the session epoch is current, and endpoint readiness permits input. Local input remains enabled.

### Clipboard

Clipboard synchronization is text-only and separately permissioned by direction. Browser security restrictions are honored through explicit copy/paste gestures where required. Clipboard data exists only in endpoint/browser memory for the active operation and is never included in logs, audit metadata, or server persistence.

## Browser Experience

An online capable agent detail page exposes a Remote Desktop action to administrators. The dedicated viewer provides:

- the remote display
- scaling and fullscreen
- monitor selection
- view-only/control toggle
- quality and latency status
- clipboard controls
- special-key menu
- immediate End Session
- clear local-permission, secure-desktop, locked, unavailable, and reconnecting overlays

The viewer never claims control succeeded until the endpoint acknowledges it. Trust-root setup, device approval, unexpected identity changes, revocation, recovery, and cryptographic reset use explicit security language and confirmation.

## Windows Endpoint Design

Windows V1 adds:

- a Windows service that owns Hank credentials, authorization, endpoint identity, and the data-plane connection
- `HankDesktopHost.exe` in the active console user session
- authenticated, bounded, versioned local IPC between service and host
- host launch and supervision across active-user changes
- Windows Graphics Capture or Desktop Duplication for the normal desktop
- Media Foundation H.264 encoding with software fallback
- `SendInput`-style normal interactive input
- text clipboard synchronization
- a persistent visible local indicator
- a privileged service bridge for elevated applications, UAC secure desktop, protected input, and supported special-key operations

The service validates the helper process identity, active session, IPC peer, message bounds, and session authorization. Failure to display the indicator, validate IPC, or maintain the correct active-user session stops capture and input.

Secure-desktop transitions are explicit provider changes. If secure capture or input cannot be established, the endpoint blocks input and reports `desktop.secure_desktop` unavailable rather than continuing with stale frames.

## macOS Endpoint Design

macOS V1 adds:

- a privileged daemon that owns Hank credentials, authorization, endpoint identity, and the data-plane connection
- a user-session Hank Desktop Host
- authenticated XPC/local IPC with code-sign and audit-token validation
- ScreenCaptureKit capture
- VideoToolbox H.264 encoding with software fallback
- Accessibility/CGEvent input
- text clipboard synchronization
- a persistent visible local indicator
- Screen Recording and Accessibility readiness and guided setup
- host launch and supervision across console-user changes

The user-session host receives session-scoped material only. Permission loss, lock-screen state, console-user change, or helper failure pauses capture and blocks input while reporting a precise state. Stale frames are never presented as live.

## Failure Handling

Remote Desktop fails closed:

- authorization or readiness failure prevents capture startup
- an expired, consumed, wrong-side, or revoked credential is rejected
- signature, certificate, identity, transcript, AEAD, replay, or sequence failure terminates the epoch and records a security audit event
- join credentials cannot be retried or reused
- key or identity changes require explicit reauthorization
- relay frame, bandwidth, idle, connection, and duration limits are enforced without parsing ciphertext
- losing the local indicator stops capture and input
- losing Desktop Host or privileged IPC validation stops capture and input
- permission loss pauses capture and blocks input
- clipboard failure is non-fatal and cannot expand view/control permissions
- a helper crash receives one supervised restart attempt; repeated failure terminates the session
- reconnect outside 90 seconds terminates the session
- revocation, identity rotation, hard expiration, or explicit termination closes both sockets and zeroizes in-memory keys

Browser, server, and endpoint surface stable reason codes and human-readable guidance. Internal errors do not expose tokens, keys, private file content, desktop content, clipboard content, or implementation stack traces.

## Local Indicator

Every active or reconnecting session displays a small persistent Hank remote-access indicator in the physical console session. It identifies Hank Remote Desktop and provides an endpoint-side termination action.

The indicator cannot be disabled by ordinary session configuration. Failure to create it blocks session activation. If it disappears unexpectedly, the endpoint immediately pauses capture and input, attempts one supervised restart, and terminates the session if the indicator is not restored within 10 seconds.

## Auditing and Metrics

Audit events cover:

- session requested
- authorization allowed or denied
- agent offered and ready
- browser and agent joined
- handshake or identity failure
- view/control mode changed
- display or quality changed
- reconnect started, succeeded, or expired
- permission required, granted, or lost
- secure desktop entered or exited
- local or server termination
- session failure or expiration
- identity approved, revoked, rotated, recovered, or reset

Audit metadata may include session, home, operator, operator-device, agent, permission, state, reason, connection type, source IP/user-agent hashes, timestamps, duration, key epoch, and aggregate byte counts.

Hank never logs or stores screen contents, video frames, keys, keystrokes, pointer payloads, or clipboard contents. Audit metadata passes through the existing redaction layer.

Operational metrics include active/reconnecting sessions, join/reconnect outcomes, termination reasons, opaque bytes relayed, relay backpressure, frame drops reported by endpoints/viewers, latency, and platform readiness counts.

## Delivery Milestones

### Milestone 1: Shared foundation

- migrations and store interfaces
- authorization and session APIs
- trust-root, identity, recovery, revocation, and rotation contracts
- audit model
- control-plane protocol
- relay interface and limits
- shared cryptographic and protocol fixtures

### Milestone 2: Synthetic end-to-end slice

- browser viewer shell
- browser and agent key exchange and encrypted record layer
- in-process binary relay
- deterministic synthetic H.264 providers on Windows and macOS
- encrypted video, input, clipboard, statistics, termination, and reconnect
- proof that Hank Server cannot decrypt known payloads

### Milestone 3: Native viewing

- active-console discovery on both platforms
- native capture and H.264 encoding
- display inventory, changes, scaling, quality, statistics, and indicator

### Milestone 4: Native control

- normalized pointer and physical keyboard control
- view-only/control mode
- clipboard
- monitor selection
- special-key paths

### Milestone 5: Privileged and permission behavior

- Windows elevated applications, UAC secure desktop, session switching, and privileged input
- macOS permission readiness, guided setup, lock-screen transitions, and helper supervision

### Milestone 6: Production hardening

- adaptive quality and backpressure
- recovery and rotation UX
- packaging, installation, upgrade, and rollback behavior
- resource and abuse limits
- complete audit and operational UX
- native-device acceptance passes

Each milestone passes its cross-repository gate before the next begins. Each milestone receives its own implementation plan derived from this design.

## Verification Strategy

### Shared protocol and cryptography

- identical golden handshake, certificate, transcript, HKDF, nonce, AES-GCM, record, and error vectors in Go, TypeScript, Swift, and C#
- tampering, replay, wrong-direction, wrong-epoch, wrong-identity, expiration, rotation, and revocation tests
- compatibility tests for optional and required inner messages

### HankServerside

- store and migration tests, strict migration status, and schema drift checks
- authorization, administrator, home/agent/operator scope, CSRF, and confirmation tests
- single-use credential and one-operator tests
- state-machine and terminal-state tests
- relay pairing, size, bandwidth, idle, backpressure, disconnect, restart, reconnect, revocation, and termination tests
- audit completeness and redaction tests
- proof tests demonstrating that known plaintext cannot be recovered from server-held session state or relayed ciphertext

### Browser viewer

- key and record-layer tests using shared vectors
- H.264 configuration, keyframe, generation, scaling, and decoder recovery tests
- pointer, keyboard, clipboard, view/control, monitor, fullscreen, and special-key tests
- permission, secure-state, reconnect, identity-change, and termination UI tests
- synthetic full-flow browser integration tests

### Hankagent

- portable Desktop Host contract tests with fake providers
- endpoint identity storage and signed-handshake tests
- local IPC identity, authorization, bounds, and disconnect tests
- helper supervision and local-indicator fail-closed tests
- native capture, encode, input, clipboard, permission, and display-change tests
- Windows and macOS packaging and upgrade tests

### Native acceptance

Windows and macOS acceptance must prove:

1. The browser shows the exact physical console visible locally.
2. Local and remote pointer and keyboard input remain active together.
3. View-only mode prevents input.
4. Text clipboard follows independent read/write permissions.
5. A brief interruption reconnects with fresh credentials and keys.
6. The local indicator remains visible and can terminate the session.
7. Browser, server, and endpoint termination stop capture and input immediately.
8. The relay cannot decrypt known desktop, input, or clipboard payloads.
9. Audit history is complete and contains no captured content.
10. Windows can operate elevated applications and supported UAC secure-desktop flows.
11. macOS clearly reports and recovers from Screen Recording or Accessibility permission loss.

## Security Impact

Remote Desktop is a new highest-risk capability. It adds administrator-only authorization, trust identities, short-lived join credentials, an end-to-end encrypted data plane, local privileged helpers, input injection, clipboard access, and detailed audit requirements. The design keeps reusable Hank credentials out of Desktop Hosts and content out of Hank Server persistence and logs.

## Database Impact

V1 requires versioned migrations for trust roots, identities, sessions, join credentials, and session events. Migrations must define foreign keys, uniqueness, state constraints, revocation behavior, compatibility, and retention. No schema mutation may occur in startup code.

## Deployment Boundary

This design authorizes implementation in HankServerside and Hankagent only. It does not authorize deployment, device installation, publishing, tagging, or rollout. Those actions require a separate explicit request and their applicable release gates.
