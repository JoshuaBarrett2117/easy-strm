package service

import (
	"context"
	"easy-strm/internal/domain"
	"strings"
	"testing"
)

func TestShareBaselineAdoptionInvalidInputs(test *testing.T) {
	valid := domain.ShareBaselineAdoptionRequest{ManualConfirmation: "人工确认采纳", Reason: "独立核验", DBAAttestation: "DBA-42", ExpectedOutputPath: "/isolated/output", ActorUserID: 1, Expected: map[string]int64{"config_revision": 3, "prepared_revision": 3, "completed_revision": 0, "selection_count": 2, "output_count": 2, "pending_count": 1, "dirty_revision_sum": 2, "selection_revision_sum": 2}}
	for _, field := range []string{"confirmation", "reason", "attestation", "missing", "negative", "unknown", "root", "relative", "unclean", "actor", "long-reason", "long-attestation", "bad-counts", "bad-revisions", "zero-selection"} {
		test.Run(field, func(test *testing.T) {
			request := valid
			request.Expected = map[string]int64{}
			for key, value := range valid.Expected {
				request.Expected[key] = value
			}
			switch field {
			case "confirmation":
				request.ManualConfirmation = ""
			case "reason":
				request.Reason = " "
			case "attestation":
				request.DBAAttestation = " "
			case "missing":
				delete(request.Expected, "completed_revision")
			case "negative":
				request.Expected["pending_count"] = -1
			case "unknown":
				request.Expected["other"] = 1
			case "root":
				request.ExpectedOutputPath = "/"
			case "relative":
				request.ExpectedOutputPath = "relative"
			case "unclean":
				request.ExpectedOutputPath = "/isolated/../output"
			case "actor":
				request.ActorUserID = 0
			case "long-reason":
				request.Reason = strings.Repeat("a", 2001)
			case "long-attestation":
				request.DBAAttestation = strings.Repeat("a", 501)
			case "bad-counts":
				request.Expected["output_count"]++
			case "bad-revisions":
				request.Expected["prepared_revision"]++
			case "zero-selection":
				request.Expected["selection_count"] = 0
				request.Expected["output_count"] = 0
			}
			if err := ValidateShareBaselineAdoption(request); err == nil {
				test.Fatal("invalid request accepted")
			}
			if _, err := (&ShareStrmService{}).StartBaselineAdoptionContext(context.Background(), request); err == nil {
				test.Fatal("invalid request started task")
			}
		})
	}
	if err := ValidateShareBaselineAdoption(valid); err != nil {
		test.Fatal(err)
	}
}
