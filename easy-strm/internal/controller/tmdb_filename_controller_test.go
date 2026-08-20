package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

type controllerFilenameRuleStore struct {
	value string
}

func (s *controllerFilenameRuleStore) GetByKey(string) (*domain.SystemConfig, error) {
	return &domain.SystemConfig{ConfigVal: s.value}, nil
}

func (s *controllerFilenameRuleStore) Upsert(_ string, value string) error {
	s.value = value
	return nil
}

func TestTmdbControllerParseFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := &TmdbController{tmdbService: service.NewTmdbService("fake-key", nil)}
	router := gin.New()
	router.POST("/parse", controller.ParseFilename)

	request := httptest.NewRequest(http.MethodPost, "/parse", strings.NewReader(`{"filename":"妖精的尾巴 百年任务 - S01E05 - 艰难的决断.mp4"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data service.ParsedFilename `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.Data.Title != "妖精的尾巴 百年任务" || payload.Data.Season != 1 || payload.Data.Episode != 5 {
		t.Fatalf("unexpected parse payload: %+v", payload.Data)
	}
}

func TestTmdbControllerParseFilenameRejectsEmptyInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := &TmdbController{tmdbService: service.NewTmdbService("fake-key", nil)}
	router := gin.New()
	router.POST("/parse", controller.ParseFilename)

	request := httptest.NewRequest(http.MethodPost, "/parse", strings.NewReader(`{"filename":" "}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestTmdbControllerUpdateFilenameRecognitionRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmdbService := service.NewTmdbService("fake-key", nil)
	store := &controllerFilenameRuleStore{}
	tmdbService.SetFilenameRecognitionRuleStore(store)
	controller := &TmdbController{tmdbService: tmdbService}
	router := gin.New()
	router.PUT("/rules", controller.UpdateFilenameRecognitionRules)

	body := `{"rules":[{"id":"custom_tv","name":"自定义","pattern":"^(?P<title>.+?)\\s+P(?P<episode>\\d{1,3})$","media_type":"tv","default_season":1,"example":"Show P03.mkv","enabled":true,"priority":10}]}`
	request := httptest.NewRequest(http.MethodPut, "/rules", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	parsed := tmdbService.ParseFilename("Show.P03.mkv")
	if parsed.Title != "Show" || parsed.Season != 1 || parsed.Episode != 3 {
		t.Fatalf("updated rule was not applied: %+v", parsed)
	}
}

func TestTmdbControllerUpdateFilenameRecognitionRulesRejectsInvalidRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmdbService := service.NewTmdbService("fake-key", nil)
	tmdbService.SetFilenameRecognitionRuleStore(&controllerFilenameRuleStore{})
	controller := &TmdbController{tmdbService: tmdbService}
	router := gin.New()
	router.PUT("/rules", controller.UpdateFilenameRecognitionRules)

	body := `{"rules":[{"id":"broken","name":"错误规则","pattern":"(?P<title>.+","media_type":"tv","enabled":true}]}`
	request := httptest.NewRequest(http.MethodPut, "/rules", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}
