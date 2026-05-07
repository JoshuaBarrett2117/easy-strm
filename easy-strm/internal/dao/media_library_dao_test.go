package dao

import (
	"regexp"
	"testing"
	"time"

	"easy-strm/internal/domain"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestTaskStepDAOCreateAndList(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	now := time.Now()
	dao := NewTaskStepDAO()
	stepRows := sqlmock.NewRows([]string{
		"id", "task_id", "step_key", "step_name", "status", "sort_order", "input_summary", "output_summary", "error_message", "started_at", "finished_at", "created_at", "updated_at",
	}).AddRow(1, "task-1", "scan_source", "扫描源目录", "pending", 10, "source_id=1", "", "", nil, nil, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_task_step (task_id, step_key, step_name, status, sort_order, input_summary, output_summary, error_message, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (task_id, step_key) DO UPDATE SET
			step_name=EXCLUDED.step_name,
			status=EXCLUDED.status,
			sort_order=EXCLUDED.sort_order,
			input_summary=EXCLUDED.input_summary,
			output_summary=EXCLUDED.output_summary,
			error_message=EXCLUDED.error_message,
			started_at=EXCLUDED.started_at,
			finished_at=EXCLUDED.finished_at,
			updated_at=CURRENT_TIMESTAMP
		RETURNING id, task_id, step_key, step_name, status, sort_order, input_summary, output_summary, error_message, started_at, finished_at, created_at, updated_at`)).
		WithArgs("task-1", "scan_source", "扫描源目录", "pending", 10, "source_id=1", "", "", nil, nil).
		WillReturnRows(stepRows)

	step, err := dao.Create(&domain.TaskStep{
		TaskID:       "task-1",
		StepKey:      "scan_source",
		StepName:     "扫描源目录",
		Status:       "pending",
		SortOrder:    10,
		InputSummary: "source_id=1",
	})
	if err != nil {
		t.Fatalf("expected create step to succeed: %v", err)
	}
	if step.ID != 1 || step.StepKey != "scan_source" {
		t.Fatalf("unexpected step: %+v", step)
	}

	listRows := sqlmock.NewRows([]string{
		"id", "task_id", "step_key", "step_name", "status", "sort_order", "input_summary", "output_summary", "error_message", "started_at", "finished_at", "created_at", "updated_at",
	}).AddRow(1, "task-1", "scan_source", "扫描源目录", "pending", 10, "source_id=1", "", "", nil, nil, now, now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, step_key, step_name, status, sort_order, input_summary, output_summary, error_message, started_at, finished_at, created_at, updated_at FROM t_task_step WHERE task_id=$1 ORDER BY sort_order ASC, id ASC`)).
		WithArgs("task-1").
		WillReturnRows(listRows)

	steps, err := dao.ListByTask("task-1")
	if err != nil {
		t.Fatalf("expected list steps to succeed: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected one step, got %d", len(steps))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSyncIndexDAOUpsert(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	now := time.Now()
	modified := now.Add(-time.Hour)
	dao := NewMediaSyncIndexDAO()
	rows := sqlmock.NewRows([]string{
		"id", "source_id", "source_type", "source_file_id", "source_path", "source_name", "source_pick_code", "source_sha1", "source_size", "source_modified_time", "target_path", "strm_path", "metadata_path", "media_server_type", "media_server_library_id", "tmdb_id", "media_type", "identity_status", "sync_status", "last_change_type", "last_task_id", "created_at", "updated_at",
	}).AddRow(7, 3, "local", "Movie.mkv", `C:\media\Movie.mkv`, "Movie.mkv", "", "", int64(100), modified, "/library/Movie.mkv", "", "", "emby", "lib-1", 0, "", "unknown", "active", "scanned", "task-1", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_media_sync_index (
			source_id, source_type, source_file_id, source_path, source_name, source_pick_code, source_sha1,
			source_size, source_modified_time, target_path, strm_path, metadata_path, media_server_type,
			media_server_library_id, tmdb_id, media_type, identity_status, sync_status, last_change_type, last_task_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (source_id, source_file_id) DO UPDATE SET
			source_type = EXCLUDED.source_type,
			source_path = EXCLUDED.source_path,
			source_name = EXCLUDED.source_name,
			source_pick_code = EXCLUDED.source_pick_code,
			source_sha1 = EXCLUDED.source_sha1,
			source_size = EXCLUDED.source_size,
			source_modified_time = EXCLUDED.source_modified_time,
			target_path = EXCLUDED.target_path,
			strm_path = EXCLUDED.strm_path,
			metadata_path = EXCLUDED.metadata_path,
			media_server_type = EXCLUDED.media_server_type,
			media_server_library_id = EXCLUDED.media_server_library_id,
			tmdb_id = EXCLUDED.tmdb_id,
			media_type = EXCLUDED.media_type,
			identity_status = EXCLUDED.identity_status,
			sync_status = EXCLUDED.sync_status,
			last_change_type = EXCLUDED.last_change_type,
			last_task_id = EXCLUDED.last_task_id,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, source_id, source_type, source_file_id, source_path, source_name, source_pick_code, source_sha1, source_size, source_modified_time, target_path, strm_path, metadata_path, media_server_type, media_server_library_id, tmdb_id, media_type, identity_status, sync_status, last_change_type, last_task_id, created_at, updated_at`)).
		WithArgs(3, "local", "Movie.mkv", `C:\media\Movie.mkv`, "Movie.mkv", "", "", int64(100), &modified, "/library/Movie.mkv", "", "", "emby", "lib-1", 0, "", "unknown", "active", "scanned", "task-1").
		WillReturnRows(rows)

	item, err := dao.Upsert(&domain.MediaSyncIndex{
		SourceID:             3,
		SourceType:           "local",
		SourceFileID:         "Movie.mkv",
		SourcePath:           `C:\media\Movie.mkv`,
		SourceName:           "Movie.mkv",
		SourceSize:           100,
		SourceModifiedTime:   &modified,
		TargetPath:           "/library/Movie.mkv",
		MediaServerType:      "emby",
		MediaServerLibraryID: "lib-1",
		IdentityStatus:       "unknown",
		SyncStatus:           "active",
		LastChangeType:       "scanned",
		LastTaskID:           "task-1",
	})
	if err != nil {
		t.Fatalf("expected upsert to succeed: %v", err)
	}
	if item.ID != 7 || item.SourceFileID != "Movie.mkv" {
		t.Fatalf("unexpected index item: %+v", item)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSyncIndexDAOGetByID(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	now := time.Now()
	dao := NewMediaSyncIndexDAO()
	rows := sqlmock.NewRows([]string{
		"id", "source_id", "source_type", "source_file_id", "source_path", "source_name", "source_pick_code", "source_sha1", "source_size", "source_modified_time", "target_path", "strm_path", "metadata_path", "media_server_type", "media_server_library_id", "tmdb_id", "media_type", "identity_status", "sync_status", "last_change_type", "last_task_id", "created_at", "updated_at",
	}).AddRow(9, 3, "local", "Movie.mkv", `C:\media\Movie.mkv`, "Movie.mkv", "", "", int64(100), nil, "/library/Movie.mkv", "", "", "emby", "lib-1", 0, "", "unknown", "active", "scanned", "task-1", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, source_id, source_type, source_file_id, source_path, source_name, source_pick_code, source_sha1, source_size, source_modified_time, target_path, strm_path, metadata_path, media_server_type, media_server_library_id, tmdb_id, media_type, identity_status, sync_status, last_change_type, last_task_id, created_at, updated_at FROM t_media_sync_index WHERE id=$1`)).
		WithArgs(9).
		WillReturnRows(rows)

	item, err := dao.GetByID(9)
	if err != nil {
		t.Fatalf("expected get by id to succeed: %v", err)
	}
	if item == nil || item.ID != 9 || item.SourceFileID != "Movie.mkv" {
		t.Fatalf("unexpected index item: %+v", item)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPendingMediaDAOCreateAndUpdateStatus(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	now := time.Now()
	dao := NewPendingMediaDAO()
	createRows := sqlmock.NewRows([]string{
		"id", "source_kind", "source_id", "source_file_id", "source_path", "title", "year", "media_type", "season", "episode", "tmdb_id", "status", "reason", "related_task_id", "created_at", "updated_at",
	}).AddRow(2, "sync", 5, "file-1", "/downloads/Movie.mkv", "Movie", 2024, "movie", 0, 0, 0, "pending", "识别失败", "task-1", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_pending_media_item (source_kind, source_id, source_file_id, source_path, title, year, media_type, season, episode, tmdb_id, status, reason, related_task_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, source_kind, source_id, source_file_id, source_path, title, year, media_type, season, episode, tmdb_id, status, reason, related_task_id, created_at, updated_at`)).
		WithArgs("sync", 5, "file-1", "/downloads/Movie.mkv", "Movie", 2024, "movie", 0, 0, 0, "pending", "识别失败", "task-1").
		WillReturnRows(createRows)

	item, err := dao.Create(&domain.PendingMediaItem{
		SourceKind:    "sync",
		SourceID:      5,
		SourceFileID:  "file-1",
		SourcePath:    "/downloads/Movie.mkv",
		Title:         "Movie",
		Year:          2024,
		MediaType:     "movie",
		Status:        "pending",
		Reason:        "识别失败",
		RelatedTaskID: "task-1",
	})
	if err != nil {
		t.Fatalf("expected create pending item to succeed: %v", err)
	}
	if item.ID != 2 || item.Status != "pending" {
		t.Fatalf("unexpected pending item: %+v", item)
	}

	updateRows := sqlmock.NewRows([]string{
		"id", "source_kind", "source_id", "source_file_id", "source_path", "title", "year", "media_type", "season", "episode", "tmdb_id", "status", "reason", "related_task_id", "created_at", "updated_at",
	}).AddRow(2, "sync", 5, "file-1", "/downloads/Movie.mkv", "Movie", 2024, "movie", 0, 0, 0, "ignored", "用户忽略", "task-1", now, now)
	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE t_pending_media_item
		SET status=$2, reason=$3, related_task_id=COALESCE(NULLIF($4, ''), related_task_id), updated_at=CURRENT_TIMESTAMP
		WHERE id=$1
		RETURNING id, source_kind, source_id, source_file_id, source_path, title, year, media_type, season, episode, tmdb_id, status, reason, related_task_id, created_at, updated_at`)).
		WithArgs(2, "ignored", "用户忽略", "").
		WillReturnRows(updateRows)

	item, err = dao.UpdateStatus(2, "ignored", "用户忽略", "")
	if err != nil {
		t.Fatalf("expected update pending item to succeed: %v", err)
	}
	if item.Status != "ignored" {
		t.Fatalf("expected ignored status, got %s", item.Status)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
