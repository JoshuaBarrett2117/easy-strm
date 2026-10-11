package service

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

type workReexportArgument struct{ check func(driver.Value) bool }

func (argument workReexportArgument) Match(value driver.Value) bool { return argument.check(value) }

func (fixture *workReexportFixture) values() []domain.ShareWorkReexportSelection {
	values := []domain.ShareWorkReexportSelection{}
	for _, work := range fixture.request.WorkKeys {
		values = append(values, fixture.store.works[work]...)
	}
	return values
}

func (fixture *workReexportFixture) expectPaths() {
	for _, value := range fixture.values() {
		fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock\\(hashtextextended").WithArgs(filepath.Join(fixture.request.ExpectedOutputPath, value.Selection.RelativePath)).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	}
}

func (fixture *workReexportFixture) expectUnlockPaths() {
	values := fixture.values()
	for index := len(values) - 1; index >= 0; index-- {
		fixture.mock.ExpectExec("SELECT pg_advisory_unlock\\(hashtextextended").WithArgs(filepath.Join(fixture.request.ExpectedOutputPath, values[index].Selection.RelativePath)).WillReturnResult(sqlmock.NewResult(0, 1))
	}
}

func workReexportSourceRows(test *testing.T, source domain.ShareStrmSource) *sqlmock.Rows {
	test.Helper()
	raw, err := json.Marshal(source.Result)
	if err != nil {
		test.Fatal(err)
	}
	var result map[string]interface{}
	if err = json.Unmarshal(raw, &result); err != nil {
		test.Fatal(err)
	}
	result["_media_id"], result["_file_version"], result["_share_version"], result["_file_size"], result["_available"] = source.MediaID, source.FileVersion, source.ShareVersion, source.FileSize, source.Available
	raw, err = json.Marshal(result)
	if err != nil {
		test.Fatal(err)
	}
	episodes, err := json.Marshal(source.Episodes)
	if err != nil {
		test.Fatal(err)
	}
	return sqlmock.NewRows([]string{"id", "share", "name", "work", "url", "password", "file", "remote", "result", "episodes"}).AddRow(source.ID, source.ShareID, source.ShareName, source.WorkKey, source.URL, source.Password, source.FileName, source.RemoteFileID, raw, episodes)
}

func workReexportStateRows(state dao.ExportState) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"path", "content", "mapping", "playback", "run"}).AddRow(state.Path, state.Content, state.Mapping, state.Playback, state.Run)
}

