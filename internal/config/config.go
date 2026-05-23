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
	NoteAttachmentDir  string
	OpenAIClientID     string
	OpenAIClientSecret string
	OpenAIRedirectURI  string
	OpenAIScopes       string
	AssistantAI        AssistantAI
	APNS               APNS
}

type AssistantAI struct {
	Provider              string
	OllamaBaseURL         string
	OllamaChatModel       string
	OllamaEmbeddingModel  string
	OpenAIBaseURL         string
	OpenAIAPIKey          string
	OpenAIChatModel       string
	OpenAIEmbeddingModel  string
	ChatGPTOAuthEnabled   bool
	ChatGPTAuthIssuer     string
	ChatGPTBackendBaseURL string
	ChatGPTClientID       string
	ChatGPTChatModel      string
	ProjectDocsDir        string
	EmbeddingDimension    int
}

type APNS struct {
	TeamID      string
	KeyID       string
	PrivateKey  string
	Topic       string
	Environment string
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
	Media      Media
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

type Media struct {
	GramatonEnabled bool
	GramatonBaseURL string
	Username        string
	Password        string
	DestinationPath string
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
		NoteAttachmentDir:  envOrDefault("HANK_REMOTE_NOTE_ATTACHMENTS_DIR", "/var/lib/hank/note-attachments"),
		OpenAIClientID:     strings.TrimSpace(os.Getenv("HANK_REMOTE_OPENAI_CLIENT_ID")),
		OpenAIClientSecret: strings.TrimSpace(os.Getenv("HANK_REMOTE_OPENAI_CLIENT_SECRET")),
		OpenAIRedirectURI:  strings.TrimSpace(os.Getenv("HANK_REMOTE_OPENAI_REDIRECT_URI")),
		OpenAIScopes:       envOrDefault("HANK_REMOTE_OPENAI_SCOPES", "openid profile email"),
		APNS: APNS{
			TeamID:      strings.TrimSpace(os.Getenv("HANK_REMOTE_APNS_TEAM_ID")),
			KeyID:       strings.TrimSpace(os.Getenv("HANK_REMOTE_APNS_KEY_ID")),
			PrivateKey:  strings.TrimSpace(os.Getenv("HANK_REMOTE_APNS_PRIVATE_KEY")),
			Topic:       strings.TrimSpace(os.Getenv("HANK_REMOTE_APNS_TOPIC")),
			Environment: envOrDefault("HANK_REMOTE_APNS_ENVIRONMENT", "sandbox"),
		},
		AssistantAI: AssistantAI{
			Provider:              strings.ToLower(envOrDefault("HANK_REMOTE_AI_PROVIDER", "auto")),
			OllamaBaseURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("HANK_REMOTE_OLLAMA_BASE_URL")), "/"),
			OllamaChatModel:       envOrDefault("HANK_REMOTE_OLLAMA_CHAT_MODEL", "llama3.1"),
			OllamaEmbeddingModel:  envOrDefault("HANK_REMOTE_OLLAMA_EMBEDDING_MODEL", "nomic-embed-text"),
			OpenAIBaseURL:         strings.TrimRight(envOrDefault("HANK_REMOTE_OPENAI_API_BASE_URL", "https://api.openai.com"), "/"),
			OpenAIAPIKey:          strings.TrimSpace(os.Getenv("HANK_REMOTE_OPENAI_API_KEY")),
			OpenAIChatModel:       envOrDefault("HANK_REMOTE_OPENAI_CHAT_MODEL", "gpt-4o-mini"),
			OpenAIEmbeddingModel:  envOrDefault("HANK_REMOTE_OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
			ChatGPTOAuthEnabled:   boolEnvOrDefault("HANK_REMOTE_CHATGPT_OAUTH_ENABLED", false),
			ChatGPTAuthIssuer:     strings.TrimRight(envOrDefault("HANK_REMOTE_CHATGPT_AUTH_ISSUER", "https://auth.openai.com"), "/"),
			ChatGPTBackendBaseURL: strings.TrimRight(envOrDefault("HANK_REMOTE_CHATGPT_BACKEND_BASE_URL", "https://chatgpt.com/backend-api/codex"), "/"),
			ChatGPTClientID:       envOrDefault("HANK_REMOTE_CHATGPT_CLIENT_ID", "app_EMoamEEZ73f0CkXaXp7hrann"),
			ChatGPTChatModel:      envOrDefault("HANK_REMOTE_CHATGPT_CHAT_MODEL", "gpt-5.4-mini"),
			ProjectDocsDir:        envOrDefault("HANK_REMOTE_PROJECT_DOCS_DIR", "."),
			EmbeddingDimension:    embeddingDimensionValue,
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
		Media: Media{
			GramatonEnabled: boolEnvOrDefault("HANK_REMOTE_MEDIA_GRAMATON_ENABLED", false),
			GramatonBaseURL: strings.TrimRight(envOrDefault("HANK_REMOTE_MEDIA_GRAMATON_BASE_URL", "https://gramaton.io"), "/"),
			Username:        strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_GRAMATON_USERNAME")),
			Password:        os.Getenv("HANK_REMOTE_MEDIA_GRAMATON_PASSWORD"),
			DestinationPath: strings.TrimSpace(os.Getenv("HANK_REMOTE_MEDIA_DESTINATION_PATH")),
		},
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

func boolEnvOrDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationSeconds(key string, fallback int) (time.Duration, error) {
	raw := envOrDefault(key, strconv.Itoa(fallback))
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return time.Duration(seconds) * time.Second, nil
}
