package cloud

import (
	"context"
	"io"
	"log/slog"
	"net/http"
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
		{name: "note append typo delimiter", prompt: "add fix hankai conversation output the the hank features note", want: assistantIntentNotesAppend},
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
	for _, want := range []string{"Grocery List", "Hank Features", "Shared Checklist"} {
		if !strings.Contains(listed.Text, want) {
			t.Fatalf("note list missing %q: %s", want, listed.Text)
		}
	}
	if len(listed.Cards) < 3 {
		t.Fatalf("note list cards = %#v, want at least 3", listed.Cards)
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
