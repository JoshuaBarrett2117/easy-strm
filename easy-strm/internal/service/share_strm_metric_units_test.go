package service

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"reflect"
	"testing"
)

func TestShareStrmMetricUnitsPreserveCountsAndInput(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		metadata map[string]interface{}
		unit     string
	}{
		{"source-export", map[string]interface{}{"export_query": domain.ShareLibraryQuery{}}, shareStrmSourceProgressUnit},
		{"incremental", map[string]interface{}{"incremental": true, "processed_works": 4}, shareStrmWorkProgressUnit},
		{"building-recovery-new-task", map[string]interface{}{"processed_works": 4}, shareStrmWorkProgressUnit},
		{"legacy-full-query-with-work-progress", map[string]interface{}{"export_query": domain.ShareLibraryQuery{}, "processed_works": 4}, shareStrmWorkProgressUnit},
		{"retry-full-preparing", map[string]interface{}{"export_query": domain.ShareLibraryQuery{}, "processed_works": 4, "phase": "preparing"}, shareStrmSourceProgressUnit},
		{"full-with-consumption", map[string]interface{}{"processed_works": 4, "full_export_completed": true}, shareStrmSourceProgressUnit},
		{"same-task-recovery", map[string]interface{}{"incremental": false, "processed_works": 4, "full_export_completed": true}, shareStrmSourceProgressUnit},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			metadata := scenario.metadata
			for key, value := range map[string]interface{}{"share_export": true, "written": 155023, "exported_files": 155023, "skipped_dedupe": 287521, "consumption_written": 2, "consumption_skipped_dedupe": 3, "pending_works": 100, "pending_works_exact": false, "phase": "consuming"} {
				if _, exists := metadata[key]; !exists {
					metadata[key] = value
				}
			}
			before, _ := json.Marshal(metadata)
			clarified := shareStrmMetricMetadata(metadata)
			after, _ := json.Marshal(metadata)
			if string(before) != string(after) {
				t.Fatal("单位说明不得修改传入元数据")
			}
			for key, value := range metadata {
				if !reflect.DeepEqual(clarified[key], value) {
					t.Fatalf("字段 %s 被单位说明修改：got=%v want=%v", key, clarified[key], value)
				}
			}
			for _, key := range []string{"total_files_unit", "processed_files_unit", "success_files_unit", "failed_files_unit"} {
				if clarified[key] != scenario.unit {
					t.Fatalf("%s 进度单位不符：%v", key, clarified)
				}
			}
			if clarified["pending_works_unit"] != "个作品待办" || clarified["consumption_written_unit"] != shareStrmWrittenUnit || clarified["consumption_skipped_dedupe_unit"] != shareStrmSkippedDedupeUnit || clarified["metric_count_note"] != shareStrmMetricCountNote {
				t.Fatalf("缺少作品/STRM/季集单位隔离：%v", clarified)
			}
			originalPhase := metadata["phase"]
			clarified["phase"] = "changed-copy"
			if metadata["phase"] != originalPhase {
				t.Fatal("单位说明应返回独立顶层副本")
			}
		})
	}
	for _, metadata := range []map[string]interface{}{nil, {"written": 7}} {
		if !reflect.DeepEqual(shareStrmMetricMetadata(metadata), metadata) {
			t.Fatal("不得扩展非分享任务元数据")
		}
	}
}

func TestShareStrmMetricUnitsMultiEpisodeSourceCountsAreNotAdditive(t *testing.T) {
	first := filesystemSource(11001, 501, 300, filesystemEpisode(1, 1), filesystemEpisode(1, 2), filesystemEpisode(2, 1), filesystemEpisode(2, 2))
	winner := filesystemSource(11002, 502, 400, filesystemEpisode(1, 1), filesystemEpisode(2, 1))
	loser := filesystemSource(11003, 503, 100, filesystemEpisode(1, 1), filesystemEpisode(2, 1), filesystemEpisode(2, 1))
	service, _, cfg := newStrmFilesystemFixture(t, []domain.ShareStrmSource{first, winner, loser}, true)
	metadata, err := runFilesystemExport(t, service, cfg, "unit-mixed")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.tasks.Get("unit-mixed")
	if err != nil {
		t.Fatal(err)
	}
	if task["processed_files"] != float64(3) || metadata["written"] != float64(4) || metadata["exported_files"] != float64(4) || metadata["skipped_dedupe"] != float64(4) || len(filesystemCollection(t, cfg.OutputPath)) != 4 {
		t.Fatalf("澄清单位不应更改原有计数或实际落盘：task=%v metadata=%v", task, metadata)
	}
	if metadata["processed_files_unit"] != shareStrmSourceProgressUnit || metadata["written_unit"] != shareStrmWrittenUnit || metadata["skipped_dedupe_unit"] != shareStrmSkippedDedupeUnit || metadata["metric_count_note"] != shareStrmMetricCountNote {
		t.Fatalf("一来源多季集的真实输出缺少单位说明：%v", metadata)
	}
}

func TestShareStrmMetricUnitsScheduledProgressAndFullRetention(t *testing.T) {
	for _, mode := range []string{"incremental", "reconciliation"} {
		t.Run(mode, func(t *testing.T) {
			fixture, store, runner := phase03Fixture(t)
			store.bump("work:a")
			store.bump("work:b")
			wantUnit := shareStrmWorkProgressUnit
			if mode == "reconciliation" {
				wantUnit = shareStrmSourceProgressUnit
				full := runner.full
				runner.full = func(ctx context.Context, input domain.ShareExportInput, id string) error {
					if err := fixture.service.tasks.UpdateProgressContext(ctx, id, 8, 8, 8, 0); err != nil {
						return err
					}
					if err := fixture.service.updateExportMetadata(ctx, id, map[string]interface{}{"written": 3, "exported_files": 3, "skipped_dedupe": 5}); err != nil {
						return err
					}
					return full(ctx, input, id)
				}
			}
			if err := phase03Run(t, fixture, runner, mode, "unit-scheduled", context.Background()); err != nil {
				t.Fatal(err)
			}
			task, err := fixture.service.tasks.Get("unit-scheduled")
			if err != nil {
				t.Fatal(err)
			}
			metadata := task["metadata"].(map[string]interface{})
			if metadata["processed_files_unit"] != wantUnit || metadata["processed_works"] != float64(2) || metadata["consumed_works_unit"] != "个作品待办" || metadata["metric_count_note"] != shareStrmMetricCountNote {
				t.Fatalf("消费单位覆盖了来源统计或缺少作品说明：%v", metadata)
			}
			if mode == "reconciliation" && (task["processed_files"] != float64(8) || metadata["written"] != float64(3) || metadata["skipped_dedupe"] != float64(5)) {
				t.Fatalf("全量指标被单位澄清改写：task=%v metadata=%v", task, metadata)
			}
		})
	}
}
