package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

var errAdminRoleRequired = errors.New("admin role required")
var errFeaturePermissionDenied = errors.New("feature permission denied")

func (s *Server) requireSingletonHomeMembership(ctx context.Context, userID string) (domain.Home, domain.HomeMembership, error) {
	home, err := s.store.GetSingletonHomeForUser(ctx, userID)
	if err != nil {
		return domain.Home{}, domain.HomeMembership{}, err
	}
	membership, err := s.store.GetHomeMembership(ctx, home.ID, userID)
	if err != nil {
		return domain.Home{}, domain.HomeMembership{}, err
	}
	return home, membership, nil
}

func (s *Server) requireSingletonHomeAdmin(ctx context.Context, userID string) (domain.Home, domain.HomeMembership, error) {
	home, membership, err := s.requireSingletonHomeMembership(ctx, userID)
	if err != nil {
		return domain.Home{}, domain.HomeMembership{}, err
	}
	if membership.Role != domain.HomeRoleAdmin {
		return domain.Home{}, domain.HomeMembership{}, errAdminRoleRequired
	}
	return home, membership, nil
}

func (s *Server) homeFeatureAllowed(ctx context.Context, home domain.Home, membership domain.HomeMembership, userID string, feature string) (bool, error) {
	if membership.Role == domain.HomeRoleAdmin {
		return true, nil
	}

	defaults, err := s.store.GetHomePermissions(ctx, home.ID)
	if err != nil {
		return false, err
	}
	override, err := s.store.GetHomeMemberPermissions(ctx, home.ID, userID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return false, err
	}

	switch feature {
	case domain.HomePermissionFeatureHomeAssistant:
		if override.HomeAssistantEnabled != nil {
			return *override.HomeAssistantEnabled, nil
		}
		return defaults.HomeAssistantEnabled, nil
	case domain.HomePermissionFeatureFiles:
		if override.FilesEnabled != nil {
			return *override.FilesEnabled, nil
		}
		return defaults.FilesEnabled, nil
	case domain.HomePermissionFeatureNotes:
		if override.NotesEnabled != nil {
			return *override.NotesEnabled, nil
		}
		return defaults.NotesEnabled, nil
	default:
		return false, errors.New("unsupported feature")
	}
}

func (s *Server) requireHomeFeature(ctx context.Context, home domain.Home, membership domain.HomeMembership, userID string, feature string) error {
	allowed, err := s.homeFeatureAllowed(ctx, home, membership, userID, feature)
	if err != nil {
		return err
	}
	if !allowed {
		return errFeaturePermissionDenied
	}
	return nil
}

func (s *Server) handleHomeInvitationAccept(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body.Token = strings.TrimSpace(body.Token)
	if body.Token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}

	invitation, err := s.store.GetHomeInvitationByTokenHash(r.Context(), hashToken(body.Token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invitation.AcceptedAt != nil {
		http.Error(w, "invitation already accepted", http.StatusConflict)
		return
	}
	if invitation.ExpiresAt != nil && invitation.ExpiresAt.Before(time.Now().UTC()) {
		http.Error(w, "invitation expired", http.StatusGone)
		return
	}
	if !strings.EqualFold(invitation.Email, auth.User.Email) {
		http.Error(w, "invitation email does not match current user", http.StatusForbidden)
		return
	}

	if err := s.store.AcceptHomeInvitation(r.Context(), invitation.ID, auth.User, invitation.Role); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	home, err := s.store.GetHomeByID(r.Context(), invitation.HomeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"home": home,
	})
}

