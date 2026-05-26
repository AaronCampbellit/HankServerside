package cloud

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

func TestAssistantSettingsEndpointUpdatesHarness(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_harness_settings", Email: "harness-settings@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_harness_settings", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AppSession{ID: "sess_harness_settings", UserID: user.ID, TokenHash: hashToken("harness-settings-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, session))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var defaults assistantSettingsResponse
	requestJSON(t, testServer, "harness-settings-token", http.MethodGet, "/v1/home/assistant/settings", nil, &defaults)
	if !defaults.Settings.FilesEnabled || !defaults.Settings.HomeAssistantEnabled || defaults.Settings.SystemPrompt != defaultAssistantSystemPrompt {
		t.Fatalf("default settings = %#v", defaults.Settings)
	}
	if !defaults.Settings.ProfileNotesEnabled || !defaults.Settings.HomeNotesEnabled {
		t.Fatalf("note source defaults = false, settings=%#v", defaults.Settings)
	}
	if !defaults.Settings.ProjectDocsEnabled {
		t.Fatalf("project docs default = false, settings=%#v", defaults.Settings)
	}
	if defaults.Settings.MaxContextItems != maxAssistantContextItems {
		t.Fatalf("default max context = %d, want %d", defaults.Settings.MaxContextItems, maxAssistantContextItems)
	}
	if defaults.Settings.ChatModel != "" {
		t.Fatalf("default chat model override = %q, want empty", defaults.Settings.ChatModel)
	}
	if !strings.Contains(defaults.Settings.SystemPrompt, "You are HankAI") || !strings.Contains(defaults.Settings.SystemPrompt, "privacy boundary") {
		t.Fatalf("default prompt does not include harness guidance: %q", defaults.Settings.SystemPrompt)
	}
	if toolStatus(defaults.Tools, "media_download") != "Agent setup needed" {
		t.Fatalf("default media tool status = %#v", defaults.Tools)
	}
	legacySettings := normalizeAssistantSettings(domain.AssistantSettings{SystemPrompt: legacyAssistantSystemPrompt})
	if legacySettings.SystemPrompt != defaultAssistantSystemPrompt {
		t.Fatalf("legacy prompt was not upgraded")
	}

	var updated assistantSettingsResponse
	requestJSON(t, testServer, "harness-settings-token", http.MethodPut, "/v1/home/assistant/settings", map[string]any{
		"files_enabled":         false,
		"calendar_enabled":      false,
		"homeassistant_enabled": true,
		"project_docs_enabled":  false,
		"profile_notes_enabled": false,
		"home_notes_enabled":    true,
		"chat_model":            "gpt-codex-large",
		"system_prompt":         "Use only the supplied Hank test context.",
	}, &updated)
	if updated.Settings.FilesEnabled || updated.Settings.CalendarEnabled || updated.Settings.ProjectDocsEnabled {
		t.Fatalf("source toggles were not saved: %#v", updated.Settings)
	}
	if updated.Settings.ProfileNotesEnabled || !updated.Settings.HomeNotesEnabled {
		t.Fatalf("note toggles were not saved: %#v", updated.Settings)
	}
	if updated.Settings.SystemPrompt != "Use only the supplied Hank test context." || updated.Settings.MaxContextItems != maxAssistantContextItems || updated.Settings.ChatModel != "gpt-codex-large" {
		t.Fatalf("prompt/context settings = %#v", updated.Settings)
	}
	if toolStatus(updated.Tools, "media_download") != "Files off" {
		t.Fatalf("media tool status after disabling files = %#v", updated.Tools)
	}

	var persisted assistantSettingsResponse
	requestJSON(t, testServer, "harness-settings-token", http.MethodGet, "/v1/home/assistant/settings", nil, &persisted)
	if persisted.Settings.FilesEnabled || persisted.Settings.CalendarEnabled || persisted.Settings.ProjectDocsEnabled || persisted.Settings.MaxContextItems != maxAssistantContextItems || persisted.Settings.ChatModel != "gpt-codex-large" {
		t.Fatalf("persisted settings = %#v", persisted.Settings)
	}
}

func toolStatus(tools []assistantSettingsTool, key string) string {
	for _, tool := range tools {
		if tool.Key == key {
			return tool.Status
		}
	}
	return ""
}

