package service

import (
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	"rts_for_rating_on_larp/internal/data"
)

type HealthHandler struct {
	logger log.Logger
	db     *data.PostgresPool
	redis  *data.RedisClient
}

func NewHealthHandler(logger log.Logger, db *data.PostgresPool, redis *data.RedisClient) *HealthHandler {
	return &HealthHandler{logger: logger, db: db, redis: redis}
}

func (h *HealthHandler) ServeHTTP(ctx kratoshttp.Context) error {
	type subsystemStatus struct {
		Status string `json:"status"`
		Detail string `json:"detail,omitempty"`
	}

	helper := log.NewHelper(h.logger)

	status := http.StatusOK
	payload := struct {
		Status    string                     `json:"status"`
		Timestamp time.Time                  `json:"timestamp"`
		Services  map[string]subsystemStatus `json:"services"`
	}{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
		Services:  make(map[string]subsystemStatus),
	}

	if err := h.db.Ping(ctx.Request().Context()); err != nil {
		status = http.StatusServiceUnavailable
		payload.Status = "degraded"
		payload.Services["postgres"] = subsystemStatus{Status: "unavailable", Detail: err.Error()}
		helper.Errorf("postgres ping failed: %v", err)
	} else {
		payload.Services["postgres"] = subsystemStatus{Status: "ok"}
	}

	if err := h.redis.Ping(ctx.Request().Context()); err != nil {
		status = http.StatusServiceUnavailable
		payload.Status = "degraded"
		payload.Services["redis"] = subsystemStatus{Status: "unavailable", Detail: err.Error()}
		helper.Errorf("redis ping failed: %v", err)
	} else {
		payload.Services["redis"] = subsystemStatus{Status: "ok"}
	}

	return ctx.JSON(status, payload)
}