func (fixture *workReexportFixture) expectMetadata(test *testing.T, id, failure string, failAt int) {
	test.Helper()
	fixture.expectPaths()
	fixture.mock.ExpectBegin()
	fixture.mock.ExpectExec("SELECT pg_advisory_xact_lock\\(34983,1\\)").WillReturnResult(sqlmock.NewResult(0, 1))
	fixture.mock.ExpectQuery("SELECT config_revision.*FOR SHARE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(fixture.request.ExpectedConfigRevision))
	mappings, items, paths, relatives, urls, itemMappings := []string{}, []string{}, []string{}, []string{}, []string{}, []string{}
	media := []int64{}
	for _, value := range fixture.values() {
		source, err := fixture.store.GetStrmSource(context.Background(), value.SourceID)
		if err != nil {
			test.Fatal(err)
		}
		entry, err := domain.BuildShareStrmEntry(source)
		if err != nil {
			test.Fatal(err)
		}
		state := fixture.store.states[value.Selection.ItemKey]
		file, err := readWorkReexportFile(state.Path)
		if err != nil {
			test.Fatal(err)
		}
		mappings = append(mappings, entry.ID)
		media = append(media, int64(entry.MediaID))
		items = append(items, value.Selection.ItemKey)
		paths = append(paths, filepath.Join(fixture.request.ExpectedOutputPath, value.Selection.RelativePath))
		relatives = append(relatives, value.Selection.RelativePath)
		urls = append(urls, file.mapping)
		itemMappings = append(itemMappings, domain.ShareWorkReexportMappingID(entry.ID, value.Selection.ItemKey))
	}
	preflight := fixture.mock.ExpectQuery("WITH targets AS").WithArgs(pq.Array(mappings), pq.Array(media), pq.Array(items), pq.Array(paths), pq.Array(relatives), pq.Array(fixture.request.WorkKeys), domain.ShareWorkReexportMaxReferences, pq.Array(urls), pq.Array(itemMappings))
	if failure == "closure" {
		preflight.WillReturnRows(sqlmock.NewRows([]string{"total", "rejected", "work", "item", "reason"}).AddRow(4, 1, "outside", "unprocessed", "outside_work_scope"))
		fixture.mock.ExpectRollback()
		fixture.expectUnlockPaths()
		return
	}
	preflight.WillReturnRows(sqlmock.NewRows([]string{"total", "rejected", "work", "item", "reason"}).AddRow(len(fixture.values())*2, 0, "", "", ""))
	states := []dao.ExportState{}
	entries := []domain.ShareStrmEntry{}
	oldIDs, newIDs := map[string]bool{}, map[string]bool{}
	for index, value := range fixture.values() {
		selection := value.Selection
		selectedRevision := selection.Revision
		if failure == "revision" && index == failAt {
			selectedRevision++
		}
		fixture.mock.ExpectQuery("SELECT selection_revision,selected_candidate_id,stable_relative_path,work_key").WithArgs(selection.ItemKey).WillReturnRows(sqlmock.NewRows([]string{"revision", "candidate", "path", "work"}).AddRow(selectedRevision, selection.CandidateID, selection.RelativePath, selection.WorkKey))
		if failure == "revision" && index == failAt {
			break
		}
		source, err := fixture.store.GetStrmSource(context.Background(), value.SourceID)
		if err != nil {
			test.Fatal(err)
		}
		fresh := source
		if failure == "source" && index == failAt {
			fresh.FileVersion++
		}
		fixture.mock.ExpectQuery("SELECT f.id,f.share_id.*FOR SHARE OF f,s,m").WithArgs(source.ID).WillReturnRows(workReexportSourceRows(test, fresh))
		if failure == "source" && index == failAt {
			break
		}
		state := fixture.store.states[selection.ItemKey]
		if failure == "state_path" && index == failAt {
			state.Path += ".wrong"
		}
		fixture.mock.ExpectQuery("SELECT output_path,content_fingerprint,mapping_fingerprint,.*last_seen_run_id").WithArgs(selection.ItemKey).WillReturnRows(workReexportStateRows(state))
		if failure == "state_path" && index == failAt {
			break
		}
		mapping := fixture.store.entries[state.Playback]
		if failure == "mapping" && index == failAt {
			mapping.FilePath = "different/actual-file.mkv"
		}
		if failure == "file_id" && index == failAt {
			mapping.FileID = "cached-unproven"
		}
		payload, _ := json.Marshal(mapping)
		fixture.mock.ExpectQuery("SELECT payload FROM t_share_strm.*FOR UPDATE").WithArgs(state.Playback).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
		file, err := readWorkReexportFile(state.Path)
		if err != nil {
			test.Fatal(err)
		}
		urlMapping := fixture.store.entries[file.mapping]
		payload, _ = json.Marshal(urlMapping)
		fixture.mock.ExpectQuery("SELECT payload FROM t_share_strm.*FOR UPDATE").WithArgs(file.mapping).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
		entry, err := domain.BuildShareWorkReexportEntry(source, selection.ItemKey)
		if err != nil {
			test.Fatal(err)
		}
		entries = append(entries, entry)
		legacy, _ := domain.BuildShareStrmEntry(source)
		oldIDs[state.Playback], oldIDs[file.mapping], oldIDs[legacy.ID] = true, true, true
		newIDs[entry.ID] = true
		state.Playback, state.Run = entry.ID, id
		payload, _ = json.Marshal(entry)
		state.Mapping = fmt.Sprintf("%x", sha256.Sum256(payload))
		states = append(states, state)
	}
	if failure == "revision" || failure == "source" || failure == "state_path" || failure == "mapping" || failure == "file_id" || failure == "content" || failure == "file_changed" {
		fixture.mock.ExpectRollback()
		fixture.expectUnlockPaths()
		return
	}
	for index, value := range fixture.values() {
		entry, state := entries[index], states[index]
		payload, _ := json.Marshal(entry)
		insertion := fixture.mock.ExpectExec("INSERT INTO t_share_strm\\(id,payload\\)").WithArgs(entry.ID, workReexportArgument{check: func(actual driver.Value) bool {
			if actual != string(payload) {
				return false
			}
			fixture.store.entries[entry.ID] = entry
			return true
		}})
		if failure == "insert" && index == failAt {
			insertion.WillReturnError(errors.New("fixture insertion failed"))
			break
		}
		insertion.WillReturnResult(sqlmock.NewResult(0, 1))
		if failure == "occupied" && index == failAt {
			occupied := entry
			occupied.MediaID++
			occupiedPayload, _ := json.Marshal(occupied)
			fixture.mock.ExpectQuery("SELECT payload FROM t_share_strm.*FOR UPDATE").WithArgs(entry.ID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(occupiedPayload))
			break
		}
		fixture.mock.ExpectQuery("SELECT payload FROM t_share_strm.*FOR UPDATE").WithArgs(entry.ID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
		fixture.mock.ExpectExec("INSERT INTO t_strm_export_history SELECT").WithArgs(state.Owner, state.Key, state.Path).WillReturnResult(sqlmock.NewResult(0, 0))
		fixture.mock.ExpectExec("INSERT INTO t_strm_export_state").WithArgs(state.Owner, state.Key, state.Path, state.Content, state.Mapping, state.Playback, workReexportArgument{check: func(actual driver.Value) bool {
			if actual != id {
				return false
			}
			fixture.store.states[state.Key] = state
			return true
		}}).WillReturnResult(sqlmock.NewResult(0, 1))
		selection := value.Selection
		changed := int64(1)
		if failure == "CAS" && index == failAt {
			changed = 0
		}
		fixture.mock.ExpectExec("UPDATE t_share_media_selection SET exported_revision").WithArgs(selection.ItemKey, selection.Revision, selection.CandidateID, selection.RelativePath, selection.WorkKey).WillReturnResult(sqlmock.NewResult(0, changed))
		if changed == 0 {
			break
		}
		if failure == "post_state" && index == failAt {
			state.Content = "incorrect-written-digest"
			fixture.mock.ExpectQuery("SELECT output_path,content_fingerprint,mapping_fingerprint,.*last_seen_run_id").WithArgs(selection.ItemKey).WillReturnRows(workReexportStateRows(state))
			break
		}
		fixture.mock.ExpectQuery("SELECT output_path,content_fingerprint,mapping_fingerprint,.*last_seen_run_id").WithArgs(selection.ItemKey).WillReturnRows(workReexportStateRows(state))
	}
	if failure == "insert" || failure == "CAS" || failure == "occupied" || failure == "post_state" {
		fixture.mock.ExpectRollback()
	} else {
		remaining := []string{}
		for old := range oldIDs {
			if !newIDs[old] {
				remaining = append(remaining, old)
			}
		}
		sort.Strings(remaining)
		fixture.mock.ExpectQuery("SELECT EXISTS.*share_strm_id=ANY").WithArgs(pq.Array(remaining)).WillReturnRows(sqlmock.NewRows([]string{"active"}).AddRow(failure == "remaining"))
		if failure == "remaining" || failure == "post_file" || failure == "post_source" || failure == "cancel_midway" {
			fixture.mock.ExpectRollback()
		} else if failure == "commit" {
			fixture.mock.ExpectCommit().WillReturnError(errors.New("fixture commit failed"))
		} else {
			fixture.mock.ExpectCommit()
		}
	}
	fixture.expectUnlockPaths()
}

func TestShareWorkReexportPerItemMetadataPreservesHistoryAndFiles(test *testing.T) {
	for _, sharedAcrossMedia := range []bool{false, true} {
		test.Run(fmt.Sprintf("different_media=%v", sharedAcrossMedia), func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			if sharedAcrossMedia {
				first := fixture.store.sources[0]
				second := fixture.store.sources[2]
				second.ShareID, second.URL, second.FileName = first.ShareID, first.URL, first.FileName
				fixture.store.sources[2] = second
				fixture.request.WorkKeys = append(fixture.request.WorkKeys, second.WorkKey)
				legacy, _ := domain.BuildShareStrmEntry(first)
				value := fixture.store.works[second.WorkKey][0]
				state := fixture.store.states[value.Selection.ItemKey]
				state.Playback = legacy.ID
				content := filesystemContent(first)
				if err := os.WriteFile(state.Path, content, 0644); err != nil {
					test.Fatal(err)
				}
				state.Content = fmt.Sprintf("%x", sha256.Sum256(append([]byte(state.Path+"\x00"), content...)))
				fixture.store.states[state.Key] = state
			}
			files := filesystemCollection(test, fixture.request.ExpectedOutputPath)
			originalMappings := maps.Clone(fixture.store.entries)
			modified := map[string]os.FileInfo{}
			for path := range files {
				filename := filepath.Join(fixture.request.ExpectedOutputPath, path)
				info, err := os.Stat(filename)
				if err != nil {
					test.Fatal(err)
				}
				modified[filename] = info
			}
			for attempt := 0; attempt < 2; attempt++ {
				id := fmt.Sprintf("share_work_reexport_metadata_%d", attempt)
				fixture.expectMetadata(test, id, "", 0)
				fixture.expectClose()
				result, err := fixture.run(test, context.Background(), id)
				if err != nil || result.ProcessedRows != len(fixture.values()) || result.ProcessedWorkCount != len(fixture.request.WorkKeys) || result.Written != 0 {
					test.Fatalf("%+v %v", result, err)
				}
				assertFilesystemCollection(test, fixture.request.ExpectedOutputPath, files)
				active := map[string]bool{}
				for _, value := range fixture.values() {
					state := fixture.store.states[value.Selection.ItemKey]
					source, _ := fixture.store.GetStrmSource(context.Background(), value.SourceID)
					entry, _ := domain.BuildShareWorkReexportEntry(source, value.Selection.ItemKey)
					if state.Playback != entry.ID || active[state.Playback] || !reflect.DeepEqual(fixture.store.entries[state.Playback], entry) {
						test.Fatal("必须每 item 独立重绑正确 mapping")
					}
					active[state.Playback] = true
					info, err := os.Stat(state.Path)
					if err != nil || !info.ModTime().Equal(modified[state.Path].ModTime()) || !os.SameFile(info, modified[state.Path]) {
						test.Fatal("文件路径、mtime 或 inode 改变")
					}
				}
				for id, mapping := range originalMappings {
					actual, err := fixture.store.GetStrmEntry(context.Background(), id)
					if err != nil || !reflect.DeepEqual(actual, mapping) || active[id] {
						test.Fatal("旧播放 URL 映射不得覆盖，完整闭包后不能保留 active 引用")
					}
				}
				if err = fixture.mock.ExpectationsWereMet(); err != nil {
					test.Fatal(err)
				}
			}
		})
	}
}

func TestShareWorkReexportMetadataRollbackAndRetry(test *testing.T) {
	for _, failure := range []string{"insert", "CAS", "revision", "source", "state_path", "remaining", "closure", "content", "mapping", "file_id", "occupied", "post_state", "commit"} {
		test.Run(failure, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			originalEntries, originalStates := maps.Clone(fixture.store.entries), maps.Clone(fixture.store.states)
			files := filesystemCollection(test, fixture.request.ExpectedOutputPath)
			if failure == "content" {
				key := fixture.values()[0].Selection.ItemKey
				state := fixture.store.states[key]
				state.Content = "not-the-file-digest"
				fixture.store.states[key] = state
			}
			id := "share_work_reexport_rollback_" + failure
			fixture.expectMetadata(test, id, failure, 1)
			fixture.expectClose()
			result, err := fixture.run(test, context.Background(), id)
			if err == nil || result.ProcessedRows != 0 || result.ProcessedWorkCount != 0 {
				test.Fatalf("整批失败必须回滚：%+v %v", result, err)
			}
			if failure != "insert" && failure != "commit" && !errors.Is(err, dao.ErrShareExportChanged) {
				test.Fatalf("变化必须保留错误类型: %v", err)
			}
			if !reflect.DeepEqual(originalEntries, fixture.store.entries) {
				test.Fatal("回滚后不得残留新 mapping")
			}
			fixture.store.states = originalStates
			assertFilesystemCollection(test, fixture.request.ExpectedOutputPath, files)
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
			id += "_retry"
			fixture.expectMetadata(test, id, "", 0)
			fixture.expectClose()
			result, err = fixture.run(test, context.Background(), id)
			if err != nil || result.ProcessedRows != 2 {
				test.Fatalf("失败后重试必须成功: %+v %v", result, err)
			}
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestShareWorkReexportRejectsMissingOrDifferentURLFiles(test *testing.T) {
	for _, scenario := range []string{"missing", "different_url", "symlink"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			state := fixture.store.states[fixture.values()[0].Selection.ItemKey]
			switch scenario {
			case "missing":
				if err := os.Remove(state.Path); err != nil {
					test.Fatal(err)
				}
			case "different_url":
				if err := os.WriteFile(state.Path, []byte("https://wrong.example.test/other\n"), 0644); err != nil {
					test.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(state.Path); err != nil {
					test.Fatal(err)
				}
				if err := os.Symlink("missing", state.Path); err != nil {
					test.Fatal(err)
				}
			}
			fixture.expectClose()
			result, err := fixture.run(test, context.Background(), "share_work_reexport_file_"+scenario)
			if !errors.Is(err, dao.ErrShareExportChanged) || result.ProcessedRows != 0 {
				test.Fatalf("%+v %v", result, err)
			}
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestShareWorkReexportGuardsBeforeTransaction(test *testing.T) {
	for _, scenario := range []string{"config", "fingerprint", "selection", "scope", "source", "cancel"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.expectPaths()
			fixture.checkpoint.onInput = func(read int) {
				if read != 2 {
					return
				}
				switch scenario {
				case "config":
					fixture.checkpoint.input.ConfigRevision++
				case "fingerprint":
					fixture.checkpoint.input.Settings.BaseURL = "https://changed.example.test"
				case "selection":
					fixture.store.works[fixture.request.WorkKeys[0]][0].Selection.CandidateID++
				case "scope":
					work := fixture.request.WorkKeys[0]
					fixture.store.works[work] = append(fixture.store.works[work], fixture.store.works[work][0])
				case "source":
					fixture.store.sources[0].FileVersion++
				case "cancel":
					cancel()
				}
			}
			fixture.expectUnlockPaths()
			fixture.expectClose()
			result, err := fixture.run(test, ctx, "share_work_reexport_guard_"+scenario)
			if err == nil || result.ProcessedRows != 0 {
				test.Fatalf("%+v %v", result, err)
			}
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestShareWorkReexportScopeRefusesOnlyOneSide(test *testing.T) {
	fixture := newWorkReexportFixture(test)
	entry, _ := domain.BuildShareStrmEntry(fixture.store.sources[0])
	fixture.checkpoint.references = []workReexportReference{{work: "tmdb:tv:missing", item: "missing:1", mapping: entry.ID, relative: "other.strm", media: 2}}
	fixture.expectClose()
	result, err := fixture.run(test, context.Background(), "share_work_reexport_one_side")
	if !errors.Is(err, dao.ErrShareExportChanged) || result.ProcessedRows != 0 {
		test.Fatalf("%+v %v", result, err)
	}
	if err = fixture.mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestShareWorkReexportNoOrdinaryWriterOrCleanup(test *testing.T) {
	fixture := newWorkReexportFixture(test)
	id := "share_work_reexport_metadata_only"
	fixture.expectMetadata(test, id, "", 0)
	fixture.expectClose()
	result, err := fixture.run(test, context.Background(), id)
	if err != nil || result.Written != 0 || len(fixture.store.savedSources) != 0 || fixture.store.bulkResolves != 0 || fixture.store.syncs != 0 {
		test.Fatalf("不得调用普通 writer、full 清理或自动准备: %+v %v", result, err)
	}
	if result.GlobalInvalidBefore == nil || *result.GlobalInvalidBefore != 2 || result.GlobalInvalidAfter == nil || *result.GlobalInvalidAfter != 0 {
		test.Fatal("只有本地 fixture 的 active 凭证被修复")
	}
	if !strings.Contains(result.InvalidScope, "global") {
		test.Fatal(result.InvalidScope)
	}
	if err = fixture.mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}
