package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Cloud struct {
	Addr               string
	DatabaseURL        string
	SessionTTL         time.Duration
	RequestTimeout     time.Duration
	DBOpsStateDir      string
	DBOpsLogDir        string
	DBOpsIntentSecret  string
	OpenAIClientID     string
	OpenAIClientSecret string
	OpenAIRedirectURI  string
	OpenAIScopes       string
	AssistantAI        AssistantAI
}

type AssistantAI struct {
	Provider             string
	OllamaBaseURL        string
	OllamaChatModel      string
	OllamaEmbeddingModel string
	OpenAIBaseURL        string
	OpenAIAPIKey         string
	OpenAIChatModel      string
	OpenAIEmbeddingModel string
	EmbeddingDimension   int
}

type DBOps struct {
	StateDir           string
	LogDir             string
	IntentSecret       string
	RepoCipherPass     string
	DatabaseURL        string
	Stanza             string
	PGDataPath         string
	RestoreDataPath    string
	RestoreDatabaseURL string
	ComposeFile        string
}

type Agent struct {
	CloudURL   string
	AgentID    string
	Token      string
	HomeName   string
	ConfigPath string
	HA         HomeAssistant
	SMB        SMB
	FilesRoot  string
	NotesRoot  string
}

type HomeAssistant struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

type SMB struct {
	Host     string
	Share    string
	Username string
	Password string
	Domain   string
}

func LoadCloud() (Cloud, error) {
	sessionTTL, err := durationSeconds("HANK_REMOTE_SESSION_TTL_SECONDS", 60*60*24*7)
	if err != nil {
		return Cloud{}, err
	}

	requestTimeout, err := durationSeconds("HANK_REMOTE_REQUEST_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Cloud{}, err
	}

	embeddingDimension := envOrDefault("HANK_REMOTE_AI_EMBEDDING_DIMENSION", "768")
	embeddingDimensionValue, err := strconv.Atoi(embeddingDimension)
	if err != nil || embeddingDimensionValue <= 0 {
		return Cloud{}, fmt.Errorf("HANK_REMOTE_AI_EMBEDDING_DIMENSION must be a positive integer")
	}

	return Cloud{
		Addr:               envOrDefault("HANK_REMOTE_CLOUD_ADDR", ":8080"),
		DatabaseURL:        envOrDefault("HANK_REMOTE_CLOUD_DATABASE_URL", "postgres://hankremote:hankremote@127.0.0.1:5432/hankremote?sslmode=disable"),
		SessionTTL:         sessionTTL,
		RequestTimeout:     requestTimeout,
		DBOpsStateDir:      envOrDefault("HANK_REMOTE_DB_OPS_STATE_DIR", "/var/lib/hank/db-ops/state"),
		DBOpsLogDir:        envOrDefault("HANK_REMOTE_DB_OPS_LOG_DIR", "/var/log/hank/db-ops"),
		DBOpsIntentSecret:  envOrDefault("HANK_REMOTE_DB_OPS_INTENT_SECRET", "replace-with-a-long-random-db-ops-secret"),
		OpenAIClientID:     strings.TrimSpace(os.Getenv("HANK_REMOTE_OPENAI_CLIENT_ID")),
		OpenAIClientSecret: strings.TrimSpace(os.Getenv("HANK_REMOTE_OPENAI_CLIENT_SECRET")),
		OpenAIRedirectURI:  strings.TrimSpace(os.Getenv("HANK_REMOTE_OPENAI_REDIRECT_URI")),
		OpenAIScopes:       envOrDefault("HANK_REMOTE_OPENAI_SCOPES", "openid profile email"),
		AssistantAI: AssistantAI{
			Provider:             strings.ToLower(envOrDefault("HANK_REMOTE_AI_PROVIDER", "auto")),
			OllamaBaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("HANK_REMOTE_OLLAMA_BASE_URL")), "/"),
			OllamaChatModel:      envOrDefault("HANK_REMOTE_OLLAMA_CHAT_MODEL", "llama3.1"),
			OllamaEmbeddingModel: envOrDefault("HANK_REMOTE_OLLAMA_EMBEDDING_MODEL", "nomic-embed-text"),
			OpenAIBaseURL:        strings.TrimRight(envOrDefault("HANK_REMOTE_OPENAI_API_BASE_URL", "https://api.openai.com"), "/"),
			OpenAIAPIKey:         strings.TrimSpace(os.Getenv("HANK_REMOTE_OPENAI_API_KEY")),
			OpenAIChatModel:      envOrDefault("HANK_REMOTE_OPENAI_CHAT_MODEL", "gpt-4o-mini"),
			OpenAIEmbeddingModel: envOrDefault("HANK_REMOTE_OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
			EmbeddingDimension:   embeddingDimensionValue,
		},
	}, nil
}

