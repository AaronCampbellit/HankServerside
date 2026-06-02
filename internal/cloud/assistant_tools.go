package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dropfile/hankremote/internal/domain"
)

type assistantToolRuntime struct {
	Home       domain.Home
	Membership domain.HomeMembership
	Auth       authContext
	Settings   domain.AssistantSettings
	Prompt     string
	Session    *domain.AssistantSession
}

type assistantTool struct {
	Kind         assistantIntentKind
	Description  string
	Match        func(prompt string) (assistantIntent, bool)
	RefreshIndex func(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent)
	Execute      func(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error)
}

var assistantToolRegistry = []assistantTool{
	{
		Kind:        assistantIntentHermesChat,
		Description: "Route an explicit /Hermes prompt to the Hermes agent through the home agent.",
		Match: func(prompt string) (assistantIntent, bool) {
			query, ok := hermesCommandPrompt(prompt)
			if !ok {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentHermesChat, Query: query}, true
		},
		Execute: executeAssistantHermesChatTool,
	},
	{
		Kind:        assistantIntentNotesAppend,
		Description: "Append a short item to a uniquely matched Hank note or list.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isNoteAppendPrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentNotesAppend, Query: noteSearchQuery(prompt)}, true
		},
		Execute: executeAssistantNotesAppendTool,
	},
	{
		Kind:        assistantIntentNotesCreate,
		Description: "Create a new personal Hank note after confirmation.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isNoteCreatePrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentNotesCreate, Query: strings.TrimSpace(prompt)}, true
		},
		Execute: executeAssistantNotesCreateTool,
	},
	{
		Kind:        assistantIntentNotesSummarize,
		Description: "Summarize a uniquely matched note or a small matched set of notes.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isNoteSummarizePrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentNotesSummarize, Query: noteSummaryQuery(prompt)}, true
		},
		Execute: executeAssistantNotesSummarizeTool,
	},
	{
		Kind:        assistantIntentCalendarUpdate,
		Description: "Plan a confirmed device calendar event update.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isCalendarMutationUpdatePrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentCalendarUpdate, Query: calendarMutationQuery(prompt)}, true
		},
		Execute: executeAssistantCalendarUpdateTool,
	},
	{
		Kind:        assistantIntentCalendarDelete,
		Description: "Plan a confirmed device calendar event delete.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isCalendarDeletePrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentCalendarDelete, Query: calendarMutationQuery(prompt)}, true
		},
		Execute: executeAssistantCalendarDeleteTool,
	},
	{
		Kind:        assistantIntentCalendarSearch,
		Description: "Search indexed device calendar snapshots.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isCalendarSearchPrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentCalendarSearch, Query: calendarSearchQuery(prompt)}, true
		},
		Execute: executeAssistantCalendarSearchTool,
	},
	{
		Kind:        assistantIntentMemorySearch,
		Description: "Search private HankAI conversation memory.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isAssistantMemoryPrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentMemorySearch, Query: assistantMemoryQuery(prompt)}, true
		},
		Execute: executeAssistantMemorySearchTool,
	},
	{
		Kind:        assistantIntentProjectDocs,
		Description: "Answer questions about Hank Remote project documentation.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isProjectDocsPrompt(strings.ToLower(strings.TrimSpace(prompt))) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentProjectDocs, Query: strings.TrimSpace(prompt)}, true
		},
		RefreshIndex: func(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) {
			if runtime.Settings.ProjectDocsEnabled {
				if err := server.indexAssistantProjectDocs(ctx, runtime.Home.ID, runtime.Auth.User.ID); err != nil {
					server.logger.Warn("assistant project docs indexing failed", "error", err)
				}
			}
		},
		Execute: executeAssistantProjectDocsTool,
	},
	{
		Kind:        assistantIntentHomeAssistantQuery,
		Description: "Read Home Assistant entity state through the home agent.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isHomeAssistantPrompt(strings.ToLower(strings.TrimSpace(prompt))) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentHomeAssistantQuery, Query: homeAssistantQueryDisplay(prompt)}, true
		},
		RefreshIndex: func(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) {
			if runtime.Settings.HomeAssistantEnabled {
				if err := server.indexAssistantHomeAssistantStates(ctx, runtime.Home, runtime.Membership, runtime.Auth); err != nil {
					server.logger.Warn("assistant Home Assistant indexing failed", "error", err)
				}
			}
		},
		Execute: executeAssistantHomeAssistantQueryTool,
	},
	{
		Kind:        assistantIntentNotesList,
		Description: "List all visible personal and shared Hank notes.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isNoteListPrompt(strings.ToLower(strings.TrimSpace(prompt))) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentNotesList, Query: strings.TrimSpace(prompt)}, true
		},
		Execute: executeAssistantNotesTool,
	},
	{
		Kind:        assistantIntentNotesSearch,
		Description: "Search visible personal and shared Hank notes.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isNoteSearchPrompt(strings.ToLower(strings.TrimSpace(prompt))) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentNotesSearch, Query: noteSearchQuery(prompt)}, true
		},
		Execute: executeAssistantNotesTool,
	},
	{
		Kind:        assistantIntentMediaSearch,
		Description: "Search authorized media downloads through the home agent.",
		Match: func(prompt string) (assistantIntent, bool) {
			query, ok := mediaAvailabilityQuery(prompt)
			if !ok {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentMediaSearch, Query: query}, true
		},
		Execute: executeAssistantMediaSearchTool,
	},
	{
		Kind:        assistantIntentFilesCreateFolder,
		Description: "Create a File Server folder after Hank confirmation.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isFileCreateFolderPrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentFilesCreateFolder, Query: parseCreateFolderPath(prompt)}, true
		},
		RefreshIndex: func(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) {
			if runtime.Settings.FilesEnabled {
				if err := server.indexAssistantFiles(ctx, runtime.Home, runtime.Membership, runtime.Auth, runtime.Settings); err != nil {
					server.logger.Warn("assistant file indexing failed", "error", err)
				}
			}
		},
		Execute: executeAssistantFilesCreateFolderTool,
	},
	{
		Kind:        assistantIntentFilesListFolder,
		Description: "List a File Server folder's immediate contents.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isFileListFolderPrompt(prompt) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentFilesListFolder, Query: fileFolderQuery(prompt)}, true
		},
		RefreshIndex: func(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) {
			if runtime.Settings.FilesEnabled {
				if err := server.indexAssistantFiles(ctx, runtime.Home, runtime.Membership, runtime.Auth, runtime.Settings); err != nil {
					server.logger.Warn("assistant file indexing failed", "error", err)
				}
			}
		},
		Execute: executeAssistantFilesListFolderTool,
	},
	{
		Kind:        assistantIntentFilesSearch,
		Description: "Search indexed SMB files and folders through Hank Remote.",
		Match: func(prompt string) (assistantIntent, bool) {
			if !isFileSearchPrompt(strings.ToLower(strings.TrimSpace(prompt))) {
				return assistantIntent{}, false
			}
			return assistantIntent{Kind: assistantIntentFilesSearch, Query: fileQuery(prompt)}, true
		},
		RefreshIndex: func(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) {
			if runtime.Settings.FilesEnabled {
				if err := server.indexAssistantFiles(ctx, runtime.Home, runtime.Membership, runtime.Auth, runtime.Settings); err != nil {
					server.logger.Warn("assistant file indexing failed", "error", err)
				}
			}
		},
		Execute: executeAssistantFilesSearchTool,
	},
	{
		Kind:        assistantIntentGeneral,
		Description: "Answer from already enabled Hank context when no specific tool matches.",
		Match: func(prompt string) (assistantIntent, bool) {
			return assistantIntent{Kind: assistantIntentGeneral, Query: strings.TrimSpace(prompt)}, true
		},
		Execute: executeAssistantGeneralTool,
	},
}