func TestAssistantSessionCanBeDeleted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_delete_assistant_session", Email: "delete-assistant-session@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_delete_assistant_session", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AppSession{ID: "sess_delete_assistant_session", UserID: user.ID, TokenHash: hashToken("delete-assistant-session-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, session))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var created assistantAPISession
	requestJSON(t, testServer, "delete-assistant-session-token", http.MethodPost, "/v1/home/assistant/sessions", nil, &created)
	if created.ID == "" {
		t.Fatal("created session has no id")
	}

	requestJSON(t, testServer, "delete-assistant-session-token", http.MethodDelete, "/v1/home/assistant/sessions/"+created.ID, nil, nil)

	var list assistantSessionListResponse
	requestJSON(t, testServer, "delete-assistant-session-token", http.MethodGet, "/v1/home/assistant/sessions", nil, &list)
	if len(list.Sessions) != 0 {
		t.Fatalf("sessions after delete = %#v", list.Sessions)
	}
	if _, err := db.GetAssistantSession(ctx, created.ID); err != store.ErrNotFound {
		t.Fatalf("GetAssistantSession err = %v, want ErrNotFound", err)
	}
}

func TestAssistantSettingsPromptAndSourceFiltersAffectLLMRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_harness_prompt", Email: "harness-prompt@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_harness_prompt", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))

	userID := user.ID
	must(t, db.UpsertAssistantDocumentWithChunks(ctx, domain.AssistantDocument{
		ID:           "adoc_harness_note",
		HomeID:       home.ID,
		UserID:       &userID,
		SourceType:   "profile_note",
		SourceID:     "note-harness",
		SourceKey:    "profile_note:" + user.ID + ":note-harness",
		Title:        "Tax note",
		Path:         "tax-note",
		CanonicalURI: "hank://notes/tax-note",
		MetadataJSON: "{}",
		SearchText:   "tax refund household note",
		UpdatedAt:    now,
	}, []domain.AssistantChunk{{
		ID:               "achunk_harness_note",
		Content:          "tax refund household note",
		TokenCount:       4,
		EmbeddingJSON:    "[]",
		EmbeddingModel:   "test",
		EmbeddingVersion: "test",
		UpdatedAt:        now,
	}}))
	must(t, db.UpsertAssistantFileIndex(ctx, domain.AssistantFileIndex{
		ID:               "afile_harness_tax",
		HomeID:           home.ID,
		Path:             "/private/tax.pdf",
		Name:             "tax.pdf",
		SearchText:       "tax refund private file",
		MetadataJSON:     "{}",
		EmbeddingJSON:    "[]",
		EmbeddingModel:   "test",
		EmbeddingVersion: "test",
		UpdatedAt:        now,
	}))

	var sentMessages []assistantLLMMessage
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []assistantLLMMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		sentMessages = body.Messages
		writeJSON(w, http.StatusOK, map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "Filtered answer."}},
			},
		})
	}))
	defer provider.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: "openai", OpenAIBaseURL: provider.URL, OpenAIAPIKey: "api-key", OpenAIChatModel: "gpt-test"})

	settings := defaultAssistantSettings(home.ID, user.ID)
	settings.SystemPrompt = "Use only visible Hank harness context."
	settings.FilesEnabled = false
	auth := authContext{User: user}
	membership := domain.HomeMembership{HomeID: home.ID, UserID: user.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}

	answer, err := server.answerRetrievedPrompt(ctx, home, membership, auth, settings, "tax refund")
	if err != nil {
		t.Fatalf("answerRetrievedPrompt: %v", err)
	}
	if answer.Text != "Filtered answer." {
		t.Fatalf("answer = %#v", answer)
	}
	if len(sentMessages) != 2 {
		t.Fatalf("messages = %#v", sentMessages)
	}
	if sentMessages[0].Content != "Use only visible Hank harness context." {
		t.Fatalf("system prompt = %q", sentMessages[0].Content)
	}
	if strings.Contains(sentMessages[1].Content, "[file]") || strings.Contains(sentMessages[1].Content, "/private/tax.pdf") {
		t.Fatalf("file context leaked into provider prompt: %s", sentMessages[1].Content)
	}
	if !strings.Contains(sentMessages[1].Content, "[profile_note] Tax note") {
		t.Fatalf("note context missing from provider prompt: %s", sentMessages[1].Content)
	}
}