func LoadDBOps() (DBOps, error) {
	repoCipherPass := strings.TrimSpace(os.Getenv("HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS"))
	if repoCipherPass == "" {
		return DBOps{}, fmt.Errorf("HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS is required")
	}

	return DBOps{
		StateDir:           envOrDefault("HANK_REMOTE_DB_OPS_STATE_DIR", "/var/lib/hank/db-ops/state"),
		LogDir:             envOrDefault("HANK_REMOTE_DB_OPS_LOG_DIR", "/var/log/hank/db-ops"),
		IntentSecret:       envOrDefault("HANK_REMOTE_DB_OPS_INTENT_SECRET", "replace-with-a-long-random-db-ops-secret"),
		RepoCipherPass:     repoCipherPass,
		DatabaseURL:        envOrDefault("HANK_REMOTE_CLOUD_DATABASE_URL", "postgres://hankremote:hankremote@127.0.0.1:5432/hankremote?sslmode=disable"),
		Stanza:             envOrDefault("HANK_REMOTE_DB_OPS_STANZA", "hank"),
		PGDataPath:         envOrDefault("HANK_REMOTE_DB_OPS_PGDATA", "/var/lib/postgresql/data"),
		RestoreDataPath:    envOrDefault("HANK_REMOTE_DB_OPS_RESTORE_PGDATA", "/var/lib/postgresql/restore"),
		RestoreDatabaseURL: envOrDefault("HANK_REMOTE_DB_OPS_RESTORE_DATABASE_URL", "postgres://hankremote:hankremote@postgres-restore:5432/hankremote?sslmode=disable"),
		ComposeFile:        envOrDefault("HANK_REMOTE_DB_OPS_COMPOSE_FILE", "/workspace/docker-compose.yml"),
	}, nil
}

func LoadAgent() (Agent, error) {
	haTimeout, err := durationSeconds("HANK_REMOTE_HA_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Agent{}, err
	}

	cfg := Agent{
		CloudURL:   envOrDefault("HANK_REMOTE_AGENT_CLOUD_URL", "ws://127.0.0.1:8080/ws/agent"),
		AgentID:    strings.TrimSpace(os.Getenv("HANK_REMOTE_AGENT_ID")),
		Token:      strings.TrimSpace(os.Getenv("HANK_REMOTE_AGENT_TOKEN")),
		HomeName:   strings.TrimSpace(os.Getenv("HANK_REMOTE_AGENT_HOME_NAME")),
		ConfigPath: strings.TrimSpace(os.Getenv("HANK_REMOTE_AGENT_CONFIG_PATH")),
		FilesRoot:  strings.TrimSpace(os.Getenv("HANK_REMOTE_AGENT_FILES_ROOT")),
		NotesRoot:  strings.TrimSpace(os.Getenv("HANK_REMOTE_AGENT_NOTES_ROOT")),
		HA: HomeAssistant{
			BaseURL: strings.TrimSpace(os.Getenv("HANK_REMOTE_HA_BASE_URL")),
			Token:   strings.TrimSpace(os.Getenv("HANK_REMOTE_HA_TOKEN")),
			Timeout: haTimeout,
		},
		SMB: SMB{
			Host:     strings.TrimSpace(os.Getenv("HANK_REMOTE_SMB_HOST")),
			Share:    strings.TrimSpace(os.Getenv("HANK_REMOTE_SMB_SHARE")),
			Username: strings.TrimSpace(os.Getenv("HANK_REMOTE_SMB_USERNAME")),
			Password: os.Getenv("HANK_REMOTE_SMB_PASSWORD"),
			Domain:   strings.TrimSpace(os.Getenv("HANK_REMOTE_SMB_DOMAIN")),
		},
	}

	switch {
	case cfg.AgentID == "":
		return Agent{}, fmt.Errorf("HANK_REMOTE_AGENT_ID is required")
	case cfg.Token == "":
		return Agent{}, fmt.Errorf("HANK_REMOTE_AGENT_TOKEN is required")
	default:
		return cfg, nil
	}
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func durationSeconds(key string, fallback int) (time.Duration, error) {
	raw := envOrDefault(key, strconv.Itoa(fallback))
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return time.Duration(seconds) * time.Second, nil
}
