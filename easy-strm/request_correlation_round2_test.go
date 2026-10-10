package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	pkglogger "easy-strm/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestHTTPConcurrentRequestIDsAndThreeGoroutines(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	pkglogger.SetFormat("json")
	pkglogger.SetOutputs(&output, &output, &output, &output)
	t.Cleanup(func() { pkglogger.SetFormat(""); pkglogger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr) })
	router := gin.New()
	router.Use(requestLoggingMiddleware())
	router.GET("/workers", func(request *gin.Context) {
		ctx := request.Request.Context()
		var workers sync.WaitGroup
		for worker := 0; worker < 3; worker++ {
			workers.Add(1)
			go func(worker int) {
				defer workers.Done()
				pkglogger.WithContext(ctx, "http_worker").Log(pkglogger.INFO, "子调用完成", pkglogger.Fields{"worker": worker, "nested": map[string]interface{}{"api_key": "round2-secret-key", "headers": map[string]string{"Cookie": "round2-secret-cookie"}}, "url": "https://fixture.invalid/play?token=round2-secret-token"}, fmt.Errorf("Authorization: Bearer round2-secret-auth"))
			}(worker)
		}
		workers.Wait()
		request.Status(204)
	})
	server := httptest.NewServer(router)
	defer server.Close()
	const count = 240
	ids := make(chan string, count)
	var requests sync.WaitGroup
	for number := 0; number < count; number++ {
		requests.Add(1)
		go func() {
			defer requests.Done()
			request, _ := http.NewRequest("GET", server.URL+"/workers", nil)
			request.Header.Set("X-Trace-ID", "one-browser-action")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Error(err)
				return
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != 204 {
				t.Errorf("status=%d", response.StatusCode)
			}
			ids <- response.Header.Get("X-Request-ID")
		}()
	}
	requests.Wait()
	close(ids)
	unique := map[string]bool{}
	for id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.Version() != 7 || unique[id] {
			t.Fatalf("非唯一 UUIDv7: %s", id)
		}
		unique[id] = true
	}
	if len(unique) != count {
		t.Fatalf("requests=%d", len(unique))
	}
	workers := map[string]int{}
	sequences := map[string]map[uint64]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var entry struct {
			RequestID string `json:"request_id"`
			TraceID   string `json:"trace_id"`
			Module    string `json:"module"`
			Seq       uint64 `json:"seq"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if !unique[entry.RequestID] || entry.TraceID != "one-browser-action" {
			t.Fatalf("上下文丢失: %s", line)
		}
		if sequences[entry.RequestID] == nil {
			sequences[entry.RequestID] = map[uint64]bool{}
		}
		if entry.Seq == 0 || sequences[entry.RequestID][entry.Seq] {
			t.Fatalf("请求内 seq 重复: %s", line)
		}
		sequences[entry.RequestID][entry.Seq] = true
		if entry.Module == "http_worker" {
			workers[entry.RequestID]++
		}
	}
	for id := range unique {
		if workers[id] != 3 || len(sequences[id]) != 4 {
			t.Fatalf("请求 %s 未贯穿三个 goroutine", id)
		}
	}
	if strings.Contains(output.String(), "round2-secret") {
		t.Fatal("递归脱敏有泄漏")
	}
	t.Logf("%d concurrent HTTP requests, 3 captured worker logs each, unique UUIDv7 and seq, recursive secrets zero hits", count)
}

func TestHTTPUpstreamRequestAndTraceparentReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestLoggingMiddleware())
	router.GET("/ids", func(request *gin.Context) { request.String(200, pkglogger.RequestID(request.Request.Context())) })
	for _, scenario := range []struct{ header, value, expected string }{
		{"X-Request-ID", "upstream-request-123", "upstream-request-123"},
		{"traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01", "0123456789abcdef0123456789abcdef"},
	} {
		request := httptest.NewRequest("GET", "/ids", nil)
		request.Header.Set(scenario.header, scenario.value)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Body.String() != scenario.expected || response.Header().Get("X-Request-ID") != scenario.expected {
			t.Fatalf("未沿用 %s", scenario.header)
		}
	}
}

func TestRequestIDMultiProcessUniqueness(t *testing.T) {
	if os.Getenv("ESTRM_UUID_CHILD") == "1" {
		for number := 0; number < 300; number++ {
			ctx, err := pkglogger.HTTPContext(httptest.NewRequest("GET", "/fixture", nil))
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println("ID=" + pkglogger.RequestID(ctx))
		}
		return
	}
	var workers sync.WaitGroup
	results := make(chan []byte, 4)
	for number := 0; number < 4; number++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			command := exec.Command(os.Args[0], "-test.run=^TestRequestIDMultiProcessUniqueness$")
			command.Env = append(os.Environ(), "ESTRM_UUID_CHILD=1")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Errorf("子进程失败: %v %s", err, output)
				return
			}
			results <- output
		}()
	}
	workers.Wait()
	close(results)
	unique := map[string]bool{}
	for output := range results {
		for _, line := range strings.Split(string(output), "\n") {
			if !strings.HasPrefix(line, "ID=") {
				continue
			}
			id := strings.TrimPrefix(line, "ID=")
			parsed, err := uuid.Parse(id)
			if err != nil || parsed.Version() != 7 || unique[id] {
				t.Fatalf("跨进程重复或无效标识: %s", id)
			}
			unique[id] = true
		}
	}
	if len(unique) != 1200 {
		t.Fatalf("子进程标识数=%d", len(unique))
	}
	t.Log("4 concurrent subprocesses, 1200 distinct UUIDv7; empirical check, not a mathematical uniqueness proof")
}
