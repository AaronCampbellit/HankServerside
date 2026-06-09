package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadCloudDefaults(t *testing.T) {
	t.Setenv("HANK_REMOTE_CLOUD_ADDR", "")
	t.Setenv("HANK_REMOTE_CLOUD_DATABASE_URL", "")
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "test-db-ops-intent-secret")
	t.Setenv("HANK_REMOTE_SECRET_ENCRYPTION_KEY", "test-secret-encryption-key")
	t.Setenv("HANK_REMOTE_SESSION_TTL_SECONDS", "")
	t.Setenv("HANK_REMOTE_REQUEST_TIMEOUT_SECONDS", "")
	t.Setenv("HANK_REMOTE_MAINTENANCE_INTERVAL_SECONDS", "")
	t.Setenv("HANK_REMOTE_MAINTENANCE_RETENTION_DAYS", "")
	t.Setenv("HANK_REMOTE_CHATGPT_OAUTH_ENABLED", "")
	t.Setenv("HANK_REMOTE_CHATGPT_AUTH_ISSUER", "")
	t.Setenv("HANK_REMOTE_CHATGPT_BACKEND_BASE_URL", "")
	t.Setenv("HANK_REMOTE_CHATGPT_CLIENT_ID", "")
	t.Setenv("HANK_REMOTE_CHATGPT_CHAT_MODEL", "")
	t.Setenv("HANK_REMOTE_PROJECT_DOCS_DIR", "")

	cfg, err := LoadCloud()
	if err != nil {
		t.Fatalf("LoadCloud error: %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if !strings.Contains(cfg.DatabaseURL, "postgres://") {
		t.Fatalf("DatabaseURL = %q, want default postgres URL", cfg.DatabaseURL)
	}
	if cfg.SessionTTL != 7*24*time.Hour {
		t.Fatalf("SessionTTL = %s, want %s", cfg.SessionTTL, 7*24*time.Hour)
	}
	if cfg.RequestTimeout != 120*time.Second {
		t.Fatalf("RequestTimeout = %s, want %s", cfg.RequestTimeout, 120*time.Second)
	}
	if cfg.MaintenanceInterval != time.Hour {
		t.Fatalf("MaintenanceInterval = %s, want %s", cfg.MaintenanceInterval, time.Hour)
	}
	if cfg.MaintenanceRetention != 30*24*time.Hour {
		t.Fatalf("MaintenanceRetention = %s, want %s", cfg.MaintenanceRetention, 30*24*time.Hour)
	}
	if cfg.AssistantAI.ChatGPTOAuthEnabled {
		t.Fatal("ChatGPTOAuthEnabled = true, want false by default")
	}
	if cfg.AssistantAI.ChatGPTAuthIssuer != "https://auth.openai.com" {
		t.Fatalf("ChatGPTAuthIssuer = %q", cfg.AssistantAI.ChatGPTAuthIssuer)
	}
	if cfg.AssistantAI.ChatGPTBackendBaseURL != "https://chatgpt.com/backend-api/codex" {
		t.Fatalf("ChatGPTBackendBaseURL = %q", cfg.AssistantAI.ChatGPTBackendBaseURL)
	}
	if cfg.AssistantAI.ChatGPTClientID != "app_EMoamEEZ73f0CkXaXp7hrann" {
		t.Fatalf("ChatGPTClientID = %q", cfg.AssistantAI.ChatGPTClientID)
	}
	if cfg.AssistantAI.ChatGPTChatModel != "gpt-5.4-mini" {
		t.Fatalf("ChatGPTChatModel = %q", cfg.AssistantAI.ChatGPTChatModel)
	}
	if cfg.AssistantAI.ProjectDocsDir != "." {
		t.Fatalf("ProjectDocsDir = %q", cfg.AssistantAI.ProjectDocsDir)
	}
}