func (s *Server) handleHomeMembers(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership, parts []string) bool {
	if len(parts) == 1 && parts[0] == "members" && r.Method == http.MethodGet {
		members, err := s.store.ListHomeMembers(r.Context(), home.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"members": members})
		return true
	}

	if len(parts) == 2 && parts[0] == "members" && parts[1] == "invitations" && r.Method == http.MethodGet {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		invitations, err := s.store.ListPendingHomeInvitations(r.Context(), home.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"invitations": invitations})
		return true
	}

	if len(parts) == 2 && parts[0] == "members" && parts[1] == "invitations" && r.Method == http.MethodPost {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		var body struct {
			Email string `json:"email"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		body.Email = strings.TrimSpace(strings.ToLower(body.Email))
		if body.Email == "" {
			http.Error(w, "email is required", http.StatusBadRequest)
			return true
		}
		inviteToken := newToken()
		now := time.Now().UTC()
		expiresAt := now.Add(7 * 24 * time.Hour)
		invitation := domain.HomeInvitation{
			ID:        newID("invite"),
			HomeID:    home.ID,
			Email:     body.Email,
			Role:      domain.HomeRoleMember,
			TokenHash: hashToken(inviteToken),
			ExpiresAt: &expiresAt,
			CreatedAt: now,
		}
		if err := s.store.CreateHomeInvitation(r.Context(), invitation); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.emitMembersChanged(r.Context(), map[string]any{"home_id": home.ID, "kind": "invitation_created"})
		writeJSON(w, http.StatusCreated, map[string]any{
			"invitation_id": invitation.ID,
			"home_id":       invitation.HomeID,
			"email":         invitation.Email,
			"role":          invitation.Role,
			"token":         inviteToken,
			"expires_at":    invitation.ExpiresAt,
		})
		return true
	}

	if len(parts) == 3 && parts[0] == "members" && parts[1] == "invitations" && r.Method == http.MethodDelete {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		if err := s.store.DeletePendingHomeInvitation(r.Context(), home.ID, parts[2]); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return true
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.emitMembersChanged(r.Context(), map[string]any{"home_id": home.ID, "kind": "invitation_deleted"})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return true
	}

	if len(parts) == 2 && parts[0] == "members" && r.Method == http.MethodDelete {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		targetUserID := parts[1]
		targetMembership, err := s.store.GetHomeMembership(r.Context(), home.ID, targetUserID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return true
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		if targetMembership.Role == domain.HomeRoleOwner {
			owners, err := s.store.CountHomeOwners(r.Context(), home.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return true
			}
			if owners <= 1 {
				http.Error(w, "cannot remove the last admin", http.StatusConflict)
				return true
			}
		}
		if err := s.store.RemoveHomeMembership(r.Context(), home.ID, targetUserID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return true
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		_ = s.store.RemoveNoteSharesForHomeUser(r.Context(), home.ID, targetUserID)
		s.markHomeNotesDirty(r.Context(), home.ID, "")
		s.evictCollaborator(home.ID, targetUserID, "membership_revoked")
		s.emitMembersChanged(r.Context(), map[string]any{"home_id": home.ID, "kind": "member_removed", "user_id": targetUserID})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return true
	}

	if len(parts) == 3 && parts[0] == "members" && parts[2] == "role" && r.Method == http.MethodPut {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		targetUserID := parts[1]
		targetMembership, err := s.store.GetHomeMembership(r.Context(), home.ID, targetUserID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return true
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		var body struct {
			Role string `json:"role"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		role := strings.TrimSpace(strings.ToLower(body.Role))
		if role != domain.HomeRoleAdmin && role != domain.HomeRoleMember {
			http.Error(w, "invalid role", http.StatusBadRequest)
			return true
		}
		if targetMembership.Role == domain.HomeRoleAdmin && role != domain.HomeRoleAdmin {
			admins, err := s.store.CountHomeOwners(r.Context(), home.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return true
			}
			if admins <= 1 {
				http.Error(w, "cannot demote the last admin", http.StatusConflict)
				return true
			}
		}
		if err := s.store.UpdateHomeMembershipRole(r.Context(), home.ID, targetUserID, role); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return true
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		updated, err := s.store.GetHomeMembership(r.Context(), home.ID, targetUserID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.emitMembersChanged(r.Context(), map[string]any{"home_id": home.ID, "kind": "member_role_changed", "user_id": targetUserID})
		writeJSON(w, http.StatusOK, updated)
		return true
	}

	if len(parts) == 1 && parts[0] == "permissions" {
		return s.handleHomePermissions(w, r, home, membership)
	}

	if len(parts) == 3 && parts[0] == "members" && parts[2] == "permissions" {
		return s.handleHomeMemberPermissions(w, r, home, membership, parts[1])
	}

	return false
}

func (s *Server) handleHomeNotesHTTP(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership, auth authContext, parts []string) bool {
	if len(parts) == 0 || parts[0] != "notes" {
		return false
	}
	if err := s.requireHomeFeature(r.Context(), home, membership, auth.User.ID, domain.HomePermissionFeatureNotes); err != nil {
		if errors.Is(err, errFeaturePermissionDenied) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return true
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		notes, err := s.notes.ListHome(r.Context(), home.ID, auth.User.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
		return true
	}

	if len(parts) >= 2 {
		if noteID, attachmentID, ok := splitNoteAttachmentRoute(parts); ok {
			s.handleHomeNoteAttachmentsHTTP(w, r, home, auth, noteID, attachmentID)
			return true
		}
		noteID := strings.Join(parts[1:], "/")
		if len(parts) >= 4 && parts[len(parts)-2] == "shares" {
			baseNoteID := strings.Join(parts[1:len(parts)-2], "/")
			switch r.Method {
			case http.MethodGet:
				shares, err := s.notes.ListShares(r.Context(), home.ID, baseNoteID, auth.User.ID)
				if err != nil {
					if errors.Is(err, store.ErrNotFound) {
						http.NotFound(w, r)
						return true
					}
					http.Error(w, err.Error(), http.StatusForbidden)
					return true
				}
				writeJSON(w, http.StatusOK, protocol.NotesSharesResponse{Shares: shares})
				return true
			case http.MethodDelete:
				targetUserID := parts[len(parts)-1]
				note, removedLast, err := s.notes.RevokeShare(r.Context(), home.ID, baseNoteID, auth.User.ID, targetUserID)
				if err != nil {
					if errors.Is(err, store.ErrNotFound) {
						http.NotFound(w, r)
						return true
					}
					http.Error(w, err.Error(), http.StatusForbidden)
					return true
				}
				s.collaboration.revokeNoteUser(note.ID, targetUserID, "share_revoked")
				if removedLast {
					s.markHomeNotesDirty(r.Context(), home.ID, "")
				}
				s.emitHomeNotesChanged(r.Context(), "notes.share_changed", map[string]any{"home_id": home.ID, "note_id": note.NoteID})
				writeJSON(w, http.StatusOK, map[string]any{"ok": true})
				return true
			}
		}
		if len(parts) >= 3 && parts[len(parts)-1] == "shares" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
			baseNoteID := strings.Join(parts[1:len(parts)-1], "/")
			if r.Method == http.MethodGet {
				shares, err := s.notes.ListShares(r.Context(), home.ID, baseNoteID, auth.User.ID)
				if err != nil {
					if errors.Is(err, store.ErrNotFound) {
						http.NotFound(w, r)
						return true
					}
					http.Error(w, err.Error(), http.StatusForbidden)
					return true
				}
				writeJSON(w, http.StatusOK, protocol.NotesSharesResponse{Shares: shares})
				return true
			}
			var body protocol.NotesShareCreateRequest
			if err := parseJSON(w, r, &body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return true
			}
			note, firstShare, err := s.notes.GrantShare(r.Context(), home.ID, baseNoteID, auth.User.ID, body.UserID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					http.NotFound(w, r)
					return true
				}
				http.Error(w, err.Error(), http.StatusForbidden)
				return true
			}
			if firstShare {
				s.markHomeNotesDirty(r.Context(), home.ID, "")
			}
			s.emitHomeNotesChanged(r.Context(), "notes.share_changed", map[string]any{"home_id": home.ID, "note_id": note.NoteID})
			writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "note_id": note.NoteID})
			return true
		}

		switch r.Method {
		case http.MethodGet:
			note, err := s.notes.FetchHome(r.Context(), home.ID, auth.User.ID, noteID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					http.NotFound(w, r)
					return true
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return true
			}
			if noteRecord, err := s.store.GetHomeNoteVisibleToUser(r.Context(), home.ID, auth.User.ID, noteID); err == nil {
				note = s.addNoteAttachmentsToResponse(r.Context(), note, noteRecord, "home")
			}
			writeJSON(w, http.StatusOK, note)
			return true

		case http.MethodPut:
			var body protocol.NotesSaveRequest
			if err := parseJSON(w, r, &body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return true
			}
			body.NoteID = noteID
			response, err := s.notes.SaveHome(r.Context(), home.ID, auth.User.ID, noteID, body)
			if err != nil {
				conflict := &noteConflictError{}
				if errors.As(err, &conflict) {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "note_conflict", "current": conflict.Current})
					return true
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return true
			}
			s.markHomeNotesDirty(r.Context(), home.ID, "")
			s.emitHomeNotesChanged(r.Context(), "notes.changed", map[string]any{"home_id": home.ID, "note_id": noteID})
			writeJSON(w, http.StatusOK, response)
			return true

		case http.MethodDelete:
			if err := s.notes.DeleteHome(r.Context(), home.ID, auth.User.ID, noteID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					http.NotFound(w, r)
					return true
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return true
			}
			s.markHomeNotesDirty(r.Context(), home.ID, "")
			s.emitHomeNotesChanged(r.Context(), "notes.deleted", map[string]any{"home_id": home.ID, "note_id": noteID})
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return true
		}
	}

	return false
}

