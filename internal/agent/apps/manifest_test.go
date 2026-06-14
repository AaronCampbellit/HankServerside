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

func TestValidateManifestAcceptsTypedSettingsAndFileSourcePermission(t *testing.T) {
	t.Parallel()
	manifest := validHermesManifest()
	manifest.Config.Settings = SettingsSchema{
		Fields: []SettingsField{
			{
				Key:      "api_base_url",
				Label:    "Hermes URL",
				Type:     "url",
				Required: true,
			},
			{
				Key:       "api_key",
				Label:     "API key",
				Type:      "password",
				Secret:    true,
				SecretKey: "api_key",
			},
			{
				Key:    "source_id",
				Label:  "Media source",
				Type:   "select",
				Source: "file_sources",
			},
		},
	}
	manifest.Permissions.Files = []FilePermission{{
		Kind:  "configured_source",
		Field: "source_id",
	}}

	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest error: %v", err)
	}
}

func TestValidateManifestRejectsInvalidSettingsFields(t *testing.T) {
	t.Parallel()
	manifest := validHermesManifest()
	manifest.Config.Settings = SettingsSchema{
		Fields: []SettingsField{{
			Key:  "../bad",
			Type: "text",
		}},
	}

	err := ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "settings field") {
		t.Fatalf("ValidateManifest error = %v, want settings field", err)
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

func TestValidateManifestRejectsDuplicateSlashCommands(t *testing.T) {
	t.Parallel()
	manifest := validHermesManifest()
	manifest.Assistant.SlashCommands = append(manifest.Assistant.SlashCommands, SlashCommand{
		Command:     "/Hermes",
		CommandID:   "chat",
		Description: "Duplicate route.",
	})
	err := ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "duplicate slash command") {
		t.Fatalf("ValidateManifest error = %v, want duplicate slash command", err)
	}
}

func TestValidateManifestRejectsReservedSlashCommands(t *testing.T) {
	t.Parallel()
	manifest := validHermesManifest()
	manifest.Assistant.SlashCommands[0].Command = "/files"
	err := ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "reserved slash command") {
		t.Fatalf("ValidateManifest error = %v, want reserved slash command", err)
	}
}

