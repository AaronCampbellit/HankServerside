package cloud

import (
	"encoding/json"
	"testing"

	"github.com/dropfile/hankremote/internal/domain"
)

func TestNoteAttachmentResponseIncludesPostUploadRevision(t *testing.T) {
	response := noteAttachmentToProtocol(
		domain.NoteAttachment{ID: "natt-1", Filename: "capture.png", ContentType: "image/png"},
		domain.UserNote{NoteID: "work", Revision: "revision-after-upload"},
		"profile",
	)
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal attachment response: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode attachment response: %v", err)
	}
	if got := decoded["note_revision"]; got != "revision-after-upload" {
		t.Fatalf("note_revision = %#v, want post-upload revision", got)
	}
}
