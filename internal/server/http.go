package server

import (
	"github.com/go-kratos/kratos/v2/log"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	"rts_for_rating_on_larp/internal/data"
	"rts_for_rating_on_larp/internal/service"
)

func NewHTTPServer(logger log.Logger, db *data.PostgresPool, redis *data.RedisClient) *kratoshttp.Server {
	srv := kratoshttp.NewServer(
		kratoshttp.Address(":8080"),
	)

	srv.Handle("/healthz", service.NewHealthHandler(logger, db, redis))

	return srv
}
