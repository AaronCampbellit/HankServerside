# Hank Native Remote Desktop Shared Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Milestone 1 of Hank Native Remote Desktop: the versioned trust, recovery, identity, session, authorization, control-plane, audit, opaque-relay, and cross-language fixture foundation shared by HankServerside and Hankagent.

**Architecture:** HankServerside remains the canonical contract owner and persists only public trust material, encrypted recovery envelopes, metadata, hashed join credentials, and session state. A dedicated service layer enforces administrator scope and the session state machine, while an internal relay interface pairs separately authenticated browser and agent sockets without parsing ciphertext. Hankagent consumes the versioned control types and golden fixtures on macOS and Windows but does not advertise desktop capability until Milestone 2 provides a functioning session coordinator.

**Tech Stack:** Go 1.24 standard library, PostgreSQL versioned migrations, `database/sql` with pgx, `github.com/coder/websocket`, React/Vite TypeScript contract fixtures, Swift 6/CryptoKit-compatible encodings, .NET 8/CNG-compatible encodings, SHA-256, ECDSA/ECDH P-256, HKDF-SHA-256, and AES-256-GCM.

**Milestone:** 1 of 6 — Shared foundation.

## Global Constraints

- The authoritative design is `docs/superpowers/specs/2026-07-21-hank-native-remote-desktop-v1-design.md`.
- HankServerside owns every shared API, persistence, protocol, authorization, relay, audit, and fixture contract; Hankagent conforms to it.
- V1 targets one self-hosted home, one remote operator per endpoint, Windows and macOS, and the exact active physical console.
- V1 cryptography is ECDSA P-256/SHA-256, ephemeral ECDH P-256, HKDF-SHA-256, AES-256-GCM, and SHA-256.
- Identity public keys are DER SubjectPublicKeyInfo; ephemeral public keys are uncompressed P-256 points; JSON binary values are unpadded base64url.
- Signed transcripts use the specified length-prefixed binary encoding and never ordinary JSON serialization.
- The server may store public trust material, encrypted recovery envelopes, hashes, session metadata, and ciphertext metrics; it must never store private keys, recovery secrets, plaintext credentials, screen contents, video, input, keystrokes, pointer payloads, or clipboard contents.
- Browser and agent join credentials are distinct random 256-bit values, server-stored only as SHA-256 hashes, single-use, side-bound, session-bound, epoch-bound, and short-lived.
- Browser credentials use a `Secure`, `HttpOnly`, `SameSite=Strict`, path-limited cookie; agent credentials use the authenticated control plane and an `Authorization: Bearer` header. Neither credential may appear in a URL.
- Initial join expires after 60 seconds, reconnect after 90 seconds, and hard session authorization after eight hours.
- Reconnect increments the key epoch and issues fresh credentials; it never restores keys, sequence counters, credentials, or sockets.
- All trust writes and session creation/termination writes require established cookie-CSRF protection; trust mutation additionally requires home administrator role and explicit confirmation where specified.
- No Remote Desktop capability is advertised by Hankagent in this milestone.
- All schema changes use `internal/migrations/sql/*.up.sql`; startup code must not mutate schema.
- Existing untracked `.codex/` artifacts and unrelated user work remain untouched.
- This plan does not authorize deployment, device installation, publishing, tagging, or pushing either repository.

---

## File Map

### HankServerside

- `internal/protocol/desktop.go`: canonical desktop constants, JSON control types, permission/state validation, and transcript field definitions.
- `internal/protocol/desktop_test.go`: table-driven validation and JSON compatibility tests.
- `internal/desktopcrypto/encoding.go`: length-prefixed handshake/certificate/recovery-proof encoding, unpadded base64url, fingerprints, HKDF labels, nonces, and record headers.
- `internal/desktopcrypto/encoding_test.go`: deterministic handshake, certificate, recovery, record, and tamper vectors loaded from the public fixture.
- `schemas/desktop/v1/test-vectors.json`: language-neutral canonical handshake, certificate, recovery-proof, digest, HKDF, nonce, AES-GCM, and invalid-vector fixtures.
- `internal/migrations/sql/000021_remote_desktop_foundation.up.sql`: trust, identity, session, join-credential, and event tables plus constraints/indexes.
- `internal/migrations/migrations_test.go`: schema text assertions for sensitive constraints.
- `internal/domain/desktop.go`: storage-facing trust, identity, credential, session, and event models.
- `internal/store/desktop_trust.go`: trust-root and identity create/read/revoke/rotate/reset methods.
- `internal/store/desktop_trust_test.go`: PostgreSQL-backed trust lifecycle and ownership tests.
- `internal/store/desktop_sessions.go`: atomic session creation, transitions, credential consumption, reconnect, counters, and event listing.
- `internal/store/desktop_sessions_test.go`: PostgreSQL-backed state-machine, race, expiration, and one-operator tests.
- `internal/cloud/desktop_service.go`: authorization-independent orchestration, clocks, token source, state transitions, and typed service errors.
- `internal/cloud/desktop_service_test.go`: deterministic service policy tests.
- `internal/cloud/desktop_trust_handlers.go`: `/v1/home/desktop-trust` admin routes and confirmation/audit rules.
- `internal/cloud/desktop_trust_handlers_test.go`: membership, CSRF, malformed key, recovery, revocation, and reset HTTP tests.
- `internal/cloud/desktop_session_handlers.go`: session create/read/reconnect/terminate/events HTTP routes and secure browser-cookie issuance.
- `internal/cloud/desktop_session_handlers_test.go`: route ownership, agent readiness, single-use credential, cookie, and expiration tests.
- `internal/cloud/desktop_control.go`: control-plane offer/readiness/lifecycle routing and agent event validation.
- `internal/cloud/desktop_control_test.go`: fake-agent offer and wrong-scope event tests.
- `internal/cloud/desktop_relay.go`: internal relay interface, join claims, limits, lifecycle callbacks, and Milestone 2 implementation boundary.
- `internal/cloud/desktop_relay_test.go`: limit validation, join-claim validation, lifecycle callback, and opacity-boundary tests using a fake relay.
- `internal/cloud/home_singleton.go`: trust route delegation.
- `internal/cloud/server.go`: service/relay construction and session/WebSocket route registration.
- `internal/cloud/realtime.go`: allow only session-owned desktop lifecycle events.
- `internal/cloud/production_validation_test.go`: route inventory and security policy coverage.
- `internal/store/production_state.go`: lifecycle prune counters for expired credentials and retained session metadata.
- `internal/maintenance/lifecycle.go`: call the new desktop pruning method from the existing maintenance pass.
- `docs/api.md`: document the exact Milestone 1 HTTP/control/data-plane contract and reason codes.
- `docs/security.md`: document the relay visibility boundary, trust recovery, reset consequence, and browser-JavaScript limitation.

### Hankagent

- `Sources/HankKit/DesktopProtocol.swift`: Swift control-plane constants and Codable payloads.
- `Sources/HankKit/DesktopCryptoFixture.swift`: fixture loader and deterministic encoding helpers used by the self-test.
- `Sources/HankKitSelftest/main.swift`: Swift conformance assertions for the canonical vectors.
- `HankAgent-Windows/src/HankAgent.Contracts/DesktopProtocol.cs`: .NET control-plane constants and records.
- `HankAgent-Windows/src/HankAgent.Contracts/DesktopCryptoFixture.cs`: .NET deterministic encoding helpers used by tests.
- `HankAgent-Windows/tests/HankAgent.Tests/DesktopContractTests.cs`: .NET conformance assertions for the canonical vectors.
- `Fixtures/desktop-v1-test-vectors.json`: byte-identical copy of the canonical HankServerside fixture, checked by repository tests.

---

### Task 1: Canonical desktop protocol types and validators

**Files:**
- Create: `internal/protocol/desktop.go`
- Create: `internal/protocol/desktop_test.go`

**Interfaces:**
- Consumes: existing `protocol.Envelope`, `protocol.RoutedCommand`, and JSON conventions in `internal/protocol/messages.go`.
- Produces: `DesktopPermission`, `DesktopSessionState`, `DesktopIdentityType`, command/event constants, `DesktopSessionOffer`, `DesktopSessionReady`, `DesktopSessionLifecycleEvent`, `DesktopHandshakeParty`, and their `Validate` methods.

- [x] **Step 1: Write failing validation tests**

```go
func TestDesktopPermissionSetRejectsUnknownOrElevateWithoutControl(t *testing.T) {
	t.Parallel()
	if err := ValidateDesktopPermissions([]DesktopPermission{DesktopPermissionView, "desktop.unknown"}); err == nil {
		t.Fatal("unknown permission accepted")
	}
	if err := ValidateDesktopPermissions([]DesktopPermission{DesktopPermissionView, DesktopPermissionElevate}); err == nil {
		t.Fatal("elevate without control accepted")
	}
}

func TestDesktopSessionStateTransitions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		from DesktopSessionState
		to   DesktopSessionState
		ok   bool
	}{
		{DesktopSessionRequested, DesktopSessionOffered, true},
		{DesktopSessionJoining, DesktopSessionActive, true},
		{DesktopSessionActive, DesktopSessionReconnecting, true},
		{DesktopSessionReconnecting, DesktopSessionActive, true},
		{DesktopSessionTerminated, DesktopSessionActive, false},
		{DesktopSessionRequested, DesktopSessionActive, false},
	} {
		if got := CanTransitionDesktopSession(tc.from, tc.to); got != tc.ok {
			t.Fatalf("CanTransitionDesktopSession(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.ok)
		}
	}
}

func TestDesktopOfferValidationBindsSessionHomeAgentAndEpoch(t *testing.T) {
	t.Parallel()
	offer := DesktopSessionOffer{
		Protocol: "desktop.v1", SessionID: "desk_01J1V2J4S5Q6R7T8V9W0X1Y2Z3",
		HomeID: "home_1", AgentID: "agent_1", OperatorUserID: "usr_1",
		OperatorDeviceID: "device_1", Permissions: []DesktopPermission{DesktopPermissionView},
		KeyEpoch: 1, JoinExpiresAt: time.Now().Add(time.Minute), HardExpiresAt: time.Now().Add(time.Hour),
		AgentJoinCredential: "agent-credential", OperatorCertificate: "certificate", OperatorCertificateFingerprint: "fingerprint",
	}
	if err := offer.Validate(time.Now()); err != nil { t.Fatalf("Validate: %v", err) }
	offer.KeyEpoch = 0
	if err := offer.Validate(time.Now()); err == nil { t.Fatal("zero epoch accepted") }
}
```

- [x] **Step 2: Run the protocol tests and verify the red state**

Run: `go test ./internal/protocol -run 'TestDesktop' -count=1`

Expected: compilation fails because the desktop types do not exist.

- [x] **Step 3: Add the canonical constants, types, and validators**

```go
package protocol

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

const DesktopProtocolVersion = "desktop.v1"

type DesktopPermission string
const (
	DesktopPermissionView DesktopPermission = "desktop.view"
	DesktopPermissionControl DesktopPermission = "desktop.control"
	DesktopPermissionClipboardRead DesktopPermission = "desktop.clipboard.read"
	DesktopPermissionClipboardWrite DesktopPermission = "desktop.clipboard.write"
	DesktopPermissionElevate DesktopPermission = "desktop.elevate"
	DesktopPermissionSecureDesktop DesktopPermission = "desktop.secure_desktop"
	DesktopPermissionUnattended DesktopPermission = "desktop.unattended"
)

type DesktopSessionState string
const (
	DesktopSessionRequested DesktopSessionState = "requested"
	DesktopSessionOffered DesktopSessionState = "offered"
	DesktopSessionAgentReady DesktopSessionState = "agent_ready"
	DesktopSessionJoining DesktopSessionState = "joining"
	DesktopSessionActive DesktopSessionState = "active"
	DesktopSessionReconnecting DesktopSessionState = "reconnecting"
	DesktopSessionDenied DesktopSessionState = "denied"
	DesktopSessionFailed DesktopSessionState = "failed"
	DesktopSessionExpired DesktopSessionState = "expired"
	DesktopSessionTerminated DesktopSessionState = "terminated"
)

type DesktopIdentityType string
const (
	DesktopIdentityOperatorDevice DesktopIdentityType = "operator_device"
	DesktopIdentityEndpoint DesktopIdentityType = "endpoint"
)

const (
	CommandDesktopStatus = "desktop.status"
	CommandDesktopSessionOffer = "desktop.session.offer"
	CommandDesktopSessionActivate = "desktop.session.activate"
	CommandDesktopSessionClose = "desktop.session.close"
	CommandDesktopSessionSetControl = "desktop.session.set_control"
	CommandDesktopSessionSetDisplay = "desktop.session.set_display"
	CommandDesktopSessionSetQuality = "desktop.session.set_quality"
	EventDesktopSessionReady = "desktop.session.ready"
	EventDesktopSessionConnected = "desktop.session.connected"
	EventDesktopSessionDisconnected = "desktop.session.disconnected"
	EventDesktopDisplayChanged = "desktop.display.changed"
	EventDesktopPermissionRequired = "desktop.permission.required"
	EventDesktopSecureDesktopEntered = "desktop.secure_desktop.entered"
	EventDesktopSecureDesktopExited = "desktop.secure_desktop.exited"
	EventDesktopSessionStats = "desktop.session.stats"
	EventDesktopSessionError = "desktop.session.error"
	EventDesktopSessionTerminated = "desktop.session.terminated"
)

var desktopIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type DesktopHandshakeParty struct {
	IdentityID string `json:"identity_id"`
	Certificate string `json:"certificate"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	Signature string `json:"signature"`
}

