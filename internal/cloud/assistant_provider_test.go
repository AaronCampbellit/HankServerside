package cloud

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
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

func TestAssistantStatusUsesLinkedOpenAIWhenConfigured(t *testing.T) {
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
		UserID:      user.ID,
		AccessToken: "linked-token",
		TokenType:   "Bearer",
		Scope:       "chat",
		CreatedAt:   now,
		UpdatedAt:   now,
	}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureAssistantAI(AssistantAIConfig{Provider: "openai", OpenAIChatModel: "gpt-test", OpenAIEmbeddingModel: "embed-test"})
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var status map[string]any
	requestJSON(t, testServer, "assistant-status-token", http.MethodGet, "/v1/home/assistant/status", nil, &status)
	if status["provider"] != "openai" {
		t.Fatalf("provider = %#v, want openai", status["provider"])
	}
	if status["chat_configured"] != true {
		encoded, _ := json.Marshal(status)
		t.Fatalf("chat_configured = false, status=%s", encoded)
	}
}
