package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/protocol"
)

const defaultModel = "hermes-agent"

var ErrDisabled = errors.New("hermes is not configured")

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type Service struct {
	mu      sync.RWMutex
	baseURL string
	apiKey  string
	model   string
	timeout time.Duration
	client  *http.Client
}

func New(cfg Config) *Service {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Service{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   defaultString(strings.TrimSpace(cfg.Model), defaultModel),
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

func (s *Service) Enabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.baseURL != "" && s.apiKey != ""
}

func (s *Service) ApplyConfig(baseURL string, apiKey string, model string, timeout time.Duration) {
	if s == nil {
		return
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	s.apiKey = strings.TrimSpace(apiKey)
	s.model = defaultString(strings.TrimSpace(model), defaultModel)
	s.timeout = timeout
	s.client = &http.Client{Timeout: timeout}
}

func (s *Service) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		APIBaseURL:     s.baseURL,
		Model:          s.model,
		TimeoutSeconds: int(s.timeout.Seconds()),
		APIKeySet:      s.apiKey != "",
	}
}

func (s *Service) APIKey() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiKey
}

type Snapshot struct {
	APIBaseURL     string `json:"api_base_url"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	APIKeySet      bool   `json:"api_key_set"`
}

func (s *Service) Chat(ctx context.Context, request protocol.HermesChatRequest) (protocol.HermesChatResponse, error) {
	if !s.Enabled() {
		return protocol.HermesChatResponse{}, ErrDisabled
	}
	s.mu.RLock()
	baseURL := s.baseURL
	apiKey := s.apiKey
	model := s.model
	client := s.client
	s.mu.RUnlock()
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return protocol.HermesChatResponse{}, errors.New("prompt is required")
	}

	payload := hermesResponsesRequest{
		Model:        model,
		Input:        prompt,
		Store:        true,
		Conversation: strings.TrimSpace(request.ConversationID),
		Instructions: "You are being reached from Hank chat through the home Hank Remote agent. Answer the user directly and keep responses practical.",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return protocol.HermesChatResponse{}, err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, responsesURL(baseURL), bytes.NewReader(body))
	if err != nil {
		return protocol.HermesChatResponse{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	if sessionKey := strings.TrimSpace(request.SessionKey); sessionKey != "" {
		httpRequest.Header.Set("X-Hermes-Session-Key", sessionKey)
	}

	response, err := client.Do(httpRequest)
	if err != nil {
		return protocol.HermesChatResponse{}, err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return protocol.HermesChatResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return protocol.HermesChatResponse{}, fmt.Errorf("hermes status %d: %s", response.StatusCode, excerpt(data, 512))
	}

	var decoded hermesResponsesResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return protocol.HermesChatResponse{}, err
	}
	text := strings.TrimSpace(decoded.OutputText)
	if text == "" {
		text = strings.TrimSpace(decodedTextOutput(decoded.Output))
	}
	if text == "" {
		return protocol.HermesChatResponse{}, errors.New("hermes returned no text")
	}
	return protocol.HermesChatResponse{
		Text:           text,
		Model:          decoded.Model,
		ResponseID:     decoded.ID,
		ConversationID: strings.TrimSpace(request.ConversationID),
	}, nil
}

func (s *Service) responsesURL() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return responsesURL(s.baseURL)
}

func responsesURL(baseURL string) string {
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/responses"
	}
	return baseURL + "/v1/responses"
}

type hermesResponsesRequest struct {
	Model        string `json:"model"`
	Input        string `json:"input"`
	Instructions string `json:"instructions,omitempty"`
	Store        bool   `json:"store"`
	Conversation string `json:"conversation,omitempty"`
}

type hermesResponsesResponse struct {
	ID         string                 `json:"id"`
	Model      string                 `json:"model"`
	OutputText string                 `json:"output_text"`
	Output     []hermesResponseOutput `json:"output"`
}

type hermesResponseOutput struct {
	Type    string                  `json:"type"`
	Role    string                  `json:"role"`
	Content []hermesResponseContent `json:"content"`
}

type hermesResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func decodedTextOutput(items []hermesResponseOutput) string {
	var builder strings.Builder
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		if item.Role != "" && item.Role != "assistant" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" || content.Type == "text" {
				if text := strings.TrimSpace(content.Text); text != "" {
					if builder.Len() > 0 {
						builder.WriteString("\n")
					}
					builder.WriteString(text)
				}
			}
		}
	}
	return builder.String()
}

func excerpt(data []byte, limit int) string {
	value := strings.TrimSpace(string(data))
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