type DesktopSessionOffer struct {
	Protocol string `json:"protocol"`
	SessionID string `json:"session_id"`
	HomeID string `json:"home_id"`
	AgentID string `json:"agent_id"`
	OperatorUserID string `json:"operator_user_id"`
	OperatorDeviceID string `json:"operator_device_id"`
	Permissions []DesktopPermission `json:"permissions"`
	KeyEpoch uint32 `json:"key_epoch"`
	JoinExpiresAt time.Time `json:"join_expires_at"`
	HardExpiresAt time.Time `json:"hard_expires_at"`
	AgentJoinCredential string `json:"agent_join_credential"`
	OperatorCertificate string `json:"operator_certificate"`
	OperatorCertificateFingerprint string `json:"operator_certificate_fingerprint"`
}

type DesktopSessionReady struct {
	Protocol string `json:"protocol"`
	SessionID string `json:"session_id"`
	KeyEpoch uint32 `json:"key_epoch"`
	EndpointCertificate string `json:"endpoint_certificate"`
	EndpointCertificateFingerprint string `json:"endpoint_certificate_fingerprint"`
	Readiness map[string]string `json:"readiness"`
}

type DesktopSessionLifecycleEvent struct {
	Protocol string `json:"protocol"`
	SessionID string `json:"session_id"`
	KeyEpoch uint32 `json:"key_epoch"`
	ReasonCode string `json:"reason_code,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func ValidateDesktopPermissions(values []DesktopPermission) error {
	if len(values) == 0 || !slices.Contains(values, DesktopPermissionView) { return errors.New("desktop.view is required") }
	allowed := map[DesktopPermission]bool{
		DesktopPermissionView:true, DesktopPermissionControl:true,
		DesktopPermissionClipboardRead:true, DesktopPermissionClipboardWrite:true,
		DesktopPermissionElevate:true, DesktopPermissionSecureDesktop:true,
		DesktopPermissionUnattended:true,
	}
	seen := map[DesktopPermission]bool{}
	for _, value := range values {
		if !allowed[value] { return errors.New("unsupported desktop permission") }
		if seen[value] { return errors.New("duplicate desktop permission") }
		seen[value] = true
	}
	if (seen[DesktopPermissionElevate] || seen[DesktopPermissionSecureDesktop]) && !seen[DesktopPermissionControl] {
		return errors.New("elevated and secure-desktop access require desktop.control")
	}
	return nil
}

func CanTransitionDesktopSession(from, to DesktopSessionState) bool {
	allowed := map[DesktopSessionState][]DesktopSessionState{
		DesktopSessionRequested:{DesktopSessionOffered, DesktopSessionDenied, DesktopSessionFailed, DesktopSessionExpired, DesktopSessionTerminated},
		DesktopSessionOffered:{DesktopSessionAgentReady, DesktopSessionDenied, DesktopSessionFailed, DesktopSessionExpired, DesktopSessionTerminated},
		DesktopSessionAgentReady:{DesktopSessionJoining, DesktopSessionDenied, DesktopSessionFailed, DesktopSessionExpired, DesktopSessionTerminated},
		DesktopSessionJoining:{DesktopSessionActive, DesktopSessionDenied, DesktopSessionFailed, DesktopSessionExpired, DesktopSessionTerminated},
		DesktopSessionActive:{DesktopSessionReconnecting, DesktopSessionFailed, DesktopSessionExpired, DesktopSessionTerminated},
		DesktopSessionReconnecting:{DesktopSessionActive, DesktopSessionFailed, DesktopSessionExpired, DesktopSessionTerminated},
	}
	return slices.Contains(allowed[from], to)
}

func (m DesktopSessionOffer) Validate(now time.Time) error {
	if m.Protocol != DesktopProtocolVersion || !desktopIDPattern.MatchString(m.SessionID) { return errors.New("invalid desktop offer identity") }
	if strings.TrimSpace(m.HomeID) == "" || strings.TrimSpace(m.AgentID) == "" || strings.TrimSpace(m.OperatorUserID) == "" || strings.TrimSpace(m.OperatorDeviceID) == "" { return errors.New("desktop offer scope is incomplete") }
	if m.KeyEpoch == 0 || strings.TrimSpace(m.AgentJoinCredential) == "" || strings.TrimSpace(m.OperatorCertificate) == "" || strings.TrimSpace(m.OperatorCertificateFingerprint) == "" { return errors.New("desktop offer credential, certificate, or epoch is invalid") }
	if !m.JoinExpiresAt.After(now) || !m.HardExpiresAt.After(m.JoinExpiresAt) { return errors.New("desktop offer timestamps are invalid") }
	return ValidateDesktopPermissions(m.Permissions)
}
```

- [x] **Step 4: Add JSON round-trip and event allowlist tests**

Add tests that marshal every command/event payload, reject an unknown protocol version, reject empty identity material, reject duplicate permissions, and prove terminal states have no outbound transitions. Use exact JSON field names from the structs above.

- [x] **Step 5: Run the protocol contract gates and preserve the local changes**

Run: `go test ./internal/protocol -count=1`

Expected: PASS.

```bash
git add internal/protocol/desktop.go internal/protocol/desktop_test.go
git commit -m "feat(remote-desktop): define v1 control contract"
```

### Task 2: Deterministic transcript, record, and cryptographic fixtures

**Files:**
- Create: `internal/desktopcrypto/encoding.go`
- Create: `internal/desktopcrypto/encoding_test.go`
- Create: `schemas/desktop/v1/test-vectors.json`

**Interfaces:**
- Consumes: `protocol.DesktopPermission` and the algorithm/encoding rules in the approved design.
- Produces: `EncodeHandshakeTranscript(HandshakeTranscript) ([]byte, error)`, `EncodeIdentityCertificate(IdentityCertificateClaims)`, `EncodeRecoveryContext`, `DeriveRecoveryKey`, `EncodeRecoveryEnrollmentProof(RecoveryEnrollmentClaims)`, `VerifyP256Signature`, `FingerprintSPKI([]byte) string`, `DeriveDirectionalKeys`, `Nonce`, `RecordHeader.MarshalBinary`, and one canonical JSON fixture.

- [x] **Step 1: Write failing deterministic-vector tests**

```go
func TestHandshakeTranscriptMatchesGoldenVector(t *testing.T) {
	vector := loadVector(t, "valid_initial_join")
	encoded, err := EncodeHandshakeTranscript(vector.Transcript.Input())
	if err != nil { t.Fatalf("EncodeHandshakeTranscript: %v", err) }
	if got := base64.RawURLEncoding.EncodeToString(encoded); got != vector.Transcript.EncodedBase64URL {
		t.Fatalf("encoded transcript = %s, want %s", got, vector.Transcript.EncodedBase64URL)
	}
	digest := sha256.Sum256(encoded)
	if got := base64.RawURLEncoding.EncodeToString(digest[:]); got != vector.Transcript.SHA256Base64URL {
		t.Fatalf("digest = %s, want %s", got, vector.Transcript.SHA256Base64URL)
	}
}

func TestRecordRejectsWrongEpochDirectionSequenceAndTag(t *testing.T) {
	vector := loadVector(t, "valid_initial_join")
	for _, mutation := range []string{"epoch", "direction", "sequence", "tag"} {
		if err := decryptMutatedRecord(vector, mutation); err == nil { t.Fatalf("%s mutation accepted", mutation) }
	}
}
```

- [x] **Step 2: Run the tests and verify the red state**

Run: `go test ./internal/desktopcrypto -count=1`

Expected: compilation fails because the package and fixture do not exist.

- [x] **Step 3: Implement the exact binary encodings**

```go
package desktopcrypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/crypto/hkdf"
	"io"
)

const HandshakeLabel = "Hank Desktop Handshake v1"
const RecordVersion byte = 1
const DirectionBrowserToAgent byte = 1
const DirectionAgentToBrowser byte = 2

type HandshakeTranscript struct {
	HomeID, SessionID, AgentID, OperatorUserID, OperatorDeviceID string
	Permissions []string
	BrowserEphemeralPublicKey []byte
	JoinExpiresAtUnixMS, HardExpiresAtUnixMS int64
	KeyEpoch uint32
}

type IdentityCertificateClaims struct {
	CertificateVersion string
	HomeID, IdentityID, IdentityType, UserID, DeviceID, AgentID string
	PublicKeySPKI []byte
	Capabilities []string
	TrustRootGeneration uint32
	CreatedAtUnixMS, ExpiresAtUnixMS int64
}

type RecoveryEnrollmentClaims struct {
	Label string
	HomeID string
	TrustRootGeneration uint32
	NewOperatorIdentityID, NewOperatorDeviceID string
	NewOperatorPublicKeySPKI []byte
	IssuedAtUnixMS int64
	Challenge []byte
}

type RootRotationClaims struct {
	HomeID string
	OldGeneration, NewGeneration uint32
	NewRootPublicKeySPKI, NewRecoveryEnvelopeHash []byte
	ReplacementOperatorIdentityID string
	IssuedAtUnixMS int64
}

func writeField(dst *bytes.Buffer, field []byte) error {
	if len(field) > 1<<20 { return errors.New("desktop transcript field exceeds 1 MiB") }
	if err := binary.Write(dst, binary.BigEndian, uint32(len(field))); err != nil { return err }
	_, err := dst.Write(field)
	return err
}

func EncodeHandshakeTranscript(value HandshakeTranscript) ([]byte, error) {
	var out bytes.Buffer
	fields := [][]byte{[]byte(HandshakeLabel), []byte(value.HomeID), []byte(value.SessionID), []byte(value.AgentID), []byte(value.OperatorUserID), []byte(value.OperatorDeviceID)}
	for _, field := range fields { if err := writeField(&out, field); err != nil { return nil, err } }
	if err := binary.Write(&out, binary.BigEndian, uint32(len(value.Permissions))); err != nil { return nil, err }
	for _, permission := range value.Permissions { if err := writeField(&out, []byte(permission)); err != nil { return nil, err } }
	if err := writeField(&out, value.BrowserEphemeralPublicKey); err != nil { return nil, err }
	for _, number := range []any{value.JoinExpiresAtUnixMS, value.HardExpiresAtUnixMS, value.KeyEpoch} {
		if err := binary.Write(&out, binary.BigEndian, number); err != nil { return nil, err }
	}
	return out.Bytes(), nil
}

func EncodeIdentityCertificate(value IdentityCertificateClaims) ([]byte, error) {
	var out bytes.Buffer
	for _, field := range [][]byte{[]byte("Hank Desktop Identity Certificate v1"), []byte(value.CertificateVersion), []byte(value.HomeID), []byte(value.IdentityID), []byte(value.IdentityType), []byte(value.UserID), []byte(value.DeviceID), []byte(value.AgentID), value.PublicKeySPKI} {
		if err := writeField(&out, field); err != nil { return nil, err }
	}
	if err := binary.Write(&out, binary.BigEndian, uint32(len(value.Capabilities))); err != nil { return nil, err }
	for _, capability := range value.Capabilities { if err := writeField(&out, []byte(capability)); err != nil { return nil, err } }
	for _, number := range []any{value.TrustRootGeneration, value.CreatedAtUnixMS, value.ExpiresAtUnixMS} { if err := binary.Write(&out, binary.BigEndian, number); err != nil { return nil, err } }
	return out.Bytes(), nil
}

func EncodeRecoveryEnrollmentProof(value RecoveryEnrollmentClaims) ([]byte, error) {
	var out bytes.Buffer
	for _, field := range [][]byte{[]byte("Hank Desktop Recovery Enrollment v1"), []byte(value.Label), []byte(value.HomeID)} { if err := writeField(&out, field); err != nil { return nil, err } }
	if err := binary.Write(&out, binary.BigEndian, value.TrustRootGeneration); err != nil { return nil, err }
	for _, field := range [][]byte{[]byte(value.NewOperatorIdentityID), []byte(value.NewOperatorDeviceID), value.NewOperatorPublicKeySPKI} { if err := writeField(&out, field); err != nil { return nil, err } }
	if err := binary.Write(&out, binary.BigEndian, value.IssuedAtUnixMS); err != nil { return nil, err }
	if err := writeField(&out, value.Challenge); err != nil { return nil, err }
	return out.Bytes(), nil
}

func EncodeRecoveryContext(homeID string, generation uint32) ([]byte, error) {
	if homeID == "" || generation == 0 { return nil, errors.New("invalid recovery context") }
	var out bytes.Buffer
	for _, field := range [][]byte{[]byte("Hank Desktop Root Recovery v1"), []byte(homeID)} { if err := writeField(&out, field); err != nil { return nil, err } }
	if err := binary.Write(&out, binary.BigEndian, generation); err != nil { return nil, err }
	return out.Bytes(), nil
}

func EncodeRootRotationProof(value RootRotationClaims) ([]byte, error) {
	if value.HomeID == "" || value.OldGeneration == 0 || value.NewGeneration != value.OldGeneration+1 { return nil, errors.New("invalid root rotation scope") }
	var out bytes.Buffer
	for _, field := range [][]byte{[]byte("Hank Desktop Root Rotation v1"), []byte(value.HomeID)} { if err := writeField(&out, field); err != nil { return nil, err } }
	for _, number := range []uint32{value.OldGeneration, value.NewGeneration} { if err := binary.Write(&out, binary.BigEndian, number); err != nil { return nil, err } }
	for _, field := range [][]byte{value.NewRootPublicKeySPKI, value.NewRecoveryEnvelopeHash, []byte(value.ReplacementOperatorIdentityID)} { if err := writeField(&out, field); err != nil { return nil, err } }
	if err := binary.Write(&out, binary.BigEndian, value.IssuedAtUnixMS); err != nil { return nil, err }
	return out.Bytes(), nil
}

func DeriveRecoveryKey(secret, context []byte) ([32]byte, error) {
	var key [32]byte
	if len(secret) != 32 { return key, errors.New("recovery secret must be 32 bytes") }
	salt := sha256.Sum256(context)
	_, err := io.ReadFull(hkdf.New(sha256.New, secret, salt[:], []byte("hank-desktop-v1/root-recovery/key")), key[:])
	return key, err
}

func VerifyP256Signature(publicKey *ecdsa.PublicKey, encoded, signature []byte) error {
	if publicKey == nil || publicKey.Curve != elliptic.P256() { return errors.New("P-256 public key required") }
	digest := sha256.Sum256(encoded)
	if !ecdsa.VerifyASN1(publicKey, digest[:], signature) { return errors.New("invalid ECDSA signature") }
	return nil
}

func FingerprintSPKI(spki []byte) string {
	sum := sha256.Sum256(spki)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type DirectionalKeys struct { BrowserToAgent, AgentToBrowser [32]byte; BrowserNoncePrefix, AgentNoncePrefix [4]byte }

func DeriveDirectionalKeys(sharedSecret, transcriptDigest []byte) (DirectionalKeys, error) {
	var keys DirectionalKeys
	for label, target := range map[string][]byte{
		"hank-desktop-v1/browser-to-agent/key": keys.BrowserToAgent[:],
		"hank-desktop-v1/agent-to-browser/key": keys.AgentToBrowser[:],
		"hank-desktop-v1/browser-to-agent/nonce": keys.BrowserNoncePrefix[:],
		"hank-desktop-v1/agent-to-browser/nonce": keys.AgentNoncePrefix[:],
	} {
		if _, err := io.ReadFull(hkdf.New(sha256.New, sharedSecret, transcriptDigest, []byte(label)), target); err != nil { return DirectionalKeys{}, err }
	}
	return keys, nil
}

func Nonce(prefix [4]byte, sequence uint64) [12]byte {
	var nonce [12]byte
	copy(nonce[:4], prefix[:])
	binary.BigEndian.PutUint64(nonce[4:], sequence)
	return nonce
}

type RecordHeader struct { Version, Direction byte; KeyEpoch uint32; Sequence uint64; CiphertextLength uint32 }
func (h RecordHeader) MarshalBinary() ([]byte, error) {
	if h.Version != RecordVersion || (h.Direction != DirectionBrowserToAgent && h.Direction != DirectionAgentToBrowser) || h.KeyEpoch == 0 { return nil, errors.New("invalid desktop record header") }
	result := make([]byte, 18)
	result[0], result[1] = h.Version, h.Direction
	binary.BigEndian.PutUint32(result[2:6], h.KeyEpoch)
	binary.BigEndian.PutUint64(result[6:14], h.Sequence)
	binary.BigEndian.PutUint32(result[14:18], h.CiphertextLength)
	return result, nil
}

func DecodeRawBase64URL(value string) ([]byte, error) {
	if value == "" { return nil, errors.New("empty base64url value") }
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil { return nil, fmt.Errorf("decode unpadded base64url: %w", err) }
	return decoded, nil
}
```

- [x] **Step 4: Add the canonical fixture with fixed keys and expected outputs**

Generate the fixture once with a small Go test helper using fixed private scalars `1` and `2`, fixed recovery secret bytes `00` through `1f`, epoch `1`, sequence `0`, and plaintext `desktop-fixture-payload`. Commit only public/fixed test material. The JSON must contain these top-level keys and explicit invalid mutations:

```json
{
  "version": "desktop.v1",
  "valid_initial_join": {
    "transcript": {
      "label": "Hank Desktop Handshake v1",
      "home_id": "home_fixture",
      "session_id": "desk_fixture_0001",
      "agent_id": "agent_fixture",
      "operator_user_id": "usr_fixture",
      "operator_device_id": "device_fixture",
      "permissions": ["desktop.view", "desktop.control"],
      "browser_ephemeral_public_key_base64url": "BGsX0fLhLEJH-Lzm5WOkQPJ3A32BLeszoPShOUXYmMKWT-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfU",
      "join_expires_at_unix_ms": 1784678460000,
      "hard_expires_at_unix_ms": 1784707200000,
      "key_epoch": 1,
      "encoded_base64url": "AAAAGUhhbmsgRGVza3RvcCBIYW5kc2hha2UgdjEAAAAMaG9tZV9maXh0dXJlAAAAEWRlc2tfZml4dHVyZV8wMDAxAAAADWFnZW50X2ZpeHR1cmUAAAALdXNyX2ZpeHR1cmUAAAAOZGV2aWNlX2ZpeHR1cmUAAAACAAAADGRlc2t0b3AudmlldwAAAA9kZXNrdG9wLmNvbnRyb2wAAABBBGsX0fLhLEJH-Lzm5WOkQPJ3A32BLeszoPShOUXYmMKWT-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfUAAAGfhyAqYAAAAZ-I1rQAAAAAAQ",
      "sha256_base64url": "MpyvYaj8LF-7zqjVEFP-GAzOarJp86LofGu7EP7uUHA"
    },
    "hkdf": {
      "shared_secret_base64url": "fPJ7GI0DT36KUjgDBLUaw8CJaeJ38hs1pgtI_EdmmXg",
      "browser_to_agent_key_base64url": "V0kM6_E8g7LBDii17e1xXqkNxwLVCexMVSd-yroPT8I",
      "agent_to_browser_key_base64url": "scXrd1jQSlB37i6pbsX1tuaFZhUc9fzS-Dqv915AqM8",
      "browser_nonce_prefix_base64url": "1pGa5w",
      "agent_nonce_prefix_base64url": "XiGUDA"
    },
    "record": {
      "direction": 1,
      "sequence": 0,
      "plaintext_base64url": "ZGVza3RvcC1maXh0dXJlLXBheWxvYWQ",
      "header_base64url": "AQEAAAABAAAAAAAAAAAAAAAn",
      "ciphertext_base64url": "hbkLBBaz-0UMDxVZ8t0GkOacOplkXIQ04NEdj53YGDuU_5QbOpMh"
    }
  },
  "identity_certificates": {
    "root_public_key_spki_base64url": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEXsvk0aYzCkTI9--VHUvxZebGtyHvramF-0FmG8bn_WyHNGQMSZj_fjdLBs4aZKLs2CqwNjhPuD2aebEnon1QMg",
    "operator_device": {
      "public_key_spki_base64url": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE4lNKNTLQj7ugLd5lnuYr0AMf4tt4VZbvUJMCRGsDCFLg8VdaTGM8xxnf7l_ahi12TvyWw_MO4AVcQsI_GE7Yxg",
      "encoded_base64url": "AAAAJEhhbmsgRGVza3RvcCBJZGVudGl0eSBDZXJ0aWZpY2F0ZSB2MQAAAApkZXNrdG9wLnYxAAAADGhvbWVfZml4dHVyZQAAABRkaWRfb3BlcmF0b3JfZml4dHVyZQAAAA9vcGVyYXRvcl9kZXZpY2UAAAALdXNyX2ZpeHR1cmUAAAAOZGV2aWNlX2ZpeHR1cmUAAAAAAAAAWzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABOJTSjUy0I-7oC3eZZ7mK9ADH-LbeFWW71CTAkRrAwhS4PFXWkxjPMcZ3-5f2oYtdk78lsPzDuAFXELCPxhO2MYAAAACAAAAEGVuZHBvaW50LmFwcHJvdmUAAAANdHJ1c3QucmVjb3ZlcgAAAAEAAAGfhx9AAAAAAabe0GwA",
      "signature_base64url": "MEUCIQDqhTyeQTUVAV3BZPiId6ZsSfWDcverL4LaNd8yxhMTdQIgF_yf7qpX5x1KYEtKYMriX2lMN6dRo5xQCujQl3X-xOU"
    },
    "endpoint": {
      "public_key_spki_base64url": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEUVkLelFRQNLXhMhWCGaP3--Mgv0fW-UkIVVKDcPQM-3gwX2okEpyfYrhvza_inkmDQEvANTYCIjR0LtE_aFtpA",
      "encoded_base64url": "AAAAJEhhbmsgRGVza3RvcCBJZGVudGl0eSBDZXJ0aWZpY2F0ZSB2MQAAAApkZXNrdG9wLnYxAAAADGhvbWVfZml4dHVyZQAAABRkaWRfZW5kcG9pbnRfZml4dHVyZQAAAAhlbmRwb2ludAAAAAAAAAAAAAAADWFnZW50X2ZpeHR1cmUAAABbMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEUVkLelFRQNLXhMhWCGaP3--Mgv0fW-UkIVVKDcPQM-3gwX2okEpyfYrhvza_inkmDQEvANTYCIjR0LtE_aFtpAAAAAIAAAAMZGVza3RvcC52aWV3AAAAD2Rlc2t0b3AuY29udHJvbAAAAAEAAAGfhx9AAAAAAabe0GwA",
      "signature_base64url": "MEQCIHP1AJopwjmI9-bB6_2YaoqjyqVeGkZWaJdWQIxXq_-UAiBr5VUC0p0wdadyRiTPILHM0dDqVsJ1Mk3jjfIKe2EVkA"
    }
  },
  "recovery_enrollment": {
    "challenge_base64url": "ICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj8",
    "new_operator_public_key_spki_base64url": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEsBoXKnakYCyS0yQsuJfd4wJMdA3rshW0xrCq6TwikanoXBB0Mjfa1W_sDi37pwN5HAD3cBx-Fr39fEhTj8d_4g",
    "encoded_base64url": "AAAAI0hhbmsgRGVza3RvcCBSZWNvdmVyeSBFbnJvbGxtZW50IHYxAAAAF3JlY292ZXJfb3BlcmF0b3JfZGV2aWNlAAAADGhvbWVfZml4dHVyZQAAAAEAAAAWZGlkX29wZXJhdG9yX3JlY292ZXJlZAAAABByZWNvdmVyZWRfZGV2aWNlAAAAWzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABLAaFyp2pGAsktMkLLiX3eMCTHQN67IVtMawquk8IpGp6FwQdDI32tVv7A4t-6cDeRwA93Acfha9_XxIU4_Hf-IAAAGfhx9AAAAAACAgISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-Pw",
    "root_signature_base64url": "MEYCIQCjkGi8fJc0XkIGygXPxyXaOp-CriZVlQ-bVo6fnxbqtwIhAPi_h-uaFxTODRadgbh63_3F-2jDspRObkO1YdPnWaFz"
  },
  "recovery_envelope": {
    "context_base64url": "AAAAHUhhbmsgRGVza3RvcCBSb290IFJlY292ZXJ5IHYxAAAADGhvbWVfZml4dHVyZQAAAAE",
    "salt_sha256_base64url": "1-xN-3Ale_twQJ-GoG2PsqXZ_Oz7Rluc1zGxqOkAE3I",
    "secret_base64url": "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
    "key_base64url": "K5PWiiKHYgzyhaRJMkIqLKh87QkstFqyMwSptUkmMIw",
    "nonce_base64url": "oKGio6Slpqeoqaqr",
    "root_private_key_pkcs8_base64url": "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAOhRANCAARey-TRpjMKRMj375UdS_Fl5sa3Ie-tqYX7QWYbxuf9bIc0ZAxJmP9-N0sGzhpkouzYKrA2OE-4PZp5sSeifVAy",
    "ciphertext_and_tag_base64url": "eDVVsh7uJFlViIiqYRTj878smSFKiQvQn2U3u6kFELD0BXC98_omeTCkbbYl7QyoAISV8uUPK6zKPaxJvAZN7FJ2eAgsp78eucQpqvdSd-zvOavgtEoKM2n0xE-xEnhoj2P26Jtyfm3QA52Z6zZHL5UrZnnaUY-B_MOgy4P8cZcZ_aeRbM34ZU9FyHeyKCugrVJsWZqZZxbPbA"
  },
  "root_rotation": {
    "new_root_public_key_spki_base64url": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEjlM7b6C_e0YluzBmfAH7YH75-LioD-9bMAYocDGHsqNz6x294DMYNm0Gn4Om9ZAAU8c2M8sEGyHFXhqGwfQAtA",
    "new_recovery_envelope_sha256_base64url": "pnwceAE1g4YQ_KnS4wlN4xgzvafbZfiuIXpp1io0UHM",
    "encoded_base64url": "AAAAHUhhbmsgRGVza3RvcCBSb290IFJvdGF0aW9uIHYxAAAADGhvbWVfZml4dHVyZQAAAAEAAAACAAAAWzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABI5TO2-gv3tGJbswZnwB-2B--fi4qA_vWzAGKHAxh7Kjc-sdveAzGDZtBp-DpvWQAFPHNjPLBBshxV4ahsH0ALQAAAAgpnwceAE1g4YQ_KnS4wlN4xgzvafbZfiuIXpp1io0UHMAAAAUZGlkX29wZXJhdG9yX3JvdGF0ZWQAAAGfhx9AAA",
    "old_root_signature_base64url": "MEUCIHUha5Dv7LVGkju3p-Bnp3NuES8-ZlrhOgFbVtaneV67AiEAoo_ta04uf-bDsuPTV2YoLb7M1TIoPQRHnPnnB3Wthuw"
  },
  "invalid_records": ["wrong_epoch", "wrong_direction", "duplicate_sequence", "skipped_sequence", "bad_tag"],
  "invalid_trust": ["wrong_root_generation", "wrong_home", "wrong_identity_type", "expired_certificate", "revoked_approver", "bad_certificate_signature", "replayed_recovery_challenge"]
}
```

The fixture-update test must reproduce the committed values from the fixed private scalars and claims, but normal test execution keeps the fixture read-only. Signature verification tests consume the committed DER ECDSA signatures; they do not require signature generation to be byte-deterministic.

- [x] **Step 5: Run deterministic and tamper tests twice**

Run: `go test ./internal/desktopcrypto -count=2`

Expected: both runs PASS with byte-identical results.

- [x] **Step 6: Finalize the canonical crypto contract locally (commit deferred until explicitly authorized)**

```bash
git add internal/desktopcrypto/encoding.go internal/desktopcrypto/encoding_test.go schemas/desktop/v1/test-vectors.json
git commit -m "feat(remote-desktop): add canonical crypto fixtures"
```

### Task 3: Versioned persistence migration

**Files:**
- Create: `internal/migrations/sql/000021_remote_desktop_foundation.up.sql`
- Modify: `internal/migrations/migrations_test.go`

**Interfaces:**
- Consumes: existing `users`, `homes`, and `agents` primary keys.
- Produces: the five tables and database-enforced ownership/state/side/permission constraints used by Tasks 4 and 5.

- [x] **Step 1: Add a failing migration constraint test**

```go
func TestRemoteDesktopFoundationMigrationHasSecurityConstraints(t *testing.T) {
	t.Parallel()
	migrations, err := All()
	if err != nil { t.Fatalf("All: %v", err) }
	var body strings.Builder
	for _, migration := range migrations {
		if migration.Version == 21 { for _, statement := range migration.Statements { body.WriteString(statement); body.WriteByte('\n') } }
	}
	text := body.String()
	for _, required := range []string{
		"CREATE TABLE desktop_trust_roots", "CREATE TABLE desktop_identities",
		"CREATE TABLE desktop_sessions", "CREATE TABLE desktop_join_credentials",
		"CREATE TABLE desktop_session_events", "desktop_sessions_one_live_operator_idx",
		"credential_hash BYTEA NOT NULL UNIQUE", "CHECK (side IN ('browser', 'agent'))",
		"CHECK (state IN ('requested', 'offered', 'agent_ready', 'joining', 'active', 'reconnecting', 'denied', 'failed', 'expired', 'terminated'))",
		"CHECK (requested_permissions <@ ARRAY['desktop.view'", "CHECK (key_epoch > 0)",
	} {
		if !strings.Contains(text, required) { t.Fatalf("migration 21 missing %q", required) }
	}
}
```

- [x] **Step 2: Run and verify the test fails**

Run: `go test ./internal/migrations -run TestRemoteDesktopFoundationMigrationHasSecurityConstraints -count=1`

Expected: FAIL because migration 21 is absent.

- [x] **Step 3: Create migration 21**

```sql
CREATE TABLE desktop_trust_roots (
    home_id TEXT PRIMARY KEY REFERENCES homes(id) ON DELETE CASCADE,
    generation INTEGER NOT NULL CHECK (generation > 0),
    algorithm TEXT NOT NULL CHECK (algorithm = 'ECDSA_P256_SHA256'),
    public_key_spki BYTEA NOT NULL,
    fingerprint TEXT NOT NULL,
    recovery_envelope BYTEA,
    recovery_challenge_hash BYTEA,
    recovery_challenge_expires_at TIMESTAMPTZ,
    recovery_challenge_consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    rotated_at TIMESTAMPTZ,
    UNIQUE (home_id, generation),
    UNIQUE (home_id, fingerprint)
);

CREATE TABLE desktop_identities (
    id TEXT PRIMARY KEY,
    home_id TEXT NOT NULL REFERENCES homes(id) ON DELETE CASCADE,
    identity_type TEXT NOT NULL CHECK (identity_type IN ('operator_device', 'endpoint')),
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    device_id TEXT,
    agent_id TEXT REFERENCES agents(id) ON DELETE CASCADE,
    public_key_spki BYTEA NOT NULL,
    certificate BYTEA NOT NULL,
    fingerprint TEXT NOT NULL,
    capabilities TEXT[] NOT NULL DEFAULT '{}',
    trust_root_generation INTEGER NOT NULL CHECK (trust_root_generation > 0),
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revocation_reason TEXT,
    CHECK ((identity_type = 'operator_device' AND user_id IS NOT NULL AND device_id IS NOT NULL AND agent_id IS NULL)
        OR (identity_type = 'endpoint' AND user_id IS NULL AND device_id IS NULL AND agent_id IS NOT NULL)),
    UNIQUE (home_id, fingerprint)
);
CREATE UNIQUE INDEX desktop_operator_device_identity_idx ON desktop_identities(home_id, device_id) WHERE identity_type = 'operator_device' AND revoked_at IS NULL;
CREATE UNIQUE INDEX desktop_endpoint_identity_idx ON desktop_identities(home_id, agent_id) WHERE identity_type = 'endpoint' AND revoked_at IS NULL;

CREATE TABLE desktop_sessions (
    id TEXT PRIMARY KEY,
    home_id TEXT NOT NULL REFERENCES homes(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    operator_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    operator_device_identity_id TEXT NOT NULL REFERENCES desktop_identities(id),
    requested_permissions TEXT[] NOT NULL,
    effective_permissions TEXT[] NOT NULL,
    state TEXT NOT NULL,
    key_epoch INTEGER NOT NULL CHECK (key_epoch > 0),
    requested_at TIMESTAMPTZ NOT NULL,
    join_expires_at TIMESTAMPTZ NOT NULL,
    active_at TIMESTAMPTZ,
    reconnect_expires_at TIMESTAMPTZ,
    hard_expires_at TIMESTAMPTZ NOT NULL,
    terminated_at TIMESTAMPTZ,
    termination_reason TEXT,
    source_ip_hash TEXT NOT NULL DEFAULT '',
    source_user_agent_hash TEXT NOT NULL DEFAULT '',
    browser_to_agent_bytes BIGINT NOT NULL DEFAULT 0 CHECK (browser_to_agent_bytes >= 0),
    agent_to_browser_bytes BIGINT NOT NULL DEFAULT 0 CHECK (agent_to_browser_bytes >= 0),
    CHECK (state IN ('requested', 'offered', 'agent_ready', 'joining', 'active', 'reconnecting', 'denied', 'failed', 'expired', 'terminated')),
    CHECK (requested_permissions <@ ARRAY['desktop.view','desktop.control','desktop.clipboard.read','desktop.clipboard.write','desktop.elevate','desktop.secure_desktop','desktop.unattended']::TEXT[]),
    CHECK (effective_permissions <@ requested_permissions),
    CHECK ('desktop.view' = ANY(requested_permissions) AND 'desktop.view' = ANY(effective_permissions)),
    CHECK (join_expires_at <= requested_at + INTERVAL '60 seconds'),
    CHECK (hard_expires_at <= requested_at + INTERVAL '8 hours'),
    CHECK (reconnect_expires_at IS NULL OR reconnect_expires_at <= hard_expires_at)
);
CREATE UNIQUE INDEX desktop_sessions_one_live_operator_idx ON desktop_sessions(agent_id)
    WHERE state IN ('requested','offered','agent_ready','joining','active','reconnecting');

CREATE TABLE desktop_join_credentials (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES desktop_sessions(id) ON DELETE CASCADE,
    side TEXT NOT NULL CHECK (side IN ('browser', 'agent')),
    credential_hash BYTEA NOT NULL UNIQUE,
    key_epoch INTEGER NOT NULL CHECK (key_epoch > 0),
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    UNIQUE (session_id, side, key_epoch)
);

CREATE TABLE desktop_session_events (
    session_id TEXT NOT NULL REFERENCES desktop_sessions(id) ON DELETE CASCADE,
    sequence BIGINT NOT NULL CHECK (sequence > 0),
    event_type TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('user', 'agent', 'server', 'browser')),
    actor_id TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'error', 'security')),
    reason_code TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (session_id, sequence)
);
CREATE INDEX desktop_session_events_occurred_idx ON desktop_session_events(occurred_at DESC);
CREATE INDEX desktop_sessions_home_requested_idx ON desktop_sessions(home_id, requested_at DESC);
```

- [x] **Step 4: Run migration formatting checks and record unavailable database validation**

Run: `go test ./internal/migrations -count=1`

Expected: PASS.

Run with a configured test database: `HANK_REMOTE_TEST_DATABASE_URL="$HANK_REMOTE_TEST_DATABASE_URL" go test ./internal/store -run TestRemoteDesktopMigrationConstraints -count=1`

Expected: PASS; if the environment variable is unavailable, record this as skipped and do not claim database validation.

- [x] **Step 5: Finalize the migration locally (commit deferred until explicitly authorized)**

```bash
git add internal/migrations/sql/000021_remote_desktop_foundation.up.sql internal/migrations/migrations_test.go
git commit -m "feat(remote-desktop): add foundation schema"
```

### Task 4: Trust root, operator identity, endpoint identity, recovery, and reset store

**Files:**
- Create: `internal/domain/desktop.go`
- Create: `internal/store/desktop_trust.go`
- Create: `internal/store/desktop_trust_test.go`

**Interfaces:**
- Consumes: migration 21 tables and `store.ErrNotFound`/`store.ErrConflict`.
- Produces: typed trust/identity records plus atomic create, rotate, revoke, recover, and reset store operations.

- [x] **Step 1: Write PostgreSQL-backed trust lifecycle tests**

```go
func TestDesktopTrustLifecycleIsHomeScopedAndResetRevokesIdentities(t *testing.T) {
	db := openTestStore(t); defer db.Close()
	ctx := context.Background()
	homeA, userA, agentA := seedDesktopOwnerAgent(t, db, "a")
	homeB, _, _ := seedDesktopOwnerAgent(t, db, "b")
	root := domain.DesktopTrustRoot{HomeID: homeA.ID, Generation: 1, Algorithm: domain.DesktopTrustAlgorithm, PublicKeySPKI: []byte("root-a"), Fingerprint: "fp-root-a", RecoveryEnvelope: []byte("ciphertext"), CreatedAt: time.Now().UTC()}
	operator := domain.DesktopIdentity{ID:"did_operator_a", HomeID:homeA.ID, IdentityType:domain.DesktopIdentityOperatorDevice, UserID:userA.ID, DeviceID:"device-a", PublicKeySPKI:[]byte("operator"), Certificate:[]byte("certificate"), Fingerprint:"fp-operator-a", Capabilities:[]string{"endpoint.approve"}, TrustRootGeneration:1, CreatedAt:root.CreatedAt, ExpiresAt:root.CreatedAt.AddDate(1,0,0)}
	if err := db.BootstrapDesktopTrust(ctx, root, operator); err != nil { t.Fatalf("BootstrapDesktopTrust: %v", err) }
	endpoint := domain.DesktopIdentity{ID:"did_endpoint_a", HomeID:homeA.ID, IdentityType:domain.DesktopIdentityEndpoint, AgentID:agentA.ID, PublicKeySPKI:[]byte("endpoint"), Certificate:[]byte("certificate"), Fingerprint:"fp-endpoint-a", TrustRootGeneration:1, CreatedAt:root.CreatedAt, ExpiresAt:root.CreatedAt.AddDate(1,0,0)}
	if err := db.CreateDesktopIdentity(ctx, endpoint); err != nil { t.Fatalf("endpoint: %v", err) }
	if _, err := db.GetActiveDesktopEndpointIdentity(ctx, homeB.ID, agentA.ID, root.CreatedAt); !errors.Is(err, ErrNotFound) { t.Fatalf("cross-home identity = %v", err) }
	resetRoot := root; resetRoot.Generation = 2; resetRoot.Fingerprint = "fp-root-a-2"; resetRoot.PublicKeySPKI = []byte("root-a-2")
	replacement := operator; replacement.ID = "did_operator_a_2"; replacement.DeviceID = "device-a-2"; replacement.Fingerprint = "fp-operator-a-2"; replacement.TrustRootGeneration = 2
	if err := db.ResetDesktopTrust(ctx, resetRoot, replacement, root.CreatedAt.Add(time.Minute), "cryptographic_reset"); err != nil { t.Fatalf("ResetDesktopTrust: %v", err) }
	if _, err := db.GetActiveDesktopOperatorIdentity(ctx, homeA.ID, userA.ID, "device-a", root.CreatedAt.Add(2*time.Minute)); !errors.Is(err, ErrNotFound) { t.Fatalf("operator survived reset: %v", err) }
	if _, err := db.GetActiveDesktopOperatorIdentity(ctx, homeA.ID, userA.ID, "device-a-2", root.CreatedAt.Add(2*time.Minute)); err != nil { t.Fatalf("replacement operator missing: %v", err) }
}
```

- [x] **Step 2: Run and verify the red state**

Run: `go test ./internal/store -run TestDesktopTrust -count=1`

Expected: compilation fails because domain and store APIs are absent, or the test is skipped only because `HANK_REMOTE_TEST_DATABASE_URL` is unavailable.

- [x] **Step 3: Add the storage-facing domain types**

```go
package domain

import "time"

const DesktopTrustAlgorithm = "ECDSA_P256_SHA256"
const DesktopIdentityOperatorDevice = "operator_device"
const DesktopIdentityEndpoint = "endpoint"

type DesktopTrustRoot struct {
	HomeID string
	Generation int
	Algorithm string
	PublicKeySPKI []byte
	Fingerprint string
	RecoveryEnvelope []byte
	CreatedAt time.Time
	RotatedAt *time.Time
}

type DesktopIdentity struct {
	ID, HomeID, IdentityType, UserID, DeviceID, AgentID string
	PublicKeySPKI, Certificate []byte
	Fingerprint string
	Capabilities []string
	TrustRootGeneration int
	CreatedAt, ExpiresAt time.Time
	RevokedAt *time.Time
	RevocationReason string
}

type DesktopSession struct {
	ID, HomeID, AgentID, OperatorUserID, OperatorDeviceIdentityID string
	RequestedPermissions, EffectivePermissions []string
	State string
	KeyEpoch uint32
	RequestedAt, JoinExpiresAt, HardExpiresAt time.Time
	ActiveAt, ReconnectExpiresAt, TerminatedAt *time.Time
	TerminationReason, SourceIPHash, SourceUserAgentHash string
	BrowserToAgentBytes, AgentToBrowserBytes int64
}

type DesktopJoinCredential struct {
	ID, SessionID, Side string
	CredentialHash []byte
	KeyEpoch uint32
	CreatedAt, ExpiresAt time.Time
	ConsumedAt, RevokedAt *time.Time
}

type DesktopSessionEvent struct {
	SessionID string
	Sequence int64
	EventType, ActorType, ActorID string
	OccurredAt time.Time
	Severity, ReasonCode, MetadataJSON string
}
```

- [x] **Step 4: Implement focused transactional trust methods**

Add these exact exported methods in `internal/store/desktop_trust.go`; validate identity shape before SQL, use `pq.Array`-compatible pgx array scanning through the existing driver, and return `ErrConflict` on uniqueness violations:

```go
func (s *Store) BootstrapDesktopTrust(ctx context.Context, root domain.DesktopTrustRoot, firstOperator domain.DesktopIdentity) error
func (s *Store) GetDesktopTrustRoot(ctx context.Context, homeID string) (domain.DesktopTrustRoot, error)
func (s *Store) RotateDesktopTrust(ctx context.Context, root domain.DesktopTrustRoot, replacementOperator domain.DesktopIdentity, rotatedAt time.Time, reason string) error
func (s *Store) CreateDesktopIdentity(ctx context.Context, identity domain.DesktopIdentity) error
func (s *Store) GetActiveDesktopOperatorIdentity(ctx context.Context, homeID, userID, deviceID string, now time.Time) (domain.DesktopIdentity, error)
func (s *Store) GetActiveDesktopEndpointIdentity(ctx context.Context, homeID, agentID string, now time.Time) (domain.DesktopIdentity, error)
func (s *Store) ListDesktopIdentities(ctx context.Context, homeID string) ([]domain.DesktopIdentity, error)
func (s *Store) RevokeDesktopIdentity(ctx context.Context, homeID, identityID, reason string, revokedAt time.Time) (bool, error)
func (s *Store) ReplaceDesktopRecoveryEnvelope(ctx context.Context, homeID string, generation int, envelope []byte, rotatedAt time.Time) error
func (s *Store) IssueDesktopRecoveryChallenge(ctx context.Context, homeID string, challengeHash []byte, expiresAt time.Time) error
func (s *Store) ConsumeDesktopRecoveryChallengeAndCreateOperator(ctx context.Context, homeID string, generation int, challengeHash []byte, now time.Time, operator domain.DesktopIdentity) error
func (s *Store) ResetDesktopTrust(ctx context.Context, root domain.DesktopTrustRoot, replacementOperator domain.DesktopIdentity, resetAt time.Time, reason string) error
```

`BootstrapDesktopTrust` must atomically insert generation 1 and its first operator identity so a home is never left with an ownerless root. Recovery challenge issuance replaces any older unconsumed challenge; consumption uses `UPDATE ... WHERE recovery_challenge_hash = ? AND recovery_challenge_consumed_at IS NULL AND recovery_challenge_expires_at > ? RETURNING home_id` inside the same transaction that inserts the recovered operator. `RotateDesktopTrust` and `ResetDesktopTrust` both use one `SERIALIZABLE` transaction: lock the root row, require `root.Generation == current.Generation+1`, revoke every old-generation identity, terminate every live session, revoke every unconsumed credential, clear any recovery challenge, update the root key/fingerprint/envelope/generation, and insert the replacement operator identity. Rotation uses reason `trust_rotated` and additionally requires a verified old-root rotation proof in the handler; reset uses `trust_reset` and the explicit destructive confirmation. A failure rolls the entire transaction back.

- [x] **Step 5: Add negative tests and run the store suite**

Add explicit tests for wrong-home revoke, expired identity lookup, generation skip, duplicate fingerprint, malformed operator/endpoint actor columns, and recovery-envelope replacement with the wrong generation.

Run: `go test ./internal/store -run 'TestDesktopTrust|TestDesktopIdentity' -count=1`

Expected: PASS with PostgreSQL configured; otherwise SKIP with the exact environment-variable message.

- [x] **Step 6: Finalize trust persistence locally (commit deferred until explicitly authorized)**

```bash
git add internal/domain/desktop.go internal/store/desktop_trust.go internal/store/desktop_trust_test.go
git commit -m "feat(remote-desktop): persist trust identities"
```

### Task 5: Durable session state machine and single-use credentials

**Files:**
- Create: `internal/store/desktop_sessions.go`
- Create: `internal/store/desktop_sessions_test.go`

**Interfaces:**
- Consumes: `domain.DesktopSession`, `domain.DesktopJoinCredential`, `domain.DesktopSessionEvent`, and migration 21.
- Produces: atomic session creation, legal transition, reconnect credential rotation, credential consumption, counters, event listing, and pruning APIs.

- [x] **Step 1: Write failing state, race, and credential tests**

```go
func TestDesktopSessionCredentialIsSingleUseAndEpochBound(t *testing.T) {
	db := openTestStore(t); defer db.Close()
	ctx := context.Background()
	session, browser, agent := seedDesktopSessionRecords(t, db)
	first, err := db.ConsumeDesktopJoinCredential(ctx, browser.CredentialHash, "browser", session.ID, 1, time.Now().UTC())
	if err != nil { t.Fatalf("first consume: %v", err) }
	if first.SessionID != session.ID { t.Fatalf("session = %q", first.SessionID) }
	if _, err := db.ConsumeDesktopJoinCredential(ctx, browser.CredentialHash, "browser", session.ID, 1, time.Now().UTC()); !errors.Is(err, ErrNotFound) { t.Fatalf("reuse = %v", err) }
	if _, err := db.ConsumeDesktopJoinCredential(ctx, agent.CredentialHash, "browser", session.ID, 1, time.Now().UTC()); !errors.Is(err, ErrNotFound) { t.Fatalf("wrong side = %v", err) }
}

func TestDesktopSessionOnlyOneLiveOperatorPerAgent(t *testing.T) {
	db := openTestStore(t); defer db.Close()
	first, browser, agent := newDesktopSessionRecords("desk_first")
	if err := db.CreateDesktopSession(context.Background(), first, browser, agent, requestedEvent(first)); err != nil { t.Fatalf("first: %v", err) }
	second, browser2, agent2 := newDesktopSessionRecords("desk_second")
	if err := db.CreateDesktopSession(context.Background(), second, browser2, agent2, requestedEvent(second)); !errors.Is(err, ErrConflict) { t.Fatalf("second = %v", err) }
}

func TestDesktopSessionTerminalStateCannotReactivate(t *testing.T) {
	db := openTestStore(t); defer db.Close()
	session := seedDesktopSession(t, db)
	if _, err := db.TransitionDesktopSession(context.Background(), session.ID, []string{"requested"}, "terminated", "user_ended", time.Now().UTC(), requestedEvent(session)); err != nil { t.Fatalf("terminate: %v", err) }
	if _, err := db.TransitionDesktopSession(context.Background(), session.ID, []string{"terminated"}, "active", "", time.Now().UTC(), requestedEvent(session)); !errors.Is(err, ErrConflict) { t.Fatalf("reactivate = %v", err) }
}
```

- [x] **Step 2: Run the focused tests and verify the red state**

Run: `go test ./internal/store -run TestDesktopSession -count=1`

Expected: compilation failure or database skip before implementation.

- [x] **Step 3: Implement the atomic session APIs**

```go
func (s *Store) CreateDesktopSession(ctx context.Context, session domain.DesktopSession, browser, agent domain.DesktopJoinCredential, event domain.DesktopSessionEvent) error
func (s *Store) GetDesktopSession(ctx context.Context, sessionID string) (domain.DesktopSession, error)
func (s *Store) GetDesktopSessionForUser(ctx context.Context, sessionID, userID string) (domain.DesktopSession, error)
func (s *Store) TransitionDesktopSession(ctx context.Context, sessionID string, allowedFrom []string, nextState, reason string, at time.Time, event domain.DesktopSessionEvent) (domain.DesktopSession, error)
func (s *Store) BeginDesktopReconnect(ctx context.Context, sessionID string, expectedEpoch uint32, reconnectExpiresAt time.Time, browser, agent domain.DesktopJoinCredential, event domain.DesktopSessionEvent) (domain.DesktopSession, error)
func (s *Store) ConsumeDesktopJoinCredential(ctx context.Context, hash []byte, side, sessionID string, epoch uint32, now time.Time) (domain.DesktopJoinCredential, error)
func (s *Store) AddDesktopRelayBytes(ctx context.Context, sessionID string, browserToAgent, agentToBrowser int64) error
func (s *Store) ListDesktopSessionEvents(ctx context.Context, sessionID string, afterSequence int64, limit int) ([]domain.DesktopSessionEvent, error)
func (s *Store) ExpireDesktopSessions(ctx context.Context, now time.Time) (int64, error)
func (s *Store) PruneDesktopState(ctx context.Context, credentialBefore, eventBefore, sessionBefore time.Time) (credentials, events, sessions int64, err error)
```

Every mutating method must use one transaction and append the metadata event within it. `TransitionDesktopSession` must select the row `FOR UPDATE`, call `protocol.CanTransitionDesktopSession`, require the current state in `allowedFrom`, set terminal timestamps/reason for terminal states, and revoke remaining credentials. `BeginDesktopReconnect` must require state `active`, `expectedEpoch == current.KeyEpoch`, `reconnectExpiresAt <= hard_expires_at`, increment the epoch exactly once, revoke old credentials, and insert both new credentials.

- [x] **Step 4: Prove concurrent consumption and reconnect are safe**

Add a two-goroutine barrier test. Exactly one concurrent credential consume returns success; the other returns `ErrNotFound`. Exactly one concurrent reconnect increments epoch from `1` to `2`; the other returns `ErrConflict`. Verify event sequences remain gap-free per session.

- [x] **Step 5: Run session persistence gates and preserve the local changes**

Run: `go test ./internal/store -run TestDesktopSession -count=1`

Expected: PASS with PostgreSQL configured; otherwise SKIP and record the gap.

```bash
git add internal/store/desktop_sessions.go internal/store/desktop_sessions_test.go
git commit -m "feat(remote-desktop): persist secure session state"
```

### Task 6: Desktop service policy and credential issuance

**Files:**
- Create: `internal/cloud/desktop_service.go`
- Create: `internal/cloud/desktop_service_test.go`

**Interfaces:**
- Consumes: store APIs from Tasks 4 and 5, router agent presence/capabilities, `protocol.ValidateDesktopPermissions`, `newID`, `newToken`, and existing audit helpers.
- Produces: `desktopService.Create`, `Reconnect`, `Terminate`, `AgentReady`, `RelayJoined`, and typed errors mapped by HTTP/control-plane layers.

- [x] **Step 1: Write deterministic policy tests with injected clock and token source**

```go
func TestDesktopServiceCreateUsesSixtySecondJoinAndEightHourHardLimit(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	store := &fakeDesktopStore{operator: activeOperator(), endpoint: activeEndpoint()}
	service := newDesktopService(store, fakeDesktopAgents{online:true, capabilities:desktopCapabilities()}, func() time.Time { return now }, func() string { return strings.Repeat("a", 43) })
	result, err := service.Create(context.Background(), desktopCreateInput{HomeID:"home_1", AgentID:"agent_1", OperatorUserID:"usr_1", OperatorDeviceID:"device_1", Permissions:[]protocol.DesktopPermission{protocol.DesktopPermissionView}})
	if err != nil { t.Fatalf("Create: %v", err) }
	if got := result.Session.JoinExpiresAt.Sub(now); got != 60*time.Second { t.Fatalf("join TTL = %v", got) }
	if got := result.Session.HardExpiresAt.Sub(now); got != 8*time.Hour { t.Fatalf("hard TTL = %v", got) }
	if result.BrowserCredential == result.AgentCredential { t.Fatal("credential sides reused a secret") }
}

func TestDesktopServiceRejectsOfflineRevokedWrongHomeAndSecondOperator(t *testing.T) {
	for _, reason := range []string{"offline", "revoked_operator", "revoked_endpoint", "wrong_home", "existing_live_session"} {
		service := serviceForDeniedCase(reason)
		if _, err := service.Create(context.Background(), validCreateInput()); err == nil { t.Fatalf("%s accepted", reason) }
	}
}
```

- [x] **Step 2: Run and verify the red state**

Run: `go test ./internal/cloud -run TestDesktopService -count=1`

Expected: compilation fails because the service does not exist.

- [x] **Step 3: Implement the service boundary**

```go
var errDesktopAdminRequired = errors.New("desktop administrator role required")
var errDesktopAgentOffline = errors.New("desktop agent is offline")
var errDesktopCapabilityMissing = errors.New("desktop capability is unavailable")
var errDesktopIdentityUntrusted = errors.New("desktop identity is not trusted")
var errDesktopSessionConflict = errors.New("desktop session already exists for agent")
var errDesktopSessionExpired = errors.New("desktop session expired")

const desktopInitialJoinTTL = 60 * time.Second
const desktopReconnectTTL = 90 * time.Second
const desktopHardTTL = 8 * time.Hour

type desktopStore interface {
	GetActiveDesktopOperatorIdentity(context.Context, string, string, string, time.Time) (domain.DesktopIdentity, error)
	GetActiveDesktopEndpointIdentity(context.Context, string, string, time.Time) (domain.DesktopIdentity, error)
	CreateDesktopSession(context.Context, domain.DesktopSession, domain.DesktopJoinCredential, domain.DesktopJoinCredential, domain.DesktopSessionEvent) error
	GetDesktopSession(context.Context, string) (domain.DesktopSession, error)
	TransitionDesktopSession(context.Context, string, []string, string, string, time.Time, domain.DesktopSessionEvent) (domain.DesktopSession, error)
	BeginDesktopReconnect(context.Context, string, uint32, time.Time, domain.DesktopJoinCredential, domain.DesktopJoinCredential, domain.DesktopSessionEvent) (domain.DesktopSession, error)
}

type desktopCreateInput struct { HomeID, AgentID, OperatorUserID, OperatorDeviceID, SourceIPHash, SourceUserAgentHash string; Permissions []protocol.DesktopPermission }
type desktopCreateResult struct { Session domain.DesktopSession; BrowserCredential, AgentCredential string; OperatorIdentity, EndpointIdentity domain.DesktopIdentity }

type desktopService struct { store desktopStore; agents desktopAgentResolver; now func() time.Time; token func() string }

func desktopCredentialHash(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return append([]byte(nil), sum[:]...)
}
```

The `Create` body must: validate permissions; resolve online agent under the same home; require every requested capability; load active operator and endpoint identities; create two independent token values; hash them with SHA-256 before calling the store; set `requested`, epoch `1`, `joinExpiresAt=now+60s`, `hardExpiresAt=now+8h`; and return plaintext credentials only in the result. Never log or embed either credential in an error.

- [x] **Step 4: Implement reconnect and termination rules**

`Reconnect` accepts only the owning user and `active` state, clamps reconnect expiry to the earlier of `now+90s` and hard expiry, generates two new credentials, calls `BeginDesktopReconnect`, and returns the plaintext pair once. `Terminate` is idempotent for terminal sessions and transitions every live state to `terminated`. Add tests proving reconnect cannot extend hard expiry, wrong users cannot reconnect/terminate, and token-generator collisions return a conflict without exposing the token.

- [x] **Step 5: Run the service-layer gates and preserve the local changes**

Run: `go test ./internal/cloud -run TestDesktopService -count=1`

Expected: PASS.

```bash
git add internal/cloud/desktop_service.go internal/cloud/desktop_service_test.go
git commit -m "feat(remote-desktop): enforce session policy"
```

### Task 7: Trust administration HTTP API

**Files:**
- Create: `internal/cloud/desktop_trust_handlers.go`
- Create: `internal/cloud/desktop_trust_handlers_test.go`
- Modify: `internal/cloud/home_singleton.go`

**Interfaces:**
- Consumes: `requireAuth`, singleton-home membership/admin patterns, CSRF enforcement, trust store APIs, fingerprint helper, and `s.audit`.
- Produces: all `/v1/home/desktop-trust` routes in the approved design.

- [x] **Step 1: Write failing route security tests**

```go
func TestDesktopTrustWritesRequireAdminCSRFAndConfirmation(t *testing.T) {
	server, ownerToken, memberToken := setupDesktopTrustHTTP(t)
	requestJSONStatus(t, server, memberToken, http.MethodPost, "/v1/home/desktop-trust/operator-devices", validOperatorIdentityBody(), http.StatusForbidden)
	requestJSONStatusWithoutCSRF(t, server, ownerToken, http.MethodPost, "/v1/home/desktop-trust/operator-devices", validOperatorIdentityBody(), http.StatusForbidden)
	requestJSONStatus(t, server, ownerToken, http.MethodPost, "/v1/home/desktop-trust/reset", map[string]any{"confirmation":"wrong"}, http.StatusBadRequest)
}

func TestDesktopTrustResponseNeverContainsRecoverySecretOrPrivateKey(t *testing.T) {
	server, ownerToken, _ := setupDesktopTrustHTTP(t)
	body := requestRaw(t, server, ownerToken, http.MethodGet, "/v1/home/desktop-trust", nil, http.StatusOK)
	for _, forbidden := range []string{"private_key", "recovery_secret", "join_credential"} {
		if bytes.Contains(body, []byte(forbidden)) { t.Fatalf("response contains %q", forbidden) }
	}
}
```

- [x] **Step 2: Run and verify the red state**

Run: `go test ./internal/cloud -run TestDesktopTrust -count=1`

Expected: FAIL with 404 or missing handler helpers.

- [x] **Step 3: Add exact request types and strict key validation**

```go
type desktopOperatorDeviceRequest struct { IdentityID string `json:"identity_id"`; DeviceID string `json:"device_id"`; PublicKeySPKI string `json:"public_key_spki"`; Certificate string `json:"certificate"`; Capabilities []string `json:"capabilities"`; ExpiresAt time.Time `json:"expires_at"` }
type desktopTrustBootstrapRequest struct { Generation int `json:"generation"`; PublicKeySPKI string `json:"public_key_spki"`; RecoveryEnvelope string `json:"recovery_envelope"`; FirstOperator desktopOperatorDeviceRequest `json:"first_operator"`; Confirmation string `json:"confirmation"` }
type desktopEndpointApprovalRequest struct { IdentityID string `json:"identity_id"`; PublicKeySPKI string `json:"public_key_spki"`; Certificate string `json:"certificate"`; Capabilities []string `json:"capabilities"`; ExpiresAt time.Time `json:"expires_at"` }
type desktopRevocationRequest struct { Reason string `json:"reason"`; Confirmation string `json:"confirmation"` }
type desktopRecoveryRequest struct { Generation int `json:"generation"`; Operator desktopOperatorDeviceRequest `json:"operator"`; Challenge string `json:"challenge"`; RootSignature string `json:"root_signature"`; Confirmation string `json:"confirmation"` }
type desktopRotationRequest struct { Generation int `json:"generation"`; PublicKeySPKI string `json:"public_key_spki"`; RecoveryEnvelope string `json:"recovery_envelope"`; ReplacementOperator desktopOperatorDeviceRequest `json:"replacement_operator"`; OldRootSignature string `json:"old_root_signature"`; Confirmation string `json:"confirmation"` }
type desktopResetRequest struct { Generation int `json:"generation"`; PublicKeySPKI string `json:"public_key_spki"`; RecoveryEnvelope string `json:"recovery_envelope"`; ReplacementOperator desktopOperatorDeviceRequest `json:"replacement_operator"`; Confirmation string `json:"confirmation"` }
```

Decode public keys with `desktopcrypto.DecodeRawBase64URL`, parse with `x509.ParsePKIXPublicKey`, require `*ecdsa.PublicKey` on `elliptic.P256()`, cap certificate and recovery-envelope decoded sizes at 64 KiB, cap capability count at 32 and each value at 128 bytes, and cap identity lifetime at two years.

- [x] **Step 4: Implement the route delegate and audit calls**

`handleHomeDesktopTrust(w, r, home, auth, membership, parts) bool` owns paths whose first component is `desktop-trust`. GET allows any authenticated home administrator and returns root public metadata plus identities. Every POST requires admin. Bootstrap requires generation `1`, `confirmation == "create desktop trust"`, a valid root-signed first-operator certificate, and one atomic store call. Revoke requires `confirmation == "revoke desktop identity"`; recovery enrollment requires `confirmation == "recover desktop trust"`; root rotation at `POST /v1/home/desktop-trust/rotate` requires `confirmation == "rotate desktop trust"`; reset requires `confirmation == "reset desktop trust"`.

Trust capabilities are the closed set `operator.approve`, `endpoint.approve`, `trust.recover`, and `trust.rotate`. Bootstrap, recovered, rotated, and reset replacement administrators require all four so the home cannot enter a cryptographic dead end. For ordinary operator-device approval, verify the new certificate signature with an active operator identity that has `operator.approve`; for endpoint approval, verify with an active operator identity that has `endpoint.approve`. For recovery, first issue a 32-byte random, single-use, five-minute server challenge stored only as a hash; verify `EncodeRecoveryEnrollmentProof` with the stored root public key, consume the challenge atomically, and insert the recovered operator identity. For rotation, verify `EncodeRootRotationProof` with the old root and verify the replacement operator certificate with the new root before the atomic rotation. For reset, require the replacement operator certificate to verify under the new root and perform root update plus replacement-operator insertion in the same transaction. Certificate claims must exactly match the request path, authenticated user, home, key fingerprint, identity type, generation, capabilities, and timestamps.

Add this delegation before the final `http.NotFound` in `handleHomeSubroutes`:

```go
if s.handleHomeDesktopTrust(w, r, home, auth, membership, parts) { return }
```

Audit event names are exactly `desktop.trust.created`, `desktop.identity.approved`, `desktop.identity.revoked`, `desktop.recovery.completed`, `desktop.trust.rotated`, and `desktop.trust.reset`. Metadata includes only home ID, identity type/ID, fingerprint, generation, capabilities, and reason.

- [x] **Step 5: Add malformed/cross-home tests and run**

Test invalid base64url, RSA keys, P-384 keys, bad root/operator signatures, mismatched certificate claims, oversized certificate/envelope, wrong generation, wrong agent home, expired/revoked approver, missing approver capability, missing confirmation, non-admin member, absent CSRF, replayed/expired recovery challenge, and reset rollback on replacement-identity failure. Assert audit rows never contain key bytes, signature bytes, certificate bytes, recovery envelope, challenge plaintext, or request bodies.

Run: `go test ./internal/cloud -run TestDesktopTrust -count=1`

Expected: PASS with PostgreSQL configured; database-backed cases otherwise SKIP.

- [x] **Step 6: Finalize the trust API locally (commit deferred until explicitly authorized)**

```bash
git add internal/cloud/desktop_trust_handlers.go internal/cloud/desktop_trust_handlers_test.go internal/cloud/home_singleton.go
git commit -m "feat(remote-desktop): add trust administration API"
```

### Task 8: Session HTTP API and secure browser credential cookie

**Files:**
- Create: `internal/cloud/desktop_session_handlers.go`
- Create: `internal/cloud/desktop_session_handlers_test.go`
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/production_validation_test.go`

**Interfaces:**
- Consumes: `desktopService`, established auth/CSRF middleware, store event listing, and agent router capability state.
- Produces: create/read/reconnect/terminate/events routes and a browser join cookie; sends only the agent credential through `desktop.session.offer`.

- [x] **Step 1: Write failing API and cookie tests**

```go
func TestCreateDesktopSessionSetsOnlySecurePathLimitedBrowserCookie(t *testing.T) {
	server, token, agentID := setupDesktopSessionHTTP(t)
	response := requestJSONResponse(t, server, token, http.MethodPost, "/v1/agents/"+agentID+"/desktop-sessions", map[string]any{"operator_device_id":"device_1","permissions":[]string{"desktop.view"}}, http.StatusCreated)
	cookies := response.Cookies()
	if len(cookies) != 1 { t.Fatalf("cookies = %d", len(cookies)) }
	cookie := cookies[0]
	if cookie.Name != "hank_desktop_join" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/ws/desktop/browser/" { t.Fatalf("unsafe cookie: %#v", cookie) }
	body, _ := io.ReadAll(response.Body)
	if bytes.Contains(body, []byte("agent_join_credential")) || bytes.Contains(body, []byte(cookie.Value)) { t.Fatal("plaintext join credential leaked in JSON") }
}

func TestDesktopSessionWritesRequireCSRFAndOwnership(t *testing.T) {
	server, ownerToken, otherToken, sessionID := setupExistingDesktopSessionHTTP(t)
	requestJSONStatusWithoutCSRF(t, server, ownerToken, http.MethodPost, "/v1/desktop-sessions/"+sessionID+"/terminate", nil, http.StatusForbidden)
	requestJSONStatus(t, server, otherToken, http.MethodGet, "/v1/desktop-sessions/"+sessionID, nil, http.StatusNotFound)
}
```

- [x] **Step 2: Run and verify the red state**

Run: `go test ./internal/cloud -run 'TestCreateDesktopSession|TestDesktopSessionWrites' -count=1`

Expected: FAIL because routes are not registered.

- [x] **Step 3: Implement exact route parsing and response shapes**

Register:

```go
s.http.HandleFunc("/v1/agents/", s.handleAgentResourceRoutes)
s.http.HandleFunc("/v1/desktop-sessions/", s.handleDesktopSessionRoutes)
```

`POST /v1/agents/{agentID}/desktop-sessions` accepts:

```go
type createDesktopSessionRequest struct { OperatorDeviceID string `json:"operator_device_id"`; Permissions []protocol.DesktopPermission `json:"permissions"` }
```

The response contains only sanitized session metadata, endpoint public certificate/fingerprint, join/hard expiration, key epoch, and `websocket_path: "/ws/desktop/browser/{sessionID}"`. Do not include browser or agent credentials. Set the browser secret in `hank_desktop_join` with `Secure`, `HttpOnly`, `SameSiteStrictMode`, `Path=/ws/desktop/browser/`, and `MaxAge=60`. Signed ephemeral browser/agent handshake exchange begins only after the Milestone 2 data-plane joins; session creation authorizes and offers the session but does not claim that an E2EE epoch is active.

`GET /v1/desktop-sessions/{sessionID}` and `/events` require owning operator user plus current home membership. `POST reconnect` issues a replacement browser cookie with `MaxAge` capped by 90 seconds/hard expiry. `POST terminate` clears the cookie with the same path. Wrong user/home returns 404 to avoid existence disclosure.

- [x] **Step 4: Route the agent credential only over the authenticated control plane**

After durable creation succeeds, send `protocol.DesktopSessionOffer` via the resolved agent peer. On successful write, atomically transition `requested -> offered` and append `desktop.session.offered`. If send fails, transition the session to `failed` with reason `agent_offer_failed`, revoke credentials, clear the browser cookie, and return `503`. If the success transition itself fails, send `desktop.session.close` to the agent, terminate the durable session, and return `503`. The HTTP JSON and logs must never include `AgentJoinCredential`.

- [x] **Step 5: Add route inventory and failure-path tests**

Cover method mismatch, malformed IDs, offline agent, missing capability, untrusted operator/endpoint, second live operator, expired initial join, hard expiry, agent-offer failure, reconnect rotation, idempotent termination, and event pagination. Add `/v1/agents/` and `/v1/desktop-sessions/` to the production route inventory test. Assert the response reserves `/ws/desktop/browser/{sessionID}` as the Milestone 2 data-plane path without registering or serving that route yet.

Run: `go test ./internal/cloud -run 'TestCreateDesktopSession|TestDesktopSession|TestProductionRoute' -count=1`

Expected: PASS.

- [x] **Step 6: Finalize the session API locally (commit deferred until explicitly authorized)**

```bash
git add internal/cloud/desktop_session_handlers.go internal/cloud/desktop_session_handlers_test.go internal/cloud/server.go internal/cloud/production_validation_test.go
git commit -m "feat(remote-desktop): add secure session API"
```

### Task 9: Agent control-plane lifecycle routing

**Files:**
- Create: `internal/cloud/desktop_control.go`
- Create: `internal/cloud/desktop_control_test.go`
- Modify: `internal/cloud/realtime.go`
- Modify: `internal/cloud/server.go`

**Interfaces:**
- Consumes: protocol control types, desktop service transitions, router-resolved agent identity, and audit helper.
- Produces: validated agent readiness/connected/disconnected/error/terminated transitions and server close/activate commands.

- [x] **Step 1: Write failing wrong-scope and transition tests**

```go
func TestDesktopAgentReadyRequiresOwningAgentHomeAndEpoch(t *testing.T) {
	server := desktopControlServerForTest(t, "home_1", "agent_1", "desk_1", 1)
	for _, event := range []protocol.DesktopSessionReady{
		{Protocol:protocol.DesktopProtocolVersion, SessionID:"desk_1", KeyEpoch:2},
		{Protocol:protocol.DesktopProtocolVersion, SessionID:"desk_other", KeyEpoch:1},
	} {
		if err := server.handleDesktopSessionReady(context.Background(), "home_1", "agent_1", event); err == nil { t.Fatalf("bad event accepted: %#v", event) }
	}
	if err := server.handleDesktopSessionReady(context.Background(), "home_2", "agent_1", validReady()); err == nil { t.Fatal("wrong home accepted") }
	if err := server.handleDesktopSessionReady(context.Background(), "home_1", "agent_2", validReady()); err == nil { t.Fatal("wrong agent accepted") }
}
```

- [x] **Step 2: Run and verify the red state**

Run: `go test ./internal/cloud -run TestDesktopAgent -count=1`

Expected: compilation fails because desktop lifecycle routing is absent.

- [x] **Step 3: Implement event allowlisting and durable transitions**

`handleDesktopAgentEvent` accepts only the ten `EventDesktop*` constants. Decode ready events as `DesktopSessionReady` and all other lifecycle events as `DesktopSessionLifecycleEvent`; require protocol `desktop.v1`, exact home, exact agent, exact session, and exact key epoch. Map events as follows:

```go
var desktopEventTransitions = map[string]struct{ From []string; To string }{
	protocol.EventDesktopSessionReady: {[]string{"offered"}, "agent_ready"},
	protocol.EventDesktopSessionConnected: {[]string{"agent_ready", "joining", "reconnecting"}, "active"},
	protocol.EventDesktopSessionDisconnected: {[]string{"active"}, "reconnecting"},
	protocol.EventDesktopSessionError: {[]string{"requested", "offered", "agent_ready", "joining", "active", "reconnecting"}, "failed"},
	protocol.EventDesktopSessionTerminated: {[]string{"requested", "offered", "agent_ready", "joining", "active", "reconnecting"}, "terminated"},
}
```

Display, permission, secure-desktop, and stats events append metadata-only events but do not change durable state. Reject metadata keys outside a fixed allowlist and values over 256 bytes. Never persist a raw agent body.

- [x] **Step 4: Integrate with existing agent event handling**

In `handleAgentEvent`, dispatch events whose name begins with `desktop.` to `handleDesktopAgentEvent` before generic broadcast. Never broadcast desktop events to a home-wide topic; publish only to `desktop.session:{sessionID}` after verifying the authenticated operator owns the session. Add subscription authorization for that exact topic using a store ownership lookup.

- [x] **Step 5: Run control-plane routing gates and preserve the local changes**

Run: `go test ./internal/cloud -run 'TestDesktopAgent|TestDesktopSessionTopic' -count=1`

Expected: PASS.

```bash
git add internal/cloud/desktop_control.go internal/cloud/desktop_control_test.go internal/cloud/realtime.go internal/cloud/server.go
git commit -m "feat(remote-desktop): route session lifecycle"
```

### Task 10: Internal opaque-relay interface and limit contract

**Files:**
- Create: `internal/cloud/desktop_relay.go`
- Create: `internal/cloud/desktop_relay_test.go`

**Interfaces:**
- Consumes: single-use credential claims, desktop session state, service lifecycle transitions, and the approved data-plane boundary.
- Produces: replaceable `desktopRelay`, validated join claims, production limit configuration, lifecycle callbacks, and an explicit interface for the Milestone 2 in-process implementation.

- [x] **Step 1: Write failing interface and limit tests**

```go
func TestDesktopRelayJoinClaimBindsSessionSideEpochAndAgent(t *testing.T) {
	valid := desktopRelayJoinClaim{SessionID:"desk_0001", Side:desktopRelayBrowser, KeyEpoch:1, AgentID:"agent_1", HardExpiresAt:time.Now().Add(time.Hour)}
	if err := valid.Validate(time.Now()); err != nil { t.Fatalf("Validate: %v", err) }
	for _, mutate := range []func(*desktopRelayJoinClaim){
		func(value *desktopRelayJoinClaim) { value.SessionID = "" },
		func(value *desktopRelayJoinClaim) { value.Side = "server" },
		func(value *desktopRelayJoinClaim) { value.KeyEpoch = 0 },
		func(value *desktopRelayJoinClaim) { value.AgentID = "" },
		func(value *desktopRelayJoinClaim) { value.HardExpiresAt = time.Now().Add(-time.Second) },
	} {
		candidate := valid; mutate(&candidate)
		if err := candidate.Validate(time.Now()); err == nil { t.Fatalf("invalid claim accepted: %#v", candidate) }
	}
}

func TestDesktopRelayProductionLimitsAreBounded(t *testing.T) {
	limits := defaultDesktopRelayLimits()
	if err := limits.Validate(); err != nil { t.Fatalf("Validate: %v", err) }
	if limits.JoinTimeout != 60*time.Second || limits.MaxDuration != 8*time.Hour || limits.MaxFrameBytes != 4<<20 { t.Fatalf("unexpected limits: %#v", limits) }
}
```

- [x] **Step 2: Run and verify the red state**

Run: `go test ./internal/cloud -run TestDesktopRelay -count=1`

Expected: compilation fails because the relay is absent.

- [x] **Step 3: Define the replaceable relay interface, claims, callbacks, and limits**

```go
type desktopRelayLimits struct {
	JoinTimeout time.Duration
	IdleTimeout time.Duration
	MaxDuration time.Duration
	MaxFrameBytes int64
	MaxBytesPerSecond int64
	MaxSessions int
}

type desktopRelaySide string
const ( desktopRelayBrowser desktopRelaySide = "browser"; desktopRelayAgent desktopRelaySide = "agent" )
type desktopRelayJoinClaim struct { SessionID string; Side desktopRelaySide; KeyEpoch uint32; AgentID string; HardExpiresAt time.Time }
type desktopRelayLifecycleEvent struct { SessionID string; KeyEpoch uint32; Kind, Reason string; BrowserToAgentBytes, AgentToBrowserBytes int64 }
type desktopRelayLifecycleSink func(context.Context, desktopRelayLifecycleEvent)
type desktopRelayFactory func(desktopRelayLimits, desktopRelayLifecycleSink) desktopRelay
type desktopRelay interface {
	Join(context.Context, desktopRelayJoinClaim, desktopRelayEndpoint) error
	Revoke(sessionID, reason string)
	Snapshot(sessionID string) desktopRelaySnapshot
}
type desktopRelayEndpoint interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
	Close(reason string) error
}
```

Production defaults: join timeout 60 seconds, idle timeout 30 seconds, maximum duration eight hours, maximum binary frame 4 MiB, maximum opaque throughput 50 MiB/s per direction, and maximum active sessions 32 per process. `Validate` rejects non-positive values, a max duration over eight hours, a frame limit over 4 MiB, and an active-session limit over 32. Join claims reject unknown sides, zero epochs, expired sessions, and incomplete scope.

- [x] **Step 4: Add a fake relay to prove service decoupling and opaque callback metadata**

The production constructor accepts a `desktopRelayFactory` but Milestone 1 wires an unavailable implementation that returns `errDesktopRelayNotReady` if any data-plane join is attempted. Session APIs may reserve the future WebSocket path, but no `/ws/desktop/*` handler is registered. The fake used in service tests records only this metadata:

```go
type desktopRelaySnapshot struct {
	SessionID string
	KeyEpoch uint32
	BrowserConnected bool
	AgentConnected bool
	BrowserToAgentBytes int64
	AgentToBrowserBytes int64
	StartedAt time.Time
}
```

Add a test that sends a byte slice containing `desktop-fixture-payload` through the fake endpoint and asserts snapshots/events expose only byte counts and lifecycle fields, never payload bytes or string representations.

- [x] **Step 5: Document the Milestone 2 implementation obligations in code comments and tests**

The interface comment must require separate browser/agent authentication, exact side/session/epoch pairing, one connection per side, binary-only frames, join/idle/frame/bandwidth/duration/session limits, bidirectional opaque forwarding, payload-free metrics, and closing both sides on revoke/expire/failure. The test asserts this contract appears in the interface documentation so the Milestone 2 implementation cannot silently weaken it.

- [x] **Step 6: Run the relay contract tests**

Run: `go test ./internal/cloud -run TestDesktopRelay -count=1`

Expected: PASS.

- [x] **Step 7: Finalize the relay contract locally (commit deferred until explicitly authorized)**

```bash
git add internal/cloud/desktop_relay.go internal/cloud/desktop_relay_test.go
git commit -m "feat(remote-desktop): define opaque relay boundary"
```

### Task 11: Hankagent Swift and .NET contract conformance

**Files:**
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Fixtures/desktop-v1-test-vectors.json`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKit/DesktopProtocol.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKit/DesktopCryptoFixture.swift`
- Modify: `/Volumes/CampbellDrive/Projects/Hankagent/Sources/HankKitSelftest/main.swift`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Contracts/DesktopProtocol.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/src/HankAgent.Contracts/DesktopCryptoFixture.cs`
- Create: `/Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows/tests/HankAgent.Tests/DesktopContractTests.cs`

**Interfaces:**
- Consumes: `schemas/desktop/v1/test-vectors.json` and Task 1 control-plane JSON names.
- Produces: byte-for-byte compatible Swift/.NET handshake, identity-certificate, recovery-proof, digest, signature verification, HKDF, nonce, record-header, and AES-GCM fixture readers without advertising runtime capabilities.

- [x] **Step 1: Copy the canonical fixture and add a drift gate**

Use `cp` only for this byte-identical generated fixture copy, then assert hashes match:

```bash
mkdir -p /Volumes/CampbellDrive/Projects/Hankagent/Fixtures
cp schemas/desktop/v1/test-vectors.json /Volumes/CampbellDrive/Projects/Hankagent/Fixtures/desktop-v1-test-vectors.json
shasum -a 256 schemas/desktop/v1/test-vectors.json /Volumes/CampbellDrive/Projects/Hankagent/Fixtures/desktop-v1-test-vectors.json
```

Expected: both SHA-256 values are identical. Normal source edits still use `apply_patch`; this copy is an exact fixture synchronization step.

- [x] **Step 2: Write failing Swift self-test assertions**

```swift
let fixtureURL = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
    .appendingPathComponent("Fixtures/desktop-v1-test-vectors.json")
let fixture = try DesktopCryptoFixture.load(from: fixtureURL)
let transcript = try DesktopTranscript.encode(fixture.validInitialJoin.transcript.input)
precondition(transcript.base64URLEncodedString() == fixture.validInitialJoin.transcript.encodedBase64URL)
precondition(Data(SHA256.hash(data: transcript)).base64URLEncodedString() == fixture.validInitialJoin.transcript.sha256Base64URL)
precondition(try DesktopTrustFixture.verifyOperatorCertificate(fixture.identityCertificates))
precondition(try DesktopTrustFixture.verifyRecoveryEnrollment(fixture.recoveryEnrollment))
precondition(DesktopProtocol.commandSessionOffer == "desktop.session.offer")
```

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift run hankkit-selftest`

Expected: compilation fails because Swift desktop contract types are absent.

- [x] **Step 3: Implement Swift constants, Codable payloads, and encoding helpers**

Define `DesktopProtocol`, `DesktopPermission`, `DesktopSessionOffer`, `DesktopSessionReady`, `DesktopSessionLifecycleEvent`, `DesktopTranscript`, `DesktopIdentityCertificate`, `DesktopRecoveryEnrollment`, and `DesktopRecordHeader` with the exact JSON keys and byte order from Tasks 1 and 2. Use Foundation and CryptoKit only. Encoders must reject fields over 1 MiB and use big-endian UInt32 lengths; header must be exactly 18 bytes. Verify committed DER ECDSA signatures with P-256 public keys, derive all four HKDF outputs, and decrypt the committed AES-GCM record.

Do not add any `desktop.*` string to `WorkerAgent.currentCapabilities()` in this milestone.

- [x] **Step 4: Write failing .NET fixture tests**

```csharp
[Fact]
public void DesktopTranscriptAndRecordHeaderMatchCanonicalFixture()
{
    var fixture = DesktopCryptoFixture.Load(TestPaths.Repository("Fixtures/desktop-v1-test-vectors.json"));
    var transcript = DesktopTranscript.Encode(fixture.ValidInitialJoin.Transcript.ToInput());
    Assert.Equal(fixture.ValidInitialJoin.Transcript.EncodedBase64Url, Base64Url.Encode(transcript));
    Assert.Equal(fixture.ValidInitialJoin.Transcript.Sha256Base64Url, Base64Url.Encode(SHA256.HashData(transcript)));
    Assert.True(DesktopTrustFixture.VerifyOperatorCertificate(fixture.IdentityCertificates));
    Assert.True(DesktopTrustFixture.VerifyRecoveryEnrollment(fixture.RecoveryEnrollment));
    Assert.Equal(18, DesktopRecordHeader.CreateBrowserToAgent(1, 0, 24).Encode().Length);
    Assert.Equal("desktop.session.offer", DesktopProtocol.CommandSessionOffer);
}

[Fact]
public void WorkerDoesNotAdvertiseDesktopBeforeCoordinatorExists()
{
    var worker = WorkerFixture.Create();
    Assert.DoesNotContain(worker.Capabilities, value => value.StartsWith("desktop.", StringComparison.Ordinal));
}
```

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj --filter DesktopContractTests`

Expected: compilation fails because .NET desktop contract types are absent.

- [x] **Step 5: Implement .NET records and deterministic encoders**

Define `DesktopProtocol`, `DesktopPermission`, `DesktopSessionOffer`, `DesktopSessionReady`, `DesktopSessionLifecycleEvent`, `DesktopTranscript`, `DesktopIdentityCertificate`, `DesktopRecoveryEnrollment`, `DesktopRecordHeader`, `Base64Url`, and fixture records in `HankAgent.Contracts`. Use `BinaryPrimitives.WriteUInt32BigEndian`, `BinaryPrimitives.WriteUInt64BigEndian`, `ECDsa`, `SHA256.HashData`, `HKDF.DeriveKey`, `AesGcm`, and `System.Text.Json`; no new NuGet dependency is needed. Verify certificate/recovery signatures and decrypt the committed AES-GCM record.

Do not add any `desktop.*` string to `WorkerCommandDispatcher.Capabilities` in this milestone.

- [x] **Step 6: Run both platform conformance gates and preserve Hankagent locally**

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Expected: PASS and self-test prints its existing success marker.

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj`

Expected: PASS. If the system `dotnet` is unavailable, use the already-proven local .NET 8 SDK path or install-free SDK bootstrap documented by the project; report the exact SDK path.

```bash
git -C /Volumes/CampbellDrive/Projects/Hankagent add Fixtures/desktop-v1-test-vectors.json Sources/HankKit/DesktopProtocol.swift Sources/HankKit/DesktopCryptoFixture.swift Sources/HankKitSelftest/main.swift HankAgent-Windows/src/HankAgent.Contracts/DesktopProtocol.cs HankAgent-Windows/src/HankAgent.Contracts/DesktopCryptoFixture.cs HankAgent-Windows/tests/HankAgent.Tests/DesktopContractTests.cs
git -C /Volumes/CampbellDrive/Projects/Hankagent commit -m "feat(remote-desktop): consume v1 foundation contract"
```

### Task 12: Retention, audit redaction, documentation, and full foundation gate

**Files:**
- Modify: `internal/store/production_state.go`
- Modify: `internal/maintenance/lifecycle.go`
- Modify: `internal/cloud/audit_test.go`
- Modify: `docs/api.md`
- Modify: `docs/security.md`

**Interfaces:**
- Consumes: all prior tasks.
- Produces: bounded retention, complete metadata-only audit evidence, operator-facing contract documentation, and a verified Milestone 1 checkpoint.

- [x] **Step 1: Write failing lifecycle and audit-redaction tests**

```go
func TestLifecyclePrunesDesktopCredentialsBeforeMetadata(t *testing.T) {
	summary := runLifecycleWithDesktopRows(t)
	if summary.DesktopJoinCredentialsDeleted != 2 { t.Fatalf("credential deletes = %d", summary.DesktopJoinCredentialsDeleted) }
	if summary.DesktopSessionEventsDeleted != 1 { t.Fatalf("event deletes = %d", summary.DesktopSessionEventsDeleted) }
	if summary.DesktopSessionsDeleted != 1 { t.Fatalf("session deletes = %d", summary.DesktopSessionsDeleted) }
}

func TestDesktopAuditNeverIncludesSensitivePayloads(t *testing.T) {
	metadata := desktopAuditMetadataForTest("desk_1")
	encoded, _ := json.Marshal(metadata)
	for _, forbidden := range []string{"private_key", "recovery_secret", "recovery_envelope", "join_credential", "clipboard", "keystroke", "video", "ciphertext"} {
		if bytes.Contains(bytes.ToLower(encoded), []byte(forbidden)) { t.Fatalf("audit metadata contains %q", forbidden) }
	}
}
```

- [x] **Step 2: Run and verify the red state**

Run: `go test ./internal/store ./internal/maintenance ./internal/cloud -run 'TestLifecyclePrunesDesktop|TestDesktopAudit' -count=1`

Expected: compilation failure because lifecycle summary fields and prune wiring are absent.

- [x] **Step 3: Wire retention with conservative defaults**

Add summary fields `DesktopJoinCredentialsDeleted`, `DesktopSessionEventsDeleted`, and `DesktopSessionsDeleted`. In the existing lifecycle transaction, delete consumed/revoked/expired join credentials older than 24 hours, session events older than 180 days, and terminal sessions older than 365 days only after their events and credentials are gone. Never delete a live session. Include counts in existing structured lifecycle logs without IDs or metadata.

- [x] **Step 4: Document exact contracts and limitations**

In `docs/api.md`, document the HTTP paths, request/response fields, cookie/header credential transport, command/event names, state transitions, stable public reason codes, 60-second join, 90-second reconnect, and eight-hour hard maximum. In `docs/security.md`, document protected relay threats, visible metadata, offline recovery, cryptographic reset consequences, private-key storage ownership, content exclusions, and the explicit inability to protect against a server that replaces browser viewer JavaScript.

- [x] **Step 5: Run focused security gates and record database-only skips**

Run: `go test ./internal/protocol ./internal/desktopcrypto ./internal/cloud ./internal/store ./internal/maintenance -count=1`

Expected: PASS; PostgreSQL-backed tests must run when `HANK_REMOTE_TEST_DATABASE_URL` is configured and otherwise report SKIP.

Run: `make fmt && git diff --check`

Expected: PASS with no formatting or whitespace errors.

Run: `make migrate-status && make schema-drift-check`

Expected: PASS against the configured development database. If no database URL is configured, record both as skipped.

- [x] **Step 6: Run whole-repository and cross-repository gates**

Run: `go test ./... && make build`

Expected: PASS.

Run: `make frontend-test && make frontend-check && make frontend-build`

Expected: PASS; no browser viewer is added in Milestone 1.

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent && swift build && swift run hankkit-selftest`

Expected: PASS.

Run: `cd /Volumes/CampbellDrive/Projects/Hankagent/HankAgent-Windows && dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj`

Expected: PASS.

- [x] **Step 7: Inspect security-sensitive diffs before final commits**

Run:

```bash
git diff --check
git status --short
git diff --stat HEAD~12..HEAD
git -C /Volumes/CampbellDrive/Projects/Hankagent diff --check
git -C /Volumes/CampbellDrive/Projects/Hankagent status --short
```

Expected: only planned source/documentation changes plus the preserved HankServerside `.codex/` artifacts; no secrets, binaries, generated build directories, plaintext credentials, recovery secrets, or captured content.

- [x] **Step 8: Finalize retention and documentation locally (commit deferred until explicitly authorized)**

```bash
git add internal/store/production_state.go internal/maintenance/lifecycle.go internal/cloud/audit_test.go docs/api.md docs/security.md
git commit -m "docs(remote-desktop): complete foundation security gate"
```

Do not push either repository. Hankagent currently has no configured remote; HankServerside remains local-ahead until the user explicitly authorizes a push.

---

## Milestone 1 Exit Criteria

- [x] HankServerside owns a versioned, validated desktop control contract and canonical fixtures.
- [x] Go, Swift, and .NET produce byte-identical handshake, certificate, recovery, rotation, digest, HKDF, nonce, header, signature-verification, and AES-GCM fixture results.
- [x] Five migration-backed tables enforce ownership, identity shape, permission, state, side, epoch, and one-live-session constraints.
- [x] The server stores only public trust material, encrypted recovery envelopes, hashes, metadata, and byte counts.
- [x] Trust setup, approval, recovery-envelope replacement, revocation, rotation, and reset are administrator/CSRF protected and audited.
- [x] Password reset cannot replace the trust root; cryptographic reset revokes all operator and endpoint identities and live sessions.
- [x] Session creation requires a trusted operator device, trusted endpoint identity, online capable agent, home scope, and administrator role.
- [x] Browser/agent credentials are separate, random, hashed, single-use, URL-free, side/session/epoch-bound, and expired at the correct deadlines.
- [x] The durable state machine rejects illegal and terminal-to-live transitions under concurrency.
- [x] The relay interface fixes exact side/session/epoch claims, opaque-only forwarding semantics, payload-free metrics, production limits, and revoke/expire/failure callbacks; no binary WebSocket endpoint is served until Milestone 2.
- [x] Audit, logs, persistence, and errors contain no private keys, recovery secrets, join credentials, screen/video/input/clipboard content, or raw ciphertext samples.
- [x] Hankagent parses the contract and fixtures on both macOS and Windows but advertises no desktop capability yet.
- [x] Targeted, whole Go, frontend, Swift, and .NET gates pass; database-dependent skips are reported explicitly.
- [x] No deployment, installation, publish, tag, or push has occurred.

## Next Plan Boundary

Milestone 2 receives a separate plan after this exit gate passes. It will add the browser viewer shell, browser and agent cryptographic implementations, the encrypted inner record layer, deterministic synthetic H.264 providers on Windows and macOS, reconnect across fresh epochs, and proof that the server cannot recover known plaintext. Native capture, native input, privileged helpers, UAC, and macOS permission workflows remain outside this foundation plan.
