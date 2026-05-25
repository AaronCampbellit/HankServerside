package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	"github.com/dropfile/hankremote/internal/protocol"
	"golang.org/x/net/html"
)

const (
	preferredQuality      = "1080p"
	downloadSniffByteSize = 4096
)

var (
	errDisabled        = errors.New("media source is not configured")
	yearPattern        = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	tokenPattern       = regexp.MustCompile(`token_key\s*=\s*["']([^"']+)["']`)
	posterImagePattern = regexp.MustCompile(`(?i)\bposterImage\s*=\s*["']([^"']+)["']`)
	seasonIDPattern    = regexp.MustCompile(`(?i)^season(\d+)$`)
	qualityPattern     = regexp.MustCompile(`(?i)(720|1080)p`)
	unsafeNameRunes    = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)
	spacingPattern     = regexp.MustCompile(`\s+`)
	mediaPathPattern   = regexp.MustCompile(`^/(movies|series)/([^/?#]+)`)
	mediaPageIDPattern = regexp.MustCompile(`^/(movies|series)/(\d+)`)
	searchRouteByType  = map[string]string{
		protocol.MediaTypeMovie:  "/movies",
		protocol.MediaTypeSeries: "/series",
	}
	searchEndpointByType = map[string]string{
		protocol.MediaTypeMovie:  "/index/loadmovies",
		protocol.MediaTypeSeries: "/index/loadmovies",
	}
	mediaPathHintsByQuery = map[string][]string{
		"project hail mary": {"/movies/20429-project-hail-mary"},
	}
	searchParamTypeByType = map[string]string{
		protocol.MediaTypeMovie:  "movie",
		protocol.MediaTypeSeries: "tv",
	}
	mediaSearchStopWords = map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "available": {}, "availabe": {}, "download": {}, "downloads": {},
		"episode": {}, "episodes": {}, "film": {}, "films": {}, "for": {}, "in": {}, "movie": {},
		"movies": {}, "of": {}, "on": {}, "or": {}, "season": {}, "seasons": {}, "series": {},
		"show": {}, "shows": {}, "the": {}, "to": {}, "tv": {}, "with": {},
	}
	mediaSearchTokenCorrections = map[string]string{
		"marshalls": "marshals",
	}
)

type Config struct {
	Enabled         bool
	BaseURL         string
	Username        string
	Password        string
	DestinationPath string
	EnvPath         string
}

type EventSink func(ctx context.Context, event string, topic string, payload any) error

type Service struct {
	cfg    Config
	files  *agentfiles.Service
	client *http.Client
	logger *slog.Logger

	mu            sync.Mutex
	authenticated bool
	eventSink     EventSink
	jobs          map[string]*downloadJob
}

type plannedDownload struct {
	item        protocol.MediaDownloadItem
	downloadURL string
}

type movieLinkPayload struct {
	Download   string `json:"dl"`
	DownloadHD string `json:"dl_hd"`
}

type downloadJob struct {
	mu     sync.Mutex
	status protocol.MediaDownloadJobStatus
	cancel context.CancelFunc
}

func New(cfg Config, files *agentfiles.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if files == nil {
		files = agentfiles.New("")
	}
	jar, _ := cookiejar.New(nil)
	return &Service{
		cfg: Config{
			Enabled:         cfg.Enabled,
			BaseURL:         strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
			Username:        strings.TrimSpace(cfg.Username),
			Password:        cfg.Password,
			DestinationPath: cleanSharePath(cfg.DestinationPath),
			EnvPath:         strings.TrimSpace(cfg.EnvPath),
		},
		files:  files,
		client: &http.Client{Jar: jar},
		logger: logger,
		jobs:   make(map[string]*downloadJob),
	}
}

func (s *Service) SetEventSink(sink EventSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventSink = sink
}

func (s *Service) Enabled() bool {
	cfg := s.configSnapshot()
	return cfg.Enabled &&
		cfg.BaseURL != "" &&
		cfg.Username != "" &&
		cfg.Password != "" &&
		s.files != nil &&
		s.files.Enabled()
}

func (s *Service) Settings(_ context.Context) protocol.MediaSettingsStatusResponse {
	return protocol.MediaSettingsStatusResponse{
		Settings: s.settingsSnapshot(),
		Jobs:     s.jobSnapshots(),
	}
}

func (s *Service) ApplySettings(_ context.Context, request protocol.MediaSettingsApplyRequest) (protocol.MediaSettingsApplyResponse, error) {
	settings := request.Settings
	s.mu.Lock()
	cfg := s.cfg
	cfg.Enabled = settings.Enabled
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(firstNonBlank(settings.BaseURL, cfg.BaseURL)), "/")
	cfg.Username = strings.TrimSpace(settings.Username)
	cfg.DestinationPath = cleanSharePath(settings.DestinationPath)
	if request.Password != "" {
		cfg.Password = request.Password
	}
	s.cfg = cfg
	s.authenticated = false
	s.mu.Unlock()

	if request.Persist {
		if err := s.persistSettings(cfg); err != nil {
			return protocol.MediaSettingsApplyResponse{}, err
		}
	}
	return protocol.MediaSettingsApplyResponse{Settings: s.settingsSnapshot()}, nil
}

func (s *Service) Jobs(_ context.Context) protocol.MediaDownloadJobsResponse {
	return protocol.MediaDownloadJobsResponse{Jobs: s.jobSnapshots()}
}

func (s *Service) Cancel(ctx context.Context, jobID string) (protocol.MediaDownloadCancelResponse, error) {
	s.mu.Lock()
	job := s.jobs[strings.TrimSpace(jobID)]
	s.mu.Unlock()
	if job == nil {
		return protocol.MediaDownloadCancelResponse{}, fmt.Errorf("media download job not found")
	}
	job.mu.Lock()
	if job.cancel != nil {
		job.cancel()
	}
	if job.status.Status == protocol.MediaJobStatusQueued || job.status.Status == protocol.MediaJobStatusRunning {
		now := time.Now().UTC()
		job.status.Status = protocol.MediaJobStatusCancelled
		job.status.CompletedAt = now
		job.status.ErrorMessage = "Cancelled by user."
	}
	status := job.status
	job.mu.Unlock()
	s.emitJob(ctx, "media.download_completed", job)
	return protocol.MediaDownloadCancelResponse{Job: status}, nil
}

func (s *Service) configSnapshot() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Service) settingsSnapshot() protocol.MediaSettings {
	cfg := s.configSnapshot()
	return protocol.MediaSettings{
		Enabled:             cfg.Enabled,
		BaseURL:             cfg.BaseURL,
		Username:            cfg.Username,
		HasPassword:         cfg.Password != "",
		DestinationPath:     cfg.DestinationPath,
		PreferredQuality:    preferredQuality,
		RequireConfirmation: true,
	}
}

