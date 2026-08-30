package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmbyManagementControllerRejectsInvalidServerID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewEmbyManagementController(nil)
	router.GET("/emby/servers/:server_id/users", controller.ListUsers)

	request := httptest.NewRequest(http.MethodGet, "/emby/servers/not-a-number/users", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 %d", response.Code, http.StatusBadRequest)
	}
}

func TestRunPluginTaskRequiresAutoConfigureConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewEmbyManagementController(nil)
	router.POST("/emby/servers/:server_id/plugin/strm-assistant/tasks", controller.RunPluginTask)

	request := httptest.NewRequest(http.MethodPost, "/emby/servers/1/plugin/strm-assistant/tasks", bytes.NewBufferString(`{"action":"strm_scan_capture","library_id":"lib-1"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 %d", response.Code, http.StatusBadRequest)
	}
}
