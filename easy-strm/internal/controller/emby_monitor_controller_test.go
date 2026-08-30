package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"easy-strm/internal/domain"
	"github.com/gin-gonic/gin"
)

type embyMonitorControllerStub struct{}

func (embyMonitorControllerStub) GetOverview(int) (*domain.EmbyMonitorOverview, error) {
	return &domain.EmbyMonitorOverview{Sessions: []domain.EmbyMonitorSession{}}, nil
}
func (embyMonitorControllerStub) GetRankings(int, string, string, string) (*domain.EmbyMonitorRankingResponse, error) {
	return &domain.EmbyMonitorRankingResponse{Data: []domain.EmbyMonitorRankingItem{}}, nil
}
func (embyMonitorControllerStub) GetHeatmap(int, string, string) (*domain.EmbyMonitorHeatmapResponse, error) {
	return &domain.EmbyMonitorHeatmapResponse{Users: []domain.EmbyMonitorUserHeatmap{}}, nil
}
func (embyMonitorControllerStub) GetRecentItems(int, string, string, string) (*domain.EmbyMonitorRecentResponse, error) {
	return &domain.EmbyMonitorRecentResponse{Data: []domain.EmbyMonitorItem{}}, nil
}
func (embyMonitorControllerStub) GetItemImage(int, string) ([]byte, string, error) {
	return []byte("image"), "image/jpeg", nil
}

func TestEmbyMonitorControllerRejectsInvalidServerID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := &EmbyMonitorController{service: embyMonitorControllerStub{}}
	router.GET("/emby/servers/:server_id/monitor/overview", controller.GetOverview)
	request := httptest.NewRequest(http.MethodGet, "/emby/servers/no/monitor/overview", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEmbyMonitorControllerReturnsStableOverview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := &EmbyMonitorController{service: embyMonitorControllerStub{}}
	router.GET("/emby/servers/:server_id/monitor/overview", controller.GetOverview)
	request := httptest.NewRequest(http.MethodGet, "/emby/servers/1/monitor/overview", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); body == "" || body == "null" {
		t.Fatalf("响应不能为空: %s", body)
	}
}
