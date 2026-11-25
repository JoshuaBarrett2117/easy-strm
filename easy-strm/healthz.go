package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	pkglogger "easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

// healthzPath 是无需鉴权的存活/就绪探针；nginx 同时暴露 /healthz 与 /api/healthz。
const healthzPath = "/healthz"

const healthzTimeout = 2 * time.Second

type dependencyStatus struct {
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type healthzResponse struct {
	Status string                      `json:"status"`
	Checks map[string]dependencyStatus `json:"checks"`
}

// healthzState 只在依赖状态变化时写日志，探针每 30 秒一次，持续故障不刷屏。
var healthzState = struct {
	sync.Mutex
	last map[string]string
}{last: map[string]string{}}

type pinger func(ctx context.Context) error

// newHealthzHandler 依次探测 PostgreSQL 与 Redis：全部可用返回 200，任一不可用返回 503。
func newHealthzHandler(checks map[string]pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), healthzTimeout)
		defer cancel()
		response := healthzResponse{Status: "ok", Checks: map[string]dependencyStatus{}}
		for name, check := range checks {
			started := time.Now()
			result := dependencyStatus{Status: "ok"}
			if err := check(ctx); err != nil {
				result.Status = "error"
				result.Error = pkglogger.SanitizeText(err.Error())
				response.Status = "error"
			}
			result.LatencyMS = time.Since(started).Milliseconds()
			response.Checks[name] = result
			recordHealthTransition(c.Request.Context(), name, result)
		}
		code := http.StatusOK
		if response.Status != "ok" {
			code = http.StatusServiceUnavailable
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(code, response)
	}
}

func recordHealthTransition(ctx context.Context, name string, result dependencyStatus) {
	healthzState.Lock()
	previous, seen := healthzState.last[name]
	healthzState.last[name] = result.Status
	healthzState.Unlock()
	if previous == result.Status || (!seen && result.Status == "ok") {
		return
	}
	entry := pkglogger.WithContext(ctx, "healthz")
	if result.Status == "ok" {
		entry.Log(pkglogger.INFO, "依赖恢复可用", pkglogger.Fields{"dependency": name}, nil)
		return
	}
	entry.Log(pkglogger.WARN, "依赖不可用", pkglogger.Fields{"dependency": name, "error": result.Error}, nil)
}

// defaultHealthChecks 使用全局 PostgreSQL / Redis 连接。
func defaultHealthChecks() map[string]pinger {
	return map[string]pinger{
		"postgres": func(ctx context.Context) error {
			if db == nil {
				return errNotInitialized
			}
			return db.PingContext(ctx)
		},
		"redis": func(ctx context.Context) error {
			if redisClient == nil {
				return errNotInitialized
			}
			return redisClient.Ping(ctx).Err()
		},
	}
}

type notInitializedError struct{}

func (notInitializedError) Error() string { return "not initialized" }

var errNotInitialized error = notInitializedError{}
