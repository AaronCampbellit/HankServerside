package hermes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/protocol"
)

func TestServiceChatPostsResponsesRequest(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotPath string
	var gotSessionKey string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotSessionKey = r.Header.Get("X-Hermes-Session-Key")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_test",
			"model":"hermes-agent",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hermes answer."}]}]
		}`))
	}))
	defer server.Close()

	service := New(Config{
		BaseURL: server.URL,
		APIKey:  "secret",
		Timeout: time.Second,
	})
	response, err := service.Chat(t.Context(), protocol.HermesChatRequest{
		Prompt:         "hello hermes",
		ConversationID: "hank:home:user:session",
		SessionKey:     "agent:main:hank:home:user",
	})
	if err != nil {
		t.Fatalf("Chat error = %v", err)
	}
	if response.Text != "Hermes answer." || response.ResponseID != "resp_test" || response.Model != "hermes-agent" {
		t.Fatalf("response = %#v", response)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want /v1/responses", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotSessionKey != "agent:main:hank:home:user" {
		t.Fatalf("X-Hermes-Session-Key = %q", gotSessionKey)
	}
	if gotBody["input"] != "hello hermes" || gotBody["conversation"] != "hank:home:user:session" || gotBody["model"] != defaultModel {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestServiceChatDisabledWithoutCredentials(t *testing.T) {
	t.Parallel()

	service := New(Config{})
	if service.Enabled() {
		t.Fatal("empty config should not enable Hermes")
	}
	if _, err := service.Chat(t.Context(), protocol.HermesChatRequest{Prompt: "hello"}); err != ErrDisabled {
		t.Fatalf("Chat error = %v, want ErrDisabled", err)
	}
}
