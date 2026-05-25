package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	"github.com/dropfile/hankremote/internal/protocol"
	"golang.org/x/net/html"
)

func TestParseSearchResultsAndDetailPages(t *testing.T) {
	t.Parallel()

	results := parseSearchResults(strings.NewReader(`
		<div class="item">
			<a class="movie-card-link" href="/movies/2787-batman-returns"></a>
			<img src="/images/batman-returns.jpg">
			<h2>Batman Returns</h2>
			<p>Action, Fantasy PG-13 7.1 1992 watch</p>
		</div>
		<div class="item">
			<a class="movie-card-link" href="/series/1073-spongebob-squarepants?recent=true&season=1&episode=1"></a>
			<img data-src="https://images.example/spongebob.jpg">
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
	if results[0].PosterURL != "/images/batman-returns.jpg" {
		t.Fatalf("movie poster = %q", results[0].PosterURL)
	}
	if results[1].Type != protocol.MediaTypeSeries || results[1].PagePath != "/series/1073-spongebob-squarepants" || results[1].Year != 1999 {
		t.Fatalf("series result = %#v", results[1])
	}
	if results[1].PosterURL != "https://images.example/spongebob.jpg" {
		t.Fatalf("series poster = %q", results[1].PosterURL)
	}

	page := parseDetailPage(strings.NewReader(`
		<h1>Fixture Show</h1>
		<script>var posterImage = "https://images.example/fixture-show.jpg";</script>
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
	if page.posterURL != "https://images.example/fixture-show.jpg" {
		t.Fatalf("detail poster = %q", page.posterURL)
	}
	if got := chooseDownloadLink(page.downloads); got.quality != "1080p" {
		t.Fatalf("chosen link = %#v, want 1080p", got)
	}
	if len(page.episodes) != 3 || page.episodes[2].season != 2 || page.episodes[2].episode != 1 {
		t.Fatalf("episodes = %#v", page.episodes)
	}
}

func TestParseDetailPageIgnoresPlaceholderDownloadLinks(t *testing.T) {
	t.Parallel()

	page := parseDetailPage(strings.NewReader(`
		<h1>Fixture Movie</h1>
		<a id="dlbtn" href="#">Download 1080p</a>
		<a href="/">Download 1080p</a>
		<a href="javascript:void(0)">Download 720p</a>
		<a href="/assets/poster_1080p.jpg">Download 1080p</a>
	`))
	if len(page.downloads) != 0 {
		t.Fatalf("downloads = %#v, want placeholder links ignored", page.downloads)
	}
	if !page.placeholderButton {
		t.Fatal("placeholderButton = false, want placeholder button detected")
	}
}

func TestParseDetailPageReportsBlockedDownload(t *testing.T) {
	t.Parallel()

	page := parseDetailPage(strings.NewReader(`
		<h1>Fixture Movie</h1>
		<div class="premium-expired"></div>
		<a id="dlbtn" href="#">Download</a>
	`))
	if reason := missingDownloadReason(page); !strings.Contains(reason, "premium") {
		t.Fatalf("missing reason = %q, want premium/expired access reason", reason)
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

func TestSearchAcceptsDirectMediaURL(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var searchEndpointCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/session/logout">Sign out</a>`)
	})
	mux.HandleFunc("/movies/20429-project-hail-mary", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<h1>Project Hail Mary</h1><script>var posterImage = "/posters/project-hail-mary.jpg";</script><a href="https://dl.example/project_hail_mary_1080p.mp4?file=Project_Hail_Mary_1080p.mp4">Download 1080p</a>`)
	})
	mux.HandleFunc("/index/loadmovies", func(w http.ResponseWriter, r *http.Request) {
		searchEndpointCalled = true
		fmt.Fprint(w, `<div></div>`)
	})
	mux.HandleFunc("/index/loadmoviesnew", func(w http.ResponseWriter, r *http.Request) {
		searchEndpointCalled = true
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

	response, err := service.Search(ctx, "https://gramaton.io/movies/20429-project-hail-mary", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if searchEndpointCalled {
		t.Fatal("direct media URL should fetch the detail page without using the search endpoint")
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one direct URL result", response.Results)
	}
	result := response.Results[0]
	if result.Title != "Project Hail Mary" || result.Type != protocol.MediaTypeMovie || result.PagePath != "/movies/20429-project-hail-mary" {
		t.Fatalf("direct result = %#v", result)
	}
	if result.PosterURL != server.URL+"/posters/project-hail-mary.jpg" {
		t.Fatalf("direct poster = %q", result.PosterURL)
	}
}

func TestSearchUsesKnownMediaPathHintWhenProviderSearchIsEmpty(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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
	mux.HandleFunc("/movies/20429-project-hail-mary", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<h1>Project Hail Mary</h1><script>var posterImage = "/posters/project-hail-mary.jpg";</script><a href="https://dl.example/project_hail_mary_1080p.mp4?file=Project_Hail_Mary_2026_1080p.mp4">Download 1080p</a>`)
	})
	mux.HandleFunc("/index/loadmovies", func(w http.ResponseWriter, r *http.Request) {
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

	response, err := service.Search(ctx, "project hail mary", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one path-hint result", response.Results)
	}
	result := response.Results[0]
	if result.Title != "Project Hail Mary" || result.PagePath != "/movies/20429-project-hail-mary" {
		t.Fatalf("path-hint result = %#v", result)
	}
	if result.PosterURL != server.URL+"/posters/project-hail-mary.jpg" {
		t.Fatalf("path-hint poster = %q", result.PosterURL)
	}
}

func TestSearchStopsAfterExactProviderMatch(t *testing.T) {
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
		t.Fatal("series route should not be needed when provider search returns mixed media types")
	})
	mux.HandleFunc("/index/loadmovies", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		query := r.FormValue("q")
		seenQueries = append(seenQueries, query)
		fmt.Fprint(w, `
			<div class="item">
				<a class="movie-card-link" href="/movies/20429-project-hail-mary"></a>
				<h2>Project Hail Mary</h2>
				<p>Drama, Sci-Fi PG-13 7.2 2026 watch</p>
			</div>
		`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	service := New(Config{
		Enabled:  true,
		BaseURL:  server.URL,
		Username: "user@example.com",
		Password: "password",
	}, agentfiles.New(t.TempDir()), nil)

	response, err := service.Search(ctx, "project hail mary", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Title != "Project Hail Mary" {
		t.Fatalf("results = %#v, want exact Project Hail Mary match", response.Results)
	}
	if len(seenQueries) != 1 || seenQueries[0] != "project hail mary" {
		t.Fatalf("seen queries = %#v, want only exact query", seenQueries)
	}
}

func TestSearchUsesCorrectedQueryVariant(t *testing.T) {
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
	mux.HandleFunc("/index/loadmovies", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		query := r.FormValue("q")
		seenQueries = append(seenQueries, query)
		if strings.EqualFold(query, "marshals") {
			fmt.Fprint(w, `
				<div class="item">
					<a class="movie-card-link" href="/series/20242-marshals"></a>
					<h2>Marshals</h2>
					<p>Crime, Drama TV-MA 7.1 2026 watch</p>
				</div>
				<div class="item">
					<a class="movie-card-link" href="/movies/1655-us-marshals"></a>
					<h2>U.S. Marshals</h2>
					<p>Action, Crime PG-13 6.6 1998 watch</p>
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

	response, err := service.Search(ctx, "marshalls", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Title != "Marshals" {
		t.Fatalf("results = %#v, want corrected Marshals match", response.Results)
	}
	if len(seenQueries) != 2 || seenQueries[0] != "marshalls" || seenQueries[1] != "marshals" {
		t.Fatalf("seen queries = %#v, want original then corrected query", seenQueries)
	}
}

func TestPreferExactMediaMatches(t *testing.T) {
	t.Parallel()

	results := []protocol.MediaSearchResult{
		{Title: "Project Hail Mary", Type: protocol.MediaTypeMovie, PagePath: "/movies/20429-project-hail-mary", SearchText: "Project Hail Mary"},
		{Title: "Mother Mary", Type: protocol.MediaTypeMovie, PagePath: "/movies/20443-mother-mary", SearchText: "Mother Mary"},
	}
	filtered := preferExactMediaMatches(results, "project hail mary")
	if len(filtered) != 1 || filtered[0].Title != "Project Hail Mary" {
		t.Fatalf("filtered = %#v, want only exact match", filtered)
	}

	results = []protocol.MediaSearchResult{
		{Title: "Normal", Type: protocol.MediaTypeMovie, PagePath: "/movies/20444-normal", SearchText: "Normal"},
		{Title: "Normal People", Type: protocol.MediaTypeSeries, PagePath: "/series/19709-normal-people", SearchText: "Normal People"},
		{Title: "Paranormal Activity", Type: protocol.MediaTypeMovie, PagePath: "/movies/2965-paranormal-activity", SearchText: "Paranormal Activity"},
	}
	filtered = preferExactMediaMatches(results, "normal movie")
	if len(filtered) != 1 || filtered[0].Title != "Normal" {
		t.Fatalf("normal filtered = %#v, want only exact Normal match", filtered)
	}
	if score := mediaResultScore(results[2], "normal"); score != 0 {
		t.Fatalf("Paranormal score = %d, want 0 for normal word-boundary query", score)
	}

	results = []protocol.MediaSearchResult{
		{Title: "Reminders of Him", Type: protocol.MediaTypeMovie, PagePath: "/movies/20363-reminders-of-him", SearchText: "Reminders of Him"},
		{Title: "HIM", Type: protocol.MediaTypeMovie, PagePath: "/movies/19922-him", SearchText: "HIM"},
	}
	filtered = preferExactMediaMatches(results, "reminder of him")
	if len(filtered) != 1 || filtered[0].Title != "Reminders of Him" {
		t.Fatalf("reminder filtered = %#v, want plural-tolerant exact match", filtered)
	}
}

func TestLiveMediaSearchFromEnv(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE")), "true") {
		t.Skip("set HANK_REMOTE_MEDIA_LIVE=true with media source env vars to run live diagnostics")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	service := liveMediaServiceFromEnv(t)
	if err := service.ensureAuthenticated(ctx); err != nil {
		t.Fatalf("authenticate media source: %v", err)
	}

	requireResults := !strings.EqualFold(strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE_REQUIRE_RESULTS")), "false")
	for _, query := range liveMediaQueries() {
		runLiveMediaSearch(t, ctx, service, query, requireResults)
	}
}

func TestLiveMediaCatalogFromEnv(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE")), "true") {
		t.Skip("set HANK_REMOTE_MEDIA_LIVE=true with media source env vars to run live diagnostics")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	service := liveMediaServiceFromEnv(t)
	if err := service.ensureAuthenticated(ctx); err != nil {
		t.Fatalf("authenticate media source: %v", err)
	}

	limit := liveMediaSampleLimit()
	for _, sample := range []struct {
		route     string
		mediaType string
	}{
		{route: "/movies", mediaType: protocol.MediaTypeMovie},
		{route: "/series", mediaType: protocol.MediaTypeSeries},
	} {
		body, status, err := service.fetchText(ctx, http.MethodGet, sample.route, nil)
		if err != nil {
			t.Logf("catalog route=%s error=%v", sample.route, err)
			continue
		}
		results := parseSearchResults(strings.NewReader(body), sample.mediaType)
		t.Logf("catalog route=%s status=%d body_len=%d parsed=%d login_marker=%v", sample.route, status, len(body), len(results), looksLikeLoginPage(body))
		for index, result := range results {
			if index >= limit {
				break
			}
			t.Logf("  catalog[%d] title=%q year=%d type=%s path=%q poster=%q summary=%q", index+1, result.Title, result.Year, result.Type, result.PagePath, result.PosterURL, result.Summary)
		}
	}
}

func TestLiveMediaDownloadProbeFromEnv(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE_DOWNLOAD_PROBE")), "true") {
		t.Skip("set HANK_REMOTE_MEDIA_LIVE_DOWNLOAD_PROBE=true with media source env vars to probe a live download")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	service := liveMediaServiceFromEnv(t)
	if err := service.ensureAuthenticated(ctx); err != nil {
		t.Fatalf("authenticate media source: %v", err)
	}

	query := strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE_QUERY"))
	if query == "" {
		query = "project hail mary"
	}
	search, err := service.Search(ctx, query, 1)
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	if len(search.Results) == 0 {
		t.Fatalf("search %q returned no results", query)
	}
	plan, downloads, err := service.buildPlan(ctx, search.Results[0])
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	logLiveDownloadAnchors(t, ctx, service, search.Results[0].PagePath)
	if len(downloads) == 0 || downloads[0].downloadURL == "" {
		t.Fatalf("plan has no downloadable item: %#v", plan)
	}

	download := downloads[0]
	t.Logf("selected title=%q type=%s quality=%s filename=%q page=%q download_url=%q",
		plan.Selection.Title,
		download.item.MediaType,
		download.item.Quality,
		download.item.Filename,
		download.item.PagePath,
		redactedURLForLog(download.downloadURL),
	)

	req, err := service.newDownloadRequest(ctx, download)
	if err != nil {
		t.Fatalf("new download request: %v", err)
	}
	req.Header.Set("Range", "bytes=0-4095")
	resp, err := service.client.Do(req)
	if err != nil {
		t.Fatalf("probe request: %v", err)
	}
	defer resp.Body.Close()

	sniff, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr != nil {
		t.Fatalf("read probe body: %v", readErr)
	}
	t.Logf("probe status=%d final_url=%q content_type=%q content_length=%d content_range=%q accept_ranges=%q sniff=%q",
		resp.StatusCode,
		redactedURLForLog(resp.Request.URL.String()),
		resp.Header.Get("Content-Type"),
		resp.ContentLength,
		resp.Header.Get("Content-Range"),
		resp.Header.Get("Accept-Ranges"),
		truncateForLog(cleanText(string(sniff)), 240),
	)
	if err := validateDownloadResponse(resp, sniff); err != nil {
		t.Fatalf("download probe rejected response: %v", err)
	}
}

func logLiveDownloadAnchors(t *testing.T, ctx context.Context, service *Service, pagePath string) {
	t.Helper()

	body, status, err := service.fetchText(ctx, http.MethodGet, pagePath, nil)
	if err != nil {
		t.Logf("detail page fetch error: %v", err)
		return
	}
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Logf("detail page parse error: %v", err)
		return
	}
	t.Logf("detail page status=%d body_len=%d", status, len(body))
	t.Logf("detail page premium_or_expired_overlay=%v", pageHasDownloadAccessOverlay(root))
	for _, node := range findAll(root, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "a" && attr(node, "href") != ""
	}) {
		href := attr(node, "href")
		text := cleanText(nodeText(node))
		quality := linkQuality(text, href)
		loweredText := strings.ToLower(text)
		loweredHref := strings.ToLower(href)
		if quality == "" &&
			!strings.Contains(loweredText, "download") &&
			!strings.Contains(loweredHref, "download") &&
			!strings.Contains(loweredHref, "/dl") {
			continue
		}
		t.Logf("detail anchor href=%q quality=%q text=%q", redactedURLForLog(href), quality, truncateForLog(text, 240))
	}
}

func runLiveMediaSearch(t *testing.T, ctx context.Context, service *Service, query string, requireResults bool) {
	t.Helper()

	variants := mediaSearchQueries(query)
	t.Logf("live media search query=%q variants=%q", query, variants)
	var rawResults []protocol.MediaSearchResult
	for _, mediaType := range []string{protocol.MediaTypeMovie, protocol.MediaTypeSeries} {
		for _, variant := range variants {
			if os.Getenv("HANK_REMOTE_MEDIA_LIVE_DEBUG") == "true" {
				logLiveSearchFetch(t, ctx, service, mediaType, variant)
			}
			found, err := service.searchType(ctx, mediaType, variant)
			if err != nil {
				t.Logf("provider search type=%s query=%q error=%v", mediaType, variant, err)
				continue
			}
			t.Logf("provider search type=%s query=%q parsed=%d", mediaType, variant, len(found))
			for _, result := range found {
				score := mediaResultScore(result, query)
				t.Logf("  score=%d title=%q year=%d type=%s path=%q poster=%q", score, result.Title, result.Year, result.Type, result.PagePath, result.PosterURL)
			}
			rawResults = append(rawResults, found...)
		}
	}

	unique := uniqueSearchResults(rawResults)
	t.Logf("raw_unique=%d", len(unique))
	for _, result := range unique {
		t.Logf("  unique score=%d title=%q year=%d type=%s path=%q poster=%q", mediaResultScore(result, query), result.Title, result.Year, result.Type, result.PagePath, result.PosterURL)
	}

	response, err := service.Search(ctx, query, 10)
	if err != nil {
		t.Fatalf("live media search: %v", err)
	}
	t.Logf("filtered=%d", len(response.Results))
	for _, result := range response.Results {
		t.Logf("  filtered score=%d title=%q year=%d type=%s path=%q poster=%q", mediaResultScore(result, query), result.Title, result.Year, result.Type, result.PagePath, result.PosterURL)
	}
	if requireResults && len(response.Results) == 0 {
		t.Fatalf("live search returned no filtered results for %q; inspect provider search and parsed result logs above", query)
	}
}

func liveMediaServiceFromEnv(t *testing.T) *Service {
	t.Helper()

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_GRAMATON_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://gramaton.io"
	}
	username := strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_GRAMATON_USERNAME"))
	password := os.Getenv("HANK_REMOTE_MEDIA_GRAMATON_PASSWORD")
	if username == "" || password == "" {
		t.Fatal("set HANK_REMOTE_MEDIA_GRAMATON_USERNAME and HANK_REMOTE_MEDIA_GRAMATON_PASSWORD to run live diagnostics")
	}
	return New(Config{
		Enabled:  true,
		BaseURL:  baseURL,
		Username: username,
		Password: password,
	}, agentfiles.New(t.TempDir()), nil)
}

func liveMediaQueries() []string {
	values := splitLiveValues(os.Getenv("HANK_REMOTE_MEDIA_LIVE_QUERIES"))
	if len(values) > 0 {
		return values
	}
	query := strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE_QUERY"))
	if query == "" {
		query = "project hail mary"
	}
	return []string{query}
}

func splitLiveValues(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			values = append(values, field)
		}
	}
	return values
}

func liveMediaSampleLimit() int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_LIVE_SAMPLE_LIMIT")))
	if err != nil || value <= 0 {
		return 12
	}
	if value > 50 {
		return 50
	}
	return value
}

func looksLikeLoginPage(body string) bool {
	lowered := strings.ToLower(body)
	return strings.Contains(lowered, "userlogin") ||
		strings.Contains(lowered, "forgotpassword") ||
		(strings.Contains(lowered, "password") && strings.Contains(lowered, "login"))
}

func redactedURLForLog(rawURL string) string {
	if strings.HasPrefix(strings.TrimSpace(rawURL), "#") {
		return "#"
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "(invalid)"
	}
	if parsed.Scheme == "" && parsed.Host == "" {
		if parsed.Path == "" && parsed.Fragment != "" {
			return "#"
		}
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func truncateForLog(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func logLiveSearchFetch(t *testing.T, ctx context.Context, service *Service, mediaType string, query string) {
	t.Helper()

	route := searchRouteByType[mediaType]
	endpoint := searchEndpointByType[mediaType]
	paramType := searchParamTypeByType[mediaType]
	token, tokenErr := service.pageToken(ctx, route)
	values := url.Values{}
	values.Set("loadmovies", "showData")
	values.Set("page", "1")
	values.Set("abc", "All")
	values.Set("genres", "")
	values.Set("sortby", "Recent")
	values.Set("quality", "All")
	values.Set("type", paramType)
	values.Set("q", query)
	values.Set("search", query)
	values.Set("token", token)

	body, status, err := service.fetchSearchText(ctx, route, endpoint, values)
	t.Logf("provider raw type=%s endpoint=%s query=%q token_present=%v token_error=%v status=%d body_len=%d error=%v", mediaType, endpoint, query, token != "", tokenErr, status, len(body), err)
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

func TestMediaDownloadRejectsHTMLResponse(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/session/logout">Sign out</a>`)
	})
	mux.HandleFunc("/movies/html-error", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<h1>Fixture Movie</h1><a href="%s/download/html_1080p.mp4?file=Fixture_Movie_1080p.mp4">Download 1080p</a>`, baseURL)
	})
	mux.HandleFunc("/download/html_1080p.mp4", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body><form><input name="password"></form>login</body></html>`)
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
		ID:       "movies/html-error",
		Title:    "Fixture Movie",
		Type:     protocol.MediaTypeMovie,
		PagePath: "/movies/html-error",
	}})
	if err != nil {
		t.Fatalf("StartDownload: %v", err)
	}
	status := waitForJob(t, ctx, service, start.Job.JobID)
	if status.Status != protocol.MediaJobStatusFailed || status.FailedCount != 1 || status.CompletedCount != 0 {
		t.Fatalf("job status = %#v, want failed HTML download", status)
	}
	if !strings.Contains(status.ErrorMessage, "HTML") && !strings.Contains(status.ErrorMessage, "web page") {
		t.Fatalf("error message = %q, want HTML/web page failure", status.ErrorMessage)
	}
	if _, err := os.Stat(filepath.Join(root, "Fixture_Movie_1080p.mp4")); !os.IsNotExist(err) {
		t.Fatalf("final file exists or unexpected error: %v", err)
	}
}

