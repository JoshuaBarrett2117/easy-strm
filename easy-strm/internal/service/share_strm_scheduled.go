package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
)

// ShareReconciliationCheckpointStore 扩展现有消费协议，原子记录全量完成及待办准备，不新增持久模型。
type ShareReconciliationCheckpointStore interface {
	ShareExportCheckpointStore
	CompleteReconciliation(context.Context, domain.ShareExportInput, string) error
}

// ShareStrmScheduledService 编排显式全量准备与作品边界续跑，持有一次 owner 锁，不在重启时主动运行。
type ShareStrmScheduledService struct {
	worker *ShareStrmIncrementalService
	store  ShareReconciliationCheckpointStore
	full   func(context.Context, domain.ShareExportInput, string) error
}

type shareScheduledMetadataContext struct{}

func (s *ShareStrmService) updateExportMetadata(ctx context.Context, id string, metadata map[string]interface{}) error {
	if s.tasks == nil {
		return nil
	}
	if accumulated, ok := ctx.Value(shareScheduledMetadataContext{}).(map[string]interface{}); ok {
		for key, value := range metadata {
			accumulated[key] = value
		}
		return s.tasks.UpdateMetadata(id, accumulated)
	}
	return s.tasks.UpdateMetadata(id, metadata)
}

// NewShareStrmScheduledService 复用分享导出和检查点，构造过程不连接数据库或启动任务。
func NewShareStrmScheduledService(exporter *ShareStrmService, store ShareReconciliationCheckpointStore) *ShareStrmScheduledService {
	runner := &ShareStrmScheduledService{worker: NewShareStrmIncrementalService(exporter, store), store: store}
	runner.full = runner.reconcile
	return runner
}

func (s *ShareStrmScheduledService) input(ctx context.Context) (domain.ShareExportInput, string, error) {
	input, err := s.store.ReadInput(ctx)
	if err != nil {
		return input, "", err
	}
	if input.ProtocolVersion != 1 || (input.BaselineState != "required" && input.BaselineState != "building" && input.BaselineState != "ready") {
		return input, "", fmt.Errorf("分享检查点协议或阶段无效，请 DBA 检查现有 schema")
	}
	if err = validateShareStrmSettings(&input.Settings); err != nil {
		return input, "", err
	}
	input.Settings.OutputPath, err = NormalizeStrmOutputPath(input.Settings.OutputPath)
	if err != nil {
		return input, "", err
	}
	fingerprint, err := shareExportFingerprint(input)
	return input, fingerprint, err
}

func shareReconciliationReason(input domain.ShareExportInput, fingerprint string) string {
	if !input.LegacyOutputsReconciled {
		return "历史输出凭证不可信，强制全量对账"
	}
	if input.BaselineState == "required" {
		return "检查点要求全量对账"
	}
	if input.PreparedRevision != input.ConfigRevision {
		return "配置版本未完成对账"
	}
	if input.Fingerprint != fingerprint {
		return "输出配置指纹不可信"
	}
	return ""
}

