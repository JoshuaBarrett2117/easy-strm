package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func shareExportSQLFixture(t *testing.T) (*ShareStrmService, *strmMemory, domain.ShareStrmSettings, sqlmock.Sqlmock) {
	t.Helper()
	service, memory, _ := strmFixture(t)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		database.Close()
	})
	service.SetExportDatabase(database)
	settings, err := service.Settings()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT to_regclass").WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(false))
	return service, memory, settings, mock
}

func expectShareExportUnlock(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock_all()")).WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectShareExportLocks(mock sqlmock.Sqlmock, root, busy string) {
	chain := []string{}
	for directory := root; ; directory = filepath.Dir(directory) {
		chain = append([]string{directory}, chain...)
		if filepath.Dir(directory) == directory {
			break
		}
	}
	for index, directory := range chain {
		locked := busy != "directory" || index != len(chain)-1
		mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock_shared(hashtextextended($1, 34981))")).WithArgs(directory).
			WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(locked))
	}
	if busy != "directory" {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock(hashtextextended('share:default',34982))")).
			WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(busy != "owner"))
	}
	if busy != "" {
		expectShareExportUnlock(mock)
	}
}

func expectShareExportSnapshot(mock sqlmock.Sqlmock, root string) {
	legacy := filepath.Join(root, "legacy.strm")
	mock.ExpectQuery("SELECT strm_config_id,local_strm_path").WillReturnRows(sqlmock.NewRows([]string{"id", "path"}).AddRow(-1, legacy))
	mock.ExpectExec("INSERT INTO t_strm_export_history SELECT").WithArgs("share:default", legacy).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT output_path FROM t_strm_export_state UNION SELECT output_path FROM t_strm_export_history").
		WillReturnRows(sqlmock.NewRows([]string{"path"}).AddRow(legacy))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT export_key,last_seen_run_id FROM t_strm_export_state WHERE owner_key=$1")).WithArgs("share:default").
		WillReturnRows(sqlmock.NewRows([]string{"key", "run"}).AddRow("1:1:1", "previous-live").AddRow("stale", "previous-stale"))
}

func expectShareExportFinish(mock sqlmock.Sqlmock, keys, runs []string) *sqlmock.ExpectedExec {
	return mock.ExpectExec(regexp.QuoteMeta(`UPDATE t_strm_export_state output SET state='stale'
 FROM unnest($2::text[],$3::text[]) target(export_key,run)
 WHERE output.owner_key=$1 AND output.export_key=target.export_key AND output.last_seen_run_id=target.run`)).
		WithArgs("share:default", pq.Array(keys), pq.Array(runs))
}

