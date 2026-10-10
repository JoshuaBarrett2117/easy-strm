package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestExportLoggingHasTaskRequestAndSingleFileEvents(t *testing.T) {
	output := captureEmbyProxyLog(t)
	service, store, _ := strmFixture(t)
	source := phaseSource(1)
	source.Password = "synthetic-private-password"
	source.URL += "?password=synthetic-private-query"
	store.sources = []domain.ShareStrmSource{source}
	cfg, err := service.Settings()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BaseURL = "https://example.invalid?token=synthetic-private-playback"
	ctx := logger.WithRequestID(context.Background(), "request-synthetic")
	if err := service.export(ctx, cfg, domain.ShareLibraryQuery{}, "task-synthetic"); err != nil {
		t.Fatal(err)
	}
	logs := output.String()
	for _, required := range []string{`"event":"export_start"`, `"event":"export_single"`, `"event":"export_complete"`, `"written":true`} {
		if !strings.Contains(logs, required) {
			t.Fatal("真实导出流程缺少日志事件")
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var entry map[string]interface{}
		if json.Unmarshal([]byte(line), &entry) != nil || entry["task_id"] != "task-synthetic" || entry["request_id"] != "request-synthetic" {
			t.Fatal("导出上下文未传到单文件步骤")
		}
	}
	store.fileErr = errors.New("登记失败: api_key=synthetic-private-error")
	if err := service.export(ctx, cfg, domain.ShareLibraryQuery{}, "task-synthetic-failed"); err == nil {
		t.Fatal("模拟登记失败应返回错误")
	}
	logs = output.String()
	if !strings.Contains(logs, `"event":"export_single_error"`) || !strings.Contains(logs, `"level":"ERROR"`) || strings.Contains(logs, "synthetic-private") {
		t.Fatal("导出错误日志缺失或泄露敏感内容")
	}
}

func TestTaskLifecycleLogsHaveTaskID(t *testing.T) {
	output := captureEmbyProxyLog(t)
	service := shareProgressFaultTasks(t, &shareProgressFaultHook{})
	if err := service.CreateWithPriority("task-synthetic", "share_export", "synthetic-private-name", 5); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"task_id":"task-synthetic"`) || strings.Contains(output.String(), "synthetic-private-name") {
		t.Fatal("任务日志必须关联 ID 且不记录任务原始负载")
	}
}

func TestLoginMismatchNeverLogsPasswordOrAccount(t *testing.T) {
	output := captureEmbyProxyLog(t)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	dao.InitDAO(database)
	t.Cleanup(func() { dao.InitDAO(nil) })
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, password, create_time, update_time FROM t_user WHERE name = $1")).
		WithArgs("synthetic-private-name").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "password", "create_time", "update_time"}).AddRow(1, "synthetic-private-name", "synthetic-private-password", time.Now(), time.Now()))
	service := NewAuthService(dao.NewUserDAO(), "synthetic-private-jwt")
	if _, _, err := service.Login("synthetic-private-name", "synthetic-private-input"); err == nil {
		t.Fatal("密码不匹配应失败")
	}
	if strings.Contains(output.String(), "synthetic-private") || !strings.Contains(output.String(), "密码不匹配") {
		t.Fatal("登录日志不应记录账号或任意无标签密码")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTokenVerificationLogsExcludeJWTSecret(t *testing.T) {
	output := captureEmbyProxyLog(t)
	logger.SetLevel(logger.DEBUG)
	t.Cleanup(func() { logger.SetLevel(logger.INFO) })
	service := NewAuthService(nil, "synthetic private jwt secret")
	if _, err := service.VerifyToken("synthetic-invalid-token"); err == nil {
		t.Fatal("无效 token 必须失败")
	}
	if strings.Contains(output.String(), "private jwt secret") {
		t.Fatal("不得把 JWT secret 交给日志文本清洗兜底")
	}
}

func TestExternalHTTPFailuresOmitResponseBodies(t *testing.T) {
	client := &http.Client{Transport: playbackTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("synthetic-private-response-body"))}, nil
	})}
	emby := NewEmbyService(&mockSystemConfigDAO{configs: map[string]string{"emby_url": "https://example.invalid", "emby_api_key": "synthetic-private-api-key", "emby_enabled": "true"}}, client)
	for _, action := range []func() error{
		func() error { _, _, err := emby.CheckConnection(); return err },
		func() error { _, err := emby.ListLibraries(); return err },
		func() error { _, err := emby.RefreshAll(); return err },
	} {
		if err := action(); err == nil || strings.Contains(err.Error(), "synthetic-private") {
			t.Fatal("外部 HTTP 错误不得把响应体放入错误链")
		}
	}
	result, err := emby.RefreshLibrary("library-synthetic")
	if err != nil || result == nil || result.Success || strings.Contains(result.Message, "synthetic-private") {
		t.Fatal("Emby 失败诊断不得携带响应体")
	}
	request, err := http.NewRequest("GET", "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	notifications := NewNotificationService(nil, client)
	if err := notifications.doWeComJSON(request, &map[string]interface{}{}, WeComConfig{}); err == nil || strings.Contains(err.Error(), "synthetic-private") {
		t.Fatal("企业微信 HTTP 错误不得携带响应体")
	}
}
