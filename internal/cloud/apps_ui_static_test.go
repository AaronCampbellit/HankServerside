package cloud

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestYDownloadSettingsUseDropdownsForBoundedInputs(t *testing.T) {
	data, err := os.ReadFile("../../packages/ydownload/app.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Config struct {
			SettingsSchema struct {
				Fields []struct {
					Key     string           `json:"key"`
					Type    string           `json:"type"`
					Options []map[string]any `json:"options"`
				} `json:"fields"`
			} `json:"settings_schema"`
		} `json:"config"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}

	fields := map[string]struct {
		Type    string
		Options []map[string]any
	}{}
	for _, field := range manifest.Config.SettingsSchema.Fields {
		fields[field.Key] = struct {
			Type    string
			Options []map[string]any
		}{Type: field.Type, Options: field.Options}
	}

	for _, key := range []string{
		"destination_path",
		"format",
		"output_template",
		"subtitle_languages",
		"subtitle_format",
		"rate_limit",
		"cookies_file_path",
		"yt_dlp_path",
		"timeout_seconds",
	} {
		field, ok := fields[key]
		if !ok {
			t.Fatalf("missing ydownload settings field %q", key)
		}
		if field.Type != "select" {
			t.Fatalf("field %q type = %q, want select", key, field.Type)
		}
		if len(field.Options) == 0 {
			t.Fatalf("field %q has no dropdown options", key)
		}
	}
}

func TestAppsSettingsUIUsesInstallModalAndAppDropdown(t *testing.T) {
	htmlData, err := os.ReadFile("ui/apps.html")
	if err != nil {
		t.Fatal(err)
	}
	jsData, err := os.ReadFile("ui/apps.js")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	js := string(jsData)

	for _, required := range []string{
		`id="app-install-open-button"`,
		`id="app-install-dialog"`,
		`id="installed-app-select"`,
		`id="selected-app-panel"`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("apps.html missing %s", required)
		}
	}
	for _, oldPattern := range []string{
		`id="apps-list"`,
		`id="app-import-form" class="stack"`,
		`<h2>Import App</h2>`,
	} {
		if strings.Contains(html, oldPattern) {
			t.Fatalf("apps.html still contains old pattern %s", oldPattern)
		}
	}
	for _, required := range []string{
		"openInstallDialog",
		"renderAppSelector",
		"renderSelectedAppPanel",
		"installedAppSelect",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("apps.js missing %s", required)
		}
	}
}
