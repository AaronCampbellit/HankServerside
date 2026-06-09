package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gramatonmedia "github.com/dropfile/hankremote/cmd/hank-app-gramaton/internal/media"
	"github.com/dropfile/hankremote/internal/agent/apps"
	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	"github.com/dropfile/hankremote/internal/config"
	"github.com/dropfile/hankremote/internal/protocol"
)

const (
	maxRequestBytes = 1 << 20
	stateDirName    = ".gramaton"
)

type gramatonConfig struct {
	Enabled              *bool  `json:"enabled,omitempty"`
	BaseURL              string `json:"base_url,omitempty"`
	Username             string `json:"username,omitempty"`
	SourceID             string `json:"source_id,omitempty"`
	DestinationPath      string `json:"destination_path,omitempty"`
	MovieDestinationPath string `json:"movie_destination_path,omitempty"`
	TVDestinationPath    string `json:"tv_destination_path,omitempty"`
	RequireConfirmation  *bool  `json:"require_confirmation,omitempty"`
	FilesRoot            string `json:"files_root,omitempty"`
}

type gramatonSecrets struct {
	Password string `json:"password,omitempty"`
}

type appError struct {
	code    string
	message string
}

func (e appError) Error() string {
	return e.code + ": " + e.message
}

type jobRecord struct {
	Job     protocol.MediaDownloadJobStatus    `json:"job"`
	Request protocol.MediaDownloadStartRequest `json:"request"`
	Config  json.RawMessage                    `json:"config,omitempty"`
	Secrets json.RawMessage                    `json:"secrets,omitempty"`
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--worker" {
		os.Exit(runWorker(context.Background(), os.Args[2]))
	}
	os.Exit(run(context.Background(), os.Stdin, os.Stdout, os.Stderr, http.DefaultClient))
}

func run(ctx context.Context, input io.Reader, output io.Writer, stderr io.Writer, _ *http.Client) int {
	var request apps.AppStdioRequest
	if err := json.NewDecoder(io.LimitReader(input, maxRequestBytes)).Decode(&request); err != nil {
		return writeFailure(output, stderr, "", "invalid_request", "invalid app request")
	}
	response, err := dispatch(ctx, request, stderr)
	if err != nil {
		var appErr appError
		if errors.As(err, &appErr) {
			return writeFailure(output, stderr, request.RequestID, appErr.code, appErr.message)
		}
		return writeFailure(output, stderr, request.RequestID, "app_error", "Gramaton command failed")
	}
	return writeResponse(output, stderr, apps.AppStdioResponse{
		RequestID: request.RequestID,
		OK:        true,
		Output:    response.Output,
		Events:    response.Events,
	}, 0)
}

type commandResponse struct {
	Output json.RawMessage
	Events []apps.AppStdioEvent
}

