package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/config"
	httpapi "github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/http"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/platform/fileclient"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/repository"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration failed", "service", "document", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo, err := repository.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database initialization failed", "service", "document", "dependency", "postgres", "error", err)
		os.Exit(1)
	}
	defer repo.Close()

	files, err := fileclient.New(cfg.FileServiceURL, nil)
	if err != nil {
		logger.Error("file client initialization failed", "service", "document", "dependency", "file", "error", err)
		os.Exit(1)
	}
	taskClient := worker.NewClient(cfg.RedisAddr)
	documents := service.New(repo, files)
	reportService := service.NewReportService(repo)
	jobService := service.NewJobService(repo, taskClient)

	handler := httpapi.NewServer(httpapi.Config{
		Logger:          logger,
		ReadyChecker:    repo,
		DocumentService: documents,
		ReportService:   reportService,
		JobSvc:          jobService,
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		logger.Info("document service starting", "service", "document", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("document service stopped unexpectedly", "service", "document", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	logger.Info("document service shutdown started", "service", "document")
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("document service shutdown failed", "service", "document", "error", err)
		os.Exit(1)
	}
	logger.Info("document service shutdown complete", "service", "document")
}
