package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"github.com/gin-gonic/gin"
)

type adoptionActionsStub struct {
	strmActionsStub
	request domain.ShareBaselineAdoptionRequest
	calls   int
}

func (stub *adoptionActionsStub) StartBaselineAdoptionContext(_ context.Context, request domain.ShareBaselineAdoptionRequest) (string, error) {
	stub.calls++
	stub.request = request
	return "adopt-task", stub.err
}

func TestShareBaselineAdoptionController(test *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := `{"manual_confirmation":"人工确认采纳","reason":"独立核验","dba_attestation":"DBA-42","expected_output_path":"/isolated/output","expected":{"config_revision":3,"prepared_revision":3,"completed_revision":0,"selection_count":2,"output_count":2,"pending_count":1,"dirty_revision_sum":2,"selection_revision_sum":2}}`
	benign := strings.Replace(valid, `"reason":`, `"allow_known_benign_invalid":68,"benign_attestation":"  核验原文\nMediaID-only  ","reason":`, 1)
	for _, scenario := range []struct {
		name, body           string
		actor, status, calls int
		failure              bool
	}{
		{"success", valid, 42, 200, 1, false},
		{"benign", benign, 42, 200, 1, false},
		{"benign-unauthenticated", benign, 0, 401, 0, false},
		{"fractional-allowance", strings.Replace(benign, `"allow_known_benign_invalid":68`, `"allow_known_benign_invalid":68.5`, 1), 42, 400, 0, false},
		{"string-allowance", strings.Replace(benign, `"allow_known_benign_invalid":68`, `"allow_known_benign_invalid":"68"`, 1), 42, 400, 0, false},
		{"overflow-allowance", strings.Replace(benign, `"allow_known_benign_invalid":68`, `"allow_known_benign_invalid":9223372036854775808`, 1), 42, 400, 0, false},
		{"bad-benign-attestation", strings.Replace(benign, `"benign_attestation":"  核验原文\nMediaID-only  "`, `"benign_attestation":68`, 1), 42, 400, 0, false},
		{"service-failed", valid, 42, 400, 1, true},
		{"unauthenticated", valid, 0, 401, 0, false},
		{"empty", `{}`, 42, 400, 0, false},
		{"bad-json", `{`, 42, 400, 0, false},
		{"missing-confirmation", strings.Replace(valid, "人工确认采纳", "", 1), 42, 400, 0, false},
		{"fractional-count", strings.Replace(valid, `"pending_count":1`, `"pending_count":1.5`, 1), 42, 400, 0, false},
		{"overflow-count", strings.Replace(valid, `"pending_count":1`, `"pending_count":9223372036854775808`, 1), 42, 400, 0, false},
		{"missing-guard", strings.Replace(valid, `"completed_revision":0,`, "", 1), 42, 400, 0, false},
		{"unknown-field", strings.Replace(valid, `"reason":`, `"auto":true,"reason":`, 1), 42, 400, 0, false},
		{"spoof-actor", strings.Replace(valid, `"reason":`, `"ActorUserID":5,"reason":`, 1), 42, 400, 0, false},
		{"extra-json", valid + ` {}`, 42, 400, 0, false},
		{"oversize", strings.Repeat(" ", 17*1024) + valid, 42, 400, 0, false},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			stub := &adoptionActionsStub{}
			if scenario.failure {
				stub.err = errors.New("任务存储失败")
			}
			controller := &ShareStrmController{s: stub}
			router := gin.New()
			router.Use(func(ctx *gin.Context) { ctx.Set("userID", scenario.actor) })
			router.POST("/adopt", controller.AdoptBaseline)
			response := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/adopt", strings.NewReader(scenario.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != scenario.status || stub.calls != scenario.calls {
				test.Fatalf("status=%d calls=%d body=%s", response.Code, stub.calls, response.Body.String())
			}
			if scenario.status == 200 {
				var envelope Response
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					test.Fatal(err)
				}
				data := envelope.Data.(map[string]interface{})
				if !envelope.State || data["task_id"] != "adopt-task" || data["manual_confirmation"] != "人工确认采纳" || stub.request.ActorUserID != 42 {
					test.Fatalf("unexpected envelope %+v", envelope)
				}
				if scenario.name == "benign" && (stub.request.AllowKnownBenignInvalid != 68 || stub.request.BenignAttestation != "  核验原文\nMediaID-only  ") {
					test.Fatalf("allowance binding altered: %+v", stub.request)
				}
				if scenario.name == "success" && (stub.request.AllowKnownBenignInvalid != 0 || stub.request.BenignAttestation != "") {
					test.Fatalf("allowance defaults altered: %+v", stub.request)
				}
			}
		})
	}
}