func dispatch(ctx context.Context, request apps.AppStdioRequest, stderr io.Writer) (commandResponse, error) {
	switch request.CommandID {
	case "settings_status":
		service, err := newMediaService(request.Config, request.Secrets, stderr)
		if err != nil {
			return commandResponse{}, err
		}
		return marshalOutput(service.Settings(ctx))
	case "settings_apply":
		service, err := newMediaService(request.Config, request.Secrets, stderr)
		if err != nil {
			return commandResponse{}, err
		}
		var body protocol.MediaSettingsApplyRequest
		if err := decodeRaw(request.Input, &body); err != nil {
			return commandResponse{}, appError{"invalid_request", "invalid settings request"}
		}
		response, err := service.ApplySettings(ctx, body)
		if err != nil {
			return commandResponse{}, appError{"validation_failed", err.Error()}
		}
		return marshalOutput(response)
	case "search":
		service, err := newMediaService(request.Config, request.Secrets, stderr)
		if err != nil {
			return commandResponse{}, err
		}
		var body protocol.MediaSearchRequest
		if err := decodeRaw(request.Input, &body); err != nil {
			return commandResponse{}, appError{"invalid_request", "invalid search request"}
		}
		response, err := service.Search(ctx, body.Query, body.Limit)
		if err != nil {
			return commandResponse{}, appError{"media_error", err.Error()}
		}
		return marshalOutput(response)
	case "plan_download":
		service, err := newMediaService(request.Config, request.Secrets, stderr)
		if err != nil {
			return commandResponse{}, err
		}
		var body protocol.MediaPlanDownloadRequest
		if err := decodeRaw(request.Input, &body); err != nil {
			return commandResponse{}, appError{"invalid_request", "invalid plan request"}
		}
		response, err := service.PlanDownload(ctx, body)
		if err != nil {
			return commandResponse{}, appError{"media_error", err.Error()}
		}
		return marshalOutput(response)
	case "download_start":
		var body protocol.MediaDownloadStartRequest
		if err := decodeRaw(request.Input, &body); err != nil {
			return commandResponse{}, appError{"invalid_request", "invalid download request"}
		}
		return startDownloadWorker(ctx, request, body)
	case "download_status":
		var body protocol.MediaDownloadStatusRequest
		if err := decodeRaw(request.Input, &body); err != nil {
			return commandResponse{}, appError{"invalid_request", "invalid status request"}
		}
		status, err := readJobStatus(body.JobID)
		if err != nil {
			return commandResponse{}, appError{"job_not_found", err.Error()}
		}
		return marshalOutputWithJobEvent(protocol.MediaDownloadStatusResponse{Job: status}, status)
	case "download_jobs":
		statuses, err := listJobStatuses()
		if err != nil {
			return commandResponse{}, appError{"job_error", err.Error()}
		}
		return marshalOutput(protocol.MediaDownloadJobsResponse{Jobs: statuses})
	case "download_cancel":
		var body protocol.MediaDownloadCancelRequest
		if err := decodeRaw(request.Input, &body); err != nil {
			return commandResponse{}, appError{"invalid_request", "invalid cancel request"}
		}
		status, err := cancelJob(body.JobID)
		if err != nil {
			return commandResponse{}, appError{"job_not_found", err.Error()}
		}
		return marshalOutputWithJobEvent(protocol.MediaDownloadCancelResponse{Job: status}, status)
	case "image_fetch":
		service, err := newMediaService(request.Config, request.Secrets, stderr)
		if err != nil {
			return commandResponse{}, err
		}
		var body protocol.MediaImageFetchRequest
		if err := decodeRaw(request.Input, &body); err != nil {
			return commandResponse{}, appError{"invalid_request", "invalid image request"}
		}
		response, err := service.FetchImage(ctx, body.URL)
		if err != nil {
			return commandResponse{}, appError{"media_error", err.Error()}
		}
		return marshalOutput(response)
	default:
		return commandResponse{}, appError{"invalid_request", "unsupported Gramaton command"}
	}
}