func (s *Server) handleProfileNotesHTTP(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/me/notes")
	path = strings.Trim(path, "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			notes, err := s.notes.ListProfile(r.Context(), auth.User.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.logger.Info("profile notes list served", "user_id", auth.User.ID, "note_count", len(notes))
			writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
			return
		case http.MethodPost:
			var body protocol.NotesSaveRequest
			if err := parseJSON(w, r, &body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			response, err := s.notes.SaveProfile(r.Context(), auth.User.ID, body.NoteID, body)
			if err != nil {
				conflict := &noteConflictError{}
				if errors.As(err, &conflict) {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "note_conflict", "current": conflict.Current})
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.logger.Info("profile note saved via http create", "user_id", auth.User.ID, "note_id", response.NoteID, "revision", response.Revision)
			s.emitProfileNotesChanged(r.Context(), map[string]any{"user_id": auth.User.ID, "note_id": response.NoteID})
			writeJSON(w, http.StatusCreated, response)
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}

	if parts := strings.Split(path, "/"); len(parts) >= 2 {
		if noteID, attachmentID, ok := splitNoteAttachmentRoute(append([]string{"notes"}, parts...)); ok {
			s.handleProfileNoteAttachmentsHTTP(w, r, auth, noteID, attachmentID)
			return
		}
	}

	noteID := path
	switch r.Method {
	case http.MethodGet:
		note, err := s.notes.FetchProfile(r.Context(), auth.User.ID, noteID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if noteRecord, err := s.store.GetProfileNote(r.Context(), auth.User.ID, noteID); err == nil {
			note = s.addNoteAttachmentsToResponse(r.Context(), note, noteRecord, "profile")
		}
		s.logger.Info("profile note fetched", "user_id", auth.User.ID, "note_id", noteID, "revision", note.Revision)
		writeJSON(w, http.StatusOK, note)
	case http.MethodPut:
		var body protocol.NotesSaveRequest
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body.NoteID = noteID
		response, err := s.notes.SaveProfile(r.Context(), auth.User.ID, noteID, body)
		if err != nil {
			conflict := &noteConflictError{}
			if errors.As(err, &conflict) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "note_conflict", "current": conflict.Current})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.logger.Info("profile note saved via http update", "user_id", auth.User.ID, "note_id", response.NoteID, "revision", response.Revision)
		s.emitProfileNotesChanged(r.Context(), map[string]any{"user_id": auth.User.ID, "note_id": response.NoteID})
		writeJSON(w, http.StatusOK, response)
	case http.MethodDelete:
		if err := s.notes.DeleteProfile(r.Context(), auth.User.ID, noteID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.logger.Info("profile note deleted via http", "user_id", auth.User.ID, "note_id", noteID)
		s.broadcastAppEvent(r.Context(), topicNotesProfile, "notes.deleted", map[string]any{"user_id": auth.User.ID, "note_id": noteID})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProfileSettingsHTTP(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := s.store.GetUserProfileSettings(r.Context(), auth.User.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]any{
					"revision":   0,
					"updated_at": nil,
					"settings":   map[string]any{},
				})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision":   record.Revision,
			"updated_at": record.UpdatedAt,
			"settings":   record.Settings,
		})
	case http.MethodPut:
		var body struct {
			ExpectedRevision *int            `json:"expected_revision"`
			Settings         json.RawMessage `json:"settings"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		record, err := s.store.SaveUserProfileSettings(r.Context(), auth.User.ID, body.ExpectedRevision, body.Settings)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "conflict"})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision":   record.Revision,
			"updated_at": record.UpdatedAt,
			"settings":   record.Settings,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProfileSecretVaultHTTP(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := s.store.GetUserProfileSecretVault(r.Context(), auth.User.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]any{
					"revision":   0,
					"key_id":     "",
					"updated_at": nil,
					"vault":      map[string]any{},
				})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision":   record.Revision,
			"key_id":     record.KeyID,
			"updated_at": record.UpdatedAt,
			"vault":      record.Vault,
		})
	case http.MethodPut:
		var body struct {
			ExpectedRevision *int            `json:"expected_revision"`
			KeyID            string          `json:"key_id"`
			Vault            json.RawMessage `json:"vault"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		record, err := s.store.SaveUserProfileSecretVault(r.Context(), auth.User.ID, body.ExpectedRevision, body.KeyID, body.Vault)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "conflict"})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision":   record.Revision,
			"key_id":     record.KeyID,
			"updated_at": record.UpdatedAt,
			"vault":      record.Vault,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProfileBackupHTTP(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := s.store.GetUserProfileBackup(r.Context(), auth.User.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision":   record.Revision,
			"updated_at": record.UpdatedAt,
			"snapshot":   record.Snapshot,
		})
	case http.MethodPut:
		var body struct {
			ExpectedRevision *int            `json:"expected_revision"`
			Snapshot         json.RawMessage `json:"snapshot"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(body.Snapshot) == 0 || !json.Valid(body.Snapshot) {
			http.Error(w, "snapshot must be valid json", http.StatusBadRequest)
			return
		}
		record, err := s.store.SaveUserProfileBackup(r.Context(), auth.User.ID, body.ExpectedRevision, body.Snapshot)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "conflict"})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision":   record.Revision,
			"updated_at": record.UpdatedAt,
			"snapshot":   record.Snapshot,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleHomeSyncHTTP(w http.ResponseWriter, r *http.Request, home domain.Home) bool {
	if len(strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/homes/"+home.ID), "/"), "/")) != 1 {
		return false
	}
	return false
}

func (s *Server) syncResponse(ctx context.Context, home domain.Home) map[string]any {
	latestBackupAt, _ := s.store.GetLatestHomeNoteUpdate(ctx, home.ID)
	noteState, err := s.store.GetHomeNoteSyncState(ctx, home.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		noteState = domain.HomeNoteSyncState{HomeID: home.ID, Status: domain.SyncStatusDegraded, LastError: err.Error()}
	}
	if _, ok := s.router.GetAgent(home.ID); !ok {
		if noteState.Status == "" {
			noteState.Status = domain.SyncStatusOffline
		}
		if noteState.LastError == "" {
			noteState.LastError = "agent offline"
		}
	}
	profiles, _ := s.store.ListHomeServiceProfiles(ctx, home.ID)

	profileStatuses := map[string]any{}
	for _, profile := range profiles {
		profileStatuses[profile.ServiceType] = map[string]any{
			"status":          profile.Status,
			"updated_at":      profile.UpdatedAt,
			"last_backup_at":  profile.LastBackupAt,
			"last_error":      profile.LastError,
			"secret_version":  profile.SecretVersion,
			"applied_version": profile.AppliedVersion,
		}
	}

	return map[string]any{
		"home_id": home.ID,
		"notes": map[string]any{
			"status":                    noteState.Status,
			"last_manifest_at":          noteState.LastManifestAt,
			"last_pull_at":              noteState.LastPullAt,
			"last_push_at":              noteState.LastPushAt,
			"last_successful_sync_at":   noteState.LastSuccessfulSyncAt,
			"last_successful_backup_at": latestBackupAt,
			"pending_pull_count":        noteState.PendingPullCount,
			"pending_push_count":        noteState.PendingPushCount,
			"last_error":                noteState.LastError,
		},
		"profiles": profileStatuses,
	}
}

func (s *Server) handleHomeServiceProfiles(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, membership domain.HomeMembership, parts []string) bool {
	if len(parts) == 1 && parts[0] == "service-profiles" && r.Method == http.MethodGet {
		profiles, err := s.store.ListHomeServiceProfiles(r.Context(), home.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
		return true
	}

	if len(parts) == 2 && parts[0] == "service-profiles" && r.Method == http.MethodPut {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}

		serviceType := parts[1]
		if serviceType != domain.ServiceTypeHomeAssistant && serviceType != domain.ServiceTypeSMB {
			http.Error(w, "unsupported service type", http.StatusBadRequest)
			return true
		}

		var body struct {
			PublicConfig json.RawMessage `json:"public_config"`
			Secrets      json.RawMessage `json:"secrets"`
			Persist      bool            `json:"persist"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}

		existing, err := s.store.GetHomeServiceProfile(r.Context(), home.ID, serviceType)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		if errors.Is(err, store.ErrNotFound) {
			existing = domain.HomeServiceProfile{HomeID: home.ID, ServiceType: serviceType, UpdatedBy: auth.User.ID}
		}

		now := time.Now().UTC()
		profile := existing
		profile.UpdatedAt = now
		profile.UpdatedBy = auth.User.ID
		if len(body.PublicConfig) > 0 {
			profile.PublicConfigJSON = strings.TrimSpace(string(body.PublicConfig))
		}

		secretVersion := existing.SecretVersion
		if len(body.Secrets) > 0 {
			secretVersion++
		}
		profile.SecretVersion = secretVersion

		if len(body.Secrets) > 0 {
			agentConn, ok := s.router.GetAgent(home.ID)
			if !ok {
				profile.Status = domain.SyncStatusDegraded
				profile.LastError = "agent offline"
				_ = s.store.UpsertHomeServiceProfile(r.Context(), profile)
				writeJSON(w, http.StatusConflict, map[string]any{"error": "agent_offline", "profile": profile})
				return true
			}

			response, err := s.sendAgentCommand(r.Context(), home.ID, "config.apply", protocol.ConfigApplyRequest{
				ServiceType:   serviceType,
				PublicConfig:  body.PublicConfig,
				Secrets:       body.Secrets,
				SecretVersion: secretVersion,
				Persist:       body.Persist,
			})
			if err != nil {
				profile.Status = domain.SyncStatusDegraded
				profile.LastError = err.Error()
				_ = s.store.UpsertHomeServiceProfile(r.Context(), profile)
				http.Error(w, err.Error(), http.StatusBadGateway)
				return true
			}
			if response.Error != nil {
				profile.Status = domain.SyncStatusDegraded
				profile.LastError = response.Error.Message
				_ = s.store.UpsertHomeServiceProfile(r.Context(), profile)
				http.Error(w, response.Error.Message, http.StatusBadGateway)
				return true
			}
			applied, err := protocol.DecodePayload[protocol.ConfigApplyResponse](response)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return true
			}
			profile.PublicConfigJSON = strings.TrimSpace(string(applied.Profile.PublicConfig))
			profile.AppliedVersion = applied.Profile.AppliedVersion
			profile.Status = applied.Profile.Status
			profile.LastError = applied.Profile.LastError
			lastBackup := now
			profile.LastBackupAt = &lastBackup
			_ = agentConn
		} else {
			profile.Status = domain.SyncStatusPending
			profile.LastError = ""
		}

		if err := s.store.UpsertHomeServiceProfile(r.Context(), profile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.emitSettingsChanged(r.Context(), "service_profiles.changed", map[string]any{"home_id": home.ID, "service_type": serviceType})
		writeJSON(w, http.StatusOK, profile)
		return true
	}

	return false
}

