package media

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	"github.com/dropfile/hankremote/internal/protocol"
)

func TestParseSearchResultsAndDetailPages(t *testing.T) {
	t.Parallel()

	results := parseSearchResults(strings.NewReader(`
		<div class="item">
			<a class="movie-card-link" href="/movies/2787-batman-returns"></a>
			<h2>Batman Returns</h2>
			<p>Action, Fantasy PG-13 7.1 1992 watch</p>
		</div>
		<div class="item">
			<a class="movie-card-link" href="/series/1073-spongebob-squarepants?recent=true&season=1&episode=1"></a>
			<h2>SpongeBob SquarePants</h2>
			<p>Animation, Comedy TV-Y7 8.2 1999 watch</p>
		</div>
	`), protocol.MediaTypeMovie)
	if len(results) != 2 {
		t.Fatalf("results = %#v, want 2", results)
	}
	if results[0].Type != protocol.MediaTypeMovie || results[0].PagePath != "/movies/2787-batman-returns" || results[0].Year != 1992 {
		t.Fatalf("movie result = %#v", results[0])
	}
	if results[1].Type != protocol.MediaTypeSeries || results[1].PagePath != "/series/1073-spongebob-squarepants" || results[1].Year != 1999 {
		t.Fatalf("series result = %#v", results[1])
	}

	page := parseDetailPage(strings.NewReader(`
		<h1>Fixture Show</h1>
		<a href="https://dl.example/fixture_720p.mp4?file=Fixture_720p.mp4">Download 720p</a>
		<a href="https://dl.example/fixture_1080p.mp4?file=Fixture_1080p.mp4">Download 1080p</a>
		<div class="tv-details-episodes">
			<ol id="season1">
				<li data-episode="1">Pilot</li>
				<li data-episode="2">Second</li>
			</ol>
			<ol id="season2">
				<li data-episode="1">Return</li>
			</ol>
		</div>
	`))
	if page.title != "Fixture Show" {
		t.Fatalf("title = %q", page.title)
	}
	if got := chooseDownloadLink(page.downloads); got.quality != "1080p" {
		t.Fatalf("chosen link = %#v, want 1080p", got)
	}
	if len(page.episodes) != 3 || page.episodes[2].season != 2 || page.episodes[2].episode != 1 {
		t.Fatalf("episodes = %#v", page.episodes)
	}
}

