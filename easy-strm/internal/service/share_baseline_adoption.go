package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"github.com/google/uuid"
)

// ShareBaselineAdoptionStore 限定人工采纳所需的检查、输入快照和原子记账能力。
type ShareBaselineAdoptionStore interface {
	CheckSchema(context.Context) error
	ReadInput(context.Context) (domain.ShareExportInput, error)
	AdoptBaseline(context.Context, domain.ShareBaselineAdoptionRequest, string, string, ...func() error) (domain.ShareBaselineAdoptionResult, error)
}

// ValidateShareBaselineAdoption 要求逐项明确的快照护栏、确认短语和 DBA 核验引用。
func ValidateShareBaselineAdoption(request domain.ShareBaselineAdoptionRequest) error {
	if request.TimeoutMinutes != nil {
		if err := validateShareBaselineTimeout(*request.TimeoutMinutes); err != nil {
			return err
		}
	}
	if request.ManualConfirmation != "人工确认采纳" || strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.DBAAttestation) == "" || len(request.Reason) > 2000 || len(request.DBAAttestation) > 500 {
		return fmt.Errorf("必须明确填写人工确认采纳、理由及 DBA 独立核验引用（理由≤2000字节，引用≤500字节）")
	}
	if !filepath.IsAbs(request.ExpectedOutputPath) || filepath.Clean(request.ExpectedOutputPath) != request.ExpectedOutputPath || request.ExpectedOutputPath == string(filepath.Separator) {
		return fmt.Errorf("必须填写规范的绝对 expected_output_path，禁止根目录")
	}
	keys := []string{"config_revision", "prepared_revision", "completed_revision", "selection_count", "output_count", "pending_count", "dirty_revision_sum", "selection_revision_sum"}
	if len(request.Expected) != len(keys) {
		return fmt.Errorf("expected 必须包含全部八个计数与版本护栏，不接受额外字段")
	}
	for _, key := range keys {
		value, present := request.Expected[key]
		if !present || value < 0 {
			return fmt.Errorf("expected.%s 必须明确提供非负整数", key)
		}
	}
	if request.Expected["config_revision"] == 0 || request.Expected["prepared_revision"] != request.Expected["config_revision"] || request.Expected["completed_revision"] > request.Expected["prepared_revision"] || request.Expected["selection_count"] == 0 || request.Expected["output_count"] != request.Expected["selection_count"] || request.Expected["selection_revision_sum"] < request.Expected["selection_count"] || request.Expected["dirty_revision_sum"] < request.Expected["pending_count"] {
		return fmt.Errorf("expected 计数与版本关系不自洽，请重新独立核验")
	}
	if request.ActorUserID <= 0 {
		return fmt.Errorf("采纳必须由已认证用户明确确认")
	}
	return nil
}

func runShareBaselineAdoption(ctx context.Context, store ShareBaselineAdoptionStore, request domain.ShareBaselineAdoptionRequest, id string, beforeCommit ...func() error) (result domain.ShareBaselineAdoptionResult, runErr error) {
	ctx = logger.WithTaskID(ctx, id)
	entry := logger.WithContext(ctx, "share_baseline_adoption")
	if err := ValidateShareBaselineAdoption(request); err != nil {
		return result, err
	}
	entry.Log(logger.INFO, "人工确认采纳：开始校验基线", logger.Fields{"event": "manual_baseline_adoption", "reason": request.Reason, "dba_attestation": request.DBAAttestation, "confirmed_by": request.ActorUserID, "expected": request.Expected}, nil)
	defer func() {
		level := logger.INFO
		if runErr != nil {
			level = logger.ERROR
		}
		entry.Log(level, "人工确认采纳：操作结束", logger.Fields{"event": "manual_baseline_adoption_result", "result": result}, runErr)
	}()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := store.CheckSchema(ctx); err != nil {
		return result, err
	}
	input, err := store.ReadInput(ctx)
	if err != nil {
		return result, err
	}
	if err = validateShareStrmSettings(&input.Settings); err != nil {
		return result, err
	}
	input.Settings.OutputPath = filepath.Clean(input.Settings.OutputPath)
	if input.Settings.OutputPath != request.ExpectedOutputPath || input.ConfigRevision != request.Expected["config_revision"] || input.PreparedRevision != request.Expected["prepared_revision"] {
		return result, dao.ErrShareExportChanged
	}
	fingerprint, err := shareExportFingerprint(input)
	if err != nil {
		return result, err
	}
	return store.AdoptBaseline(ctx, request, fingerprint, id, beforeCommit...)
}

// StartBaselineAdoptionContext 创建仅显式人工触发的审计任务；不注册定时任务或自动重试。
func (exporter *ShareStrmService) StartBaselineAdoptionContext(parent context.Context, request domain.ShareBaselineAdoptionRequest) (string, error) {
	if err := ValidateShareBaselineAdoption(request); err != nil {
		return "", err
	}
	if exporter == nil || exporter.tasks == nil || exporter.exportDB == nil {
		return "", fmt.Errorf("采纳任务服务未初始化")
	}
	return exporter.startBaselineAdoptionContext(parent, request, dao.NewShareExportCheckpointDAO(exporter.exportDB))
}