func TestLoadCloudParsesChatGPTOAuthConfig(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "test-db-ops-intent-secret")
	t.Setenv("HANK_REMOTE_SECRET_ENCRYPTION_KEY", "test-secret-encryption-key")
	t.Setenv("HANK_REMOTE_CHATGPT_OAUTH_ENABLED", "true")
	t.Setenv("HANK_REMOTE_CHATGPT_AUTH_ISSUER", " https://auth.example.com/ ")
	t.Setenv("HANK_REMOTE_CHATGPT_BACKEND_BASE_URL", " https://chatgpt.example.com/backend-api/codex/ ")
	t.Setenv("HANK_REMOTE_CHATGPT_CLIENT_ID", "test-client")
	t.Setenv("HANK_REMOTE_CHATGPT_CHAT_MODEL", "gpt-test")
	t.Setenv("HANK_REMOTE_PROJECT_DOCS_DIR", "/srv/hank/docs")

	cfg, err := LoadCloud()
	if err != nil {
		t.Fatalf("LoadCloud error: %v", err)
	}
	if !cfg.AssistantAI.ChatGPTOAuthEnabled {
		t.Fatal("ChatGPTOAuthEnabled = false, want true")
	}
	if cfg.AssistantAI.ChatGPTAuthIssuer != "https://auth.example.com" {
		t.Fatalf("ChatGPTAuthIssuer = %q", cfg.AssistantAI.ChatGPTAuthIssuer)
	}
	if cfg.AssistantAI.ChatGPTBackendBaseURL != "https://chatgpt.example.com/backend-api/codex" {
		t.Fatalf("ChatGPTBackendBaseURL = %q", cfg.AssistantAI.ChatGPTBackendBaseURL)
	}
	if cfg.AssistantAI.ChatGPTClientID != "test-client" {
		t.Fatalf("ChatGPTClientID = %q", cfg.AssistantAI.ChatGPTClientID)
	}
	if cfg.AssistantAI.ChatGPTChatModel != "gpt-test" {
		t.Fatalf("ChatGPTChatModel = %q", cfg.AssistantAI.ChatGPTChatModel)
	}
	if cfg.AssistantAI.ProjectDocsDir != "/srv/hank/docs" {
		t.Fatalf("ProjectDocsDir = %q", cfg.AssistantAI.ProjectDocsDir)
	}
}

func TestLoadCloudRejectsInvalidDuration(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "test-db-ops-intent-secret")
	t.Setenv("HANK_REMOTE_SECRET_ENCRYPTION_KEY", "test-secret-encryption-key")
	t.Setenv("HANK_REMOTE_SESSION_TTL_SECONDS", "0")

	_, err := LoadCloud()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_SESSION_TTL_SECONDS") {
		t.Fatalf("LoadCloud error = %v, want session ttl validation error", err)
	}
}

func TestLoadCloudRejectsInvalidMaintenanceRetention(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "test-db-ops-intent-secret")
	t.Setenv("HANK_REMOTE_SECRET_ENCRYPTION_KEY", "test-secret-encryption-key")
	t.Setenv("HANK_REMOTE_MAINTENANCE_RETENTION_DAYS", "0")

	_, err := LoadCloud()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_MAINTENANCE_RETENTION_DAYS") {
		t.Fatalf("LoadCloud error = %v, want maintenance retention validation error", err)
	}
}

func TestLoadCloudRequiresDBOpsIntentSecret(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "")

	_, err := LoadCloud()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_DB_OPS_INTENT_SECRET") {
		t.Fatalf("LoadCloud error = %v, want missing db ops intent secret error", err)
	}
}

