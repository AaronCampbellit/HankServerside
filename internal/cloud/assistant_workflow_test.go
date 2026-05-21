package cloud

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

func TestAssistantIntentClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt string
		want   assistantIntentKind
	}{
		{name: "note list", prompt: "what notes do I have", want: assistantIntentNotesList},
		{name: "note search", prompt: "find grocery list note", want: assistantIntentNotesSearch},
		{name: "note append", prompt: "add eggs to the grocery list", want: assistantIntentNotesAppend},
		{name: "note append work note", prompt: "add call patrick to the work note", want: assistantIntentNotesAppend},
		{name: "note append typo delimiter", prompt: "add fix hankai conversation output the the hank features note", want: assistantIntentNotesAppend},
		{name: "note append referenced link", prompt: "https://docs.gitlab.com/install/docker/installation/\nAdd this link to work note", want: assistantIntentNotesAppend},
		{name: "home assistant on", prompt: "what entities are on", want: assistantIntentHomeAssistantQuery},
		{name: "home assistant garage", prompt: "garage entities", want: assistantIntentHomeAssistantQuery},
		{name: "home assistant conversational query", prompt: "can you find all the garage light entities", want: assistantIntentHomeAssistantQuery},
		{name: "file search", prompt: "find 2025 taxes", want: assistantIntentFilesSearch},
		{name: "project docs", prompt: "what does AGENTS.md say", want: assistantIntentProjectDocs},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := classifyAssistantIntent(test.prompt)
			if got.Kind != test.want {
				t.Fatalf("classifyAssistantIntent(%q) = %s, want %s", test.prompt, got.Kind, test.want)
			}
		})
	}
}

func TestAssistantToolRegistryShape(t *testing.T) {
	t.Parallel()

	if len(assistantToolRegistry) == 0 {
		t.Fatal("assistant tool registry is empty")
	}
	seen := map[assistantIntentKind]bool{}
	for index, tool := range assistantToolRegistry {
		if tool.Kind == "" {
			t.Fatalf("tool %d has empty kind", index)
		}
		if seen[tool.Kind] {
			t.Fatalf("tool kind %s registered more than once", tool.Kind)
		}
		seen[tool.Kind] = true
		if tool.Match == nil {
			t.Fatalf("tool %s is missing matcher", tool.Kind)
		}
		if tool.Execute == nil {
			t.Fatalf("tool %s is missing executor", tool.Kind)
		}
	}
	last := assistantToolRegistry[len(assistantToolRegistry)-1]
	if last.Kind != assistantIntentGeneral {
		t.Fatalf("last registry tool = %s, want %s fallback", last.Kind, assistantIntentGeneral)
	}
}

func TestAssistantNoteRankingPrefersExactTitleThenTitleThenBody(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	results := rankScoredNotes([]domain.UserNote{
		{
			ID:        "note_body",
			NoteID:    "body",
			Title:     "Personal Reminders",
			Content:   "Call Patrick about work tomorrow.",
			PageType:  protocol.NotePageTypeText,
			UpdatedAt: now.Add(2 * time.Hour),
		},
		{
			ID:        "note_title",
			NoteID:    "title",
			Title:     "Work Projects",
			Content:   "Deployment notes.",
			PageType:  protocol.NotePageTypeText,
			UpdatedAt: now.Add(time.Hour),
		},
		{
			ID:        "note_exact",
			NoteID:    "exact",
			Title:     "Work",
			Content:   "Deployment notes.",
			PageType:  protocol.NotePageTypeText,
			UpdatedAt: now,
		},
	}, "work")

	got := []string{results[0].Note.Title, results[1].Note.Title, results[2].Note.Title}
	want := []string{"Work", "Work Projects", "Personal Reminders"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranked notes = %v, want %v", got, want)
		}
	}
}

