package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"easy-strm/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

func TestLogViewerHumanAndJSONCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { logger.SetFormat(""); logger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr) })
	for _, format := range []string{"", "json"} {
		directory := t.TempDir()
		filename := "info_fixture.log"
		file, err := os.Create(filepath.Join(directory, filename))
		if err != nil {
			t.Fatal(err)
		}
		logger.SetFormat(format)
		logger.SetOutputs(file, file, file, file)
		entry := logger.WithContext(logger.WithRequestID(context.Background(), "viewer-request"), "viewer")
		entry.Log(logger.INFO, "first", nil, nil)
		entry.Log(logger.INFO, "second", nil, nil)
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		logger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr)
		controller := NewLogController(nil)
		controller.SetLogDir(directory)
		controller.SetIsAdminChecker(func(*gin.Context) (string, bool) { return "fixture", true })
		router := gin.New()
		router.GET("/logs/:filename", controller.GetFileContent)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/logs/"+filename+"?lines=2", nil))
		var result struct {
			Data struct {
				Content string `json:"content"`
				Lines   int    `json:"lines"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if response.Code != 200 || result.Data.Lines != 2 || strings.Index(result.Data.Content, "second") >= strings.Index(result.Data.Content, "first") {
			t.Fatalf("viewer incompatible: %s", response.Body.String())
		}
		for _, line := range strings.Split(strings.TrimSpace(result.Data.Content), "\n") {
			if format == "json" {
				var record map[string]interface{}
				if err := json.Unmarshal([]byte(line), &record); err != nil || record["request_id"] != "viewer-request" {
					t.Fatalf("JSON parsing: %s", line)
				}
			} else if !strings.Contains(line, "[req=viewer-request]") {
				t.Fatalf("human display: %s", line)
			}
		}
	}
}
