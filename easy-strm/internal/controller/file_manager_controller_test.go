package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
)

type fakeFileManagerUseCase struct {
	locations []domain.FileManagerLocation
}

func (f *fakeFileManagerUseCase) ListLocations() ([]domain.FileManagerLocation, error) {
	return f.locations, nil
}
func (f *fakeFileManagerUseCase) Browse(location domain.FileManagerLocationRef, path string) (*domain.FileManagerBrowseResult, error) {
	return &domain.FileManagerBrowseResult{Path: path, Entries: []domain.FileManagerEntry{}, Total: 0}, nil
}
func (f *fakeFileManagerUseCase) StartTransfer(req domain.FileManagerTransferRequest) (*domain.FileManagerTransferResponse, error) {
	return &domain.FileManagerTransferResponse{TaskID: "task-1", Total: len(req.Items)}, nil
}
func (f *fakeFileManagerUseCase) Delete(domain.FileManagerDeleteRequest) error { return nil }

func TestFileManagerControllerLocationsAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &FileManagerController{service: &fakeFileManagerUseCase{locations: []domain.FileManagerLocation{{Type: "local", ID: 1, Name: "媒体"}}}}
	router := gin.New()
	router.GET("/locations", c.ListLocations)
	router.GET("/files", c.Browse)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/locations", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("媒体")) {
		t.Fatalf("unexpected locations response: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/files?location_type=local&location_id=bad", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestFileManagerControllerCreatesTransferTask(t *testing.T) {
	c := &FileManagerController{service: &fakeFileManagerUseCase{}}
	router := gin.New()
	router.POST("/transfers", c.Transfer)
	body, _ := json.Marshal(domain.FileManagerTransferRequest{Operation: "copy", Source: domain.FileManagerLocationRef{Type: "local", ID: 1}, Target: domain.FileManagerLocationRef{Type: "cloud115", ID: 2}, Items: []domain.FileManagerTransferItem{{ID: "a", Name: "a", Path: "a"}}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/transfers", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !bytes.Contains(response.Body.Bytes(), []byte("task-1")) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
