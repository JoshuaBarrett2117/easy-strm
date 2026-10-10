package dao

import (
	"context"
	"errors"
	"reflect"
	"regexp"
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

func TestSelectionSyncAllLocksAreBounded(t *testing.T) {
	if len(shareSelectionSyncSQL) == 0 || !strings.Contains(shareSelectionSyncSQL[0], "SET LOCAL lock_timeout='30s'") {
		t.Fatal("除全局advisory锁外，后续行锁及外键锁也必须有30秒等待上限")
	}
}

func TestSelectionSyncDiffPredicateCoversRebindNullsAndVersions(t *testing.T) {
	var update string
	for _, query := range shareSelectionSyncSQL {
		if strings.HasPrefix(query, "UPDATE t_share_export_candidate c SET source_file_id=") {
			update = query
		}
	}
	predicate := regexp.MustCompile(`AND\s*\((c\.[^)]+)\)\s+IS DISTINCT FROM\s*\((o\.[^)]+)\)`).FindStringSubmatch(update)
	if len(predicate) != 3 {
		t.Fatal("候选更新缺少空值安全的差异谓词")
	}
	currentColumns, observedColumns := strings.Split(predicate[1], ","), strings.Split(predicate[2], ",")
	columns := []string{"source_file_id", "file_version", "share_version", "file_size", "available", "state", "episode_sig"}
	if len(currentColumns) != len(columns) || len(observedColumns) != len(columns) {
		t.Fatal("差异谓词必须覆盖全部同步字段")
	}
	for index, column := range columns {
		if strings.TrimSpace(currentColumns[index]) != "c."+column || strings.TrimSpace(observedColumns[index]) != "o."+column {
			t.Fatalf("差异谓词字段对齐错误：%s", column)
		}
	}
	current := map[string]interface{}{"source_file_id": 7, "file_version": 1, "share_version": 2, "file_size": int64(300), "available": true, "state": "active", "episode_sig": "0:1,1:2"}
	distinct := func(observed map[string]interface{}) bool {
		for index := range currentColumns {
			if !reflect.DeepEqual(current[strings.TrimPrefix(strings.TrimSpace(currentColumns[index]), "c.")], observed[strings.TrimPrefix(strings.TrimSpace(observedColumns[index]), "o.")]) {
				return true
			}
		}
		return false
	}
	if distinct(current) {
		t.Fatal("重复扫描不应触发候选更新")
	}
	for _, column := range columns {
		observed := make(map[string]interface{}, len(current))
		for key, value := range current {
			observed[key] = value
		}
		observed[column] = nil
		if !distinct(observed) {
			t.Fatalf("空值变化或来源解绑未被捕获：%s", column)
		}
	}
	for _, sourceID := range []int{8, 7} {
		observed := make(map[string]interface{}, len(current))
		for key, value := range current {
			observed[key] = value
		}
		observed["source_file_id"] = sourceID
		if distinct(observed) != (sourceID != 7) {
			t.Fatal("同一身份重新绑定文件时必须更新，未变绑定必须跳过")
		}
	}
}