func TestAssistantNotesSearchAppendAndReindex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_assistant_notes", Email: "assistant-notes@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	otherUser := domain.User{ID: "usr_assistant_notes_other", Email: "assistant-notes-other@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_assistant_notes", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	note := domain.UserNote{
		ID:           "note_assistant_grocery",
		NoteID:       "11111111-1111-1111-1111-111111111111",
		OwnerUserID:  user.ID,
		Title:        "Grocery List",
		Content:      "- milk",
		BodyMarkdown: "- milk",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now,
		UpdatedBy:    user.ID,
	}
	featuresNote := domain.UserNote{
		ID:           "note_assistant_features",
		NoteID:       "22222222-2222-2222-2222-222222222222",
		OwnerUserID:  user.ID,
		Title:        "Hank Features",
		Content:      "- existing feature",
		BodyMarkdown: "- existing feature",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now.Add(-time.Minute),
		UpdatedBy:    user.ID,
	}
	workNote := domain.UserNote{
		ID:           "note_assistant_work",
		NoteID:       "44444444-4444-4444-8444-444444444444",
		OwnerUserID:  user.ID,
		Title:        "Work",
		Content:      "- deployment notes",
		BodyMarkdown: "- deployment notes",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now.Add(-3 * time.Minute),
		UpdatedBy:    user.ID,
	}
	workProjectsNote := domain.UserNote{
		ID:           "note_assistant_work_projects",
		NoteID:       "55555555-5555-5555-8555-555555555555",
		OwnerUserID:  user.ID,
		Title:        "Work Projects",
		Content:      "- planning",
		BodyMarkdown: "- planning",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now.Add(5 * time.Minute),
		UpdatedBy:    user.ID,
	}
	workBodyNote := domain.UserNote{
		ID:           "note_assistant_work_body",
		NoteID:       "66666666-6666-6666-8666-666666666666",
		OwnerUserID:  user.ID,
		Title:        "Personal Reminders",
		Content:      "- work follow up",
		BodyMarkdown: "- work follow up",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now.Add(6 * time.Minute),
		UpdatedBy:    user.ID,
	}
	sharedNote := domain.UserNote{
		ID:           "note_assistant_shared",
		NoteID:       "33333333-3333-3333-3333-333333333333",
		OwnerUserID:  otherUser.ID,
		HomeID:       home.ID,
		Title:        "Shared Checklist",
		Content:      "- shared item",
		BodyMarkdown: "- shared item",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now.Add(-2 * time.Minute),
		UpdatedBy:    otherUser.ID,
	}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateUser(ctx, otherUser))
	must(t, db.CreateHome(ctx, home))
	must(t, db.UpsertUserNote(ctx, note))
	must(t, db.UpsertUserNote(ctx, featuresNote))
	must(t, db.UpsertUserNote(ctx, workNote))
	must(t, db.UpsertUserNote(ctx, workProjectsNote))
	must(t, db.UpsertUserNote(ctx, workBodyNote))
	must(t, db.UpsertUserNote(ctx, sharedNote))
	must(t, db.AddNoteShare(ctx, domain.NoteShare{
		NoteID:       sharedNote.ID,
		HomeID:       home.ID,
		TargetUserID: user.ID,
		SharedBy:     otherUser.ID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	settings := defaultAssistantSettings(home.ID, user.ID)
	settings.ProjectDocsEnabled = false
	auth := authContext{User: user}
	membership := domain.HomeMembership{HomeID: home.ID, UserID: user.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}

	listed, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, "what notes do I have")
	if err != nil {
		t.Fatalf("generateAssistantResponse list: %v", err)
	}
	for _, want := range []string{"Grocery List", "Hank Features", "Work", "Work Projects", "Personal Reminders", "Shared Checklist"} {
		if !strings.Contains(listed.Text, want) {
			t.Fatalf("note list missing %q: %s", want, listed.Text)
		}
	}
	if len(listed.Cards) != 6 {
		t.Fatalf("note list cards = %#v, want one card per listed note", listed.Cards)
	}
	for _, want := range []string{"Grocery List", "Hank Features", "Work", "Work Projects", "Personal Reminders", "Shared Checklist"} {
		if !assistantCardsContainTitle(listed.Cards, want) {
			t.Fatalf("note list cards missing %q: %#v", want, listed.Cards)
		}
	}

	found, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, "find grocery list note")
	if err != nil {
		t.Fatalf("generateAssistantResponse search: %v", err)
	}
	if len(found.Cards) != 1 || found.Cards[0].Kind != "note" || found.Cards[0].Title != "Grocery List" {
		t.Fatalf("note search cards = %#v", found.Cards)
	}

	appended, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, "add eggs to the grocery list")
	if err != nil {
		t.Fatalf("generateAssistantResponse append: %v", err)
	}
	if !strings.Contains(appended.Text, "Added `eggs`") {
		t.Fatalf("append text = %q", appended.Text)
	}
	updated, err := db.GetUserNoteByID(ctx, note.ID)
	if err != nil {
		t.Fatalf("GetUserNoteByID: %v", err)
	}
	if !strings.Contains(updated.Content, "- eggs") || !strings.Contains(updated.BodyMarkdown, "- eggs") {
		t.Fatalf("updated note did not include appended item: %#v", updated)
	}
	results, err := db.SearchAssistantContext(ctx, home.ID, user.ID, "eggs grocery", nil, 5)
	if err != nil {
		t.Fatalf("SearchAssistantContext: %v", err)
	}
	if len(results) == 0 || results[0].SourceType != "profile_note" || results[0].Title != "Grocery List" {
		t.Fatalf("reindexed note search results = %#v", results)
	}

	appendedTypo, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, "add fix hankai conversation output the the hank features note")
	if err != nil {
		t.Fatalf("generateAssistantResponse append typo: %v", err)
	}
	if !strings.Contains(appendedTypo.Text, "Added `fix hankai conversation output`") {
		t.Fatalf("append typo text = %q", appendedTypo.Text)
	}
	updatedFeatures, err := db.GetUserNoteByID(ctx, featuresNote.ID)
	if err != nil {
		t.Fatalf("GetUserNoteByID features: %v", err)
	}
	if !strings.Contains(updatedFeatures.Content, "- fix hankai conversation output") {
		t.Fatalf("features note did not include appended item: %#v", updatedFeatures)
	}

	linkPrompt := "https://docs.gitlab.com/install/docker/installation/\nAdd this link to work note"
	appendedLink, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, linkPrompt)
	if err != nil {
		t.Fatalf("generateAssistantResponse append link: %v", err)
	}
	if !strings.Contains(appendedLink.Text, "Added `https://docs.gitlab.com/install/docker/installation/`") {
		t.Fatalf("append link text = %q", appendedLink.Text)
	}
	updatedWork, err := db.GetUserNoteByID(ctx, workNote.ID)
	if err != nil {
		t.Fatalf("GetUserNoteByID work: %v", err)
	}
	if !strings.Contains(updatedWork.Content, "- https://docs.gitlab.com/install/docker/installation/") ||
		strings.Contains(updatedWork.Content, "- this link") {
		t.Fatalf("work note did not include appended URL: %#v", updatedWork)
	}

	appendedCall, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, "add call patrick to the work note")
	if err != nil {
		t.Fatalf("generateAssistantResponse append call: %v", err)
	}
	if !strings.Contains(appendedCall.Text, "Added `call patrick` to `Work`") {
		t.Fatalf("append call text = %q", appendedCall.Text)
	}
	updatedWork, err = db.GetUserNoteByID(ctx, workNote.ID)
	if err != nil {
		t.Fatalf("GetUserNoteByID work after call: %v", err)
	}
	if !strings.Contains(updatedWork.Content, "- call patrick") {
		t.Fatalf("work note did not include appended call item: %#v", updatedWork)
	}
	updatedWorkProjects, err := db.GetUserNoteByID(ctx, workProjectsNote.ID)
	if err != nil {
		t.Fatalf("GetUserNoteByID work projects: %v", err)
	}
	if strings.Contains(updatedWorkProjects.Content, "- call patrick") {
		t.Fatalf("work projects note received exact-title append: %#v", updatedWorkProjects)
	}
	updatedWorkBody, err := db.GetUserNoteByID(ctx, workBodyNote.ID)
	if err != nil {
		t.Fatalf("GetUserNoteByID work body: %v", err)
	}
	if strings.Contains(updatedWorkBody.Content, "- call patrick") {
		t.Fatalf("body-only work note received exact-title append: %#v", updatedWorkBody)
	}
}