func (s *Service) jobSnapshots() []protocol.MediaDownloadJobStatus {
	s.mu.Lock()
	jobs := make([]*downloadJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	s.mu.Unlock()

	statuses := make([]protocol.MediaDownloadJobStatus, 0, len(jobs))
	for _, job := range jobs {
		statuses = append(statuses, job.snapshot())
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].StartedAt.After(statuses[j].StartedAt)
	})
	return statuses
}

func (s *Service) Search(ctx context.Context, query string, limit int) (protocol.MediaSearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return protocol.MediaSearchResponse{Query: query}, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	if err := s.ensureAuthenticated(ctx); err != nil {
		return protocol.MediaSearchResponse{}, err
	}
	if result, ok, err := s.searchDirectMediaPath(ctx, query); ok || err != nil {
		if err != nil {
			return protocol.MediaSearchResponse{}, err
		}
		return protocol.MediaSearchResponse{Query: query, Results: []protocol.MediaSearchResult{result}}, nil
	}

	searchQueries := mediaSearchQueries(query)
	var results []protocol.MediaSearchResult
	for _, searchQuery := range searchQueries {
		found, err := s.searchType(ctx, protocol.MediaTypeMovie, searchQuery)
		if err != nil {
			s.logger.Debug("media source search failed", "type", protocol.MediaTypeMovie, "query", searchQuery, "error", err)
			continue
		}
		results = append(results, found...)
		if exact := exactMediaMatches(uniqueSearchResults(results), query); len(exact) > 0 {
			if len(exact) > limit {
				exact = exact[:limit]
			}
			return protocol.MediaSearchResponse{Query: query, Results: exact}, nil
		}
	}
	results = uniqueSearchResults(results)
	sort.Slice(results, func(i, j int) bool {
		left := mediaResultScore(results[i], query)
		right := mediaResultScore(results[j], query)
		if left == right {
			return results[i].Title < results[j].Title
		}
		return left > right
	})
	filtered := results[:0]
	for _, result := range results {
		if mediaResultScore(result, query) > 0 {
			filtered = append(filtered, result)
		}
	}
	filtered = preferExactMediaMatches(filtered, query)
	if len(filtered) == 0 {
		filtered = s.searchMediaPathHints(ctx, query)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return protocol.MediaSearchResponse{Query: query, Results: filtered}, nil
}

func preferExactMediaMatches(results []protocol.MediaSearchResult, query string) []protocol.MediaSearchResult {
	exact := exactMediaMatches(results, query)
	if len(exact) == 0 {
		return results
	}
	return exact
}

func exactMediaMatches(results []protocol.MediaSearchResult, query string) []protocol.MediaSearchResult {
	exact := results[:0]
	for _, result := range results {
		if mediaResultExactTitleMatch(result, query) {
			exact = append(exact, result)
		}
	}
	return exact
}

func mediaResultExactTitleMatch(result protocol.MediaSearchResult, query string) bool {
	title := singularizeNormalizedMediaTitle(canonicalizeMediaSearchTokens(normalizeForMatch(stripTrailingYear(result.Title))))
	query = singularizeNormalizedMediaTitle(stripTrailingMediaTypeHint(canonicalizeMediaSearchTokens(normalizeForMatch(stripTrailingYear(query)))))
	if title == "" || query == "" {
		return false
	}
	if title == query {
		return true
	}
	return stripLeadingNormalizedArticle(title) == stripLeadingNormalizedArticle(query)
}

func stripTrailingMediaTypeHint(query string) string {
	tokens := strings.Fields(query)
	for len(tokens) > 1 && mediaTypeHintToken(tokens[len(tokens)-1]) {
		tokens = tokens[:len(tokens)-1]
	}
	return strings.Join(tokens, " ")
}

func stripLeadingNormalizedArticle(value string) string {
	tokens := strings.Fields(value)
	for len(tokens) > 1 {
		switch tokens[0] {
		case "a", "an", "the":
			tokens = tokens[1:]
		default:
			return strings.Join(tokens, " ")
		}
	}
	return strings.Join(tokens, " ")
}

func mediaTypeHintToken(token string) bool {
	switch token {
	case "movie", "movies", "film", "films", "series", "show", "shows", "tv", "season", "seasons", "episode", "episodes":
		return true
	default:
		return false
	}
}

func canonicalizeMediaSearchTokens(value string) string {
	tokens := strings.Fields(value)
	for index, token := range tokens {
		if corrected, ok := mediaSearchTokenCorrections[token]; ok {
			tokens[index] = corrected
		}
	}
	return strings.Join(tokens, " ")
}

func singularizeNormalizedMediaTitle(value string) string {
	tokens := strings.Fields(value)
	for index, token := range tokens {
		if len(token) > 3 && strings.HasSuffix(token, "s") && !strings.HasSuffix(token, "ss") {
			tokens[index] = strings.TrimSuffix(token, "s")
		}
	}
	return strings.Join(tokens, " ")
}

func (s *Service) searchMediaPathHints(ctx context.Context, query string) []protocol.MediaSearchResult {
	paths := mediaPathHintCandidates(query)
	if len(paths) == 0 {
		return nil
	}
	results := make([]protocol.MediaSearchResult, 0, len(paths))
	for _, pagePath := range paths {
		result, ok, err := s.searchDirectMediaPath(ctx, pagePath)
		if err != nil {
			s.logger.Debug("media path hint failed", "query", query, "path", pagePath, "error", err)
			continue
		}
		if ok && mediaResultScore(result, query) > 0 {
			results = append(results, result)
		}
	}
	return uniqueSearchResults(results)
}

func mediaPathHintCandidates(query string) []string {
	key := normalizeForMatch(stripTrailingYear(query))
	if key == "" {
		return nil
	}
	return mediaPathHintsByQuery[key]
}

func (s *Service) searchDirectMediaPath(ctx context.Context, query string) (protocol.MediaSearchResult, bool, error) {
	pagePath := canonicalMediaPath(query)
	if pagePath == "" {
		return protocol.MediaSearchResult{}, false, nil
	}
	page, err := s.fetchPage(ctx, pagePath)
	if err != nil {
		return protocol.MediaSearchResult{}, true, err
	}
	title := firstNonBlank(pageTitle(page), mediaTitleFromPath(pagePath))
	result := protocol.MediaSearchResult{
		ID:         mediaID(pagePath),
		Title:      title,
		Year:       parseYear(title),
		Type:       mediaTypeFromPath(pagePath),
		PosterURL:  s.resolveMediaAssetURL(page.posterURL),
		PagePath:   pagePath,
		SearchText: cleanText(title + " " + pagePath),
	}
	return result, true, nil
}

func (s *Service) PlanDownload(ctx context.Context, request protocol.MediaPlanDownloadRequest) (protocol.MediaPlanDownloadResponse, error) {
	if err := s.ensureAuthenticated(ctx); err != nil {
		return protocol.MediaPlanDownloadResponse{}, err
	}
	plan, _, err := s.buildPlan(ctx, request.Selection)
	if err != nil {
		return protocol.MediaPlanDownloadResponse{}, err
	}
	return protocol.MediaPlanDownloadResponse{Plan: plan}, nil
}

func (s *Service) StartDownload(ctx context.Context, request protocol.MediaDownloadStartRequest) (protocol.MediaDownloadStartResponse, error) {
	if err := s.ensureAuthenticated(ctx); err != nil {
		return protocol.MediaDownloadStartResponse{}, err
	}
	plan, downloads, err := s.buildPlan(ctx, request.Selection)
	if err != nil {
		return protocol.MediaDownloadStartResponse{}, err
	}
	jobID := newJobID()
	now := time.Now().UTC()
	jobCtx, cancel := context.WithCancel(context.Background())
	job := &downloadJob{status: protocol.MediaDownloadJobStatus{
		JobID:      jobID,
		Status:     protocol.MediaJobStatusQueued,
		Title:      firstNonBlank(plan.Selection.Title, request.Selection.Title),
		TotalCount: len(downloads),
		StartedAt:  now,
	}, cancel: cancel}
	s.mu.Lock()
	s.jobs[jobID] = job
	s.mu.Unlock()

	go s.runDownloadJob(jobCtx, job, downloads)
	return protocol.MediaDownloadStartResponse{Job: job.snapshot()}, nil
}

func (s *Service) Status(_ context.Context, jobID string) (protocol.MediaDownloadStatusResponse, error) {
	s.mu.Lock()
	job := s.jobs[strings.TrimSpace(jobID)]
	s.mu.Unlock()
	if job == nil {
		return protocol.MediaDownloadStatusResponse{}, fmt.Errorf("media download job not found")
	}
	return protocol.MediaDownloadStatusResponse{Job: job.snapshot()}, nil
}

func (s *Service) ensureAuthenticated(ctx context.Context) error {
	if !s.Enabled() {
		return errDisabled
	}
	s.mu.Lock()
	if s.authenticated {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	body, status, err := s.fetchText(ctx, http.MethodGet, "/", nil)
	if err == nil && status < 400 && strings.Contains(body, "/session/logout") {
		s.mu.Lock()
		s.authenticated = true
		s.mu.Unlock()
		return nil
	}

	if err := s.login(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	s.authenticated = true
	s.mu.Unlock()
	return nil
}

func (s *Service) login(ctx context.Context) error {
	cfg := s.configSnapshot()
	formBody, status, err := s.fetchText(ctx, http.MethodGet, "/session/login", nil)
	if err != nil || status == http.StatusNotFound {
		formBody, status, err = s.fetchText(ctx, http.MethodGet, "/login", nil)
	}
	if err != nil || status >= 500 {
		return fmt.Errorf("media source login page unavailable")
	}

	action, values := parseLoginForm(formBody)
	if action == "" {
		action = "/session/login"
	}
	if !hasAnyKey(values, "email", "username", "login") {
		values.Set("email", cfg.Username)
	}
	if _, ok := values["password"]; !ok {
		values.Set("password", cfg.Password)
	}
	for key := range values {
		lowered := strings.ToLower(key)
		if strings.Contains(lowered, "user") || strings.Contains(lowered, "email") || strings.Contains(lowered, "login") {
			values.Set(key, cfg.Username)
		}
		if strings.Contains(lowered, "pass") {
			values.Set(key, cfg.Password)
		}
	}

	body, status, err := s.fetchText(ctx, http.MethodPost, action, values)
	if err != nil {
		return fmt.Errorf("media source login failed: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("media source login failed")
	}
	if strings.Contains(strings.ToLower(body), "captcha") {
		return fmt.Errorf("media source requires an interactive challenge")
	}
	return nil
}

func (s *Service) searchType(ctx context.Context, mediaType string, query string) ([]protocol.MediaSearchResult, error) {
	route := searchRouteByType[mediaType]
	endpoint := searchEndpointByType[mediaType]
	paramType := searchParamTypeByType[mediaType]
	if route == "" || endpoint == "" || paramType == "" {
		return nil, nil
	}
	token, _ := s.pageToken(ctx, route)
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

	body, status, err := s.fetchSearchText(ctx, route, endpoint, values)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("media search returned status %d", status)
	}
	results := parseSearchResults(strings.NewReader(body), mediaType)
	s.resolveMediaResultAssets(results)
	return results, nil
}

func (s *Service) fetchSearchText(ctx context.Context, route string, endpoint string, form url.Values) (string, int, error) {
	target, err := s.resolveURL(endpoint)
	if err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 HankRemoteMedia/1.0")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if referer, err := s.resolveURL(route); err == nil {
		req.Header.Set("Referer", referer)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(data), resp.StatusCode, nil
}

func (s *Service) pageToken(ctx context.Context, path string) (string, error) {
	body, status, err := s.fetchText(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	if status >= 400 {
		return "", fmt.Errorf("media source page returned status %d", status)
	}
	match := tokenPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		return "", nil
	}
	return match[1], nil
}

func (s *Service) buildPlan(ctx context.Context, selection protocol.MediaSearchResult) (protocol.MediaDownloadPlan, []plannedDownload, error) {
	selection.PagePath = canonicalMediaPath(selection.PagePath)
	if selection.PagePath == "" {
		return protocol.MediaDownloadPlan{}, nil, fmt.Errorf("media selection is missing a page path")
	}
	if selection.Type == "" {
		selection.Type = mediaTypeFromPath(selection.PagePath)
	}
	switch selection.Type {
	case protocol.MediaTypeMovie:
		return s.buildMoviePlan(ctx, selection)
	case protocol.MediaTypeSeries:
		return s.buildSeriesPlan(ctx, selection)
	default:
		return protocol.MediaDownloadPlan{}, nil, fmt.Errorf("unsupported media type %q", selection.Type)
	}
}

func (s *Service) buildMoviePlan(ctx context.Context, selection protocol.MediaSearchResult) (protocol.MediaDownloadPlan, []plannedDownload, error) {
	page, err := s.fetchPage(ctx, selection.PagePath)
	if err != nil {
		return protocol.MediaDownloadPlan{}, nil, err
	}
	title := firstNonBlank(selection.Title, pageTitle(page), mediaTitleFromPath(selection.PagePath))
	selection.Title = title
	if selection.PosterURL == "" {
		selection.PosterURL = s.resolveMediaAssetURL(page.posterURL)
	}
	link := chooseDownloadLink(page.downloads)
	if link.href == "" {
		link = s.dynamicMovieDownloadLink(ctx, selection.PagePath, page.token)
	}
	downloadURL := ""
	if link.href != "" {
		downloadURL, _ = s.resolveURL(link.href)
	}
	item := protocol.MediaDownloadItem{
		ID:         selection.ID,
		Title:      title,
		MediaType:  protocol.MediaTypeMovie,
		Quality:    link.quality,
		Filename:   downloadFilename(link.href, title, 0, 0, link.quality),
		PagePath:   selection.PagePath,
		DownloadOK: downloadURL != "",
	}
	if downloadURL == "" {
		item.ErrorReason = missingDownloadReason(page)
	}
	item.Existing = s.fileExists(ctx, item.Filename)
	downloads := []plannedDownload{{item: item, downloadURL: downloadURL}}
	return s.planFromDownloads(selection, downloads), downloads, nil
}

func (s *Service) dynamicMovieDownloadLink(ctx context.Context, pagePath string, token string) downloadLink {
	id := mediaPageID(pagePath)
	if id == "" {
		return downloadLink{}
	}
	if token == "" {
		token, _ = s.pageToken(ctx, pagePath)
	}
	if token == "" {
		return downloadLink{}
	}
	values := url.Values{}
	values.Set("id", id)
	values.Set("token", token)
	values.Set("oPid", "")
	body, status, err := s.fetchText(ctx, http.MethodGet, "/movies/getMovieLink?"+values.Encode(), nil)
	if err != nil || status >= 400 {
		if err != nil {
			s.logger.Debug("dynamic movie download link failed", "page", pagePath, "error", err)
		} else {
			s.logger.Debug("dynamic movie download link returned status", "page", pagePath, "status", status)
		}
		return downloadLink{}
	}
	var payload movieLinkPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		s.logger.Debug("dynamic movie download link JSON decode failed", "page", pagePath, "error", err)
		return downloadLink{}
	}
	if href := strings.TrimSpace(payload.DownloadHD); href != "" {
		return downloadLink{href: href, quality: "1080p"}
	}
	if href := strings.TrimSpace(payload.Download); href != "" {
		return downloadLink{href: href, quality: "720p"}
	}
	return downloadLink{}
}

func (s *Service) buildSeriesPlan(ctx context.Context, selection protocol.MediaSearchResult) (protocol.MediaDownloadPlan, []plannedDownload, error) {
	page, err := s.fetchPage(ctx, selection.PagePath)
	if err != nil {
		return protocol.MediaDownloadPlan{}, nil, err
	}
	title := firstNonBlank(selection.Title, pageTitle(page), mediaTitleFromPath(selection.PagePath))
	selection.Title = title
	if selection.PosterURL == "" {
		selection.PosterURL = s.resolveMediaAssetURL(page.posterURL)
	}
	episodes := page.episodes
	if len(episodes) == 0 {
		return protocol.MediaDownloadPlan{}, nil, fmt.Errorf("no episodes were found on the series page")
	}
	sort.Slice(episodes, func(i, j int) bool {
		if episodes[i].season == episodes[j].season {
			return episodes[i].episode < episodes[j].episode
		}
		return episodes[i].season < episodes[j].season
	})

	downloads := make([]plannedDownload, 0, len(episodes))
	for _, episode := range episodes {
		pagePath := fmt.Sprintf("%s?season=%d&episode=%d", selection.PagePath, episode.season, episode.episode)
		episodePage, err := s.fetchPage(ctx, pagePath)
		itemTitle := firstNonBlank(episode.title, fmt.Sprintf("S%dE%d", episode.season, episode.episode))
		item := protocol.MediaDownloadItem{
			ID:        fmt.Sprintf("%s:s%d:e%d", selection.ID, episode.season, episode.episode),
			Title:     itemTitle,
			MediaType: protocol.MediaTypeSeries,
			Season:    episode.season,
			Episode:   episode.episode,
			PagePath:  pagePath,
		}
		var link downloadLink
		if err == nil {
			link = chooseDownloadLink(episodePage.downloads)
			downloadURL := ""
			if link.href != "" {
				downloadURL, _ = s.resolveURL(link.href)
			}
			item.Quality = link.quality
			item.Filename = downloadFilename(link.href, title, episode.season, episode.episode, link.quality)
			item.DownloadOK = downloadURL != ""
			link.href = downloadURL
		}
		if err != nil {
			item.Filename = episodeFilename(title, episode.season, episode.episode, "")
			item.ErrorReason = err.Error()
		} else if link.href == "" {
			item.Filename = episodeFilename(title, episode.season, episode.episode, "")
			item.ErrorReason = missingDownloadReason(episodePage)
		}
		item.Existing = s.fileExists(ctx, item.Filename)
		downloads = append(downloads, plannedDownload{item: item, downloadURL: link.href})
	}
	return s.planFromDownloads(selection, downloads), downloads, nil
}

func (s *Service) fetchPage(ctx context.Context, pagePath string) (parsedPage, error) {
	body, status, err := s.fetchText(ctx, http.MethodGet, pagePath, nil)
	if err != nil {
		return parsedPage{}, err
	}
	if status >= 400 {
		return parsedPage{}, fmt.Errorf("media source page returned status %d", status)
	}
	return parseDetailPage(strings.NewReader(body)), nil
}

func (s *Service) fetchText(ctx context.Context, method string, rawPath string, form url.Values) (string, int, error) {
	target, err := s.resolveURL(rawPath)
	if err != nil {
		return "", 0, err
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", "HankRemoteMedia/1.0")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(data), resp.StatusCode, nil
}

func (s *Service) resolveURL(rawPath string) (string, error) {
	if strings.HasPrefix(rawPath, "http://") || strings.HasPrefix(rawPath, "https://") {
		return rawPath, nil
	}
	cfg := s.configSnapshot()
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return "", fmt.Errorf("invalid media source base URL")
	}
	relative, err := url.Parse(rawPath)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(relative).String(), nil
}

func (s *Service) resolveMediaResultAssets(results []protocol.MediaSearchResult) {
	for index := range results {
		results[index].PosterURL = s.resolveMediaAssetURL(results[index].PosterURL)
	}
}

func (s *Service) resolveMediaAssetURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	resolved, err := s.resolveURL(rawURL)
	if err != nil {
		return rawURL
	}
	return resolved
}

func (s *Service) fileExists(ctx context.Context, name string) bool {
	name = s.destinationFilePath(name)
	if name == "" {
		return false
	}
	item, err := s.files.Stat(ctx, name)
	return err == nil && !item.IsDirectory && item.Size > 0
}

func (s *Service) destinationFilePath(name string) string {
	name = cleanSharePath(name)
	cfg := s.configSnapshot()
	destination := cleanSharePath(cfg.DestinationPath)
	if destination == "" {
		return name
	}
	return cleanSharePath(path.Join(destination, name))
}

func (s *Service) runDownloadJob(ctx context.Context, job *downloadJob, downloads []plannedDownload) {
	job.update(func(status *protocol.MediaDownloadJobStatus) {
		status.Status = protocol.MediaJobStatusRunning
	})
	s.emitJob(ctx, "media.download_progress", job)

	for index, download := range downloads {
		if ctx.Err() != nil {
			s.markJobCancelled(job)
			s.emitJob(context.Background(), "media.download_completed", job)
			return
		}
		item := download.item
		job.update(func(status *protocol.MediaDownloadJobStatus) {
			status.CurrentIndex = index + 1
			status.CurrentFile = item.Filename
			status.BytesWritten = 0
		})
		if item.Existing {
			job.update(func(status *protocol.MediaDownloadJobStatus) {
				status.SkippedCount++
				status.CompletedCount++
			})
			s.emitJob(ctx, "media.download_progress", job)
			continue
		}
		if !item.DownloadOK || download.downloadURL == "" {
			job.update(func(status *protocol.MediaDownloadJobStatus) {
				status.FailedCount++
				status.ErrorMessage = firstNonBlank(item.ErrorReason, "No download link was available.")
			})
			s.emitJob(ctx, "media.download_progress", job)
			continue
		}
		if err := s.downloadOne(ctx, job, download); err != nil {
			if ctx.Err() != nil {
				s.markJobCancelled(job)
				s.emitJob(context.Background(), "media.download_completed", job)
				return
			}
			job.update(func(status *protocol.MediaDownloadJobStatus) {
				status.FailedCount++
				status.ErrorMessage = err.Error()
			})
			s.emitJob(ctx, "media.download_progress", job)
			continue
		}
		if ctx.Err() != nil {
			s.markJobCancelled(job)
			s.emitJob(context.Background(), "media.download_completed", job)
			return
		}
		job.update(func(status *protocol.MediaDownloadJobStatus) {
			status.CompletedCount++
			status.ErrorMessage = ""
		})
		s.emitJob(ctx, "media.download_progress", job)
	}

	if ctx.Err() != nil {
		s.markJobCancelled(job)
		s.emitJob(context.Background(), "media.download_completed", job)
		return
	}
	completedAt := time.Now().UTC()
	job.update(func(status *protocol.MediaDownloadJobStatus) {
		status.CompletedAt = completedAt
		if status.FailedCount > 0 {
			status.Status = protocol.MediaJobStatusFailed
		} else {
			status.Status = protocol.MediaJobStatusCompleted
		}
	})
	s.emitJob(ctx, "media.download_completed", job)
}

func (s *Service) markJobCancelled(job *downloadJob) {
	now := time.Now().UTC()
	job.update(func(status *protocol.MediaDownloadJobStatus) {
		status.Status = protocol.MediaJobStatusCancelled
		status.CompletedAt = now
		status.ErrorMessage = "Cancelled by user."
	})
}

func (s *Service) downloadOne(ctx context.Context, job *downloadJob, download plannedDownload) error {
	req, err := s.newDownloadRequest(ctx, download)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	sniff, err := io.ReadAll(io.LimitReader(resp.Body, downloadSniffByteSize))
	if err != nil {
		return err
	}
	if err := validateDownloadResponse(resp, sniff); err != nil {
		return err
	}

	finalPath := s.destinationFilePath(download.item.Filename)
	partPath := finalPath + ".part"
	writer, _, err := s.files.OpenWriter(ctx, partPath, 0)
	if err != nil {
		return err
	}
	var written int64
	_, copyErr := io.Copy(writer, progressReader{
		reader: io.MultiReader(bytes.NewReader(sniff), resp.Body),
		onProgress: func(n int64) {
			written += n
			job.update(func(status *protocol.MediaDownloadJobStatus) {
				status.BytesWritten = written
			})
		},
	})
	closeErr := writer.Close()
	if copyErr != nil {
		_ = s.files.Delete(context.Background(), partPath, false)
		return copyErr
	}
	if closeErr != nil {
		_ = s.files.Delete(context.Background(), partPath, false)
		return closeErr
	}
	if err := s.files.Rename(ctx, partPath, finalPath); err != nil {
		_ = s.files.Delete(context.Background(), partPath, false)
		return err
	}
	return nil
}

func (s *Service) newDownloadRequest(ctx context.Context, download plannedDownload) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, download.downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 HankRemoteMedia/1.0")
	req.Header.Set("Accept", "video/*, application/octet-stream;q=0.9, */*;q=0.8")
	if referer, err := s.resolveURL(download.item.PagePath); err == nil {
		req.Header.Set("Referer", referer)
	}
	return req, nil
}

