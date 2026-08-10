package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
)

// stubOfflineDownloadService 云下载服务桩实现，记录入参并返回预置结果。
type stubOfflineDownloadService struct {
	submitResp *domain.OfflineDownloadSubmitResponse
	submitErr  error
	listResp   *domain.OfflineDownloadTaskListResponse
	listErr    error
	deleteErr  error

	lastReq         domain.OfflineDownloadSubmitRequest
	lastListArgs    map[string]interface{}
	lastDeleteID    int64
	lastDeleteFiles bool
}

func (s *stubOfflineDownloadService) Submit(ctx context.Context, req domain.OfflineDownloadSubmitRequest) (*domain.OfflineDownloadSubmitResponse, error) {
	s.lastReq = req
	return s.submitResp, s.submitErr
}

func (s *stubOfflineDownloadService) List(ctx context.Context, cloud115ID int, status string, page, pageSize int) (*domain.OfflineDownloadTaskListResponse, error) {
	s.lastListArgs = map[string]interface{}{
		"cloud115_id": cloud115ID, "status": status, "page": page, "page_size": pageSize,
	}
	return s.listResp, s.listErr
}

func (s *stubOfflineDownloadService) DeleteRecord(ctx context.Context, id int64, deleteFiles bool) error {
	s.lastDeleteID = id
	s.lastDeleteFiles = deleteFiles
	return s.deleteErr
}

// newOfflineDownloadTestRouter 构建云下载控制器测试路由。
func newOfflineDownloadTestRouter(stub *stubOfflineDownloadService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	c := NewOfflineDownloadController(stub)
	router := gin.New()
	router.POST("/v1/resource/115-offline/submit", c.Submit)
	router.GET("/v1/resource/115-offline/tasks", c.List)
	router.DELETE("/v1/resource/115-offline/tasks/:id", c.Delete)
	return router
}

// doRequest 执行请求并返回响应记录器。
func doOfflineRequest(router *gin.Engine, method, target string, body []byte) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// parseRespBody 解析统一响应结构，返回 state 与 data。
func parseRespBody(t *testing.T, response *httptest.ResponseRecorder) (bool, map[string]interface{}) {
	t.Helper()
	var parsed struct {
		State bool                   `json:"state"`
		Data  map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("响应体解析失败: %v, body=%s", err, response.Body.String())
	}
	return parsed.State, parsed.Data
}

// TestOfflineDownloadSubmitSuccess 验证提交成功时返回任务ID与逐链接结果。
func TestOfflineDownloadSubmitSuccess(t *testing.T) {
	stub := &stubOfflineDownloadService{
		submitResp: &domain.OfflineDownloadSubmitResponse{
			TaskId:   "offline-123",
			Total:    1,
			Accepted: 1,
			Results: []domain.OfflineDownloadUrlResult{
				{Url: "ed2k://|file|a.mkv|1|HASH|/", InfoHash: "hash", Accepted: true},
			},
		},
	}
	router := newOfflineDownloadTestRouter(stub)

	body := `{"cloud115_id":3,"directory":"/云下载","urls":["ed2k://|file|a.mkv|1|HASH|/"]}`
	response := doOfflineRequest(router, http.MethodPost, "/v1/resource/115-offline/submit", []byte(body))

	if response.Code != http.StatusOK {
		t.Fatalf("状态码不符合预期: got %d want %d, body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	state, data := parseRespBody(t, response)
	if !state {
		t.Fatal("期望 state=true")
	}
	if data["task_id"] != "offline-123" {
		t.Fatalf("task_id 不符合预期: %v", data["task_id"])
	}
	if stub.lastReq.Cloud115ID != 3 || stub.lastReq.Directory != "/云下载" || len(stub.lastReq.Urls) != 1 {
		t.Fatalf("请求参数未正确透传: %+v", stub.lastReq)
	}
}

// TestOfflineDownloadSubmitMissingAccount 验证未选择账号时返回400。
func TestOfflineDownloadSubmitMissingAccount(t *testing.T) {
	stub := &stubOfflineDownloadService{}
	router := newOfflineDownloadTestRouter(stub)

	response := doOfflineRequest(router, http.MethodPost, "/v1/resource/115-offline/submit",
		[]byte(`{"cloud115_id":0,"urls":["magnet:?xt=urn:btih:abc"]}`))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码不符合预期: got %d want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "115账号") {
		t.Fatalf("错误信息不符合预期: %s", response.Body.String())
	}
}

// TestOfflineDownloadSubmitEmptyUrls 验证链接为空时返回400。
func TestOfflineDownloadSubmitEmptyUrls(t *testing.T) {
	stub := &stubOfflineDownloadService{}
	router := newOfflineDownloadTestRouter(stub)

	response := doOfflineRequest(router, http.MethodPost, "/v1/resource/115-offline/submit",
		[]byte(`{"cloud115_id":1,"urls":[]}`))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码不符合预期: got %d want %d", response.Code, http.StatusBadRequest)
	}
}

