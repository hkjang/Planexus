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

	"github.com/hkjang/Planexus/internal/config"
	"github.com/hkjang/Planexus/internal/database"
	"github.com/hkjang/Planexus/internal/secure"
	"github.com/hkjang/Planexus/internal/server"
)

var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	db, err := database.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		cancel()
		slog.Error("database startup failed", "error", err)
		os.Exit(1)
	}
	defer db.Pool.Close()
	if err = db.Migrate(ctx); err != nil {
		cancel()
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	if err = db.BootstrapAdmin(ctx, cfg.BootstrapAdmin, cfg.BootstrapAdminPassword); err != nil {
		cancel()
		slog.Error("bootstrap failed", "error", err)
		os.Exit(1)
	}
	cancel()
	vault, err := secure.NewVault(cfg.EncryptionKey)
	if err != nil {
		slog.Error("encryption setup failed", "error", err)
		os.Exit(1)
	}
	app := server.New(db.Pool, vault, version)
	jobCtx, stopJobs := context.WithCancel(context.Background())
	defer stopJobs()
	app.Start(jobCtx)
	httpServer := &http.Server{Addr: ":8080", Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 10 * time.Minute, WriteTimeout: 10 * time.Minute, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		slog.Info("Planexus started", "version", version, "address", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()
	<-stop
	stopJobs()
	slog.Info("Planexus shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}
