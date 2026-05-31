package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/dropfile/hankremote/internal/store"
)

type Jobs struct {
	Store     *store.Store
	Retention time.Duration
	Logger    *slog.Logger
}

func (j Jobs) RunOnce(ctx context.Context) error {
	if j.Store == nil {
		return nil
	}
	retention := j.Retention
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	err := j.Store.PruneLifecycle(ctx, time.Now().UTC(), retention)
	if err != nil && j.Logger != nil {
		j.Logger.Warn("maintenance cleanup failed", "error", err)
	}
	return err
}

func (j Jobs) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := j.RunOnce(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