func (exporter *ShareStrmService) startBaselineAdoptionContext(parent context.Context, request domain.ShareBaselineAdoptionRequest, store ShareBaselineAdoptionStore) (string, error) {
	minutes, err := exporter.baselineAdoptionTimeout(parent, request)
	if err != nil {
		return "", err
	}
	if request.TimeoutMinutes != nil {
		override := *request.TimeoutMinutes
		request.TimeoutMinutes = &override
	}
	expected := make(map[string]int64, len(request.Expected))
	for key, value := range request.Expected {
		expected[key] = value
	}
	request.Expected = expected
	generated, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	id := "share_baseline_adopt_" + generated.String()
	parent = logger.WithTaskID(context.WithoutCancel(parent), id)
	if err = exporter.tasks.CreateContext(parent, id, "share_baseline_adopt", "人工确认采纳分享 STRM 基线"); err != nil {
		return "", err
	}
	metadata := map[string]interface{}{"manual_confirmation": "人工确认采纳", "adoption_request": request, "confirmed_by": request.ActorUserID, "timeout_minutes": minutes}
	if err = exporter.tasks.UpdateMetadataContext(parent, id, metadata); err != nil {
		_ = exporter.tasks.SetErrorContext(parent, id, err.Error())
		return "", err
	}
	ctx, cancel := newShareTaskContext(parent, minutes)
	var finalizationMutex sync.Mutex
	finalizing := false
	exporter.tasks.RegisterCancelGuard(id, func() error {
		finalizationMutex.Lock()
		defer finalizationMutex.Unlock()
		if finalizing {
			return fmt.Errorf("人工采纳已进入提交阶段，不能取消；请查看任务结果与持久审计凭证")
		}
		cancel()
		return nil
	})
	beginCommit := func() error {
		finalizationMutex.Lock()
		defer finalizationMutex.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		finalizing = true
		return nil
	}
	exporter.tasks.RegisterCancel(id, cancel)
	go func() {
		defer cancel()
		defer exporter.tasks.RemoveCancelGuard(id)
		defer exporter.tasks.RemoveCancel(id)
		terminal := context.WithoutCancel(ctx)
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = exporter.tasks.SetErrorContext(terminal, id, fmt.Sprint(recovered))
			}
		}()
		if err := exporter.tasks.UpdateStatusContext(ctx, id, "running"); err != nil {
			_ = exporter.tasks.SetErrorContext(terminal, id, err.Error())
			return
		}
		if exporter.tasks.IsCancelledContext(ctx, id) {
			_ = exporter.tasks.UpdateStatusContext(terminal, id, "cancelled")
			return
		}
		result, err := runShareBaselineAdoption(ctx, store, request, id, beginCommit)
		if err != nil {
			if ctx.Err() != nil {
				_ = exporter.tasks.UpdateStatusContext(terminal, id, "cancelled")
			} else {
				_ = exporter.tasks.SetErrorContext(terminal, id, err.Error())
			}
			return
		}
		metadata["baseline_adoption"] = result
		if err = exporter.tasks.UpdateMetadataContext(terminal, id, metadata); err != nil {
			message := "采纳已提交，结果显示失败；请按任务 ID 查询持久审计凭证："
			if result.AlreadyReady {
				message = "基线已 ready，本次未消费待办，结果显示失败；请查询先前的实际采纳凭证："
			}
			_ = exporter.tasks.SetErrorContext(terminal, id, message+err.Error())
			return
		}
		if err = exporter.tasks.UpdateStatusContext(terminal, id, "completed"); err != nil {
			message := "采纳已提交，任务状态更新失败："
			if result.AlreadyReady {
				message = "基线已 ready，本次未消费待办，任务状态更新失败："
			}
			_ = exporter.tasks.SetErrorContext(terminal, id, message+err.Error())
		}
	}()
	return id, nil
}

const shareBaselineTimeoutKey = "share_strm_baseline_adoption_timeout_minutes"
const defaultShareBaselineTimeoutMinutes = 20
const maxShareBaselineTimeoutMinutes = 43200

func validateShareBaselineTimeout(minutes int) error {
	if minutes < 0 || minutes > maxShareBaselineTimeoutMinutes {
		return fmt.Errorf("人工采纳 timeout_minutes 需为0至43200的整数；0表示不设总时限")
	}
	return nil
}

func (exporter *ShareStrmService) baselineAdoptionTimeout(ctx context.Context, request domain.ShareBaselineAdoptionRequest) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	minutes := defaultShareBaselineTimeoutMinutes
	if request.TimeoutMinutes != nil {
		minutes = *request.TimeoutMinutes
	} else if exporter.settings != nil {
		value, present, err := exporter.readBaselineTimeoutSetting(ctx)
		if err != nil {
			return 0, err
		}
		if present {
			minutes, err = strconv.Atoi(value)
			if err != nil {
				return 0, fmt.Errorf("人工采纳超时配置无效：%w", err)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return minutes, validateShareBaselineTimeout(minutes)
}

func (exporter *ShareStrmService) readBaselineTimeoutSetting(ctx context.Context) (string, bool, error) {
	if reader, supported := exporter.settings.(interface {
		GetHotConfigs(context.Context, ...string) (map[string]string, error)
	}); supported {
		values, err := reader.GetHotConfigs(ctx, shareBaselineTimeoutKey)
		value, present := values[shareBaselineTimeoutKey]
		return value, present, err
	}
	row, err := exporter.settings.GetByKey(shareBaselineTimeoutKey)
	if row == nil || err != nil {
		return "", false, err
	}
	return row.ConfigVal, true, nil
}
