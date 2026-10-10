package logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Level 表示最低输出级别，保留原有枚举数值。
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// Fields 表示业务诊断字段；禁止传入原始请求、响应或无标签凭据。
type Fields map[string]interface{}

// TimestampLayout 固定保留三位毫秒和时区；UTC 使用 Z。
const TimestampLayout = "2006-01-02T15:04:05.000Z07:00"

type record struct {
	Timestamp  string      `json:"timestamp"`
	Level      string      `json:"level"`
	RequestID  string      `json:"request_id"`
	TaskID     string      `json:"task_id"`
	TraceID    string      `json:"trace_id"`
	Sequence   uint64      `json:"seq,omitempty"`
	Module     string      `json:"module"`
	Message    string      `json:"message"`
	Fields     interface{} `json:"fields"`
	ErrorChain []string    `json:"error_chain"`
}

var state = struct {
	sync.Mutex
	level   Level
	format  string
	outputs [4]io.Writer
}{level: INFO, outputs: [4]io.Writer{os.Stdout, os.Stdout, os.Stderr, os.Stderr}}

// SetOutputs 原子切换四级输出；nil 表示丢弃。所有输出共用写锁，调用方负责文件生命周期。
func SetOutputs(debugOutput, infoOutput, warnOutput, errorOutput io.Writer) {
	state.Lock()
	defer state.Unlock()
	state.outputs = [4]io.Writer{debugOutput, infoOutput, warnOutput, errorOutput}
}

// SetLevel 并发安全地设置最低级别，非法值回退 INFO。
func SetLevel(level Level) {
	if level < DEBUG || level > ERROR {
		level = INFO
	}
	state.Lock()
	defer state.Unlock()
	state.level = level
}

// SetFormat 显式选择 json；其余值均使用默认人类可读单行格式。
func SetFormat(format string) {
	state.Lock()
	defer state.Unlock()
	state.format = strings.ToLower(strings.TrimSpace(format))
}

// Logger 保存不可变的调用上下文；不同请求不得共享可变 Fields。
type Logger struct {
	ctx    context.Context
	module string
}

// WithContext 创建带请求及任务关联的日志入口；nil 上下文视为后台调用。
func WithContext(ctx context.Context, module string) *Logger {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Logger{ctx: ctx, module: module}
}

// Log 按统一 JSON 模板输出一行，递归脱敏字段并展开包装及多分支错误链。
func (entry *Logger) Log(level Level, message string, fields Fields, err error) {
	entry.write(level, message, fields, []error{err})
}

func (entry *Logger) write(level Level, message string, fields Fields, errs []error) {
	if level < DEBUG || level > ERROR {
		return
	}
	chain := []string{}
	for _, err := range errs {
		appendErrorChain(&chain, err, 0)
	}
	if fields == nil {
		fields = Fields{}
	}
	entryRecord := record{
		Timestamp: time.Now().Format(TimestampLayout), Level: [...]string{"DEBUG", "INFO", "WARN", "ERROR"}[level],
		RequestID: contextLabel(RequestID(entry.ctx)), TaskID: contextLabel(TaskID(entry.ctx)),
		TraceID: contextLabel(TraceID(entry.ctx)), Sequence: nextSequence(entry.ctx),
		Module: sanitizeText(entry.module), Message: sanitizeText(message), Fields: sanitizeValue(fields), ErrorChain: chain,
	}
	state.Lock()
	defer state.Unlock()
	if level < state.level || state.outputs[level] == nil {
		return
	}
	var line []byte
	if state.format == "json" {
		line, _ = json.Marshal(entryRecord)
	} else {
		line = humanRecord(entryRecord)
	}
	line = append(line, '\n')
	if _, err := state.outputs[level].Write(line); err != nil {
		_, _ = io.WriteString(os.Stderr, "["+time.Now().Format(TimestampLayout)+"][ERROR][req=-][task=-][module=logger][trace=-] 日志输出失败，原始错误已省略\n")
	}
}

func contextLabel(value string) string {
	if value == "" {
		return "-"
	}
	return sanitizeText(value)
}

func humanRecord(entry record) []byte {
	var output strings.Builder
	fmt.Fprintf(&output, "[%s][%s][req=%s][task=%s][module=%s][trace=%s] %s", entry.Timestamp, entry.Level, singleLine(entry.RequestID), singleLine(entry.TaskID), singleLine(entry.Module), singleLine(entry.TraceID), singleLine(entry.Message))
	if entry.Sequence > 0 {
		fmt.Fprintf(&output, " seq=%d", entry.Sequence)
	}
	fields, _ := entry.Fields.(map[string]interface{})
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, _ := json.Marshal(fields[key])
		fmt.Fprintf(&output, " %s=%s", singleLine(key), value)
	}
	if len(entry.ErrorChain) > 0 {
		value, _ := json.Marshal(entry.ErrorChain)
		fmt.Fprintf(&output, " error_chain=%s", value)
	}
	return []byte(output.String())
}

func singleLine(value string) string {
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}

func appendErrorChain(chain *[]string, err error, depth int) {
	if err == nil || len(*chain) >= 32 || depth >= 16 {
		return
	}
	value := reflect.ValueOf(err)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return
	}
	*chain = append(*chain, sanitizeText(err.Error()))
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			appendErrorChain(chain, child, depth+1)
		}
	} else {
		appendErrorChain(chain, errors.Unwrap(err), depth+1)
	}
}

func legacy(level Level, format string, values ...interface{}) {
	module := "legacy"
	for depth := 2; depth < 8; depth++ {
		programCounter, file, _, ok := runtime.Caller(depth)
		if !ok {
			break
		}
		function := runtime.FuncForPC(programCounter)
		if function != nil && !strings.Contains(function.Name(), "internal/pkg/logger.") && filepath.Base(file) != "log.go" {
			module = strings.TrimSuffix(filepath.Base(file), ".go")
			break
		}
	}
	errs := []error{}
	for _, value := range values {
		if err, ok := value.(error); ok {
			errs = append(errs, err)
		}
	}
	WithContext(nil, module).write(level, fmt.Sprintf(format, values...), nil, errs)
}

// Debug 兼容旧格式化调用，输出 DEBUG JSON。
func Debug(format string, values ...interface{}) { legacy(DEBUG, format, values...) }

// Debugf 是 Debug 的兼容别名。
func Debugf(format string, values ...interface{}) { legacy(DEBUG, format, values...) }

// Info 兼容旧格式化调用，输出 INFO JSON。
func Info(format string, values ...interface{}) { legacy(INFO, format, values...) }

// Infof 是 Info 的兼容别名。
func Infof(format string, values ...interface{}) { legacy(INFO, format, values...) }

// Warn 兼容旧格式化调用，输出 WARN JSON。
func Warn(format string, values ...interface{}) { legacy(WARN, format, values...) }

// Warnf 是 Warn 的兼容别名。
func Warnf(format string, values ...interface{}) { legacy(WARN, format, values...) }

// Error 兼容旧格式化调用，并从 error 参数提取错误链。
func Error(format string, values ...interface{}) { legacy(ERROR, format, values...) }

// Errorf 是 Error 的兼容别名。
func Errorf(format string, values ...interface{}) { legacy(ERROR, format, values...) }

type externalWriter struct{ module string }

// Writer 接管标准库及框架日志旁路，只记录字节数而不信任外部原始文本。
func Writer(module string) io.Writer { return externalWriter{module: module} }

func (writer externalWriter) Write(content []byte) (int, error) {
	WithContext(nil, writer.module).Log(WARN, "外部日志文本已省略", Fields{"bytes": len(content)}, nil)
	return len(content), nil
}
