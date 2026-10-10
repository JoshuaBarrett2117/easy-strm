package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easy-strm/internal/domain"
	"github.com/gin-gonic/gin"
)

func TestBatchFileOperationHTTPRejectsOversizeBeforeLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := &FileOperationController{}
	router := gin.New()
	router.POST("/batch", controller.BatchOperation)
	items := make([]domain.BatchOperationItem, 501)
	body, err := json.Marshal(gin.H{"operation": "move", "source_id": 1, "items": items})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("大批次未在配置查询前拒绝：%d %s", response.Code, response.Body.String())
	}
}
