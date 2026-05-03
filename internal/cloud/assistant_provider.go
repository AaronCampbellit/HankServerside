package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/store"
)

type AssistantAIConfig struct {
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

type assistantProviderStatus struct {
	Provider            string `json:"provider"`
	ChatConfigured      bool   `json:"chat_configured"`
	EmbeddingConfigured bool   `json:"embedding_configured"`
	ChatModel           string `json:"chat_model"`
	EmbeddingModel      string `json:"embedding_model"`
	VectorStore         string `json:"vector_store"`
}

type assistantLLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *AssistantAIConfig) normalize() {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	if c.Provider == "" {
		c.Provider = "auto"
	}
	c.OllamaBaseURL = strings.TrimRight(strings.TrimSpace(c.OllamaBaseURL), "/")
	c.OllamaChatModel = strings.TrimSpace(c.OllamaChatModel)
	if c.OllamaChatModel == "" {
		c.OllamaChatModel = "llama3.1"
	}
	c.OllamaEmbeddingModel = strings.TrimSpace(c.OllamaEmbeddingModel)
	if c.OllamaEmbeddingModel == "" {
		c.OllamaEmbeddingModel = "nomic-embed-text"
	}
	c.OpenAIBaseURL = strings.TrimRight(strings.TrimSpace(c.OpenAIBaseURL), "/")
	if c.OpenAIBaseURL == "" {
		c.OpenAIBaseURL = "https://api.openai.com"
	}
	c.OpenAIChatModel = strings.TrimSpace(c.OpenAIChatModel)
	if c.OpenAIChatModel == "" {
		c.OpenAIChatModel = "gpt-4o-mini"
	}
	c.OpenAIEmbeddingModel = strings.TrimSpace(c.OpenAIEmbeddingModel)
	if c.OpenAIEmbeddingModel == "" {
		c.OpenAIEmbeddingModel = "text-embedding-3-small"
	}
	if c.EmbeddingDimension <= 0 {
		c.EmbeddingDimension = 768
	}
}

func (s *Server) assistantStatus(ctx context.Context, userID string) assistantProviderStatus {
	cfg := s.assistantAI
	cfg.normalize()
	provider, token := s.resolveAssistantProvider(ctx, userID)
	status := assistantProviderStatus{
		Provider:       provider,
		VectorStore:    "postgres",
		ChatModel:      cfg.OllamaChatModel,
		EmbeddingModel: cfg.OllamaEmbeddingModel,
		ChatConfigured: provider == "ollama" && cfg.OllamaBaseURL != "",
	}
	if provider == "openai" {
		status.ChatModel = cfg.OpenAIChatModel
		status.EmbeddingModel = cfg.OpenAIEmbeddingModel
		status.ChatConfigured = token != ""
	}
	status.EmbeddingConfigured = (provider == "ollama" && cfg.OllamaBaseURL != "") || (provider == "openai" && token != "")
	if !s.store.VectorAvailable() {
		status.VectorStore = "postgres_without_pgvector"
	}
	return status
}

func (s *Server) resolveAssistantProvider(ctx context.Context, userID string) (string, string) {
	cfg := s.assistantAI
	cfg.normalize()
	openAIToken := strings.TrimSpace(cfg.OpenAIAPIKey)
	if openAIToken == "" {
		if account, err := s.store.GetOpenAIAccount(ctx, userID); err == nil {
			openAIToken = strings.TrimSpace(account.AccessToken)
		} else if !errors.Is(err, store.ErrNotFound) {
			s.logger.Warn("assistant OpenAI account lookup failed", "error", err)
		}
	}

	switch cfg.Provider {
	case "ollama":
		return "ollama", ""
	case "openai":
		return "openai", openAIToken
	case "disabled":
		return "local", ""
	default:
		if cfg.OllamaBaseURL != "" {
			return "ollama", ""
		}
		if openAIToken != "" {
			return "openai", openAIToken
		}
		return "local", ""
	}
}

func (s *Server) generateAssistantLLMResponse(ctx context.Context, userID string, messages []assistantLLMMessage) (string, string, error) {
	cfg := s.assistantAI
	cfg.normalize()
	provider, token := s.resolveAssistantProvider(ctx, userID)
	switch provider {
	case "ollama":
		if cfg.OllamaBaseURL == "" {
			return "", "", errors.New("Ollama is not configured")
		}
		text, err := postOllamaChat(ctx, cfg.OllamaBaseURL, cfg.OllamaChatModel, messages)
		return text, "ollama:" + cfg.OllamaChatModel, err
	case "openai":
		if token == "" {
			return "", "", errors.New("OpenAI is not configured")
		}
		text, err := postOpenAIChat(ctx, cfg.OpenAIBaseURL, token, cfg.OpenAIChatModel, messages)
		return text, "openai:" + cfg.OpenAIChatModel, err
	default:
		return "", "local", errors.New("assistant provider is not configured")
	}
}

