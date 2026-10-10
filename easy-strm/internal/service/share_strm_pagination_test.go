package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-redis/redis/v8"
)

type strmPaginationStore struct {
	*strmMemory
	reader     func(context.Context, domain.ShareLibraryQuery, int) ([]domain.ShareStrmSource, error)
	traversals [][]int
	savedPaths []string
}

func (store *strmPaginationStore) StrmSources(ctx context.Context, query domain.ShareLibraryQuery, after int) ([]domain.ShareStrmSource, error) {
	if after == 0 {
		store.traversals = append(store.traversals, []int{})
	}
	var rows []domain.ShareStrmSource
	var err error
	if store.reader != nil {
		rows, err = store.reader(ctx, query, after)
	} else {
		for _, source := range store.sources {
			if source.ID > after && len(rows) < 100 {
				rows = append(rows, source)
			}
		}
	}
	for _, source := range rows {
		index := len(store.traversals) - 1
		store.traversals[index] = append(store.traversals[index], source.ID)
	}
	return rows, err
}

func (store *strmPaginationStore) SaveStrmEntry(ctx context.Context, entry domain.ShareStrmEntry) error {
	store.savedPaths = append(store.savedPaths, entry.FilePath)
	return store.strmMemory.SaveStrmEntry(ctx, entry)
}

type strmPaginationProgressHook struct {
	shareProgressFaultHook
	snapshots []map[string]interface{}
}

func (hook *strmPaginationProgressHook) BeforeProcess(ctx context.Context, command redis.Cmder) (context.Context, error) {
	if command.Name() != "set" || len(command.Args()) < 3 {
		return ctx, nil
	}
	raw, ok := command.Args()[2].([]byte)
	if !ok {
		return ctx, nil
	}
	var task map[string]interface{}
	if err := json.Unmarshal(raw, &task); err != nil {
		return ctx, nil
	}
	processed, _ := task["processed_files"].(float64)
	if processed > 0 && (len(hook.snapshots) == 0 || processed > hook.snapshots[len(hook.snapshots)-1]["processed_files"].(float64)) {
		hook.snapshots = append(hook.snapshots, task)
	}
	return ctx, nil
}

func strmPaginationSources(count int) ([]domain.ShareStrmSource, []int) {
	sources, ids := []domain.ShareStrmSource{}, []int{}
	for index := 0; index < count; index++ {
		id := 7 + index*13
		source := filesystemSource(id, 9, 12345, filesystemEpisode(1, 1))
		if count == 205 && index == count-1 {
			source.Episodes = nil
		}
		sources, ids = append(sources, source), append(ids, id)
	}
	return sources, ids
}

func expectStrmPaginationTraversal(t *testing.T, mock sqlmock.Sqlmock, sources []domain.ShareStrmSource) {
	t.Helper()
	for start := 0; start <= len(sources); start = min(start+100, len(sources)) {
		after := 0
		if start > 0 {
			after = sources[start-1].ID
		}
		rows := sqlmock.NewRows([]string{"id", "share_id", "share_name", "work_key", "url", "password", "file_name", "remote_id", "result", "episodes"})
		for _, source := range sources[start:min(start+101, len(sources))] {
			raw, err := json.Marshal(source.Result)
			if err != nil {
				t.Fatal(err)
			}
			result := map[string]interface{}{}
			if err = json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			result["_media_id"], result["_file_size"], result["_available"] = source.MediaID, source.FileSize, source.Available
			raw, err = json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			episodes, err := json.Marshal(source.Episodes)
			if err != nil {
				t.Fatal(err)
			}
			rows.AddRow(source.ID, source.ShareID, source.ShareName, source.WorkKey, source.URL, source.Password, source.FileName, source.RemoteFileID, raw, episodes)
		}
		mock.ExpectQuery(`SELECT f.id,f.share_id.*AND m.work_key=\$1 AND f.id>\$2 ORDER BY f.id LIMIT 101$`).WithArgs("tmdb:tv:8801", after).WillReturnRows(rows).RowsWillBeClosed()
		if start == len(sources) {
			break
		}
	}
}

