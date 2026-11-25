package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easy-strm/internal/domain"

	"github.com/gin-gonic/gin"
)

func TestTaskControllerGetTypesReturnsCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/tasks/types", (&TaskController{}).GetTypes)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/tasks/types", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d", recorder.Code)
	}
	var body struct {
		Data []domain.TaskTypeInfo `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != len(domain.TaskTypeCatalog) || body.Data[0].Value == "" || body.Data[0].Label == "" {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}