func TestValidateManifestRejectsInvalidSettingsDefaultsAndOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		field SettingsField
		want  string
	}{
		{
			name: "boolean default string",
			field: SettingsField{
				Key:     "enabled",
				Type:    "boolean",
				Default: json.RawMessage(`"true"`),
			},
			want: "default",
		},
		{
			name: "number default string",
			field: SettingsField{
				Key:     "timeout_seconds",
				Type:    "number",
				Default: json.RawMessage(`"900"`),
			},
			want: "default",
		},
		{
			name: "text default boolean",
			field: SettingsField{
				Key:     "model",
				Type:    "text",
				Default: json.RawMessage(`false`),
			},
			want: "default",
		},
		{
			name: "select option object",
			field: SettingsField{
				Key:  "format",
				Type: "select",
				Options: []SettingsOption{{
					Value: json.RawMessage(`{"bad":true}`),
					Label: "Bad",
				}},
			},
			want: "option",
		},
		{
			name: "select default outside options",
			field: SettingsField{
				Key:     "format",
				Type:    "select",
				Default: json.RawMessage(`"missing"`),
				Options: []SettingsOption{{
					Value: json.RawMessage(`"best"`),
					Label: "Best",
				}},
			},
			want: "default must match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validHermesManifest()
			manifest.Config.Settings = SettingsSchema{Fields: []SettingsField{tt.field}}
			err := ValidateManifest(manifest)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateManifest error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateManifestAcceptsScalarSelectDefaultsAndEmptyOptions(t *testing.T) {
	t.Parallel()
	manifest := validHermesManifest()
	manifest.Config.Settings = SettingsSchema{
		Fields: []SettingsField{
			{
				Key:     "rate_limit",
				Type:    "select",
				Default: json.RawMessage(`""`),
				Options: []SettingsOption{
					{Value: json.RawMessage(`""`), Label: "No limit"},
					{Value: json.RawMessage(`"10M"`), Label: "10 MB/s"},
				},
			},
			{
				Key:     "timeout_seconds",
				Type:    "select",
				Default: json.RawMessage(`900`),
				Options: []SettingsOption{
					{Value: json.RawMessage(`300`), Label: "5 minutes"},
					{Value: json.RawMessage(`900`), Label: "15 minutes"},
				},
			},
		},
	}

	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest error: %v", err)
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

func TestPreviewArchiveRejectsTrailingManifestJSON(t *testing.T) {
	t.Parallel()
	manifest := validHermesManifest()
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "trailing-json.hankapp")
	writeArchiveEntries(t, archivePath, hermesPackageEntries(string(rawManifest)+"\n{}"))

	_, err = PreviewArchive(archivePath)
	if err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("PreviewArchive error = %v, want trailing JSON", err)
	}
}

func TestPreviewArchiveRejectsOversizedManifest(t *testing.T) {
	t.Parallel()
	manifest := validHermesManifest()
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "oversized-manifest.hankapp")
	writeArchiveEntries(t, archivePath, hermesPackageEntries(string(rawManifest)+strings.Repeat(" ", 128*1024)))

	_, err = PreviewArchive(archivePath)
	if err == nil || !strings.Contains(err.Error(), "app.json too large") {
		t.Fatalf("PreviewArchive error = %v, want app.json too large", err)
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
	writeArchiveEntries(t, archivePath, hermesPackageEntries(string(rawManifest)))

	preview, err := PreviewArchive(archivePath)
	if err != nil {
		t.Fatalf("PreviewArchive error: %v", err)
	}
	if preview.Manifest.ID != "hermes" || preview.Manifest.Commands[0].ID != "chat" {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestPreviewArchiveRejectsUnsafeArchivePaths(t *testing.T) {
	t.Parallel()
	manifest := validHermesManifest()
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	validEntries := func() []archiveEntry {
		return archiveEntriesFromMap(hermesPackageEntries(string(rawManifest)))
	}
	tests := []struct {
		name    string
		entries []archiveEntry
		want    string
	}{
		{
			name:    "absolute POSIX path",
			entries: []archiveEntry{{Name: "/tmp/escape", Body: "bad"}},
			want:    "unsafe archive path",
		},
		{
			name:    "Windows volume path",
			entries: []archiveEntry{{Name: "C:/tmp/escape", Body: "bad"}},
			want:    "unsafe archive path",
		},
		{
			name:    "backslash path",
			entries: []archiveEntry{{Name: `dir\file`, Body: "bad"}},
			want:    "unsafe archive path",
		},
		{
			name:    "dot relative path",
			entries: []archiveEntry{{Name: "./app.json", Body: "bad"}},
			want:    "unsafe archive path",
		},
		{
			name:    "double slash path",
			entries: []archiveEntry{{Name: "dir//file", Body: "bad"}},
			want:    "unsafe archive path",
		},
		{
			name:    "directory and file same path",
			entries: []archiveEntry{{Name: "dir/", Body: ""}, {Name: "dir", Body: "bad"}},
			want:    "duplicate archive path",
		},
		{
			name:    "file contains child path",
			entries: append(validEntries(), archiveEntry{Name: "bin", Body: "not a directory"}),
			want:    "archive path collision",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := filepath.Join(t.TempDir(), strings.ReplaceAll(tt.name, " ", "-")+".hankapp")
			writeArchiveEntryList(t, archivePath, tt.entries)
			_, err := PreviewArchive(archivePath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("PreviewArchive error = %v, want containing %q", err, tt.want)
			}
		})
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

type archiveEntry struct {
	Name string
	Body string
}

func hermesPackageEntries(rawManifest string) map[string]string {
	return map[string]string{
		"app.json":                        rawManifest,
		"bin/hermes-app":                  "#!/bin/sh\n",
		"schemas/config.schema.json":      `{"type":"object"}`,
		"schemas/chat.input.schema.json":  `{"type":"object"}`,
		"schemas/chat.output.schema.json": `{"type":"object"}`,
	}
}

func archiveEntriesFromMap(entries map[string]string) []archiveEntry {
	list := make([]archiveEntry, 0, len(entries))
	for name, body := range entries {
		list = append(list, archiveEntry{Name: name, Body: body})
	}
	return list
}

func writeArchiveEntries(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()
	writeArchiveEntryList(t, archivePath, archiveEntriesFromMap(entries))
}

func writeArchiveEntryList(t *testing.T, archivePath string, entries []archiveEntry) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for _, entry := range entries {
		writer, err := zw.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(entry.Body)); err != nil {
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
