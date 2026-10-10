package logger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestHumanAndJSONRecursiveRedactionEquivalent(t *testing.T) {
	for _, format := range []string{"", "json"} {
		t.Run(fmt.Sprintf("format-%q", format), func(t *testing.T) {
			output := captureLogs(t)
			SetFormat(format)
			ctx := WithTraceID(WithTaskID(WithRequestID(context.Background(), "redaction-request"), "redaction-task"), "redaction-action")
			cause := errors.New("Cookie: sid=round3-sensitive-cookie\nAuthorization: Bearer round3-sensitive-auth")
			WithContext(ctx, "redaction").Log(ERROR, "失败 https://fixture.invalid/private?api_key=round3-sensitive-url\n单行", Fields{
				"nested": []interface{}{map[string]interface{}{"account": "round3-sensitive-account", "api_key": "round3-sensitive-key", "headers": map[string]string{"X-Api-Key": "round3-sensitive-header"}}},
				"query":  "/play?token=round3-sensitive-query", "safe": "diagnostic",
			}, fmt.Errorf("wrapped: %w", cause))
			line := output.String()
			if strings.Contains(line, "round3-sensitive") || strings.Count(line, "\n") != 1 || !strings.Contains(line, "diagnostic") || !strings.Contains(line, "error_chain") {
				t.Fatalf("redaction/line contract failed: %s", line)
			}
			if format == "" && !strings.Contains(line, "[req=redaction-request][task=redaction-task][module=redaction][trace=redaction-action]") {
				t.Fatal("human identity fields missing")
			}
			if format == "json" {
				decodeRecords(t, output)
			}
			t.Logf("validated captured redacted log: %s", line)
		})
	}
}
