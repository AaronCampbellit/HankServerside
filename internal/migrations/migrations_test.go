package migrations

import (
	"errors"
	"testing"
	"time"
)

func TestCheckStatusesAcceptsLegacyVersionOneBaselineChecksum(t *testing.T) {
	t.Parallel()

	if err := CheckStatuses([]Status{{
		Version:    1,
		Name:       "baseline",
		Checksum:   "50f7f293c05c9e0636f07dc8ddba6e0a682ef58d31686c820719a411a07ca035",
		AppliedAt:  time.Now(),
		DurationMS: 0,
	}}); !errors.Is(err, ErrPendingMigrations) {
		t.Fatalf("CheckStatuses legacy baseline = %v, want ErrPendingMigrations for later migrations only", err)
	}
}