func validateDownloadResponse(resp *http.Response, sniff []byte) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}
	if len(sniff) == 0 {
		return fmt.Errorf("download returned an empty response")
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	body := strings.ToLower(strings.TrimSpace(string(sniff)))
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		return fmt.Errorf("download returned an HTML page instead of media")
	}
	if strings.HasPrefix(body, "<!doctype html") ||
		strings.HasPrefix(body, "<html") ||
		strings.Contains(body, "</html>") ||
		strings.Contains(body, "<form") ||
		(strings.Contains(body, "login") && strings.Contains(body, "password")) {
		return fmt.Errorf("download returned a web page instead of media")
	}
	if strings.Contains(contentType, "application/json") &&
		(strings.Contains(body, "error") || strings.Contains(body, "message")) {
		return fmt.Errorf("download returned an error response instead of media")
	}
	return nil
}

func (s *Service) emitJob(ctx context.Context, event string, job *downloadJob) {
	s.mu.Lock()
	sink := s.eventSink
	s.mu.Unlock()
	if sink == nil {
		return
	}
	status := job.snapshot()
	if err := sink(ctx, event, "media.downloads", status); err != nil {
		s.logger.Debug("media download event failed", "event", event, "job_id", status.JobID, "error", err)
	}
}

func (j *downloadJob) update(fn func(status *protocol.MediaDownloadJobStatus)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	fn(&j.status)
}

