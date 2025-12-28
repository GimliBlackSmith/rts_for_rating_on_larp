package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rts_for_rating_on_larp/internal/admin"
	"rts_for_rating_on_larp/internal/config"
	"rts_for_rating_on_larp/internal/db"
	"rts_for_rating_on_larp/internal/telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	if cfg.TelegramToken == "" {
		logger.Error("TELEGRAM_TOKEN is required")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := db.NewStore(pool)
	if cfg.MigrateOnStart {
		if err := db.RunMigrations(ctx, pool); err != nil {
			logger.Error("run migrations", "error", err)
			os.Exit(1)
		}
	}
	if err := store.EnsureSystemConfig(ctx); err != nil {
		logger.Error("ensure system config", "error", err)
		os.Exit(1)
	}

	botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		logger.Error("init telegram bot", "error", err)
		os.Exit(1)
	}

	if cfg.WebhookURL != "" {
		webhookURL := cfg.WebhookURL + cfg.WebhookPath
		var webhook tgbotapi.WebhookConfig
		if cfg.WebhookCert != "" {
			webhook, err = tgbotapi.NewWebhookWithCert(webhookURL, tgbotapi.FilePath(cfg.WebhookCert))
		} else {
			webhook, err = tgbotapi.NewWebhook(webhookURL)
		}
		if err != nil {
			logger.Error("create webhook", "error", err)
			os.Exit(1)
		}
		if _, err := botAPI.Request(webhook); err != nil {
			logger.Error("set webhook", "error", err)
			os.Exit(1)
		}
		logger.Info("webhook configured", "url", webhookURL)
	}

	bot := telegram.New(botAPI, store, logger)
	adminHandler, err := admin.New(store, cfg.AdminToken)
	if err != nil {
		logger.Error("init admin handler", "error", err)
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/admin", adminHandler)
	mux.Handle("/admin/action", adminHandler)
	mux.Handle(cfg.WebhookPath, bot.WebhookHandler())

	server := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server started", "addr", cfg.ServerAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