// Run 在调度任务内同步执行；不可信检查点升级为显式全量模式，building 重试只消费未确认作品。
func (s *ShareStrmScheduledService) Run(ctx context.Context, requested, id string) error {
	ctx = logger.WithTaskID(ctx, id)
	if s == nil || s.store == nil || s.worker == nil || s.worker.exporter == nil || s.worker.exporter.exportDB == nil || id == "" {
		return fmt.Errorf("分享调度导出未初始化或任务ID为空")
	}
	if requested != "incremental" && requested != "reconciliation" {
		return fmt.Errorf("分享导出模式无效")
	}
	if err := s.worker.cancelled(ctx, id); err != nil {
		return err
	}
	if err := s.store.CheckSchema(ctx); err != nil {
		return err
	}
	if s.worker.exporter.exportDB.Stats().MaxOpenConnections == 1 {
		return fmt.Errorf("分享调度导出需要至少两个数据库连接")
	}
	input, _, err := s.input(ctx)
	if err != nil {
		return err
	}
	output, err := waitStrmOutput(ctx, s.worker.exporter.exportDB, input.Settings.OutputPath, "share:default", id, func() bool { return s.worker.cancelled(ctx, id) != nil }, false)
	if err != nil {
		return err
	}
	defer output.Store.Close()
	if err = s.store.CheckSchema(ctx); err != nil {
		return err
	}
	input, fingerprint, err := s.input(ctx)
	if err != nil {
		return err
	}
	if output.Root != input.Settings.OutputPath {
		return dao.ErrShareExportChanged
	}
	reason := shareReconciliationReason(input, fingerprint)
	resume := reason == "" && input.BaselineState == "building"
	effective := requested
	if reason != "" || resume {
		effective = "reconciliation"
	}
	metadata := map[string]interface{}{"share_export": true, "incremental": effective == "incremental", "requested_mode": requested, "effective_mode": effective, "fallback_reason": reason, "recovery": "", "phase": "consuming", "output_path": output.Root, "config_revision": input.ConfigRevision}
	if resume {
		metadata["recovery"] = "继续已准备版本的未完成作品，不重复全量或种子"
	}
	if effective == "reconciliation" && !resume {
		if input.BaselineState != "required" {
			if err = s.store.RequireBaseline(ctx); err != nil {
				return err
			}
			input, fingerprint, err = s.input(ctx)
			if err != nil {
				return err
			}
			if input.Settings.OutputPath != output.Root {
				return dao.ErrShareExportChanged
			}
		}
		metadata["config_revision"] = input.ConfigRevision
		metadata["recovery"] = "全量准备；未准备完成的中断会在下次调度重试"
		metadata["phase"] = "preparing"
	}
	if tasks := s.worker.exporter.tasks; tasks != nil {
		task, taskErr := tasks.Get(id)
		if taskErr != nil {
			return taskErr
		}
		if task == nil {
			return fmt.Errorf("分享调度任务不存在")
		}
		accumulated, _ := task["metadata"].(map[string]interface{})
		if accumulated == nil {
			accumulated = map[string]interface{}{}
		}
		ctx = context.WithValue(ctx, shareScheduledMetadataContext{}, accumulated)
		if err = s.worker.exporter.updateExportMetadata(ctx, id, metadata); err != nil {
			return err
		}
	}
	ctx = s.worker.exporter.Coordinator().batchContext(ctx, id)
	ctx = context.WithValue(ctx, shareExportOutputContext{}, output)
	if effective == "reconciliation" && !resume {
		if err = s.worker.cancelled(ctx, id); err != nil {
			return err
		}
		if err = s.full(ctx, input, id); err != nil {
			return err
		}
		if tasks := s.worker.exporter.tasks; tasks != nil {
			if err = s.worker.exporter.updateExportMetadata(ctx, id, map[string]interface{}{"phase": "consuming", "prepared_revision": input.ConfigRevision, "recovery": "全量已准备，按作品消费；中断后只恢复未完成待办"}); err != nil {
				return err
			}
		}
	}
	return s.worker.runPrepared(ctx, id, input, fingerprint, output)
}

func (s *ShareStrmScheduledService) reconcile(ctx context.Context, input domain.ShareExportInput, id string) error {
	output, _ := ctx.Value(shareExportOutputContext{}).(*StrmOutput)
	if output == nil {
		return fmt.Errorf("全量对账缺少输出 owner 锁")
	}
	if err := output.prepareSnapshot(ctx); err != nil {
		return err
	}
	fingerprint, err := shareExportFingerprint(input)
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, shareReconciliationCompletionContext{}, func(ctx context.Context) error {
		return s.store.CompleteReconciliation(ctx, input, fingerprint)
	})
	if err = s.worker.exporter.export(ctx, input.Settings, domain.ShareLibraryQuery{}, id); err != nil {
		return err
	}
	if tasks := s.worker.exporter.tasks; tasks != nil {
		unseen := 0
		for key := range output.Snapshot {
			if !output.Seen[key] {
				unseen++
			}
		}
		return s.worker.exporter.updateExportMetadata(ctx, id, map[string]interface{}{"reconciliation_unseen_outputs": unseen})
	}
	return nil
}

// RegisterShareStrmCronHandlers 注册独立增量与每周对账入口；两个处理器在 CronService 内共享互斥。
func RegisterShareStrmCronHandlers(cron *CronService, exporter *ShareStrmService) {
	runner := NewShareStrmScheduledService(exporter, dao.NewShareExportCheckpointDAO(exporter.exportDB))
	for _, mode := range []string{"incremental", "reconciliation"} {
		key, name := "share_strm_incremental_export", "分享库 STRM 增量导出（每日多次）"
		expression := "0 */6 * * *"
		if mode == "reconciliation" {
			key, name = "share_strm_full_reconciliation", "分享库 STRM 全量对账（每周）"
			expression = "0 3 * * 0"
		}
		cron.Register(CronHandler{Key: key, Name: name, DefaultCron: expression, Parameters: []CronParameter{}, Execute: func(ctx context.Context, task *domain.CronTask, id string) (string, error) {
			if err := runner.Run(ctx, mode, id); err != nil {
				return "", err
			}
			return "分享库 STRM 导出完成，实际模式与恢复原因见任务详情", nil
		}})
	}
}
