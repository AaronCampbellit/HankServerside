package cloud

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

const (
	legacyAssistantSystemPrompt  = "You are HankAI inside Hank Remote. Answer only from the provided Hank context. If the context is not enough, say what is missing. Do not claim you changed notes, files, calendars, or Home Assistant unless a typed tool result says it already happened."
	defaultAssistantSystemPrompt = "You are HankAI, the assistant inside Hank Remote. Use the enabled Hank context sources to answer questions about the user's home, notes, files, calendar, Home Assistant state, and Hank Remote project documentation. Stay grounded in the supplied context; if the context is missing or stale, say what is missing instead of guessing. Keep answers practical and concise, with concrete next steps when useful. Do not claim you changed notes, files, calendars, Home Assistant, server settings, or project files unless a typed Hank tool result says that action already happened. Treat external provider access as a privacy boundary: only use the context included in the request and do not ask for secrets, passwords, API keys, or private tokens. For project questions, prefer README, AGENTS, SERVER_SYNC, docs, and runbooks over general assumptions. For home-control or destructive actions, explain the intended action and require confirmation through Hank before execution."

	maxAssistantContextItems      = 20
	maxAssistantSystemPromptBytes = 6000
)

type assistantSettingsSource struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

type assistantSettingsResponse struct {
	Settings domain.AssistantSettings  `json:"settings"`
	Defaults map[string]any            `json:"defaults"`
	Sources  []assistantSettingsSource `json:"sources"`
}

type assistantSettingsUpdateRequest struct {
	ProfileNotesEnabled  *bool   `json:"profile_notes_enabled"`
	HomeNotesEnabled     *bool   `json:"home_notes_enabled"`
	FilesEnabled         *bool   `json:"files_enabled"`
	CalendarEnabled      *bool   `json:"calendar_enabled"`
	HomeAssistantEnabled *bool   `json:"homeassistant_enabled"`
	ProjectDocsEnabled   *bool   `json:"project_docs_enabled"`
	ConversationsEnabled *bool   `json:"conversations_enabled"`
	SystemPrompt         *string `json:"system_prompt"`
}

