package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
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

type captureResult struct {
	data []byte
	err  error
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return AppStdioResponse{}, fmt.Errorf("open app stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return AppStdioResponse{}, fmt.Errorf("open app stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return AppStdioResponse{}, fmt.Errorf("start app: %w", err)
	}

	stdoutCh := make(chan captureResult, 1)
	stderrCh := make(chan captureResult, 1)
	go func() {
		stdoutCh <- readLimited(stdout, byteLimit(r.MaxOutputBytes, defaultMaxOutputBytes), "stdout", cancel)
	}()
	go func() {
		stderrCh <- readLimited(stderr, byteLimit(r.MaxStderrBytes, defaultMaxStderrBytes), "stderr", cancel)
	}()

	waitErr := cmd.Wait()
	stdoutResult := <-stdoutCh
	stderrResult := <-stderrCh

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return AppStdioResponse{}, fmt.Errorf("app invocation timed out after %s", spec.Timeout)
	}
	if stdoutResult.err != nil {
		return AppStdioResponse{}, stdoutResult.err
	}
	if stderrResult.err != nil {
		return AppStdioResponse{}, stderrResult.err
	}
	if errors.Is(runCtx.Err(), context.Canceled) && waitErr != nil {
		return AppStdioResponse{}, fmt.Errorf("app invocation canceled: %w", runCtx.Err())
	}
	if waitErr != nil {
		return AppStdioResponse{}, fmt.Errorf("app invocation failed: %w", waitErr)
	}

	return decodeAppResponse(stdoutResult.data)
}

func readLimited(reader io.Reader, maxBytes int64, streamName string, cancel context.CancelFunc) captureResult {
	var buf bytes.Buffer
	_, err := io.CopyN(&buf, reader, maxBytes+1)
	if err == nil {
		cancel()
		data := buf.Bytes()
		if int64(len(data)) > maxBytes {
			data = data[:maxBytes]
		}
		return captureResult{
			data: data,
			err:  fmt.Errorf("app %s exceeded %d bytes", streamName, maxBytes),
		}
	}
	if errors.Is(err, io.EOF) {
		return captureResult{data: buf.Bytes()}
	}
	return captureResult{
		data: buf.Bytes(),
		err:  fmt.Errorf("read app %s: %w", streamName, err),
	}
}

func byteLimit(configured int64, fallback int64) int64 {
	if configured > 0 {
		return configured
	}
	return fallback
}

func decodeAppResponse(data []byte) (AppStdioResponse, error) {
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
		return response, nil
	default:
		return AppStdioResponse{}, fmt.Errorf("invalid app response: %w", err)
	}
}