func TestAssistantConfirmationResponseIncludesStructuredSummary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_assistant_confirm", Email: "assistant-confirm@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_assistant_confirm", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AssistantSession{
		ID:            "asess_confirm",
		HomeID:        home.ID,
		UserID:        user.ID,
		Title:         "Calendar confirmation",
		LastMessageAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateAssistantSession(ctx, session))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	auth := authContext{User: user}
	membership := domain.HomeMembership{HomeID: home.ID, UserID: user.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}

	response, err := server.processAssistantMessage(ctx, home, membership, auth, session, "create dentist appointment on May 3", "device-confirm", "UTC")
	if err != nil {
		t.Fatalf("processAssistantMessage: %v", err)
	}
	if !response.RequiresConfirmation {
		t.Fatalf("RequiresConfirmation = false, response=%#v", response)
	}
	if response.PendingActionSummary == nil {
		t.Fatalf("missing pending action summary: %#v", response)
	}
	if response.PendingActionSummary.Kind != "calendar_create" || response.PendingActionSummary.Title != "Create calendar event" {
		t.Fatalf("unexpected pending action summary: %#v", response.PendingActionSummary)
	}
	if !assistantSummaryDetailsContain(response.PendingActionSummary.Details, "Event", "dentist appointment") ||
		!assistantSummaryDetailsContain(response.PendingActionSummary.Details, "Requested date", "May 3") ||
		!assistantSummaryDetailsContain(response.PendingActionSummary.Details, "All day", "Yes") {
		t.Fatalf("summary details missing expected values: %#v", response.PendingActionSummary.Details)
	}

	run, err := db.GetAssistantRun(ctx, response.ID)
	if err != nil {
		t.Fatalf("GetAssistantRun: %v", err)
	}
	fetched := server.assistantRunResponseForSession(ctx, session, run)
	if fetched.PendingActionSummary == nil || fetched.PendingActionSummary.Kind != "calendar_create" {
		t.Fatalf("fetched run missing pending action summary: %#v", fetched)
	}
}