func resolveAssistantTool(prompt string) (assistantTool, assistantIntent) {
	for _, tool := range assistantToolRegistry {
		if tool.Match == nil {
			continue
		}
		intent, ok := tool.Match(prompt)
		if !ok {
			continue
		}
		if intent.Kind == "" {
			intent.Kind = tool.Kind
		}
		return tool, intent
	}
	fallback := assistantToolRegistry[len(assistantToolRegistry)-1]
	return fallback, assistantIntent{Kind: assistantIntentGeneral, Query: strings.TrimSpace(prompt)}
}

func (s *Server) resolveAssistantToolWithLocalModel(ctx context.Context, settings domain.AssistantSettings, userID string, prompt string) (assistantTool, assistantIntent, bool) {
	if settings.AIProvider != "ollama" && settings.PromptProfile != "local" {
		return assistantTool{}, assistantIntent{}, false
	}
	choices := make([]string, 0, len(assistantToolRegistry))
	allowed := map[assistantIntentKind]assistantTool{}
	for _, tool := range assistantToolRegistry {
		if tool.Kind == assistantIntentGeneral || tool.Kind == assistantIntentMediaSelection {
			continue
		}
		choices = append(choices, fmt.Sprintf("- %s: %s", tool.Kind, tool.Description))
		allowed[tool.Kind] = tool
	}
	plannerPrompt := strings.Join([]string{
		"Choose the best Hank tool for this user request.",
		"Return only compact JSON with keys tool and query.",
		"Use tool \"general\" when no listed tool clearly fits.",
		"Do not invent tools. Do not include explanations.",
		"",
		"Available tools:",
		strings.Join(choices, "\n"),
		"",
		"User request:",
		strings.TrimSpace(prompt),
	}, "\n")
	answer, _, err := s.generateAssistantLLMResponseWithSettings(ctx, userID, settings, []assistantLLMMessage{
		{Role: "system", Content: "You classify HankAI requests into existing typed tools. Return JSON only."},
		{Role: "user", Content: plannerPrompt},
	})
	if err != nil {
		s.logger.Warn("assistant local planner failed; using deterministic tool", "error", err)
		return assistantTool{}, assistantIntent{}, false
	}
	toolName, query := assistantPlannerChoice(answer)
	if toolName == assistantIntentGeneral || toolName == "" {
		return assistantTool{}, assistantIntent{}, false
	}
	tool, ok := allowed[toolName]
	if !ok {
		s.logger.Warn("assistant local planner selected unknown tool", "tool", toolName)
		return assistantTool{}, assistantIntent{}, false
	}
	if strings.TrimSpace(query) == "" {
		query = strings.TrimSpace(prompt)
	}
	return tool, assistantIntent{Kind: tool.Kind, Query: query}, true
}