func TestAssistantProjectDocsAreIndexedAsHarnessSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_project_docs", Email: "project-docs@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_project_docs", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))

	root := t.TempDir()
	must(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# Hank Remote Test README\n\nThe frobnicator deployment rule lives here."), 0o600))
	must(t, os.MkdirAll(filepath.Join(root, "docs", "runbooks"), 0o700))
	must(t, os.WriteFile(filepath.Join(root, "docs", "runbooks", "frobnicator.md"), []byte("# Frobnicator Runbook\n\nRestart the frobnicator from the Hank cloud service."), 0o600))

	var sentMessages []assistantLLMMessage
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []assistantLLMMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		sentMessages = body.Messages
		writeJSON(w, http.StatusOK, map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "Project docs answer."}},
			},
		})
	}))
	defer provider.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{
		Provider:        "openai",
		OpenAIBaseURL:   provider.URL,
		OpenAIAPIKey:    "api-key",
		OpenAIChatModel: "gpt-test",
		ProjectDocsDir:  root,
	})

	settings := defaultAssistantSettings(home.ID, user.ID)
	settings.ProfileNotesEnabled = false
	settings.HomeNotesEnabled = false
	settings.FilesEnabled = false
	settings.CalendarEnabled = false
	settings.HomeAssistantEnabled = false
	settings.ProjectDocsEnabled = true
	auth := authContext{User: user}
	membership := domain.HomeMembership{HomeID: home.ID, UserID: user.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}

	answer, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, "frobnicator deployment rule")
	if err != nil {
		t.Fatalf("generateAssistantResponse: %v", err)
	}
	if answer.Text != "Project docs answer." {
		t.Fatalf("answer = %#v", answer)
	}
	if len(sentMessages) != 2 {
		t.Fatalf("messages = %#v", sentMessages)
	}
	if !strings.Contains(sentMessages[1].Content, "[project_doc]") || !strings.Contains(sentMessages[1].Content, "frobnicator") {
		t.Fatalf("project docs were not sent as context: %s", sentMessages[1].Content)
	}
}

func TestAssistantConversationMemoryIsIndexedAndFiltered(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_conversation_memory", Email: "conversation-memory@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_conversation_memory", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AssistantSession{ID: "asess_conversation_memory", HomeID: home.ID, UserID: user.ID, Title: "New Conversation", LastMessageAt: now, CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateAssistantSession(ctx, session))

	settings := defaultAssistantSettings(home.ID, user.ID)
	settings.ProfileNotesEnabled = false
	settings.HomeNotesEnabled = false
	settings.FilesEnabled = false
	settings.CalendarEnabled = false
	settings.HomeAssistantEnabled = false
	settings.ProjectDocsEnabled = false
	settings.ConversationsEnabled = true
	must(t, db.UpsertAssistantSettings(ctx, settings))

	var providerCalls int
	var sentMessages []assistantLLMMessage
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		var body struct {
			Messages []assistantLLMMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		sentMessages = body.Messages
		writeJSON(w, http.StatusOK, map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "Conversation memory answer."}},
			},
		})
	}))
	defer provider.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: "openai", OpenAIBaseURL: provider.URL, OpenAIAPIKey: "api-key", OpenAIChatModel: "gpt-test"})
	auth := authContext{User: user}
	membership := domain.HomeMembership{HomeID: home.ID, UserID: user.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}

	if _, err := server.processAssistantMessage(ctx, home, membership, auth, session, "Remember that the blue cabinet has spare fuses.", "test-device", "UTC"); err != nil {
		t.Fatalf("process first assistant message: %v", err)
	}
	stats, err := db.AssistantIndexStats(ctx, home.ID, user.ID)
	if err != nil {
		t.Fatalf("AssistantIndexStats: %v", err)
	}
	if stats.ConversationCount != 1 {
		t.Fatalf("conversation count = %d, want 1; stats=%#v", stats.ConversationCount, stats)
	}

	session, err = db.GetAssistantSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetAssistantSession: %v", err)
	}
	if _, err := server.processAssistantMessage(ctx, home, membership, auth, session, "Where are the spare fuses?", "test-device", "UTC"); err != nil {
		t.Fatalf("process second assistant message: %v", err)
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want 1", providerCalls)
	}
	if len(sentMessages) != 2 || !strings.Contains(sentMessages[1].Content, "[assistant_conversation]") || !strings.Contains(sentMessages[1].Content, "blue cabinet has spare fuses") {
		t.Fatalf("conversation memory was not sent as context: %#v", sentMessages)
	}

	settings.ConversationsEnabled = false
	must(t, db.UpsertAssistantSettings(ctx, settings))
	session, err = db.GetAssistantSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetAssistantSession disabled: %v", err)
	}
	if _, err := server.processAssistantMessage(ctx, home, membership, auth, session, "Where are the spare fuses now?", "test-device", "UTC"); err != nil {
		t.Fatalf("process disabled assistant message: %v", err)
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls after disabling conversations = %d, want 1", providerCalls)
	}
}