func (s *Server) handleHomePermissions(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership) bool {
	switch r.Method {
	case http.MethodGet:
		permissions, err := s.store.GetHomePermissions(r.Context(), home.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return true
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, permissions)
		return true
	case http.MethodPut:
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		current, err := s.store.GetHomePermissions(r.Context(), home.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		var body struct {
			HomeAssistant *bool `json:"homeassistant"`
			Files         *bool `json:"files"`
			Notes         *bool `json:"notes"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		if body.HomeAssistant != nil {
			current.HomeAssistantEnabled = *body.HomeAssistant
		}
		if body.Files != nil {
			current.FilesEnabled = *body.Files
		}
		if body.Notes != nil {
			current.NotesEnabled = *body.Notes
		}
		current.UpdatedAt = time.Now().UTC()
		current.UpdatedBy = membership.UserID
		if err := s.store.UpsertHomePermissions(r.Context(), current); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.emitPermissionsChanged(r.Context(), map[string]any{"home_id": home.ID})
		writeJSON(w, http.StatusOK, current)
		return true
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
}

func (s *Server) handleHomeMemberPermissions(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership, targetUserID string) bool {
	if membership.Role != domain.HomeRoleAdmin {
		http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
		return true
	}
	if _, err := s.store.GetHomeMembership(r.Context(), home.ID, targetUserID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return true
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	switch r.Method {
	case http.MethodGet:
		permissions, err := s.store.GetHomeMemberPermissions(r.Context(), home.ID, targetUserID)
		if errors.Is(err, store.ErrNotFound) {
			permissions = domain.HomeMemberPermissions{HomeID: home.ID, UserID: targetUserID}
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, permissions)
		return true
	case http.MethodPut:
		var body struct {
			HomeAssistant *bool `json:"homeassistant"`
			Files         *bool `json:"files"`
			Notes         *bool `json:"notes"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		permissions := domain.HomeMemberPermissions{
			HomeID:               home.ID,
			UserID:               targetUserID,
			HomeAssistantEnabled: body.HomeAssistant,
			FilesEnabled:         body.Files,
			NotesEnabled:         body.Notes,
			UpdatedAt:            time.Now().UTC(),
			UpdatedBy:            membership.UserID,
		}
		if err := s.store.UpsertHomeMemberPermissions(r.Context(), permissions); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.emitPermissionsChanged(r.Context(), map[string]any{"home_id": home.ID, "user_id": targetUserID})
		writeJSON(w, http.StatusOK, permissions)
		return true
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
}
