package apps

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dropfile/hankremote/internal/protocol"
)

func TestManagerListEmptyDirectory(t *testing.T) {
	t.Parallel()
	manager := NewManager(filepath.Join(t.TempDir(), "apps"), filepath.Join(t.TempDir(), "staging"), Runner{})

	if err := manager.Load(context.Background()); err != nil {
		t.Fatalf("Load error: %v", err)
	}
	response := manager.List(context.Background())
	if len(response.Apps) != 0 {
		t.Fatalf("apps = %#v, want empty", response.Apps)
	}
}

func TestManagerPreviewAndActivateHermesPackage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	appsDir := filepath.Join(t.TempDir(), "apps")
	stagingDir := filepath.Join(t.TempDir(), "staging")
	archivePath := writeManagerHermesPackage(t, t.TempDir(), hermesRuntimeScript(`{"installed":true}`))
	manager := NewManager(appsDir, stagingDir, Runner{})

	preview, err := manager.PreviewPackage(ctx, protocol.AppsPackagePreviewRequest{
		StagingID:   "stage_1",
		DownloadURL: fileURL(t, archivePath),
	})
	if err != nil {
		t.Fatalf("PreviewPackage error: %v", err)
	}
	if preview.StagingID != "stage_1" || preview.App.ID != "hermes" || preview.Replacing {
		t.Fatalf("preview = %#v", preview)
	}

	activated, err := manager.ActivatePackage(ctx, protocol.AppsPackageActivateRequest{StagingID: "stage_1"})
	if err != nil {
		t.Fatalf("ActivatePackage error: %v", err)
	}
	if activated.App.ID != "hermes" || activated.App.Enabled {
		t.Fatalf("activated app = %#v, want installed disabled hermes app", activated.App)
	}
	if _, err := os.Stat(filepath.Join(appsDir, "hermes", "app.json")); err != nil {
		t.Fatalf("installed app.json missing: %v", err)
	}
}

func TestManagerPreviewRejectsOversizedPackage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "oversized.hankapp")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxPackageBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(t.TempDir(), "apps"), filepath.Join(t.TempDir(), "staging"), Runner{})

	_, err = manager.PreviewPackage(ctx, protocol.AppsPackagePreviewRequest{
		StagingID:   "stage_1",
		DownloadURL: fileURL(t, archivePath),
	})
	if err == nil || !strings.Contains(err.Error(), "app package exceeds") {
		t.Fatalf("PreviewPackage error = %v, want package size limit", err)
	}
}

func TestManagerPreviewHTTPDownloadUsesAgentAndPackageTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	archivePath := writeManagerHermesPackage(t, t.TempDir(), hermesRuntimeScript(`{"installed":true}`))
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer agent-secret" {
			t.Fatalf("Authorization = %q, want agent bearer token", got)
		}
		if got := r.Header.Get("X-Hank-Agent-ID"); got != "agent_1" {
			t.Fatalf("X-Hank-Agent-ID = %q", got)
		}
		if got := r.Header.Get("X-Hank-App-Package-Token"); got != "package-secret" {
			t.Fatalf("X-Hank-App-Package-Token = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	manager := NewManager(filepath.Join(t.TempDir(), "apps"), filepath.Join(t.TempDir(), "staging"), Runner{})
	manager.SetPackageDownloadAuth("agent_1", "agent-secret")
	preview, err := manager.PreviewPackage(ctx, protocol.AppsPackagePreviewRequest{
		StagingID:     "stage_1",
		DownloadURL:   server.URL + "/hermes.hankapp",
		DownloadToken: "package-secret",
	})
	if err != nil {
		t.Fatalf("PreviewPackage error: %v", err)
	}
	if preview.App.ID != "hermes" {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestManagerConfigApplyEnablesAppAndTracksSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := installManagerHermesPackage(t, false)
	enable := true

	response, err := manager.ConfigApply(ctx, protocol.AppsConfigApplyRequest{
		AppID:        "hermes",
		PublicConfig: json.RawMessage(`{"api_base_url":"https://hermes.local"}`),
		Secrets:      json.RawMessage(`{"api_key":"secret"}`),
		Enable:       &enable,
	})
	if err != nil {
		t.Fatalf("ConfigApply error: %v", err)
	}
	if !response.App.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if string(response.App.PublicConfig) != `{"api_base_url":"https://hermes.local"}` {
		t.Fatalf("PublicConfig = %s", response.App.PublicConfig)
	}
	if !response.App.SecretFieldsSet["api_key"] {
		t.Fatalf("SecretFieldsSet = %#v, want api_key set", response.App.SecretFieldsSet)
	}
}

func TestManagerCapabilitiesOnlyIncludesEnabledApps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := installManagerHermesPackage(t, false)

	if capabilities := manager.Capabilities(); len(capabilities) != 0 {
		t.Fatalf("Capabilities = %#v, want none for disabled app", capabilities)
	}

	enable := true
	if _, err := manager.ConfigApply(ctx, protocol.AppsConfigApplyRequest{
		AppID:  "hermes",
		Enable: &enable,
	}); err != nil {
		t.Fatalf("ConfigApply error: %v", err)
	}
	capabilities := manager.Capabilities()
	if len(capabilities) != 1 || capabilities[0] != "apps.hermes.chat" {
		t.Fatalf("Capabilities = %#v, want enabled hermes chat capability", capabilities)
	}
}

func TestManagerConfigApplyPreservesExistingSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := installManagerHermesPackageWithScript(t, true, hermesSecretEchoScript())

	response, err := manager.ConfigApply(ctx, protocol.AppsConfigApplyRequest{
		AppID:   "hermes",
		Secrets: json.RawMessage(`{"api_key":"original-secret"}`),
	})
	if err != nil {
		t.Fatalf("ConfigApply initial secret error: %v", err)
	}
	if !response.App.SecretFieldsSet["api_key"] {
		t.Fatalf("SecretFieldsSet = %#v, want api_key set", response.App.SecretFieldsSet)
	}
	assertHermesReceivesAPIKey(t, manager, "original-secret")

	for _, secrets := range []json.RawMessage{
		json.RawMessage(`{"api_key":""}`),
		json.RawMessage(`{"api_key":null}`),
		json.RawMessage(`{}`),
	} {
		response, err := manager.ConfigApply(ctx, protocol.AppsConfigApplyRequest{
			AppID:   "hermes",
			Secrets: secrets,
		})
		if err != nil {
			t.Fatalf("ConfigApply preserving secret %s error: %v", secrets, err)
		}
		if !response.App.SecretFieldsSet["api_key"] {
			t.Fatalf("SecretFieldsSet after %s = %#v, want api_key preserved", secrets, response.App.SecretFieldsSet)
		}
		assertHermesReceivesAPIKey(t, manager, "original-secret")
	}
}

