package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/agent/apps"
	"github.com/dropfile/hankremote/internal/protocol"
)

func TestHermesAppRunChatSuccess(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotSessionKey string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotSessionKey = r.Header.Get("X-Hermes-Session-Key")
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_123",
			"model":"hermes-agent",
			"output_text":"Hermes answer"
		}`))
	}))
	defer server.Close()

	code, stdout, stderr := runHermesApp(t, server.Client(), apps.AppStdioRequest{
		ProtocolVersion: "hank.app.stdio.v1",
		RequestID:       "req_123",
		AppID:           "hermes",
		CommandID:       "chat",
		Config:          json.RawMessage(`{"api_base_url":` + quote(server.URL) + `,"model":"hermes-agent","timeout_seconds":2}`),
		Secrets:         json.RawMessage(`{"api_key":"secret-token"}`),
		Input:           json.RawMessage(`{"prompt":"hello hermes","conversation_id":"conv_123","session_key":"sess_123"}`),
	})
	if code != 0 {
		t.Fatalf("run code = %d stderr=%s stdout=%s", code, stderr, stdout)
	}
	response := decodeStdioResponse(t, stdout)
	if !response.OK {
		t.Fatalf("response = %#v stderr=%s", response, stderr)
	}
	var output protocol.HermesChatResponse
	if err := json.Unmarshal(response.Output, &output); err != nil {
		t.Fatalf("Decode output: %v", err)
	}
	if output.Text != "Hermes answer" || output.Model != "hermes-agent" || output.ResponseID != "resp_123" || output.ConversationID != "conv_123" {
		t.Fatalf("output = %#v", output)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotSessionKey != "sess_123" {
		t.Fatalf("X-Hermes-Session-Key = %q", gotSessionKey)
	}
	if gotBody["input"] != "hello hermes" || gotBody["conversation"] != "conv_123" || gotBody["model"] != "hermes-agent" {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestHermesAppRunRejectsEmptyPrompt(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runHermesApp(t, http.DefaultClient, apps.AppStdioRequest{
		RequestID: "req_empty",
		AppID:     "hermes",
		CommandID: "chat",
		Config:    json.RawMessage(`{"api_base_url":"http://127.0.0.1:1","model":"hermes-agent","timeout_seconds":2}`),
		Secrets:   json.RawMessage(`{"api_key":"secret-token"}`),
		Input:     json.RawMessage(`{"prompt":"   "}`),
	})
	if code == 0 {
		t.Fatalf("run code = %d, want failure stdout=%s", code, stdout)
	}
	response := decodeStdioResponse(t, stdout)
	if response.OK || response.Error == nil || response.Error.Code != "invalid_request" {
		t.Fatalf("response = %#v", response)
	}
	if strings.Contains(stderr, "secret-token") {
		t.Fatalf("stderr leaked secret: %s", stderr)
	}
}

func TestHermesAppRunReturnsUpstreamError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	code, stdout, stderr := runHermesApp(t, server.Client(), apps.AppStdioRequest{
		RequestID: "req_upstream",
		AppID:     "hermes",
		CommandID: "chat",
		Config:    json.RawMessage(`{"api_base_url":` + quote(server.URL) + `,"model":"hermes-agent","timeout_seconds":2}`),
		Secrets:   json.RawMessage(`{"api_key":"secret-token"}`),
		Input:     json.RawMessage(`{"prompt":"hello"}`),
	})
	if code == 0 {
		t.Fatalf("run code = %d, want failure stdout=%s", code, stdout)
	}
	response := decodeStdioResponse(t, stdout)
	if response.OK || response.Error == nil || response.Error.Code != "upstream_error" {
		t.Fatalf("response = %#v", response)
	}
	if strings.Contains(stderr, "secret-token") {
		t.Fatalf("stderr leaked secret: %s", stderr)
	}
}

func runHermesApp(t *testing.T, client *http.Client, request apps.AppStdioRequest) (int, string, string) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code := run(ctx, bytes.NewReader(append(raw, '\n')), &stdout, &stderr, client)
	return code, stdout.String(), stderr.String()
}

func decodeStdioResponse(t *testing.T, raw string) apps.AppStdioResponse {
	t.Helper()
	var response apps.AppStdioResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatalf("Decode response %q: %v", raw, err)
	}
	return response
}

func quote(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
