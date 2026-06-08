package apps

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validHermesManifest() Manifest {
	return Manifest{
		SchemaVersion: "hank.app.v1",
		ID:            "hermes",
		Name:          "Hermes",
		Version:       "1.0.0",
		Publisher:     "Hank",
		Description:   "Route explicit /Hermes prompts to a local Hermes API server.",
		Runtime: Runtime{
			Type:    "stdio",
			Command: "bin/hermes-app",
		},
		Assistant: Assistant{
			SlashCommands: []SlashCommand{{
				Command:     "/Hermes",
				CommandID:   "chat",
				Description: "Send a prompt to Hermes.",
			}},
		},
		Commands: []Command{{
			ID:             "chat",
			Mode:           "request_response",
			InputSchema:    "schemas/chat.input.schema.json",
			OutputSchema:   "schemas/chat.output.schema.json",
			TimeoutSeconds: 120,
			AdminOnly:      true,
		}},
		Config: Config{
			Schema:       "schemas/config.schema.json",
			SecretFields: []string{"api_key"},
		},
		Permissions: Permissions{
			Network: []NetworkPermission{{
				Kind:  "configured_base_url",
				Field: "api_base_url",
			}},
		},
	}
}

func TestValidateManifestAcceptsHermesShape(t *testing.T) {
	t.Parallel()
	if err := ValidateManifest(validHermesManifest()); err != nil {
		t.Fatalf("ValidateManifest error: %v", err)
	}
}

func TestValidateManifestRejectsUnsafeIDsAndPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"bad app id", func(m *Manifest) { m.ID = "../bad" }, "app id"},
		{"bad command path", func(m *Manifest) { m.Runtime.Command = "../bad" }, "runtime command"},
		{"bad schema path", func(m *Manifest) { m.Commands[0].InputSchema = "/tmp/schema.json" }, "schema path"},
		{"unknown permission", func(m *Manifest) { m.Permissions.Network[0].Kind = "internet" }, "permission"},
		{"missing command", func(m *Manifest) { m.Commands = nil }, "command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validHermesManifest()
			tt.mutate(&manifest)
			err := ValidateManifest(manifest)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateManifest error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestPreviewArchiveRejectsTraversal(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "bad.hankapp")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	writer, err := zw.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = PreviewArchive(archivePath)
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("PreviewArchive error = %v", err)
	}
}

func TestPreviewArchiveAcceptsHermesPackage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
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
	for name, body := range map[string]string{
		"app.json":                        string(rawManifest),
		"bin/hermes-app":                  "#!/bin/sh\n",
		"schemas/config.schema.json":      `{"type":"object"}`,
		"schemas/chat.input.schema.json":  `{"type":"object"}`,
		"schemas/chat.output.schema.json": `{"type":"object"}`,
	} {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewArchive(archivePath)
	if err != nil {
		t.Fatalf("PreviewArchive error: %v", err)
	}
	if preview.Manifest.ID != "hermes" || preview.Manifest.Commands[0].ID != "chat" {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestPreviewArchiveRejectsDuplicateAndSymlinkEntries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		write func(*testing.T, *zip.Writer)
		want  string
	}{
		{
			name: "duplicate path",
			write: func(t *testing.T, zw *zip.Writer) {
				t.Helper()
				for i := 0; i < 2; i++ {
					writer, err := zw.Create("app.json")
					if err != nil {
						t.Fatal(err)
					}
					if _, err := writer.Write([]byte("{}")); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: "duplicate archive path",
		},
		{
			name: "symlink",
			write: func(t *testing.T, zw *zip.Writer) {
				t.Helper()
				header := &zip.FileHeader{Name: "link"}
				header.SetMode(os.ModeSymlink | 0o777)
				writer, err := zw.CreateHeader(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Write([]byte("app.json")); err != nil {
					t.Fatal(err)
				}
			},
			want: "symlink",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := filepath.Join(t.TempDir(), "bad.hankapp")
			file, err := os.Create(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(file)
			tt.write(t, zw)
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			_, err = PreviewArchive(archivePath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("PreviewArchive error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestPreviewArchiveRequiresReferencedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifest := validHermesManifest()
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		entries map[string]string
		want    string
	}{
		{
			name: "missing runtime command",
			entries: map[string]string{
				"app.json": string(rawManifest),
			},
			want: "runtime command",
		},
		{
			name: "missing schema",
			entries: map[string]string{
				"app.json":       string(rawManifest),
				"bin/hermes-app": "#!/bin/sh\n",
			},
			want: "schema path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-")+".hankapp")
			writeArchiveEntries(t, archivePath, tt.entries)
			_, err := PreviewArchive(archivePath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("PreviewArchive error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func writeArchiveEntries(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, body := range entries {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
