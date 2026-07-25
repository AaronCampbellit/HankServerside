package cloud

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/desktopcrypto"
	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

const desktopEnrollmentChallengeTTL = 5 * time.Minute

type desktopAutoChallengeRequest struct {
	InstallationID string `json:"installation_id"`
}

type desktopAutoEnrollmentProof struct {
	ChallengeID    string `json:"challenge_id"`
	Challenge      string `json:"challenge"`
	InstallationID string `json:"installation_id"`
	PublicKeySPKI  string `json:"public_key_spki"`
	Signature      string `json:"signature"`
}

type desktopBrowserAutoEnrollmentRequest struct {
	desktopAutoEnrollmentProof
	DeviceID string `json:"device_id"`
}

type desktopMacAutoEnrollmentRequest struct {
	desktopAutoEnrollmentProof
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
}

func (s *Server) handleDesktopAutoChallenge(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, purpose string) {
	var body desktopAutoChallengeRequest
	if err := parseJSON(w, r, &body); err != nil || !validDesktopInstallationID(body.InstallationID) {
		http.Error(w, "invalid device enrollment request", http.StatusBadRequest)
		return
	}
	if purpose == "mac_agent" && home.AgentEnrollmentPolicy != "all_users" {
		membership, err := s.store.GetHomeMembership(r.Context(), home.ID, auth.User.ID)
		if err != nil || membership.Role != domain.HomeRoleAdmin {
			http.Error(w, "agent enrollment requires a home administrator", http.StatusForbidden)
			return
		}
	}
	now := time.Now().UTC()
	raw := newToken()
	hash := sha256.Sum256([]byte(raw))
	challenge := domain.DesktopEnrollmentChallenge{ID: newID("denroll"), HomeID: home.ID, UserID: auth.User.ID, SessionID: auth.Session.ID,
		Purpose: purpose, InstallationID: strings.TrimSpace(body.InstallationID), ChallengeHash: hash[:], CreatedAt: now, ExpiresAt: now.Add(desktopEnrollmentChallengeTTL)}
	if err := s.store.CreateDesktopEnrollmentChallenge(r.Context(), challenge); err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"challenge_id": challenge.ID, "challenge": raw, "expires_at": challenge.ExpiresAt})
}

func (s *Server) validateDesktopAutoEnrollment(r *http.Request, home domain.Home, auth authContext, purpose string, proof desktopAutoEnrollmentProof) ([]byte, error) {
	if !validDesktopInstallationID(proof.InstallationID) || strings.TrimSpace(proof.ChallengeID) == "" || strings.TrimSpace(proof.Challenge) == "" {
		return nil, errors.New("invalid device enrollment scope")
	}
	key, spki, err := decodeDesktopP256SPKI(proof.PublicKeySPKI)
	if err != nil {
		return nil, err
	}
	signature, err := desktopcrypto.DecodeRawBase64URL(proof.Signature)
	if err != nil {
		return nil, err
	}
	encoded := desktopAutoEnrollmentTranscript(home.ID, auth.User.ID, auth.Session.ID, purpose, proof.ChallengeID, proof.Challenge, proof.InstallationID, spki)
	if desktopcrypto.VerifyP256Signature(key, encoded, signature) != nil {
		return nil, errors.New("device enrollment signature is invalid")
	}
	hash := sha256.Sum256([]byte(proof.Challenge))
	if err := s.store.ConsumeDesktopEnrollmentChallenge(r.Context(), proof.ChallengeID, home.ID, auth.User.ID, auth.Session.ID, purpose, proof.InstallationID, hash[:], time.Now().UTC()); err != nil {
		return nil, err
	}
	return spki, nil
}

func desktopAutoEnrollmentTranscript(homeID, userID, sessionID, purpose, challengeID, challenge, installationID string, spki []byte) []byte {
	values := []string{"Hank Desktop Device Enrollment v1", homeID, userID, sessionID, purpose, challengeID, challenge, installationID, base64.RawURLEncoding.EncodeToString(spki)}
	return []byte(strings.Join(values, "\n"))
}

func validDesktopInstallationID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 16 || len(value) > 160 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func (s *Server) ensureServerDesktopAuthority(r *http.Request, home domain.Home, auth authContext) (domain.DesktopTrustRoot, *ecdsa.PrivateKey, bool, error) {
	root, key, migrated, err := s.store.EnsureServerManagedDesktopTrust(r.Context(), home.ID, time.Now().UTC())
	if err == nil && migrated {
		s.audit(r.Context(), "desktop.trust.server_managed", auditSeverityCritical, auth.User.ID, "", home.ID, requestIDFromContext(r.Context()), "desktop_trust", home.ID, map[string]any{"generation": root.Generation})
	}
	return root, key, migrated, err
}