func TestLoadCloudRequiresSecretEncryptionKeyUnlessPlaintextOptOut(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "test-db-ops-intent-secret")
	t.Setenv("HANK_REMOTE_SECRET_ENCRYPTION_KEY", "")
	t.Setenv("HANK_REMOTE_ALLOW_PLAINTEXT_SECRETS", "")

	_, err := LoadCloud()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_SECRET_ENCRYPTION_KEY") {
		t.Fatalf("LoadCloud error = %v, want missing secret encryption key error", err)
	}

	t.Setenv("HANK_REMOTE_ALLOW_PLAINTEXT_SECRETS", "true")
	cfg, err := LoadCloud()
	if err != nil {
		t.Fatalf("LoadCloud with plaintext opt-out error: %v", err)
	}
	if !cfg.AllowPlaintextSecrets {
		t.Fatal("AllowPlaintextSecrets = false, want true for explicit dev opt-out")
	}
}

func TestLoadDBOpsRequiresIntentSecret(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "")
	t.Setenv("HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS", "cipher-secret")

	_, err := LoadDBOps()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_DB_OPS_INTENT_SECRET") {
		t.Fatalf("LoadDBOps error = %v, want missing intent secret error", err)
	}
}

func TestLoadDBOpsRequiresRepoCipherPass(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "test-db-ops-intent-secret")
	t.Setenv("HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS", "")

	_, err := LoadDBOps()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS") {
		t.Fatalf("LoadDBOps error = %v, want missing cipher pass error", err)
	}
}

func TestLoadDBOpsParsesRepoCipherPass(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "test-db-ops-intent-secret")
	t.Setenv("HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS", " cipher-secret ")

	cfg, err := LoadDBOps()
	if err != nil {
		t.Fatalf("LoadDBOps error: %v", err)
	}
	if cfg.RepoCipherPass != "cipher-secret" {
		t.Fatalf("RepoCipherPass = %q, want trimmed cipher pass", cfg.RepoCipherPass)
	}
}

func TestLoadAgentRequiresIdentityAndToken(t *testing.T) {
	t.Setenv("HANK_REMOTE_AGENT_ID", "")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "")

	_, err := LoadAgent()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_AGENT_ID") {
		t.Fatalf("LoadAgent error = %v, want missing agent id error", err)
	}

	t.Setenv("HANK_REMOTE_AGENT_ID", "home-main")

	_, err = LoadAgent()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_AGENT_TOKEN") {
		t.Fatalf("LoadAgent error = %v, want missing agent token error", err)
	}
}

func TestLoadAgentParsesValidConfig(t *testing.T) {
	t.Setenv("HANK_REMOTE_AGENT_CLOUD_URL", "ws://cloud.example/ws/agent")
	t.Setenv("HANK_REMOTE_AGENT_ID", "home-main")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "secret-token")
	t.Setenv("HANK_REMOTE_AGENT_HOME_NAME", "Campbell Home")
	t.Setenv("HANK_REMOTE_HA_BASE_URL", "http://127.0.0.1:8123")
	t.Setenv("HANK_REMOTE_HA_TOKEN", "ha-token")
	t.Setenv("HANK_REMOTE_HA_TIMEOUT_SECONDS", "12")
	t.Setenv("HANK_REMOTE_SMB_SHARES_JSON", `[{"id":"media","host":"192.168.1.20","share":"media","username":"aaron","password":"secret","domain":"WORKGROUP"}]`)
	t.Setenv("HANK_REMOTE_AGENT_FILES_ROOT", "/srv/hank/files")
	t.Setenv("HANK_REMOTE_AGENT_NOTES_ROOT", "/srv/hank/notes")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent error: %v", err)
	}

	if cfg.CloudURL != "ws://cloud.example/ws/agent" {
		t.Fatalf("CloudURL = %q", cfg.CloudURL)
	}
	if cfg.AgentID != "home-main" || cfg.Token != "secret-token" {
		t.Fatalf("agent identity = %#v", cfg)
	}
	if cfg.HA.Timeout != 12*time.Second {
		t.Fatalf("HA.Timeout = %s, want %s", cfg.HA.Timeout, 12*time.Second)
	}
	if len(cfg.SMBShares) != 1 || cfg.SMBShares[0].Share != "media" {
		t.Fatalf("SMBShares = %#v, want JSON SMB share", cfg.SMBShares)
	}
	if cfg.FilesRoot != "/srv/hank/files" || cfg.NotesRoot != "/srv/hank/notes" {
		t.Fatalf("roots = files:%q notes:%q", cfg.FilesRoot, cfg.NotesRoot)
	}
}