func TestAssistantAttachmentClarificationReusesStagedMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_attachment_followup", Email: "attachment-followup@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_attachment_followup", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AssistantSession{ID: "asess_attachment_followup", HomeID: home.ID, UserID: user.ID, Title: "Uploads", LastMessageAt: now, CreatedAt: now, UpdatedAt: now}
	note := domain.UserNote{
		ID:           "note_attachment_project_ideas",
		NoteID:       "11111111-3333-4333-8333-111111111111",
		OwnerUserID:  user.ID,
		Title:        "Project Ideas",
		Content:      "Ideas",
		BodyMarkdown: "Ideas",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now,
		UpdatedBy:    user.ID,
	}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateAssistantSession(ctx, session))
	must(t, db.UpsertUserNote(ctx, note))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	auth := authContext{User: user}
	membership := domain.HomeMembership{HomeID: home.ID, UserID: user.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}
	attachments := []assistantMessageAttachment{{
		ClientAttachmentID: "client_attachment_project",
		Filename:           "idea.png",
		ContentType:        "image/png",
		SizeBytes:          12,
		ChecksumSHA256:     "abc123",
		Kind:               "image",
	}}

	first, err := server.processAssistantMessageWithAttachments(ctx, home, membership, auth, session, "uploaded this", attachments, "device", "UTC")
	if err != nil {
		t.Fatalf("process first attachment message: %v", err)
	}
	if first.RequiresConfirmation {
		t.Fatalf("first response unexpectedly requires confirmation: %#v", first)
	}
	if first.AssistantMessage == nil || !strings.Contains(first.AssistantMessage.Text, "Where should I store") {
		t.Fatalf("first response did not ask for destination: %#v", first.AssistantMessage)
	}

	followup, err := server.processAssistantMessageWithAttachments(ctx, home, membership, auth, session, "Project Ideas note", nil, "device", "UTC")
	if err != nil {
		t.Fatalf("process follow-up message: %v", err)
	}
	if !followup.RequiresConfirmation || followup.PendingActionSummary == nil {
		t.Fatalf("follow-up did not reuse staged attachment for confirmation: %#v", followup)
	}
	if followup.PendingActionSummary.Kind != "attachment_commit" {
		t.Fatalf("pending kind = %q, want attachment_commit", followup.PendingActionSummary.Kind)
	}
}

func TestAssistantAttachmentToolErrorCompletesAndExpiresMissingStagedBytes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_attachment_error", Email: "attachment-error@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_attachment_error", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	appSession := domain.AppSession{ID: "sess_attachment_error", UserID: user.ID, TokenHash: hashToken("attachment-error-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	note := domain.UserNote{
		ID:           "note_attachment_error_project",
		NoteID:       "22222222-3333-4333-8333-111111111111",
		OwnerUserID:  user.ID,
		Title:        "Project Ideas",
		Content:      "Ideas",
		BodyMarkdown: "Ideas",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now,
		UpdatedBy:    user.ID,
	}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, appSession))
	must(t, db.UpsertUserNote(ctx, note))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var apiSession assistantAPISession
	requestJSON(t, testServer, "attachment-error-token", http.MethodPost, "/v1/home/assistant/sessions", nil, &apiSession)

	var initial assistantRunResponse
	requestJSON(t, testServer, "attachment-error-token", http.MethodPost, "/v1/home/assistant/sessions/"+apiSession.ID+"/messages", map[string]any{
		"content": "put this in Project Ideas note",
		"attachments": []map[string]any{
			{
				"client_attachment_id": "client_attachment_missing",
				"filename":             "missing.pdf",
				"content_type":         "application/pdf",
				"size_bytes":           24,
				"checksum_sha256":      "abc123",
				"kind":                 "document",
			},
		},
	}, &initial)
	if !initial.RequiresConfirmation {
		t.Fatalf("initial run did not request confirmation: %#v", initial)
	}

	var toolRun assistantRunResponse
	requestJSON(t, testServer, "attachment-error-token", http.MethodPost, "/v1/home/assistant/runs/"+initial.ID+"/confirm", map[string]any{"approved": true}, &toolRun)
	if !toolRun.RequiresClientTools || toolRun.ClientToolRequest == nil || toolRun.ClientToolRequest.ToolName != "attachments.commit" {
		t.Fatalf("confirmed run did not request attachment client tool: %#v", toolRun)
	}

	var completed assistantRunResponse
	requestJSON(t, testServer, "attachment-error-token", http.MethodPost, "/v1/home/assistant/runs/"+toolRun.ID+"/client-tool-results", map[string]any{
		"results": []map[string]any{
			{
				"tool_name": "attachments.commit",
				"error":     "The staged upload is no longer available on this device.",
				"result": map[string]any{
					"destination_kind":       "note_attachment",
					"attachment_ids":         []string{"client_attachment_missing"},
					"expired_attachment_ids": []string{"client_attachment_missing"},
					"error_code":             "missing_staged_attachment",
				},
			},
		},
	}, &completed)
	if completed.State != assistantStateCompleted || completed.AssistantMessage == nil {
		t.Fatalf("tool error did not complete run: %#v", completed)
	}
	if !strings.Contains(completed.AssistantMessage.Text, "could not store") {
		t.Fatalf("unexpected completion text: %#v", completed.AssistantMessage)
	}
	records, err := db.ListAssistantAttachments(ctx, apiSession.ID)
	if err != nil {
		t.Fatalf("ListAssistantAttachments: %v", err)
	}
	if len(records) != 1 || records[0].Status != "expired" {
		t.Fatalf("attachment status = %#v, want one expired record", records)
	}
}