func (j *downloadJob) snapshot() protocol.MediaDownloadJobStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

type progressReader struct {
	reader     io.Reader
	onProgress func(int64)
}

func (r progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && r.onProgress != nil {
		r.onProgress(int64(n))
	}
	return n, err
}

type parsedPage struct {
	title             string
	token             string
	posterURL         string
	downloads         []downloadLink
	episodes          []seriesEpisode
	downloadBlocked   bool
	placeholderButton bool
}

type downloadLink struct {
	href    string
	quality string
}

type seriesEpisode struct {
	season  int
	episode int
	title   string
}

func parseSearchResults(reader io.Reader, fallbackType string) []protocol.MediaSearchResult {
	root, err := html.Parse(reader)
	if err != nil {
		return nil
	}
	var results []protocol.MediaSearchResult
	for _, node := range findAll(root, func(node *html.Node) bool {
		return node.Type == html.ElementNode && hasClass(node, "item")
	}) {
		href := mediaHref(node)
		if href == "" {
			continue
		}
		mediaType := mediaTypeFromPath(href)
		if mediaType == "" {
			mediaType = fallbackType
		}
		title := cardTitle(node, href)
		if title == "" {
			continue
		}
		text := cleanText(nodeText(node))
		result := protocol.MediaSearchResult{
			ID:         mediaID(href),
			Title:      title,
			Year:       parseYear(text),
			Type:       mediaType,
			Summary:    searchSummary(title, text),
			Rating:     parseRating(text),
			PosterURL:  mediaImageURL(node),
			PagePath:   canonicalMediaPath(href),
			SearchText: text,
		}
		results = append(results, result)
	}
	return results
}

