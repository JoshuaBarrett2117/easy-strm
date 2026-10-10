package service

import (
	"context"
	"easy-strm/internal/domain"
	"strings"
	"testing"
)

func TestShareFullReconciliationRejectsScheduledTrigger(t *testing.T) {
	scheduler, mock := cronTestService(t)
	scheduler.Register(CronHandler{Key: "share_strm_full_reconciliation", Execute: func(context.Context, *domain.CronTask, string) (string, error) {
		t.Error("定时触发了全量")
		return "", nil
	}})
	expectCronRead(mock, 11, 0, "share_strm_full_reconciliation", "enabled", false)
	if id, err := scheduler.Run(11, "scheduled"); id != "" || err == nil || !strings.Contains(err.Error(), "仅允许手动") {
		t.Fatalf("全量只允许明确手动触发: id=%s err=%v", id, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