func TestAssistantAttachmentNoteCommitCompletesAndMarksCommitted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_attachment_note_success", Email: "attachment-note-success@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_attachment_note_success", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	appSession := domain.AppSession{ID: "sess_attachment_note_success", UserID: user.ID, TokenHash: hashToken("attachment-note-success-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	note := domain.UserNote{
		ID:           "note_attachment_note_success",
		NoteID:       "33333333-3333-4333-8333-111111111111",
		OwnerUserID:  user.ID,
		Title:        "Project Ideas",
		Content:      "Ideas",
		BodyMarkdown: "Ideas",
		BodyFormat:   "markdown",
		PageType:     protocol.NotePageTypeText,
		Revision:     "rev_initial",
		Checksum:     "checksum_initial",
		CreatedAt:    now,
		UpdatedAt:    now,
		UpdatedBy:    user.ID,
	}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, appSession))
	must(t, db.UpsertUserNote(ctx, note))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var apiSession assistantAPISession
	requestJSON(t, testServer, "attachment-note-success-token", http.MethodPost, "/v1/home/assistant/sessions", nil, &apiSession)

	var initial assistantRunResponse
	requestJSON(t, testServer, "attachment-note-success-token", http.MethodPost, "/v1/home/assistant/sessions/"+apiSession.ID+"/messages", map[string]any{
		"content": "Add this photo to the Project Ideas note",
		"attachments": []map[string]any{
			{
				"client_attachment_id": "client_attachment_note_success",
				"filename":             "idea.png",
				"content_type":         "image/png",
				"size_bytes":           12,
				"checksum_sha256":      "abc123",
				"kind":                 "image",
			},
		},
	}, &initial)
	if !initial.RequiresConfirmation || initial.PendingActionSummary == nil {
		t.Fatalf("initial run did not request attachment confirmation: %#v", initial)
	}

	var toolRun assistantRunResponse
	requestJSON(t, testServer, "attachment-note-success-token", http.MethodPost, "/v1/home/assistant/runs/"+initial.ID+"/confirm", map[string]any{"approved": true}, &toolRun)
	if !toolRun.RequiresClientTools || toolRun.ClientToolRequest == nil || toolRun.ClientToolRequest.ToolName != "attachments.commit" {
		t.Fatalf("confirmed run did not request attachment client tool: %#v", toolRun)
	}

	var completed assistantRunResponse
	requestJSON(t, testServer, "attachment-note-success-token", http.MethodPost, "/v1/home/assistant/runs/"+toolRun.ID+"/client-tool-results", map[string]any{
		"tool_name": "attachments.commit",
		"result": map[string]any{
			"destination_kind": "note_attachment",
			"note_id":          note.NoteID,
			"note_scope":       "profile",
			"note_title":       note.Title,
			"attachment_ids":   []string{"client_attachment_note_success"},
			"files": []map[string]any{
				{
					"client_attachment_id": "client_attachment_note_success",
					"attachment_id":        "natt_success",
					"filename":             "idea.png",
					"content_type":         "image/png",
					"size_bytes":           12,
				},
			},
		},
	}, &completed)
	if completed.State != assistantStateCompleted || completed.AssistantMessage == nil {
		t.Fatalf("note commit did not complete: %#v", completed)
	}
	if !strings.Contains(completed.AssistantMessage.Text, "Stored 1 attachment(s) in `Project Ideas`.") {
		t.Fatalf("unexpected completion text: %#v", completed.AssistantMessage.Text)
	}
	if len(completed.AssistantMessage.Cards) != 1 || completed.AssistantMessage.Cards[0].Kind != "note" || completed.AssistantMessage.Cards[0].NoteID != note.NoteID {
		t.Fatalf("completion cards = %#v, want note card", completed.AssistantMessage.Cards)
	}
	records, err := db.ListAssistantAttachments(ctx, apiSession.ID)
	if err != nil {
		t.Fatalf("ListAssistantAttachments: %v", err)
	}
	if len(records) != 1 || records[0].Status != "committed" || records[0].CommittedAt == nil {
		t.Fatalf("attachment status = %#v, want one committed record", records)
	}
}