func signServerDesktopIdentity(key *ecdsa.PrivateKey, root domain.DesktopTrustRoot, identity domain.DesktopIdentity) (domain.DesktopIdentity, error) {
	claims, err := desktopcrypto.EncodeIdentityCertificate(desktopcrypto.IdentityCertificateClaims{
		CertificateVersion: "desktop.v1", HomeID: identity.HomeID, IdentityID: identity.ID, IdentityType: identity.IdentityType,
		UserID: identity.UserID, DeviceID: identity.DeviceID, AgentID: identity.AgentID, PublicKeySPKI: identity.PublicKeySPKI,
		Capabilities: identity.Capabilities, TrustRootGeneration: uint32(root.Generation), CreatedAtUnixMS: identity.CreatedAt.UnixMilli(), ExpiresAtUnixMS: identity.ExpiresAt.UnixMilli(),
	})
	if err != nil {
		return domain.DesktopIdentity{}, err
	}
	digest := sha256.Sum256(claims)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		return domain.DesktopIdentity{}, err
	}
	certificate, err := json.Marshal(desktopSignedCertificateEnvelope{Claims: base64.RawURLEncoding.EncodeToString(claims), Signature: base64.RawURLEncoding.EncodeToString(signature)})
	if err != nil {
		return domain.DesktopIdentity{}, err
	}
	identity.Certificate = certificate
	identity.Fingerprint = desktopcrypto.FingerprintSPKI(identity.PublicKeySPKI)
	identity.TrustRootGeneration = root.Generation
	return identity, nil
}

func (s *Server) installServerDesktopIdentity(r *http.Request, home domain.Home, auth authContext, root domain.DesktopTrustRoot, key *ecdsa.PrivateKey, identity domain.DesktopIdentity, event string) (domain.DesktopIdentity, error) {
	signed, err := signServerDesktopIdentity(key, root, identity)
	if err != nil {
		return domain.DesktopIdentity{}, err
	}
	var existing domain.DesktopIdentity
	if signed.IdentityType == domain.DesktopIdentityOperatorDevice {
		existing, err = s.store.GetActiveDesktopOperatorIdentity(r.Context(), home.ID, signed.UserID, signed.DeviceID, time.Now().UTC())
	} else {
		existing, err = s.store.GetActiveDesktopEndpointIdentity(r.Context(), home.ID, signed.AgentID, time.Now().UTC())
	}
	if err == nil {
		sessions, replaceErr := s.store.ReplaceDesktopIdentity(r.Context(), existing.ID, signed, time.Now().UTC(), "identity_refreshed")
		if replaceErr != nil {
			return domain.DesktopIdentity{}, replaceErr
		}
		s.revokeDesktopRelays(sessions, "identity_refreshed")
	} else if errors.Is(err, store.ErrNotFound) {
		if err := s.store.CreateDesktopIdentity(r.Context(), signed); err != nil {
			return domain.DesktopIdentity{}, err
		}
	} else {
		return domain.DesktopIdentity{}, err
	}
	s.auditDesktopIdentity(r, auth, home.ID, event, signed, "server_managed")
	return signed, nil
}

func (s *Server) handleDesktopBrowserAutoEnrollment(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	var body desktopBrowserAutoEnrollmentRequest
	if err := parseJSON(w, r, &body); err != nil || strings.TrimSpace(body.DeviceID) == "" || len(body.DeviceID) > 160 {
		http.Error(w, "invalid browser enrollment", http.StatusBadRequest)
		return
	}
	spki, err := s.validateDesktopAutoEnrollment(r, home, auth, "browser_operator", body.desktopAutoEnrollmentProof)
	if err != nil {
		http.Error(w, "browser enrollment rejected", http.StatusForbidden)
		return
	}
	root, key, _, err := s.ensureServerDesktopAuthority(r, home, auth)
	if err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	now := time.Now().UTC()
	identity, err := s.installServerDesktopIdentity(r, home, auth, root, key, domain.DesktopIdentity{ID: newID("dop"), HomeID: home.ID, IdentityType: domain.DesktopIdentityOperatorDevice,
		UserID: auth.User.ID, DeviceID: body.DeviceID, PublicKeySPKI: spki, Capabilities: append([]string(nil), desktopRequiredAdminCapabilities...), CreatedAt: now, ExpiresAt: auth.Session.ExpiresAt, AuthSessionID: auth.Session.ID}, "desktop.identity.auto_enrolled")
	if err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"root": map[string]any{"generation": root.Generation, "algorithm": root.Algorithm, "fingerprint": root.Fingerprint, "public_key_spki": base64.RawURLEncoding.EncodeToString(root.PublicKeySPKI)}, "identity": desktopIdentityPublicSnapshot(identity)})
}

