package main

import (
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"testing"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestFindFileByPickCodeRequiresExactMatch(t *testing.T) {
	files := []driver.File{
		{FileID: "file-1", PickCode: "pick-1", Sha1: "sha-1"},
		{FileID: "file-2", PickCode: "pick-2", Sha1: "sha-2"},
	}

	matched := findFileByPickCode(files, "pick-2")
	if matched == nil || matched.FileID != "file-2" || matched.Sha1 != "sha-2" {
		t.Fatalf("pickcode精确匹配结果错误: %+v", matched)
	}
	if missing := findFileByPickCode(files, "file-2"); missing != nil {
		t.Fatalf("file_id不得被当作pickcode匹配: %+v", missing)
	}
}

func TestGetFileInfoAcceptsStringOffsetFrom115Search(t *testing.T) {
	const cookie = "UID=1;CID=2;SEID=3;KID=4"
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("pick_code") != "pick-1" {
			t.Fatalf("搜索请求pick_code错误: %s", request.URL.RawQuery)
		}
		body := `{"state":true,"offset":"0","data":[{"fid":"file-1","cid":"0","n":"demo.mkv","s":"123","sha":"abcdef","pc":"pick-1"}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	cached := driver.New(driver.WithClient(httpClient)).ImportCredential(&driver.Credential{})
	driverCache.Lock()
	driverCache.drivers[99115] = cached115Driver{driver: cached, cookieHash: sha256.Sum256([]byte(cookie))}
	driverCache.Unlock()
	t.Cleanup(func() {
		driverCache.Lock()
		delete(driverCache.drivers, 99115)
		driverCache.Unlock()
	})

	file, err := (&Client{}).GetFileInfo("pick-1", 99115, cookie)
	if err != nil {
		t.Fatalf("字符串offset不应导致解析失败: %v", err)
	}
	if file.FileID != "file-1" || file.PickCode != "pick-1" || file.Size != 123 || file.Sha1 != "ABCDEF" {
		t.Fatalf("文件元数据错误: %+v", file)
	}
}
