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