func TestMoviePlanUsesDynamic1080pLink(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var sawDynamicRequest bool
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/session/logout">Sign out</a>`)
	})
	mux.HandleFunc("/movies/20429-project-hail-mary", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `
			<h1>Project Hail Mary</h1>
			<script>var token_key = "dynamic-token";</script>
			<a id="dlbtn" href="#">Download 720p</a>
			<a id="dlbtn-hd" href="#">Download 1080p</a>
		`)
	})
	mux.HandleFunc("/movies/getMovieLink", func(w http.ResponseWriter, r *http.Request) {
		sawDynamicRequest = true
		if r.URL.Query().Get("id") != "20429" || r.URL.Query().Get("token") != "dynamic-token" {
			t.Fatalf("dynamic query = %s, want id and token", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"dl":"%s/download/web_720p.mp4?file=Project_Hail_Mary_720p.mp4","dl_hd":"%s/download/web_1080p.mp4?file=Project_Hail_Mary_1080p.mp4"}`, baseURL, baseURL)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	baseURL = server.URL

	service := New(Config{
		Enabled:  true,
		BaseURL:  server.URL,
		Username: "user@example.com",
		Password: "password",
	}, agentfiles.New(t.TempDir()), nil)

	response, err := service.PlanDownload(ctx, protocol.MediaPlanDownloadRequest{Selection: protocol.MediaSearchResult{
		ID:       "movies/20429-project-hail-mary",
		Title:    "Project Hail Mary",
		Type:     protocol.MediaTypeMovie,
		PagePath: "/movies/20429-project-hail-mary",
	}})
	if err != nil {
		t.Fatalf("PlanDownload: %v", err)
	}
	if !sawDynamicRequest {
		t.Fatal("dynamic movie link endpoint was not called")
	}
	if len(response.Plan.Items) != 1 {
		t.Fatalf("items = %#v, want one", response.Plan.Items)
	}
	item := response.Plan.Items[0]
	if !item.DownloadOK || item.Quality != "1080p" || item.Filename != "Project_Hail_Mary_1080p.mp4" {
		t.Fatalf("dynamic item = %#v, want 1080p dynamic link item", item)
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
