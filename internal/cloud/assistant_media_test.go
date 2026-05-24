package cloud

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

func TestMediaAvailabilityPromptMatching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		prompt string
		want   string
		ok     bool
	}{
		{prompt: "is Spongebob Squarepants available for download", want: "Spongebob Squarepants", ok: true},
		{prompt: "is Batman Returns availabe for download", want: "Batman Returns", ok: true},
		{prompt: "can I download The Matrix", want: "The Matrix", ok: true},
		{prompt: "find The Office for download", want: "The Office", ok: true},
		{prompt: `find the normal movie for download`, want: "normal movie", ok: true},
		{prompt: `find "normal" movie`, want: "normal", ok: true},
		{prompt: `search for "normal" movie for download`, want: "normal", ok: true},
		{prompt: `search "normal" movie for download`, want: "normal", ok: true},
		{prompt: "find normal movie", want: "normal movie", ok: true},
		{prompt: "find 2025 taxes", ok: false},
		{prompt: "find 2025 taxes for download", ok: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.prompt, func(t *testing.T) {
			t.Parallel()
			got, ok := mediaAvailabilityQuery(test.prompt)
			if ok != test.ok || got != test.want {
				t.Fatalf("mediaAvailabilityQuery(%q) = %q, %v; want %q, %v", test.prompt, got, ok, test.want, test.ok)
			}
			if test.ok {
				intent := classifyAssistantIntent(test.prompt)
				if intent.Kind != assistantIntentMediaSearch {
					t.Fatalf("classified intent = %s, want %s", intent.Kind, assistantIntentMediaSearch)
				}
			}
		})
	}
}

func TestResolvePreviousMediaSelection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_media_selection", Email: "media-selection@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_media_selection", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AssistantSession{ID: "as_media_selection", HomeID: home.ID, UserID: user.ID, Title: "Media", CreatedAt: now, UpdatedAt: now, LastMessageAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateAssistantSession(ctx, session))

	content := assistantMessageContent{
		Text: "I found media matches.",
		Cards: []assistantResultCard{
			{Kind: "media", Title: "Batman Returns (1992)", Path: "/movies/2787-batman-returns", MediaOptionID: "movies/2787-batman-returns", MediaType: "movie", Year: 1992},
			{Kind: "media", Title: "SpongeBob SquarePants (1999)", Path: "/series/1073-spongebob-squarepants", MediaOptionID: "series/1073-spongebob-squarepants", MediaType: "series", Year: 1999},
		},
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	must(t, db.CreateAssistantMessage(ctx, domain.AssistantMessage{
		ID:          "amsg_media_selection",
		SessionID:   session.ID,
		Role:        assistantRoleAssistant,
		Status:      assistantStateCompleted,
		ContentJSON: string(encoded),
		ModelName:   assistantModelName,
		CreatedAt:   now,
	}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	selected, ok := server.resolvePreviousMediaSelection(ctx, session.ID, "option 2")
	if !ok || selected.Title != "SpongeBob SquarePants (1999)" {
		t.Fatalf("option selection = %#v, %v", selected, ok)
	}
	selected, ok = server.resolvePreviousMediaSelection(ctx, session.ID, "Batman Returns")
	if !ok || selected.Path != "/movies/2787-batman-returns" {
		t.Fatalf("title selection = %#v, %v", selected, ok)
	}
}
