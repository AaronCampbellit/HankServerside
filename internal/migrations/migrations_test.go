package migrations

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
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

func TestHomeServiceProfileConstraintIncludesSupportedServiceTypes(t *testing.T) {
	t.Parallel()

	migrations, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	var constraintText strings.Builder
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			if strings.Contains(statement, "home_service_profiles_service_type_check") {
				constraintText.WriteString(statement)
				constraintText.WriteByte('\n')
			}
		}
	}
	body := constraintText.String()
	if body == "" {
		t.Fatal("home_service_profiles_service_type_check migration statement missing")
	}
	for _, serviceType := range []string{domain.ServiceTypeHomeAssistant, domain.ServiceTypeSMB, domain.ServiceTypeHermes} {
		if !strings.Contains(body, "'"+serviceType+"'") {
			t.Fatalf("home_service_profiles_service_type_check missing %q in:\n%s", serviceType, body)
		}
	}
}

func TestRequiredPostgresExtensionsAreCreatedByMigrations(t *testing.T) {
	t.Parallel()

	migrations, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	var body strings.Builder
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			body.WriteString(statement)
			body.WriteByte('\n')
		}
	}
	all := body.String()
	for _, extension := range []string{
		"vector",
		"pg_trgm",
		"pg_stat_statements",
		"pg_buffercache",
		"amcheck",
	} {
		if !strings.Contains(all, "CREATE EXTENSION IF NOT EXISTS "+extension) {
			t.Fatalf("required extension %q is not created by migrations", extension)
		}
	}
	for _, extension := range []string{
		"pgaudit",
		"pgmq",
		"pg_partman",
	} {
		if strings.Contains(all, "CREATE EXTENSION IF NOT EXISTS "+extension) {
			t.Fatalf("deferred extension %q should not be created by migrations", extension)
		}
	}
}