func (s *Server) handleAssistantSettings(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.currentAssistantSettings(r.Context(), home.ID, auth.User.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, assistantSettingsPayload(settings))
	case http.MethodPut:
		current, err := s.currentAssistantSettings(r.Context(), home.ID, auth.User.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var request assistantSettingsUpdateRequest
		if err := parseJSON(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, err := applyAssistantSettingsUpdate(current, request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		updated.UpdatedAt = now
		updated.UpdatedBy = auth.User.ID
		if updated.CreatedAt.IsZero() {
			updated.CreatedAt = now
		}
		if err := s.store.UpsertAssistantSettings(r.Context(), updated); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, assistantSettingsPayload(updated))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) currentAssistantSettings(ctx context.Context, homeID string, userID string) (domain.AssistantSettings, error) {
	settings, err := s.store.GetAssistantSettings(ctx, homeID, userID)
	if err == nil {
		normalized := normalizeAssistantSettings(settings)
		if assistantSettingsNeedPersistence(settings, normalized) {
			now := time.Now().UTC()
			normalized.UpdatedAt = now
			normalized.UpdatedBy = userID
			if normalized.CreatedAt.IsZero() {
				normalized.CreatedAt = now
			}
			if err := s.store.UpsertAssistantSettings(ctx, normalized); err != nil {
				return domain.AssistantSettings{}, err
			}
		}
		return normalized, nil
	}
	if errors.Is(err, store.ErrNotFound) {
		defaults := defaultAssistantSettings(homeID, userID)
		if err := s.store.UpsertAssistantSettings(ctx, defaults); err != nil {
			return domain.AssistantSettings{}, err
		}
		return defaults, nil
	}
	return domain.AssistantSettings{}, err
}

func assistantSettingsNeedPersistence(current domain.AssistantSettings, normalized domain.AssistantSettings) bool {
	return current.SystemPrompt != normalized.SystemPrompt || current.MaxContextItems != normalized.MaxContextItems
}

func defaultAssistantSettings(homeID string, userID string) domain.AssistantSettings {
	now := time.Now().UTC()
	return domain.AssistantSettings{
		HomeID:               homeID,
		UserID:               userID,
		ProfileNotesEnabled:  true,
		HomeNotesEnabled:     true,
		FilesEnabled:         true,
		CalendarEnabled:      true,
		HomeAssistantEnabled: true,
		ProjectDocsEnabled:   true,
		ConversationsEnabled: true,
		SystemPrompt:         defaultAssistantSystemPrompt,
		MaxContextItems:      maxAssistantContextItems,
		CreatedAt:            now,
		UpdatedAt:            now,
		UpdatedBy:            userID,
	}
}

func applyAssistantSettingsUpdate(settings domain.AssistantSettings, request assistantSettingsUpdateRequest) (domain.AssistantSettings, error) {
	if request.ProfileNotesEnabled != nil {
		settings.ProfileNotesEnabled = *request.ProfileNotesEnabled
	}
	if request.HomeNotesEnabled != nil {
		settings.HomeNotesEnabled = *request.HomeNotesEnabled
	}
	if request.FilesEnabled != nil {
		settings.FilesEnabled = *request.FilesEnabled
	}
	if request.CalendarEnabled != nil {
		settings.CalendarEnabled = *request.CalendarEnabled
	}
	if request.HomeAssistantEnabled != nil {
		settings.HomeAssistantEnabled = *request.HomeAssistantEnabled
	}
	if request.ProjectDocsEnabled != nil {
		settings.ProjectDocsEnabled = *request.ProjectDocsEnabled
	}
	if request.ConversationsEnabled != nil {
		settings.ConversationsEnabled = *request.ConversationsEnabled
	}
	if request.SystemPrompt != nil {
		settings.SystemPrompt = strings.TrimSpace(*request.SystemPrompt)
	}
	settings = normalizeAssistantSettings(settings)
	if len(settings.SystemPrompt) > maxAssistantSystemPromptBytes {
		return domain.AssistantSettings{}, errors.New("system_prompt is too long")
	}
	return settings, nil
}

func normalizeAssistantSettings(settings domain.AssistantSettings) domain.AssistantSettings {
	trimmedPrompt := strings.TrimSpace(settings.SystemPrompt)
	if trimmedPrompt == "" || trimmedPrompt == legacyAssistantSystemPrompt {
		settings.SystemPrompt = defaultAssistantSystemPrompt
	} else {
		settings.SystemPrompt = trimmedPrompt
	}
	settings.MaxContextItems = maxAssistantContextItems
	return settings
}

func assistantSettingsPayload(settings domain.AssistantSettings) assistantSettingsResponse {
	settings = normalizeAssistantSettings(settings)
	return assistantSettingsResponse{
		Settings: settings,
		Defaults: map[string]any{
			"system_prompt":     defaultAssistantSystemPrompt,
			"max_context_items": maxAssistantContextItems,
		},
		Sources: assistantSettingsSources(settings),
	}
}

func assistantSettingsSources(settings domain.AssistantSettings) []assistantSettingsSource {
	settings = normalizeAssistantSettings(settings)
	return []assistantSettingsSource{
		{
			Key:         "project_docs",
			Label:       "Project docs",
			Enabled:     settings.ProjectDocsEnabled,
			Description: "README, AGENTS, SERVER_SYNC, and docs markdown from HankServerside.",
		},
		{
			Key:         "assistant_conversation",
			Label:       "Past conversations",
			Enabled:     settings.ConversationsEnabled,
			Description: "Your private HankAI conversation history.",
		},
		{
			Key:         "profile_notes",
			Label:       "Personal notes",
			Enabled:     settings.ProfileNotesEnabled,
			Description: "Notes stored in your Hank profile.",
		},
		{
			Key:         "home_notes",
			Label:       "Shared notes",
			Enabled:     settings.HomeNotesEnabled,
			Description: "Notes shared with your Home.",
		},
		{
			Key:         "files",
			Label:       "Files",
			Enabled:     settings.FilesEnabled,
			Description: "File and folder names indexed from the home agent.",
		},
		{
			Key:         "calendar",
			Label:       "Calendar",
			Enabled:     settings.CalendarEnabled,
			Description: "Calendar entries synced from Hank clients.",
		},
		{
			Key:         "homeassistant",
			Label:       "Home Assistant",
			Enabled:     settings.HomeAssistantEnabled,
			Description: "Home Assistant entity names and states.",
		},
	}
}

func assistantSettingsAllowSource(settings domain.AssistantSettings, sourceType string) bool {
	settings = normalizeAssistantSettings(settings)
	switch sourceType {
	case "profile_note":
		return settings.ProfileNotesEnabled
	case "shared_note":
		return settings.HomeNotesEnabled
	case "file":
		return settings.FilesEnabled
	case "calendar_event":
		return settings.CalendarEnabled
	case "homeassistant_entity":
		return settings.HomeAssistantEnabled
	case assistantProjectDocSourceType:
		return settings.ProjectDocsEnabled
	case assistantConversationSourceType:
		return settings.ConversationsEnabled
	default:
		return false
	}
}

func assistantSettingsHasEnabledSources(settings domain.AssistantSettings) bool {
	for _, source := range assistantSettingsSources(settings) {
		if source.Enabled {
			return true
		}
	}
	return false
}

func assistantSettingsEnabledSourceLabels(settings domain.AssistantSettings) string {
	sources := assistantSettingsSources(settings)
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		if source.Enabled {
			labels = append(labels, source.Label)
		}
	}
	if len(labels) == 0 {
		return "no Hank sources"
	}
	return strings.Join(labels, ", ")
}