func newMediaService(rawConfig json.RawMessage, rawSecrets json.RawMessage, stderr io.Writer) (*gramatonmedia.Service, error) {
	agentCfg, err := config.LoadAgent()
	if err != nil {
		return nil, appError{"invalid_environment", "agent environment is not available"}
	}
	var appCfg gramatonConfig
	if err := decodeRaw(rawConfig, &appCfg); err != nil {
		return nil, appError{"invalid_request", "invalid Gramaton config"}
	}
	var secrets gramatonSecrets
	if err := decodeRaw(rawSecrets, &secrets); err != nil {
		return nil, appError{"invalid_request", "invalid Gramaton secrets"}
	}
	filesRoot := firstNonBlank(appCfg.FilesRoot, agentCfg.FilesRoot)
	files := agentfiles.NewWithConfig(agentfiles.Config{
		Root:   filesRoot,
		Shares: agentSMBShares(agentCfg.SMBShares),
	})
	requireConfirmation := true
	if appCfg.RequireConfirmation != nil {
		requireConfirmation = *appCfg.RequireConfirmation
	}
	enabled := true
	if appCfg.Enabled != nil {
		enabled = *appCfg.Enabled
	}
	mediaCfg := gramatonmedia.Config{
		Enabled:                       enabled,
		BaseURL:                       firstNonBlank(strings.TrimSpace(appCfg.BaseURL), "https://gramaton.io"),
		Username:                      strings.TrimSpace(appCfg.Username),
		Password:                      secrets.Password,
		SourceID:                      strings.TrimSpace(appCfg.SourceID),
		DestinationPath:               strings.TrimSpace(appCfg.DestinationPath),
		MovieDestinationPath:          strings.TrimSpace(appCfg.MovieDestinationPath),
		TVDestinationPath:             strings.TrimSpace(appCfg.TVDestinationPath),
		RequireConfirmation:           requireConfirmation,
		RequireConfirmationConfigured: true,
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return gramatonmedia.New(mediaCfg, files, logger), nil
}

func startDownloadWorker(ctx context.Context, request apps.AppStdioRequest, body protocol.MediaDownloadStartRequest) (commandResponse, error) {
	service, err := newMediaService(request.Config, request.Secrets, io.Discard)
	if err != nil {
		return commandResponse{}, err
	}
	plan, err := service.PlanDownload(ctx, protocol.MediaPlanDownloadRequest{Selection: body.Selection})
	if err != nil {
		return commandResponse{}, appError{"media_error", err.Error()}
	}
	jobID := newJobID()
	status := protocol.MediaDownloadJobStatus{
		JobID:      jobID,
		Status:     protocol.MediaJobStatusQueued,
		Title:      firstNonBlank(plan.Plan.Selection.Title, body.Selection.Title),
		TotalCount: plan.Plan.ItemCount,
		StartedAt:  time.Now().UTC(),
	}
	record := jobRecord{
		Job:     status,
		Request: body,
		Config:  cloneRaw(request.Config),
		Secrets: cloneRaw(request.Secrets),
	}
	if err := writeJobRecord(record); err != nil {
		return commandResponse{}, appError{"job_error", err.Error()}
	}
	if err := spawnWorker(jobPath(jobID)); err != nil {
		status.Status = protocol.MediaJobStatusFailed
		status.ErrorMessage = "failed to start download worker"
		status.CompletedAt = time.Now().UTC()
		record.Job = status
		_ = writeJobRecord(record)
		return commandResponse{}, appError{"job_error", "failed to start download worker"}
	}
	return marshalOutputWithJobEvent(protocol.MediaDownloadStartResponse{Job: status}, status)
}

func runWorker(ctx context.Context, path string) int {
	record, err := readJobRecordPath(path)
	if err != nil {
		return 1
	}
	service, err := newMediaService(record.Config, record.Secrets, io.Discard)
	if err != nil {
		record.Job.Status = protocol.MediaJobStatusFailed
		record.Job.ErrorMessage = "invalid Gramaton worker configuration"
		record.Job.CompletedAt = time.Now().UTC()
		_ = writeJobRecord(record)
		return 1
	}
	service.SetEventSink(func(ctx context.Context, event string, topic string, payload any) error {
		if status, ok := payload.(protocol.MediaDownloadJobStatus); ok {
			status.JobID = record.Job.JobID
			record.Job = status
			return writeJobRecord(record)
		}
		return nil
	})
	start, err := service.StartDownload(ctx, record.Request)
	if err != nil {
		record.Job.Status = protocol.MediaJobStatusFailed
		record.Job.ErrorMessage = err.Error()
		record.Job.CompletedAt = time.Now().UTC()
		_ = writeJobRecord(record)
		return 1
	}
	innerJobID := start.Job.JobID
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(cancelPath(record.Job.JobID)); err == nil {
			_, _ = service.Cancel(ctx, innerJobID)
			_ = os.Remove(cancelPath(record.Job.JobID))
		}
		status, err := service.Status(ctx, innerJobID)
		if err == nil {
			status.Job.JobID = record.Job.JobID
			record.Job = status.Job
			_ = writeJobRecord(record)
			if isTerminalStatus(status.Job.Status) {
				return 0
			}
		}
		select {
		case <-ctx.Done():
			return 1
		case <-ticker.C:
		}
	}
}

func marshalOutput(value any) (commandResponse, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return commandResponse{}, err
	}
	return commandResponse{Output: raw}, nil
}

func marshalOutputWithJobEvent(value any, status protocol.MediaDownloadJobStatus) (commandResponse, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return commandResponse{}, err
	}
	body, err := json.Marshal(status)
	if err != nil {
		return commandResponse{}, err
	}
	event := "media.download_progress"
	if isTerminalStatus(status.Status) {
		event = "media.download_completed"
	}
	return commandResponse{
		Output: raw,
		Events: []apps.AppStdioEvent{{
			Event: event,
			Topic: "media.downloads",
			Body:  body,
		}},
	}, nil
}