func expectShareExportWrite(mock sqlmock.Sqlmock, outputPath, protection string) {
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock(hashtextextended($1, 34981))")).WithArgs(outputPath).
			WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	}
	owners := sqlmock.NewRows([]string{"owner", "key"})
	if protection == "legacy-own" {
		owners.AddRow("share:default", "")
	} else if protection == "legacy-foreign" {
		owners.AddRow("cloud115:7", "")
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT owner_key,export_key FROM t_strm_export_state WHERE output_path=$1 AND state<>'missing'
 UNION ALL SELECT owner_key,'' FROM t_strm_export_history WHERE output_path=$1
 UNION ALL SELECT CASE WHEN strm_config_id=-1 THEN 'share:default' ELSE 'cloud115:'||strm_config_id::text END,''
 FROM t_strm_file WHERE local_strm_path=$1
 AND NOT EXISTS(SELECT 1 FROM t_strm_export_state WHERE output_path=$1)`)).WithArgs(outputPath).WillReturnRows(owners)
	if protection == "" || protection == "legacy-own" {
		mock.ExpectExec("INSERT INTO t_strm_export_history VALUES").WithArgs("share:default", outputPath).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO t_strm_export_history SELECT").WithArgs("share:default", "1:1:1", outputPath).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO t_strm_export_state").WithArgs("share:default", "1:1:1", outputPath, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "export-test").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock(hashtextextended($1, 34981))")).WithArgs(outputPath).WillReturnResult(sqlmock.NewResult(0, 0))
	}
}

func TestShareStrmExportSnapshotScope(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		query domain.ShareLibraryQuery
		full  bool
	}{
		{"work", domain.ShareLibraryQuery{WorkKey: "tmdb:tv:1"}, false},
		{"file", domain.ShareLibraryQuery{FileIDs: []int{1}}, false},
		{"keyword", domain.ShareLibraryQuery{Keyword: "Show1"}, false},
		{"full", domain.ShareLibraryQuery{}, true},
		{"full-display-options", domain.ShareLibraryQuery{Page: 2, PageSize: 20, Sort: "title", Direction: "desc", Available: true}, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, memory, settings, mock := shareExportSQLFixture(t)
			source := phaseSource(1)
			source.Result.SeasonNumber, source.Result.EpisodeNumber = 1, 1
			memory.sources = []domain.ShareStrmSource{source}
			relative, err := service.strmRelativePath(source, domain.ShareFileInfo{Name: "Show.S01E01.mkv", Path: source.FileName}, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(settings.OutputPath, relative)
			expectShareExportLocks(mock, settings.OutputPath, "")
			if scenario.full {
				expectShareExportSnapshot(mock, settings.OutputPath)
			}
			expectShareExportWrite(mock, outputPath, "")
			if scenario.full {
				expectShareExportFinish(mock, []string{"stale"}, []string{"previous-stale"}).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			expectShareExportUnlock(mock)
			if err := service.export(context.Background(), settings, scenario.query, "export-test"); err != nil {
				t.Fatal(err)
			}
			entry, err := domain.BuildShareStrmEntry(source)
			if err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(outputPath)
			if err != nil || string(content) != settings.BaseURL+"/share-strm/"+entry.ID+"\n" || len(memory.files) != 1 {
				t.Fatalf("导出文件或登记错误: content=%q files=%d err=%v", content, len(memory.files), err)
			}
		})
	}
}

func TestShareStrmExportFilteredPathProtection(t *testing.T) {
	for _, protection := range []string{"legacy-own", "legacy-foreign", "unregistered"} {
		t.Run(protection, func(t *testing.T) {
			service, memory, settings, mock := shareExportSQLFixture(t)
			source := phaseSource(1)
			source.Result.SeasonNumber, source.Result.EpisodeNumber = 1, 1
			memory.sources = []domain.ShareStrmSource{source}
			relative, err := service.strmRelativePath(source, domain.ShareFileInfo{Name: "Show.S01E01.mkv", Path: source.FileName}, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(settings.OutputPath, relative)
			if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(outputPath, []byte("protected\n"), 0644); err != nil {
				t.Fatal(err)
			}
			expectShareExportLocks(mock, settings.OutputPath, "")
			expectShareExportWrite(mock, outputPath, protection)
			expectShareExportUnlock(mock)
			err = service.export(context.Background(), settings, domain.ShareLibraryQuery{FileIDs: []int{1}}, "export-test")
			if (err == nil) != (protection == "legacy-own") {
				t.Fatalf("路径保护结果错误: %v", err)
			}
			content, readErr := os.ReadFile(outputPath)
			if readErr != nil || (protection != "legacy-own" && string(content) != "protected\n") {
				t.Fatalf("受保护文件被改写: %q err=%v", content, readErr)
			}
		})
	}
}

func TestShareStrmExportLockRetryBeforeSnapshot(t *testing.T) {
	for _, busy := range []string{"directory", "owner"} {
		t.Run(busy, func(t *testing.T) {
			service, _, settings, mock := shareExportSQLFixture(t)
			expectShareExportLocks(mock, settings.OutputPath, busy)
			expectShareExportLocks(mock, settings.OutputPath, "")
			expectShareExportSnapshot(mock, settings.OutputPath)
			expectShareExportFinish(mock, []string{"1:1:1", "stale"}, []string{"previous-live", "previous-stale"}).WillReturnResult(sqlmock.NewResult(0, 2))
			expectShareExportUnlock(mock)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := service.export(ctx, settings, domain.ShareLibraryQuery{}, "export-test"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareStrmExportLockCancellation(t *testing.T) {
	for _, busy := range []string{"directory", "owner"} {
		t.Run(busy, func(t *testing.T) {
			service, _, settings, mock := shareExportSQLFixture(t)
			expectShareExportLocks(mock, settings.OutputPath, busy)
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			if err := service.export(ctx, settings, domain.ShareLibraryQuery{}, "export-test"); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("等待锁取消错误: %v", err)
			}
		})
	}
}

func TestShareStrmExportSnapshotFailures(t *testing.T) {
	for _, phase := range []string{"lock", "legacy", "known", "snapshot", "categories", "finish"} {
		t.Run(phase, func(t *testing.T) {
			service, _, settings, mock := shareExportSQLFixture(t)
			failure := errors.New("导出故障: " + phase)
			if phase == "lock" {
				mock.ExpectQuery("SELECT pg_try_advisory_lock_shared").WillReturnError(failure)
			} else {
				expectShareExportLocks(mock, settings.OutputPath, "")
				legacy := mock.ExpectQuery("SELECT strm_config_id,local_strm_path")
				if phase == "legacy" {
					legacy.WillReturnError(failure)
				} else {
					legacy.WillReturnRows(sqlmock.NewRows([]string{"id", "path"}))
					known := mock.ExpectQuery("SELECT output_path FROM t_strm_export_state")
					if phase == "known" {
						known.WillReturnError(failure)
					} else {
						known.WillReturnRows(sqlmock.NewRows([]string{"path"}))
						snapshot := mock.ExpectQuery("SELECT export_key,last_seen_run_id").WithArgs("share:default")
						if phase == "snapshot" {
							snapshot.WillReturnError(failure)
						} else {
							snapshot.WillReturnRows(sqlmock.NewRows([]string{"key", "run"}).AddRow("stale", "previous-stale"))
							if phase == "categories" {
								service.categoriesContext = func(context.Context) ([]*domain.MediaCategory, error) { return nil, failure }
							} else {
								expectShareExportFinish(mock, []string{"stale"}, []string{"previous-stale"}).WillReturnError(failure)
							}
						}
					}
				}
			}
			expectShareExportUnlock(mock)
			if err := service.export(context.Background(), settings, domain.ShareLibraryQuery{}, "export-test"); !errors.Is(err, failure) {
				t.Fatalf("原始故障丢失: %v", err)
			}
		})
	}
}