func assistantPlannerChoice(answer string) (assistantIntentKind, string) {
	type plannerResponse struct {
		Tool  string `json:"tool"`
		Query string `json:"query"`
	}
	trimmed := strings.TrimSpace(answer)
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			trimmed = trimmed[start : end+1]
		}
	}
	var response plannerResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return "", ""
	}
	return assistantIntentKind(strings.TrimSpace(response.Tool)), strings.TrimSpace(response.Query)
}

func executeAssistantFilesSearchTool(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error) {
	if !runtime.Settings.FilesEnabled {
		return assistantMessageContent{Text: "File access is turned off in HankAI settings."}, nil
	}
	if err := server.requireHomeFeature(ctx, runtime.Home, runtime.Membership, runtime.Auth.User.ID, domain.HomePermissionFeatureFiles); err != nil {
		if errors.Is(err, errFeaturePermissionDenied) {
			return assistantMessageContent{Text: "File access is disabled for your Home membership right now."}, nil
		}
		return assistantMessageContent{}, err
	}
	return server.answerFilePrompt(ctx, runtime.Home, runtime.Settings, runtime.Prompt)
}

func executeAssistantMediaSearchTool(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error) {
	if !runtime.Settings.FilesEnabled {
		return assistantMessageContent{Text: "File access is turned off in HankAI settings."}, nil
	}
	if err := server.requireHomeFeature(ctx, runtime.Home, runtime.Membership, runtime.Auth.User.ID, domain.HomePermissionFeatureFiles); err != nil {
		if errors.Is(err, errFeaturePermissionDenied) {
			return assistantMessageContent{Text: "File access is disabled for your Home membership right now."}, nil
		}
		return assistantMessageContent{}, err
	}
	if intent.Kind == assistantIntentMediaSelection {
		if intent.MediaSelection == nil {
			return assistantMessageContent{Text: "I need a media option number or title from the last result list."}, nil
		}
		return server.answerMediaSelection(ctx, runtime.Home, *intent.MediaSelection)
	}
	return server.answerMediaSearch(ctx, runtime.Home, intent.Query)
}

