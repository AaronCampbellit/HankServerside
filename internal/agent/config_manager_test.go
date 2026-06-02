package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	agenthermes "github.com/dropfile/hankremote/internal/agent/hermes"
	agentha "github.com/dropfile/hankremote/internal/agent/homeassistant"
	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

func TestConfigManagerApplyHermesPersistsEnvAndPreservesSecret(t *testing.T) {
	t.Parallel()

	envPath := filepath.Join(t.TempDir(), ".env.agent")
	if err := os.WriteFile(envPath, []byte("HANK_REMOTE_AGENT_ID=home-main\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	hermes := agenthermes.New(agenthermes.Config{})
	manager := newConfigManager(envPath, agentha.New("", "", 0), agentfiles.New(""), hermes)

	publicConfig, err := json.Marshal(map[string]any{
		"api_base_url":    "http://hermes-vm:8642",
		"model":           "hermes-agent",
		"timeout_seconds": 90,
	})
	if err != nil {
		t.Fatalf("Marshal public config: %v", err)
	}
	secrets, err := json.Marshal(map[string]any{"api_key": "key-one"})
	if err != nil {
		t.Fatalf("Marshal secrets: %v", err)
	}
	if _, err := manager.Apply(t.Context(), protocol.ConfigApplyRequest{
		ServiceType:   domain.ServiceTypeHermes,
		PublicConfig:  publicConfig,
		Secrets:       secrets,
		SecretVersion: 1,
		Persist:       true,
	}); err != nil {
		t.Fatalf("Apply Hermes: %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	envText := string(data)
	for _, want := range []string{
		"HANK_REMOTE_HERMES_API_BASE_URL=http://hermes-vm:8642",
		"HANK_REMOTE_HERMES_API_KEY=key-one",
		"HANK_REMOTE_HERMES_MODEL=hermes-agent",
		"HANK_REMOTE_HERMES_TIMEOUT_SECONDS=90",
	} {
		if !strings.Contains(envText, want) {
			t.Fatalf("env file missing %q:\n%s", want, envText)
		}
	}

	nextPublicConfig, err := json.Marshal(map[string]any{
		"api_base_url":    "http://hermes-vm:9000/v1",
		"model":           "hermes-agent-pro",
		"timeout_seconds": 120,
	})
	if err != nil {
		t.Fatalf("Marshal next public config: %v", err)
	}
	if _, err := manager.Apply(t.Context(), protocol.ConfigApplyRequest{
		ServiceType:   domain.ServiceTypeHermes,
		PublicConfig:  nextPublicConfig,
		SecretVersion: 1,
		Persist:       true,
	}); err != nil {
		t.Fatalf("Apply Hermes without new secret: %v", err)
	}
	data, err = os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile after second apply: %v", err)
	}
	envText = string(data)
	if !strings.Contains(envText, "HANK_REMOTE_HERMES_API_KEY=key-one") {
		t.Fatalf("Hermes API key was not preserved:\n%s", envText)
	}
	if !strings.Contains(envText, "HANK_REMOTE_HERMES_API_BASE_URL=http://hermes-vm:9000/v1") {
		t.Fatalf("Hermes API URL was not updated:\n%s", envText)
	}
}
