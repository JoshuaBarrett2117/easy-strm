package logger

import (
	"bytes"
	"strings"
	"testing"
)

// TestServiceLogsReachConfiguredOutputs 验证目录与识别结果写入页面读取的 INFO 输出，DEBUG 独立分流。
func TestServiceLogsReachConfiguredOutputs(t *testing.T) {
	oldDebug, oldInfo, oldWarn, oldError, oldLevel := debugLogger, infoLogger, warnLogger, errorLogger, currentLevel
	t.Cleanup(func() {
		debugLogger, infoLogger, warnLogger, errorLogger, currentLevel = oldDebug, oldInfo, oldWarn, oldError, oldLevel
	})
	var debug, info bytes.Buffer
	SetOutputs(&debug, &info, &info, &info)
	SetLevel(DEBUG)
	Infof("[ShareIdentify] directory=%q title=%q", "剧集甲 (2020)", "剧集甲")
	Warnf("解析重试")
	Errorf("识别失败")
	Debugf("调试详情")
	for _, want := range []string{"[INFO]", "剧集甲 (2020)", "title=", "解析重试", "识别失败"} {
		if !strings.Contains(info.String(), want) {
			t.Fatalf("INFO输出缺少 %q: %s", want, info.String())
		}
	}
	if strings.Contains(info.String(), "调试详情") || !strings.Contains(debug.String(), "调试详情") {
		t.Fatal("DEBUG 日志未正确分流")
	}
	SetLevel(ERROR)
	info.Reset()
	Infof("不应输出")
	if info.Len() != 0 {
		t.Fatal("未遵守配置的日志级别")
	}
}
