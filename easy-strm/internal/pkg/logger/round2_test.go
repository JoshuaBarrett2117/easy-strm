package logger

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDefaultHumanLifecycleFormat(t *testing.T) {
	var output bytes.Buffer
	SetOutputs(&output, &output, &output, &output)
	t.Cleanup(func() { SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr) })
	WithContext(nil, "lifecycle").Log(INFO, "启动", Fields{"phase": "startup"}, nil)
	WithContext(nil, "lifecycle").Log(INFO, "停机", Fields{"phase": "shutdown"}, nil)
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if !regexp.MustCompile(`^\[\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}(Z|[+-]\d\d:\d\d)\]\[INFO\]\[req=-\]\[task=-\]\[module=lifecycle\]\[trace=-\]`).MatchString(line) {
			t.Fatalf("默认生命周期日志不符合人类模板: %s", line)
		}
	}
}