func TestLoadAgentParsesAppRuntimePaths(t *testing.T) {
	t.Setenv("HANK_REMOTE_AGENT_CLOUD_URL", "ws://cloud.example/ws/agent")
	t.Setenv("HANK_REMOTE_AGENT_ID", "home-main")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "secret-token")
	t.Setenv("HANK_REMOTE_AGENT_APPS_DIR", " /srv/hank/apps ")
	t.Setenv("HANK_REMOTE_AGENT_APP_STAGING_DIR", " /srv/hank/app-staging ")

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent error: %v", err)
	}
	if cfg.AppsDir != "/srv/hank/apps" {
		t.Fatalf("AppsDir = %q", cfg.AppsDir)
	}
	if cfg.AppStagingDir != "/srv/hank/app-staging" {
		t.Fatalf("AppStagingDir = %q", cfg.AppStagingDir)
	}
}

func TestLoadAgentAppRuntimePathDefaults(t *testing.T) {
	t.Setenv("HANK_REMOTE_AGENT_CLOUD_URL", "ws://cloud.example/ws/agent")
	t.Setenv("HANK_REMOTE_AGENT_ID", "home-main")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "secret-token")
	t.Setenv("HANK_REMOTE_AGENT_APPS_DIR", "")
	t.Setenv("HANK_REMOTE_AGENT_APP_STAGING_DIR", "")

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent error: %v", err)
	}
	if cfg.AppsDir != "/var/lib/hank/apps" {
		t.Fatalf("AppsDir = %q", cfg.AppsDir)
	}
	if cfg.AppStagingDir != "/var/lib/hank/app-staging" {
		t.Fatalf("AppStagingDir = %q", cfg.AppStagingDir)
	}
}

func TestLoadAgentIgnoresLegacySingleShareSMBEnv(t *testing.T) {
	t.Setenv("HANK_REMOTE_AGENT_ID", "home-main")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "secret-token")
	t.Setenv("HANK_REMOTE_SMB_HOST", "192.168.1.20")
	t.Setenv("HANK_REMOTE_SMB_SHARE", "media")
	t.Setenv("HANK_REMOTE_SMB_USERNAME", "aaron")
	t.Setenv("HANK_REMOTE_SMB_PASSWORD", "secret")
	t.Setenv("HANK_REMOTE_SMB_DOMAIN", "WORKGROUP")

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent error: %v", err)
	}
	if len(cfg.SMBShares) != 0 {
		t.Fatalf("SMBShares = %#v, want legacy single-share env ignored", cfg.SMBShares)
	}
}

func TestLoadAgentParsesMultipleSMBSharesJSON(t *testing.T) {
	t.Setenv("HANK_REMOTE_AGENT_ID", "home-main")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "secret-token")
	t.Setenv("HANK_REMOTE_SMB_SHARES_JSON", `[
		{"id":"media","name":"Media","host":"192.168.1.20","share":"media","username":"aaron","password":"media-secret","domain":"WORKGROUP"},
		{"id":"archive","name":"Archive","host":"192.168.1.21","share":"archive","username":"aaron","password":"archive-secret"}
	]`)

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent error: %v", err)
	}

	if len(cfg.SMBShares) != 2 {
		t.Fatalf("SMBShares count = %d, want 2", len(cfg.SMBShares))
	}
	if cfg.SMBShares[0].ID != "media" || cfg.SMBShares[0].Password != "media-secret" {
		t.Fatalf("first SMB share = %#v, want media with password", cfg.SMBShares[0])
	}
	if cfg.SMBShares[1].ID != "archive" || cfg.SMBShares[1].Share != "archive" {
		t.Fatalf("second SMB share = %#v, want archive", cfg.SMBShares[1])
	}
}
