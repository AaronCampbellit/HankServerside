package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

func (s *Server) handleHomeFileJobs(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, membership domain.HomeMembership, parts []string) bool {
	if len(parts) == 0 || parts[0] != "file-jobs" {
		return false
	}
	if err := s.requireHomeFeature(r.Context(), home, membership, auth.User.ID, domain.HomePermissionFeatureFiles); err != nil {
		if errors.Is(err, errFeaturePermissionDenied) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return true
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		jobs, err := s.store.ListFileOperationJobs(r.Context(), home.ID, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": fileOperationJobSnapshots(jobs)})
		return true
	}

	jobID := strings.TrimSpace(parts[1])
	if jobID == "" {
		http.NotFound(w, r)
		return true
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		job, err := s.store.GetFileOperationJob(r.Context(), jobID)
		if errors.Is(err, store.ErrNotFound) || err == nil && job.HomeID != home.ID {
			http.NotFound(w, r)
			return true
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, fileOperationJobSnapshot(job))
		return true
	}
	if len(parts) == 3 && parts[2] == "cancel" && r.Method == http.MethodPost {
		job, err := s.store.GetFileOperationJob(r.Context(), jobID)
		if errors.Is(err, store.ErrNotFound) || err == nil && job.HomeID != home.ID {
			http.NotFound(w, r)
			return true
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		if job.Status == "completed" || job.Status == "failed" || job.Status == "cancelled" || job.Status == "rolled_back" {
			writeJSON(w, http.StatusOK, fileOperationJobSnapshot(job))
			return true
		}
		now := time.Now().UTC()
		if err := s.store.UpdateFileOperationJob(r.Context(), job.ID, "cancelled", job.BytesDone, job.FilesDone, "cancelled by user", &now); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.audit(r.Context(), "file_operation.cancelled", auditSeverityInfo, auth.User.ID, "", home.ID, requestIDFromContext(r.Context()), "file_operation_job", job.ID, map[string]any{"operation": job.Operation})
		job, _ = s.store.GetFileOperationJob(r.Context(), job.ID)
		writeJSON(w, http.StatusOK, fileOperationJobSnapshot(job))
		return true
	}
	if len(parts) == 3 && parts[2] == "retry" && r.Method == http.MethodPost {
		job, err := s.retryFileOperationJob(r.Context(), home, auth, jobID)
		if errors.Is(err, store.ErrNotFound) || err == nil && job.HomeID != home.ID {
			http.NotFound(w, r)
			return true
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		writeJSON(w, http.StatusOK, fileOperationJobSnapshot(job))
		return true
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return true
}

func (s *Server) prepareManagedFileCommand(ctx context.Context, home domain.Home, auth authContext, command protocol.RoutedCommand) (protocol.RoutedCommand, string, error) {
	if command.Command != "files.move" {
		return command, "", nil
	}
	request, err := decodeBody[protocol.FilesMoveRequest](command.Body)
	if err != nil {
		return command, "", err
	}
	if strings.TrimSpace(request.DestinationSourceID) == "" {
		request.DestinationSourceID = request.SourceID
	}
	jobID := newID("filejob")
	request.JobID = jobID
	body, err := protocol.EncodeBody(request)
	if err != nil {
		return command, "", err
	}
	now := time.Now().UTC()
	job := store.FileOperationJob{
		ID:                  jobID,
		HomeID:              home.ID,
		UserID:              auth.User.ID,
		Operation:           protocol.FileOperationMove,
		SourceID:            strings.TrimSpace(request.SourceID),
		DestinationSourceID: strings.TrimSpace(request.DestinationSourceID),
		FromPath:            strings.TrimSpace(request.From),
		ToPath:              strings.TrimSpace(request.To),
		IsDirectory:         request.IsDirectory,
		Status:              "queued",
		FilesTotal:          1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := s.store.CreateFileOperationJob(ctx, job); err != nil {
		return command, "", err
	}
	s.audit(ctx, "file_operation.requested", auditSeverityInfo, auth.User.ID, "", home.ID, requestIDFromContext(ctx), "file_operation_job", jobID, map[string]any{
		"operation":             protocol.FileOperationMove,
		"source_id":             request.SourceID,
		"destination_source_id": request.DestinationSourceID,
		"from_path_hash":        stableAuditTarget(request.From),
		"to_path_hash":          stableAuditTarget(request.To),
		"is_directory":          request.IsDirectory,
	})
	command.Body = body
	return command, jobID, nil
}

func (s *Server) markFileJobRunning(ctx context.Context, jobID string) {
	if jobID == "" {
		return
	}
	if err := s.store.UpdateFileOperationJob(ctx, jobID, "running", 0, 0, "", nil); err != nil {
		s.logger.Warn("failed to mark file job running", "job_id", jobID, "error", err)
	}
}

func (s *Server) completePendingFileJob(ctx context.Context, pending *pendingRequest, envelope protocol.Envelope) {
	if pending == nil || pending.fileJobID == "" {
		return
	}
	jobID := pending.fileJobID
	if job, err := s.store.GetFileOperationJob(ctx, jobID); err == nil && job.Status == "cancelled" {
		s.logger.Info("ignoring file job completion after cancellation", "job_id", jobID, "request_id", pending.requestID)
		return
	}
	if envelope.Error != nil {
		now := time.Now().UTC()
		if err := s.store.UpdateFileOperationJob(ctx, jobID, "failed", 0, 0, envelope.Error.Message, &now); err != nil {
			s.logger.Warn("failed to mark file job failed", "job_id", jobID, "error", err)
		}
		s.audit(ctx, "file_operation.failed", auditSeverityWarning, pending.app.userID, "", pending.homeID, pending.requestID, "file_operation_job", jobID, map[string]any{"error_code": envelope.Error.Code})
		return
	}
	response := protocol.FileOperationJobResponse{OK: true, JobID: jobID, Status: "completed", FilesDone: 1}
	if len(envelope.Payload) > 0 {
		_ = json.Unmarshal(envelope.Payload, &response)
	}
	if response.Status == "" {
		response.Status = "completed"
	}
	if response.JobID == "" {
		response.JobID = jobID
	}
	now := time.Now().UTC()
	if err := s.store.UpdateFileOperationJob(ctx, response.JobID, response.Status, response.BytesDone, response.FilesDone, "", &now); err != nil {
		s.logger.Warn("failed to mark file job completed", "job_id", response.JobID, "status", response.Status, "error", err)
	}
	if response.BytesTotal > 0 || response.FilesTotal > 0 {
		if err := s.store.UpdateFileOperationJobTotals(ctx, response.JobID, response.BytesTotal, response.FilesTotal); err != nil {
			s.logger.Warn("failed to update file job totals", "job_id", response.JobID, "error", err)
		}
	}
	s.audit(ctx, "file_operation.completed", auditSeverityInfo, pending.app.userID, "", pending.homeID, pending.requestID, "file_operation_job", response.JobID, map[string]any{"status": response.Status})
}

func (s *Server) failFileJob(ctx context.Context, jobID string, status string, message string) {
	if jobID == "" {
		return
	}
	if status == "" {
		status = "failed"
	}
	now := time.Now().UTC()
	if err := s.store.UpdateFileOperationJob(ctx, jobID, status, 0, 0, message, &now); err != nil {
		s.logger.Warn("failed to update file job failure", "job_id", jobID, "status", status, "error", err)
	}
}

func (s *Server) retryFileOperationJob(ctx context.Context, home domain.Home, auth authContext, jobID string) (store.FileOperationJob, error) {
	job, err := s.store.GetFileOperationJob(ctx, jobID)
	if err != nil {
		return store.FileOperationJob{}, err
	}
	if job.HomeID != home.ID {
		return job, nil
	}
	if job.Operation != protocol.FileOperationMove {
		return store.FileOperationJob{}, errors.New("only move jobs can be retried")
	}
	if job.Status != "failed" && job.Status != "rollback_required" && job.Status != "cancelled" {
		return job, nil
	}
	if err := s.store.UpdateFileOperationJob(ctx, job.ID, "running", job.BytesDone, job.FilesDone, "", nil); err != nil {
		return store.FileOperationJob{}, err
	}
	request := protocol.FilesMoveRequest{
		SourceID:            job.SourceID,
		DestinationSourceID: job.DestinationSourceID,
		JobID:               job.ID,
		From:                job.FromPath,
		To:                  job.ToPath,
		IsDirectory:         job.IsDirectory,
	}
	response, err := s.sendAgentCommand(ctx, home.ID, "files.move", request)
	if err != nil {
		now := time.Now().UTC()
		_ = s.store.UpdateFileOperationJob(ctx, job.ID, "failed", job.BytesDone, job.FilesDone, err.Error(), &now)
		return s.store.GetFileOperationJob(ctx, job.ID)
	}
	if response.Error != nil {
		now := time.Now().UTC()
		_ = s.store.UpdateFileOperationJob(ctx, job.ID, "failed", job.BytesDone, job.FilesDone, response.Error.Message, &now)
		return s.store.GetFileOperationJob(ctx, job.ID)
	}
	payload := protocol.FileOperationJobResponse{OK: true, JobID: job.ID, Status: "completed", FilesDone: 1}
	_ = json.Unmarshal(response.Payload, &payload)
	if payload.Status == "" {
		payload.Status = "completed"
	}
	now := time.Now().UTC()
	if err := s.store.UpdateFileOperationJob(ctx, job.ID, payload.Status, payload.BytesDone, payload.FilesDone, "", &now); err != nil {
		return store.FileOperationJob{}, err
	}
	s.audit(ctx, "file_operation.retried", auditSeverityInfo, auth.User.ID, "", home.ID, requestIDFromContext(ctx), "file_operation_job", job.ID, map[string]any{"status": payload.Status})
	return s.store.GetFileOperationJob(ctx, job.ID)
}

func fileOperationJobSnapshots(jobs []store.FileOperationJob) []map[string]any {
	out := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, fileOperationJobSnapshot(job))
	}
	return out
}

func fileOperationJobSnapshot(job store.FileOperationJob) map[string]any {
	return map[string]any{
		"id":                    job.ID,
		"home_id":               job.HomeID,
		"user_id":               job.UserID,
		"operation":             job.Operation,
		"source_id":             job.SourceID,
		"destination_source_id": job.DestinationSourceID,
		"from_path":             job.FromPath,
		"to_path":               job.ToPath,
		"is_directory":          job.IsDirectory,
		"status":                job.Status,
		"bytes_total":           job.BytesTotal,
		"bytes_done":            job.BytesDone,
		"files_total":           job.FilesTotal,
		"files_done":            job.FilesDone,
		"error_message":         job.ErrorMessage,
		"created_at":            job.CreatedAt,
		"updated_at":            job.UpdatedAt,
		"completed_at":          job.CompletedAt,
	}
}