func TestSearchUsesDistinctiveTitleVariants(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var seenQueries []string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/session/logout">Sign out</a>`)
	})
	mux.HandleFunc("/movies", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<script>var token_key = "token";</script>`)
	})
	mux.HandleFunc("/series", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<script>var token_key = "token";</script>`)
	})
	mux.HandleFunc("/index/loadmovies", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		query := r.FormValue("q")
		seenQueries = append(seenQueries, query)
		if strings.EqualFold(query, "spongebob") && r.FormValue("type") == "movie" {
			fmt.Fprint(w, `
				<div class="item">
					<a class="movie-card-link" href="/movies/123-the-spongebob-squarepants-movie"></a>
					<h2>The SpongeBob SquarePants Movie</h2>
					<p>Animation, Comedy PG 7.2 2004 watch</p>
				</div>
			`)
			return
		}
		fmt.Fprint(w, `<div></div>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	service := New(Config{
		Enabled:  true,
		BaseURL:  server.URL,
		Username: "user@example.com",
		Password: "password",
	}, agentfiles.New(t.TempDir()), nil)

	response, err := service.Search(ctx, "SpongeBob movie", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one fuzzy variant match", response.Results)
	}
	if response.Results[0].Title != "The SpongeBob SquarePants Movie" {
		t.Fatalf("result title = %q", response.Results[0].Title)
	}
	if !containsStringFold(seenQueries, "spongebob") {
		t.Fatalf("seen queries = %#v, want distinctive title search variant", seenQueries)
	}
}

func TestLiveMediaSearchFromEnv(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE")), "true") {
		t.Skip("set HANK_REMOTE_MEDIA_LIVE=true with media source env vars to run live diagnostics")
	}

	query := strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE_QUERY"))
	if query == "" {
		query = "project hail mary"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_GRAMATON_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://gramaton.io"
	}
	username := strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_GRAMATON_USERNAME"))
	password := os.Getenv("HANK_REMOTE_MEDIA_GRAMATON_PASSWORD")
	if username == "" || password == "" {
		t.Fatal("set HANK_REMOTE_MEDIA_GRAMATON_USERNAME and HANK_REMOTE_MEDIA_GRAMATON_PASSWORD to run live diagnostics")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	service := New(Config{
		Enabled:  true,
		BaseURL:  baseURL,
		Username: username,
		Password: password,
	}, agentfiles.New(t.TempDir()), nil)
	if err := service.ensureAuthenticated(ctx); err != nil {
		t.Fatalf("authenticate media source: %v", err)
	}

	variants := mediaSearchQueries(query)
	t.Logf("live media search query=%q variants=%q", query, variants)
	var rawResults []protocol.MediaSearchResult
	for _, mediaType := range []string{protocol.MediaTypeMovie, protocol.MediaTypeSeries} {
		for _, variant := range variants {
			found, err := service.searchType(ctx, mediaType, variant)
			if err != nil {
				t.Logf("provider search type=%s query=%q error=%v", mediaType, variant, err)
				continue
			}
			t.Logf("provider search type=%s query=%q parsed=%d", mediaType, variant, len(found))
			for _, result := range found {
				score := mediaResultScore(result, query)
				t.Logf("  score=%d title=%q year=%d type=%s path=%q", score, result.Title, result.Year, result.Type, result.PagePath)
			}
			rawResults = append(rawResults, found...)
		}
	}

	unique := uniqueSearchResults(rawResults)
	t.Logf("raw_unique=%d", len(unique))
	for _, result := range unique {
		t.Logf("  unique score=%d title=%q year=%d type=%s path=%q", mediaResultScore(result, query), result.Title, result.Year, result.Type, result.PagePath)
	}

	response, err := service.Search(ctx, query, 10)
	if err != nil {
		t.Fatalf("live media search: %v", err)
	}
	t.Logf("filtered=%d", len(response.Results))
	for _, result := range response.Results {
		t.Logf("  filtered score=%d title=%q year=%d type=%s path=%q", mediaResultScore(result, query), result.Title, result.Year, result.Type, result.PagePath)
	}
	if len(response.Results) == 0 {
		t.Fatalf("live search returned no filtered results for %q; inspect provider search and parsed result logs above", query)
	}
}

func TestMediaDownloadJobPrefers1080FallsBackAndSkipsExisting(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/session/logout">Sign out</a><script>var token_key = "token";</script>`)
	})
	mux.HandleFunc("/series/fixture-show", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("season") == "1" && r.URL.Query().Get("episode") == "1":
			fmt.Fprintf(w, `<h1>Fixture Show</h1><a href="%s/download/s1e1_1080p.mp4?file=Fixture_Show_S01E01_1080p.mp4">Download 1080p</a>`, baseURL)
		case r.URL.Query().Get("season") == "1" && r.URL.Query().Get("episode") == "2":
			fmt.Fprintf(w, `<h1>Fixture Show</h1><a href="%s/download/s1e2_720p.mp4?file=Fixture_Show_S01E02_720p.mp4">Download 720p</a>`, baseURL)
		default:
			fmt.Fprint(w, `<h1>Fixture Show</h1><div class="tv-details-episodes"><ol id="season1"><li data-episode="1">Pilot</li><li data-episode="2">Second</li></ol></div>`)
		}
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "download:"+filepath.Base(r.URL.Path))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	baseURL = server.URL

	root := t.TempDir()
	existingPath := filepath.Join(root, "Fixture_Show_S01E02_720p.mp4")
	if err := os.WriteFile(existingPath, []byte("already"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	service := New(Config{
		Enabled:  true,
		BaseURL:  server.URL,
		Username: "user@example.com",
		Password: "password",
	}, agentfiles.New(root), nil)
	var events []protocol.MediaDownloadJobStatus
	service.SetEventSink(func(ctx context.Context, event string, topic string, payload any) error {
		if status, ok := payload.(protocol.MediaDownloadJobStatus); ok {
			events = append(events, status)
		}
		return nil
	})

	selection := protocol.MediaSearchResult{
		ID:       "series/fixture-show",
		Title:    "Fixture Show",
		Type:     protocol.MediaTypeSeries,
		PagePath: "/series/fixture-show",
	}
	planResponse, err := service.PlanDownload(ctx, protocol.MediaPlanDownloadRequest{Selection: selection})
	if err != nil {
		t.Fatalf("PlanDownload: %v", err)
	}
	plan := planResponse.Plan
	if plan.ItemCount != 2 || plan.PreferredQualityCount != 1 || plan.FallbackQualityCount != 1 || plan.ExistingCount != 1 {
		t.Fatalf("plan = %#v", plan)
	}

	start, err := service.StartDownload(ctx, protocol.MediaDownloadStartRequest{Selection: selection})
	if err != nil {
		t.Fatalf("StartDownload: %v", err)
	}
	status := waitForJob(t, ctx, service, start.Job.JobID)
	if status.Status != protocol.MediaJobStatusCompleted || status.CompletedCount != 2 || status.SkippedCount != 1 || status.FailedCount != 0 {
		t.Fatalf("job status = %#v", status)
	}
	data, err := os.ReadFile(filepath.Join(root, "Fixture_Show_S01E01_1080p.mp4"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !strings.Contains(string(data), "s1e1_1080p") {
		t.Fatalf("downloaded data = %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(root, "Fixture_Show_S01E01_1080p.mp4.part")); !os.IsNotExist(err) {
		t.Fatalf("part file still exists or unexpected error: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected progress/completed events")
	}
}

func TestApplySettingsPersistsMediaEnv(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	envPath := filepath.Join(root, ".env.agent")
	if err := os.WriteFile(envPath, []byte("HANK_REMOTE_AGENT_ID=agent_1\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	service := New(Config{
		EnvPath: envPath,
	}, agentfiles.New(root), nil)

	response, err := service.ApplySettings(ctx, protocol.MediaSettingsApplyRequest{
		Settings: protocol.MediaSettings{
			Enabled:         true,
			BaseURL:         "https://gramaton.io/",
			Username:        "media@example.com",
			DestinationPath: "Shows/Fixture",
		},
		Password: "test-password",
		Persist:  true,
	})
	if err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}
	if !response.Settings.Enabled || !response.Settings.HasPassword || response.Settings.DestinationPath != "Shows/Fixture" {
		t.Fatalf("settings response = %#v", response.Settings)
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	env := string(data)
	for _, want := range []string{
		"HANK_REMOTE_AGENT_ID=agent_1",
		"HANK_REMOTE_MEDIA_GRAMATON_ENABLED=true",
		"HANK_REMOTE_MEDIA_GRAMATON_BASE_URL=https://gramaton.io",
		"HANK_REMOTE_MEDIA_GRAMATON_USERNAME=media@example.com",
		"HANK_REMOTE_MEDIA_GRAMATON_PASSWORD=test-password",
		"HANK_REMOTE_MEDIA_DESTINATION_PATH=Shows/Fixture",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("env file missing %q:\n%s", want, env)
		}
	}
}

func TestMediaDownloadJobCanBeCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	started := make(chan struct{})
	var startedOnce sync.Once
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/session/logout">Sign out</a>`)
	})
	mux.HandleFunc("/movies/fixture-movie", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<h1>Fixture Movie</h1><a href="%s/download/slow_1080p.mp4?file=Fixture_Movie_1080p.mp4">Download 1080p</a>`, baseURL)
	})
	mux.HandleFunc("/download/slow_1080p.mp4", func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	baseURL = server.URL

	root := t.TempDir()
	service := New(Config{
		Enabled:  true,
		BaseURL:  server.URL,
		Username: "user@example.com",
		Password: "password",
	}, agentfiles.New(root), nil)

	start, err := service.StartDownload(ctx, protocol.MediaDownloadStartRequest{Selection: protocol.MediaSearchResult{
		ID:       "movies/fixture-movie",
		Title:    "Fixture Movie",
		Type:     protocol.MediaTypeMovie,
		PagePath: "/movies/fixture-movie",
	}})
	if err != nil {
		t.Fatalf("StartDownload: %v", err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("download did not start: %v", ctx.Err())
	}
	cancelResponse, err := service.Cancel(ctx, start.Job.JobID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelResponse.Job.Status != protocol.MediaJobStatusCancelled {
		t.Fatalf("cancel response = %#v", cancelResponse.Job)
	}
	status := waitForJob(t, ctx, service, start.Job.JobID)
	if status.Status != protocol.MediaJobStatusCancelled {
		t.Fatalf("job status = %#v, want cancelled", status)
	}
	waitForPathAbsent(t, ctx, filepath.Join(root, "Fixture_Movie_1080p.mp4.part"))
}

func containsStringFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func waitForJob(t *testing.T, ctx context.Context, service *Service, jobID string) protocol.MediaDownloadJobStatus {
	t.Helper()
	for {
		response, err := service.Status(ctx, jobID)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		switch response.Job.Status {
		case protocol.MediaJobStatusCompleted, protocol.MediaJobStatusFailed, protocol.MediaJobStatusCancelled:
			return response.Job
		}
		select {
		case <-ctx.Done():
			t.Fatalf("job did not finish: %v", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func waitForPathAbsent(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		} else if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("path still exists: %s", path)
		case <-time.After(20 * time.Millisecond):
		}
	}
}
