package apps

import (
	"archive/zip"
	"context"
	"encoding/json"
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

func hermesSecretEchoScript() string {
	return "#!/bin/sh\nread line\nrequest_id=$(printf '%s' \"$line\" | sed -n 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/p')\napi_key=$(printf '%s' \"$line\" | sed -n 's/.*\"api_key\":\"\\([^\"]*\\)\".*/\\1/p')\nprintf '%s\\n' '{\"request_id\":\"'\"$request_id\"'\",\"ok\":true,\"output\":{\"api_key\":\"'\"$api_key\"'\"}}'\n"
}
