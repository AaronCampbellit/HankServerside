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

type assistantIntentKind string

const (
	assistantIntentGeneral            assistantIntentKind = "general"
	assistantIntentNotesList          assistantIntentKind = "notes.list"
	assistantIntentNotesSearch        assistantIntentKind = "notes.search"
	assistantIntentNotesAppend        assistantIntentKind = "notes.append"
	assistantIntentFilesSearch        assistantIntentKind = "files.search"
	assistantIntentHomeAssistantQuery assistantIntentKind = "homeassistant.query"
	assistantIntentProjectDocs        assistantIntentKind = "project_docs"
)

type assistantIntent struct {
	Kind  assistantIntentKind
	Query string
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
		s.logger.Warn("assistant index stats unavailable", "home_id", home.ID, "user_id", auth.User.ID, "error", err)
		indexStats = domain.AssistantIndexStats{
			VectorAvailable: s.store.VectorAvailable(),
			VectorMode:      "json_fallback",
		}
		if indexStats.VectorAvailable {
			indexStats.VectorMode = "pgvector"
		}
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
	tool, intent := resolveAssistantTool(prompt)
	runtime := assistantToolRuntime{
		Home:       home,
		Membership: membership,
		Auth:       auth,
		Settings:   settings,
		Prompt:     prompt,
	}
	s.refreshAssistantIndex(ctx, runtime, tool, intent)
	if tool.Execute == nil {
		return assistantMessageContent{
			Text: "This HankAI tool is registered but does not have an executor yet.",
		}, nil
	}
	return tool.Execute(ctx, s, runtime, intent)
}

func (s *Server) answerNoteListPrompt(ctx context.Context, home domain.Home, auth authContext, settings domain.AssistantSettings) (assistantMessageContent, error) {
	notes, err := s.assistantVisibleNotes(ctx, home.ID, auth.User.ID, settings)
	if err != nil {
		return assistantMessageContent{}, err
	}
	notes = uniqueAssistantNotes(notes)
	if len(notes) == 0 {
		return assistantMessageContent{
			Text: "I couldn't find any notes you can access in Hank Remote yet.",
		}, nil
	}
	sort.Slice(notes, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(notes[i].Title))
		right := strings.ToLower(strings.TrimSpace(notes[j].Title))
		if left == right {
			return notes[i].UpdatedAt.After(notes[j].UpdatedAt)
		}
		return left < right
	})

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("You have access to %d notes:", len(notes)))
	cards := make([]assistantResultCard, 0, min(len(notes), 12))
	for _, note := range notes {
		title := firstNonBlank(note.Title, note.NoteID, "Untitled Note")
		builder.WriteString("\n- ")
		builder.WriteString(title)
		builder.WriteString(" (")
		builder.WriteString(noteAccessLabel(note))
		builder.WriteString(")")
		if len(cards) < 12 {
			cards = append(cards, assistantResultCard{
				Kind:        "note",
				Title:       title,
				Summary:     notePreview(note.Content),
				ActionTitle: "Open in Notes",
				NoteID:      note.NoteID,
				SearchText:  title,
			})
		}
	}
	return assistantMessageContent{Text: builder.String(), Cards: cards}, nil
}

