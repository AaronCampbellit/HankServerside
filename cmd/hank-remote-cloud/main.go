package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dropfile/hankremote/internal/cloud"
	"github.com/dropfile/hankremote/internal/config"
	"github.com/dropfile/hankremote/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.LoadCloud()
	if err != nil {
		logger.Error("failed to load cloud config", "error", err)
		os.Exit(1)
	}
	for _, warning := range config.RuntimeEnvFileWarnings(".env.cloud") {
		logger.Warn("runtime env file is group/world-readable; run chmod 600", "path", warning.Path, "mode", warning.Mode.String())
	}

	ctx := context.Background()
	if len(os.Args) >= 2 && os.Args[1] == "migrate" {
		if err := runMigrateCommand(ctx, cfg, os.Args[2:]); err != nil {
			logger.Error("migration command failed", "error", err)
			os.Exit(1)
		}
		return
	}

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to open cloud storage", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.ConfigureSecretEncryption(cfg.SecretKey); err != nil {
		logger.Error("failed to configure secret encryption", "error", err)
		os.Exit(1)
	}

	server := cloud.NewServer(cfg.Addr, db, cfg.SessionTTL, cfg.RequestTimeout, logger)
	server.ConfigureOpenAI(cfg.OpenAIClientID, cfg.OpenAIClientSecret, cfg.OpenAIRedirectURI, cfg.OpenAIScopes)
	server.ConfigureAPNS(cloud.APNSConfig{
		TeamID:      cfg.APNS.TeamID,
		KeyID:       cfg.APNS.KeyID,
		PrivateKey:  cfg.APNS.PrivateKey,
		Topic:       cfg.APNS.Topic,
		Environment: cfg.APNS.Environment,
	})
	server.ConfigureAssistantAI(cloud.AssistantAIConfig{
		Provider:              cfg.AssistantAI.Provider,
		OllamaBaseURL:         cfg.AssistantAI.OllamaBaseURL,
		OllamaChatModel:       cfg.AssistantAI.OllamaChatModel,
		OllamaEmbeddingModel:  cfg.AssistantAI.OllamaEmbeddingModel,
		OpenAIBaseURL:         cfg.AssistantAI.OpenAIBaseURL,
		OpenAIAPIKey:          cfg.AssistantAI.OpenAIAPIKey,
		OpenAIChatModel:       cfg.AssistantAI.OpenAIChatModel,
		OpenAIEmbeddingModel:  cfg.AssistantAI.OpenAIEmbeddingModel,
		ChatGPTOAuthEnabled:   cfg.AssistantAI.ChatGPTOAuthEnabled,
		ChatGPTAuthIssuer:     cfg.AssistantAI.ChatGPTAuthIssuer,
		ChatGPTBackendBaseURL: cfg.AssistantAI.ChatGPTBackendBaseURL,
		ChatGPTClientID:       cfg.AssistantAI.ChatGPTClientID,
		ChatGPTChatModel:      cfg.AssistantAI.ChatGPTChatModel,
		ProjectDocsDir:        cfg.AssistantAI.ProjectDocsDir,
		EmbeddingDimension:    cfg.AssistantAI.EmbeddingDimension,
	})
	server.ConfigureStorageOps(cfg.DBOpsStateDir, cfg.DBOpsLogDir, cfg.DBOpsIntentSecret)
	server.ConfigureNoteAttachmentStorage(cfg.NoteAttachmentDir)
	if err := server.StartRuntime(ctx, "dev"); err != nil {
		logger.Error("failed to start runtime coordination", "error", err)
		os.Exit(1)
	}
	server.StartMaintenance(time.Hour, 30*24*time.Hour)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	shutdownSignalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil {
			logger.Error("cloud server failed", "error", err)
			os.Exit(1)
		}
	case <-shutdownSignalCtx.Done():
		logger.Info("shutting down cloud server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("cloud shutdown failed", "error", err)
		os.Exit(1)
	}
}

func runMigrateCommand(ctx context.Context, cfg config.Cloud, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hank-remote-cloud migrate [up|status|baseline]")
	}

	switch args[0] {
	case "up", "baseline":
		db, err := store.OpenMigrating(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
		return db.CheckMigrations(ctx)
	case "status":
		db, err := store.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
		statuses, err := db.MigrationStatuses(ctx)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			fmt.Printf("%06d %s checksum=%s applied_at=%s duration_ms=%d\n", status.Version, status.Name, status.Checksum, status.AppliedAt.Format(time.RFC3339), status.DurationMS)
		}
		if len(args) > 1 && args[1] == "--strict" {
			return db.CheckMigrations(ctx)
		}
		return nil
	default:
		return errors.New("unknown migrate command")
	}
}
