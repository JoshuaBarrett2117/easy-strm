package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

type directoryTreeDownloadURLProviderStub struct {
	downloadInfo *driver.DownloadInfo
	requestedUA  string
}

func TestBuildFullGenerateCronTaskNameUsesConfigID(t *testing.T) {
	t.Parallel()

	if actual := buildFullGenerateCronTaskName(2); actual != "STRM全量生成-2" {
		t.Fatalf("全量任务名称 = %q，期望 %q", actual, "STRM全量生成-2")
	}
}

func (s *directoryTreeDownloadURLProviderStub) DownloadWithUA(_ string, ua string) (*driver.DownloadInfo, error) {
	s.requestedUA = ua
	return s.downloadInfo, nil
}

func TestDownloadDirectoryTreeFileReusesSignatureHeaders(t *testing.T) {
	t.Parallel()

	const expectedBody = "directory tree"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.UserAgent() != driver.UA115Browser {
			response.WriteHeader(http.StatusForbidden)
			_, _ = response.Write([]byte(`{"status":403,"message":"invalid signature"}`))
			return
		}
		if cookie, err := request.Cookie("UID"); err != nil || cookie.Value != "test-user" {
			response.WriteHeader(http.StatusForbidden)
			_, _ = response.Write([]byte(`{"status":403,"message":"invalid cookie"}`))
			return
		}
		_, _ = response.Write([]byte(expectedBody))
	}))
	defer server.Close()

	headers := http.Header{}
	headers.Set("User-Agent", driver.UA115Browser)
	headers.Set("Cookie", "UID=test-user; Path=/; HttpOnly")
	provider := &directoryTreeDownloadURLProviderStub{downloadInfo: &driver.DownloadInfo{
		Url:    driver.FileDownloadUrl{Url: server.URL},
		Header: headers,
	}}

	body, err := downloadDirectoryTreeFile(provider, server.Client(), "pick-code")
	if err != nil {
		t.Fatalf("下载目录树失败: %v", err)
	}
	if provider.requestedUA != driver.UA115Browser {
		t.Fatalf("签名请求 User-Agent = %q，期望 %q", provider.requestedUA, driver.UA115Browser)
	}
	if string(body) != expectedBody {
		t.Fatalf("目录树内容 = %q，期望 %q", string(body), expectedBody)
	}
}

func TestDownloadDirectoryTreeFileReturnsSignatureError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusForbidden)
		_, _ = response.Write([]byte(`{"status":403,"message":"invalid signature"}`))
	}))
	defer server.Close()

	provider := &directoryTreeDownloadURLProviderStub{downloadInfo: &driver.DownloadInfo{
		Url:    driver.FileDownloadUrl{Url: server.URL},
		Header: http.Header{"User-Agent": []string{driver.UA115Browser}},
	}}

	_, err := downloadDirectoryTreeFile(provider, server.Client(), "pick-code")
	if err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("期望保留签名错误详情，实际错误: %v", err)
	}
}