func TestShareStrmPaginationIterationAndExactProgress(t *testing.T) {
	for _, mode := range []string{"memory", "sqlmock"} {
		for _, dedupe := range []bool{false, true} {
			for _, count := range []int{0, 1, 99, 100, 101, 205} {
				t.Run(fmt.Sprintf("%s/dedupe=%t/%d", mode, dedupe, count), func(t *testing.T) {
					sources, ids := strmPaginationSources(count)
					service, memory, cfg := newStrmFilesystemFixture(t, sources, dedupe)
					store := &strmPaginationStore{strmMemory: memory}
					service.store = store
					var mock sqlmock.Sqlmock
					if mode == "sqlmock" {
						db, sqlMock, err := sqlmock.New()
						if err != nil {
							t.Fatal(err)
						}
						defer db.Close()
						mock = sqlMock
						store.reader = dao.NewShareRecordDAO(db).StrmSources
						expectStrmPaginationTraversal(t, mock, sources)
						expectStrmPaginationTraversal(t, mock, sources)
					}
					client := setupTaskRedisMock(t)
					defer client.Close()
					progress := &strmPaginationProgressHook{}
					client.AddHook(progress)
					service.tasks = NewTaskService(dao.NewTaskRedisDAO(client))
					if err := service.tasks.Create("pagination", "strm_generate", "分页回归"); err != nil {
						t.Fatal(err)
					}
					err := service.export(context.Background(), cfg, domain.ShareLibraryQuery{WorkKey: "tmdb:tv:8801"}, "pagination")
					failed := 0
					if count == 205 {
						failed = 1
					}
					if (err != nil) != (failed > 0) {
						t.Fatalf("导出错误: %v", err)
					}
					if len(store.traversals) != 2 {
						t.Fatalf("不应增加计数遍历: %v", store.traversals)
					}
					for _, traversal := range store.traversals {
						if !reflect.DeepEqual(traversal, ids) {
							t.Fatalf("丢失或重复处理来源: %v", traversal)
						}
					}
					if len(progress.snapshots) != (count+99)/100 {
						t.Fatalf("批次进度: %v", progress.snapshots)
					}
					for index, snapshot := range progress.snapshots {
						processed := min((index+1)*100, count)
						if snapshot["total_files"] != float64(count) || snapshot["processed_files"] != float64(processed) || snapshot["progress"] != float64(processed*100/count) {
							t.Fatalf("中间进度必须使用完整来源总数（包括无季集来源）: %v", snapshot)
						}
					}
					task, taskErr := service.tasks.Get("pagination")
					if taskErr != nil || task["total_files"] != float64(count) || task["processed_files"] != float64(count) || task["failed_files"] != float64(failed) || task["success_files"] != float64(count-failed) {
						t.Fatalf("最终进度: %v %v", task, taskErr)
					}
					written, skipped := count-failed, 0
					if written > 0 {
						skipped, written = written-1, 1
					}
					metadata := task["metadata"].(map[string]interface{})
					if count > 0 {
						assertFilesystemMetadata(t, metadata, written, skipped)
					} else if metadata["written"] != nil || metadata["skipped_dedupe"] != nil {
						t.Fatalf("空导出不应新增进度刷新: %v", metadata)
					}
					if len(memory.files) != written || len(store.savedPaths) != count-failed || len(memory.entries) != count-failed {
						t.Fatalf("重复导出或去重行为改变: files=%d saves=%d entries=%d", len(memory.files), len(store.savedPaths), len(memory.entries))
					}
					if mock != nil {
						if err = mock.ExpectationsWereMet(); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func TestShareStrmPaginationConflictTraversalOnce(t *testing.T) {
	sources, ids := strmPaginationSources(205)
	service, memory, _ := newStrmFilesystemFixture(t, sources, true)
	store := &strmPaginationStore{strmMemory: memory}
	service.store = store
	conflicts, err := service.shareStrmConflicts(context.Background(), domain.ShareLibraryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.traversals) != 1 || !reflect.DeepEqual(store.traversals[0], ids) {
		t.Fatalf("冲突预遍历重复或丢失来源: %v", store.traversals)
	}
	conflict := conflicts[shareStrmIdentity(sources[0], filesystemEpisode(1, 1))]
	if len(conflicts) != 1 || conflict == nil || len(conflict.sources) != 204 || conflict.fileCounts[9] != 204 || conflict.winners[shareStrmIdentity(sources[0], filesystemEpisode(1, 1))] != ids[0] {
		t.Fatalf("前瞻行改变冲突计数或去重排名: %+v", conflict)
	}
}
