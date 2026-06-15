package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/agent/apps"
	"github.com/dropfile/hankremote/internal/protocol"
)

func TestGramatonAppRunSettingsStatusUsesInheritedAgentFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Media", "Movies"), 0o755); err != nil {
		t.Fatal(err)
	}
	setRequiredAgentEnv(t, root)

	code, stdout, stderr := runGramatonApp(t, apps.AppStdioRequest{
		RequestID: "req_settings",
		AppID:     "gramaton",
		CommandID: "settings_status",
		Config:    json.RawMessage(`{"enabled":true,"base_url":"https://gramaton.example","username":"aaron","source_id":"local","destination_path":"Media","movie_destination_path":"Media/Movies","tv_destination_path":"Media/TV","require_confirmation":true}`),
		Secrets:   json.RawMessage(`{"password":"secret"}`),
	})
	if code != 0 {
		t.Fatalf("run code = %d stderr=%s stdout=%s", code, stderr, stdout)
	}
	response := decodeGramatonResponse(t, stdout)
	var output protocol.MediaSettingsStatusResponse
	if err := json.Unmarshal(response.Output, &output); err != nil {
		t.Fatalf("Decode output: %v", err)
	}
	if output.Settings.SourceID != "local" || output.Settings.DestinationPath != "Media" || !output.Settings.HasPassword {
		t.Fatalf("settings = %#v", output.Settings)
	}
	if !destinationOptionsContain(output.DestinationOptions, "local", "Media/Movies") {
		t.Fatalf("destination options missing inherited local folder: %#v", output.DestinationOptions)
	}
}