func TestAssistantAttachmentSMBCommitCompletesAndMarksCommitted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_attachment_smb_success", Email: "attachment-smb-success@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_attachment_smb_success", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	appSession := domain.AppSession{ID: "sess_attachment_smb_success", UserID: user.ID, TokenHash: hashToken("attachment-smb-success-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, appSession))
	must(t, db.UpsertAssistantFileIndex(ctx, domain.AssistantFileIndex{
		ID:             "afile_tax_folder_success",
		HomeID:         home.ID,
		Path:           "Documents/Taxes",
		Name:           "Taxes",
		IsDirectory:    true,
		SearchText:     "Documents Taxes",
		MetadataJSON:   "{}",
		EmbeddingJSON:  "[]",
		UpdatedAt:      now,
		EmbeddingModel: "test",
	}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var apiSession assistantAPISession
	requestJSON(t, testServer, "attachment-smb-success-token", http.MethodPost, "/v1/home/assistant/sessions", nil, &apiSession)

	var initial assistantRunResponse
	requestJSON(t, testServer, "attachment-smb-success-token", http.MethodPost, "/v1/home/assistant/sessions/"+apiSession.ID+"/messages", map[string]any{
		"content": "store this in the Taxes folder on SMB share",
		"attachments": []map[string]any{
			{
				"client_attachment_id": "client_attachment_smb_success",
				"filename":             "tax.pdf",
				"content_type":         "application/pdf",
				"size_bytes":           24,
				"checksum_sha256":      "def456",
				"kind":                 "document",
			},
		},
	}, &initial)
	if !initial.RequiresConfirmation || initial.PendingActionSummary == nil {
		t.Fatalf("initial run did not request attachment confirmation: %#v", initial)
	}

	var toolRun assistantRunResponse
	requestJSON(t, testServer, "attachment-smb-success-token", http.MethodPost, "/v1/home/assistant/runs/"+initial.ID+"/confirm", map[string]any{"approved": true}, &toolRun)
	if !toolRun.RequiresClientTools || toolRun.ClientToolRequest == nil || toolRun.ClientToolRequest.ToolName != "attachments.commit" {
		t.Fatalf("confirmed run did not request attachment client tool: %#v", toolRun)
	}

	var completed assistantRunResponse
	requestJSON(t, testServer, "attachment-smb-success-token", http.MethodPost, "/v1/home/assistant/runs/"+toolRun.ID+"/client-tool-results", map[string]any{
		"tool_name": "attachments.commit",
		"result": map[string]any{
			"destination_kind": "smb",
			"target_path":      "Documents/Taxes",
			"attachment_ids":   []string{"client_attachment_smb_success"},
			"files": []map[string]any{
				{
					"client_attachment_id": "client_attachment_smb_success",
					"filename":             "tax.pdf",
					"path":                 "Documents/Taxes/tax.pdf",
					"content_type":         "application/pdf",
					"size_bytes":           24,
				},
			},
		},
	}, &completed)
	if completed.State != assistantStateCompleted || completed.AssistantMessage == nil {
		t.Fatalf("SMB commit did not complete: %#v", completed)
	}
	if !strings.Contains(completed.AssistantMessage.Text, "Stored 1 attachment(s) in `Documents/Taxes`.") {
		t.Fatalf("unexpected completion text: %#v", completed.AssistantMessage.Text)
	}
	if len(completed.AssistantMessage.Cards) != 1 || completed.AssistantMessage.Cards[0].Kind != "file" || completed.AssistantMessage.Cards[0].Path != "Documents/Taxes/tax.pdf" {
		t.Fatalf("completion cards = %#v, want file card", completed.AssistantMessage.Cards)
	}
	records, err := db.ListAssistantAttachments(ctx, apiSession.ID)
	if err != nil {
		t.Fatalf("ListAssistantAttachments: %v", err)
	}
	if len(records) != 1 || records[0].Status != "committed" || records[0].CommittedAt == nil {
		t.Fatalf("attachment status = %#v, want one committed record", records)
	}
}

func TestAssistantAttachmentSMBFolderResolutionUsesFileIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_attachment_smb", Email: "attachment-smb@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_attachment_smb", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AssistantSession{ID: "asess_attachment_smb", HomeID: home.ID, UserID: user.ID, Title: "SMB uploads", LastMessageAt: now, CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateAssistantSession(ctx, session))
	must(t, db.UpsertAssistantFileIndex(ctx, domain.AssistantFileIndex{
		ID:             "afile_tax_folder",
		HomeID:         home.ID,
		Path:           "Documents/Taxes",
		Name:           "Taxes",
		IsDirectory:    true,
		SearchText:     "Documents Taxes",
		MetadataJSON:   "{}",
		EmbeddingJSON:  "[]",
		UpdatedAt:      now,
		EmbeddingModel: "test",
	}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	auth := authContext{User: user}
	membership := domain.HomeMembership{HomeID: home.ID, UserID: user.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}
	attachments := []assistantMessageAttachment{{
		ClientAttachmentID: "client_attachment_tax",
		Filename:           "tax.pdf",
		ContentType:        "application/pdf",
		SizeBytes:          12,
		ChecksumSHA256:     "abc123",
		Kind:               "document",
	}}

	response, err := server.processAssistantMessageWithAttachments(ctx, home, membership, auth, session, "store this in the Taxes folder on SMB share", attachments, "device", "UTC")
	if err != nil {
		t.Fatalf("process smb attachment message: %v", err)
	}
	if !response.RequiresConfirmation || response.PendingActionSummary == nil {
		t.Fatalf("SMB folder did not resolve to confirmation: %#v", response)
	}
	if !assistantSummaryDetailsContain(response.PendingActionSummary.Details, "Target folder", "Documents/Taxes") {
		t.Fatalf("confirmation details did not include resolved folder: %#v", response.PendingActionSummary.Details)
	}
}

func TestAssistantFileSearchUsesFileIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_assistant_files", Email: "assistant-files@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_assistant_files", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.UpsertAssistantFileIndex(ctx, domain.AssistantFileIndex{
		ID:               "afile_2025_taxes",
		HomeID:           home.ID,
		Path:             "Documents/Taxes/2025 Taxes",
		Name:             "2025 Taxes",
		IsDirectory:      true,
		SearchText:       "Documents Taxes 2025 Taxes",
		MetadataJSON:     "{}",
		EmbeddingJSON:    "[]",
		EmbeddingModel:   "test",
		EmbeddingVersion: "test",
		UpdatedAt:        now,
	}))
	must(t, db.UpsertAssistantFileIndex(ctx, domain.AssistantFileIndex{
		ID:               "afile_2025_w2",
		HomeID:           home.ID,
		Path:             "Documents/Taxes/2025 Taxes/W2.pdf",
		Name:             "W2.pdf",
		IsDirectory:      false,
		SearchText:       "Documents Taxes 2025 W2",
		MetadataJSON:     "{}",
		EmbeddingJSON:    "[]",
		EmbeddingModel:   "test",
		EmbeddingVersion: "test",
		UpdatedAt:        now.Add(-time.Minute),
	}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	settings := defaultAssistantSettings(home.ID, user.ID)
	settings.ProjectDocsEnabled = false
	auth := authContext{User: user}
	membership := domain.HomeMembership{HomeID: home.ID, UserID: user.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}

	answer, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, "find 2025 taxes")
	if err != nil {
		t.Fatalf("generateAssistantResponse: %v", err)
	}
	if len(answer.Cards) != 1 || answer.Cards[0].Kind != "file" || answer.Cards[0].Path != "Documents/Taxes/2025 Taxes" {
		t.Fatalf("file search cards = %#v", answer.Cards)
	}

	allAnswer, err := server.generateAssistantResponse(ctx, home, membership, auth, settings, "show me all 2025 taxes")
	if err != nil {
		t.Fatalf("generateAssistantResponse all files: %v", err)
	}
	if len(allAnswer.Cards) < 2 {
		t.Fatalf("broad file search cards = %#v, want multiple", allAnswer.Cards)
	}
}

