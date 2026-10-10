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
)

type shareIncrementalAttemptContext struct{}

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
	DirtyBatch(context.Context, []string) ([]domain.ShareExportDirty, error)
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
	raw, err := json.Marshal(struct {
		Protocol   int
		Naming     string
		Settings   domain.ShareStrmSettings
		Templates  map[string]string
		Categories []*domain.MediaCategory
	}{1, "share-fixed-path-v1", input.Settings, input.Templates, input.Categories})
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
	if err := s.checkpoints.CheckSchema(ctx); err != nil {
		return err
	}
	input, err := s.checkpoints.ReadInput(ctx)
	if err != nil {
		return err
	}
	if !input.LegacyOutputsReconciled {
		return dao.ErrShareExportBaselineRequired
	}
	fingerprint, err := shareExportFingerprint(input)
	if err != nil {
		return err
	}
	if input.BaselineState != "required" && !(input.BaselineState == "building" && input.PreparedRevision == input.ConfigRevision && input.Fingerprint == fingerprint) {
		if err = s.checkpoints.RequireBaseline(ctx); err != nil {
			return err
		}
	}
	return s.Run(ctx, id)
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
	entry.Log(logger.INFO, "分享 STRM 增量导出开始", logger.Fields{"event": "export_start"}, nil)
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
	output, err := waitStrmOutput(ctx, s.exporter.exportDB, input.Settings.OutputPath, "share:default", id, func() bool { return s.cancelled(ctx, id) != nil }, false)
	if err != nil {
		return err
	}
	defer output.Store.Close()
	return s.runPrepared(ctx, id, input, fingerprint, output)
}

func (s *ShareStrmIncrementalService) runPrepared(ctx context.Context, id string, input domain.ShareExportInput, fingerprint string, output *StrmOutput) (runErr error) {
	ctx = s.exporter.Coordinator().batchContext(ctx, id)
	ctx = context.WithValue(ctx, shareExportOutputContext{}, output)
	if err := s.checkpoints.Prepare(ctx, input, fingerprint); err != nil {
		return err
	}
	var err error
	attempted := []string{}
	processed, sources, stale := 0, 0, 0
	written, skippedDedupe := 0, 0
	failures := []error{}
	persist := func() error {
		if s.exporter.tasks == nil {
			return nil
		}
		progressErr := s.exporter.tasks.UpdateProgress(id, processed, processed, processed-len(failures), len(failures))
		metadataErr := s.exporter.updateExportMetadata(ctx, id, map[string]interface{}{"share_export": true, "processed_works": processed, "affected_sources": sources, "stale_marked": stale, "pending_works": len(failures), "config_revision": input.ConfigRevision, "added": output.Added, "updated": output.Updated, "skipped": output.Skipped, "conflicts": output.Conflicts, "exported_files": written, "written": written, "written_unit": shareStrmWrittenUnit, "skipped_dedupe": skippedDedupe, "skipped_dedupe_unit": shareStrmSkippedDedupeUnit, "conflict_policy": shareStrmConflictPolicy})
		return errors.Join(progressErr, metadataErr)
	}
	defer func() { runErr = errors.Join(runErr, persist()) }()
	for {
		if err = s.cancelled(ctx, id); err != nil {
			return err
		}
		batch, batchErr := s.checkpoints.DirtyBatch(ctx, attempted)
		if batchErr != nil {
			return batchErr
		}
		if len(batch) == 0 {
			break
		}
		for _, dirty := range batch {
			if err = s.cancelled(ctx, id); err != nil {
				return err
			}
			attempted = append(attempted, dirty.WorkKey)
			count, marked, result := s.consumeWork(ctx, id, input, dirty)
			sources += count
			stale += marked
			written += result.Written
			skippedDedupe += result.SkippedDedupe
			workErr := result.Err
			processed++
			if workErr != nil {
				failures = append(failures, fmt.Errorf("作品 %s：%w", dirty.WorkKey, workErr))
			}
			if errors.Is(workErr, context.Canceled) || errors.Is(workErr, context.DeadlineExceeded) {
				return workErr
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
	return nil
}

func (s *ShareStrmIncrementalService) consumeWork(ctx context.Context, id string, input domain.ShareExportInput, dirty domain.ShareExportDirty) (int, int, strmExportResult) {
	result := strmExportResult{}
	states, err := s.checkpoints.ObserveWork(ctx, dirty.WorkKey)
	if err != nil {
		return 0, 0, strmExportResult{Err: err}
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
	marked, err := s.checkpoints.CompleteWork(ctx, dirty, input.ConfigRevision, id, states, snapshot)
	result.Err = err
	return processed, marked, result
}