func parseDetailPage(reader io.Reader) parsedPage {
	root, err := html.Parse(reader)
	if err != nil {
		return parsedPage{}
	}
	page := parsedPage{
		title:             pageTitleFromNode(root),
		token:             pageTokenFromHTML(root),
		posterURL:         pagePosterURL(root),
		downloadBlocked:   pageHasDownloadAccessOverlay(root),
		placeholderButton: pageHasPlaceholderDownloadButton(root),
	}
	for _, node := range findAll(root, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "a" && attr(node, "href") != ""
	}) {
		href := strings.TrimSpace(attr(node, "href"))
		if !downloadHrefLooksUsable(href) {
			continue
		}
		text := cleanText(nodeText(node))
		quality := linkQuality(text, href)
		if quality == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(text), "download") && !downloadHrefLooksLikeMedia(href) {
			continue
		}
		page.downloads = append(page.downloads, downloadLink{href: href, quality: quality})
	}
	page.episodes = parseEpisodes(root)
	return page
}

func pagePosterURL(root *html.Node) string {
	if match := posterImagePattern.FindStringSubmatch(nodeText(root)); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return mediaImageURL(root)
}

func mediaImageURL(root *html.Node) string {
	for _, node := range findAll(root, func(node *html.Node) bool {
		return node.Type == html.ElementNode && (node.Data == "img" || node.Data == "source")
	}) {
		for _, name := range []string{"data-src", "data-original", "data-lazy-src", "data-lazy", "src", "poster"} {
			if candidate := imageCandidate(attr(node, name)); candidate != "" {
				return candidate
			}
		}
		if candidate := imageCandidateFromSrcset(attr(node, "srcset")); candidate != "" {
			return candidate
		}
	}
	return ""
}