func (s *Server) handleMacAutoEnrollment(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, membership domain.HomeMembership) {
	if home.AgentEnrollmentPolicy != "all_users" && membership.Role != domain.HomeRoleAdmin {
		http.Error(w, "agent enrollment requires a home administrator", http.StatusForbidden)
		return
	}
	var body desktopMacAutoEnrollmentRequest
	if err := parseJSON(w, r, &body); err != nil || strings.TrimSpace(body.AgentID) == "" || len(body.AgentID) > 128 || len(strings.TrimSpace(body.Name)) > 256 {
		http.Error(w, "invalid Mac enrollment", http.StatusBadRequest)
		return
	}
	spki, err := s.validateDesktopAutoEnrollment(r, home, auth, "mac_agent", body.desktopAutoEnrollmentProof)
	if err != nil {
		http.Error(w, "Mac enrollment rejected", http.StatusForbidden)
		return
	}
	now := time.Now().UTC()
	agent, err := s.store.GetAgentByInstallationID(r.Context(), home.ID, body.InstallationID)
	if errors.Is(err, store.ErrNotFound) {
		agent = domain.Agent{ID: body.AgentID, HomeID: home.ID, Name: strings.TrimSpace(body.Name), Status: domain.AgentStatusOffline, AgentType: AgentTypeWorker, InstallationID: body.InstallationID, EnrolledByUserID: auth.User.ID, CreatedAt: now, UpdatedAt: now}
	} else if err != nil {
		writeDesktopStoreError(w, err)
		return
	} else {
		agent.Name = strings.TrimSpace(body.Name)
		if agent.Name == "" {
			agent.Name = agent.ID
		}
		agent.EnrolledByUserID = auth.User.ID
		agent.UpdatedAt = now
	}
	if agent.Name == "" {
		agent.Name = agent.ID
	}
	if err := s.store.UpsertAgent(r.Context(), agent); err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	root, key, _, err := s.ensureServerDesktopAuthority(r, home, auth)
	if err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	identity, err := s.installServerDesktopIdentity(r, home, auth, root, key, domain.DesktopIdentity{ID: newID("dep"), HomeID: home.ID, IdentityType: domain.DesktopIdentityEndpoint,
		AgentID: agent.ID, PublicKeySPKI: spki, Capabilities: []string{"desktop.view", "desktop.control", "desktop.clipboard.read", "desktop.clipboard.write", "desktop.elevate", "desktop.secure_desktop", "desktop.unattended"}, CreatedAt: now, ExpiresAt: now.Add(365 * 24 * time.Hour)}, "desktop.endpoint.auto_enrolled")
	if err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	if err := s.store.RevokeAgentTokensForAgent(r.Context(), home.ID, agent.ID, now); err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	rawToken := newToken()
	token := domain.AgentToken{ID: newID("agtok"), HomeID: home.ID, AgentID: agent.ID, TokenHash: hashToken(rawToken), CreatedAt: now}
	if err := s.store.CreateAgentToken(r.Context(), token); err != nil {
		writeDesktopStoreError(w, err)
		return
	}
	s.audit(r.Context(), "agent.auto_enrolled", auditSeverityCritical, auth.User.ID, "", home.ID, requestIDFromContext(r.Context()), "agent", agent.ID, map[string]any{"installation_id": agent.InstallationID, "policy": home.AgentEnrollmentPolicy})
	writeJSON(w, http.StatusCreated, map[string]any{"agent_id": agent.ID, "agent_name": agent.Name, "token": rawToken, "token_id": token.ID,
		"endpoint": desktopIdentityPublicSnapshot(identity), "root": map[string]any{"generation": root.Generation, "fingerprint": root.Fingerprint, "public_key_spki": base64.RawURLEncoding.EncodeToString(root.PublicKeySPKI)}})
}
