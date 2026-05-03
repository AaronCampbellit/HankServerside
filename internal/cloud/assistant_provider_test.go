package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

func TestAssistantProviderUsesConfiguredOllama(t *testing.T) {
	t.Parallel()

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			writeJSON(w, http.StatusOK, map[string]any{
				"message": map[string]any{"content": "Grounded answer from Ollama."},
			})
		case "/api/embeddings":
			writeJSON(w, http.StatusOK, map[string]any{
				"embedding": []float64{1, 2, 3, 4},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	db := storeForTest(t)
	defer db.Close()
	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{
		Provider:             "ollama",
		OllamaBaseURL:        provider.URL,
		OllamaChatModel:      "test-chat",
		OllamaEmbeddingModel: "test-embed",
		EmbeddingDimension:   4,
	})

	answer, model, err := server.generateAssistantLLMResponse(context.Background(), "usr_provider", []assistantLLMMessage{
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("generateAssistantLLMResponse: %v", err)
	}
	if answer != "Grounded answer from Ollama." {
		t.Fatalf("answer = %q", answer)
	}
	if model != "ollama:test-chat" {
		t.Fatalf("model = %q", model)
	}

	embedding, modelName, version := server.embedAssistantText(context.Background(), "usr_provider", "hello")
	if len(embedding) != 4 {
		t.Fatalf("embedding len = %d, want 4", len(embedding))
	}
	if modelName != "test-embed" || version != "ollama" {
		t.Fatalf("embedding metadata = %q/%q", modelName, version)
	}
}

func TestAssistantStatusUsesLinkedChatGPTCodexWhenConfigured(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_assistant_status", Email: "assistant-status@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_assistant_status", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AppSession{ID: "sess_assistant_status", UserID: user.ID, TokenHash: hashToken("assistant-status-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, session))
	must(t, db.UpsertOpenAIAccount(ctx, domain.OpenAIAccount{
		UserID:          user.ID,
		ProviderUserID:  "workspace-123",
		AuthProvider:    openAIAccountProviderChatGPTCodex,
		ChatGPTPlanType: "plus",
		AccessToken:     "linked-token",
		TokenType:       "Bearer",
		Scope:           "chat",
		CreatedAt:       now,
		UpdatedAt:       now,
	}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: assistantProviderChatGPTCodex, ChatGPTOAuthEnabled: true, ChatGPTChatModel: "gpt-codex-test", OpenAIEmbeddingModel: "embed-test"})
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var status map[string]any
	requestJSON(t, testServer, "assistant-status-token", http.MethodGet, "/v1/home/assistant/status", nil, &status)
	if status["provider"] != assistantProviderChatGPTCodex {
		t.Fatalf("provider = %#v, want %s", status["provider"], assistantProviderChatGPTCodex)
	}
	if status["chat_configured"] != true {
		encoded, _ := json.Marshal(status)
		t.Fatalf("chat_configured = false, status=%s", encoded)
	}
	if status["chat_model"] != "gpt-codex-test" {
		t.Fatalf("chat_model = %#v", status["chat_model"])
	}
	if status["embedding_configured"] != false {
		t.Fatalf("embedding_configured = %#v, want false", status["embedding_configured"])
	}
}

