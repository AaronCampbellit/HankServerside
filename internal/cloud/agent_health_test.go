package cloud

import (
	"testing"

	"github.com/dropfile/hankremote/internal/protocol"
)

func TestManagementCommandsRequireAdmin(t *testing.T) {
	t.Parallel()

	commands := []string{
		"host.lock",
		"host.status",
		"shell.exec",
		"wol.send",
		protocol.CommandSystemRestart,
	}
	for _, command := range commands {
		if !isManagementCommand(command) {
			t.Errorf("isManagementCommand(%q) = false, want true", command)
		}
	}
}

func TestNoteSyncSchedulingRequiresRegisteredPrimary(t *testing.T) {
	t.Parallel()

	capabilities := []string{"notes.sync"}
	if shouldScheduleNoteSync(false, capabilities) {
		t.Fatal("pre-registration or worker heartbeat scheduled note sync")
	}
	if !shouldScheduleNoteSync(true, capabilities) {
		t.Fatal("registered primary with notes.sync did not schedule note sync")
	}
	if shouldScheduleNoteSync(true, []string{"files.list"}) {
		t.Fatal("primary without notes.sync scheduled note sync")
	}
}
