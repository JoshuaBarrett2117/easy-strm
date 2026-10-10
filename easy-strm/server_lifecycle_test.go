package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	pkglogger "easy-strm/internal/pkg/logger"
)

func TestActualServerLifecycleLogsUseAbsentRequestMarker(t *testing.T) {
	var output bytes.Buffer
	pkglogger.SetFormat("")
	pkglogger.SetOutputs(&output, &output, &output, &output)
	t.Cleanup(func() { pkglogger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr) })
	logServerLifecycle("startup", nil)
	logServerLifecycle("shutdown", nil)
	logServerLifecycle("startup_error", errors.New("api_key=lifecycle-secret"))
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatal("生命周期缺少日志")
	}
	for _, line := range lines {
		if !strings.Contains(line, "[req=-][task=-][module=lifecycle][trace=-]") {
			t.Fatalf("无请求字段错误: %s", line)
		}
	}
	if strings.Contains(output.String(), "lifecycle-secret") {
		t.Fatal("生命周期错误泄漏")
	}
	t.Logf("validated captured lifecycle logs:\n%s", output.String())
}
