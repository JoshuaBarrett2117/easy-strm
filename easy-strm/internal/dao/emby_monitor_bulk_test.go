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

const embyMonitorUpsertSQL = `INSERT INTO t_emby_monitor_item(server_id,item_id,item_type,name,series_name,production_year,image_tag,date_created,first_seen_at,is_baseline)
	SELECT $2,item.item_id,item.item_type,item.name,COALESCE(item.series_name,''),COALESCE(item.year,0),COALESCE(item.image_tag,''),item.date_created,$3,$4
	FROM jsonb_to_recordset($1::jsonb) AS item(item_id text,item_type text,name text,series_name text,year integer,image_tag text,date_created timestamptz)
	ON CONFLICT(server_id,item_id) DO UPDATE SET item_type=EXCLUDED.item_type,name=EXCLUDED.name,
	series_name=EXCLUDED.series_name,production_year=EXCLUDED.production_year,image_tag=EXCLUDED.image_tag,date_created=EXCLUDED.date_created
	WHERE (t_emby_monitor_item.item_type,t_emby_monitor_item.name,t_emby_monitor_item.series_name,t_emby_monitor_item.production_year,t_emby_monitor_item.image_tag,t_emby_monitor_item.date_created)
	IS DISTINCT FROM (EXCLUDED.item_type,EXCLUDED.name,EXCLUDED.series_name,EXCLUDED.production_year,EXCLUDED.image_tag,EXCLUDED.date_created)`

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
	for _, failure := range []int{-1, 0, 1, 2} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			now := time.Now()
			items := make([]domain.EmbyMonitorItem, 0, 1201)
			for index := 0; index < 1200; index++ {
				items = append(items, domain.EmbyMonitorItem{ItemID: fmt.Sprint(index), Name: "original", DateCreated: now})
			}
			items = append(items, domain.EmbyMonitorItem{ItemID: "0", Name: "latest", DateCreated: now})
			failureErr := errors.New("chunk failed")
			mock.ExpectBegin()
			for index, size := range []int{500, 500, 200} {
				expect := mock.ExpectExec(embyMonitorUpsertSQL).WithArgs(embyMonitorBatchArgument{size}, 3, now, true)
				if index == failure {
					expect.WillReturnError(failureErr)
					break
				}
				expect.WillReturnResult(sqlmock.NewResult(0, int64(size)))
			}
			if failure < 0 {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = NewEmbyMonitorDAO(database).UpsertItems(3, items, true, now)
			if (failure < 0 && err != nil) || (failure >= 0 && !errors.Is(err, failureErr)) {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEmbyMonitorUpsertItemsSQLAndParameters(t *testing.T) {
	for _, baseline := range []bool{false, true} {
		t.Run(fmt.Sprint(baseline), func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
			item := domain.EmbyMonitorItem{ItemID: "item", ItemType: "Episode", Name: "名称", SeriesName: "剧集", Year: 2024, ImageTag: "image", DateCreated: now.Add(-time.Hour)}
			payload, err := json.Marshal([]domain.EmbyMonitorItem{item})
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectBegin()
			mock.ExpectExec(embyMonitorUpsertSQL).WithArgs(string(payload), 3, now, baseline).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectCommit()
			if err := NewEmbyMonitorDAO(database).UpsertItems(3, []domain.EmbyMonitorItem{item}, baseline, now); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEmbyMonitorUpsertItemsEmptyAndTransactionFailures(t *testing.T) {
	for _, phase := range []string{"empty", "begin", "commit"} {
		t.Run(phase, func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			var items []domain.EmbyMonitorItem
			var expectedErr error
			now := time.Now()
			if phase != "empty" {
				items = []domain.EmbyMonitorItem{{ItemID: "item", DateCreated: now}}
				expectedErr = errors.New("事务故障: " + phase)
				if phase == "begin" {
					mock.ExpectBegin().WillReturnError(expectedErr)
				} else {
					mock.ExpectBegin()
					mock.ExpectExec(embyMonitorUpsertSQL).WithArgs(embyMonitorBatchArgument{1}, 3, now, false).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit().WillReturnError(expectedErr)
				}
			}
			if err := NewEmbyMonitorDAO(database).UpsertItems(3, items, false, now); !errors.Is(err, expectedErr) {
				t.Fatalf("事务错误丢失: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
