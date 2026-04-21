package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Cloud struct {
	Addr           string
	DatabaseURL    string
	SessionTTL     time.Duration
	RequestTimeout time.Duration
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

	return Cloud{
		Addr:           envOrDefault("HANK_REMOTE_CLOUD_ADDR", ":8080"),
		DatabaseURL:    envOrDefault("HANK_REMOTE_CLOUD_DATABASE_URL", "postgres://hankremote:hankremote@127.0.0.1:5432/hankremote?sslmode=disable"),
		SessionTTL:     sessionTTL,
		RequestTimeout: requestTimeout,
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
