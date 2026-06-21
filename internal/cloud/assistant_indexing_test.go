package cloud

import (
	"encoding/json"
	"testing"
)

func TestAssistantFileIndexSourceIDsFromProfileConfigIncludesLocalAndSMB(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(map[string]any{
		"active_source_id": "hankdemo",
		"file_sources": []map[string]any{
			{"id": "hankdemo", "type": "smb", "smb_enabled": true},
			{"id": "hankdemo2", "type": "smb", "smb_enabled": true},
			{"id": "local", "type": "local", "local_root_enabled": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := assistantFileIndexSourceIDsFromProfileConfig(string(raw))
	want := []string{"hankdemo", "hankdemo2", "local"}
	if !equalStringSlices(got, want) {
		t.Fatalf("source IDs = %#v, want %#v", got, want)
	}
}

func TestAssistantFileIndexSourceIDsFromProfileConfigFallsBackToDefault(t *testing.T) {
	t.Parallel()

	got := assistantFileIndexSourceIDsFromProfileConfig(`{"file_sources":[]}`)
	want := []string{""}
	if !equalStringSlices(got, want) {
		t.Fatalf("source IDs = %#v, want %#v", got, want)
	}
}

func TestAssistantFileIndexSourceIDsFromProfileConfigAcceptsLegacyShares(t *testing.T) {
	t.Parallel()

	got := assistantFileIndexSourceIDsFromProfileConfig(`{"shares":[{"id":"archive","host":"nas.local","share":"Archive"}]}`)
	want := []string{"archive"}
	if !equalStringSlices(got, want) {
		t.Fatalf("source IDs = %#v, want %#v", got, want)
	}
}

func TestAssistantFileIndexListRequestPreservesSourceID(t *testing.T) {
	t.Parallel()

	request := assistantFileIndexListRequest("local", "Documents/Taxes")
	if request.SourceID != "local" || request.Path != "Documents/Taxes" {
		t.Fatalf("request = %#v, want source-aware file list request", request)
	}
}

func equalStringSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
