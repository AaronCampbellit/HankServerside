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
	t.Setenv("HANK_REMOTE_SESSION_TTL_SECONDS", "")
	t.Setenv("HANK_REMOTE_REQUEST_TIMEOUT_SECONDS", "")
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
	t.Setenv("HANK_REMOTE_SESSION_TTL_SECONDS", "0")

	_, err := LoadCloud()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_SESSION_TTL_SECONDS") {
		t.Fatalf("LoadCloud error = %v, want session ttl validation error", err)
	}
}

func TestLoadCloudRequiresDBOpsIntentSecret(t *testing.T) {
	t.Setenv("HANK_REMOTE_DB_OPS_INTENT_SECRET", "")

	_, err := LoadCloud()
	if err == nil || !strings.Contains(err.Error(), "HANK_REMOTE_DB_OPS_INTENT_SECRET") {
		t.Fatalf("LoadCloud error = %v, want missing db ops intent secret error", err)
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
	t.Setenv("HANK_REMOTE_SMB_HOST", "192.168.1.20")
	t.Setenv("HANK_REMOTE_SMB_SHARE", "media")
	t.Setenv("HANK_REMOTE_SMB_USERNAME", "aaron")
	t.Setenv("HANK_REMOTE_SMB_PASSWORD", "secret")
	t.Setenv("HANK_REMOTE_SMB_DOMAIN", "WORKGROUP")
	t.Setenv("HANK_REMOTE_AGENT_FILES_ROOT", "/srv/hank/files")
	t.Setenv("HANK_REMOTE_AGENT_NOTES_ROOT", "/srv/hank/notes")
	t.Setenv("HANK_REMOTE_MEDIA_REQUIRE_CONFIRMATION", "false")

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
	if cfg.SMB.Host != "192.168.1.20" || cfg.SMB.Share != "media" || cfg.SMB.Username != "aaron" || cfg.SMB.Password != "secret" || cfg.SMB.Domain != "WORKGROUP" {
		t.Fatalf("SMB config = %#v", cfg.SMB)
	}
	if cfg.FilesRoot != "/srv/hank/files" || cfg.NotesRoot != "/srv/hank/notes" {
		t.Fatalf("roots = files:%q notes:%q", cfg.FilesRoot, cfg.NotesRoot)
	}
	if cfg.Media.RequireConfirmation {
		t.Fatal("Media.RequireConfirmation = true, want false")
	}
}
