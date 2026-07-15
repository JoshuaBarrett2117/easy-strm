package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
)

type mockWatchLifecycle struct {
	started []int
	stopped []int
}

func (m *mockWatchLifecycle) StartWatching(sourceID int) error {
	m.started = append(m.started, sourceID)
	return nil
}

func (m *mockWatchLifecycle) StopWatching(sourceID int) {
	m.stopped = append(m.stopped, sourceID)
}

func setupMediaSourceControllerTest(t *testing.T) (*MediaSourceController, sqlmock.Sqlmock, func(), *mockWatchLifecycle) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	dao.Init(db, nil)
	cleanup := func() {
		_ = db.Close()
	}

	watch := &mockWatchLifecycle{}
	controller := NewMediaSourceController(service.NewMediaSourceService(dao.NewMediaSourceDAO(), nil), nil, watch, nil)
	return controller, mock, cleanup, watch
}

func newJSONContext(t *testing.T, method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	var payload []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal body: %v", err)
		}
		payload = data
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	return ctx, recorder
}

func decodeResponseBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return payload
}

func extractMediaSourcePayload(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	resp := decodeResponseBody(t, recorder)
	envelope, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected response data envelope, got %#v", resp["data"])
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected media source payload, got %#v", envelope["data"])
	}
	return data
}

func TestMediaSourceControllerCreateDisablesAutoOrganizeWhenWatchOff(t *testing.T) {
	controller, mock, cleanup, watch := setupMediaSourceControllerTest(t)
	defer cleanup()

	sourcePath := t.TempDir()
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, sourcePath, sourcePath, nil, 10, true, "/organized", "all", "skip", "move", false, false, 1800, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_media_source (name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time`)).
		WithArgs("movies", domain.SourceTypeLocal, sourcePath, sourcePath, nil, 10, true, "/organized", "all", "skip", "move", false, false, 1800, "").
		WillReturnRows(rows)

	ctx, recorder := newJSONContext(t, http.MethodPost, "/api/media/sources", map[string]any{
		"name":                 "movies",
		"source_type":          domain.SourceTypeLocal,
		"path":                 sourcePath,
		"watch_path":           sourcePath,
		"priority":             10,
		"enabled":              true,
		"organize_target_path": "/organized",
		"media_type":           "all",
		"conflict_policy":      "skip",
		"operation_mode":       "move",
		"auto_organize":        true,
		"watch_enabled":        false,
		"watch_interval":       1800,
	})

	controller.Create(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d, body: %s", recorder.Code, recorder.Body.String())
	}
	if len(watch.stopped) != 1 || watch.stopped[0] != 1 {
		t.Fatalf("unexpected stop calls: %#v", watch.stopped)
	}
	if len(watch.started) != 0 {
		t.Fatalf("unexpected start calls: %#v", watch.started)
	}

	data := extractMediaSourcePayload(t, recorder)
	if data["auto_organize"] != false {
		t.Fatalf("expected auto_organize to be disabled, got %#v", data["auto_organize"])
	}
	if data["watch_enabled"] != false {
		t.Fatalf("expected watch_enabled to stay disabled, got %#v", data["watch_enabled"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceControllerUpdateStartsWatchWhenEnabled(t *testing.T) {
	controller, mock, cleanup, watch := setupMediaSourceControllerTest(t)
	defer cleanup()

	sourcePath := t.TempDir()
	now := time.Now()
	existingRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(7, "movies", domain.SourceTypeLocal, sourcePath, sourcePath, nil, 10, true, "/organized", "all", "skip", "move", false, false, 1800, "", now, now)
	updatedRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(7, "movies", domain.SourceTypeLocal, sourcePath, sourcePath, nil, 10, true, "/organized", "all", "skip", "move", true, true, 1800, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(7).
		WillReturnRows(existingRows)
	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE t_media_source SET name=$2, source_type=$3, path=$4, watch_path=$5, cloud115_id=$6, priority=$7, enabled=$8, organize_target_path=$9, media_type=$10, conflict_policy=$11, operation_mode=$12, auto_organize=$13, watch_enabled=$14, watch_interval=$15, emby_library_id=$16
		WHERE id=$1
		RETURNING id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time`)).
		WithArgs(7, "movies", domain.SourceTypeLocal, sourcePath, sourcePath, nil, 10, true, "/organized", "all", "skip", "move", true, true, 1800, "").
		WillReturnRows(updatedRows)

	ctx, recorder := newJSONContext(t, http.MethodPut, "/api/media/sources/7", map[string]any{
		"name":                 "movies",
		"source_type":          domain.SourceTypeLocal,
		"path":                 sourcePath,
		"watch_path":           sourcePath,
		"priority":             10,
		"enabled":              true,
		"organize_target_path": "/organized",
		"media_type":           "all",
		"conflict_policy":      "skip",
		"operation_mode":       "move",
		"auto_organize":        true,
		"watch_enabled":        true,
		"watch_interval":       1800,
	})
	ctx.Params = gin.Params{{Key: "id", Value: "7"}}

	controller.Update(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d, body: %s", recorder.Code, recorder.Body.String())
	}
	if len(watch.stopped) != 1 || watch.stopped[0] != 7 {
		t.Fatalf("unexpected stop calls: %#v", watch.stopped)
	}
	if len(watch.started) != 1 || watch.started[0] != 7 {
		t.Fatalf("unexpected start calls: %#v", watch.started)
	}

	data := extractMediaSourcePayload(t, recorder)
	if data["watch_enabled"] != true {
		t.Fatalf("expected watch_enabled to stay enabled, got %#v", data["watch_enabled"])
	}
	if data["auto_organize"] != true {
		t.Fatalf("expected auto_organize to stay enabled, got %#v", data["auto_organize"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceControllerSyncWatchStateNilSafety(t *testing.T) {
	controller := NewMediaSourceController(nil, nil, nil, nil)
	controller.syncWatchState(nil)
	controller.stopWatchState(1)
}
