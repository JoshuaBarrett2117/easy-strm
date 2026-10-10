package dao

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

type embyMonitorBatchArgument struct{ size int }

func (argument embyMonitorBatchArgument) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	var items []domain.EmbyMonitorItem
	if json.Unmarshal([]byte(raw), &items) != nil || len(items) != argument.size {
		return false
	}
	for _, item := range items {
		if item.ItemID == "0" && item.Name != "latest" {
			return false
		}
	}
	return true
}

func TestEmbyMonitorItemsChunkedAtomicAndLastDuplicateWins(t *testing.T) {
	for _, failure := range []int{-1, 0, 1} {
		database, mock, _ := sqlmock.New()
		now := time.Now()
		items := make([]domain.EmbyMonitorItem, 0, 1201)
		for index := 0; index < 1200; index++ {
			items = append(items, domain.EmbyMonitorItem{ItemID: fmt.Sprint(index), Name: "original", DateCreated: now})
		}
		items = append(items, domain.EmbyMonitorItem{ItemID: "0", Name: "latest", DateCreated: now})
		mock.ExpectBegin()
		for index, size := range []int{500, 500, 200} {
			expect := mock.ExpectExec("INSERT INTO t_emby_monitor_item.*FROM jsonb_to_recordset").WithArgs(embyMonitorBatchArgument{size}, 3, now, true)
			if index == failure {
				expect.WillReturnError(errors.New("chunk failed"))
				break
			}
			expect.WillReturnResult(sqlmock.NewResult(0, int64(size)))
		}
		if failure < 0 {
			mock.ExpectCommit()
		} else {
			mock.ExpectRollback()
		}
		err := NewEmbyMonitorDAO(database).UpsertItems(3, items, true, now)
		if (err == nil) != (failure < 0) {
			t.Fatal(err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
}
