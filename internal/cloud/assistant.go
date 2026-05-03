package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

const (
	assistantRoleUser      = "user"
	assistantRoleAssistant = "assistant"

	assistantStateCompleted         = "completed"
	assistantStateWaitingClientTool = "waiting_client_tool"
	assistantStateWaitingConfirm    = "waiting_confirmation"

	assistantModelName = "hank-assistant-v1"
)

type assistantMessageContent struct {
	Text  string                 `json:"text"`
	Cards []assistantResultCard  `json:"cards,omitempty"`
	Meta  map[string]interface{} `json:"meta,omitempty"`
}

type assistantResultCard struct {
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	ActionTitle string     `json:"action_title"`
	NoteID      string     `json:"note_id,omitempty"`
	EventID     string     `json:"event_id,omitempty"`
	TargetDate  *time.Time `json:"target_date,omitempty"`
	Path        string     `json:"path,omitempty"`
	SearchText  string     `json:"search_text,omitempty"`
}

type assistantClientToolRequest struct {
	ToolName  string                 `json:"tool_name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type assistantPendingAction struct {
	Kind           string                          `json:"kind"`
	NoteAppend     *assistantPendingNoteAppend     `json:"note_append,omitempty"`
	CalendarCreate *assistantPendingCalendarCreate `json:"calendar_create,omitempty"`
}

type assistantPendingNoteAppend struct {
	TargetNoteID  string `json:"target_note_id"`
	TargetNoteKey string `json:"target_note_key"`
	TargetTitle   string `json:"target_title"`
	AppendedText  string `json:"appended_text"`
	MatchHint     string `json:"match_hint"`
	Confirmation  string `json:"confirmation_message"`
}

type assistantPendingCalendarCreate struct {
	ToolRequest  assistantClientToolRequest `json:"tool_request"`
	Title        string                     `json:"title"`
	DateText     string                     `json:"date_text"`
	Confirmation string                     `json:"confirmation_message"`
}

type assistantRankedNote struct {
	Note  domain.UserNote
	Score int
}

type assistantRunResponse struct {
	ID                   string                      `json:"id"`
	State                string                      `json:"state"`
	RequiresClientTools  bool                        `json:"requires_client_tools"`
	RequiresConfirmation bool                        `json:"requires_confirmation"`
	AssistantMessage     *assistantAPIMessage        `json:"assistant_message,omitempty"`
	ClientToolRequest    *assistantClientToolRequest `json:"client_tool_request,omitempty"`
}

type assistantAPISession struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	LastMessageAt time.Time `json:"last_message_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type assistantAPIMessage struct {
	ID        string                `json:"id"`
	Role      string                `json:"role"`
	Text      string                `json:"text"`
	CreatedAt time.Time             `json:"created_at"`
	Cards     []assistantResultCard `json:"cards,omitempty"`
}

type assistantSessionListResponse struct {
	Sessions []assistantAPISession `json:"sessions"`
}

type assistantMessageListResponse struct {
	Messages []assistantAPIMessage `json:"messages"`
}

func (s *Server) handleHomeAssistant(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership, auth authContext, parts []string) bool {
	if len(parts) == 0 || parts[0] != "assistant" {
		return false
	}

	switch {
	case len(parts) == 2 && parts[1] == "sessions":
		s.handleAssistantSessions(w, r, home, auth)
		return true
	case len(parts) == 2 && parts[1] == "status":
		s.handleAssistantStatus(w, r, home, auth)
		return true
	case len(parts) == 2 && parts[1] == "settings":
		s.handleAssistantSettings(w, r, home, auth)
		return true
	case len(parts) == 4 && parts[1] == "sessions" && parts[3] == "messages":
		s.handleAssistantSessionMessages(w, r, home, membership, auth, parts[2])
		return true
	case len(parts) == 3 && parts[1] == "sessions":
		s.handleAssistantSession(w, r, home, auth, parts[2])
		return true
	case len(parts) == 3 && parts[1] == "runs" && r.Method == http.MethodGet:
		s.handleAssistantRun(w, r, home, auth, parts[2])
		return true
	case len(parts) == 4 && parts[1] == "runs" && parts[3] == "client-tool-results":
		s.handleAssistantClientToolResults(w, r, home, auth, parts[2])
		return true
	case len(parts) == 4 && parts[1] == "runs" && parts[3] == "confirm":
		s.handleAssistantConfirm(w, r, home, auth, parts[2])
		return true
	case len(parts) == 2 && parts[1] == "calendar-index":
		s.handleAssistantCalendarIndex(w, r, home, auth)
		return true
	default:
		http.NotFound(w, r)
		return true
	}
}