func TestGramatonAppRunSearchSuccess(t *testing.T) {
	root := t.TempDir()
	setRequiredAgentEnv(t, root)

	var gotAuth bool
	var gotSearch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="/session/login">login</a>`))
		case "/session/login":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`<form action="/session/login" method="post"><input name="email"><input name="password"></form>`))
				return
			}
			gotAuth = true
			_, _ = w.Write([]byte(`<a href="/session/logout">logout</a>`))
		case "/movies":
			_, _ = w.Write([]byte(`<script>token_key = "search-token"</script>`))
		case "/index/loadmovies":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			gotSearch = r.Form.Get("q")
			if r.Form.Get("search") != "Project Hail Mary" || r.Form.Get("type") != "movie" || r.Form.Get("token") != "search-token" {
				t.Fatalf("search form = %#v", r.Form)
			}
			_, _ = w.Write([]byte(`
				<div class="item">
					<a class="movie-card-link" href="/movies/20429-project-hail-mary"></a>
					<img src="/images/project-hail-mary.jpg">
					<h2>Project Hail Mary</h2>
					<p>Sci-Fi PG-13 2026 watch</p>
				</div>
			`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	code, stdout, stderr := runGramatonApp(t, apps.AppStdioRequest{
		RequestID: "req_search",
		AppID:     "gramaton",
		CommandID: "search",
		Config:    json.RawMessage(`{"enabled":true,"base_url":` + quoteJSON(server.URL) + `,"username":"aaron","source_id":"local","destination_path":"Media","require_confirmation":true}`),
		Secrets:   json.RawMessage(`{"password":"secret"}`),
		Input:     json.RawMessage(`{"query":"Project Hail Mary","limit":10}`),
	})
	if code != 0 {
		t.Fatalf("run code = %d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !gotAuth || gotSearch != "Project Hail Mary" {
		t.Fatalf("gotAuth=%v gotSearch=%q", gotAuth, gotSearch)
	}
	response := decodeGramatonResponse(t, stdout)
	var output protocol.MediaSearchResponse
	if err := json.Unmarshal(response.Output, &output); err != nil {
		t.Fatalf("Decode output: %v", err)
	}
	if len(output.Results) != 1 || output.Results[0].Title != "Project Hail Mary" || !strings.HasPrefix(output.Results[0].PosterURL, server.URL) {
		t.Fatalf("output = %#v", output)
	}
}

func TestGramatonAppRunDownloadStatusEmitsEvent(t *testing.T) {
	root := t.TempDir()
	setRequiredAgentEnv(t, root)
	workDir := t.TempDir()
	writeJobStatus(t, workDir, protocol.MediaDownloadJobStatus{
		JobID:      "job_1",
		Status:     protocol.MediaJobStatusRunning,
		Title:      "Fixture",
		TotalCount: 1,
		StartedAt:  time.Now().UTC(),
	})

	code, stdout, stderr := runGramatonAppInDir(t, workDir, apps.AppStdioRequest{
		RequestID: "req_status",
		AppID:     "gramaton",
		CommandID: "download_status",
		Config:    json.RawMessage(`{"enabled":true,"base_url":"https://gramaton.example","username":"aaron","source_id":"local","destination_path":"Media","require_confirmation":true}`),
		Secrets:   json.RawMessage(`{"password":"secret"}`),
		Input:     json.RawMessage(`{"job_id":"job_1"}`),
	})
	if code != 0 {
		t.Fatalf("run code = %d stderr=%s stdout=%s", code, stderr, stdout)
	}
	response := decodeGramatonResponse(t, stdout)
	if len(response.Events) != 1 || response.Events[0].Event != "media.download_progress" || response.Events[0].Topic != "media.downloads" {
		t.Fatalf("events = %#v", response.Events)
	}
	var output protocol.MediaDownloadStatusResponse
	if err := json.Unmarshal(response.Output, &output); err != nil {
		t.Fatalf("Decode output: %v", err)
	}
	if output.Job.JobID != "job_1" || output.Job.Status != protocol.MediaJobStatusRunning {
		t.Fatalf("job = %#v", output.Job)
	}
}

func TestDownloadStartPersistsEpisodeFilter(t *testing.T) {
	root := t.TempDir()
	setRequiredAgentEnv(t, root)
	workDir := t.TempDir()

	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`<a href="/session/logout">logout</a>`))
		case r.URL.Path == "/series/fixture-show" && r.URL.Query().Get("season") == "1" && r.URL.Query().Get("episode") == "1":
			_, _ = w.Write([]byte(`<h1>Fixture Show</h1><a href="` + baseURL + `/download/s1e1_1080p.mp4?file=Fixture_Show_S01E01_1080p.mp4">Download 1080p</a>`))
		case r.URL.Path == "/series/fixture-show" && r.URL.Query().Get("season") == "2" && r.URL.Query().Get("episode") == "1":
			_, _ = w.Write([]byte(`<h1>Fixture Show</h1><a href="` + baseURL + `/download/s2e1_1080p.mp4?file=Fixture_Show_S02E01_1080p.mp4">Download 1080p</a>`))
		case r.URL.Path == "/series/fixture-show":
			_, _ = w.Write([]byte(`<h1>Fixture Show</h1><div class="tv-details-episodes"><ol id="season1"><li data-episode="1">Pilot</li></ol><ol id="season2"><li data-episode="1">Return</li></ol></div>`))
		case strings.HasPrefix(r.URL.Path, "/download/"):
			_, _ = w.Write([]byte("download"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	code, stdout, stderr := runGramatonAppInDir(t, workDir, apps.AppStdioRequest{
		RequestID: "req_start",
		AppID:     "gramaton",
		CommandID: "download_start",
		Config:    json.RawMessage(`{"enabled":true,"base_url":` + quoteJSON(server.URL) + `,"username":"aaron","source_id":"local","destination_path":"Media","require_confirmation":true}`),
		Secrets:   json.RawMessage(`{"password":"secret"}`),
		Input: json.RawMessage(`{
			"selection":{"id":"series/fixture-show","title":"Fixture Show","type":"series","page_path":"/series/fixture-show"},
			"filter":{"season":2}
		}`),
	})
	if code != 0 {
		t.Fatalf("run code = %d stderr=%s stdout=%s", code, stderr, stdout)
	}
	response := decodeGramatonResponse(t, stdout)
	var output protocol.MediaDownloadStartResponse
	if err := json.Unmarshal(response.Output, &output); err != nil {
		t.Fatalf("Decode output: %v", err)
	}
	if output.Job.TotalCount != 1 {
		t.Fatalf("job total = %d, want filtered season total 1", output.Job.TotalCount)
	}
	record, err := readJobRecordPath(filepath.Join(workDir, jobsDir(), output.Job.JobID+".json"))
	if err != nil {
		t.Fatalf("read job record: %v", err)
	}
	if record.Request.Filter.Season != 2 || record.Request.Filter.Episode != 0 {
		t.Fatalf("record filter = %#v", record.Request.Filter)
	}
}

func runGramatonApp(t *testing.T, request apps.AppStdioRequest) (int, string, string) {
	t.Helper()
	return runGramatonAppInDir(t, t.TempDir(), request)
}

func runGramatonAppInDir(t *testing.T, workDir string, request apps.AppStdioRequest) (int, string, string) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	}()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code := run(ctx, bytes.NewReader(append(raw, '\n')), &stdout, &stderr, http.DefaultClient)
	return code, stdout.String(), stderr.String()
}

func decodeGramatonResponse(t *testing.T, raw string) apps.AppStdioResponse {
	t.Helper()
	var response apps.AppStdioResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatalf("Decode response %q: %v", raw, err)
	}
	if !response.OK {
		t.Fatalf("response failed: %#v", response)
	}
	return response
}

func setRequiredAgentEnv(t *testing.T, root string) {
	t.Helper()
	t.Setenv("HANK_REMOTE_AGENT_CLOUD_URL", "ws://cloud.example/ws/agent")
	t.Setenv("HANK_REMOTE_AGENT_ID", "agent_1")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "agent-token")
	t.Setenv("HANK_REMOTE_AGENT_FILES_ROOT", root)
	t.Setenv("HANK_REMOTE_SMB_SHARES_JSON", "")
}

func writeJobStatus(t *testing.T, workDir string, status protocol.MediaDownloadJobStatus) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore Chdir: %v", err)
		}
	}()
	if err := writeJobRecord(jobRecord{Job: status}); err != nil {
		t.Fatalf("writeJobRecord: %v", err)
	}
}

func destinationOptionsContain(options []protocol.MediaDestinationOption, sourceID string, value string) bool {
	for _, option := range options {
		if option.SourceID == sourceID && option.Value == value {
			return true
		}
	}
	return false
}

func quoteJSON(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
