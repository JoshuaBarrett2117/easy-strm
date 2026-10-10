package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestLegacyLogsAreSingleLineJSONAndRedactCredentials(t *testing.T) {
	output := captureLogs(t)
	Errorf("request failed\nforged line: %v", errors.New("api_key=synthetic-key cookie: synthetic-cookie\nhttps://example.invalid/play?token=synthetic-token"))
	if strings.Count(output.String(), "\n") != 1 {
		t.Fatal("日志必须只有一个物理行")
	}
	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatal("日志不是 JSON")
	}
	for _, secret := range []string{"synthetic-key", "synthetic-cookie", "synthetic-token"} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("合成敏感值未脱敏")
		}
	}
}
