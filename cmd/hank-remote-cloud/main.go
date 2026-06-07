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
	if len(os.Args) >= 2 && os.Args[1] == "secrets" {
		if err := runSecretsCommand(ctx, cfg, os.Args[2:]); err != nil {
			logger.Error("secret storage command failed", "error", err)
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
	if cfg.AllowPlaintextSecrets {
		logger.Warn("plaintext secret storage is explicitly enabled; set HANK_REMOTE_SECRET_ENCRYPTION_KEY before production use")
	}

	server := cloud.NewServer(cfg.Addr, db, cfg.SessionTTL, cfg.RequestTimeout, logger)
	server.ConfigureSecretStorageStatus(cfg.SecretKey != "", cfg.AllowPlaintextSecrets)
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
	server.StartMaintenance(cfg.MaintenanceInterval, cfg.MaintenanceRetention)

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
	case "up":
		db, err := store.OpenMigrating(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
		return db.CheckMigrations(ctx)
	case "baseline":
		return store.BaselineExisting(ctx, cfg.DatabaseURL)
	case "status":
		statuses, err := store.MigrationStatuses(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			fmt.Printf("%06d %s checksum=%s applied_at=%s duration_ms=%d\n", status.Version, status.Name, status.Checksum, status.AppliedAt.Format(time.RFC3339), status.DurationMS)
		}
		if len(args) > 1 && args[1] == "--strict" {
			return store.CheckMigrations(ctx, cfg.DatabaseURL)
		}
		return nil
	default:
		return errors.New("unknown migrate command")
	}
}

func runSecretsCommand(ctx context.Context, cfg config.Cloud, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hank-remote-cloud secrets [status|reencrypt]")
	}
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.ConfigureSecretEncryption(cfg.SecretKey); err != nil {
		return err
	}

	switch args[0] {
	case "status":
		report, err := db.SecretStorageReport(ctx)
		if err != nil {
			return err
		}
		printSecretStorageReport(report)
		if len(args) > 1 && args[1] == "--strict" && report.PlaintextTotal() > 0 {
			return fmt.Errorf("plaintext secret rows detected")
		}
		return nil
	case "reencrypt":
		report, err := db.ReencryptPlaintextSecrets(ctx)
		if err != nil {
			return err
		}
		printSecretStorageReport(report)
		fmt.Printf("secret_columns_reencrypted=%d\n", report.ReencryptedSecretColumns)
		if report.PlaintextTotal() > 0 {
			return fmt.Errorf("plaintext secret rows remain after re-encryption")
		}
		return nil
	default:
		return errors.New("unknown secrets command")
	}
}

func printSecretStorageReport(report store.SecretStorageReport) {
	fmt.Printf("plaintext_openai_access_tokens=%d\n", report.OpenAIAccessTokens)
	fmt.Printf("plaintext_openai_refresh_tokens=%d\n", report.OpenAIRefreshTokens)
	fmt.Printf("plaintext_apns_device_tokens=%d\n", report.APNSDeviceTokens)
	fmt.Printf("plaintext_user_profile_secret_vaults=%d\n", report.UserProfileSecretVaults)
	fmt.Printf("plaintext_total=%d\n", report.PlaintextTotal())
}
