package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestShareWorkReexportMetadataFinalVerificationRollsBack(test *testing.T) {
	for _, scenario := range []string{"post_file", "post_source", "cancel_midway"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			entries, states := maps.Clone(fixture.store.entries), maps.Clone(fixture.store.states)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.checkpoint.onInput = func(read int) {
				if read != 4 {
					return
				}
				switch scenario {
				case "post_file":
					state := fixture.store.states[fixture.values()[0].Selection.ItemKey]
					if err := os.WriteFile(state.Path, []byte("https://changed.example.test/share-strm/"+state.Playback+"\n"), 0644); err != nil {
						test.Fatal(err)
					}
				case "post_source":
					fixture.store.sources[0].FileVersion++
				case "cancel_midway":
					cancel()
				}
			}
			id := "share_work_reexport_final_" + scenario
			fixture.expectMetadata(test, id, scenario, 0)
			fixture.expectClose()
			result, err := fixture.run(test, ctx, id)
			if err == nil || result.ProcessedRows != 0 || !reflect.DeepEqual(entries, fixture.store.entries) || !reflect.DeepEqual(states, fixture.store.states) {
				test.Fatalf("最终核验失败必须撤销全部 SQL 写入: %+v %v", result, err)
			}
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestShareWorkReexportCannotForgeFingerprintForDifferentFile(test *testing.T) {
	fixture := newWorkReexportFixture(test)
	value := fixture.values()[0]
	state := fixture.store.states[value.Selection.ItemKey]
	content := filesystemContent(fixture.store.sources[2])
	if err := os.WriteFile(state.Path, content, 0644); err != nil {
		test.Fatal(err)
	}
	state.Content = fmt.Sprintf("%x", sha256.Sum256(append([]byte(state.Path+"\x00"), content...)))
	fixture.store.states[state.Key] = state
	id := "share_work_reexport_forged_content"
	fixture.expectMetadata(test, id, "content", 0)
	fixture.expectClose()
	result, err := fixture.run(test, context.Background(), id)
	if !errors.Is(err, dao.ErrShareExportChanged) || result.ProcessedRows != 0 || !strings.Contains(err.Error(), "不是同一分享文件") {
		test.Fatalf("内容 hash 不能替代实际分享文件身份校验: %+v %v", result, err)
	}
	if err = fixture.mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestShareWorkReexportCannotProveDifferentOrUnknownFileID(test *testing.T) {
	entry := domain.ShareStrmEntry{ShareCode: "fixture", Password: "fixture-password", FilePath: "same/movie.mkv", FileName: "movie.mkv"}
	for _, ids := range [][2]string{{"cached-unproven", ""}, {"", "source-known"}, {"cached-wrong", "source-known"}} {
		previous, next := entry, entry
		previous.FileID, next.FileID = ids[0], ids[1]
		if sameWorkReexportFile(previous, next) {
			test.Fatalf("不同/无法证明相同的实际 FileID 不得放行: old=%q new=%q", ids[0], ids[1])
		}
	}
	for _, id := range []string{"", "same-known-file"} {
		previous, next := entry, entry
		previous.FileID, next.FileID = id, id
		if !sameWorkReexportFile(previous, next) {
			test.Fatal("相同路径及相同 FileID 应允许 metadata-only")
		}
	}
}

func TestShareWorkReexportInputFailuresDoNotFallBack(test *testing.T) {
	for _, scenario := range []string{"empty", "empty_selection", "unknown", "invalid_selection", "wrong_media", "empty_path", "traversal", "path_collision", "selection_cap", "read_error", "schema", "cancel_before", "after_count"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			files := filesystemCollection(test, fixture.request.ExpectedOutputPath)
			work := fixture.request.WorkKeys[0]
			values := fixture.store.works[work]
			opened := true
			switch scenario {
			case "empty":
				fixture.request.WorkKeys = nil
				opened = false
			case "empty_selection":
				fixture.store.works[work] = nil
			case "unknown":
				fixture.request.WorkKeys = []string{"unknown"}
			case "invalid_selection":
				values[0].Selection.Valid = false
			case "wrong_media":
				values[0].MediaID++
			case "empty_path":
				values[0].Selection.RelativePath = ""
			case "traversal":
				values[0].Selection.RelativePath = "../outside.strm"
			case "path_collision":
				values[1].Selection.RelativePath = values[0].Selection.RelativePath
			case "selection_cap":
				fixture.store.works[work] = make([]domain.ShareWorkReexportSelection, domain.ShareWorkReexportMaxSelections+1)
			case "read_error":
				fixture.store.readErr = errors.New("fixture read error")
			case "schema":
				fixture.checkpoint.schemaError = errors.New("fixture schema error")
				opened = false
			case "cancel_before":
				cancel()
				opened = false
			case "after_count":
				fixture.checkpoint.countErrAt = 2
			}
			id := "share_work_reexport_input_" + scenario
			if scenario == "after_count" {
				fixture.expectMetadata(test, id, "", 0)
			}
			if opened {
				fixture.expectClose()
			}
			result, err := fixture.run(test, ctx, id)
			if err == nil {
				test.Fatalf("无效参数不得成功: %+v", result)
			}
			if scenario == "after_count" {
				if result.ProcessedRows != 2 || result.GlobalInvalidAfter != nil || result.AfterCountError == "" {
					test.Fatal("计数失败不可伪装零，已提交元数据应可幂等重试")
				}
			} else if result.ProcessedRows != 0 {
				test.Fatal("无效输入不得处理选择")
			}
			assertFilesystemCollection(test, fixture.request.ExpectedOutputPath, files)
			if scenario == "selection_cap" && !strings.Contains(err.Error(), "10000") {
				test.Fatal("必须实际验证选择上限")
			}
			if scenario != "empty" {
				task, taskErr := fixture.tasks.Get(id)
				if taskErr != nil {
					test.Fatal(taskErr)
				}
				metadata, marshalErr := json.Marshal(task["metadata"])
				if marshalErr != nil || !strings.Contains(string(metadata), `"requested_works"`) || !strings.Contains(string(metadata), `"work_reexport_request"`) || !strings.Contains(string(metadata), `"error"`) {
					test.Fatalf("失败仍须保留请求范围和错误审计: %s %v", metadata, marshalErr)
				}
			}
			if len(fixture.store.savedSources) != 0 || fixture.store.bulkResolves != 0 || fixture.store.syncs != 0 {
				test.Fatal("失败不得回退 full 或普通 export")
			}
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}