func (s *Server) handleAssistantStatus(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := s.assistantStatus(r.Context(), auth.User.ID)
	indexStats, err := s.store.AssistantIndexStats(r.Context(), home.ID, auth.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	statusPayload := map[string]any{
		"home_id":              home.ID,
		"provider":             status.Provider,
		"chat_configured":      status.ChatConfigured,
		"embedding_configured": status.EmbeddingConfigured,
		"chat_model":           status.ChatModel,
		"embedding_model":      status.EmbeddingModel,
		"vector_store":         status.VectorStore,
		"index":                indexStats,
	}
	writeJSON(w, http.StatusOK, statusPayload)
}

func (s *Server) handleAssistantSessions(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	switch r.Method {
	case http.MethodGet:
		sessions, err := s.store.ListAssistantSessions(r.Context(), home.ID, auth.User.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response := assistantSessionListResponse{Sessions: make([]assistantAPISession, 0, len(sessions))}
		for _, session := range sessions {
			response.Sessions = append(response.Sessions, assistantSessionToAPI(session))
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPost:
		now := time.Now().UTC()
		session := domain.AssistantSession{
			ID:            newID("asess"),
			HomeID:        home.ID,
			UserID:        auth.User.ID,
			Title:         "New Conversation",
			LastMessageAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := s.store.CreateAssistantSession(r.Context(), session); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, assistantSessionToAPI(session))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAssistantSession(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, sessionID string) {
	session, err := s.store.GetAssistantSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session.HomeID != home.ID || session.UserID != auth.User.ID {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, assistantSessionToAPI(session))
	case http.MethodDelete:
		if err := s.store.DeleteAssistantSession(r.Context(), session.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true, "id": session.ID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAssistantSessionMessages(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership, auth authContext, sessionID string) {
	session, err := s.store.GetAssistantSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session.HomeID != home.ID || session.UserID != auth.User.ID {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		messages, err := s.store.ListAssistantMessages(r.Context(), sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response := assistantMessageListResponse{Messages: make([]assistantAPIMessage, 0, len(messages))}
		for _, message := range messages {
			response.Messages = append(response.Messages, assistantMessageToAPI(message))
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPost:
		var body struct {
			Content            string          `json:"content"`
			ClientCapabilities map[string]bool `json:"client_capabilities"`
			DeviceContext      struct {
				DeviceID string `json:"device_id"`
				Timezone string `json:"timezone"`
			} `json:"device_context"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		content := strings.TrimSpace(body.Content)
		if content == "" {
			http.Error(w, "content is required", http.StatusBadRequest)
			return
		}

		runResponse, err := s.processAssistantMessage(r.Context(), home, membership, auth, session, content, body.DeviceContext.DeviceID, body.DeviceContext.Timezone)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, runResponse)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAssistantRun(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, runID string) {
	run, err := s.store.GetAssistantRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session, err := s.store.GetAssistantSession(r.Context(), run.SessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session.HomeID != home.ID || session.UserID != auth.User.ID {
		http.NotFound(w, r)
		return
	}
	response := assistantRunResponse{
		ID:                   run.ID,
		State:                run.State,
		RequiresClientTools:  run.RequiresClientTools,
		RequiresConfirmation: run.RequiresConfirmation,
	}
	if run.State == assistantStateCompleted {
		if messages, err := s.store.ListAssistantMessages(r.Context(), session.ID); err == nil {
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == assistantRoleAssistant {
					message := assistantMessageToAPI(messages[i])
					response.AssistantMessage = &message
					break
				}
			}
		}
	} else if run.PendingActionJSON != "" {
		var pending assistantClientToolRequest
		if json.Unmarshal([]byte(run.PendingActionJSON), &pending) == nil {
			response.ClientToolRequest = &pending
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAssistantClientToolResults(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	run, err := s.store.GetAssistantRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session, err := s.store.GetAssistantSession(r.Context(), run.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session.HomeID != home.ID || session.UserID != auth.User.ID {
		http.NotFound(w, r)
		return
	}

	var request struct {
		ToolName string                 `json:"tool_name"`
		Result   map[string]interface{} `json:"result"`
		Results  []struct {
			ToolName string                 `json:"tool_name"`
			Result   map[string]interface{} `json:"result"`
			Error    string                 `json:"error"`
		} `json:"results"`
	}
	if err := parseJSON(w, r, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var pending assistantClientToolRequest
	if err := json.Unmarshal([]byte(run.PendingActionJSON), &pending); err != nil {
		http.Error(w, "run is not waiting for a client tool", http.StatusBadRequest)
		return
	}
	if request.ToolName == "" && len(request.Results) > 0 {
		request.ToolName = request.Results[0].ToolName
		request.Result = request.Results[0].Result
	}
	if request.ToolName != pending.ToolName {
		http.Error(w, "client tool does not match pending run", http.StatusBadRequest)
		return
	}

	content, err := s.finalizeAssistantClientToolRun(r.Context(), session, run, request.ToolName, request.Result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	message, err := s.persistAssistantMessage(r.Context(), session, assistantRoleAssistant, content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	completedAt := time.Now().UTC()
	run.State = assistantStateCompleted
	run.RequiresClientTools = false
	run.PendingActionJSON = ""
	run.CompletedAt = &completedAt
	if err := s.store.UpdateAssistantRun(r.Context(), run); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	settings, err := s.currentAssistantSettings(r.Context(), home.ID, auth.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.touchAssistantSessionAndMemory(r.Context(), session, settings, completedAt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, assistantRunResponse{
		ID:               run.ID,
		State:            run.State,
		AssistantMessage: &message,
	})
}

func (s *Server) handleAssistantConfirm(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	run, session, err := s.authorizedAssistantRun(r.Context(), home, auth, runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var request struct {
		Approved bool `json:"approved"`
	}
	if err := parseJSON(w, r, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !run.RequiresConfirmation {
		response := s.assistantRunResponseForSession(r.Context(), session, run)
		writeJSON(w, http.StatusOK, response)
		return
	}

	if !request.Approved {
		completedAt := time.Now().UTC()
		run.State = "cancelled"
		run.RequiresConfirmation = false
		run.PendingActionJSON = ""
		run.CompletedAt = &completedAt
		if err := s.store.UpdateAssistantRun(r.Context(), run); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, assistantRunResponse{
			ID:                   run.ID,
			State:                run.State,
			RequiresClientTools:  false,
			RequiresConfirmation: false,
		})
		return
	}

	var pending assistantPendingAction
	if err := json.Unmarshal([]byte(run.PendingActionJSON), &pending); err != nil {
		http.Error(w, "run is not waiting for a confirmation action", http.StatusBadRequest)
		return
	}

	response, err := s.executeConfirmedAssistantAction(r.Context(), session, run, pending, auth.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAssistantCalendarIndex(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		DeviceID string `json:"device_id"`
		Timezone string `json:"timezone"`
		Entries  []struct {
			ID              string                 `json:"id"`
			ExternalEventID string                 `json:"external_event_id"`
			CalendarID      string                 `json:"calendar_id"`
			CalendarTitle   string                 `json:"calendar_title"`
			Title           string                 `json:"title"`
			Location        string                 `json:"location"`
			Notes           string                 `json:"notes"`
			StartsAt        time.Time              `json:"starts_at"`
			EndsAt          time.Time              `json:"ends_at"`
			IsAllDay        bool                   `json:"is_all_day"`
			Metadata        map[string]interface{} `json:"metadata"`
		} `json:"entries"`
	}
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	deviceID := strings.TrimSpace(body.DeviceID)
	if deviceID == "" {
		deviceID = "ios"
	}

	now := time.Now().UTC()
	entries := make([]domain.AssistantCalendarEntry, 0, len(body.Entries))
	for _, item := range body.Entries {
		metadataJSON, _ := json.Marshal(item.Metadata)
		searchText := strings.TrimSpace(strings.Join([]string{item.Title, item.CalendarTitle, item.Location, item.Notes}, " "))
		calendarID := strings.TrimSpace(item.CalendarID)
		if calendarID == "" {
			calendarID = strings.TrimSpace(item.CalendarTitle)
		}
		entries = append(entries, domain.AssistantCalendarEntry{
			ID:              defaultString(strings.TrimSpace(item.ID), newID("acal")),
			HomeID:          home.ID,
			UserID:          auth.User.ID,
			DeviceID:        deviceID,
			ExternalEventID: strings.TrimSpace(item.ExternalEventID),
			CalendarID:      calendarID,
			Title:           strings.TrimSpace(item.Title),
			Location:        strings.TrimSpace(item.Location),
			Notes:           strings.TrimSpace(item.Notes),
			StartsAt:        item.StartsAt,
			EndsAt:          item.EndsAt,
			IsAllDay:        item.IsAllDay,
			SearchText:      searchText,
			MetadataJSON:    string(metadataJSON),
			UpdatedAt:       now,
		})
	}

	if err := s.store.UpsertAssistantCalendarEntries(r.Context(), entries); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	settings, err := s.currentAssistantSettings(r.Context(), home.ID, auth.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if settings.CalendarEnabled {
		if err := s.indexAssistantCalendarEntries(r.Context(), home.ID, auth.User.ID, entries); err != nil {
			s.logger.Warn("assistant calendar index refresh failed", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "entry_count": len(entries)})
}

func (s *Server) processAssistantMessage(ctx context.Context, home domain.Home, membership domain.HomeMembership, auth authContext, session domain.AssistantSession, content string, deviceID string, timezone string) (assistantRunResponse, error) {
	settings, err := s.currentAssistantSettings(ctx, home.ID, auth.User.ID)
	if err != nil {
		return assistantRunResponse{}, err
	}
	if strings.TrimSpace(session.Title) == "" || session.Title == "New Conversation" {
		session.Title = assistantSessionTitle(content)
	}
	userContent := assistantMessageContent{Text: content}
	userMessage, err := s.persistAssistantMessage(ctx, session, assistantRoleUser, userContent)
	if err != nil {
		return assistantRunResponse{}, err
	}

	run := domain.AssistantRun{
		ID:                   newID("arun"),
		SessionID:            session.ID,
		MessageID:            userMessage.ID,
		State:                assistantStateCompleted,
		RequiresClientTools:  false,
		RequiresConfirmation: false,
		PendingActionJSON:    "",
		CreatedAt:            time.Now().UTC(),
	}

	if pending, ok := s.planCalendarTool(content, timezone, deviceID); ok {
		if !settings.CalendarEnabled {
			message, err := s.persistAssistantMessage(ctx, session, assistantRoleAssistant, assistantMessageContent{
				Text: "Calendar access is turned off in HankAI settings.",
			})
			if err != nil {
				return assistantRunResponse{}, err
			}
			completedAt := time.Now().UTC()
			run.MessageID = message.ID
			run.CompletedAt = &completedAt
			if err := s.store.CreateAssistantRun(ctx, run); err != nil {
				return assistantRunResponse{}, err
			}
			if err := s.touchAssistantSessionAndMemory(ctx, session, settings, completedAt); err != nil {
				return assistantRunResponse{}, err
			}
			return assistantRunResponse{
				ID:               run.ID,
				State:            run.State,
				AssistantMessage: &message,
			}, nil
		}
		if pending.requiresConfirmation {
			message, err := s.persistAssistantMessage(ctx, session, assistantRoleAssistant, assistantMessageContent{
				Text: pending.confirmationMessage,
			})
			if err != nil {
				return assistantRunResponse{}, err
			}
			payload, err := json.Marshal(assistantPendingAction{
				Kind: "calendar_create",
				CalendarCreate: &assistantPendingCalendarCreate{
					ToolRequest:  pending.request,
					Title:        pending.title,
					DateText:     pending.dateText,
					Confirmation: pending.confirmationMessage,
				},
			})
			if err != nil {
				return assistantRunResponse{}, err
			}
			run.State = assistantStateWaitingConfirm
			run.MessageID = message.ID
			run.RequiresConfirmation = true
			run.PendingActionJSON = string(payload)
			if err := s.store.CreateAssistantRun(ctx, run); err != nil {
				return assistantRunResponse{}, err
			}
			if err := s.store.TouchAssistantSession(ctx, session.ID, session.Title, run.CreatedAt); err != nil {
				return assistantRunResponse{}, err
			}
			return assistantRunResponse{
				ID:                   run.ID,
				State:                run.State,
				RequiresConfirmation: true,
				AssistantMessage:     &message,
			}, nil
		}

		payload, err := json.Marshal(pending.request)
		if err != nil {
			return assistantRunResponse{}, err
		}
		run.State = assistantStateWaitingClientTool
		run.RequiresClientTools = true
		run.PendingActionJSON = string(payload)
		if err := s.store.CreateAssistantRun(ctx, run); err != nil {
			return assistantRunResponse{}, err
		}
		if err := s.store.TouchAssistantSession(ctx, session.ID, session.Title, run.CreatedAt); err != nil {
			return assistantRunResponse{}, err
		}
		return assistantRunResponse{
			ID:                  run.ID,
			State:               run.State,
			RequiresClientTools: true,
			ClientToolRequest:   &pending.request,
		}, nil
	}

	assistantContent, err := s.generateAssistantResponse(ctx, home, membership, auth, settings, content)
	if err != nil {
		return assistantRunResponse{}, err
	}
	message, err := s.persistAssistantMessage(ctx, session, assistantRoleAssistant, assistantContent)
	if err != nil {
		return assistantRunResponse{}, err
	}

	if pendingAction := assistantPendingActionFromContent(assistantContent); pendingAction != nil {
		payload, err := json.Marshal(pendingAction)
		if err != nil {
			return assistantRunResponse{}, err
		}
		run.State = assistantStateWaitingConfirm
		run.MessageID = message.ID
		run.RequiresConfirmation = true
		run.PendingActionJSON = string(payload)
		if err := s.store.CreateAssistantRun(ctx, run); err != nil {
			return assistantRunResponse{}, err
		}
		if err := s.store.TouchAssistantSession(ctx, session.ID, session.Title, run.CreatedAt); err != nil {
			return assistantRunResponse{}, err
		}
		return assistantRunResponse{
			ID:                   run.ID,
			State:                run.State,
			RequiresConfirmation: true,
			AssistantMessage:     &message,
		}, nil
	}

	completedAt := time.Now().UTC()
	run.MessageID = message.ID
	run.CompletedAt = &completedAt
	if err := s.store.CreateAssistantRun(ctx, run); err != nil {
		return assistantRunResponse{}, err
	}
	if err := s.touchAssistantSessionAndMemory(ctx, session, settings, completedAt); err != nil {
		return assistantRunResponse{}, err
	}

	return assistantRunResponse{
		ID:               run.ID,
		State:            run.State,
		AssistantMessage: &message,
	}, nil
}

func (s *Server) persistAssistantMessage(ctx context.Context, session domain.AssistantSession, role string, content assistantMessageContent) (assistantAPIMessage, error) {
	encoded, err := json.Marshal(content)
	if err != nil {
		return assistantAPIMessage{}, err
	}
	message := domain.AssistantMessage{
		ID:          newID("amsg"),
		SessionID:   session.ID,
		Role:        role,
		Status:      assistantStateCompleted,
		ContentJSON: string(encoded),
		ModelName:   assistantModelName,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.store.CreateAssistantMessage(ctx, message); err != nil {
		return assistantAPIMessage{}, err
	}
	return assistantMessageToAPI(message), nil
}

func (s *Server) touchAssistantSessionAndMemory(ctx context.Context, session domain.AssistantSession, settings domain.AssistantSettings, updatedAt time.Time) error {
	if err := s.store.TouchAssistantSession(ctx, session.ID, session.Title, updatedAt); err != nil {
		return err
	}
	settings = normalizeAssistantSettings(settings)
	if !settings.ConversationsEnabled {
		return nil
	}
	session.LastMessageAt = updatedAt
	session.UpdatedAt = updatedAt
	if err := s.indexAssistantConversation(ctx, session, session.UserID); err != nil {
		s.logger.Warn("assistant conversation indexing failed", "session_id", session.ID, "error", err)
	}
	return nil
}

func (s *Server) generateAssistantResponse(ctx context.Context, home domain.Home, membership domain.HomeMembership, auth authContext, settings domain.AssistantSettings, prompt string) (assistantMessageContent, error) {
	settings = normalizeAssistantSettings(settings)
	s.refreshAssistantIndex(ctx, home, membership, auth, settings, prompt)

	lowered := strings.ToLower(prompt)
	switch {
	case strings.Contains(lowered, "find ") || strings.Contains(lowered, "folder") || strings.Contains(lowered, "file"):
		if !settings.FilesEnabled {
			return assistantMessageContent{Text: "File access is turned off in HankAI settings."}, nil
		}
		if err := s.requireHomeFeature(ctx, home, membership, auth.User.ID, domain.HomePermissionFeatureFiles); err != nil {
			if errors.Is(err, errFeaturePermissionDenied) {
				return assistantMessageContent{Text: "File access is disabled for your Home membership right now."}, nil
			}
			return assistantMessageContent{}, err
		}
		return s.answerFilePrompt(ctx, home, settings, prompt)
	case strings.Contains(lowered, "add ") && (strings.Contains(lowered, " list") || strings.Contains(lowered, "note")):
		if !settings.ProfileNotesEnabled && !settings.HomeNotesEnabled {
			return assistantMessageContent{Text: "Notes access is turned off in HankAI settings."}, nil
		}
		if err := s.requireHomeFeature(ctx, home, membership, auth.User.ID, domain.HomePermissionFeatureNotes); err != nil {
			if errors.Is(err, errFeaturePermissionDenied) {
				return assistantMessageContent{Text: "Notes access is disabled for your Home membership right now."}, nil
			}
			return assistantMessageContent{}, err
		}
		return s.answerAppendNotePrompt(ctx, home, auth, settings, prompt)
	default:
		if shouldIndexHomeAssistant(prompt) && !settings.HomeAssistantEnabled {
			return assistantMessageContent{Text: "Home Assistant access is turned off in HankAI settings."}, nil
		}
		if !assistantSettingsHasEnabledSources(settings) {
			return assistantMessageContent{Text: "All HankAI sources are turned off in AI Settings."}, nil
		}
		return s.answerRetrievedPrompt(ctx, home, membership, auth, settings, prompt)
	}
}

func (s *Server) answerNoteSearchPrompt(ctx context.Context, home domain.Home, auth authContext, settings domain.AssistantSettings, prompt string) (assistantMessageContent, error) {
	notes, err := s.assistantVisibleNotes(ctx, home.ID, auth.User.ID, settings)
	if err != nil {
		return assistantMessageContent{}, err
	}
	query := strings.TrimSpace(prompt)
	results := rankNotes(notes, query)
	if len(results) == 0 {
		return assistantMessageContent{
			Text: "I couldn't find a matching shared note in Hank Remote yet. Try the note title or ask me to add something to a specific list.",
		}, nil
	}

	top := results[0]
	return assistantMessageContent{
		Text: fmt.Sprintf("I found `%s` and pulled the closest shared note match.", top.Title),
		Cards: []assistantResultCard{
			{
				Kind:        "note",
				Title:       top.Title,
				Summary:     notePreview(top.Content),
				ActionTitle: "Open in Notes",
				NoteID:      top.NoteID,
				SearchText:  query,
			},
		},
	}, nil
}

func (s *Server) answerAppendNotePrompt(ctx context.Context, home domain.Home, auth authContext, settings domain.AssistantSettings, prompt string) (assistantMessageContent, error) {
	itemText, noteHint := extractAppendIntent(prompt)
	if noteHint == "" {
		return s.answerNoteSearchPrompt(ctx, home, auth, settings, prompt)
	}

	notes, err := s.assistantVisibleNotes(ctx, home.ID, auth.User.ID, settings)
	if err != nil {
		return assistantMessageContent{}, err
	}
	ranked := rankScoredNotes(notes, noteHint)
	if len(ranked) == 0 {
		return assistantMessageContent{
			Text: fmt.Sprintf("I couldn't find a shared note matching `%s`.", noteHint),
		}, nil
	}

	target := ranked[0].Note
	if needsNoteAppendConfirmation(ranked) {
		return assistantMessageContent{
			Text: fmt.Sprintf("I found more than one likely note for `%s`. Confirm before I add `%s` to `%s`.", noteHint, itemText, target.Title),
			Cards: []assistantResultCard{
				{
					Kind:        "note",
					Title:       target.Title,
					Summary:     notePreview(target.Content),
					ActionTitle: "Review in Notes",
					NoteID:      target.NoteID,
					SearchText:  noteHint,
				},
			},
			Meta: map[string]interface{}{
				"pending_action": assistantPendingAction{
					Kind: "note_append",
					NoteAppend: &assistantPendingNoteAppend{
						TargetNoteID:  target.ID,
						TargetNoteKey: target.NoteID,
						TargetTitle:   target.Title,
						AppendedText:  itemText,
						MatchHint:     noteHint,
						Confirmation:  fmt.Sprintf("Confirm adding `%s` to `%s`.", itemText, target.Title),
					},
				},
			},
		}, nil
	}

	newContent := strings.TrimSpace(target.Content)
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += "- " + itemText
	revision, checksum, err := revisionAndChecksum(newContent, target.PageType, target.BoardJSON)
	if err != nil {
		return assistantMessageContent{}, err
	}
	target.Content = newContent
	target.Revision = revision
	target.Checksum = checksum
	target.UpdatedAt = time.Now().UTC()
	target.UpdatedBy = auth.User.ID
	if err := s.store.UpsertUserNote(ctx, target); err != nil {
		return assistantMessageContent{}, err
	}

	return assistantMessageContent{
		Text: fmt.Sprintf("Added `%s` to `%s`.", itemText, target.Title),
		Cards: []assistantResultCard{
			{
				Kind:        "note",
				Title:       target.Title,
				Summary:     notePreview(newContent),
				ActionTitle: "Open in Notes",
				NoteID:      target.NoteID,
				SearchText:  itemText,
			},
		},
	}, nil
}

func (s *Server) answerFilePrompt(ctx context.Context, home domain.Home, settings domain.AssistantSettings, prompt string) (assistantMessageContent, error) {
	settings = normalizeAssistantSettings(settings)
	query := fileQuery(prompt)
	queryEmbedding, _, _ := s.embedAssistantText(ctx, "", query)
	if contexts, err := s.store.SearchAssistantContext(ctx, home.ID, "", query, queryEmbedding, 5); err == nil {
		for _, contextItem := range contexts {
			if contextItem.SourceType == "file" && assistantSettingsAllowSource(settings, contextItem.SourceType) {
				return assistantMessageContent{
					Text:  fmt.Sprintf("I found the closest SMB match for `%s`.", query),
					Cards: []assistantResultCard{assistantResultCardFromContext(contextItem)},
				}, nil
			}
		}
	}
	results, err := s.searchFiles(ctx, home.ID, query)
	if err != nil {
		return assistantMessageContent{
			Text: "I couldn't search SMB files because the home agent is offline right now.",
		}, nil
	}
	if len(results) == 0 {
		return assistantMessageContent{
			Text: fmt.Sprintf("I couldn't find an SMB file or folder matching `%s`.", query),
		}, nil
	}

	best := results[0]
	return assistantMessageContent{
		Text: fmt.Sprintf("I found the closest SMB match for `%s`.", query),
		Cards: []assistantResultCard{
			{
				Kind:        "file",
				Title:       best.Name,
				Summary:     best.Path,
				ActionTitle: "Open in File Server",
				Path:        best.Path,
			},
		},
	}, nil
}

func (s *Server) answerRetrievedPrompt(ctx context.Context, home domain.Home, membership domain.HomeMembership, auth authContext, settings domain.AssistantSettings, prompt string) (assistantMessageContent, error) {
	settings = normalizeAssistantSettings(settings)
	queryEmbedding, _, _ := s.embedAssistantText(ctx, auth.User.ID, prompt)
	searchLimit := settings.MaxContextItems * 3
	if searchLimit < 20 {
		searchLimit = 20
	}
	if searchLimit > 60 {
		searchLimit = 60
	}
	contexts, err := s.store.SearchAssistantContext(ctx, home.ID, auth.User.ID, prompt, queryEmbedding, searchLimit)
	if err != nil {
		return assistantMessageContent{}, err
	}
	contexts, err = s.filterAssistantContexts(ctx, home, membership, auth.User.ID, settings, contexts)
	if err != nil {
		return assistantMessageContent{}, err
	}
	if len(contexts) == 0 {
		return assistantMessageContent{
			Text: fmt.Sprintf("I could not find matching Hank context in the sources HankAI can currently use: %s.", assistantSettingsEnabledSourceLabels(settings)),
		}, nil
	}

	answer := fallbackRetrievedAnswer(prompt, contexts)
	if providerAnswer, modelName, err := s.generateAssistantLLMResponse(ctx, auth.User.ID, []assistantLLMMessage{
		{
			Role:    "system",
			Content: settings.SystemPrompt,
		},
		{
			Role:    "user",
			Content: assistantPromptWithContext(prompt, contexts),
		},
	}); err == nil && strings.TrimSpace(providerAnswer) != "" {
		answer = strings.TrimSpace(providerAnswer)
		_ = modelName
	} else if errors.Is(err, errChatGPTRelinkRequired) {
		return assistantMessageContent{
			Text: "ChatGPT/Codex needs to be linked again before HankAI can use your ChatGPT plan for chat. Open AI Settings and relink ChatGPT/Codex.",
		}, nil
	} else if err != nil {
		s.logger.Warn("assistant provider answer failed; using retrieved fallback", "error", err)
	}

	cards := make([]assistantResultCard, 0, min(len(contexts), 3))
	for _, item := range contexts {
		card := assistantResultCardFromContext(item)
		if card.Kind != "" {
			cards = append(cards, card)
		}
		if len(cards) >= 3 {
			break
		}
	}
	return assistantMessageContent{Text: answer, Cards: cards}, nil
}

func (s *Server) assistantVisibleNotes(ctx context.Context, homeID string, userID string, settings domain.AssistantSettings) ([]domain.UserNote, error) {
	settings = normalizeAssistantSettings(settings)
	notes := make([]domain.UserNote, 0)
	if settings.ProfileNotesEnabled {
		profileNotes, err := s.store.ListProfileNotes(ctx, userID, false)
		if err != nil {
			return nil, err
		}
		notes = append(notes, profileNotes...)
	}
	if settings.HomeNotesEnabled {
		homeNotes, err := s.store.ListVisibleHomeNotes(ctx, homeID, userID, false)
		if err != nil {
			return nil, err
		}
		notes = append(notes, homeNotes...)
	}
	return notes, nil
}

func (s *Server) filterAssistantContexts(ctx context.Context, home domain.Home, membership domain.HomeMembership, userID string, settings domain.AssistantSettings, contexts []domain.AssistantRetrievedContext) ([]domain.AssistantRetrievedContext, error) {
	featureAllowed := map[string]bool{}
	for _, feature := range []string{domain.HomePermissionFeatureNotes, domain.HomePermissionFeatureFiles, domain.HomePermissionFeatureHomeAssistant} {
		allowed, err := s.homeFeatureAllowed(ctx, home, membership, userID, feature)
		if err != nil {
			return nil, err
		}
		featureAllowed[feature] = allowed
	}

	filtered := make([]domain.AssistantRetrievedContext, 0, len(contexts))
	for _, item := range contexts {
		if !assistantSettingsAllowSource(settings, item.SourceType) {
			continue
		}
		switch item.SourceType {
		case "profile_note", "shared_note":
			if !featureAllowed[domain.HomePermissionFeatureNotes] {
				continue
			}
		case "file":
			if !featureAllowed[domain.HomePermissionFeatureFiles] {
				continue
			}
		case "homeassistant_entity":
			if !featureAllowed[domain.HomePermissionFeatureHomeAssistant] {
				continue
			}
		}
		filtered = append(filtered, item)
		if len(filtered) >= settings.MaxContextItems {
			break
		}
	}
	return filtered, nil
}

func (s *Server) finalizeAssistantClientToolRun(ctx context.Context, session domain.AssistantSession, run domain.AssistantRun, toolName string, result map[string]interface{}) (assistantMessageContent, error) {
	switch toolName {
	case "calendar.create_event":
		title, _ := result["title"].(string)
		startsAt, _ := result["starts_at"].(string)
		eventID, _ := result["event_id"].(string)
		if title == "" {
			title = "Calendar Event"
		}
		if startsAt == "" {
			startsAt = "the requested time"
		}
		targetDate := parseAssistantResultTime(startsAt)
		return assistantMessageContent{
			Text: fmt.Sprintf("Created `%s` on your device calendar for %s.", title, startsAt),
			Cards: []assistantResultCard{
				{
					Kind:        "calendar",
					Title:       title,
					Summary:     startsAt,
					ActionTitle: "Open in Calendar",
					EventID:     eventID,
					TargetDate:  targetDate,
				},
			},
		}, nil
	case "calendar.update_event":
		title, _ := result["title"].(string)
		startsAt, _ := result["starts_at"].(string)
		eventID, _ := result["event_id"].(string)
		targetDate := parseAssistantResultTime(startsAt)
		if title == "" {
			title = "Updated Calendar Event"
		}
		return assistantMessageContent{
			Text: fmt.Sprintf("Updated `%s` on your device calendar.", title),
			Cards: []assistantResultCard{
				{
					Kind:        "calendar",
					Title:       title,
					Summary:     startsAt,
					ActionTitle: "Open in Calendar",
					EventID:     eventID,
					TargetDate:  targetDate,
				},
			},
		}, nil
	case "calendar.search":
		query, _ := result["query"].(string)
		if matches, ok := result["matches"].([]interface{}); ok && len(matches) > 0 {
			first, _ := matches[0].(map[string]interface{})
			title, _ := first["title"].(string)
			startsAt, _ := first["starts_at"].(string)
			eventID, _ := first["event_id"].(string)
			targetDate := parseAssistantResultTime(startsAt)
			return assistantMessageContent{
				Text: fmt.Sprintf("I found a calendar match for `%s`.", query),
				Cards: []assistantResultCard{
					{
						Kind:        "calendar",
						Title:       defaultString(title, "Calendar Match"),
						Summary:     startsAt,
						ActionTitle: "Open in Calendar",
						EventID:     eventID,
						TargetDate:  targetDate,
						SearchText:  query,
					},
				},
			}, nil
		}
		return assistantMessageContent{
			Text: fmt.Sprintf("I checked your device calendar but did not find a match for `%s`.", query),
		}, nil
	default:
		_ = ctx
		_ = session
		_ = run
		return assistantMessageContent{
			Text: "The client tool finished, but I do not have a formatter for that result yet.",
		}, nil
	}
}

func (s *Server) executeConfirmedAssistantAction(
	ctx context.Context,
	session domain.AssistantSession,
	run domain.AssistantRun,
	pending assistantPendingAction,
	userID string,
) (assistantRunResponse, error) {
	switch pending.Kind {
	case "note_append":
		if pending.NoteAppend == nil {
			return assistantRunResponse{}, errors.New("pending note append is missing")
		}
		note, err := s.store.GetUserNoteByID(ctx, pending.NoteAppend.TargetNoteID)
		if err != nil {
			return assistantRunResponse{}, err
		}
		newContent := strings.TrimSpace(note.Content)
		if newContent != "" && !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		newContent += "- " + pending.NoteAppend.AppendedText
		revision, checksum, err := revisionAndChecksum(newContent, note.PageType, note.BoardJSON)
		if err != nil {
			return assistantRunResponse{}, err
		}
		note.Content = newContent
		note.Revision = revision
		note.Checksum = checksum
		note.UpdatedAt = time.Now().UTC()
		note.UpdatedBy = userID
		if err := s.store.UpsertUserNote(ctx, note); err != nil {
			return assistantRunResponse{}, err
		}
		content := assistantMessageContent{
			Text: fmt.Sprintf("Added `%s` to `%s`.", pending.NoteAppend.AppendedText, note.Title),
			Cards: []assistantResultCard{
				{
					Kind:        "note",
					Title:       note.Title,
					Summary:     notePreview(newContent),
					ActionTitle: "Open in Notes",
					NoteID:      note.NoteID,
					SearchText:  pending.NoteAppend.AppendedText,
				},
			},
		}
		message, err := s.persistAssistantMessage(ctx, session, assistantRoleAssistant, content)
		if err != nil {
			return assistantRunResponse{}, err
		}
		completedAt := time.Now().UTC()
		run.State = assistantStateCompleted
		run.RequiresConfirmation = false
		run.PendingActionJSON = ""
		run.MessageID = message.ID
		run.CompletedAt = &completedAt
		if err := s.store.UpdateAssistantRun(ctx, run); err != nil {
			return assistantRunResponse{}, err
		}
		settings, err := s.currentAssistantSettings(ctx, session.HomeID, userID)
		if err != nil {
			return assistantRunResponse{}, err
		}
		if err := s.touchAssistantSessionAndMemory(ctx, session, settings, completedAt); err != nil {
			return assistantRunResponse{}, err
		}
		return assistantRunResponse{
			ID:               run.ID,
			State:            run.State,
			AssistantMessage: &message,
		}, nil

	case "calendar_create":
		if pending.CalendarCreate == nil {
			return assistantRunResponse{}, errors.New("pending calendar create is missing")
		}
		payload, err := json.Marshal(pending.CalendarCreate.ToolRequest)
		if err != nil {
			return assistantRunResponse{}, err
		}
		run.State = assistantStateWaitingClientTool
		run.RequiresConfirmation = false
		run.RequiresClientTools = true
		run.PendingActionJSON = string(payload)
		run.CompletedAt = nil
		if err := s.store.UpdateAssistantRun(ctx, run); err != nil {
			return assistantRunResponse{}, err
		}
		return assistantRunResponse{
			ID:                  run.ID,
			State:               run.State,
			RequiresClientTools: true,
			ClientToolRequest:   &pending.CalendarCreate.ToolRequest,
		}, nil
	default:
		return assistantRunResponse{}, fmt.Errorf("unsupported pending action kind %q", pending.Kind)
	}
}

func (s *Server) searchFiles(ctx context.Context, homeID string, query string) ([]protocol.FileItem, error) {
	type queueItem struct {
		path  string
		depth int
	}
	queue := []queueItem{{path: "", depth: 0}}
	var matches []protocol.FileItem
	visited := 0

	for len(queue) > 0 && visited < 200 {
		current := queue[0]
		queue = queue[1:]
		visited++

		envelope, err := s.sendAgentCommand(ctx, homeID, "files.list", protocol.FilesListRequest{Path: current.path})
		if err != nil {
			return nil, err
		}
		if envelope.Error != nil {
			return nil, errors.New(envelope.Error.Message)
		}
		payload, err := protocol.DecodePayload[protocol.FilesListResponse](envelope)
		if err != nil {
			return nil, err
		}

		for _, item := range payload.Items {
			score := fileMatchScore(item, query)
			if score > 0 {
				matches = append(matches, item)
			}
			if item.IsDirectory && current.depth < 3 {
				queue = append(queue, queueItem{path: item.Path, depth: current.depth + 1})
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		left := fileMatchScore(matches[i], query)
		right := fileMatchScore(matches[j], query)
		if left == right {
			return matches[i].Path < matches[j].Path
		}
		return left > right
	})
	return matches, nil
}

func (s *Server) planCalendarTool(prompt string, timezone string, deviceID string) (assistantCalendarPlan, bool) {
	intent, ok := parseCalendarCreateIntent(prompt, timezone)
	if !ok {
		return assistantCalendarPlan{}, false
	}
	endsAt := intent.startsAt.Add(24 * time.Hour)
	arguments := map[string]interface{}{
		"title":      intent.title,
		"starts_at":  intent.startsAt.Format(time.RFC3339),
		"ends_at":    endsAt.Format(time.RFC3339),
		"is_all_day": true,
	}
	if deviceID != "" {
		arguments["device_id"] = deviceID
	}
	return assistantCalendarPlan{
		request: assistantClientToolRequest{
			ToolName:  "calendar.create_event",
			Arguments: arguments,
		},
		title:                intent.title,
		dateText:             intent.rawDateText,
		requiresConfirmation: !intent.explicitYear,
		confirmationMessage:  fmt.Sprintf("Confirm creating `%s` on %s.", intent.title, intent.rawDateText),
	}, true
}

func assistantSessionToAPI(session domain.AssistantSession) assistantAPISession {
	return assistantAPISession{
		ID:            session.ID,
		Title:         session.Title,
		LastMessageAt: session.LastMessageAt,
		CreatedAt:     session.CreatedAt,
		UpdatedAt:     session.UpdatedAt,
	}
}

func assistantMessageToAPI(message domain.AssistantMessage) assistantAPIMessage {
	api := assistantAPIMessage{
		ID:        message.ID,
		Role:      message.Role,
		CreatedAt: message.CreatedAt,
	}
	var content assistantMessageContent
	if err := json.Unmarshal([]byte(message.ContentJSON), &content); err == nil {
		api.Text = content.Text
		api.Cards = content.Cards
	}
	return api
}

func assistantPendingActionFromContent(content assistantMessageContent) *assistantPendingAction {
	if content.Meta == nil {
		return nil
	}
	raw, ok := content.Meta["pending_action"]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var pending assistantPendingAction
	if err := json.Unmarshal(encoded, &pending); err != nil {
		return nil
	}
	if pending.Kind == "" {
		return nil
	}
	return &pending
}

func assistantPromptWithContext(prompt string, contexts []domain.AssistantRetrievedContext) string {
	var builder strings.Builder
	builder.WriteString("User request:\n")
	builder.WriteString(prompt)
	builder.WriteString("\n\nHank context:\n")
	for index, item := range contexts {
		builder.WriteString(fmt.Sprintf("%d. [%s] %s\n", index+1, item.SourceType, item.Title))
		if item.Path != "" {
			builder.WriteString("Path: " + item.Path + "\n")
		}
		if item.Snippet != "" {
			builder.WriteString("Snippet: " + item.Snippet + "\n")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func fallbackRetrievedAnswer(prompt string, contexts []domain.AssistantRetrievedContext) string {
	if len(contexts) == 0 {
		return "I could not find matching Hank context yet."
	}
	top := contexts[0]
	return fmt.Sprintf("I found `%s` as the closest HankAI match for `%s`.", top.Title, strings.TrimSpace(prompt))
}

func assistantResultCardFromContext(item domain.AssistantRetrievedContext) assistantResultCard {
	switch item.SourceType {
	case "profile_note", "shared_note":
		return assistantResultCard{
			Kind:        "note",
			Title:       item.Title,
			Summary:     item.Snippet,
			ActionTitle: "Open in Notes",
			NoteID:      item.Path,
			SearchText:  item.Snippet,
		}
	case "calendar_event":
		targetDate := parseAssistantResultTime(item.Path)
		return assistantResultCard{
			Kind:        "calendar",
			Title:       item.Title,
			Summary:     item.Snippet,
			ActionTitle: "Open in Calendar",
			EventID:     item.SourceID,
			TargetDate:  targetDate,
		}
	case "file":
		return assistantResultCard{
			Kind:        "file",
			Title:       item.Title,
			Summary:     item.Snippet,
			ActionTitle: "Open in File Server",
			Path:        item.Path,
		}
	case "homeassistant_entity":
		return assistantResultCard{
			Kind:        "homeassistant",
			Title:       item.Title,
			Summary:     item.Snippet,
			ActionTitle: "Open Dashboard",
			SearchText:  item.SourceID,
		}
	case assistantProjectDocSourceType:
		return assistantResultCard{
			Kind:        "project_doc",
			Title:       item.Title,
			Summary:     item.Snippet,
			ActionTitle: "Open Project Doc",
			Path:        item.Path,
			SearchText:  item.SourceID,
		}
	default:
		return assistantResultCard{}
	}
}

func (s *Server) authorizedAssistantRun(ctx context.Context, home domain.Home, auth authContext, runID string) (domain.AssistantRun, domain.AssistantSession, error) {
	run, err := s.store.GetAssistantRun(ctx, runID)
	if err != nil {
		return domain.AssistantRun{}, domain.AssistantSession{}, err
	}
	session, err := s.store.GetAssistantSession(ctx, run.SessionID)
	if err != nil {
		return domain.AssistantRun{}, domain.AssistantSession{}, err
	}
	if session.HomeID != home.ID || session.UserID != auth.User.ID {
		return domain.AssistantRun{}, domain.AssistantSession{}, store.ErrNotFound
	}
	return run, session, nil
}

func (s *Server) assistantRunResponseForSession(ctx context.Context, session domain.AssistantSession, run domain.AssistantRun) assistantRunResponse {
	response := assistantRunResponse{
		ID:                   run.ID,
		State:                run.State,
		RequiresClientTools:  run.RequiresClientTools,
		RequiresConfirmation: run.RequiresConfirmation,
	}
	if run.State == assistantStateCompleted || run.RequiresConfirmation {
		if messages, err := s.store.ListAssistantMessages(ctx, session.ID); err == nil {
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == assistantRoleAssistant {
					message := assistantMessageToAPI(messages[i])
					response.AssistantMessage = &message
					break
				}
			}
		}
	} else if run.PendingActionJSON != "" {
		var pending assistantClientToolRequest
		if json.Unmarshal([]byte(run.PendingActionJSON), &pending) == nil {
			response.ClientToolRequest = &pending
		}
	}
	return response
}

func needsNoteAppendConfirmation(ranked []assistantRankedNote) bool {
	if len(ranked) == 0 {
		return false
	}
	if ranked[0].Score < 6 {
		return true
	}
	if len(ranked) > 1 && ranked[1].Score >= ranked[0].Score-1 {
		return true
	}
	return false
}

func assistantSessionTitle(prompt string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(prompt, "\n", " "))
	if len(trimmed) <= 42 {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:42]) + "..."
}

func rankScoredNotes(notes []domain.UserNote, query string) []assistantRankedNote {
	query = strings.ToLower(strings.TrimSpace(query))
	scoredNotes := make([]assistantRankedNote, 0, len(notes))
	for _, note := range notes {
		score := 0
		title := strings.ToLower(note.Title)
		content := strings.ToLower(note.Content)
		if strings.Contains(title, query) {
			score += 6
		}
		for _, token := range strings.Fields(query) {
			if strings.Contains(title, token) {
				score += 3
			}
			if strings.Contains(content, token) {
				score++
			}
		}
		if score > 0 {
			scoredNotes = append(scoredNotes, assistantRankedNote{Note: note, Score: score})
		}
	}
	sort.Slice(scoredNotes, func(i, j int) bool {
		if scoredNotes[i].Score == scoredNotes[j].Score {
			return scoredNotes[i].Note.UpdatedAt.After(scoredNotes[j].Note.UpdatedAt)
		}
		return scoredNotes[i].Score > scoredNotes[j].Score
	})
	return scoredNotes
}

func rankNotes(notes []domain.UserNote, query string) []domain.UserNote {
	scored := rankScoredNotes(notes, query)
	results := make([]domain.UserNote, 0, len(scored))
	for _, item := range scored {
		results = append(results, item.Note)
	}
	return results
}

func extractAppendIntent(prompt string) (string, string) {
	lowered := strings.ToLower(strings.TrimSpace(prompt))
	if strings.HasPrefix(lowered, "add ") && strings.Contains(lowered, " to ") {
		rest := strings.TrimSpace(prompt[4:])
		parts := strings.SplitN(rest, " to ", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(strings.TrimSuffix(parts[0], ".")), strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
		}
	}
	return strings.TrimSpace(prompt), ""
}

func fileQuery(prompt string) string {
	lowered := strings.ToLower(strings.TrimSpace(prompt))
	lowered = strings.TrimPrefix(lowered, "find ")
	lowered = strings.TrimSuffix(lowered, ".")
	return strings.TrimSpace(lowered)
}

func parseAssistantResultTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return &parsed
	}
	return nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func fileMatchScore(item protocol.FileItem, query string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 0
	}
	name := strings.ToLower(item.Name)
	fullPath := strings.ToLower(item.Path)
	score := 0
	if strings.Contains(name, query) {
		score += 8
	}
	if strings.Contains(fullPath, query) {
		score += 5
	}
	for _, token := range strings.Fields(query) {
		if strings.Contains(name, token) {
			score += 3
		}
		if strings.Contains(fullPath, token) {
			score++
		}
	}
	if item.IsDirectory {
		score++
	}
	return score
}

func notePreview(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 120 {
		return content
	}
	return strings.TrimSpace(content[:120]) + "..."
}

type assistantCalendarIntent struct {
	title        string
	startsAt     time.Time
	rawDateText  string
	explicitYear bool
}

type assistantCalendarPlan struct {
	request              assistantClientToolRequest
	title                string
	dateText             string
	requiresConfirmation bool
	confirmationMessage  string
}

func parseCalendarCreateIntent(prompt string, timezone string) (assistantCalendarIntent, bool) {
	lowered := strings.ToLower(strings.TrimSpace(prompt))
	if !strings.HasPrefix(lowered, "add ") && !strings.HasPrefix(lowered, "create ") && !strings.HasPrefix(lowered, "schedule ") {
		return assistantCalendarIntent{}, false
	}

	var splitToken string
	switch {
	case strings.Contains(lowered, " to "):
		splitToken = " to "
	case strings.Contains(lowered, " on "):
		splitToken = " on "
	default:
		return assistantCalendarIntent{}, false
	}

	pieces := strings.SplitN(strings.TrimSpace(prompt), splitToken, 2)
	if len(pieces) != 2 {
		return assistantCalendarIntent{}, false
	}

	title := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(pieces[0], "Add "), "Create "), "Schedule "))
	dateText := strings.TrimSpace(strings.TrimSuffix(pieces[1], "."))
	if title == "" || dateText == "" {
		return assistantCalendarIntent{}, false
	}

	location := time.Local
	if timezone != "" {
		if loaded, err := time.LoadLocation(timezone); err == nil {
			location = loaded
		}
	}
	now := time.Now().In(location)
	for _, layout := range []string{"January 2 2006", "Jan 2 2006", "January 2", "Jan 2"} {
		parsed, err := time.ParseInLocation(layout, dateText, location)
		if err != nil {
			continue
		}
		if !strings.Contains(layout, "2006") {
			parsed = time.Date(now.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, location)
		}
		if parsed.Before(now.Add(-24 * time.Hour)) {
			parsed = time.Date(now.Year()+1, parsed.Month(), parsed.Day(), 0, 0, 0, 0, location)
		}
		return assistantCalendarIntent{
			title:        title,
			startsAt:     parsed,
			rawDateText:  dateText,
			explicitYear: strings.Contains(layout, "2006"),
		}, true
	}
	return assistantCalendarIntent{}, false
}