func executeAssistantNotesAppendTool(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error) {
	if !runtime.Settings.ProfileNotesEnabled && !runtime.Settings.HomeNotesEnabled {
		return assistantMessageContent{Text: "Notes access is turned off in HankAI settings."}, nil
	}
	if err := server.requireHomeFeature(ctx, runtime.Home, runtime.Membership, runtime.Auth.User.ID, domain.HomePermissionFeatureNotes); err != nil {
		if errors.Is(err, errFeaturePermissionDenied) {
			return assistantMessageContent{Text: "Notes access is disabled for your Home membership right now."}, nil
		}
		return assistantMessageContent{}, err
	}
	return server.answerAppendNotePrompt(ctx, runtime.Home, runtime.Auth, runtime.Settings, runtime.Prompt)
}

func executeAssistantNotesTool(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error) {
	if !runtime.Settings.ProfileNotesEnabled && !runtime.Settings.HomeNotesEnabled {
		return assistantMessageContent{Text: "Notes access is turned off in HankAI settings."}, nil
	}
	if err := server.requireHomeFeature(ctx, runtime.Home, runtime.Membership, runtime.Auth.User.ID, domain.HomePermissionFeatureNotes); err != nil {
		if errors.Is(err, errFeaturePermissionDenied) {
			return assistantMessageContent{Text: "Notes access is disabled for your Home membership right now."}, nil
		}
		return assistantMessageContent{}, err
	}
	if intent.Kind == assistantIntentNotesList {
		return server.answerNoteListPrompt(ctx, runtime.Home, runtime.Auth, runtime.Settings)
	}
	return server.answerNoteSearchPrompt(ctx, runtime.Home, runtime.Auth, runtime.Settings, runtime.Prompt)
}

func executeAssistantHomeAssistantQueryTool(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error) {
	if !runtime.Settings.HomeAssistantEnabled {
		return assistantMessageContent{Text: "Home Assistant access is turned off in HankAI settings."}, nil
	}
	if err := server.requireHomeFeature(ctx, runtime.Home, runtime.Membership, runtime.Auth.User.ID, domain.HomePermissionFeatureHomeAssistant); err != nil {
		if errors.Is(err, errFeaturePermissionDenied) {
			return assistantMessageContent{Text: "Home Assistant access is disabled for your Home membership right now."}, nil
		}
		return assistantMessageContent{}, err
	}
	if isHomeAssistantMutationPrompt(runtime.Prompt) {
		return assistantMessageContent{
			Text: "I can look up Home Assistant state right now, but changing devices still needs a confirmed control workflow before HankAI can run it.",
		}, nil
	}
	return server.answerHomeAssistantPrompt(ctx, runtime.Home, runtime.Prompt)
}

func executeAssistantHermesChatTool(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error) {
	if runtime.Membership.Role != domain.HomeRoleAdmin {
		return assistantMessageContent{Text: "Hermes chat is only available to Home admins right now."}, nil
	}
	if strings.TrimSpace(intent.Query) == "" {
		return assistantMessageContent{Text: "Send `/Hermes` followed by the message you want Hermes to handle."}, nil
	}
	return server.answerHermesChatPrompt(ctx, runtime.Home, runtime.Auth, runtime.Session, intent.Query)
}

func executeAssistantProjectDocsTool(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error) {
	if !runtime.Settings.ProjectDocsEnabled {
		return assistantMessageContent{Text: "Project docs access is turned off in HankAI settings."}, nil
	}
	return server.answerProjectDocPrompt(ctx, runtime.Home, runtime.Auth, runtime.Settings, runtime.Prompt)
}

func executeAssistantGeneralTool(ctx context.Context, server *Server, runtime assistantToolRuntime, intent assistantIntent) (assistantMessageContent, error) {
	if !assistantSettingsHasEnabledSources(runtime.Settings) {
		return assistantMessageContent{Text: "All HankAI sources are turned off in AI Settings."}, nil
	}
	return server.answerRetrievedPrompt(ctx, runtime.Home, runtime.Membership, runtime.Auth, runtime.Settings, runtime.Prompt)
}