func TestChatGPTCodexProviderPostsResponsesWithBearerAndAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_chatgpt_provider", Email: "chatgpt-provider@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.UpsertOpenAIAccount(ctx, domain.OpenAIAccount{
		UserID:          user.ID,
		ProviderUserID:  "workspace-123",
		AuthProvider:    openAIAccountProviderChatGPTCodex,
		ChatGPTPlanType: "team",
		AccessToken:     "linked-token",
		RefreshToken:    "refresh-token",
		TokenType:       "Bearer",
		Scope:           "chatgpt_codex",
		CreatedAt:       now,
		UpdatedAt:       now,
	}))

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer linked-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("ChatGPT-Account-ID"); got != "workspace-123" {
			t.Fatalf("ChatGPT-Account-ID = %q", got)
		}
		var body struct {
			Model string                `json:"model"`
			Input []assistantLLMMessage `json:"input"`
			Store bool                  `json:"store"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "gpt-codex-test" {
			t.Fatalf("model = %q", body.Model)
		}
		if len(body.Input) != 1 || body.Input[0].Content != "hello" {
			t.Fatalf("input = %#v", body.Input)
		}
		if body.Store {
			t.Fatal("store = true, want false")
		}
		writeJSON(w, http.StatusOK, map[string]any{"output_text": "Codex-backed answer."})
	}))
	defer provider.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: assistantProviderChatGPTCodex, ChatGPTOAuthEnabled: true, ChatGPTBackendBaseURL: provider.URL, ChatGPTChatModel: "gpt-codex-test"})

	answer, model, err := server.generateAssistantLLMResponse(ctx, user.ID, []assistantLLMMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("generateAssistantLLMResponse: %v", err)
	}
	if answer != "Codex-backed answer." {
		t.Fatalf("answer = %q", answer)
	}
	if model != "chatgpt_codex:gpt-codex-test" {
		t.Fatalf("model = %q", model)
	}
}

func TestOpenAIProviderDoesNotUseChatGPTOAuthToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_openai_separate", Email: "openai-separate@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.UpsertOpenAIAccount(ctx, domain.OpenAIAccount{
		UserID:         user.ID,
		ProviderUserID: "workspace-123",
		AuthProvider:   openAIAccountProviderChatGPTCodex,
		AccessToken:    "chatgpt-token",
		TokenType:      "Bearer",
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: "openai", OpenAIChatModel: "gpt-test"})

	_, _, err := server.generateAssistantLLMResponse(ctx, user.ID, []assistantLLMMessage{{Role: "user", Content: "hello"}})
	if err == nil || err.Error() != "OpenAI is not configured" {
		t.Fatalf("err = %v, want OpenAI is not configured", err)
	}
}

func TestAssistantProviderUsesOpenAIAPIKeyForChatAndEmbeddings(t *testing.T) {
	t.Parallel()

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
			t.Fatalf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/v1/chat/completions":
			writeJSON(w, http.StatusOK, map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": "OpenAI API answer."}},
				},
			})
		case "/v1/embeddings":
			writeJSON(w, http.StatusOK, map[string]any{
				"data": []map[string]any{
					{"embedding": []float64{0.1, 0.2, 0.3, 0.4}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	db := storeForTest(t)
	defer db.Close()
	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{
		Provider:             "openai",
		OpenAIBaseURL:        provider.URL,
		OpenAIAPIKey:         "api-key",
		OpenAIChatModel:      "gpt-api-test",
		OpenAIEmbeddingModel: "embed-api-test",
		EmbeddingDimension:   4,
	})

	answer, model, err := server.generateAssistantLLMResponse(context.Background(), "usr_openai_api", []assistantLLMMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("generateAssistantLLMResponse: %v", err)
	}
	if answer != "OpenAI API answer." || model != "openai:gpt-api-test" {
		t.Fatalf("answer/model = %q/%q", answer, model)
	}
	embedding, embeddingModel, version := server.embedAssistantText(context.Background(), "usr_openai_api", "hello")
	if len(embedding) != 4 || embeddingModel != "embed-api-test" || version != "openai" {
		t.Fatalf("embedding metadata = len %d %q/%q", len(embedding), embeddingModel, version)
	}
}

func TestAssistantAutoFallsBackLocalWithoutProviders(t *testing.T) {
	t.Parallel()

	db := storeForTest(t)
	defer db.Close()
	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: "auto", EmbeddingDimension: 4})

	status := server.assistantStatus(context.Background(), "usr_auto_local")
	if status.Provider != "local" || status.ChatConfigured || status.EmbeddingConfigured {
		t.Fatalf("status = %#v", status)
	}
	embedding, model, version := server.embedAssistantText(context.Background(), "usr_auto_local", "hello")
	if len(embedding) != 4 || model != "local-hash" || version != "v1" {
		t.Fatalf("embedding = len %d %q/%q", len(embedding), model, version)
	}
}

func TestChatGPTCodexRefreshesExpiredTokenBeforeChat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	user := domain.User{ID: "usr_chatgpt_refresh", Email: "chatgpt-refresh@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.UpsertOpenAIAccount(ctx, domain.OpenAIAccount{
		UserID:         user.ID,
		ProviderUserID: "workspace-123",
		AuthProvider:   openAIAccountProviderChatGPTCodex,
		AccessToken:    "expired-token",
		RefreshToken:   "refresh-token",
		TokenType:      "Bearer",
		Scope:          "chatgpt_codex",
		ExpiresAt:      &expired,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	refreshCalls := 0
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		refreshCalls++
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "refresh-token" {
			t.Fatalf("refresh body = %#v", body)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token":  "fresh-token",
			"refresh_token": "new-refresh-token",
			"id_token":      fakeChatGPTIDToken(t, "workspace-123", "plus", now.Add(time.Hour)),
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer authServer.Close()

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fresh-token" {
			t.Fatalf("Authorization = %q", got)
		}
		writeJSON(w, http.StatusOK, map[string]any{"output_text": "Refreshed answer."})
	}))
	defer provider.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: assistantProviderChatGPTCodex, ChatGPTOAuthEnabled: true, ChatGPTAuthIssuer: authServer.URL, ChatGPTBackendBaseURL: provider.URL, ChatGPTChatModel: "gpt-codex-test"})

	answer, _, err := server.generateAssistantLLMResponse(ctx, user.ID, []assistantLLMMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("generateAssistantLLMResponse: %v", err)
	}
	if answer != "Refreshed answer." {
		t.Fatalf("answer = %q", answer)
	}
	if refreshCalls != 1 {
		t.Fatalf("refreshCalls = %d, want 1", refreshCalls)
	}
	account, err := db.GetOpenAIAccount(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if account.AccessToken != "fresh-token" || account.RefreshToken != "new-refresh-token" || account.ChatGPTPlanType != "plus" {
		t.Fatalf("refreshed account = %#v", account)
	}
}

func TestChatGPTCodexRefreshFailureDeletesLink(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	user := domain.User{ID: "usr_chatgpt_refresh_fail", Email: "chatgpt-refresh-fail@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.UpsertOpenAIAccount(ctx, domain.OpenAIAccount{
		UserID:         user.ID,
		ProviderUserID: "workspace-123",
		AuthProvider:   openAIAccountProviderChatGPTCodex,
		AccessToken:    "expired-token",
		RefreshToken:   "refresh-token",
		TokenType:      "Bearer",
		Scope:          "chatgpt_codex",
		ExpiresAt:      &expired,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"code": "refresh_token_expired"}})
	}))
	defer authServer.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: assistantProviderChatGPTCodex, ChatGPTOAuthEnabled: true, ChatGPTAuthIssuer: authServer.URL, ChatGPTBackendBaseURL: "https://chatgpt.invalid/backend-api/codex", ChatGPTChatModel: "gpt-codex-test"})

	_, _, err := server.generateAssistantLLMResponse(ctx, user.ID, []assistantLLMMessage{{Role: "user", Content: "hello"}})
	if !errors.Is(err, errChatGPTRelinkRequired) {
		t.Fatalf("err = %v, want errChatGPTRelinkRequired", err)
	}
	if _, err := db.GetOpenAIAccount(ctx, user.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetOpenAIAccount err = %v, want ErrNotFound", err)
	}
}
