package store

import (
	"context"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

func TestUpdateFileOperationJobMonotonicDoesNotRegressTerminalState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_file_job_monotonic", Email: "file-job-monotonic@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	home := domain.Home{ID: "home_file_job_monotonic", UserID: user.ID, Name: "File Job Home", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateHome(ctx, home); err != nil {
		t.Fatalf("CreateHome: %v", err)
	}
	job := FileOperationJob{
		ID:                  "filejob_monotonic",
		HomeID:              home.ID,
		UserID:              user.ID,
		Operation:           "move",
		SourceID:            "primary",
		DestinationSourceID: "secondary",
		FromPath:            "/source",
		ToPath:              "/destination",
		Status:              "running",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := db.CreateFileOperationJob(ctx, job); err != nil {
		t.Fatalf("CreateFileOperationJob: %v", err)
	}
	completedAt := now.Add(time.Second)
	if err := db.UpdateFileOperationJob(ctx, job.ID, "completed", 10, 1, "", &completedAt); err != nil {
		t.Fatalf("complete file job: %v", err)
	}

	updated, err := db.UpdateFileOperationJobMonotonic(ctx, job.ID, "running", 0, 0, "", nil)
	if err != nil {
		t.Fatalf("UpdateFileOperationJobMonotonic: %v", err)
	}
	if updated {
		t.Fatal("non-terminal update changed a completed file job")
	}
	failedAt := now.Add(2 * time.Second)
	updated, err = db.UpdateFileOperationJobMonotonic(ctx, job.ID, "failed", 0, 0, "late failure", &failedAt)
	if err != nil {
		t.Fatalf("UpdateFileOperationJobMonotonic terminal conflict: %v", err)
	}
	if updated {
		t.Fatal("later terminal update changed the first terminal file-job state")
	}

	got, err := db.GetFileOperationJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetFileOperationJob: %v", err)
	}
	if got.Status != "completed" || got.BytesDone != 10 || got.FilesDone != 1 || got.CompletedAt == nil {
		t.Fatalf("file job regressed after late running response: %#v", got)
	}
}
