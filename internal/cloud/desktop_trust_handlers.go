package cloud

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/desktopcrypto"
	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

const desktopTrustMaxBlobBytes = 64 << 10

var desktopRequiredAdminCapabilities = []string{
	"operator.approve", "endpoint.approve", "trust.recover", "trust.rotate",
}

var desktopOperatorCapabilities = map[string]bool{
	"operator.approve": true, "endpoint.approve": true, "trust.recover": true, "trust.rotate": true,
}

var desktopEndpointCapabilities = map[string]bool{
	"desktop.view": true, "desktop.control": true, "desktop.clipboard.read": true,
	"desktop.clipboard.write": true, "desktop.elevate": true, "desktop.secure_desktop": true,
	"desktop.unattended": true,
}

type desktopOperatorDeviceRequest struct {
	IdentityID    string    `json:"identity_id"`
	DeviceID      string    `json:"device_id"`
	PublicKeySPKI string    `json:"public_key_spki"`
	Certificate   string    `json:"certificate"`
	Capabilities  []string  `json:"capabilities"`
	ExpiresAt     time.Time `json:"expires_at"`
	Confirmation  string    `json:"confirmation"`
}

type desktopTrustBootstrapRequest struct {
	Generation       int                          `json:"generation"`
	PublicKeySPKI    string                       `json:"public_key_spki"`
	RecoveryEnvelope string                       `json:"recovery_envelope"`
	FirstOperator    desktopOperatorDeviceRequest `json:"first_operator"`
	Confirmation     string                       `json:"confirmation"`
}

type desktopEndpointApprovalRequest struct {
	IdentityID    string    `json:"identity_id"`
	PublicKeySPKI string    `json:"public_key_spki"`
	Certificate   string    `json:"certificate"`
	Capabilities  []string  `json:"capabilities"`
	ExpiresAt     time.Time `json:"expires_at"`
	Confirmation  string    `json:"confirmation"`
}

type desktopRevocationRequest struct {
	Reason       string `json:"reason"`
	Confirmation string `json:"confirmation"`
}

type desktopRecoveryRequest struct {
	Generation    int                          `json:"generation"`
	Operator      desktopOperatorDeviceRequest `json:"operator"`
	Challenge     string                       `json:"challenge"`
	RootSignature string                       `json:"root_signature"`
	Confirmation  string                       `json:"confirmation"`
}

type desktopRotationRequest struct {
	Generation          int                          `json:"generation"`
	PublicKeySPKI       string                       `json:"public_key_spki"`
	RecoveryEnvelope    string                       `json:"recovery_envelope"`
	ReplacementOperator desktopOperatorDeviceRequest `json:"replacement_operator"`
	OldRootSignature    string                       `json:"old_root_signature"`
	Confirmation        string                       `json:"confirmation"`
}

type desktopResetRequest struct {
	Generation          int                          `json:"generation"`
	PublicKeySPKI       string                       `json:"public_key_spki"`
	RecoveryEnvelope    string                       `json:"recovery_envelope"`
	ReplacementOperator desktopOperatorDeviceRequest `json:"replacement_operator"`
	Confirmation        string                       `json:"confirmation"`
}

type desktopSignedCertificateEnvelope struct {
	Claims            string `json:"claims"`
	Signature         string `json:"signature"`
	IssuerCertificate string `json:"issuer_certificate,omitempty"`
}

type validatedDesktopCertificate struct {
	Identity     domain.DesktopIdentity
	SignedClaims []byte
	Signature    []byte
}

func (s *Server) handleHomeDesktopTrust(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, membership domain.HomeMembership, parts []string) bool {
	if len(parts) == 0 || parts[0] != "desktop-trust" {
		return false
	}
	if membership.Role != domain.HomeRoleAdmin {
		http.Error(w, errDesktopAdminRequired.Error(), http.StatusForbidden)
		return true
	}
	if r.Method == http.MethodGet && len(parts) == 1 {
		s.handleDesktopTrustGet(w, r, home)
		return true
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}

	switch {
	case len(parts) == 2 && parts[1] == "operator-devices":
		s.handleDesktopOperatorApproval(w, r, home, auth)
	case len(parts) == 4 && parts[1] == "operator-devices" && parts[2] != "" && parts[3] == "revoke":
		s.handleDesktopIdentityRevocation(w, r, home, auth, domain.DesktopIdentityOperatorDevice, parts[2])
	case len(parts) == 4 && parts[1] == "endpoints" && parts[2] != "" && parts[3] == "approve":
		s.handleDesktopEndpointAction(w, r, home, auth, parts[2], false)
	case len(parts) == 4 && parts[1] == "endpoints" && parts[2] != "" && parts[3] == "revoke":
		s.handleDesktopEndpointAction(w, r, home, auth, parts[2], true)
	case len(parts) == 2 && parts[1] == "recovery":
		s.handleDesktopRecovery(w, r, home, auth)
	case len(parts) == 2 && parts[1] == "rotate":
		s.handleDesktopRotation(w, r, home, auth)
	case len(parts) == 2 && parts[1] == "reset":
		s.handleDesktopReset(w, r, home, auth)
	default:
		http.NotFound(w, r)
	}
	return true
}