func (s *Server) embedAssistantText(ctx context.Context, userID string, text string) ([]float64, string, string) {
	cfg := s.assistantAI
	cfg.normalize()
	provider, token := s.resolveAssistantProvider(ctx, userID)
	switch provider {
	case "ollama":
		if cfg.OllamaBaseURL != "" {
			if embedding, err := postOllamaEmbedding(ctx, cfg.OllamaBaseURL, cfg.OllamaEmbeddingModel, text); err == nil && len(embedding) > 0 {
				return normalizeEmbedding(embedding, cfg.EmbeddingDimension), cfg.OllamaEmbeddingModel, "ollama"
			} else if err != nil {
				s.logger.Warn("assistant Ollama embedding failed", "error", err)
			}
		}
	case "openai":
		if token != "" {
			if embedding, err := postOpenAIEmbedding(ctx, cfg.OpenAIBaseURL, token, cfg.OpenAIEmbeddingModel, cfg.EmbeddingDimension, text); err == nil && len(embedding) > 0 {
				return normalizeEmbedding(embedding, cfg.EmbeddingDimension), cfg.OpenAIEmbeddingModel, "openai"
			} else if err != nil {
				s.logger.Warn("assistant OpenAI embedding failed", "error", err)
			}
		}
	}
	return localEmbedding(text, cfg.EmbeddingDimension), "local-hash", "v1"
}

func postOllamaChat(ctx context.Context, baseURL string, model string, messages []assistantLLMMessage) (string, error) {
	var body struct {
		Model    string                `json:"model"`
		Messages []assistantLLMMessage `json:"messages"`
		Stream   bool                  `json:"stream"`
	}
	body.Model = model
	body.Messages = messages
	body.Stream = false
	var response struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := postJSON(ctx, baseURL+"/api/chat", "", body, &response); err != nil {
		return "", err
	}
	if response.Error != "" {
		return "", errors.New(response.Error)
	}
	return strings.TrimSpace(response.Message.Content), nil
}

func postOllamaEmbedding(ctx context.Context, baseURL string, model string, text string) ([]float64, error) {
	body := map[string]any{"model": model, "prompt": text}
	var response struct {
		Embedding []float64 `json:"embedding"`
		Error     string    `json:"error"`
	}
	if err := postJSON(ctx, baseURL+"/api/embeddings", "", body, &response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	return response.Embedding, nil
}

func postOpenAIChat(ctx context.Context, baseURL string, token string, model string, messages []assistantLLMMessage) (string, error) {
	body := map[string]any{
		"model":       model,
		"messages":    messages,
		"temperature": 0.2,
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := postJSON(ctx, baseURL+"/v1/chat/completions", token, body, &response); err != nil {
		return "", err
	}
	if response.Error != nil {
		return "", errors.New(response.Error.Message)
	}
	if len(response.Choices) == 0 {
		return "", errors.New("OpenAI returned no choices")
	}
	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

func postOpenAIEmbedding(ctx context.Context, baseURL string, token string, model string, dimensions int, text string) ([]float64, error) {
	body := map[string]any{
		"model": model,
		"input": text,
	}
	if dimensions > 0 {
		body["dimensions"] = dimensions
	}
	var response struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := postJSON(ctx, baseURL+"/v1/embeddings", token, body, &response); err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}
	if len(response.Data) == 0 {
		return nil, errors.New("OpenAI returned no embeddings")
	}
	return response.Data[0].Embedding, nil
}

func postJSON(ctx context.Context, url string, bearerToken string, body any, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	client := &http.Client{Timeout: 45 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider status %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func localEmbedding(text string, dimension int) []float64 {
	if dimension <= 0 {
		dimension = 768
	}
	values := make([]float64, dimension)
	terms := strings.Fields(strings.ToLower(text))
	if len(terms) == 0 {
		terms = []string{strings.ToLower(text)}
	}
	for _, term := range terms {
		sum := sha256.Sum256([]byte(term))
		index := int(binary.BigEndian.Uint64(sum[:8]) % uint64(dimension))
		weight := 1.0 + float64(len(term)%7)/10
		values[index] += weight
	}
	var norm float64
	for _, value := range values {
		norm += value * value
	}
	if norm == 0 {
		return values
	}
	norm = math.Sqrt(norm)
	for i := range values {
		values[i] = values[i] / norm
	}
	return values
}

func normalizeEmbedding(values []float64, dimension int) []float64 {
	if dimension <= 0 {
		return values
	}
	normalized := make([]float64, dimension)
	copy(normalized, values)
	return normalized
}