func (s *Server) answerNoteSearchPrompt(ctx context.Context, home domain.Home, auth authContext, settings domain.AssistantSettings, prompt string) (assistantMessageContent, error) {
	notes, err := s.assistantVisibleNotes(ctx, home.ID, auth.User.ID, settings)
	if err != nil {
		return assistantMessageContent{}, err
	}
	query := noteSearchQuery(prompt)
	results := rankNotes(notes, query)
	if len(results) == 0 {
		return assistantMessageContent{
			Text: "I couldn't find a matching note in Hank Remote yet. Try the note title or ask me to add something to a specific list.",
		}, nil
	}

	top := results[0]
	return assistantMessageContent{
		Text: fmt.Sprintf("I found `%s` in Notes.", top.Title),
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
	matchHint := noteSearchQuery(noteHint)
	ranked := rankScoredNotes(notes, matchHint)
	if len(ranked) == 0 {
		return assistantMessageContent{
			Text: fmt.Sprintf("I couldn't find a shared note matching `%s`.", matchHint),
		}, nil
	}

	target := ranked[0].Note
	if needsNoteAppendConfirmation(ranked) {
		return assistantMessageContent{
			Text: fmt.Sprintf("I found more than one likely note for `%s`. Confirm before I add `%s` to `%s`.", matchHint, itemText, target.Title),
			Cards: []assistantResultCard{
				{
					Kind:        "note",
					Title:       target.Title,
					Summary:     notePreview(target.Content),
					ActionTitle: "Review in Notes",
					NoteID:      target.NoteID,
					SearchText:  matchHint,
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
						MatchHint:     matchHint,
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
	target.BodyMarkdown = newContent
	if target.BodyFormat == "" {
		target.BodyFormat = "markdown"
	}
	target.Revision = revision
	target.Checksum = checksum
	target.UpdatedAt = time.Now().UTC()
	target.UpdatedBy = auth.User.ID
	if err := s.store.UpsertUserNote(ctx, target); err != nil {
		return assistantMessageContent{}, err
	}
	if err := s.indexAssistantNote(ctx, home.ID, auth.User.ID, assistantNoteSourceType(target), target); err != nil {
		s.logger.Warn("assistant note index refresh failed after append", "note_id", target.ID, "error", err)
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

func (s *Server) answerHomeAssistantPrompt(ctx context.Context, home domain.Home, prompt string) (assistantMessageContent, error) {
	envelope, err := s.sendAgentCommand(ctx, home.ID, "homeassistant.fetch_states", map[string]any{})
	if err != nil || envelope.Error != nil {
		return assistantMessageContent{
			Text: "I couldn't reach Home Assistant through the home agent right now.",
		}, nil
	}
	payload, err := protocol.DecodePayload[protocol.HomeAssistantFetchStatesResponse](envelope)
	if err != nil {
		return assistantMessageContent{}, err
	}
	if len(payload.States) == 0 {
		return assistantMessageContent{
			Text: "Home Assistant did not return any entities yet.",
		}, nil
	}

	query := parseHomeAssistantQuery(prompt)
	matches := matchingHomeAssistantStates(payload.States, query)
	if len(matches) == 0 {
		if query.OnlyOn {
			return assistantMessageContent{
				Text: fmt.Sprintf("I checked Home Assistant, but I did not find any entities matching `%s` that are on.", query.Display),
			}, nil
		}
		return assistantMessageContent{
			Text: fmt.Sprintf("I checked Home Assistant, but I did not find any entities matching `%s`.", query.Display),
		}, nil
	}

	limit := min(len(matches), 12)
	var builder strings.Builder
	if query.OnlyOn {
		builder.WriteString("These Home Assistant entities are on:")
	} else if query.Display != "" && query.Display != "entities" {
		builder.WriteString(fmt.Sprintf("I found these Home Assistant entities matching `%s`:", query.Display))
	} else {
		builder.WriteString("I found these Home Assistant entities:")
	}
	for _, state := range matches[:limit] {
		builder.WriteString("\n- ")
		builder.WriteString(homeAssistantStateLabel(state))
		builder.WriteString(": ")
		builder.WriteString(strings.TrimSpace(state.State))
	}
	if remaining := len(matches) - limit; remaining > 0 {
		builder.WriteString(fmt.Sprintf("\n- and %d more", remaining))
	}
	return assistantMessageContent{Text: builder.String()}, nil
}

func (s *Server) answerProjectDocPrompt(ctx context.Context, home domain.Home, auth authContext, settings domain.AssistantSettings, prompt string) (assistantMessageContent, error) {
	settings = normalizeAssistantSettings(settings)
	queryEmbedding, _, _ := s.embedAssistantText(ctx, auth.User.ID, prompt)
	contexts, err := s.store.SearchAssistantContext(ctx, home.ID, auth.User.ID, prompt, queryEmbedding, settings.MaxContextItems)
	if err != nil {
		return assistantMessageContent{}, err
	}
	projectContexts := make([]domain.AssistantRetrievedContext, 0, len(contexts))
	for _, item := range contexts {
		if item.SourceType == assistantProjectDocSourceType && assistantSettingsAllowSource(settings, item.SourceType) {
			projectContexts = append(projectContexts, item)
		}
		if len(projectContexts) >= settings.MaxContextItems {
			break
		}
	}
	if len(projectContexts) == 0 {
		return assistantMessageContent{
			Text: "I could not find matching Hank Remote project documentation for that.",
		}, nil
	}

	answer := fallbackRetrievedAnswer(prompt, projectContexts)
	if providerAnswer, modelName, err := s.generateAssistantLLMResponse(ctx, auth.User.ID, []assistantLLMMessage{
		{
			Role:    "system",
			Content: settings.SystemPrompt,
		},
		{
			Role:    "user",
			Content: assistantPromptWithContext(prompt, projectContexts),
		},
	}); err == nil && strings.TrimSpace(providerAnswer) != "" {
		answer = strings.TrimSpace(providerAnswer)
		_ = modelName
	} else if errors.Is(err, errChatGPTRelinkRequired) {
		return assistantMessageContent{
			Text: "ChatGPT/Codex needs to be linked again before HankAI can use your ChatGPT plan for chat. Open AI Settings and relink ChatGPT/Codex.",
		}, nil
	} else if err != nil {
		s.logger.Warn("assistant project docs provider answer failed; using retrieved fallback", "error", err)
	}

	cards := make([]assistantResultCard, 0, min(len(projectContexts), 3))
	for _, item := range projectContexts {
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
	contexts, err = s.filterAssistantContexts(ctx, home, membership, auth.User.ID, settings, contexts, assistantPromptAllowsProjectDocs(prompt))
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

func uniqueAssistantNotes(notes []domain.UserNote) []domain.UserNote {
	seen := map[string]bool{}
	unique := make([]domain.UserNote, 0, len(notes))
	for _, note := range notes {
		key := firstNonBlank(note.ID, note.NoteID)
		if key == "" {
			key = fmt.Sprintf("%s:%s:%s", note.OwnerUserID, note.HomeID, note.Title)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, note)
	}
	return unique
}

func noteAccessLabel(note domain.UserNote) string {
	if strings.TrimSpace(note.HomeID) != "" {
		return "shared"
	}
	return "personal"
}

func (s *Server) filterAssistantContexts(ctx context.Context, home domain.Home, membership domain.HomeMembership, userID string, settings domain.AssistantSettings, contexts []domain.AssistantRetrievedContext, includeProjectDocs bool) ([]domain.AssistantRetrievedContext, error) {
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
		if item.SourceType == assistantProjectDocSourceType && !includeProjectDocs {
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
	envelope, err := s.sendAgentCommand(ctx, homeID, "files.search", protocol.FilesSearchRequest{Query: query, Limit: 25})
	if err != nil {
		return nil, err
	}
	if envelope.Error == nil {
		payload, err := protocol.DecodePayload[protocol.FilesSearchResponse](envelope)
		if err != nil {
			return nil, err
		}
		sort.Slice(payload.Items, func(i, j int) bool {
			left := fileMatchScore(payload.Items[i], query)
			right := fileMatchScore(payload.Items[j], query)
			if left == right {
				return payload.Items[i].Path < payload.Items[j].Path
			}
			return left > right
		})
		return payload.Items, nil
	}

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

func classifyAssistantIntent(prompt string) assistantIntent {
	_, intent := resolveAssistantTool(prompt)
	return intent
}

func isNoteAppendPrompt(prompt string) bool {
	itemText, noteHint := extractAppendIntent(prompt)
	if strings.TrimSpace(itemText) == "" || strings.TrimSpace(noteHint) == "" {
		return false
	}
	loweredHint := strings.ToLower(noteHint)
	return strings.Contains(loweredHint, "note") ||
		strings.Contains(loweredHint, "list") ||
		strings.Contains(loweredHint, "grocery") ||
		strings.Contains(loweredHint, "groceries") ||
		strings.Contains(loweredHint, "shopping") ||
		strings.Contains(loweredHint, "store") ||
		strings.Contains(loweredHint, "todo") ||
		strings.Contains(loweredHint, "to-do")
}

func isProjectDocsPrompt(lowered string) bool {
	projectTerms := []string{
		"agents.md",
		"readme",
		"server_sync",
		"server sync",
		"project doc",
		"docs/",
		"runbook",
		"hankserverside",
		"hank remote project",
		"hank remote server",
		"deployment",
		"deploy",
		"docker",
		"compose",
		"cloudflare",
		"postgres",
		"pgvector",
		"setup-and-onboarding",
		"setup and onboarding",
		"architecture.md",
		"codebase",
		"repo",
		"repository",
	}
	for _, term := range projectTerms {
		if strings.Contains(lowered, term) {
			return true
		}
	}
	return strings.Contains(lowered, ".md") && (strings.Contains(lowered, "what") || strings.Contains(lowered, "say") || strings.Contains(lowered, "docs"))
}

func isHomeAssistantPrompt(lowered string) bool {
	if strings.Contains(lowered, "home assistant") || strings.Contains(lowered, "hass") || strings.Contains(lowered, " entit") {
		return true
	}
	terms := []string{
		"light", "lights",
		"sensor", "sensors",
		"switch", "switches",
		"fan", "fans",
		"thermostat", "thermostats",
		"cover", "covers",
		"lock", "locks",
		"garage door",
	}
	for _, term := range terms {
		if strings.Contains(lowered, term) {
			return true
		}
	}
	return false
}

func isHomeAssistantMutationPrompt(prompt string) bool {
	lowered := strings.ToLower(strings.TrimSpace(prompt))
	mutationPrefixes := []string{"turn ", "switch ", "set ", "open ", "close ", "lock ", "unlock "}
	for _, prefix := range mutationPrefixes {
		if strings.HasPrefix(lowered, prefix) {
			return true
		}
	}
	return false
}

func isNoteListPrompt(lowered string) bool {
	if !(strings.Contains(lowered, "note") || strings.Contains(lowered, "notes")) {
		return false
	}
	return strings.Contains(lowered, "what notes") ||
		strings.Contains(lowered, "which notes") ||
		strings.Contains(lowered, "all notes") ||
		strings.Contains(lowered, "list notes") ||
		strings.Contains(lowered, "list my notes") ||
		strings.Contains(lowered, "notes do i have") ||
		strings.Contains(lowered, "notes can i access") ||
		strings.Contains(lowered, "notes are there")
}

func isNoteSearchPrompt(lowered string) bool {
	if !(strings.Contains(lowered, "note") ||
		strings.Contains(lowered, "notes") ||
		strings.Contains(lowered, "grocery") ||
		strings.Contains(lowered, "groceries") ||
		strings.Contains(lowered, "shopping list") ||
		strings.Contains(lowered, "store list") ||
		strings.Contains(lowered, "todo list") ||
		strings.Contains(lowered, "to-do list")) {
		return false
	}
	return hasSearchVerb(lowered) ||
		strings.Contains(lowered, "where is") ||
		strings.Contains(lowered, "where are") ||
		strings.Contains(lowered, "show me")
}

func isFileSearchPrompt(lowered string) bool {
	if strings.Contains(lowered, "file") ||
		strings.Contains(lowered, "folder") ||
		strings.Contains(lowered, "directory") ||
		strings.Contains(lowered, "smb") ||
		strings.Contains(lowered, "share") ||
		strings.Contains(lowered, "document") ||
		strings.Contains(lowered, "pdf") ||
		strings.Contains(lowered, " tax") ||
		strings.Contains(lowered, "tax ") ||
		strings.Contains(lowered, "taxes") {
		return true
	}
	return false
}

func hasSearchVerb(lowered string) bool {
	searchPrefixes := []string{"find ", "search ", "search for ", "look for ", "open ", "show ", "show me ", "locate ", "where is ", "where are "}
	for _, prefix := range searchPrefixes {
		if strings.HasPrefix(lowered, prefix) {
			return true
		}
	}
	return false
}

func assistantPromptAllowsProjectDocs(prompt string) bool {
	return isProjectDocsPrompt(strings.ToLower(prompt))
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
	trimmed := strings.TrimSpace(prompt)
	lowered := strings.ToLower(trimmed)
	prefixLength := 0
	switch {
	case strings.HasPrefix(lowered, "add "):
		prefixLength = len("add ")
	case strings.HasPrefix(lowered, "append "):
		prefixLength = len("append ")
	default:
		return trimmed, ""
	}
	rest := strings.TrimSpace(trimmed[prefixLength:])
	loweredRest := strings.ToLower(rest)
	for _, delimiter := range []string{" to the ", " to ", " into the ", " into ", " onto the ", " onto ", " the the "} {
		if index := strings.Index(loweredRest, delimiter); index >= 0 {
			return strings.TrimSpace(strings.TrimSuffix(rest[:index], ".")), cleanNoteHint(rest[index+len(delimiter):])
		}
	}
	return trimmed, ""
}

func cleanNoteHint(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, "."))
	if strings.HasPrefix(strings.ToLower(value), "the ") {
		value = strings.TrimSpace(value[len("the "):])
	}
	return strings.TrimSpace(value)
}

func noteSearchQuery(prompt string) string {
	query := stripAssistantSearchPrefix(prompt)
	query = strings.TrimSuffix(strings.TrimSpace(query), ".")
	query = removeQueryWords(query, map[string]bool{
		"a": true, "an": true, "the": true,
		"note": true, "notes": true,
		"called": true, "named": true, "titled": true,
	})
	if strings.TrimSpace(query) == "" {
		return strings.TrimSpace(prompt)
	}
	return query
}

func fileQuery(prompt string) string {
	query := stripAssistantSearchPrefix(prompt)
	query = strings.TrimSuffix(strings.TrimSpace(query), ".")
	query = removeQueryWords(query, map[string]bool{
		"a": true, "an": true, "the": true,
		"file": true, "files": true,
		"folder": true, "folders": true,
		"directory": true, "directories": true,
		"smb": true, "share": true,
		"called": true, "named": true, "labeled": true,
	})
	if strings.TrimSpace(query) == "" {
		return strings.TrimSpace(prompt)
	}
	return query
}

func stripAssistantSearchPrefix(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	lowered := strings.ToLower(trimmed)
	prefixes := []string{"can you find all ", "can you find ", "can you show me ", "can you show ", "please find all ", "please find ", "search for ", "look for ", "show me ", "where is ", "where are ", "find all ", "find ", "search ", "open ", "show ", "locate ", "get "}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowered, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return trimmed
}

func removeQueryWords(query string, stopWords map[string]bool) string {
	parts := strings.Fields(query)
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.Trim(strings.ToLower(part), ".,?!:;\"'`()[]{}")
		if stopWords[clean] {
			continue
		}
		filtered = append(filtered, strings.Trim(part, ".,?!:;\"'`()[]{}"))
	}
	return strings.TrimSpace(strings.Join(filtered, " "))
}

func assistantNoteSourceType(note domain.UserNote) string {
	if strings.TrimSpace(note.HomeID) != "" {
		return "shared_note"
	}
	return "profile_note"
}

type assistantHomeAssistantQuery struct {
	OnlyOn  bool
	Domain  string
	Terms   []string
	Display string
}

func parseHomeAssistantQuery(prompt string) assistantHomeAssistantQuery {
	lowered := strings.ToLower(strings.TrimSpace(prompt))
	normalized := strings.NewReplacer("?", " ", ".", " ", ",", " ", ":", " ", ";", " ").Replace(lowered)
	tokens := strings.Fields(normalized)
	query := assistantHomeAssistantQuery{
		OnlyOn: strings.Contains(lowered, " are on") ||
			strings.Contains(lowered, " is on") ||
			strings.Contains(lowered, " currently on") ||
			strings.HasSuffix(lowered, " on"),
	}
	stopWords := map[string]bool{
		"what": true, "which": true, "show": true, "list": true, "find": true,
		"can": true, "you": true, "please": true, "me": true, "all": true, "any": true, "are": true, "is": true,
		"there": true, "the": true, "a": true, "an": true, "in": true,
		"of": true, "for": true, "home": true, "assistant": true, "hass": true,
		"entity": true, "entities": true, "entitied": true, "currently": true,
		"on": true, "state": true, "states": true,
	}
	for _, token := range tokens {
		if domain, ok := homeAssistantDomainToken(token); ok {
			if query.Domain == "" {
				query.Domain = domain
			}
			continue
		}
		if stopWords[token] {
			continue
		}
		query.Terms = append(query.Terms, token)
	}
	query.Display = homeAssistantQueryDisplay(prompt)
	if query.Display == "" {
		query.Display = "entities"
	}
	return query
}

func homeAssistantQueryDisplay(prompt string) string {
	query := stripAssistantSearchPrefix(prompt)
	query = removeQueryWords(query, map[string]bool{
		"what": true, "which": true, "show": true, "list": true, "find": true,
		"can": true, "you": true, "please": true, "me": true, "all": true, "any": true, "are": true, "is": true,
		"there": true, "the": true, "a": true, "an": true, "in": true,
		"of": true, "for": true, "home": true, "assistant": true, "hass": true,
		"entity": true, "entities": true, "entitied": true, "currently": true,
		"on": true, "state": true, "states": true,
	})
	return strings.TrimSpace(query)
}

func homeAssistantDomainToken(token string) (string, bool) {
	switch strings.TrimSpace(token) {
	case "light", "lights":
		return "light", true
	case "switch", "switches":
		return "switch", true
	case "sensor", "sensors":
		return "sensor", true
	case "binary_sensor", "binary_sensors":
		return "binary_sensor", true
	case "fan", "fans":
		return "fan", true
	case "thermostat", "thermostats", "climate":
		return "climate", true
	case "cover", "covers", "shade", "shades":
		return "cover", true
	case "lock", "locks":
		return "lock", true
	default:
		return "", false
	}
}

func matchingHomeAssistantStates(states []protocol.HomeAssistantState, query assistantHomeAssistantQuery) []protocol.HomeAssistantState {
	matches := make([]protocol.HomeAssistantState, 0, len(states))
	for _, state := range states {
		if query.OnlyOn && strings.ToLower(strings.TrimSpace(state.State)) != "on" {
			continue
		}
		if query.Domain != "" && !homeAssistantDomainMatches(state.EntityID, query.Domain) {
			continue
		}
		if !homeAssistantStateMatchesTerms(state, query.Terms) {
			continue
		}
		matches = append(matches, state)
	}
	sort.Slice(matches, func(i, j int) bool {
		return strings.ToLower(homeAssistantStateLabel(matches[i])) < strings.ToLower(homeAssistantStateLabel(matches[j]))
	})
	return matches
}

func homeAssistantDomainMatches(entityID string, domain string) bool {
	entityDomain := entityID
	if dot := strings.Index(entityID, "."); dot >= 0 {
		entityDomain = entityID[:dot]
	}
	if domain == "sensor" {
		return entityDomain == "sensor" || entityDomain == "binary_sensor"
	}
	return entityDomain == domain
}

func homeAssistantStateMatchesTerms(state protocol.HomeAssistantState, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	searchText := strings.ToLower(strings.Join([]string{
		state.EntityID,
		state.State,
		homeAssistantFriendlyName(state),
		assistantAttributesText(state.Attributes),
	}, "\n"))
	for _, term := range terms {
		if !strings.Contains(searchText, strings.ToLower(term)) {
			return false
		}
	}
	return true
}

func homeAssistantStateLabel(state protocol.HomeAssistantState) string {
	friendlyName := homeAssistantFriendlyName(state)
	if friendlyName == "" || friendlyName == state.EntityID {
		return state.EntityID
	}
	return fmt.Sprintf("%s (%s)", friendlyName, state.EntityID)
}

func homeAssistantFriendlyName(state protocol.HomeAssistantState) string {
	if state.Attributes == nil {
		return ""
	}
	if value, ok := state.Attributes["friendly_name"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
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