func (s *Server) handleDesktopTrustGet(w http.ResponseWriter, r *http.Request, home domain.Home) {
	root, err := s.store.GetDesktopTrustRoot(r.Context(), home.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false, "identities": []any{}})
		return
	}
	if err != nil {
		http.Error(w, "desktop trust unavailable", http.StatusInternalServerError)
		return
	}
	identities, err := s.store.ListDesktopIdentities(r.Context(), home.ID)
	if err != nil {
		http.Error(w, "desktop trust unavailable", http.StatusInternalServerError)
		return
	}
	items := make([]map[string]any, 0, len(identities))
	now := time.Now().UTC()
	for _, identity := range identities {
		if identity.RevokedAt != nil || !identity.ExpiresAt.After(now) || identity.TrustRootGeneration != root.Generation {
			continue
		}
		items = append(items, desktopIdentityPublicSnapshot(identity))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"root":       map[string]any{"generation": root.Generation, "algorithm": root.Algorithm, "fingerprint": root.Fingerprint, "public_key_spki": base64.RawURLEncoding.EncodeToString(root.PublicKeySPKI), "recovery_envelope": base64.RawURLEncoding.EncodeToString(root.RecoveryEnvelope), "created_at": root.CreatedAt, "rotated_at": root.RotatedAt},
		"identities": items,
	})
}