func TestAssistantHomeAssistantQueryUsesLiveStates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	states := []protocol.HomeAssistantState{
		{EntityID: "light.garage_overhead", State: "on", Attributes: map[string]any{"friendly_name": "Garage Overhead"}},
		{EntityID: "switch.kitchen_outlet", State: "on", Attributes: map[string]any{"friendly_name": "Kitchen Outlet"}},
		{EntityID: "sensor.garage_temperature", State: "72", Attributes: map[string]any{"friendly_name": "Garage Temperature"}},
		{EntityID: "light.kitchenlightshelly", State: "off", Attributes: map[string]any{"friendly_name": "KitchenLightShelly Switch 0"}},
		{EntityID: "switch.kitchen_light_scene", State: "on", Attributes: map[string]any{"friendly_name": "Kitchen Light Scene"}},
		{EntityID: "binary_sensor.kitchen_light_motion", State: "off", Attributes: map[string]any{"friendly_name": "Kitchen Light Motion"}},
		{EntityID: "light.kitchen_island", State: "off", Attributes: map[string]any{"friendly_name": "Kitchen Island"}},
		{EntityID: "light.porch", State: "off", Attributes: map[string]any{"friendly_name": "Porch Light"}},
	}
	go serveAssistantHomeAssistantStates(ctx, t, agentConn, agentID, homeID, states)

	var session assistantAPISession
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions", nil, &session)

	var onRun assistantRunResponse
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions/"+session.ID+"/messages", map[string]any{
		"content": "what entities are on",
		"device_context": map[string]any{
			"device_id": "test-device",
			"timezone":  "UTC",
		},
	}, &onRun)
	if onRun.AssistantMessage == nil {
		t.Fatalf("missing assistant message for on query: %#v", onRun)
	}
	if !strings.Contains(onRun.AssistantMessage.Text, "Garage Overhead") || !strings.Contains(onRun.AssistantMessage.Text, "Kitchen Outlet") {
		t.Fatalf("on query text missing live on entities: %q", onRun.AssistantMessage.Text)
	}
	if strings.Contains(onRun.AssistantMessage.Text, "Garage Temperature") {
		t.Fatalf("on query included non-on entity: %q", onRun.AssistantMessage.Text)
	}
	if !assistantCardsContainKindAndSearchText(onRun.AssistantMessage.Cards, "homeassistant", "light.garage_overhead") ||
		!assistantCardsContainKindAndSearchText(onRun.AssistantMessage.Cards, "homeassistant", "switch.kitchen_outlet") {
		t.Fatalf("on query cards missing live entities: %#v", onRun.AssistantMessage.Cards)
	}

	var garageRun assistantRunResponse
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions/"+session.ID+"/messages", map[string]any{
		"content": "garage entities",
		"device_context": map[string]any{
			"device_id": "test-device",
			"timezone":  "UTC",
		},
	}, &garageRun)
	if garageRun.AssistantMessage == nil {
		t.Fatalf("missing assistant message for garage query: %#v", garageRun)
	}
	if !strings.Contains(garageRun.AssistantMessage.Text, "Garage Overhead") || !strings.Contains(garageRun.AssistantMessage.Text, "Garage Temperature") {
		t.Fatalf("garage query text missing garage entities: %q", garageRun.AssistantMessage.Text)
	}
	if strings.Contains(garageRun.AssistantMessage.Text, "Kitchen Outlet") {
		t.Fatalf("garage query included non-garage entity: %q", garageRun.AssistantMessage.Text)
	}
	if !assistantCardsContainKindAndSearchText(garageRun.AssistantMessage.Cards, "homeassistant", "light.garage_overhead") ||
		!assistantCardsContainKindAndSearchText(garageRun.AssistantMessage.Cards, "homeassistant", "sensor.garage_temperature") {
		t.Fatalf("garage query cards missing garage entities: %#v", garageRun.AssistantMessage.Cards)
	}

	var garageLightRun assistantRunResponse
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions/"+session.ID+"/messages", map[string]any{
		"content": "can you find all the garage light entities",
		"device_context": map[string]any{
			"device_id": "test-device",
			"timezone":  "UTC",
		},
	}, &garageLightRun)
	if garageLightRun.AssistantMessage == nil {
		t.Fatalf("missing assistant message for garage light query: %#v", garageLightRun)
	}
	if !strings.Contains(garageLightRun.AssistantMessage.Text, "Garage Overhead") {
		t.Fatalf("garage light query missing garage light: %q", garageLightRun.AssistantMessage.Text)
	}
	if strings.Contains(garageLightRun.AssistantMessage.Text, "Garage Temperature") || strings.Contains(garageLightRun.AssistantMessage.Text, "Kitchen Outlet") {
		t.Fatalf("garage light query included wrong entity: %q", garageLightRun.AssistantMessage.Text)
	}
	if !assistantCardsContainKindAndSearchText(garageLightRun.AssistantMessage.Cards, "homeassistant", "light.garage_overhead") {
		t.Fatalf("garage light query cards missing garage light: %#v", garageLightRun.AssistantMessage.Cards)
	}

	var kitchenLightRun assistantRunResponse
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions/"+session.ID+"/messages", map[string]any{
		"content": "show me all the kitchen light entities",
		"device_context": map[string]any{
			"device_id": "test-device",
			"timezone":  "UTC",
		},
	}, &kitchenLightRun)
	if kitchenLightRun.AssistantMessage == nil {
		t.Fatalf("missing assistant message for kitchen light query: %#v", kitchenLightRun)
	}
	for _, want := range []string{"KitchenLightShelly", "Kitchen Light Scene", "Kitchen Light Motion", "Kitchen Island"} {
		if !strings.Contains(kitchenLightRun.AssistantMessage.Text, want) {
			t.Fatalf("kitchen light query missing %q: %q", want, kitchenLightRun.AssistantMessage.Text)
		}
	}
	if strings.Contains(kitchenLightRun.AssistantMessage.Text, "Kitchen Outlet") || strings.Contains(kitchenLightRun.AssistantMessage.Text, "Porch Light") {
		t.Fatalf("kitchen light query included wrong entity: %q", kitchenLightRun.AssistantMessage.Text)
	}
	for _, want := range []string{"light.kitchenlightshelly", "switch.kitchen_light_scene", "binary_sensor.kitchen_light_motion", "light.kitchen_island"} {
		if !assistantCardsContainKindAndSearchText(kitchenLightRun.AssistantMessage.Cards, "homeassistant", want) {
			t.Fatalf("kitchen light query cards missing %q: %#v", want, kitchenLightRun.AssistantMessage.Cards)
		}
	}
}

