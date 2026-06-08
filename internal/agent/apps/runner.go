package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const (
	defaultMaxOutputBytes int64 = 1 << 20
	defaultMaxStderrBytes int64 = 64 << 10
)

type AppStdioRequest struct {
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	AppID           string          `json:"app_id"`
	CommandID       string          `json:"command_id"`
	Config          json.RawMessage `json:"config,omitempty"`
	Secrets         json.RawMessage `json:"secrets,omitempty"`
	Input           json.RawMessage `json:"input,omitempty"`
}

type AppStdioResponse struct {
	RequestID string          `json:"request_id"`
	OK        bool            `json:"ok"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     *AppError       `json:"error,omitempty"`
}

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InvokeSpec struct {
	Executable string
	Args       []string
	WorkDir    string
	Timeout    time.Duration
	Request    AppStdioRequest
}

type Runner struct {
	MaxOutputBytes int64
	MaxStderrBytes int64
}

func (r Runner) Invoke(ctx context.Context, spec InvokeSpec) (AppStdioResponse, error) {
	requestLine, err := json.Marshal(spec.Request)
	if err != nil {
		return AppStdioResponse{}, fmt.Errorf("marshal app request: %w", err)
	}
	requestLine = append(requestLine, '\n')

	var runCtx context.Context
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, spec.Executable, spec.Args...)
	cmd.Dir = spec.WorkDir
	cmd.Stdin = bytes.NewReader(requestLine)

	stdout := newBoundedBuffer("stdout", byteLimit(r.MaxOutputBytes, defaultMaxOutputBytes), cancel)
	stderr := newBoundedBuffer("stderr", byteLimit(r.MaxStderrBytes, defaultMaxStderrBytes), cancel)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()

	if err := stdout.Err(); err != nil {
		return AppStdioResponse{}, err
	}
	if err := stderr.Err(); err != nil {
		return AppStdioResponse{}, err
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return AppStdioResponse{}, fmt.Errorf("app invocation timed out after %s", spec.Timeout)
	}
	if errors.Is(runCtx.Err(), context.Canceled) && runErr != nil {
		return AppStdioResponse{}, fmt.Errorf("app invocation canceled: %w", runCtx.Err())
	}
	if runErr != nil {
		return AppStdioResponse{}, fmt.Errorf("app invocation failed: %w", runErr)
	}

	return decodeAppResponse(stdout.Bytes(), spec.Request.RequestID)
}

type boundedBuffer struct {
	name     string
	maxBytes int64
	buf      bytes.Buffer
	err      error
	cancel   context.CancelFunc
}

func newBoundedBuffer(name string, maxBytes int64, cancel context.CancelFunc) *boundedBuffer {
	return &boundedBuffer{
		name:     name,
		maxBytes: maxBytes,
		cancel:   cancel,
	}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}

	remaining := b.maxBytes - int64(b.buf.Len())
	if remaining > 0 {
		toWrite := int64(len(p))
		if toWrite > remaining {
			toWrite = remaining
		}
		if toWrite > 0 {
			_, _ = b.buf.Write(p[:toWrite])
		}
	}

	if int64(len(p)) > remaining {
		b.err = fmt.Errorf("app %s exceeded %d bytes", b.name, b.maxBytes)
		b.cancel()
		return len(p), b.err
	}

	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *boundedBuffer) Err() error {
	return b.err
}

func byteLimit(configured int64, fallback int64) int64 {
	if configured > 0 {
		return configured
	}
	return fallback
}

func decodeAppResponse(data []byte, requestID string) (AppStdioResponse, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	var response AppStdioResponse
	if err := decoder.Decode(&response); err != nil {
		return AppStdioResponse{}, fmt.Errorf("invalid app response: %w", err)
	}

	var trailing json.RawMessage
	switch err := decoder.Decode(&trailing); {
	case err == nil:
		return AppStdioResponse{}, fmt.Errorf("invalid app response: trailing JSON token after app response")
	case errors.Is(err, io.EOF):
		return validateAppResponse(response, requestID)
	default:
		return AppStdioResponse{}, fmt.Errorf("invalid app response: %w", err)
	}
}

func validateAppResponse(response AppStdioResponse, requestID string) (AppStdioResponse, error) {
	if requestID != "" && response.RequestID != requestID {
		return AppStdioResponse{}, fmt.Errorf("invalid app response: request_id %q does not match request %q", response.RequestID, requestID)
	}
	if response.OK && response.Error != nil {
		return AppStdioResponse{}, fmt.Errorf("invalid app response: ok response must not include error")
	}
	if !response.OK && response.Error == nil {
		return AppStdioResponse{}, fmt.Errorf("invalid app response: error response must include error")
	}
	if response.Error != nil && (response.Error.Code == "" || response.Error.Message == "") {
		return AppStdioResponse{}, fmt.Errorf("invalid app response: error code and message are required")
	}
	return response, nil
}