func imageCandidateFromSrcset(srcset string) string {
	for _, part := range strings.Split(srcset, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		if candidate := imageCandidate(fields[0]); candidate != "" {
			return candidate
		}
	}
	return ""
}

func imageCandidate(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" ||
		strings.HasPrefix(strings.ToLower(rawURL), "data:") ||
		strings.HasPrefix(strings.ToLower(rawURL), "javascript:") {
		return ""
	}
	return rawURL
}

func missingDownloadReason(page parsedPage) string {
	if page.downloadBlocked {
		return "The media source did not expose a file download link; the page shows a premium or expired-access overlay."
	}
	if page.placeholderButton {
		return "The media source showed a download button, but it was only a placeholder and did not include a file URL."
	}
	return "No visible 1080p or 720p download link was found."
}

func pageTokenFromHTML(root *html.Node) string {
	match := tokenPattern.FindStringSubmatch(nodeText(root))
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func pageHasDownloadAccessOverlay(root *html.Node) bool {
	for _, node := range findAll(root, func(node *html.Node) bool {
		return node.Type == html.ElementNode &&
			(hasClass(node, "premium-expired") || hasClass(node, "premium-required"))
	}) {
		if node != nil {
			return true
		}
	}
	return false
}

func pageHasPlaceholderDownloadButton(root *html.Node) bool {
	for _, node := range findAll(root, func(node *html.Node) bool {
		if node.Type != html.ElementNode || node.Data != "a" {
			return false
		}
		href := strings.TrimSpace(attr(node, "href"))
		if href != "" && href != "#" {
			return false
		}
		text := strings.ToLower(cleanText(nodeText(node)))
		return strings.Contains(text, "download")
	}) {
		if node != nil {
			return true
		}
	}
	return false
}

func downloadHrefLooksUsable(href string) bool {
	lowered := strings.ToLower(strings.TrimSpace(href))
	if lowered == "" ||
		lowered == "#" ||
		strings.HasPrefix(lowered, "#") ||
		strings.HasPrefix(lowered, "javascript:") {
		return false
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return false
	}
	downloadPath := strings.TrimSpace(parsed.Path)
	if downloadPath == "" || downloadPath == "/" {
		return false
	}
	return downloadHrefLooksLikeMedia(href)
}

func downloadHrefLooksLikeMedia(href string) bool {
	parsed, err := url.Parse(href)
	if err != nil {
		return false
	}
	if strings.TrimSpace(parsed.Query().Get("file")) != "" {
		return true
	}
	loweredPath := strings.ToLower(parsed.Path)
	if strings.Contains(loweredPath, "/dl") || strings.Contains(loweredPath, "download") {
		return true
	}
	for _, suffix := range []string{".mp4", ".mkv", ".mov", ".avi", ".m4v"} {
		if strings.HasSuffix(loweredPath, suffix) {
			return true
		}
	}
	return false
}

func parseEpisodes(root *html.Node) []seriesEpisode {
	var episodes []seriesEpisode
	for _, ol := range findAll(root, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "ol" && seasonIDPattern.MatchString(attr(node, "id"))
	}) {
		match := seasonIDPattern.FindStringSubmatch(attr(ol, "id"))
		season, _ := strconv.Atoi(match[1])
		for child := ol.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.ElementNode || child.Data != "li" {
				continue
			}
			episode, _ := strconv.Atoi(attr(child, "data-episode"))
			if episode <= 0 {
				continue
			}
			episodes = append(episodes, seriesEpisode{
				season:  season,
				episode: episode,
				title:   cleanText(nodeText(child)),
			})
		}
	}
	return episodes
}

