package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSelectCases(t *testing.T) {
	all := []evalCase{
		{Name: "status", Group: "provider"},
		{Name: "docs", Group: "project_docs"},
		{Name: "safety", Group: "safety"},
	}

	got := selectCases(all, "provider,safety")
	if len(got) != 2 || got[0].Group != "provider" || got[1].Group != "safety" {
		t.Fatalf("selectCases returned %#v", got)
	}
}

func TestAssertRunRequiresConfirmation(t *testing.T) {
	run := assistantRunResponse{
		ID:                   "arun_1",
		State:                "waiting_confirmation",
		RequiresConfirmation: true,
		PendingActionSummary: &pendingActionSummary{Kind: "calendar_delete", Destructive: true},
		Diagnostics:          &assistantDiagnostics{ToolKind: "calendar.delete_event", IntentKind: "calendar.delete_event"},
	}

	if err := assertRun(run, evalExpect{
		ToolKind:             "calendar.delete_event",
		IntentKind:           "calendar.delete_event",
		RequiresConfirmation: boolPtr(true),
		PendingKind:          "calendar_delete",
		Destructive:          boolPtr(true),
	}); err != nil {
		t.Fatalf("assertRun returned error: %v", err)
	}
}

func TestAssertProviderStatus(t *testing.T) {
	status := assistantStatus{
		Provider:            "ollama",
		ChatConfigured:      true,
		EmbeddingConfigured: true,
		ChatModel:           "llama3.1",
		EmbeddingModel:      "nomic-embed-text",
		VectorStore:         "postgres",
		Index: assistantIndexStats{
			VectorAvailable: true,
			VectorMode:      "pgvector",
		},
	}
	settings := assistantSettingsResponse{
		Settings: assistantSettingsPayload{OllamaBaseURL: "http://192.168.86.158:11434"},
	}

	if err := assertProviderStatus(status, settings, "ollama", "", "http://192.168.86.158:11434"); err != nil {
		t.Fatalf("assertProviderStatus returned error: %v", err)
	}
	if err := assertProviderStatus(status, settings, "openai", "", ""); err == nil {
		t.Fatal("assertProviderStatus accepted wrong expected provider")
	}
}

func TestRedactSensitiveText(t *testing.T) {
	input := "Authorization: Bearer abc123 session_token=secret password=hunter2"
	got := redactSensitiveText(input)
	for _, leak := range []string{"abc123", "secret", "hunter2"} {
		if strings.Contains(got, leak) {
			t.Fatalf("redacted text leaked %q: %s", leak, got)
		}
	}
}

func TestTextPreviewIsRedactedAndBounded(t *testing.T) {
	got := textPreview("password=hunter2 " + strings.Repeat("x", 300))
	if strings.Contains(got, "hunter2") {
		t.Fatalf("preview leaked password: %s", got)
	}
	if len(got) > 180 {
		t.Fatalf("preview length=%d want <= 180", len(got))
	}
}

func TestCaseResultFromRunIncludesDiagnostics(t *testing.T) {
	run := assistantRunResponse{
		AssistantMessage: &assistantMessage{
			Text:  "Found a note.",
			Cards: []assistantResultCard{{Kind: "note", Title: "Work"}},
		},
		Diagnostics: &assistantDiagnostics{ToolKind: "notes.search", IntentKind: "notes.search", Query: "work"},
	}

	got := resultFromRun(evalCase{Name: "note search", Group: "notes", Prompt: "find work note"}, run, time.Second, nil)
	if got.Status != "pass" || got.ToolKind != "notes.search" || len(got.CardKinds) != 1 || got.CardKinds[0] != "note" {
		t.Fatalf("resultFromRun = %#v", got)
	}
}

func TestDefaultEvalCasesHaveNamesGroupsAndExpectations(t *testing.T) {
	cases := defaultEvalCases()
	if len(cases) < 8 {
		t.Fatalf("defaultEvalCases count=%d want at least 8", len(cases))
	}
	seen := map[string]bool{}
	for _, item := range cases {
		if item.Name == "" || item.Group == "" {
			t.Fatalf("case missing name/group: %#v", item)
		}
		if seen[item.Name] {
			t.Fatalf("duplicate case name %q", item.Name)
		}
		seen[item.Name] = true
		if (item.Name == "notes search" || item.Name == "files tax folder search" || item.Name == "multi source read only") && item.Prepare == nil {
			t.Fatalf("%s case has no fixture prepare step", item.Name)
		}
	}
}

func TestCalendarFixturePayloadUsesFutureDentistEvent(t *testing.T) {
	payload := calendarFixturePayload(time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC))
	entries := payload["entries"].([]map[string]any)
	if len(entries) != 1 || entries[0]["title"] != "Dentist Appointment" {
		t.Fatalf("calendar fixture payload = %#v", payload)
	}
	if !strings.Contains(entries[0]["starts_at"].(string), "2026-07-12") {
		t.Fatalf("fixture starts_at = %#v", entries[0]["starts_at"])
	}
}

func TestPutProfileNoteUsesAuthenticatedProfileNotesAPI(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method=%s want PUT", r.Method)
		}
		if r.URL.Path != "/v1/me/notes/hankai-eval-smb.md" {
			t.Fatalf("path=%s want profile note route", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization header=%q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["note_id"] != "hankai-eval-smb.md" || body["title"] != "SMB Fixture" || !strings.Contains(body["body_markdown"], "SMB access") {
			t.Fatalf("body = %#v", body)
		}
		if body["body_format"] != "markdown" || body["page_type"] != "text" {
			t.Fatalf("body metadata = %#v", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &liveClient{baseURL: baseURL, token: "test-token", http: server.Client()}
	if err := client.putProfileNote(context.Background(), "hankai-eval-smb.md", "SMB Fixture", "SMB access stays local."); err != nil {
		t.Fatalf("putProfileNote returned error: %v", err)
	}
}

func TestUploadFileFixtureUsesLocalFileTransfer(t *testing.T) {
	t.Parallel()

	var setupAuthorized bool
	var uploadAuthorized bool
	var uploadedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/home/files/uploads":
			if r.Method != http.MethodPost {
				t.Fatalf("setup method=%s want POST", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("setup authorization=%q", got)
			}
			setupAuthorized = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode setup body: %v", err)
			}
			if body["source_id"] != "local" || body["path"] != "Documents/Taxes/2025 Taxes/eval.txt" {
				t.Fatalf("setup body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"url":"/v1/file-transfers/tfr_eval","transfer_token":"transfer-token"}`))
		case "/v1/file-transfers/tfr_eval":
			if r.Method != http.MethodPut {
				t.Fatalf("upload method=%s want PUT", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer transfer-token" {
				t.Fatalf("upload authorization=%q", got)
			}
			uploadAuthorized = true
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read upload body: %v", err)
			}
			uploadedBody = string(data)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &liveClient{baseURL: baseURL, token: "test-token", http: server.Client()}
	if err := client.uploadFileFixture(context.Background(), "local", "Documents/Taxes/2025 Taxes/eval.txt", "2025 tax fixture"); err != nil {
		t.Fatalf("uploadFileFixture returned error: %v", err)
	}
	if !setupAuthorized || !uploadAuthorized {
		t.Fatalf("setupAuthorized=%t uploadAuthorized=%t", setupAuthorized, uploadAuthorized)
	}
	if uploadedBody != "2025 tax fixture" {
		t.Fatalf("uploaded body = %q", uploadedBody)
	}
}
