package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func serveHealthz(t *testing.T, checks map[string]pinger) (int, healthzResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestLoggingMiddleware())
	router.GET(healthzPath, newHealthzHandler(checks))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, healthzPath, nil))
	var body healthzResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON: %s", recorder.Body.String())
	}
	return recorder.Code, body
}

func TestHealthzAllDependenciesUp(t *testing.T) {
	ok := func(context.Context) error { return nil }
	code, body := serveHealthz(t, map[string]pinger{"postgres": ok, "redis": ok})
	if code != http.StatusOK || body.Status != "ok" || body.Checks["postgres"].Status != "ok" || body.Checks["redis"].Status != "ok" {
		t.Fatalf("code=%d body=%+v", code, body)
	}
}

func TestHealthzDependencyDownReturns503(t *testing.T) {
	ok := func(context.Context) error { return nil }
	down := func(context.Context) error { return errors.New("dial tcp 127.0.0.1:6379: connect: connection refused") }
	code, body := serveHealthz(t, map[string]pinger{"postgres": ok, "redis": down})
	if code != http.StatusServiceUnavailable || body.Status != "error" {
		t.Fatalf("code=%d body=%+v", code, body)
	}
	if body.Checks["redis"].Status != "error" || body.Checks["redis"].Error == "" || body.Checks["postgres"].Status != "ok" {
		t.Fatalf("checks=%+v", body.Checks)
	}
}

func TestHealthzReportsUninitializedConnections(t *testing.T) {
	savedDB, savedRedis := db, redisClient
	db, redisClient = nil, nil
	defer func() { db, redisClient = savedDB, savedRedis }()
	code, body := serveHealthz(t, defaultHealthChecks())
	if code != http.StatusServiceUnavailable || body.Checks["postgres"].Error != "not initialized" || body.Checks["redis"].Error != "not initialized" {
		t.Fatalf("code=%d body=%+v", code, body)
	}
}