func chooseDownloadLink(links []downloadLink) downloadLink {
	var fallback downloadLink
	for _, link := range links {
		switch strings.ToLower(link.quality) {
		case "1080p":
			return link
		case "720p":
			if fallback.href == "" {
				fallback = link
			}
		}
	}
	return fallback
}

func (s *Service) planFromDownloads(selection protocol.MediaSearchResult, downloads []plannedDownload) protocol.MediaDownloadPlan {
	items := make([]protocol.MediaDownloadItem, 0, len(downloads))
	plan := protocol.MediaDownloadPlan{
		Selection:       selection,
		DestinationPath: s.destinationLabel(),
	}
	for _, download := range downloads {
		item := download.item
		items = append(items, item)
		if item.DownloadOK {
			switch strings.ToLower(item.Quality) {
			case "1080p":
				plan.PreferredQualityCount++
			case "720p":
				plan.FallbackQualityCount++
			}
		} else {
			plan.MissingLinkCount++
		}
		if item.Existing {
			plan.ExistingCount++
		}
	}
	plan.Items = items
	plan.ItemCount = len(items)
	return plan
}

func (s *Service) destinationLabel() string {
	cfg := s.configSnapshot()
	if cfg.DestinationPath == "" {
		return "Media root"
	}
	return "Media root/" + cfg.DestinationPath
}

func (s *Service) persistSettings(cfg Config) error {
	cfg.EnvPath = strings.TrimSpace(cfg.EnvPath)
	if cfg.EnvPath == "" {
		return nil
	}
	env, err := readEnvFile(cfg.EnvPath)
	if err != nil {
		return err
	}
	env["HANK_REMOTE_MEDIA_GRAMATON_ENABLED"] = strconv.FormatBool(cfg.Enabled)
	env["HANK_REMOTE_MEDIA_GRAMATON_BASE_URL"] = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	env["HANK_REMOTE_MEDIA_GRAMATON_USERNAME"] = strings.TrimSpace(cfg.Username)
	env["HANK_REMOTE_MEDIA_GRAMATON_PASSWORD"] = cfg.Password
	env["HANK_REMOTE_MEDIA_DESTINATION_PATH"] = cleanSharePath(cfg.DestinationPath)
	return writeEnvFile(cfg.EnvPath, env)
}

func readEnvFile(envPath string) (map[string]string, error) {
	env := map[string]string{}
	data, err := os.ReadFile(envPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return env, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[strings.TrimSpace(key)] = value
	}
	return env, nil
}

func writeEnvFile(envPath string, env map[string]string) error {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		lines = append(lines, key+"="+env[key])
	}
	lines = append(lines, "")
	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0o600)
}

func parseLoginForm(body string) (string, url.Values) {
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return "", url.Values{}
	}
	for _, form := range findAll(root, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "form"
	}) {
		if !strings.Contains(strings.ToLower(renderText(form)), "password") && !formHasPassword(form) {
			continue
		}
		values := url.Values{}
		for _, input := range findAll(form, func(node *html.Node) bool {
			return node.Type == html.ElementNode && (node.Data == "input" || node.Data == "textarea" || node.Data == "select")
		}) {
			name := strings.TrimSpace(attr(input, "name"))
			if name == "" {
				continue
			}
			values.Set(name, attr(input, "value"))
		}
		return firstNonBlank(attr(form, "action"), "/session/login"), values
	}
	return "", url.Values{}
}

func formHasPassword(form *html.Node) bool {
	for _, input := range findAll(form, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "input"
	}) {
		if strings.EqualFold(attr(input, "type"), "password") || strings.Contains(strings.ToLower(attr(input, "name")), "pass") {
			return true
		}
	}
	return false
}

func hasAnyKey(values url.Values, keys ...string) bool {
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func pageTitle(page parsedPage) string {
	return page.title
}

func pageTitleFromNode(root *html.Node) string {
	for _, node := range findAll(root, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "h1"
	}) {
		title := cleanText(nodeText(node))
		if title != "" && !strings.EqualFold(title, "gramaton") && !strings.EqualFold(title, "ramaton") {
			return title
		}
	}
	return ""
}

func cardTitle(node *html.Node, href string) string {
	for _, child := range findAll(node, func(node *html.Node) bool {
		return node.Type == html.ElementNode && (node.Data == "h2" || node.Data == "h3" || hasClass(node, "title") || strings.Contains(attr(node, "class"), "name"))
	}) {
		if title := cleanText(nodeText(child)); title != "" {
			return stripTrailingYear(title)
		}
	}
	return mediaTitleFromPath(href)
}

func mediaHref(node *html.Node) string {
	for _, link := range findAll(node, func(node *html.Node) bool {
		if node.Type != html.ElementNode || node.Data != "a" {
			return false
		}
		href := attr(node, "href")
		return strings.HasPrefix(href, "/movies/") || strings.HasPrefix(href, "/series/")
	}) {
		return attr(link, "href")
	}
	return ""
}

func mediaTitleFromPath(raw string) string {
	match := mediaPathPattern.FindStringSubmatch(canonicalMediaPath(raw))
	if len(match) != 3 {
		return ""
	}
	slug := match[2]
	if dash := strings.Index(slug, "-"); dash >= 0 {
		slug = slug[dash+1:]
	}
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func mediaTypeFromPath(raw string) string {
	raw = canonicalMediaPath(raw)
	if strings.HasPrefix(raw, "/movies/") {
		return protocol.MediaTypeMovie
	}
	if strings.HasPrefix(raw, "/series/") {
		return protocol.MediaTypeSeries
	}
	return ""
}

func canonicalMediaPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Path != "" {
		raw = parsed.Path
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	match := mediaPathPattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		return ""
	}
	return "/" + match[1] + "/" + match[2]
}

func mediaID(raw string) string {
	return strings.Trim(canonicalMediaPath(raw), "/")
}

