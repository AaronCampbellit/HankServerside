package main

import (
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
