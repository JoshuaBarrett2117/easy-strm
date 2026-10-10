package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	state.Lock()
	oldOutputs, oldLevel, oldFormat := state.outputs, state.level, state.format
	state.Unlock()
	t.Cleanup(func() {
		SetOutputs(oldOutputs[0], oldOutputs[1], oldOutputs[2], oldOutputs[3])
		SetLevel(oldLevel)
		SetFormat(oldFormat)
	})
	output := &bytes.Buffer{}
	SetOutputs(output, output, output, output)
	SetLevel(DEBUG)
	SetFormat("json")
	return output
}

func decodeRecords(t *testing.T, output *bytes.Buffer) []record {
	t.Helper()
	var records []record
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		var entry record
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal("日志不是有效单行 JSON")
		}
		records = append(records, entry)
	}
	return records
}

func TestServiceLogsReachConfiguredOutputs(t *testing.T) {
	captureLogs(t)
	var debug, info bytes.Buffer
	SetOutputs(&debug, &info, &info, &info)
	Infof("[ShareIdentify] directory=%q title=%q", "剧集甲 (2020)", "剧集甲")
	Warnf("解析重试")
	Errorf("识别失败")
	Debugf("调试详情")
	entries := decodeRecords(t, &info)
	if len(entries) != 3 || entries[0].Level != "INFO" || entries[1].Level != "WARN" || entries[2].Level != "ERROR" {
		t.Fatal("日志级别分流不正确")
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

func TestTemplateTimestampAndRequestTaskContext(t *testing.T) {
	output := captureLogs(t)
	ctx := WithRequestID(context.Background(), "request-synthetic")
	taskContext := WithTaskID(ctx, "task-synthetic")
	WithContext(taskContext, "share_export").Log(INFO, "文件处理完成\n伪造行\r\n\t", Fields{"written": true}, nil)
	if strings.Count(output.String(), "\n") != 1 {
		t.Fatal("换行破坏日志边界")
	}
	entry := decodeRecords(t, output)[0]
	if entry.RequestID != "request-synthetic" || entry.TaskID != "task-synthetic" || entry.Module != "share_export" || entry.Level != "INFO" {
		t.Fatal("上下文模板错误")
	}
	if TaskID(ctx) != "" || RequestID(nil) != "" || TaskID(nil) != "" {
		t.Fatal("上下文不应污染父级或拒绝 nil")
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}(Z|[+-]\d\d:\d\d)$`).MatchString(entry.Timestamp) {
		t.Fatal("时间戳必须精确到三位毫秒并包含时区")
	}
	if _, err := time.Parse(time.RFC3339, entry.Timestamp); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 10, 10, 12, 34, 56, 7_123_456, time.FixedZone("synthetic", 8*60*60))
	if fixed.Format(TimestampLayout) != "2026-10-10T12:34:56.007+08:00" || fixed.UTC().Format(TimestampLayout) != "2026-10-10T04:34:56.007Z" {
		t.Fatal("非 UTC 时区或毫秒截断错误")
	}
	var template map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &template); err != nil {
		t.Fatal(err)
	}
	if len(template) != 10 || template["error_chain"] == nil || template["fields"] == nil {
		t.Fatal("模板字段必须完整，空字段为对象及数组")
	}
	output.Reset()
	WithContext(nil, "startup").Log(INFO, "启动", nil, nil)
	entry = decodeRecords(t, output)[0]
	if entry.RequestID != "-" || entry.TaskID != "-" || entry.TraceID != "-" {
		t.Fatal("后台上下文应使用明确占位符")
	}
}

func TestNestedSensitiveKeysAndErrors(t *testing.T) {
	output := captureLogs(t)
	type credential struct {
		APIKey   string `json:"apiKey"`
		Username string `json:"display"`
		Public   string `json:"public"`
	}
	fields := Fields{"nested": []interface{}{map[string]interface{}{}, &credential{APIKey: "synthetic-private", Username: "synthetic-private", Public: "safe"}}, "bytes": []byte("synthetic-private"), "url": "https://synthetic-private:synthetic-private@example.invalid/private?unknown=synthetic-private#synthetic-private", "raw": json.RawMessage(`{"Api.Key":"synthetic-private"}`)}
	for _, key := range []string{"api_key", "API-KEY", "Api.Key", "X-Api-Key", "apiKey", "Cookie", "Set-Cookie", "account", "AccountName", "user_name", "USERNAME", "access_token", "refreshToken", "password", "hashedPassword", "Authorization", "proxy_authorization", "app_secret", "request_headers", "responseBody", "密码", "账号"} {
		fields["nested"].([]interface{})[0].(map[string]interface{})[key] = "synthetic-private"
	}
	cause := errors.New(`api_key="synthetic-private" username='synthetic-private' /play?unknown=synthetic-private`)
	joined := errors.Join(fmt.Errorf("写入失败: %w", cause), errors.New("Authorization: Bearer synthetic-private\nCookie: sid=synthetic-private; other=synthetic-private"))
	WithContext(nil, "export").Log(ERROR, "处理失败", fields, joined)
	if strings.Contains(output.String(), "synthetic-private") {
		t.Fatal("递归字段或错误链泄露合成凭据")
	}
	entry := decodeRecords(t, output)[0]
	if len(entry.ErrorChain) != 4 || !strings.Contains(entry.ErrorChain[1], "写入失败") || !strings.Contains(entry.ErrorChain[2], redacted) {
		t.Fatal("包装及多分支错误链展开错误")
	}
	if !strings.Contains(output.String(), "safe") {
		t.Fatal("普通诊断字段不应被丢弃")
	}
	if fields["nested"].([]interface{})[0].(map[string]interface{})["api_key"] != "synthetic-private" {
		t.Fatal("脱敏不得修改调用方的数据")
	}
}

func TestConcurrentConfigurationAndLogging(t *testing.T) {
	first := captureLogs(t)
	second := &bytes.Buffer{}
	var workers sync.WaitGroup
	for worker := 0; worker < 12; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for iteration := 0; iteration < 100; iteration++ {
				switch worker % 3 {
				case 0:
					SetOutputs(first, second, first, second)
				case 1:
					SetLevel(Level(iteration % 4))
				default:
					ctx := WithTaskID(WithRequestID(nil, "synthetic-request"), "synthetic-task")
					WithContext(ctx, "concurrent").Log(ERROR, "并发日志", Fields{"worker": worker}, nil)
					Debugf("调试日志")
				}
			}
		}(worker)
	}
	workers.Wait()
	if first.Len()+second.Len() == 0 {
		t.Fatal("没有日志输出")
	}
	for _, output := range []*bytes.Buffer{first, second} {
		if output.Len() > 0 {
			decodeRecords(t, output)
		}
	}
}

func TestExternalWriterAndUnsupportedValuesFailClosed(t *testing.T) {
	output := captureLogs(t)
	content := []byte("arbitrary-unlabelled-synthetic-private\nforged")
	if count, err := Writer("external").Write(content); count != len(content) || err != nil {
		t.Fatal("外部日志适配器写入契约错误")
	}
	cycle := Fields{}
	cycle["self"] = cycle
	WithContext(nil, "test").Log(INFO, "递归对象", Fields{"cycle": cycle, "channel": make(chan string), "bytes": content}, nil)
	if strings.Contains(output.String(), "synthetic-private") {
		t.Fatal("不可信外部日志或原始字节不得输出")
	}
	decodeRecords(t, output)
	SetOutputs(nil, io.Discard, os.Stderr, os.Stderr)
	Debugf("丢弃")
}

func TestTextCredentialVariants(t *testing.T) {
	output := captureLogs(t)
	for _, message := range []string{
		`{"Cookie":"synthetic-private-text"}`,
		`{"AUTHORIZATION":"Bearer synthetic-private-text"}`,
		`Api.Key="synthetic-private-text"`,
		`ACCESS.TOKEN='synthetic-private-text'`,
		`hashedPassword=synthetic-private-text`,
		`PASSWORD_HASH="synthetic-private-text"`,
		`X-Api-Key=synthetic-private-text`,
		`User-Name="synthetic-private-text"`,
		`pq: password authentication failed for user "synthetic-private-text"`,
		"body: synthetic-private-text\nmore arbitrary input",
		"raw: synthetic-private-text",
	} {
		output.Reset()
		WithContext(nil, "test").Log(ERROR, message, nil, errors.New(message))
		if strings.Contains(output.String(), "synthetic-private-text") {
			t.Fatal("带标签的文本凭据变体未清洗")
		}
		decodeRecords(t, output)
	}
}

func TestLegacyTypedNilErrorDoesNotPanic(t *testing.T) {
	output := captureLogs(t)
	var missing *url.Error
	Errorf("缺失错误: %v", missing)
	if chain := decodeRecords(t, output)[0].ErrorChain; len(chain) != 0 {
		t.Fatal("带类型的 nil 错误不应展开")
	}
}
