package service

import (
	"context"
	"easy-strm/internal/dao"
	"testing"
)

func TestShareBaselineAdoptionResumeRequiresNewConfirmation(test *testing.T) {
	client := setupTaskRedisMock(test)
	defer client.Close()
	tasks := NewTaskService(dao.NewTaskRedisDAO(client))
	if err := tasks.Create("adopt", "share_baseline_adopt", "人工确认采纳"); err != nil {
		test.Fatal(err)
	}
	if err := tasks.SetError("adopt", "核验已失效"); err != nil {
		test.Fatal(err)
	}
	if err := tasks.ResumeContext(context.Background(), "adopt"); err == nil {
		test.Fatal("manual adoption must not resume without fresh confirmation/guards")
	}
}
