package cloud

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		home, _, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, home)
	case http.MethodPut:
		home, membership, err := s.requireSingletonHomeAdmin(r.Context(), auth.User.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			if errors.Is(err, errAdminRoleRequired) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		home, err = s.store.RenameSingletonHome(r.Context(), home.ID, body.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = membership
		writeJSON(w, http.StatusOK, home)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleHomeSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/home/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	if s.handleHomeAppPackageDownload(w, r, parts) {
		return
	}
	if s.handleHomeNotesWithNotesAuth(w, r, parts) {
		return
	}

	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	home, membership, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(parts) == 1 && parts[0] == "sync" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.syncResponse(r.Context(), home))
		return
	}
	if len(parts) == 1 && parts[0] == "setup-status" && r.Method == http.MethodGet {
		s.handleHomeSetupStatus(w, r, home)
		return
	}

	if s.handleHomeNotifications(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeSearch(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeQuickLinks(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeMembers(w, r, home, membership, parts) {
		return
	}
	if s.handleHomeAssistant(w, r, home, membership, auth, parts) {
		return
	}
	if s.handleHomeNotesAPITokens(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeServiceProfiles(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeApps(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeRecovery(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeStorage(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeAuditEvents(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeQueryTelemetry(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomePostgresExtensions(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeFileJobs(w, r, home, auth, membership, parts) {
		return
	}
	if s.handleHomeAgent(w, r, home, auth, membership, parts) {
		return
	}

	if len(parts) == 2 && parts[0] == "files" && parts[1] == "preview" && r.Method == http.MethodGet {
		if err := s.requireHomeFeature(r.Context(), home, membership, auth.User.ID, domain.HomePermissionFeatureFiles); err != nil {
			if errors.Is(err, errFeaturePermissionDenied) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.handleFilePreviewStream(w, r, home, auth)
		return
	}
	if len(parts) == 2 && parts[0] == "files" && parts[1] == "downloads" && r.Method == http.MethodPost {
		if err := s.requireHomeFeature(r.Context(), home, membership, auth.User.ID, domain.HomePermissionFeatureFiles); err != nil {
			if errors.Is(err, errFeaturePermissionDenied) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.handleFileTransferSetup(w, r, home, auth, protocol.FileTransferOperationDownload)
		return
	}
	if len(parts) == 2 && parts[0] == "files" && parts[1] == "uploads" && r.Method == http.MethodPost {
		if err := s.requireHomeFeature(r.Context(), home, membership, auth.User.ID, domain.HomePermissionFeatureFiles); err != nil {
			if errors.Is(err, errFeaturePermissionDenied) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.handleFileTransferSetup(w, r, home, auth, protocol.FileTransferOperationUpload)
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleHomeSetupStatus(w http.ResponseWriter, r *http.Request, home domain.Home) {
	firstSetupVisible := true
	if runtime, err := s.store.GetCloudRuntime(r.Context()); err == nil && runtime.StartedAt.After(home.CreatedAt) {
		firstSetupVisible = false
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"first_setup_visible": firstSetupVisible,
	})
}

func (s *Server) handleHomeAgent(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, membership domain.HomeMembership, parts []string) bool {
	if len(parts) == 1 && parts[0] == "agent" && r.Method == http.MethodGet {
		agent, err := s.store.GetAgentByHomeID(r.Context(), home.ID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"agent": nil})
			return true
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		snapshot := map[string]any{
			"agent_id":     agent.ID,
			"name":         agent.Name,
			"status":       agent.Status,
			"last_seen_at": agent.LastSeenAt,
			"home_id":      home.ID,
			"home_name":    home.Name,
		}
		if online, ok := s.router.GetAgent(home.ID); ok {
			snapshot["status"] = online.agent.Status
			snapshot["capabilities"] = online.capabilities
		}
		writeJSON(w, http.StatusOK, map[string]any{"agent": snapshot, "can_restart": membership.Role == domain.HomeRoleAdmin})
		return true
	}

	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "tokens" && r.Method == http.MethodGet {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		tokens, err := s.store.ListAgentTokensByHome(r.Context(), home.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": nonNilSlice(tokens)})
		return true
	}

	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "tokens" && r.Method == http.MethodPost {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		type request struct {
			AgentID          string `json:"agent_id"`
			Name             string `json:"name"`
			ExpiresInSeconds int    `json:"expires_in_seconds"`
		}
		var body request
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}

		body.AgentID = strings.TrimSpace(body.AgentID)
		if body.AgentID == "" {
			http.Error(w, "agent_id is required", http.StatusBadRequest)
			return true
		}
		agentName := strings.TrimSpace(body.Name)
		if agentName == "" {
			agentName = body.AgentID
		}

		now := time.Now().UTC()
		agent := domain.Agent{
			ID:        body.AgentID,
			HomeID:    home.ID,
			Name:      agentName,
			Status:    domain.AgentStatusOffline,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if existing, err := s.store.GetAgentByID(r.Context(), body.AgentID); err == nil {
			if existing.HomeID != home.ID {
				http.Error(w, "agent_id already belongs to a different home", http.StatusConflict)
				return true
			}
			agent.CreatedAt = existing.CreatedAt
			agent.LastSeenAt = existing.LastSeenAt
		}
		if err := s.store.UpsertAgent(r.Context(), agent); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}

		rawToken := newToken()
		token := domain.AgentToken{
			ID:        newID("agtok"),
			HomeID:    home.ID,
			AgentID:   agent.ID,
			TokenHash: hashToken(rawToken),
			CreatedAt: now,
		}
		if body.ExpiresInSeconds > 0 {
			expiresAt := now.Add(time.Duration(body.ExpiresInSeconds) * time.Second)
			token.ExpiresAt = &expiresAt
		}
		if err := s.store.CreateAgentToken(r.Context(), token); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.audit(r.Context(), "agent_token.created", auditSeverityInfo, auth.User.ID, "", home.ID, requestIDFromContext(r.Context()), "agent_token", token.ID, map[string]any{"agent_id": agent.ID})

		writeJSON(w, http.StatusCreated, map[string]any{
			"token_id":     token.ID,
			"home_id":      home.ID,
			"agent_id":     agent.ID,
			"agent_name":   agent.Name,
			"token":        rawToken,
			"expires_at":   token.ExpiresAt,
			"created_at":   token.CreatedAt,
			"agent_status": agent.Status,
		})
		return true
	}

	if len(parts) == 2 && parts[0] == "agent" && parts[1] == "restart" && r.Method == http.MethodPost {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandSystemRestart, protocol.SystemRestartRequest{Reason: "dashboard operator requested restart"})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		if envelope.Error != nil {
			http.Error(w, envelope.Error.Message, http.StatusBadGateway)
			return true
		}
		payload, err := protocol.DecodePayload[protocol.SystemRestartResponse](envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		s.audit(r.Context(), "agent.restart_requested", auditSeverityWarning, auth.User.ID, "", home.ID, requestIDFromContext(r.Context()), "agent", envelope.AgentID, map[string]any{"restart_at": payload.RestartAt})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         payload.OK,
			"message":    payload.Message,
			"restart_at": payload.RestartAt,
		})
		return true
	}

	if len(parts) == 3 && parts[0] == "agent" && parts[1] == "tokens" && r.Method == http.MethodDelete {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		var err error
		if r.URL.Query().Get("purge") == "1" {
			err = s.store.DeleteAgentTokenForHome(r.Context(), home.ID, parts[2])
		} else {
			err = s.store.RevokeAgentTokenForHome(r.Context(), home.ID, parts[2])
		}
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return true
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.audit(r.Context(), "agent_token.revoked", auditSeverityInfo, auth.User.ID, "", home.ID, requestIDFromContext(r.Context()), "agent_token", parts[2], map[string]any{"purged": r.URL.Query().Get("purge") == "1"})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return true
	}

	return false
}