func decodeRaw[T any](raw json.RawMessage, out *T) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeFailure(output io.Writer, stderr io.Writer, requestID string, code string, message string) int {
	return writeResponse(output, stderr, apps.AppStdioResponse{
		RequestID: requestID,
		OK:        false,
		Error: &apps.AppError{
			Code:    code,
			Message: message,
		},
	}, 1)
}

func writeResponse(output io.Writer, stderr io.Writer, response apps.AppStdioResponse, code int) int {
	if err := json.NewEncoder(output).Encode(response); err != nil {
		_, _ = fmt.Fprintln(stderr, "failed to write app response")
		return 1
	}
	if code != 0 && response.Error != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", response.Error.Code, response.Error.Message)
	}
	return code
}

func writeJobRecord(record jobRecord) error {
	if record.Job.JobID == "" {
		return fmt.Errorf("job id is required")
	}
	path := jobPath(record.Job.JobID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJobStatus(jobID string) (protocol.MediaDownloadJobStatus, error) {
	record, err := readJobRecord(jobID)
	if err != nil {
		return protocol.MediaDownloadJobStatus{}, err
	}
	return record.Job, nil
}

func readJobRecord(jobID string) (jobRecord, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" || strings.Contains(jobID, string(filepath.Separator)) || strings.Contains(jobID, "..") {
		return jobRecord{}, fmt.Errorf("invalid job id")
	}
	return readJobRecordPath(jobPath(jobID))
}

func readJobRecordPath(path string) (jobRecord, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return jobRecord{}, err
	}
	var record jobRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return jobRecord{}, err
	}
	return record, nil
}

func listJobStatuses() ([]protocol.MediaDownloadJobStatus, error) {
	dir := jobsDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	statuses := make([]protocol.MediaDownloadJobStatus, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := readJobRecordPath(filepath.Join(dir, entry.Name()))
		if err == nil {
			statuses = append(statuses, record.Job)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].StartedAt.After(statuses[j].StartedAt)
	})
	return statuses, nil
}

func cancelJob(jobID string) (protocol.MediaDownloadJobStatus, error) {
	record, err := readJobRecord(jobID)
	if err != nil {
		return protocol.MediaDownloadJobStatus{}, err
	}
	if !isTerminalStatus(record.Job.Status) {
		if err := os.WriteFile(cancelPath(record.Job.JobID), []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o600); err != nil {
			return protocol.MediaDownloadJobStatus{}, err
		}
		record.Job.Status = protocol.MediaJobStatusCancelled
		record.Job.ErrorMessage = "Cancelled by user."
		record.Job.CompletedAt = time.Now().UTC()
		_ = writeJobRecord(record)
	}
	return record.Job, nil
}

func jobPath(jobID string) string {
	return filepath.Join(jobsDir(), jobID+".json")
}

func cancelPath(jobID string) string {
	return filepath.Join(jobsDir(), jobID+".cancel")
}

func jobsDir() string {
	return filepath.Join(stateDirName, "jobs")
}

func spawnWorker(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--worker", path)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

func newJobID() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("media_%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("media_%x", random)
}

func isTerminalStatus(status string) bool {
	switch status {
	case protocol.MediaJobStatusCompleted, protocol.MediaJobStatusFailed, protocol.MediaJobStatusCancelled:
		return true
	default:
		return false
	}
}

func agentSMBShares(shares []config.SMB) []agentfiles.SMBConfig {
	configs := make([]agentfiles.SMBConfig, 0, len(shares))
	for _, share := range shares {
		configs = append(configs, agentfiles.SMBConfig{
			ID:       share.ID,
			Name:     share.Name,
			Host:     share.Host,
			Share:    share.Share,
			Username: share.Username,
			Password: share.Password,
			Domain:   share.Domain,
			Policy: agentfiles.AccessPolicy{
				Read:            share.Policy.Read,
				Write:           share.Policy.Write,
				Delete:          share.Policy.Delete,
				AllowedPrefixes: append([]string(nil), share.Policy.AllowedPrefixes...),
				BlockedPrefixes: append([]string(nil), share.Policy.BlockedPrefixes...),
				MaxUploadBytes:  share.Policy.MaxUploadBytes,
			},
		})
	}
	return configs
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return cloned
}
