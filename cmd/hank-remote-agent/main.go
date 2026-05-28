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
	agentmedia "github.com/dropfile/hankremote/internal/agent/media"
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
	legacySMB := cfg.SMB
	if len(cfg.SMBShares) > 0 {
		legacySMB = config.SMB{}
	}
	files := agentfiles.NewWithConfig(agentfiles.Config{
		Root: cfg.FilesRoot,
		SMB: agentfiles.SMBConfig{
			ID:       legacySMB.ID,
			Name:     legacySMB.Name,
			Host:     legacySMB.Host,
			Share:    legacySMB.Share,
			Username: legacySMB.Username,
			Password: legacySMB.Password,
			Domain:   legacySMB.Domain,
		},
		Shares: agentSMBShares(cfg.SMBShares),
	})
	media := agentmedia.New(agentmedia.Config{
		Enabled:                       cfg.Media.GramatonEnabled,
		BaseURL:                       cfg.Media.GramatonBaseURL,
		Username:                      cfg.Media.Username,
		Password:                      cfg.Media.Password,
		DestinationPath:               cfg.Media.DestinationPath,
		MovieDestinationPath:          cfg.Media.MovieDestinationPath,
		TVDestinationPath:             cfg.Media.TVDestinationPath,
		RequireConfirmation:           cfg.Media.RequireConfirmation,
		RequireConfirmationConfigured: true,
		EnvPath:                       cfg.ConfigPath,
	}, files, logger)
	notes := agentnotes.New(cfg.NotesRoot)

	client := agent.NewClient(cfg.CloudURL, cfg.AgentID, cfg.Token, cfg.HomeName, cfg.ConfigPath, ha, files, media, notes, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("agent exited with error", "error", err)
		os.Exit(1)
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
		})
	}
	return configs
}
