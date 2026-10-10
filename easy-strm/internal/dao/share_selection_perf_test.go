package dao

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSelectionSyncDiffAndIdentityGuards(t *testing.T) {
	queries := strings.Join(shareSelectionSyncSQL, "\n")
	for _, required := range []string{
		"selection_observed(share_id,remote_file_id,name_hash)",
		"(c.source_file_id,c.file_version,c.share_version,c.file_size,c.available,c.state,c.episode_sig) IS DISTINCT FROM (o.source_file_id,o.file_version,o.share_version,o.file_size,o.available,o.state,o.episode_sig)",
		"WHERE t_share_export_candidate_item.revoked",
	} {
		if !strings.Contains(queries, required) {
			t.Errorf("缺少身份定位或差异写入保护：%s", required)
		}
	}
	update := ""
	for _, query := range shareSelectionSyncSQL {
		if strings.HasPrefix(query, "UPDATE t_share_export_candidate c SET source_file_id=") {
			update = query
		}
	}
	if !strings.Contains(update, "IS DISTINCT FROM") {
		t.Fatal("未变化候选不应重写 updated_at")
	}
	if strings.Contains(update, "selection_revision") || strings.Contains(update, "first_seen_seq=") {
		t.Fatal("同步不得重写首次发现顺序或选择版本")
	}
}

func TestSelectionSyncCancelledDoesNotBegin(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = NewShareRecordDAO(database).SyncSelections(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误未保留：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
