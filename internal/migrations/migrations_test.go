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

func TestAgentTypeMigrationEnforcesOnePrimaryPerHome(t *testing.T) {
	t.Parallel()

	migrations, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	var body strings.Builder
	for _, migration := range migrations {
		if migration.Version != 20 {
			continue
		}
		for _, statement := range migration.Statements {
			body.WriteString(statement)
			body.WriteByte('\n')
		}
	}
	text := body.String()
	for _, required := range []string{
		"ROW_NUMBER() OVER (PARTITION BY home_id",
		"CHECK (agent_type IN ('primary', 'worker'))",
		"CREATE UNIQUE INDEX IF NOT EXISTS agents_one_primary_per_home_idx",
		"WHERE agent_type = 'primary'",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("agent type migration missing %q in:\n%s", required, text)
		}
	}
}

func TestRemoteDesktopFoundationMigrationHasSecurityConstraints(t *testing.T) {
	t.Parallel()

	migrations, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	var body strings.Builder
	for _, migration := range migrations {
		if migration.Version != 21 {
			continue
		}
		for _, statement := range migration.Statements {
			body.WriteString(statement)
			body.WriteByte('\n')
		}
	}
	text := body.String()
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS desktop_trust_roots",
		"CREATE TABLE IF NOT EXISTS desktop_identities",
		"CREATE TABLE IF NOT EXISTS desktop_sessions",
		"CREATE TABLE IF NOT EXISTS desktop_join_credentials",
		"CREATE TABLE IF NOT EXISTS desktop_session_events",
		"desktop_sessions_one_live_operator_idx",
		"credential_hash BYTEA NOT NULL UNIQUE",
		"CHECK (side IN ('browser', 'agent'))",
		"CHECK (state IN ('requested', 'offered', 'agent_ready', 'joining', 'active', 'reconnecting', 'denied', 'failed', 'expired', 'terminated'))",
		"CHECK (requested_permissions <@ ARRAY['desktop.view'",
		"CHECK (key_epoch > 0)",
		"FOREIGN KEY (home_id, agent_id) REFERENCES agents(home_id, id)",
		"FOREIGN KEY (home_id, operator_user_id, operator_device_identity_id)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration 21 missing %q", required)
		}
	}
}