// TestOfflineDownloadSubmitServiceError 验证 service 报错时以400返回业务信息。
func TestOfflineDownloadSubmitServiceError(t *testing.T) {
	stub := &stubOfflineDownloadService{submitErr: errors.New("账号cookie无效")}
	router := newOfflineDownloadTestRouter(stub)

	response := doOfflineRequest(router, http.MethodPost, "/v1/resource/115-offline/submit",
		[]byte(`{"cloud115_id":1,"urls":["magnet:?xt=urn:btih:abc"]}`))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码不符合预期: got %d want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "cookie无效") {
		t.Fatalf("错误信息不符合预期: %s", response.Body.String())
	}
}

// TestOfflineDownloadListQueryParsing 验证列表查询参数解析与透传。
func TestOfflineDownloadListQueryParsing(t *testing.T) {
	stub := &stubOfflineDownloadService{
		listResp: &domain.OfflineDownloadTaskListResponse{
			Data:  []domain.OfflineDownloadTask{{ID: 1, Status: "completed"}},
			Total: 1,
		},
	}
	router := newOfflineDownloadTestRouter(stub)

	response := doOfflineRequest(router, http.MethodGet,
		"/v1/resource/115-offline/tasks?cloud115_id=2&status=downloading&page=3&page_size=50", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("状态码不符合预期: got %d want %d, body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	args := stub.lastListArgs
	if args["cloud115_id"] != 2 || args["status"] != "downloading" || args["page"] != 3 || args["page_size"] != 50 {
		t.Fatalf("查询参数未正确透传: %+v", args)
	}
	state, data := parseRespBody(t, response)
	if !state {
		t.Fatal("期望 state=true")
	}
	if data["total"] != float64(1) {
		t.Fatalf("total 不符合预期: %v", data["total"])
	}
}

// TestOfflineDownloadDeleteInvalidID 验证非法记录ID返回400。
func TestOfflineDownloadDeleteInvalidID(t *testing.T) {
	stub := &stubOfflineDownloadService{}
	router := newOfflineDownloadTestRouter(stub)

	response := doOfflineRequest(router, http.MethodDelete, "/v1/resource/115-offline/tasks/abc", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码不符合预期: got %d want %d", response.Code, http.StatusBadRequest)
	}
}

// TestOfflineDownloadDeleteSuccess 验证删除成功时ID与delete_files正确透传。
func TestOfflineDownloadDeleteSuccess(t *testing.T) {
	stub := &stubOfflineDownloadService{}
	router := newOfflineDownloadTestRouter(stub)

	response := doOfflineRequest(router, http.MethodDelete, "/v1/resource/115-offline/tasks/9?delete_files=true", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("状态码不符合预期: got %d want %d, body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if stub.lastDeleteID != 9 {
		t.Fatalf("记录ID不符合预期: got %d want 9", stub.lastDeleteID)
	}
	if !stub.lastDeleteFiles {
		t.Fatal("delete_files 未正确透传")
	}
	state, data := parseRespBody(t, response)
	if !state {
		t.Fatal("期望 state=true")
	}
	if data["deleted"] != float64(9) {
		t.Fatalf("deleted 不符合预期: %v", data["deleted"])
	}
}
