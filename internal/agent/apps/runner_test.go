package apps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeExecutable(t *testing.T, dir string, body string) string {
	t.Helper()
	path := filepath.Join(dir, "app.sh")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestRunnerInvokeReturnsOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := writeExecutable(t, dir, "#!/bin/sh\nread line\nprintf '%s\n' '{\"request_id\":\"req_1\",\"ok\":true,\"output\":{\"text\":\"ok\"}}'\n")
	runner := Runner{MaxOutputBytes: 4096, MaxStderrBytes: 1024}
	response, err := runner.Invoke(context.Background(), InvokeSpec{
		Executable: exe,
		WorkDir:    dir,
		Timeout:    time.Second,
		Request: AppStdioRequest{
			ProtocolVersion: "hank.app.stdio.v1",
			RequestID:       "req_1",
			AppID:           "hermes",
			CommandID:       "chat",
			Input:           json.RawMessage(`{"prompt":"hello"}`),
		},
	})
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if !response.OK || string(response.Output) != `{"text":"ok"}` {
		t.Fatalf("response = %#v", response)
	}
}

func TestRunnerInvokeRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := writeExecutable(t, dir, "#!/bin/sh\nprintf '%s\n' 'not json'\n")
	runner := Runner{MaxOutputBytes: 4096, MaxStderrBytes: 1024}
	_, err := runner.Invoke(context.Background(), InvokeSpec{
		Executable: exe,
		WorkDir:    dir,
		Timeout:    time.Second,
		Request:    AppStdioRequest{RequestID: "req_1"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid app response") {
		t.Fatalf("Invoke error = %v", err)
	}
}

func TestRunnerInvokeTimesOut(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := writeExecutable(t, dir, "#!/bin/sh\nsleep 2\n")
	runner := Runner{MaxOutputBytes: 4096, MaxStderrBytes: 1024}
	_, err := runner.Invoke(context.Background(), InvokeSpec{
		Executable: exe,
		WorkDir:    dir,
		Timeout:    20 * time.Millisecond,
		Request:    AppStdioRequest{RequestID: "req_1"},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Invoke error = %v", err)
	}
}
