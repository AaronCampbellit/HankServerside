# Hank Platform API

## Native remote desktop (`desktop.v1`)

The platform defines trust administration, session authorization, the agent control plane, and an opaque end-to-end encrypted browser/agent data plane. Milestone 2 uses deterministic synthetic endpoint hosts; it does not perform physical screen capture or native input injection.

All HTTP calls require an authenticated Hank session and current singleton-home membership. Trust reads and every session/trust write require a home administrator. Unsafe cookie-authenticated requests also require the normal `X-Hank-CSRF-Token` header. IDs are home scoped; wrong-user or wrong-home session lookups return `404`.

### Trust routes

- `GET /v1/home/desktop-trust` returns public root metadata and public identity metadata only.
- `POST /v1/home/desktop-trust/operator-devices` bootstraps generation 1 or approves an operator device.
- `POST /v1/home/desktop-trust/operator-devices/{deviceID}/revoke` revokes an operator identity.
- `POST /v1/home/desktop-trust/endpoints/{agentID}/approve` approves an endpoint identity.
- `POST /v1/home/desktop-trust/endpoints/{agentID}/revoke` revokes an endpoint identity.
- `POST /v1/home/desktop-trust/recovery` issues a five-minute, single-use challenge when `challenge` and `root_signature` are absent, then completes root-authorized operator recovery when both are supplied.
- `POST /v1/home/desktop-trust/rotate` advances the root exactly one generation after old-root proof.
- `POST /v1/home/desktop-trust/reset` cryptographically resets trust, revoking all prior identities and live sessions.

Public keys are unpadded base64url P-256 SPKI. A `certificate` is unpadded base64url JSON containing unpadded base64url `claims` and DER `signature`; the signature covers the canonical `desktop.v1` claim bytes. Certificates and recovery envelopes are limited to 64 KiB. Operator capabilities are the closed set `operator.approve`, `endpoint.approve`, `trust.recover`, and `trust.rotate`. Bootstrap, recovery, rotation, and reset replacement administrators require all four.

Destructive confirmation strings are exact: `create desktop trust`, `revoke desktop identity`, `recover desktop trust`, `rotate desktop trust`, and `reset desktop trust`.

### Session routes

- `POST /v1/agents/{agentID}/desktop-sessions` accepts `operator_device_id` and `permissions`.
- `GET /v1/agents/{agentID}/desktop-readiness` returns certificate-validated trust plus the latest endpoint-reported fixed readiness checks; capabilities alone never imply trust/readiness.
- `GET /v1/desktop-sessions/{sessionID}` returns sanitized session and endpoint-certificate metadata.
- `GET /v1/desktop-sessions/{sessionID}/events?after_sequence=0&limit=100` returns ordered metadata-only lifecycle events.
- `GET /v1/home/desktop-sessions?after=0&limit=25` returns paginated terminal metadata aggregates only.
- `POST /v1/desktop-sessions/{sessionID}/reconnect` rotates both side credentials and increments the key epoch.
- `POST /v1/desktop-sessions/{sessionID}/terminate` terminates idempotently and clears the browser credential.

The create/reconnect response includes `websocket_path: /ws/desktop/browser/{sessionID}` but never contains either join credential. The browser credential is transported only in the Secure, HttpOnly, SameSite=Strict `hank_desktop_join` cookie scoped to `/ws/desktop/browser/`. Initial join is 60 seconds; reconnect is at most 90 seconds and never extends the eight-hour hard expiry. The agent credential is sent only inside the authenticated `desktop.session.offer` control command. That offer also carries the current public trust-root generation/SPKI/fingerprint and a short-lived active-operator status assertion; the endpoint must match them to its locally pinned root and validate the full operator certificate chain before accepting transcript proof.

States are `requested`, `offered`, `agent_ready`, `joining`, `active`, `reconnecting`, then terminal `denied`, `failed`, `expired`, or `terminated`. Stable public failure reasons include `agent_offer_failed`, `offer_transition_failed`, `join_expired`, `reconnect_expired`, `hard_expired`, `agent_error`, `agent_terminated`, and `user_ended`.

Control commands are `desktop.status`, `desktop.session.offer`, `desktop.session.activate`, `desktop.session.close`, `desktop.session.set_control`, `desktop.session.set_display`, and `desktop.session.set_quality`. Agent events are `desktop.session.ready`, `desktop.session.connected`, `desktop.session.disconnected`, `desktop.display.changed`, `desktop.permission.required`, `desktop.secure_desktop.entered`, `desktop.secure_desktop.exited`, `desktop.session.stats`, `desktop.session.error`, and `desktop.session.terminated`. Events are published only on the owner-authorized `desktop.session:{sessionID}` topic.

### Encrypted data plane

- Browser: `GET /ws/desktop/browser/{sessionID}` with the scoped `hank_desktop_join` cookie and a same-origin `Origin` header.
- Agent: `GET /ws/desktop/agent/{sessionID}` with its single-use bearer credential and exact `X-Hank-Agent-ID`.

Both WebSockets accept binary messages only. Each message begins with the 12-byte `HDV1` framing header and is one of browser handshake, endpoint handshake, or encrypted record. The signed P-256 transcript binds home, session, agent, operator user/device, ordered permissions, browser ephemeral key, both expiries, and key epoch. The endpoint signs the same transcript plus its fresh ephemeral key. Directional AES-256-GCM keys and four-byte nonce prefixes are HKDF-SHA-256 derived from the ephemeral ECDH secret and transcript digest. Records use strict epoch and monotonically increasing per-direction sequence numbers; reconnect advances the epoch and replaces credentials, ephemeral keys, keys, nonce prefixes, and counters.

Encrypted inner messages use an eight-byte version/type/length header. The v1 required types cover codec configuration, H.264 access units, display inventory, keyboard, pointer, clipboard, control/quality, ping/pong, statistics, permission state, secure-desktop state, and termination. Unknown required messages and oversized payloads close the session; unknown messages marked optional may be ignored. The server forwards complete opaque outer messages and records only routing scope, lifecycle reasons, timing, and directional byte counts.
