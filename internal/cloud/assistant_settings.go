package cloud

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

const (
	legacyAssistantSystemPrompt  = "You are HankAI inside Hank Remote. Answer only from the provided Hank context. If the context is not enough, say what is missing. Do not claim you changed notes, files, calendars, or Home Assistant unless a typed tool result says it already happened."
	defaultAssistantSystemPrompt = "You are HankAI, the assistant inside Hank Remote. Prefer typed Hank tools over guessing whenever the user asks about Notes, File Server or SMB shares, Calendar, Home Assistant, Hank project docs, or prior Hank chat. Treat those source names as user concepts and keep them distinct in answers. Stay grounded in the supplied Hank context; if a target note, folder, event, entity, share, calendar, or source is ambiguous, ask one short clarification. Never claim a write happened until a typed tool result or confirmed client-tool result says it happened. For attachments, distinguish staged metadata from raw bytes that the app or home agent must commit. For Calendar, use device timezone context when present and ask for missing date, time, duration, calendar, or ambiguous event matches before writes. For project-doc answers, prefer current README, AGENTS, SERVER_SYNC, docs, deployment docs, and runbooks over archived phase documents unless the user asks for history; cite the doc path or title. Treat external provider access as a privacy boundary: only use context included in the request and do not ask for secrets, passwords, API keys, or private tokens. For destructive or high-impact actions, explain the intended target and require Hank confirmation before execution."

	maxAssistantContextItems      = 20
	maxAssistantSystemPromptBytes = 6000
	maxAssistantChatModelBytes    = 120
)

type assistantSettingsSource struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

type assistantSettingsTool struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	Enabled      bool     `json:"enabled"`
	Status       string   `json:"status"`
	Description  string   `json:"description"`
	Requirements []string `json:"requirements,omitempty"`
}

type assistantSettingsResponse struct {
	Settings domain.AssistantSettings  `json:"settings"`
	Defaults map[string]any            `json:"defaults"`
	Sources  []assistantSettingsSource `json:"sources"`
	Tools    []assistantSettingsTool   `json:"tools"`
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
	ChatModel            *string `json:"chat_model"`
}

func (s *Server) handleAssistantSettings(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.currentAssistantSettings(r.Context(), home.ID, auth.User.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, s.assistantSettingsPayload(home.ID, settings))
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
		writeJSON(w, http.StatusOK, s.assistantSettingsPayload(home.ID, updated))
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
	return current.SystemPrompt != normalized.SystemPrompt || current.MaxContextItems != normalized.MaxContextItems || current.ChatModel != normalized.ChatModel
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
	if request.ChatModel != nil {
		settings.ChatModel = strings.TrimSpace(*request.ChatModel)
	}
	settings = normalizeAssistantSettings(settings)
	if len(settings.SystemPrompt) > maxAssistantSystemPromptBytes {
		return domain.AssistantSettings{}, errors.New("system_prompt is too long")
	}
	if len(settings.ChatModel) > maxAssistantChatModelBytes {
		return domain.AssistantSettings{}, errors.New("chat_model is too long")
	}
	if strings.ContainsAny(settings.ChatModel, " \t\r\n") {
		return domain.AssistantSettings{}, errors.New("chat_model cannot contain whitespace")
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
	settings.ChatModel = strings.TrimSpace(settings.ChatModel)
	settings.MaxContextItems = maxAssistantContextItems
	return settings
}

func (s *Server) assistantSettingsPayload(homeID string, settings domain.AssistantSettings) assistantSettingsResponse {
	settings = normalizeAssistantSettings(settings)
	capabilities := s.agentCapabilities(homeID)
	cfg := s.assistantAI
	cfg.normalize()
	return assistantSettingsResponse{
		Settings: settings,
		Defaults: map[string]any{
			"system_prompt":      defaultAssistantSystemPrompt,
			"max_context_items":  maxAssistantContextItems,
			"chat_model":         "",
			"chat_model_options": assistantChatModelOptions(cfg),
		},
		Sources: assistantSettingsSources(settings),
		Tools:   assistantSettingsTools(settings, capabilities),
	}
}

func (s *Server) agentCapabilities(homeID string) []string {
	if s == nil || s.router == nil {
		return nil
	}
	if agent, ok := s.router.GetAgent(homeID); ok {
		return append([]string(nil), agent.capabilities...)
	}
	return nil
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

func assistantSettingsTools(settings domain.AssistantSettings, capabilities []string) []assistantSettingsTool {
	settings = normalizeAssistantSettings(settings)
	mediaConfigurable := hasCapabilities(capabilities,
		protocol.CommandMediaSettingsStatus,
		protocol.CommandMediaSettingsApply,
	)
	mediaReady := settings.FilesEnabled &&
		hasCapabilities(capabilities,
			protocol.CommandMediaSearch,
			protocol.CommandMediaPlanDownload,
			protocol.CommandMediaDownloadStart,
			protocol.CommandMediaDownloadStatus,
		)
	mediaStatus := "Agent setup needed"
	if !settings.FilesEnabled {
		mediaStatus = "Files off"
	} else if mediaReady {
		mediaStatus = "Ready"
	} else if mediaConfigurable {
		mediaStatus = "Needs Gramaton credentials"
	}
	notesReady := settings.ProfileNotesEnabled || settings.HomeNotesEnabled
	return []assistantSettingsTool{
		{
			Key:         "notes",
			Label:       "Notes",
			Enabled:     notesReady,
			Status:      enabledStatus(notesReady),
			Description: "Create, search, and update enabled note spaces after confirmation when required.",
		},
		{
			Key:         "files",
			Label:       "Files",
			Enabled:     settings.FilesEnabled,
			Status:      enabledStatus(settings.FilesEnabled),
			Description: "Search file names and route approved file work through the home agent.",
		},
		{
			Key:         "media_download",
			Label:       "Media Downloads",
			Enabled:     mediaReady,
			Status:      mediaStatus,
			Description: "Search authorized media sources, prepare a confirmed download plan, and save approved files to the configured Media destination.",
			Requirements: []string{
				"Files enabled",
				"Media source enabled on the home agent",
				"Agent file backend pointed at the Media share",
			},
		},
		{
			Key:         "calendar",
			Label:       "Calendar",
			Enabled:     settings.CalendarEnabled,
			Status:      enabledStatus(settings.CalendarEnabled),
			Description: "Prepare calendar actions for the Hank app confirmation flow.",
		},
		{
			Key:         "homeassistant",
			Label:       "Home Assistant",
			Enabled:     settings.HomeAssistantEnabled,
			Status:      enabledStatus(settings.HomeAssistantEnabled),
			Description: "Read Home Assistant state through the home agent.",
		},
	}
}

func enabledStatus(enabled bool) string {
	if enabled {
		return "Ready"
	}
	return "Off"
}

func hasCapabilities(capabilities []string, required ...string) bool {
	seen := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		seen[capability] = struct{}{}
	}
	for _, capability := range required {
		if _, ok := seen[capability]; !ok {
			return false
		}
	}
	return true
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