func TestManagerConfigApplyEmptySecretWithoutExistingValueDoesNotSetMetadata(t *testing.T) {
	t.Parallel()
	manager := installManagerHermesPackage(t, false)

	for _, secrets := range []json.RawMessage{
		json.RawMessage(`{"api_key":""}`),
		json.RawMessage(`{"api_key":null}`),
		json.RawMessage(`{}`),
	} {
		response, err := manager.ConfigApply(context.Background(), protocol.AppsConfigApplyRequest{
			AppID:   "hermes",
			Secrets: secrets,
		})
		if err != nil {
			t.Fatalf("ConfigApply empty secret %s error: %v", secrets, err)
		}
		if response.App.SecretFieldsSet["api_key"] {
			t.Fatalf("SecretFieldsSet after %s = %#v, want api_key unset", secrets, response.App.SecretFieldsSet)
		}
	}
}

func TestManagerInvokeRefusesDisabledApp(t *testing.T) {
	t.Parallel()
	manager := installManagerHermesPackage(t, false)

	_, err := manager.Invoke(context.Background(), protocol.AppsInvokeRequest{
		AppID:     "hermes",
		CommandID: "chat",
		Input:     json.RawMessage(`{"prompt":"hello"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("Invoke error = %v, want disabled app refusal", err)
	}
}

func TestManagerInvokeRunsEnabledApp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := installManagerHermesPackage(t, true)

	response, err := manager.Invoke(ctx, protocol.AppsInvokeRequest{
		AppID:     "hermes",
		CommandID: "chat",
		Input:     json.RawMessage(`{"prompt":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if string(response.Output) != `{"text":"hello from hermes"}` {
		t.Fatalf("Output = %s", response.Output)
	}
}

func TestManagerInvokePassesContextAndEmitsEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := installManagerHermesPackageWithScript(t, true, hermesRuntimeScriptWithEvent(`{"text":"hello from hermes"}`))
	var events []AppStdioEvent
	manager.SetEventSink(func(ctx context.Context, event string, topic string, payload any) error {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		events = append(events, AppStdioEvent{Event: event, Topic: topic, Body: raw})
		return nil
	})

	response, err := manager.Invoke(ctx, protocol.AppsInvokeRequest{
		AppID:     "hermes",
		CommandID: "chat",
		Input:     json.RawMessage(`{"prompt":"hello"}`),
		Context:   json.RawMessage(`{"trace_id":"trace_1"}`),
	})
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if string(response.Output) != `{"text":"hello from hermes"}` {
		t.Fatalf("Output = %s", response.Output)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one event", events)
	}
	if events[0].Event != "media.download_progress" || events[0].Topic != "media.downloads" || string(events[0].Body) != `{"job_id":"job_1","status":"running"}` {
		t.Fatalf("event = %#v", events[0])
	}
}

func installManagerHermesPackage(t *testing.T, enable bool) *Manager {
	t.Helper()
	return installManagerHermesPackageWithScript(t, enable, hermesRuntimeScript(`{"text":"hello from hermes"}`))
}

func installManagerHermesPackageWithScript(t *testing.T, enable bool, script string) *Manager {
	t.Helper()
	ctx := context.Background()
	appsDir := filepath.Join(t.TempDir(), "apps")
	stagingDir := filepath.Join(t.TempDir(), "staging")
	archivePath := writeManagerHermesPackage(t, t.TempDir(), script)
	manager := NewManager(appsDir, stagingDir, Runner{MaxOutputBytes: 4096, MaxStderrBytes: 1024})
	preview, err := manager.PreviewPackage(ctx, protocol.AppsPackagePreviewRequest{
		StagingID:   "stage_1",
		DownloadURL: fileURL(t, archivePath),
	})
	if err != nil {
		t.Fatalf("PreviewPackage error: %v", err)
	}
	if preview.App.ID != "hermes" {
		t.Fatalf("preview app = %#v", preview.App)
	}
	if _, err := manager.ActivatePackage(ctx, protocol.AppsPackageActivateRequest{
		StagingID: "stage_1",
		Enable:    enable,
	}); err != nil {
		t.Fatalf("ActivatePackage error: %v", err)
	}
	return manager
}

func assertHermesReceivesAPIKey(t *testing.T, manager *Manager, want string) {
	t.Helper()
	response, err := manager.Invoke(context.Background(), protocol.AppsInvokeRequest{
		AppID:     "hermes",
		CommandID: "chat",
		Input:     json.RawMessage(`{"prompt":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if string(response.Output) != `{"api_key":"`+want+`"}` {
		t.Fatalf("Output = %s, want api_key %q", response.Output, want)
	}
}

func fileURL(t *testing.T, path string) string {
	t.Helper()
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func writeManagerHermesPackage(t *testing.T, dir string, script string) string {
	t.Helper()
	manifest := validHermesManifest()
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(dir, "hermes.hankapp")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	writeZipEntry(t, zw, "app.json", string(rawManifest), 0o600)
	writeZipEntry(t, zw, "bin/hermes-app", script, 0o700)
	writeZipEntry(t, zw, "schemas/config.schema.json", `{"type":"object"}`, 0o600)
	writeZipEntry(t, zw, "schemas/chat.input.schema.json", `{"type":"object"}`, 0o600)
	writeZipEntry(t, zw, "schemas/chat.output.schema.json", `{"type":"object"}`, 0o600)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return archivePath
}

func writeZipEntry(t *testing.T, zw *zip.Writer, name string, body string, mode os.FileMode) {
	t.Helper()
	header := &zip.FileHeader{Name: name}
	header.SetMode(mode)
	writer, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
}

func hermesRuntimeScript(output string) string {
	return "#!/bin/sh\nread line\nrequest_id=$(printf '%s' \"$line\" | sed -n 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/p')\nprintf '%s\\n' '{\"request_id\":\"'\"$request_id\"'\",\"ok\":true,\"output\":" + output + "}'\n"
}

func hermesRuntimeScriptWithEvent(output string) string {
	return "#!/bin/sh\nread line\ncase \"$line\" in *'\"trace_id\":\"trace_1\"'*) ;; *) printf '%s\\n' '{\"request_id\":\"req_1\",\"ok\":false,\"error\":{\"code\":\"missing_context\",\"message\":\"missing context\"}}'; exit 0 ;; esac\nrequest_id=$(printf '%s' \"$line\" | sed -n 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/p')\nprintf '%s\\n' '{\"request_id\":\"'\"$request_id\"'\",\"ok\":true,\"output\":" + output + ",\"events\":[{\"event\":\"media.download_progress\",\"topic\":\"media.downloads\",\"body\":{\"job_id\":\"job_1\",\"status\":\"running\"}}]}'\n"
}

func hermesSecretEchoScript() string {
	return "#!/bin/sh\nread line\nrequest_id=$(printf '%s' \"$line\" | sed -n 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/p')\napi_key=$(printf '%s' \"$line\" | sed -n 's/.*\"api_key\":\"\\([^\"]*\\)\".*/\\1/p')\nprintf '%s\\n' '{\"request_id\":\"'\"$request_id\"'\",\"ok\":true,\"output\":{\"api_key\":\"'\"$api_key\"'\"}}'\n"
}
