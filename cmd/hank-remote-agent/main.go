package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dropfile/hankremote/internal/agent"
	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	agentha "github.com/dropfile/hankremote/internal/agent/homeassistant"
	agentnotes "github.com/dropfile/hankremote/internal/agent/notes"
	"github.com/dropfile/hankremote/internal/config"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.LoadAgent()
	if err != nil {
		logger.Error("failed to load agent config", "error", err)
		os.Exit(1)
	}

	ha := agentha.New(cfg.HA.BaseURL, cfg.HA.Token, cfg.HA.Timeout)
	files := agentfiles.NewWithConfig(agentfiles.Config{
		Root: cfg.FilesRoot,
		SMB: agentfiles.SMBConfig{
			Host:     cfg.SMB.Host,
			Share:    cfg.SMB.Share,
			Username: cfg.SMB.Username,
			Password: cfg.SMB.Password,
			Domain:   cfg.SMB.Domain,
		},
	})
	notes := agentnotes.New(cfg.NotesRoot)

	client := agent.NewClient(cfg.CloudURL, cfg.AgentID, cfg.Token, cfg.HomeName, cfg.ConfigPath, ha, files, notes, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("agent exited with error", "error", err)
		os.Exit(1)
	}
}
