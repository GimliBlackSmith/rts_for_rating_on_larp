package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"

	"rts_for_rating_on_larp/internal/data"
	"rts_for_rating_on_larp/internal/server"
)

func main() {
	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", "rts_for_rating_on_larp",
	)
	helper := log.NewHelper(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := data.NewPostgresPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		helper.Fatalf("failed to connect to postgres: %v", err)
	}
	defer db.Close()

	redisClient, err := data.NewRedisClient(ctx, os.Getenv("REDIS_URL"))
	if err != nil {
		helper.Fatalf("failed to connect to redis: %v", err)
	}
	defer func() { _ = redisClient.Close() }()

	hs := server.NewHTTPServer(logger, db, redisClient)

	app := kratos.New(
		kratos.Name("rts_for_rating_on_larp"),
		kratos.Version("1.0.0"),
		kratos.Server(hs),
		kratos.Logger(logger),
	)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := hs.Stop(shutdownCtx); err != nil {
			helper.Error(err)
		}
	}()

	if err := app.Run(); err != nil && err != http.ErrServerClosed {
		helper.Fatalf("application error: %v", err)
	}
}
