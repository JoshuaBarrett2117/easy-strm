package service

import (
	"context"
	"crypto/sha256"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
)

type shareIncrementalAttemptContext struct{}
type shareExplicitFullContext struct{}
type shareSelectionPreparedContext struct{}

const shareIncrementalMaxRounds = 1000
const shareIncrementalPendingProbeLimit = 100
const shareIncrementalProgressBatchSize = 10

type shareIncrementalAttempt struct {
	record     func(context.Context, string) error
	keys       map[int][]string
	mappedKeys map[string]string
}

// ShareExportCheckpointStore 定义增量消费实际需要的持久协议，测试可替换文件外的数据库步骤。
type ShareExportCheckpointStore interface {
	CheckSchema(context.Context) error
	ReadInput(context.Context) (domain.ShareExportInput, error)
	Prepare(context.Context, domain.ShareExportInput, string) error
	RequireBaseline(context.Context) error
	DirtyBatch(context.Context, string) ([]domain.ShareExportDirty, error)
	ObserveWork(context.Context, string) ([]domain.ShareExportSourceState, error)
	WorkKeys(context.Context, string) ([]string, error)
	TargetSnapshot(context.Context, []string) (dao.ExportSnapshot, error)
	RecordPlannedKeys(context.Context, string, int64, []string) error
	CompleteWork(context.Context, domain.ShareExportDirty, int64, string, []domain.ShareExportSourceState, dao.ExportSnapshot) (int, error)
	FinishBaseline(context.Context, int64) (bool, error)
}

// ShareStrmIncrementalService 全局消费作品待办；不接受筛选条件、不执行旧全量或 FinishSnapshot。
type ShareStrmIncrementalService struct {
	exporter    *ShareStrmService
	checkpoints ShareExportCheckpointStore
}

// NewShareStrmIncrementalService 复用原导出器及安全输出；构造不自动运行或迁移。
func NewShareStrmIncrementalService(exporter *ShareStrmService, checkpoints ShareExportCheckpointStore) *ShareStrmIncrementalService {
	return &ShareStrmIncrementalService{exporter: exporter, checkpoints: checkpoints}
}

