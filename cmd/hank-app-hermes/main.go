package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/agent/apps"
)

const (
	defaultHermesModel          = "hermes-agent"
	defaultHermesTimeoutSeconds = 120
	maxHermesBodyBytes          = 1 << 20
)

type hermesConfig struct {
	APIBaseURL     string `json:"api_base_url"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type hermesSecrets struct {
	APIKey string `json:"api_key"`
}

type hermesResponsesRequest struct {
	Model        string `json:"model"`
	Input        string `json:"input"`
	Store        bool   `json:"store"`
	Conversation string `json:"conversation,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

type hermesResponsesResponse struct {
	ID         string               `json:"id"`
	Model      string               `json:"model"`
	OutputText string               `json:"output_text"`
	Output     []hermesOutputObject `json:"output"`
}

type hermesOutputObject struct {
	Type    string                `json:"type"`
	Role    string                `json:"role"`
	Content []hermesOutputContent `json:"content"`
}

type hermesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type hermesChatInput struct {
	Prompt         string `json:"prompt"`
	ConversationID string `json:"conversation_id,omitempty"`
	SessionKey     string `json:"session_key,omitempty"`
}

type hermesChatOutput struct {
	Text           string `json:"text"`
	Model          string `json:"model,omitempty"`
	ResponseID     string `json:"response_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}

func main() {
	os.Exit(run(context.Background(), os.Stdin, os.Stdout, os.Stderr, http.DefaultClient))
}

func run(ctx context.Context, input io.Reader, output io.Writer, stderr io.Writer, client *http.Client) int {
	var request apps.AppStdioRequest
	if err := json.NewDecoder(io.LimitReader(input, maxHermesBodyBytes)).Decode(&request); err != nil {
		return writeAppFailure(output, stderr, "", "invalid_request", "invalid app request")
	}
	if request.CommandID != "chat" {
		return writeAppFailure(output, stderr, request.RequestID, "invalid_request", "unsupported Hermes command")
	}

	response, err := runChat(ctx, request, client)
	if err != nil {
		var appErr appError
		if errors.As(err, &appErr) {
			return writeAppFailure(output, stderr, request.RequestID, appErr.code, appErr.message)
		}
		return writeAppFailure(output, stderr, request.RequestID, "upstream_error", "Hermes request failed")
	}
	rawOutput, err := json.Marshal(response)
	if err != nil {
		return writeAppFailure(output, stderr, request.RequestID, "internal_error", "failed to encode Hermes response")
	}
	return writeAppResponse(output, stderr, apps.AppStdioResponse{
		RequestID: request.RequestID,
		OK:        true,
		Output:    rawOutput,
	}, 0)
}

type appError struct {
	code    string
	message string
}

func (e appError) Error() string {
	return e.code + ": " + e.message
}

func runChat(ctx context.Context, request apps.AppStdioRequest, client *http.Client) (hermesChatOutput, error) {
	var cfg hermesConfig
	if err := decodeRaw(request.Config, &cfg); err != nil {
		return hermesChatOutput{}, appError{"invalid_request", "invalid Hermes config"}
	}
	var secrets hermesSecrets
	if err := decodeRaw(request.Secrets, &secrets); err != nil {
		return hermesChatOutput{}, appError{"invalid_request", "invalid Hermes secrets"}
	}
	var chat hermesChatInput
	if err := decodeRaw(request.Input, &chat); err != nil {
		return hermesChatOutput{}, appError{"invalid_request", "invalid Hermes chat input"}
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	apiKey := strings.TrimSpace(secrets.APIKey)
	model := defaultString(strings.TrimSpace(cfg.Model), defaultHermesModel)
	prompt := strings.TrimSpace(chat.Prompt)
	if baseURL == "" || apiKey == "" {
		return hermesChatOutput{}, appError{"invalid_request", "Hermes API base URL and API key are required"}
	}
	if prompt == "" {
		return hermesChatOutput{}, appError{"invalid_request", "Hermes prompt is required"}
	}

	timeoutSeconds := cfg.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultHermesTimeoutSeconds
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	body, err := json.Marshal(hermesResponsesRequest{
		Model:        model,
		Input:        prompt,
		Store:        true,
		Conversation: strings.TrimSpace(chat.ConversationID),
		Instructions: "You are being reached from Hank chat through the home Hank Remote agent. Answer the user directly and keep responses practical.",
	})
	if err != nil {
		return hermesChatOutput{}, err
	}
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost, responsesURL(baseURL), bytes.NewReader(body))
	if err != nil {
		return hermesChatOutput{}, appError{"invalid_request", "invalid Hermes API base URL"}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	if sessionKey := strings.TrimSpace(chat.SessionKey); sessionKey != "" {
		httpRequest.Header.Set("X-Hermes-Session-Key", sessionKey)
	}

	if client == nil {
		client = http.DefaultClient
	}
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		return hermesChatOutput{}, appError{"upstream_error", "Hermes request failed"}
	}
	defer httpResponse.Body.Close()

	data, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxHermesBodyBytes))
	if err != nil {
		return hermesChatOutput{}, appError{"upstream_error", "failed to read Hermes response"}
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return hermesChatOutput{}, appError{"upstream_error", fmt.Sprintf("Hermes returned status %d", httpResponse.StatusCode)}
	}

	var decoded hermesResponsesResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return hermesChatOutput{}, appError{"upstream_error", "invalid Hermes response"}
	}
	text := strings.TrimSpace(decoded.OutputText)
	if text == "" {
		text = strings.TrimSpace(decodedTextOutput(decoded.Output))
	}
	if text == "" {
		return hermesChatOutput{}, appError{"upstream_error", "Hermes returned no text"}
	}
	return hermesChatOutput{
		Text:           text,
		Model:          decoded.Model,
		ResponseID:     decoded.ID,
		ConversationID: strings.TrimSpace(chat.ConversationID),
	}, nil
}

func decodeRaw[T any](raw json.RawMessage, out *T) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	return json.Unmarshal(raw, out)
}

func decodedTextOutput(output []hermesOutputObject) string {
	var parts []string
	for _, item := range output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				parts = append(parts, strings.TrimSpace(content.Text))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func responsesURL(baseURL string) string {
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/responses"
	}
	return baseURL + "/v1/responses"
}

func writeAppFailure(output io.Writer, stderr io.Writer, requestID string, code string, message string) int {
	return writeAppResponse(output, stderr, apps.AppStdioResponse{
		RequestID: requestID,
		OK:        false,
		Error: &apps.AppError{
			Code:    code,
			Message: message,
		},
	}, 1)
}

func writeAppResponse(output io.Writer, stderr io.Writer, response apps.AppStdioResponse, code int) int {
	encoder := json.NewEncoder(output)
	if err := encoder.Encode(response); err != nil {
		_, _ = fmt.Fprintln(stderr, "failed to write app response")
		return 1
	}
	if code != 0 && response.Error != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", response.Error.Code, response.Error.Message)
	}
	return code
}

func defaultString(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
