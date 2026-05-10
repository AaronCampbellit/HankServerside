package main

import (
	"context"
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

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to open cloud storage", "error", err)
		os.Exit(1)
	}
	defer db.Close()

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
