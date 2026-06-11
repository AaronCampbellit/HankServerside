package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dropfile/hankremote/internal/agent/apps"
)

func TestRunDownloadsThroughConfiguredFileSource(t *testing.T) {
	filesRoot := t.TempDir()
	fakeYTDLP := filepath.Join(t.TempDir(), "yt-dlp")
	script := "#!/bin/sh\nout=''\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = '--paths' ]; then shift; out=\"$1\"; fi\n  shift\ndone\nmkdir -p \"$out\"\nprintf 'video bytes' > \"$out/example.mp4\"\n"
	if err := os.WriteFile(fakeYTDLP, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HANK_REMOTE_AGENT_ID", "agent-test")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "token-test")
	t.Setenv("HANK_REMOTE_AGENT_FILES_ROOT", filesRoot)

	writeSubs := true
	request := apps.AppStdioRequest{
		ProtocolVersion: "hank.app.stdio.v1",
		RequestID:       "req_1",
		AppID:           "ydownload",
		CommandID:       "download",
		Config: mustJSON(t, appConfig{
			SourceID:          "local",
			DestinationPath:   "Downloads",
			YTDLPPath:         fakeYTDLP,
			WriteSubtitles:    &writeSubs,
			SubtitleLanguages: "en.*",
		}),
		Input: json.RawMessage(`{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}`),
	}
	rawRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), bytes.NewReader(append(rawRequest, '\n')), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run code = %d, stderr = %q, stdout = %q", code, stderr.String(), stdout.String())
	}

	var response apps.AppStdioResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v: %s", err, stdout.String())
	}
	if !response.OK || response.RequestID != "req_1" || len(response.Output) == 0 {
		t.Fatalf("response = %#v", response)
	}
	var output commandOutput
	if err := json.Unmarshal(response.Output, &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output.Files) != 1 || output.Files[0].Path != "Downloads/example.mp4" {
		t.Fatalf("files = %#v", output.Files)
	}
	if got, err := os.ReadFile(filepath.Join(filesRoot, "Downloads", "example.mp4")); err != nil || string(got) != "video bytes" {
		t.Fatalf("stored file = %q, %v", string(got), err)
	}
}

func TestYTDLPArgsCommonSettings(t *testing.T) {
	t.Parallel()

	writeSubs := true
	writeAutoSubs := true
	writeThumb := true
	writeInfo := true
	downloadPlaylist := true
	restrict := false
	noOverwrite := false
	args := ytDLPArgs(appConfig{
		Format:             "bestvideo+bestaudio/best",
		OutputTemplate:     "%(title)s.%(ext)s",
		WriteSubtitles:     &writeSubs,
		WriteAutoSubtitles: &writeAutoSubs,
		SubtitleLanguages:  "en.*,es",
		SubtitleFormat:     "srt",
		WriteThumbnail:     &writeThumb,
		WriteInfoJSON:      &writeInfo,
		DownloadPlaylist:   &downloadPlaylist,
		RestrictFilenames:  &restrict,
		NoOverwrite:        &noOverwrite,
		RateLimit:          "5M",
		ProxyURL:           "socks5://127.0.0.1:1080",
		CookiesFilePath:    "/tmp/cookies.txt",
	}, commandInput{FilenamePrefix: "clip/name"}, "/tmp/out", "https://example.test/watch")

	joined := "\x00" + strings.Join(args, "\x00") + "\x00"
	for _, want := range []string{"\x00--write-subs\x00", "\x00--write-auto-subs\x00", "\x00--sub-langs\x00en.*,es\x00", "\x00--sub-format\x00srt\x00", "\x00--write-thumbnail\x00", "\x00--write-info-json\x00", "\x00--limit-rate\x005M\x00", "\x00--proxy\x00socks5://127.0.0.1:1080\x00", "\x00--cookies\x00/tmp/cookies.txt\x00"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %#v", want, args)
		}
	}
	for _, unwanted := range []string{"\x00--no-playlist\x00", "\x00--restrict-filenames\x00", "\x00--no-overwrites\x00"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("args unexpectedly contain %q: %#v", unwanted, args)
		}
	}
	if !strings.Contains(joined, "\x00clip-name - %(title)s.%(ext)s\x00") {
		t.Fatalf("args missing cleaned output prefix: %#v", args)
	}
}

func TestRunRejectsUnsupportedCommand(t *testing.T) {
	t.Parallel()
	request := apps.AppStdioRequest{
		RequestID: "req_2",
		AppID:     "ydownload",
		CommandID: "missing",
		Input:     json.RawMessage(`{"url":"https://example.test/watch"}`),
	}
	rawRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), bytes.NewReader(append(rawRequest, '\n')), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run code = 0, want failure")
	}
	var response apps.AppStdioResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v: %s", err, stdout.String())
	}
	if response.OK || response.Error == nil || response.Error.Code != "invalid_request" {
		t.Fatalf("response = %#v", response)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
