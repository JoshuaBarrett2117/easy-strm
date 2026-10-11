package dao

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareWorkReexportClosureBoundedRejectionList(test *testing.T) {
	for _, reason := range []string{"outside_work_scope", "missing_selection", "empty_work_key", "unprocessed_export_key", "C2_output_path", "C4_stable_relative_path", "C5_history_owner", "C6_local_strm_path", "reference_limit"} {
		test.Run(reason, func(test *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			connection, err := database.Conn(context.Background())
			if err != nil {
				test.Fatal(err)
			}
			defer connection.Close()
			store := NewShareExportCheckpointDAO(database).ForOutputConnection(connection)
			total, rejected := int64(3), int64(1)
			if reason == "reference_limit" {
				total, rejected = domain.ShareWorkReexportMaxReferences+1, 0
			}
			rows := sqlmock.NewRows([]string{"total", "rejected", "work", "item", "reason"}).AddRow(total, rejected, "tmdb:movie:2", "movie:2", reason)
			mock.ExpectQuery("WITH targets AS").WillReturnRows(rows)
			err = store.WorkReexportPreflight(context.Background(), "/fixture", []string{"tmdb:movie:1"}, []domain.ShareStrmEntry{{ID: "expected", MediaID: 1}}, []domain.ShareSelection{{ItemKey: "movie:1", RelativePath: "one.strm"}})
			if !errors.Is(err, ErrShareExportChanged) {
				test.Fatal(err)
			}
			for _, field := range []string{"work_key=\"tmdb:movie:2\"", "export_key=\"movie:2\"", reason, fmt.Sprintf("references_total=%d", total), fmt.Sprintf("rejected_total=%d", rejected), "samples=1", "truncated="} {
				if !strings.Contains(err.Error(), field) {
					test.Fatalf("缺少拒绝清单字段 %s: %v", field, err)
				}
			}
			if reason == "reference_limit" && !strings.Contains(err.Error(), "truncated=true") {
				test.Fatal("超限即使全在范围也必须返回样本与截断")
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestShareWorkReexportClosureSQLPreservesHardConflictsAndHistory(test *testing.T) {
	for _, required := range []string{
		"old.export_key=target.item_key AND old.state='active'", "output.share_strm_id=target.old_mapping_id", "output.share_strm_id=target.url_mapping_id",
		"LEFT JOIN t_share_media_selection", "selection.work_key IS NULL", "btrim(selection.work_key)=''", "NOT(selection.work_key=ANY($6::text[]))",
		"output.export_key<>target.item_key", "selection.media_item_key<>target.item_key", "history.owner_key<>'share:default'", "file.strm_config_id<>-1",
		"NOT EXISTS(SELECT 1 FROM t_strm_export_state output WHERE output.output_path=file.local_strm_path)",
		"SELECT DISTINCT", "GROUP BY kind,owner_key,export_key,work_key", "LIMIT 100", "COUNT(kind) AS total", "COUNT(reason) AS rejected", "unprocessed_export_key",
	} {
		if !strings.Contains(shareWorkReexportPreflightSQL, required) {
			test.Fatalf("丢失闭包护栏: %s", required)
		}
	}
	for _, removed := range []string{"COUNT(DISTINCT media_id)>1", "file.media_id IS DISTINCT FROM target.media_id"} {
		if strings.Contains(shareWorkReexportPreflightSQL, removed) {
			test.Fatalf("C1/C3 不得拒绝完整请求闭包: %s", removed)
		}
	}
	if strings.Contains(workReexportInsertSQL, "DO UPDATE") || strings.Contains(workReexportInsertSQL, "DELETE") {
		test.Fatal("历史映射不可变，重导只插入不存在的 item mapping")
	}
}