func mediaPageID(raw string) string {
	raw = canonicalMediaPath(raw)
	match := mediaPageIDPattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		return ""
	}
	return match[2]
}

func parseYear(text string) int {
	matches := yearPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return 0
	}
	year, _ := strconv.Atoi(matches[len(matches)-1])
	return year
}

func parseRating(text string) string {
	fields := strings.Fields(text)
	for _, field := range fields {
		trimmed := strings.Trim(field, "(),")
		if _, err := strconv.ParseFloat(trimmed, 64); err == nil && strings.Contains(trimmed, ".") {
			return trimmed
		}
	}
	return ""
}

func searchSummary(title string, text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, " watch", ""))
	text = strings.TrimSpace(strings.TrimPrefix(text, title))
	return text
}

func linkQuality(text string, href string) string {
	if match := qualityPattern.FindStringSubmatch(text); len(match) == 2 {
		return match[1] + "p"
	}
	if match := qualityPattern.FindStringSubmatch(href); len(match) == 2 {
		return match[1] + "p"
	}
	return ""
}

func downloadFilename(rawURL string, title string, season int, episode int, quality string) string {
	if rawURL != "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			if file := parsed.Query().Get("file"); file != "" {
				return cleanSharePath(file)
			}
			if base := path.Base(parsed.Path); base != "." && base != "/" && base != "" {
				return cleanSharePath(base)
			}
		}
	}
	if season > 0 || episode > 0 {
		return episodeFilename(title, season, episode, quality)
	}
	title = safeFilename(firstNonBlank(title, "media"))
	if quality != "" {
		return title + "_" + quality + ".mp4"
	}
	return title + ".mp4"
}

func episodeFilename(title string, season int, episode int, quality string) string {
	name := fmt.Sprintf("%s_S%02dE%02d", safeFilename(firstNonBlank(title, "series")), season, episode)
	if quality != "" {
		name += "_" + quality
	}
	return name + ".mp4"
}

func safeFilename(name string) string {
	name = strings.ReplaceAll(name, "_", " ")
	name = unsafeNameRunes.ReplaceAllString(name, "")
	name = spacingPattern.ReplaceAllString(strings.TrimSpace(name), "_")
	if name == "" {
		return "media"
	}
	return name
}

func cleanSharePath(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	if name == "." {
		return ""
	}
	return name
}

func uniqueSearchResults(results []protocol.MediaSearchResult) []protocol.MediaSearchResult {
	seen := map[string]struct{}{}
	unique := make([]protocol.MediaSearchResult, 0, len(results))
	for _, result := range results {
		key := result.Type + ":" + result.PagePath
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, result)
	}
	return unique
}

func mediaSearchQueries(query string) []string {
	original := cleanSearchQuery(query)
	normalized := normalizeForMatch(query)
	tokens := strings.Fields(normalized)
	distinctive := distinctiveSearchTokens(tokens)

	queries := make([]string, 0, 8)
	seen := map[string]struct{}{}
	add := func(value string) {
		value = cleanSearchQuery(value)
		if value == "" || len(queries) >= 8 {
			return
		}
		key := normalizeForMatch(value)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		queries = append(queries, value)
	}

	add(original)
	add(canonicalizeMediaSearchTokens(normalized))
	add(strings.Join(tokens, " "))
	add(strings.Join(distinctive, " "))
	for i := 0; i+1 < len(distinctive); i++ {
		add(distinctive[i] + " " + distinctive[i+1])
	}
	for _, token := range distinctive {
		add(token)
	}
	if len(queries) == 0 {
		add(query)
	}
	return queries
}

func cleanSearchQuery(value string) string {
	return spacingPattern.ReplaceAllString(strings.TrimSpace(value), " ")
}

func distinctiveSearchTokens(tokens []string) []string {
	seen := map[string]struct{}{}
	distinctive := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, stop := mediaSearchStopWords[token]; stop {
			continue
		}
		if len(token) <= 2 && !containsDigit(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		distinctive = append(distinctive, token)
	}
	return distinctive
}

func containsDigit(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func mediaResultScore(result protocol.MediaSearchResult, query string) int {
	title := normalizeForMatch(result.Title)
	search := normalizeForMatch(result.SearchText)
	query = normalizeForMatch(query)
	if query == "" {
		return 0
	}
	queryTokens := strings.Fields(query)
	distinctiveTokens := distinctiveSearchTokens(queryTokens)
	if len(distinctiveTokens) > 0 {
		matched := false
		for _, token := range distinctiveTokens {
			if containsNormalizedTerm(title, token) || containsNormalizedTerm(search, token) {
				matched = true
				break
			}
		}
		if !matched {
			return 0
		}
	}
	if title == query {
		return 100
	}
	score := 0
	if containsNormalizedTerm(title, query) {
		score += 70
	}
	if containsNormalizedTerm(search, query) {
		score += 30
	}
	score += mediaTypeHintScore(queryTokens, result.Type)
	for _, token := range queryTokens {
		if containsNormalizedTerm(title, token) {
			score += 15
		}
		if containsNormalizedTerm(search, token) {
			score += 5
		}
	}
	return score
}

func containsNormalizedTerm(value string, term string) bool {
	if value == "" || term == "" {
		return false
	}
	if value == term {
		return true
	}
	return strings.Contains(" "+value+" ", " "+term+" ")
}

func mediaTypeHintScore(tokens []string, mediaType string) int {
	for _, token := range tokens {
		switch token {
		case "movie", "movies", "film", "films":
			if mediaType == protocol.MediaTypeMovie {
				return 20
			}
		case "series", "show", "shows", "tv", "season", "seasons", "episode", "episodes":
			if mediaType == protocol.MediaTypeSeries {
				return 20
			}
		}
	}
	return 0
}

func normalizeForMatch(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	lastSpace := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(builder.String())
}

func stripTrailingYear(value string) string {
	return strings.TrimSpace(yearPattern.ReplaceAllString(value, ""))
}

func cleanText(value string) string {
	return spacingPattern.ReplaceAllString(strings.TrimSpace(value), " ")
}

func renderText(node *html.Node) string {
	return nodeText(node)
}

func nodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text := nodeText(child)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(text)
	}
	return builder.String()
}

func findAll(root *html.Node, match func(*html.Node) bool) []*html.Node {
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if match(node) {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return nodes
}

func attr(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func hasClass(node *html.Node, className string) bool {
	for _, class := range strings.Fields(attr(node, "class")) {
		if class == className {
			return true
		}
	}
	return false
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func newJobID() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("media_%d", time.Now().UnixNano())
	}
	return "media_" + hex.EncodeToString(random[:])
}