func shareExportFingerprint(input domain.ShareExportInput) (string, error) {
	input.Settings.DedupeExport = true
	raw, err := json.Marshal(struct {
		Protocol   int
		Naming     string
		Settings   domain.ShareStrmSettings
		Templates  map[string]string
		Categories []*domain.MediaCategory
	}{1, "share-item-selection-v2", input.Settings, input.Templates, input.Categories})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

// Rebuild 重建可信历史下的作品检查点；building 续跑不重复种子，未知历史必须由独立对账入口处理。
func (s *ShareStrmIncrementalService) Rebuild(ctx context.Context, id string) error {
	if s == nil || s.checkpoints == nil || s.exporter == nil {
		return fmt.Errorf("分享增量导出未初始化")
	}
	store, ok := s.checkpoints.(ShareReconciliationCheckpointStore)
	if !ok {
		return dao.ErrShareExportBaselineRequired
	}
	return NewShareStrmScheduledService(s.exporter, store).Run(ctx, "reconciliation", id)
}

func (s *ShareStrmIncrementalService) cancelled(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.exporter.tasks != nil && s.exporter.tasks.IsCancelled(id) {
		return context.Canceled
	}
	return nil
}

// Run 执行可信基线或有界待办消费；每作品失败保留 pending，不按 seq/时间跳过变更。
func (s *ShareStrmIncrementalService) Run(ctx context.Context, id string) (runErr error) {
	ctx = logger.WithTaskID(ctx, id)
	entry := logger.WithContext(ctx, "share_incremental")
	origin := "scheduled"
	if logger.RequestID(ctx) != "" {
		origin = "http"
	}
	entry.Log(logger.INFO, "分享 STRM 增量导出开始", logger.Fields{"event": "export_start", "origin": origin}, nil)
	defer func() {
		level, message := logger.INFO, "分享 STRM 增量导出完成"
		if runErr != nil {
			level, message = logger.ERROR, "分享 STRM 增量导出未完成"
		}
		entry.Log(level, message, logger.Fields{"event": "export_complete"}, runErr)
	}()
	if s == nil || id == "" || s.exporter == nil || s.checkpoints == nil || s.exporter.exportDB == nil {
		return fmt.Errorf("分享增量导出未初始化或任务ID为空")
	}
	if err := s.checkpoints.CheckSchema(ctx); err != nil {
		return err
	}
	if reader, ok := s.checkpoints.(interface {
		ReadWatermark(context.Context) (*domain.ShareIncrementalWatermark, error)
	}); ok {
		if _, err := reader.ReadWatermark(ctx); err != nil {
			return err
		}
	}
	input, err := s.checkpoints.ReadInput(ctx)
	if err != nil {
		return err
	}
	if !input.LegacyOutputsReconciled {
		return dao.ErrShareExportBaselineRequired
	}
	if s.exporter.exportDB.Stats().MaxOpenConnections == 1 {
		return fmt.Errorf("分享增量导出需要至少两个数据库连接")
	}
	if err = validateShareStrmSettings(&input.Settings); err != nil {
		return err
	}
	input.Settings.OutputPath, err = NormalizeStrmOutputPath(input.Settings.OutputPath)
	if err != nil {
		return err
	}
	fingerprint, err := shareExportFingerprint(input)
	if err != nil {
		return err
	}
	if input.BaselineState != "ready" || input.PreparedRevision != input.ConfigRevision || input.ProtocolVersion != 1 || input.Fingerprint != fingerprint {
		return dao.ErrShareExportBaselineRequired
	}
	output, err := waitStrmOutput(ctx, s.exporter.exportDB, input.Settings.OutputPath, "share:default", id, func() bool { return s.cancelled(ctx, id) != nil }, false)
	if err != nil {
		return err
	}
	defer output.Store.Close()
	return s.runPrepared(ctx, id, input, fingerprint, output)
}

func (s *ShareStrmIncrementalService) runPrepared(ctx context.Context, id string, input domain.ShareExportInput, fingerprint string, output *StrmOutput) (runErr error) {
	ctx = logger.WithTaskID(ctx, id)
	ctx = s.exporter.Coordinator().batchContext(ctx, id)
	ctx = context.WithValue(ctx, shareExportOutputContext{}, output)
	if explicit, _ := ctx.Value(shareExplicitFullContext{}).(bool); !explicit && (input.BaselineState != "ready" || !input.LegacyOutputsReconciled || input.PreparedRevision != input.ConfigRevision || input.Fingerprint != fingerprint) {
		return dao.ErrShareExportBaselineRequired
	}
	var err error
	after := ""
	processed, sources, stale := 0, 0, 0
	written, skippedDedupe := 0, 0
	rounds := 0
	roundConsumed := 0
	phase := "consuming"
	failures := []error{}
	accumulated, _ := ctx.Value(shareScheduledMetadataContext{}).(map[string]interface{})
	preserveFullMetrics := accumulated["full_export_completed"] == true && accumulated["written"] != nil
	persist := func() error {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		pending, pendingErr := s.checkpoints.DirtyBatch(persistCtx, "")
		pendingExact := len(pending) < shareIncrementalPendingProbeLimit
		consumed := processed - len(failures)
		defer func() {
			logger.WithContext(ctx, "share_incremental").Log(logger.INFO, "分享 STRM 待办消费进度", logger.Fields{"event": "consumption_progress", "phase": phase, "rounds": rounds, "processed_works": processed, "consumed_works": consumed, "round_consumed_works": roundConsumed, "pending_works": len(pending), "pending_works_exact": pendingExact && pendingErr == nil, "pending_probe_failed": pendingErr != nil, "cursor": after}, pendingErr)
		}()
		if phase == "completed" && pendingErr != nil {
			phase = "failed"
		}
		if s.exporter.tasks == nil {
			return pendingErr
		}
		var progressErr error
		if !preserveFullMetrics {
			progressErr = s.exporter.tasks.UpdateProgressContext(persistCtx, id, processed, processed, consumed, len(failures))
		}
		if phase == "completed" && (pendingErr != nil || progressErr != nil) {
			phase = "failed"
		}
		metadata := map[string]interface{}{"share_export": true, "phase": phase, "consumption_rounds": rounds, "consumption_cursor": after, "processed_works": processed, "consumed_works": consumed, "round_consumed_works": roundConsumed, "failed_works": len(failures), "affected_sources": sources, "stale_marked": stale, "config_revision": input.ConfigRevision, "added": output.Added, "updated": output.Updated, "skipped": output.Skipped, "conflicts": output.Conflicts, "consumption_written": written, "consumption_skipped_dedupe": skippedDedupe, "written_unit": shareStrmWrittenUnit, "skipped_dedupe_unit": shareStrmSkippedDedupeUnit, "conflict_policy": shareStrmConflictPolicy, "pending_probe_failed": pendingErr != nil}
		if pendingErr == nil {
			metadata["pending_works"], metadata["pending_works_exact"] = len(pending), pendingExact
		} else {
			metadata["pending_works_exact"] = false
		}
		if phase == "completed" {
			metadata["completion_boundary"] = "finish_baseline_cas"
		}
		if !preserveFullMetrics {
			metadata["exported_files"], metadata["written"], metadata["skipped_dedupe"] = written, written, skippedDedupe
		}
		metadataErr := s.exporter.updateExportMetadata(persistCtx, id, metadata)
		if metadataErr != nil && phase == "completed" {
			phase = "failed"
			metadataErr = errors.Join(metadataErr, s.exporter.updateExportMetadata(persistCtx, id, map[string]interface{}{"phase": phase}))
		}
		return errors.Join(pendingErr, progressErr, metadataErr)
	}
	defer func() {
		if runErr != nil {
			phase = "failed"
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
				phase = "cancelled"
			} else if errors.Is(runErr, dao.ErrShareExportChanged) {
				phase = "pending"
			}
		}
		runErr = errors.Join(runErr, persist())
	}()
	if err = s.cancelled(ctx, id); err != nil {
		return err
	}
	if err = s.exporter.preparationStage(ctx, "checkpoint-prepare", func(ctx context.Context, _ sharePreparationReport) error {
		return s.checkpoints.Prepare(ctx, input, fingerprint)
	}); err != nil {
		return err
	}
	selectionPrepared, _ := ctx.Value(shareSelectionPreparedContext{}).(bool)
	selectionSchemaChecked := selectionPrepared
	for {
		if err = s.cancelled(ctx, id); err != nil {
			return err
		}
		batch, batchErr := s.checkpoints.DirtyBatch(ctx, after)
		if batchErr != nil {
			return batchErr
		}
		if len(batch) == 0 {
			break
		}

		if rounds >= shareIncrementalMaxRounds {
			return fmt.Errorf("分享增量已达到 %d 轮消费上限，剩余待办留待下次运行：%w", shareIncrementalMaxRounds, dao.ErrShareExportChanged)
		}
		rounds++
		roundConsumed = 0
		for _, dirty := range batch {
			if err = s.cancelled(ctx, id); err != nil {
				return err
			}
			if dirty.WorkKey <= after {
				return fmt.Errorf("分享待办游标未推进：%q <= %q", dirty.WorkKey, after)
			}
			previous := after
			after = dirty.WorkKey
			var preparedStates []domain.ShareExportSourceState
			if !selectionPrepared {
				if selections, ok := s.exporter.store.(shareSelectionExportStore); ok {
					if !selectionSchemaChecked {
						if err = s.exporter.preparationStage(ctx, "selection-schema", func(ctx context.Context, _ sharePreparationReport) error {
							return selections.CheckSelectionSchema(ctx)
						}); err != nil {
							return err
						}
						selectionSchemaChecked = true
						ctx = context.WithValue(ctx, shareSelectionSchemaCheckedContext{}, true)
					}
					var found bool
					dirty, preparedStates, found, err = s.prepareDirtyWork(ctx, selections, dirty, previous)
					if err != nil {
						return err
					}
					if !found {
						continue
					}
				}
			}
			count, marked, result := s.consumeWork(ctx, id, input, dirty, preparedStates)
			sources += count
			stale += marked
			written += result.Written
			skippedDedupe += result.SkippedDedupe
			workErr := result.Err
			processed++
			if workErr != nil {
				failures = append(failures, fmt.Errorf("作品 %s：%w", dirty.WorkKey, workErr))
			} else {
				roundConsumed++
			}
			if errors.Is(workErr, context.Canceled) || errors.Is(workErr, context.DeadlineExceeded) {
				return workErr
			}
			if processed%shareIncrementalProgressBatchSize == 0 {
				if err = persist(); err != nil {
					return err
				}
			}
		}
		latest, inputErr := s.checkpoints.ReadInput(ctx)
		if inputErr != nil {
			return inputErr
		}
		if latest.ConfigRevision != input.ConfigRevision {
			return dao.ErrShareExportChanged
		}
		if err = persist(); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	if err = s.cancelled(ctx, id); err != nil {
		return err
	}
	ready, err := s.checkpoints.FinishBaseline(ctx, input.ConfigRevision)
	if err != nil {
		return err
	}
	if !ready {
		return dao.ErrShareExportChanged
	}
	phase = "completed"
	return nil
}

func (s *ShareStrmIncrementalService) prepareDirtyWork(ctx context.Context, selections shareSelectionExportStore, dirty domain.ShareExportDirty, previous string) (domain.ShareExportDirty, []domain.ShareExportSourceState, bool, error) {
	states, err := s.checkpoints.ObserveWork(ctx, dirty.WorkKey)
	if err != nil {
		return dirty, nil, false, err
	}
	if states == nil {
		states = []domain.ShareExportSourceState{}
	}
	if err = s.exporter.prepareSelectionWork(ctx, selections, dirty.WorkKey); err != nil {
		return dirty, nil, false, err
	}
	refreshed, err := s.checkpoints.DirtyBatch(ctx, previous)
	if err != nil {
		return dirty, nil, false, err
	}
	for _, candidate := range refreshed {
		if candidate.WorkKey == dirty.WorkKey {
			return candidate, states, true, nil
		}
	}
	return dirty, states, false, nil
}

func (s *ShareStrmIncrementalService) consumeWork(ctx context.Context, id string, input domain.ShareExportInput, dirty domain.ShareExportDirty, preparedStates []domain.ShareExportSourceState) (int, int, strmExportResult) {
	result := strmExportResult{}
	states, err := s.checkpoints.ObserveWork(ctx, dirty.WorkKey)
	if err != nil {
		return 0, 0, strmExportResult{Err: err}
	}
	if preparedStates != nil && !sameShareExportSourceStates(preparedStates, states) {
		return 0, 0, strmExportResult{Err: dao.ErrShareExportChanged}
	}
	observed := make(map[int]domain.ShareExportSourceState, len(states))
	for _, state := range states {
		observed[state.SourceFileID] = state
	}
	oldKeys, err := s.checkpoints.WorkKeys(ctx, dirty.WorkKey)
	if err != nil {
		return 0, 0, strmExportResult{Err: err}
	}
	if err = s.checkpoints.RecordPlannedKeys(ctx, dirty.WorkKey, dirty.Revision, oldKeys); err != nil {
		return 0, 0, strmExportResult{Err: err}
	}
	snapshot, err := s.checkpoints.TargetSnapshot(ctx, oldKeys)
	if err != nil {
		return 0, 0, strmExportResult{Err: err}
	}
	attempt := &shareIncrementalAttempt{keys: map[int][]string{}, record: func(ctx context.Context, key string) error {
		return s.checkpoints.RecordPlannedKeys(ctx, dirty.WorkKey, dirty.Revision, []string{key})
	}}
	ctx = context.WithValue(ctx, shareIncrementalAttemptContext{}, attempt)
	after, processed := 0, 0
	seen := map[string]bool{}
	var conflicts map[string]*shareStrmConflict
	if _, rereads := s.exporter.store.(shareStrmSourceReader); !rereads {
		conflicts, err = s.exporter.shareStrmConflicts(ctx, domain.ShareLibraryQuery{WorkKey: dirty.WorkKey})
		if err != nil {
			return 0, 0, strmExportResult{Err: err}
		}
	}
	for {
		rows, readErr := s.exporter.store.StrmSources(ctx, domain.ShareLibraryQuery{WorkKey: dirty.WorkKey}, after)
		if readErr != nil {
			result.Err = readErr
			return processed, 0, result
		}
		if len(rows) == 0 {
			break
		}
		for _, source := range rows {
			if err = s.cancelled(ctx, id); err != nil {
				result.Err = err
				return processed, 0, result
			}
			if source.ID <= after || source.WorkKey != dirty.WorkKey {
				result.Err = fmt.Errorf("作品来源分页或归属不一致")
				return processed, 0, result
			}
			after = source.ID
			processed++
			state, exists := observed[source.ID]
			if !exists || state.State != "active" || state.FileVersion != source.FileVersion || state.ShareVersion != source.ShareVersion || state.MediaID != source.MediaID || state.ShareID != source.ShareID || state.EpisodeSignature != shareSourceEpisodeSignature(source) {
				result.Err = dao.ErrShareExportChanged
				return processed, 0, result
			}
			sourceResult := s.exporter.exportLocalStrm(ctx, input.Settings, source, input.Categories, seen, conflicts)
			result.Written += sourceResult.Written
			result.SkippedDedupe += sourceResult.SkippedDedupe
			if sourceResult.Err != nil {
				result.Err = sourceResult.Err
				return processed, 0, result
			}
		}
	}
	for index := range states {
		if states[index].State == "active" {
			states[index].ExportKeys = attempt.keys[states[index].SourceFileID]
			if len(states[index].ExportKeys) == 0 {
				result.Err = dao.ErrShareExportChanged
				return processed, 0, result
			}
		}
	}
	if err = s.cancelled(ctx, id); err != nil {
		result.Err = err
		return processed, 0, result
	}
	if _, selected := s.exporter.store.(shareSelectionExportStore); selected {
		latestStates, observeErr := s.checkpoints.ObserveWork(ctx, dirty.WorkKey)
		if observeErr != nil || !sameShareExportSourceStates(states, latestStates) {
			result.Err = errors.Join(observeErr, dao.ErrShareExportChanged)
			return processed, 0, result
		}
	}
	marked, err := s.checkpoints.CompleteWork(ctx, dirty, input.ConfigRevision, id, states, snapshot)
	result.Err = err
	return processed, marked, result
}

func sameShareExportSourceStates(first, second []domain.ShareExportSourceState) bool {
	if len(first) != len(second) {
		return false
	}
	byID := make(map[int]domain.ShareExportSourceState, len(first))
	for _, state := range first {
		state.ExportKeys = nil
		byID[state.SourceFileID] = state
	}
	for _, state := range second {
		state.ExportKeys = nil
		if !reflect.DeepEqual(byID[state.SourceFileID], state) {
			return false
		}
	}
	return true
}