func (s *Server) handleDesktopOperatorApproval(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	var raw json.RawMessage
	if err := parseJSON(w, r, &raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(raw, &shape); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if _, bootstrap := shape["first_operator"]; bootstrap {
		var body desktopTrustBootstrapRequest
		if err := json.Unmarshal(raw, &body); err != nil || body.Confirmation != "create desktop trust" || body.Generation != 1 {
			http.Error(w, "invalid desktop trust bootstrap", http.StatusBadRequest)
			return
		}
		rootKey, rootSPKI, err := decodeDesktopP256SPKI(body.PublicKeySPKI)
		envelope, envelopeErr := decodeDesktopBlob(body.RecoveryEnvelope)
		certificate, certificateErr := validateDesktopOperatorRequest(body.FirstOperator, home.ID, auth.User.ID, 1, time.Now().UTC(), true)
		if err != nil || envelopeErr != nil || certificateErr != nil || desktopcrypto.VerifyP256Signature(rootKey, certificate.SignedClaims, certificate.Signature) != nil {
			http.Error(w, "invalid desktop trust bootstrap", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		root := domain.DesktopTrustRoot{HomeID: home.ID, Generation: 1, Algorithm: domain.DesktopTrustAlgorithm, PublicKeySPKI: rootSPKI, Fingerprint: desktopcrypto.FingerprintSPKI(rootSPKI), RecoveryEnvelope: envelope, CreatedAt: now}
		if err := s.store.BootstrapDesktopTrust(r.Context(), root, certificate.Identity); err != nil {
			writeDesktopStoreError(w, err)
			return
		}
		s.audit(r.Context(), "desktop.trust.created", auditSeverityCritical, auth.User.ID, "", home.ID, requestIDFromContext(r.Context()), "desktop_trust", home.ID, desktopAuditMetadata(certificate.Identity.IdentityType, certificate.Identity.ID, certificate.Identity.Fingerprint, 1, certificate.Identity.Capabilities, "created"))
		writeJSON(w, http.StatusCreated, map[string]any{"root": map[string]any{"generation": 1, "fingerprint": root.Fingerprint}, "operator": desktopIdentityPublicSnapshot(certificate.Identity)})
		return
	}

	root, err := s.store.GetDesktopTrustRoot(r.Context(), home.ID)
	if err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	var body desktopOperatorDeviceRequest
	if err := json.Unmarshal(raw, &body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	certificate, err := validateDesktopOperatorRequest(body, home.ID, auth.User.ID, root.Generation, time.Now().UTC(), false)
	if err != nil {
		http.Error(w, "desktop operator approval is invalid", http.StatusBadRequest)
		return
	}
	approver, approverErr := s.verifyDesktopApprover(r, home.ID, root.Generation, "operator.approve", certificate.SignedClaims, certificate.Signature)
	if approverErr != nil {
		http.Error(w, "desktop operator approval is invalid", http.StatusBadRequest)
		return
	}
	certificate, err = certificateWithIssuer(certificate, approver.Certificate)
	if err != nil {
		http.Error(w, "desktop operator approval is invalid", http.StatusBadRequest)
		return
	}
	existing, existingErr := s.store.GetActiveDesktopOperatorIdentity(r.Context(), home.ID, auth.User.ID, body.DeviceID, time.Now().UTC())
	if existingErr == nil {
		if existing.Fingerprint == certificate.Identity.Fingerprint {
			http.Error(w, "desktop operator identity is already approved", http.StatusConflict)
			return
		}
		if body.Confirmation != "replace changed desktop identity" {
			http.Error(w, "changed desktop operator requires explicit replacement confirmation", http.StatusConflict)
			return
		}
		sessions, err := s.store.ReplaceDesktopIdentity(r.Context(), existing.ID, certificate.Identity, time.Now().UTC(), "identity_replaced")
		if err != nil {
			writeDesktopStoreError(w, err)
			return
		}
		s.revokeDesktopRelays(sessions, "revoked")
	} else if errors.Is(existingErr, store.ErrNotFound) {
		if err := s.store.CreateDesktopIdentity(r.Context(), certificate.Identity); err != nil {
			writeDesktopStoreError(w, err)
			return
		}
	} else {
		writeDesktopStoreError(w, existingErr)
		return
	}
	s.auditDesktopIdentity(r, auth, home.ID, "desktop.identity.approved", certificate.Identity, "approved")
	writeJSON(w, http.StatusCreated, desktopIdentityPublicSnapshot(certificate.Identity))
}

func (s *Server) handleDesktopEndpointAction(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, agentID string, revoking bool) {
	var raw map[string]json.RawMessage
	if err := parseJSON(w, r, &raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if revoking {
		encoded, _ := json.Marshal(raw)
		r.Body = http.NoBody
		var body desktopRevocationRequest
		if json.Unmarshal(encoded, &body) != nil || body.Confirmation != "revoke desktop identity" {
			http.Error(w, "invalid revocation confirmation", http.StatusBadRequest)
			return
		}
		s.revokeDesktopIdentity(w, r, home, auth, domain.DesktopIdentityEndpoint, agentID, body)
		return
	}
	encoded, _ := json.Marshal(raw)
	var body desktopEndpointApprovalRequest
	if json.Unmarshal(encoded, &body) != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	root, err := s.store.GetDesktopTrustRoot(r.Context(), home.ID)
	if err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	certificate, err := validateDesktopEndpointRequest(body, home.ID, agentID, root.Generation, time.Now().UTC())
	if err != nil {
		http.Error(w, "desktop endpoint approval is invalid", http.StatusBadRequest)
		return
	}
	approver, approverErr := s.verifyDesktopApprover(r, home.ID, root.Generation, "endpoint.approve", certificate.SignedClaims, certificate.Signature)
	if approverErr != nil {
		http.Error(w, "desktop endpoint approval is invalid", http.StatusBadRequest)
		return
	}
	certificate, err = certificateWithIssuer(certificate, approver.Certificate)
	if err != nil {
		http.Error(w, "desktop endpoint approval is invalid", http.StatusBadRequest)
		return
	}
	storedAgent, err := s.store.GetAgentByID(r.Context(), agentID)
	if err != nil || storedAgent.HomeID != home.ID {
		http.Error(w, "desktop endpoint does not belong to home", http.StatusBadRequest)
		return
	}
	existing, existingErr := s.store.GetActiveDesktopEndpointIdentity(r.Context(), home.ID, agentID, time.Now().UTC())
	if existingErr == nil {
		if existing.Fingerprint == certificate.Identity.Fingerprint {
			http.Error(w, "desktop endpoint identity is already approved", http.StatusConflict)
			return
		}
		if body.Confirmation != "replace changed desktop identity" {
			http.Error(w, "changed desktop endpoint requires explicit replacement confirmation", http.StatusConflict)
			return
		}
		sessions, err := s.store.ReplaceDesktopIdentity(r.Context(), existing.ID, certificate.Identity, time.Now().UTC(), "identity_replaced")
		if err != nil {
			writeDesktopStoreError(w, err)
			return
		}
		s.revokeDesktopRelays(sessions, "revoked")
	} else if errors.Is(existingErr, store.ErrNotFound) {
		if err := s.store.CreateDesktopIdentity(r.Context(), certificate.Identity); err != nil {
			writeDesktopStoreError(w, err)
			return
		}
	} else {
		writeDesktopStoreError(w, existingErr)
		return
	}
	s.auditDesktopIdentity(r, auth, home.ID, "desktop.identity.approved", certificate.Identity, "approved")
	writeJSON(w, http.StatusCreated, desktopIdentityPublicSnapshot(certificate.Identity))
}

func (s *Server) handleDesktopIdentityRevocation(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, identityType, pathID string) {
	var body desktopRevocationRequest
	if err := parseJSON(w, r, &body); err != nil || body.Confirmation != "revoke desktop identity" {
		http.Error(w, "invalid revocation confirmation", http.StatusBadRequest)
		return
	}
	s.revokeDesktopIdentity(w, r, home, auth, identityType, pathID, body)
}

func (s *Server) revokeDesktopIdentity(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, identityType, pathID string, body desktopRevocationRequest) {
	identities, err := s.store.ListDesktopIdentities(r.Context(), home.ID)
	if err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	var target domain.DesktopIdentity
	for _, identity := range identities {
		matches := identity.IdentityType == identityType && ((identityType == domain.DesktopIdentityOperatorDevice && identity.DeviceID == pathID) || (identityType == domain.DesktopIdentityEndpoint && identity.AgentID == pathID))
		if matches && identity.RevokedAt == nil {
			target = identity
			break
		}
	}
	if target.ID == "" {
		http.NotFound(w, r)
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		reason = "administrator_revoked"
	}
	if !validDesktopReasonCode(reason) {
		http.Error(w, "invalid desktop revocation reason code", http.StatusBadRequest)
		return
	}
	operatorIdentityID, agentID := "", ""
	if target.IdentityType == domain.DesktopIdentityOperatorDevice {
		operatorIdentityID = target.ID
	} else {
		agentID = target.AgentID
	}
	liveSessions, err := s.store.ListLiveDesktopSessionIDs(r.Context(), home.ID, operatorIdentityID, agentID)
	if err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	changed, committedSessions, err := s.store.RevokeDesktopIdentity(r.Context(), home.ID, target.ID, reason, time.Now().UTC())
	if err != nil || !changed {
		writeDesktopStoreError(w, err)
		return
	}
	if len(committedSessions) > 0 {
		liveSessions = committedSessions
	}
	s.revokeDesktopRelays(liveSessions, "revoked")
	s.auditDesktopIdentity(r, auth, home.ID, "desktop.identity.revoked", target, reason)
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true})
}

func (s *Server) handleDesktopRecovery(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	var body desktopRecoveryRequest
	if err := parseJSON(w, r, &body); err != nil || body.Confirmation != "recover desktop trust" {
		http.Error(w, "invalid recovery confirmation", http.StatusBadRequest)
		return
	}
	root, err := s.store.GetDesktopTrustRoot(r.Context(), home.ID)
	if err != nil || body.Generation != root.Generation {
		http.Error(w, "desktop recovery generation mismatch", http.StatusConflict)
		return
	}
	if strings.TrimSpace(body.Challenge) == "" && strings.TrimSpace(body.RootSignature) == "" {
		challenge := make([]byte, 32)
		if _, err := rand.Read(challenge); err != nil {
			http.Error(w, "recovery challenge unavailable", http.StatusInternalServerError)
			return
		}
		hash := sha256.Sum256(challenge)
		if err := s.store.IssueDesktopRecoveryChallenge(r.Context(), home.ID, hash[:], time.Now().UTC().Add(5*time.Minute)); err != nil {
			writeDesktopStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"challenge": base64.RawURLEncoding.EncodeToString(challenge), "expires_in": 300})
		return
	}
	certificate, err := validateDesktopOperatorRequest(body.Operator, home.ID, auth.User.ID, root.Generation, time.Now().UTC(), true)
	challenge, challengeErr := desktopcrypto.DecodeRawBase64URL(body.Challenge)
	signature, signatureErr := desktopcrypto.DecodeRawBase64URL(body.RootSignature)
	rootKey, keyErr := parseDesktopP256SPKI(root.PublicKeySPKI)
	if err != nil || challengeErr != nil || len(challenge) != 32 || signatureErr != nil || keyErr != nil || desktopcrypto.VerifyP256Signature(rootKey, certificate.SignedClaims, certificate.Signature) != nil {
		http.Error(w, "invalid desktop recovery proof", http.StatusBadRequest)
		return
	}
	proof, err := desktopcrypto.EncodeRecoveryEnrollmentProof(desktopcrypto.RecoveryEnrollmentClaims{
		Label: "recover_operator_device", HomeID: home.ID, TrustRootGeneration: uint32(root.Generation),
		NewOperatorIdentityID: certificate.Identity.ID, NewOperatorDeviceID: certificate.Identity.DeviceID,
		NewOperatorPublicKeySPKI: certificate.Identity.PublicKeySPKI, IssuedAtUnixMS: certificate.Identity.CreatedAt.UnixMilli(), Challenge: challenge,
	})
	if err != nil || desktopcrypto.VerifyP256Signature(rootKey, proof, signature) != nil {
		http.Error(w, "invalid desktop recovery proof", http.StatusBadRequest)
		return
	}
	hash := sha256.Sum256(challenge)
	if err := s.store.ConsumeDesktopRecoveryChallengeAndCreateOperator(r.Context(), home.ID, root.Generation, hash[:], time.Now().UTC(), certificate.Identity); err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	s.auditDesktopIdentity(r, auth, home.ID, "desktop.recovery.completed", certificate.Identity, "recovered")
	writeJSON(w, http.StatusCreated, desktopIdentityPublicSnapshot(certificate.Identity))
}

func (s *Server) handleDesktopRotation(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	var body desktopRotationRequest
	if err := parseJSON(w, r, &body); err != nil || body.Confirmation != "rotate desktop trust" {
		http.Error(w, "invalid rotation confirmation", http.StatusBadRequest)
		return
	}
	oldRoot, err := s.store.GetDesktopTrustRoot(r.Context(), home.ID)
	newRootKey, newRootSPKI, keyErr := decodeDesktopP256SPKI(body.PublicKeySPKI)
	envelope, envelopeErr := decodeDesktopBlob(body.RecoveryEnvelope)
	certificate, certificateErr := validateDesktopOperatorRequest(body.ReplacementOperator, home.ID, auth.User.ID, body.Generation, time.Now().UTC(), true)
	signature, signatureErr := desktopcrypto.DecodeRawBase64URL(body.OldRootSignature)
	oldRootKey, oldKeyErr := parseDesktopP256SPKI(oldRoot.PublicKeySPKI)
	if err != nil || keyErr != nil || envelopeErr != nil || certificateErr != nil || signatureErr != nil || oldKeyErr != nil || body.Generation != oldRoot.Generation+1 || desktopcrypto.VerifyP256Signature(newRootKey, certificate.SignedClaims, certificate.Signature) != nil {
		http.Error(w, "invalid desktop trust rotation", http.StatusBadRequest)
		return
	}
	envelopeHash := sha256.Sum256(envelope)
	proof, err := desktopcrypto.EncodeRootRotationProof(desktopcrypto.RootRotationClaims{
		HomeID: home.ID, OldGeneration: uint32(oldRoot.Generation), NewGeneration: uint32(body.Generation), NewRootPublicKeySPKI: newRootSPKI,
		NewRecoveryEnvelopeHash: envelopeHash[:], ReplacementOperatorIdentityID: certificate.Identity.ID, IssuedAtUnixMS: certificate.Identity.CreatedAt.UnixMilli(),
	})
	if err != nil || desktopcrypto.VerifyP256Signature(oldRootKey, proof, signature) != nil {
		http.Error(w, "invalid desktop trust rotation proof", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	liveSessions, listErr := s.store.ListLiveDesktopSessionIDs(r.Context(), home.ID, "", "")
	if listErr != nil {
		writeDesktopStoreError(w, listErr)
		return
	}
	root := domain.DesktopTrustRoot{HomeID: home.ID, Generation: body.Generation, Algorithm: domain.DesktopTrustAlgorithm, PublicKeySPKI: newRootSPKI, Fingerprint: desktopcrypto.FingerprintSPKI(newRootSPKI), RecoveryEnvelope: envelope, CreatedAt: oldRoot.CreatedAt, RotatedAt: &now}
	if err := s.store.RotateDesktopTrust(r.Context(), root, certificate.Identity, now, "trust_rotated"); err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	s.revokeDesktopRelays(liveSessions, "revoked")
	s.auditDesktopIdentity(r, auth, home.ID, "desktop.trust.rotated", certificate.Identity, "trust_rotated")
	writeJSON(w, http.StatusOK, map[string]any{"generation": root.Generation, "fingerprint": root.Fingerprint})
}

func (s *Server) handleDesktopReset(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	var body desktopResetRequest
	if err := parseJSON(w, r, &body); err != nil || body.Confirmation != "reset desktop trust" {
		http.Error(w, "invalid reset confirmation", http.StatusBadRequest)
		return
	}
	oldRoot, err := s.store.GetDesktopTrustRoot(r.Context(), home.ID)
	newRootKey, newRootSPKI, keyErr := decodeDesktopP256SPKI(body.PublicKeySPKI)
	envelope, envelopeErr := decodeDesktopBlob(body.RecoveryEnvelope)
	certificate, certificateErr := validateDesktopOperatorRequest(body.ReplacementOperator, home.ID, auth.User.ID, body.Generation, time.Now().UTC(), true)
	if err != nil || keyErr != nil || envelopeErr != nil || certificateErr != nil || body.Generation != oldRoot.Generation+1 || desktopcrypto.VerifyP256Signature(newRootKey, certificate.SignedClaims, certificate.Signature) != nil {
		http.Error(w, "invalid desktop trust reset", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	liveSessions, listErr := s.store.ListLiveDesktopSessionIDs(r.Context(), home.ID, "", "")
	if listErr != nil {
		writeDesktopStoreError(w, listErr)
		return
	}
	root := domain.DesktopTrustRoot{HomeID: home.ID, Generation: body.Generation, Algorithm: domain.DesktopTrustAlgorithm, PublicKeySPKI: newRootSPKI, Fingerprint: desktopcrypto.FingerprintSPKI(newRootSPKI), RecoveryEnvelope: envelope, CreatedAt: oldRoot.CreatedAt, RotatedAt: &now}
	if err := s.store.ResetDesktopTrust(r.Context(), root, certificate.Identity, now, "cryptographic_reset"); err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	s.revokeDesktopRelays(liveSessions, "revoked")
	s.auditDesktopIdentity(r, auth, home.ID, "desktop.trust.reset", certificate.Identity, "cryptographic_reset")
	writeJSON(w, http.StatusOK, map[string]any{"generation": root.Generation, "fingerprint": root.Fingerprint})
}

func (s *Server) revokeDesktopRelays(sessionIDs []string, reason string) {
	if s.desktopRelay == nil {
		return
	}
	for _, sessionID := range sessionIDs {
		s.desktopRelay.Revoke(sessionID, reason)
	}
}

func validateDesktopOperatorRequest(request desktopOperatorDeviceRequest, homeID, userID string, generation int, now time.Time, requireAll bool) (validatedDesktopCertificate, error) {
	return validateDesktopCertificate(request.IdentityID, domain.DesktopIdentityOperatorDevice, homeID, userID, request.DeviceID, "", request.PublicKeySPKI, request.Certificate, request.Capabilities, request.ExpiresAt, generation, now, requireAll)
}

func validateDesktopEndpointRequest(request desktopEndpointApprovalRequest, homeID, agentID string, generation int, now time.Time) (validatedDesktopCertificate, error) {
	return validateDesktopCertificate(request.IdentityID, domain.DesktopIdentityEndpoint, homeID, "", "", agentID, request.PublicKeySPKI, request.Certificate, request.Capabilities, request.ExpiresAt, generation, now, false)
}

func validateDesktopCertificate(identityID, identityType, homeID, userID, deviceID, agentID, encodedSPKI, encodedEnvelope string, capabilities []string, expiresAt time.Time, generation int, now time.Time, requireAll bool) (validatedDesktopCertificate, error) {
	_, spki, err := decodeDesktopP256SPKI(encodedSPKI)
	if err != nil || generation <= 0 || strings.TrimSpace(identityID) == "" {
		return validatedDesktopCertificate{}, errors.New("invalid desktop identity key or scope")
	}
	if err := validateDesktopCapabilities(identityType, capabilities, requireAll); err != nil {
		return validatedDesktopCertificate{}, err
	}
	envelopeBytes, err := decodeDesktopBlob(encodedEnvelope)
	if err != nil {
		return validatedDesktopCertificate{}, err
	}
	var envelope desktopSignedCertificateEnvelope
	if err := json.Unmarshal(envelopeBytes, &envelope); err != nil {
		return validatedDesktopCertificate{}, errors.New("invalid signed certificate envelope")
	}
	claimsBytes, err := desktopcrypto.DecodeRawBase64URL(envelope.Claims)
	if err != nil || len(claimsBytes) > desktopTrustMaxBlobBytes {
		return validatedDesktopCertificate{}, errors.New("invalid certificate claims")
	}
	signature, err := desktopcrypto.DecodeRawBase64URL(envelope.Signature)
	if err != nil {
		return validatedDesktopCertificate{}, errors.New("invalid certificate signature")
	}
	canonicalEnvelope, err := json.Marshal(desktopSignedCertificateEnvelope{Claims: envelope.Claims, Signature: envelope.Signature})
	if err != nil {
		return validatedDesktopCertificate{}, errors.New("invalid signed certificate envelope")
	}
	claims, err := desktopcrypto.DecodeIdentityCertificate(claimsBytes)
	if err != nil {
		return validatedDesktopCertificate{}, err
	}
	createdAt := time.UnixMilli(claims.CreatedAtUnixMS).UTC()
	claimExpiry := time.UnixMilli(claims.ExpiresAtUnixMS).UTC()
	if claims.CertificateVersion != "desktop.v1" || claims.HomeID != homeID || claims.IdentityID != identityID || claims.IdentityType != identityType ||
		claims.UserID != userID || claims.DeviceID != deviceID || claims.AgentID != agentID || !bytes.Equal(claims.PublicKeySPKI, spki) ||
		!slices.Equal(claims.Capabilities, capabilities) || int(claims.TrustRootGeneration) != generation || !claimExpiry.Equal(expiresAt.UTC()) ||
		createdAt.After(now.Add(5*time.Minute)) || !claimExpiry.After(now) || claimExpiry.Sub(createdAt) > 2*365*24*time.Hour {
		return validatedDesktopCertificate{}, errors.New("desktop certificate claims do not match request")
	}
	identity := domain.DesktopIdentity{
		ID: identityID, HomeID: homeID, IdentityType: identityType, UserID: userID, DeviceID: deviceID, AgentID: agentID,
		PublicKeySPKI: spki, Certificate: canonicalEnvelope, Fingerprint: desktopcrypto.FingerprintSPKI(spki),
		Capabilities: append([]string(nil), capabilities...), TrustRootGeneration: generation, CreatedAt: createdAt, ExpiresAt: claimExpiry,
	}
	return validatedDesktopCertificate{Identity: identity, SignedClaims: claimsBytes, Signature: signature}, nil
}

func validateDesktopCapabilities(identityType string, capabilities []string, requireAll bool) error {
	if len(capabilities) == 0 || len(capabilities) > 32 {
		return errors.New("invalid desktop capability count")
	}
	allowed := desktopOperatorCapabilities
	if identityType == domain.DesktopIdentityEndpoint {
		allowed = desktopEndpointCapabilities
	}
	seen := map[string]bool{}
	for _, capability := range capabilities {
		if len(capability) == 0 || len(capability) > 128 || !allowed[capability] || seen[capability] {
			return errors.New("invalid desktop capability")
		}
		seen[capability] = true
	}
	if requireAll {
		for _, capability := range desktopRequiredAdminCapabilities {
			if !seen[capability] {
				return errors.New("replacement desktop administrator lacks required capability")
			}
		}
	}
	return nil
}

func decodeDesktopP256SPKI(value string) (*ecdsa.PublicKey, []byte, error) {
	spki, err := desktopcrypto.DecodeRawBase64URL(strings.TrimSpace(value))
	if err != nil {
		return nil, nil, err
	}
	if len(spki) == 0 || len(spki) > desktopTrustMaxBlobBytes {
		return nil, nil, errors.New("invalid or oversized desktop public key")
	}
	key, err := parseDesktopP256SPKI(spki)
	return key, spki, err
}

func parseDesktopP256SPKI(spki []byte) (*ecdsa.PublicKey, error) {
	parsed, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("P-256 SPKI public key required")
	}
	return key, nil
}

func decodeDesktopBlob(value string) ([]byte, error) {
	decoded, err := desktopcrypto.DecodeRawBase64URL(strings.TrimSpace(value))
	if err != nil || len(decoded) == 0 || len(decoded) > desktopTrustMaxBlobBytes {
		return nil, errors.New("invalid or oversized desktop trust value")
	}
	return decoded, nil
}

func (s *Server) verifyDesktopApprover(r *http.Request, homeID string, generation int, capability string, signed, signature []byte) (domain.DesktopIdentity, error) {
	identities, err := s.store.ListDesktopIdentities(r.Context(), homeID)
	if err != nil {
		return domain.DesktopIdentity{}, err
	}
	now := time.Now().UTC()
	for _, identity := range identities {
		if identity.IdentityType != domain.DesktopIdentityOperatorDevice || identity.TrustRootGeneration != generation || identity.RevokedAt != nil || !identity.ExpiresAt.After(now) || !slices.Contains(identity.Capabilities, capability) {
			continue
		}
		if !desktopCertificateChainIsActive(identity.Certificate, homeID, generation, now, identities) {
			continue
		}
		key, err := parseDesktopP256SPKI(identity.PublicKeySPKI)
		if err == nil && desktopcrypto.VerifyP256Signature(key, signed, signature) == nil {
			return identity, nil
		}
	}
	return domain.DesktopIdentity{}, errors.New("no active desktop approver accepted the certificate")
}

func certificateWithIssuer(certificate validatedDesktopCertificate, issuerCertificate []byte) (validatedDesktopCertificate, error) {
	if len(issuerCertificate) == 0 {
		return validatedDesktopCertificate{}, errors.New("desktop certificate issuer is missing")
	}
	var envelope desktopSignedCertificateEnvelope
	if err := json.Unmarshal(certificate.Identity.Certificate, &envelope); err != nil || envelope.Claims == "" || envelope.Signature == "" {
		return validatedDesktopCertificate{}, errors.New("desktop certificate envelope is invalid")
	}
	envelope.IssuerCertificate = base64.RawURLEncoding.EncodeToString(issuerCertificate)
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > desktopTrustMaxBlobBytes {
		return validatedDesktopCertificate{}, errors.New("desktop certificate chain is invalid")
	}
	certificate.Identity.Certificate = encoded
	return certificate, nil
}

func desktopIdentityPublicSnapshot(identity domain.DesktopIdentity) map[string]any {
	return map[string]any{
		"identity_id": identity.ID, "identity_type": identity.IdentityType, "user_id": identity.UserID,
		"device_id": identity.DeviceID, "agent_id": identity.AgentID, "fingerprint": identity.Fingerprint,
		"public_key_spki": base64.RawURLEncoding.EncodeToString(identity.PublicKeySPKI),
		"certificate":     base64.RawURLEncoding.EncodeToString(identity.Certificate),
		"capabilities":    nonNilSlice(identity.Capabilities), "trust_root_generation": identity.TrustRootGeneration,
		"created_at": identity.CreatedAt, "expires_at": identity.ExpiresAt, "revoked_at": identity.RevokedAt,
		"revocation_reason": identity.RevocationReason,
	}
}

func desktopAuditMetadata(identityType, identityID, fingerprint string, generation int, capabilities []string, reason string) map[string]any {
	return map[string]any{"identity_type": identityType, "identity_id": identityID, "fingerprint": fingerprint, "generation": generation, "capabilities": nonNilSlice(capabilities), "reason": reason}
}

func (s *Server) auditDesktopIdentity(r *http.Request, auth authContext, homeID, event string, identity domain.DesktopIdentity, reason string) {
	s.audit(r.Context(), event, auditSeverityCritical, auth.User.ID, "", homeID, requestIDFromContext(r.Context()), "desktop_identity", identity.ID, desktopAuditMetadata(identity.IdentityType, identity.ID, identity.Fingerprint, identity.TrustRootGeneration, identity.Capabilities, reason))
}

func writeDesktopStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "desktop trust not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrConflict) {
		http.Error(w, "desktop trust conflict", http.StatusConflict)
		return
	}
	if err == nil {
		http.Error(w, "desktop trust conflict", http.StatusConflict)
		return
	}
	http.Error(w, "desktop trust operation failed", http.StatusInternalServerError)
}

func containsFold(value, fragment string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(fragment))
}
