package dao

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

const shareTransferLogSelectSQL = `SELECT id, task_id, share_code, share_folder_name, file_name,
 file_pick_code, file_size, file_sha1, cloud115_id,
 target_directory, status, error_message, is_second_transfer,
 COALESCE(create_time::text, ''), COALESCE(update_time::text, '')
 FROM t_share_transfer_log WHERE task_id = $1 ORDER BY id ASC`

const shareTransferLogUpdateSQL = `UPDATE t_share_transfer_log
 SET status = $1, error_message = $2, update_time = CURRENT_TIMESTAMP
 WHERE task_id = $3 AND file_pick_code = $4`

func shareTransferLogInsertExpectation(logs []domain.ShareTransferLog) (string, []driver.Value) {
	values := make([]string, 0, len(logs))
	arguments := make([]driver.Value, 0, len(logs)*12)
	for index, entry := range logs {
		placeholders := make([]string, 12)
		for column := range placeholders {
			placeholders[column] = fmt.Sprintf("$%d", index*12+column+1)
		}
		values = append(values, "("+strings.Join(placeholders, ",")+")")
		arguments = append(arguments, entry.TaskId, entry.ShareCode, entry.ShareFolderName,
			entry.FileName, entry.FilePickCode, entry.FileSize, entry.FileSha1, entry.Cloud115Id,
			entry.TargetDirectory, entry.Status, entry.ErrorMessage, entry.IsSecondTransfer)
	}
	return `INSERT INTO t_share_transfer_log
 (task_id, share_code, share_folder_name, file_name, file_pick_code,
 file_size, file_sha1, cloud115_id, target_directory, status, error_message,
 is_second_transfer) VALUES ` + strings.Join(values, ","), arguments
}

func testShareTransferLog(index int) domain.ShareTransferLog {
	return domain.ShareTransferLog{
		ID: index + 1, TaskId: "transfer-test", ShareCode: "share-test", ShareFolderName: "测试分享",
		FileName: fmt.Sprintf("电影%d.mkv", index), FilePickCode: fmt.Sprintf("fid-%d", index),
		FileSize: int64(index + 100), FileSha1: "test-sha1", Cloud115Id: 7,
		TargetDirectory: "/测试目标", Status: "failed", ErrorMessage: "原错误", IsSecondTransfer: true,
		CreateTime: "2026-10-09 00:00:00", UpdateTime: "2026-10-09 00:01:00",
	}
}

// TestShareTransferLogBatchInsertDefaultTimestamps 精确限定十二列参数；sqlmock 不验证真实数据库生成时间。
func TestShareTransferLogBatchInsertDefaultTimestamps(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		count     int
		failBatch int
	}{
		{"empty", 0, 0}, {"one", 1, 0}, {"multiple", 2, 0}, {"201 records", 201, 0},
		{"first batch error", 201, 1}, {"second batch error", 201, 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			logs := make([]domain.ShareTransferLog, scenario.count)
			for index := range logs {
				logs[index] = testShareTransferLog(index)
			}
			for start, batch := 0, 1; start < len(logs); start, batch = start+200, batch+1 {
				end := min(start+200, len(logs))
				query, arguments := shareTransferLogInsertExpectation(logs[start:end])
				expected := mock.ExpectExec(query).WithArgs(arguments...)
				if batch == scenario.failBatch {
					expected.WillReturnError(errors.New("insert refused"))
					break
				}
				expected.WillReturnResult(sqlmock.NewResult(0, int64(end-start)))
			}
			err = NewShareTransferLogDAO(database).BatchInsert(context.Background(), logs)
			if scenario.failBatch == 0 && err != nil {
				t.Fatal(err)
			}
			if scenario.failBatch != 0 && (err == nil || !strings.Contains(err.Error(), fmt.Sprintf("第%d批", scenario.failBatch)) || !strings.Contains(err.Error(), "insert refused")) {
				t.Fatalf("批次错误未上报: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func shareTransferLogRows(logs ...domain.ShareTransferLog) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "task_id", "share_code", "share_folder_name", "file_name",
		"file_pick_code", "file_size", "file_sha1", "cloud115_id", "target_directory", "status",
		"error_message", "is_second_transfer", "create_time", "update_time"})
	for _, entry := range logs {
		rows.AddRow(entry.ID, entry.TaskId, entry.ShareCode, entry.ShareFolderName, entry.FileName,
			entry.FilePickCode, entry.FileSize, entry.FileSha1, entry.Cloud115Id, entry.TargetDirectory,
			entry.Status, entry.ErrorMessage, entry.IsSecondTransfer, entry.CreateTime, entry.UpdateTime)
	}
	return rows
}

// TestShareTransferLogGetByTaskId 验证重试读取 SQL、完整字段映射、结果顺序和所有错误路径的结果集关闭。
func TestShareTransferLogGetByTaskId(t *testing.T) {
	for _, scenario := range []string{"success", "empty", "query error", "scan error", "row error"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			want := []domain.ShareTransferLog{testShareTransferLog(0), testShareTransferLog(1)}
			rows := shareTransferLogRows(want...)
			expected := mock.ExpectQuery(shareTransferLogSelectSQL).WithArgs("transfer-test")
			var errorText string
			switch scenario {
			case "empty":
				rows = shareTransferLogRows()
			case "query error":
				errorText = "查询失败"
				expected.WillReturnError(errors.New("query refused"))
			case "scan error":
				errorText = "扫描行失败"
				rows = shareTransferLogRows()
				rows.AddRow("invalid-id", "transfer-test", "", "", "", "", 0, "", 0, "", "pending", "", false, "", "")
			case "row error":
				errorText = "遍历结果集失败"
				rows.RowError(1, errors.New("row refused"))
			}
			if scenario != "query error" {
				expected.WillReturnRows(rows).RowsWillBeClosed()
			}
			got, err := NewShareTransferLogDAO(database).GetByTaskId(context.Background(), "transfer-test")
			if errorText != "" {
				if err == nil || !strings.Contains(err.Error(), errorText) || got != nil {
					t.Fatalf("期望 %s 且不返回部分日志: %v", errorText, err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if scenario == "empty" {
				if len(got) != 0 {
					t.Fatal("空查询不应返回日志")
				}
			} else if !reflect.DeepEqual(got, want) {
				t.Fatalf("日志字段或顺序不符: %+v", got)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestShareTransferLogUpdateStatus 验证按 Fid 更新、数据库时间表达式、空错误清除和执行错误返回。
func TestShareTransferLogUpdateStatus(t *testing.T) {
	for _, scenario := range []struct {
		name, status, message string
		fail                  bool
	}{
		{"failure message", "failed", "测试失败", false},
		{"success clears error", "success", "", false},
		{"update error", "success", "", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			expected := mock.ExpectExec(shareTransferLogUpdateSQL).
				WithArgs(scenario.status, scenario.message, "transfer-test", "fid-1")
			if scenario.fail {
				expected.WillReturnError(errors.New("update refused"))
			} else {
				expected.WillReturnResult(sqlmock.NewResult(0, 1))
			}
			err = NewShareTransferLogDAO(database).UpdateStatus(context.Background(), "transfer-test", "fid-1", scenario.status, scenario.message)
			if (err != nil) != scenario.fail || (scenario.fail && !strings.Contains(err.Error(), "update refused")) {
				t.Fatalf("更新错误未正确上报: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
