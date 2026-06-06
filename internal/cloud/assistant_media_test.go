package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
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

func TestGramatonSlashCommandPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		prompt string
		want   string
		ok     bool
	}{
		{prompt: "/gramaton dutton ranch", want: "dutton ranch", ok: true},
		{prompt: "/Gramaton\nDutton Ranch", want: "Dutton Ranch", ok: true},
		{prompt: "/gramaton", want: "", ok: true},
		{prompt: "/gramatonic dutton ranch", ok: false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.prompt, func(t *testing.T) {
			t.Parallel()
			got, ok := gramatonCommandPrompt(test.prompt)
			if ok != test.ok || got != test.want {
				t.Fatalf("gramatonCommandPrompt(%q) = %q, %v; want %q, %v", test.prompt, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestAssistantMediaCardCarriesPosterImage(t *testing.T) {
	t.Parallel()

	card := assistantMediaCard(protocol.MediaSearchResult{
		ID:        "movies/20429-project-hail-mary",
		Title:     "Project Hail Mary",
		Year:      2026,
		Type:      protocol.MediaTypeMovie,
		PosterURL: "https://image.example/project-hail-mary.jpg",
		PagePath:  "/movies/20429-project-hail-mary",
	}, 1)
	if card.ImageURL != "https://image.example/project-hail-mary.jpg" {
		t.Fatalf("card image = %q", card.ImageURL)
	}
}

func TestAssistantMediaConfirmationCarriesPosterCard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	const posterURL = "https://image.example/fixture-movie.jpg"
	errCh := make(chan error, 1)
	go func() {
		if err := serveOneMediaAgentCommand(ctx, agentConn, agentID, homeID, protocol.CommandMediaSearch, func(body json.RawMessage) (any, error) {
			var request protocol.MediaSearchRequest
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, err
			}
			if request.Query != "Fixture Movie" {
				return nil, fmt.Errorf("media search query = %q, want Fixture Movie", request.Query)
			}
			return protocol.MediaSearchResponse{
				Query: request.Query,
				Results: []protocol.MediaSearchResult{{
					ID:        "movies/fixture-movie",
					Title:     "Fixture Movie",
					Year:      2026,
					Type:      protocol.MediaTypeMovie,
					Summary:   "Movie | Fixture",
					PosterURL: posterURL,
					PagePath:  "/movies/fixture-movie",
				}},
			}, nil
		}); err != nil {
			errCh <- err
			return
		}
		errCh <- serveOneMediaAgentCommand(ctx, agentConn, agentID, homeID, protocol.CommandMediaPlanDownload, func(body json.RawMessage) (any, error) {
			var request protocol.MediaPlanDownloadRequest
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, err
			}
			if request.Selection.PosterURL != posterURL {
				return nil, fmt.Errorf("planned selection poster = %q, want %q", request.Selection.PosterURL, posterURL)
			}
			return protocol.MediaPlanDownloadResponse{
				Plan: protocol.MediaDownloadPlan{
					Selection:             request.Selection,
					Items:                 []protocol.MediaDownloadItem{{Title: "Fixture Movie", MediaType: protocol.MediaTypeMovie, Quality: "1080p", Filename: "Fixture Movie.mp4", DownloadOK: true}},
					ItemCount:             1,
					PreferredQualityCount: 1,
					DestinationPath:       "Media root/Movies",
				},
			}, nil
		})
	}()

	var session assistantAPISession
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions", nil, &session)

	var run assistantRunResponse
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions/"+session.ID+"/messages", map[string]any{
		"content": "find Fixture Movie for download",
	}, &run)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if !run.RequiresConfirmation || run.AssistantMessage == nil {
		t.Fatalf("run did not wait for media confirmation: %#v", run)
	}
	if len(run.AssistantMessage.Cards) != 1 || run.AssistantMessage.Cards[0].ImageURL != posterURL {
		t.Fatalf("confirmation cards = %#v, want poster %q", run.AssistantMessage.Cards, posterURL)
	}
	if run.PendingActionSummary == nil || run.PendingActionSummary.Kind != "media_download" {
		t.Fatalf("pending action summary = %#v", run.PendingActionSummary)
	}
}

func TestAssistantMediaStartsImmediatelyWhenConfirmationDisabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	requireConfirmation := false
	errCh := make(chan error, 1)
	go func() {
		if err := serveOneMediaAgentCommand(ctx, agentConn, agentID, homeID, protocol.CommandMediaSearch, func(body json.RawMessage) (any, error) {
			var request protocol.MediaSearchRequest
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, err
			}
			return protocol.MediaSearchResponse{
				Query: request.Query,
				Results: []protocol.MediaSearchResult{{
					ID:        "movies/fixture-movie",
					Title:     "Fixture Movie",
					Year:      2026,
					Type:      protocol.MediaTypeMovie,
					PosterURL: "https://image.example/fixture-movie.jpg",
					PagePath:  "/movies/fixture-movie",
				}},
			}, nil
		}); err != nil {
			errCh <- err
			return
		}
		if err := serveOneMediaAgentCommand(ctx, agentConn, agentID, homeID, protocol.CommandMediaPlanDownload, func(body json.RawMessage) (any, error) {
			var request protocol.MediaPlanDownloadRequest
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, err
			}
			return protocol.MediaPlanDownloadResponse{
				Plan: protocol.MediaDownloadPlan{
					Selection:             request.Selection,
					Items:                 []protocol.MediaDownloadItem{{Title: "Fixture Movie", MediaType: protocol.MediaTypeMovie, Quality: "1080p", Filename: "Fixture Movie.mp4", DownloadOK: true}},
					ItemCount:             1,
					PreferredQualityCount: 1,
					DestinationPath:       "SMB share/Movies",
					RequireConfirmation:   &requireConfirmation,
				},
			}, nil
		}); err != nil {
			errCh <- err
			return
		}
		errCh <- serveOneMediaAgentCommand(ctx, agentConn, agentID, homeID, protocol.CommandMediaDownloadStart, func(body json.RawMessage) (any, error) {
			var request protocol.MediaDownloadStartRequest
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, err
			}
			return protocol.MediaDownloadStartResponse{Job: protocol.MediaDownloadJobStatus{
				JobID:      "job_fixture",
				Status:     protocol.MediaJobStatusQueued,
				Title:      request.Selection.Title,
				TotalCount: 1,
			}}, nil
		})
	}()

	var session assistantAPISession
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions", nil, &session)

	var run assistantRunResponse
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/sessions/"+session.ID+"/messages", map[string]any{
		"content": "find Fixture Movie for download",
	}, &run)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if run.RequiresConfirmation || run.State != assistantStateCompleted || run.AssistantMessage == nil {
		t.Fatalf("run should complete without confirmation: %#v", run)
	}
	if len(run.AssistantMessage.Cards) != 1 || run.AssistantMessage.Cards[0].JobID != "job_fixture" {
		t.Fatalf("started job card = %#v", run.AssistantMessage.Cards)
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