func assistantCardsContainTitle(cards []assistantResultCard, title string) bool {
	for _, card := range cards {
		if card.Title == title {
			return true
		}
	}
	return false
}

func assistantCardsContainKindAndSearchText(cards []assistantResultCard, kind string, searchText string) bool {
	for _, card := range cards {
		if card.Kind == kind && card.SearchText == searchText {
			return true
		}
	}
	return false
}

func assistantSummaryDetailsContain(details []assistantPendingActionDetail, label string, value string) bool {
	for _, detail := range details {
		if detail.Label == label && detail.Value == value {
			return true
		}
	}
	return false
}

func serveAssistantHomeAssistantStates(ctx context.Context, t *testing.T, agentConn *websocket.Conn, agentID string, homeID string, states []protocol.HomeAssistantState) {
	t.Helper()

	for {
		var envelope protocol.Envelope
		if err := wsjson.Read(ctx, agentConn, &envelope); err != nil {
			return
		}
		if envelope.Type != protocol.TypeCloudCommand {
			continue
		}
		command, err := protocol.DecodePayload[protocol.RoutedCommand](envelope)
		if err != nil {
			return
		}
		if command.Command != "homeassistant.fetch_states" {
			continue
		}
		response, err := protocol.NewEnvelope(protocol.TypeCloudResponse, envelope.RequestID, agentID, homeID, protocol.HomeAssistantFetchStatesResponse{States: states})
		if err != nil {
			return
		}
		_ = wsjson.Write(ctx, agentConn, response)
	}
}
