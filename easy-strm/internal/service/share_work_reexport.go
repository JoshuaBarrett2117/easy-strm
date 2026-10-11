package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

type shareWorkReexportStore interface {
	CheckSelectionSchema(context.Context) error
	WorkReexportSelections(context.Context, string) ([]domain.ShareWorkReexportSelection, error)
	GetStrmSource(context.Context, int) (domain.ShareStrmSource, error)
}

type shareWorkReexportCheckpointStore interface {
	CheckSchema(context.Context) error
	ReadInput(context.Context) (domain.ShareExportInput, error)
	WorkReexportGlobalInvalid(context.Context, string) (int64, error)
	WorkReexportPreflight(context.Context, string, []string, []domain.ShareStrmEntry, []domain.ShareSelection) error
}

type shareWorkReexportGuardContext struct{}

type shareWorkReexportRunner struct {
	exporter    *ShareStrmService
	checkpoints shareWorkReexportCheckpointStore
	openOutput  func(context.Context, string, string) (*StrmOutput, error)
}

// ValidateShareWorkReexport 要求明确有界且不重复的作品键、审计理由与配置护栏，不归一化为空或全库查询。
func ValidateShareWorkReexport(request domain.ShareWorkReexportRequest) error {
	if len(request.WorkKeys) == 0 || len(request.WorkKeys) > domain.ShareWorkReexportMaxWorks {
		return fmt.Errorf("work_keys 必须明确包含 1 至 %d 个作品", domain.ShareWorkReexportMaxWorks)
	}
	seen := map[string]bool{}
	for _, work := range request.WorkKeys {
		if work == "" || work != strings.TrimSpace(work) || len(work) > 200 || !utf8.ValidString(work) || work == "*" || strings.IndexFunc(work, unicode.IsControl) >= 0 || seen[work] {
			return fmt.Errorf("作品键不能为空、重复、通配符、含控制字符或超过 200 字节")
		}
		seen[work] = true
	}
	if strings.TrimSpace(request.Reason) == "" || len(request.Reason) > 2000 || request.ActorUserID <= 0 {
		return fmt.Errorf("重导必须由认证用户填写非空审计理由，最多 2000 字节")
	}
	if request.ExpectedConfigRevision <= 0 || !filepath.IsAbs(request.ExpectedOutputPath) || filepath.Clean(request.ExpectedOutputPath) != request.ExpectedOutputPath || request.ExpectedOutputPath == string(filepath.Separator) {
		return fmt.Errorf("必须填写正数配置版本与规范的绝对输出目录，禁止根目录")
	}
	return nil
}

func (runner *shareWorkReexportRunner) input(ctx context.Context, request domain.ShareWorkReexportRequest) (domain.ShareExportInput, error) {
	input, err := runner.checkpoints.ReadInput(ctx)
	if err != nil {
		return input, err
	}
	if input.ProtocolVersion != 1 || (input.BaselineState != "required" && input.BaselineState != "building" && input.BaselineState != "ready") {
		return input, fmt.Errorf("分享检查点协议或阶段无效")
	}
	if err = validateShareStrmSettings(&input.Settings); err != nil {
		return input, err
	}
	input.Settings.OutputPath, err = NormalizeStrmOutputPath(input.Settings.OutputPath)
	if err != nil {
		return input, err
	}
	if input.ConfigRevision != request.ExpectedConfigRevision || input.Settings.OutputPath != request.ExpectedOutputPath {
		return input, dao.ErrShareExportChanged
	}
	return input, nil
}

func validateWorkReexportSelection(work string, value domain.ShareWorkReexportSelection, source domain.ShareStrmSource) error {
	selection := value.Selection
	relative := selection.RelativePath
	clean := filepath.Clean(relative)
	if selection.WorkKey != work || !selection.Valid || selection.CandidateID <= 0 || selection.Revision <= 0 || (selection.Mode != "auto" && selection.Mode != "manual") || source.ID != value.SourceID || value.SourceID <= 0 || source.MediaID != value.MediaID || value.MediaID <= 0 || source.WorkKey != work || !source.Available {
		return dao.ErrShareSelectionUnavailable
	}
	if relative == "" || relative != clean || filepath.IsAbs(relative) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.ContainsAny(relative, "\\\x00\r\n") || filepath.Ext(relative) != ".strm" {
		return fmt.Errorf("作品 %s 的既有稳定路径为空或不安全，禁止创建新路径", work)
	}
	if selection.ItemKey != shareStrmIdentity(source, domain.ShareEpisode{SeasonNumber: selection.Season, EpisodeNumber: selection.Episode}) {
		return dao.ErrShareExportChanged
	}
	if source.Result.MediaType == "movie" && selection.Season == 0 && selection.Episode == 0 {
		return nil
	}
	if source.Result.MediaType == "tv" {
		for _, episode := range source.Episodes {
			if episode.SeasonNumber == selection.Season && episode.EpisodeNumber == selection.Episode {
				return nil
			}
		}
	}
	return dao.ErrShareSelectionUnavailable
}

func (runner *shareWorkReexportRunner) run(ctx context.Context, request domain.ShareWorkReexportRequest, id string) (result domain.ShareWorkReexportResult, runErr error) {
	result = domain.ShareWorkReexportResult{RequestedWorks: append([]string(nil), request.WorkKeys...), RequestedWorkCount: len(request.WorkKeys), ProcessedWorks: []string{}, Works: []domain.ShareWorkReexportWorkResult{}, InvalidScope: "global"}
	if err := ValidateShareWorkReexport(request); err != nil {
		return result, err
	}
	if runner == nil || runner.exporter == nil {
		return result, fmt.Errorf("作品重导服务未初始化")
	}
	exporter := runner.exporter
	store, ok := exporter.store.(shareWorkReexportStore)
	if !ok || runner.checkpoints == nil || runner.openOutput == nil || id == "" {
		return result, fmt.Errorf("作品重导服务未初始化")
	}
	ctx = logger.WithTaskID(exporter.Coordinator().batchContext(ctx, id), id)
	ctx = context.WithValue(ctx, shareScheduledMetadataContext{}, map[string]interface{}{"share_export": true, "requested_mode": "work_reexport", "effective_mode": "work_reexport", "work_reexport_request": request, "confirmed_by": request.ActorUserID})
	persist := func(persistCtx context.Context) error {
		return exporter.updateExportMetadata(persistCtx, id, map[string]interface{}{"work_reexport": result})
	}
	defer func() {
		if runErr != nil {
			result.Error = runErr.Error()
		}
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		runErr = errors.Join(runErr, persist(persistCtx))
	}()
	if err := runner.cancelled(ctx, id); err != nil {
		return result, err
	}
	if err := persist(ctx); err != nil {
		return result, err
	}
	if err := runner.checkpoints.CheckSchema(ctx); err != nil {
		return result, err
	}
	if err := store.CheckSelectionSchema(ctx); err != nil {
		return result, err
	}
	input, err := runner.input(ctx, request)
	if err != nil {
		return result, err
	}
	result.BaselineState = input.BaselineState
	fingerprint, err := shareExportFingerprint(input)
	if err != nil {
		return result, err
	}
	output, err := runner.openOutput(ctx, input.Settings.OutputPath, id)
	if err != nil {
		return result, err
	}
	defer output.Store.Close()
	defer func() { result.Written = output.Added + output.Updated }()
	ctx = context.WithValue(ctx, shareExportOutputContext{}, output)
	counts := runner.checkpoints
	if checkpoint, ok := counts.(*dao.ShareExportCheckpointDAO); ok {
		counts = checkpoint.ForOutputConnection(output.Store.Conn)
	}
	before, err := counts.WorkReexportGlobalInvalid(ctx, output.Root)
	if err != nil {
		return result, err
	}
	result.GlobalInvalidBefore = &before
	defer func() {
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		after, countErr := counts.WorkReexportGlobalInvalid(readCtx, output.Root)
		if countErr != nil {
			result.AfterCountError = countErr.Error()
			runErr = errors.Join(runErr, countErr)
		} else {
			result.GlobalInvalidAfter = &after
		}
	}()
	prepared := make([][]domain.ShareWorkReexportSelection, len(request.WorkKeys))
	entries := []domain.ShareStrmEntry{}
	selections := []domain.ShareSelection{}
	sources := map[string]domain.ShareStrmSource{}
	paths := map[string]bool{}
	for index, work := range request.WorkKeys {
		if err = runner.cancelled(ctx, id); err != nil {
			return result, err
		}
		values, readErr := store.WorkReexportSelections(ctx, work)
		if readErr != nil {
			return result, readErr
		}
		result.Works = append(result.Works, domain.ShareWorkReexportWorkResult{WorkKey: work, SelectionRows: len(values), Status: "pending"})
		result.SelectionRows += len(values)
		if len(values) == 0 || result.SelectionRows > domain.ShareWorkReexportMaxSelections {
			return result, fmt.Errorf("作品 %s 无选择行或请求超过 %d 行上限，禁止回退扫描", work, domain.ShareWorkReexportMaxSelections)
		}
		for _, value := range values {
			source, sourceErr := store.GetStrmSource(ctx, value.SourceID)
			if sourceErr != nil {
				return result, sourceErr
			}
			if err = validateWorkReexportSelection(work, value, source); err != nil {
				return result, err
			}
			entry, entryErr := domain.BuildShareStrmEntry(source)
			if entryErr != nil {
				return result, entryErr
			}
			entries = append(entries, entry)
			selections = append(selections, value.Selection)
			if paths[value.Selection.RelativePath] {
				return result, fmt.Errorf("请求中多个选择共享同一稳定路径，禁止覆盖")
			}
			paths[value.Selection.RelativePath] = true
			sources[value.Selection.ItemKey] = source
		}
		prepared[index] = values
	}
	if err = counts.WorkReexportPreflight(ctx, output.Root, request.WorkKeys, entries, selections); err != nil {
		return result, err
	}
	for index := range result.Works {
		result.Works[index].Status = "running"
	}
	if err = persist(ctx); err != nil {
		return result, err
	}
	if err = runner.reexportMetadata(ctx, request, id, store, fingerprint, prepared, sources, output); err != nil {
		for index := range result.Works {
			result.Works[index].Status, result.Works[index].Error = "failed", err.Error()
		}
		return result, err
	}
	for index, values := range prepared {
		workResult := &result.Works[index]
		workResult.Status, workResult.ProcessedRows = "completed", len(values)
		result.ProcessedRows += len(values)
		result.ProcessedWorks = append(result.ProcessedWorks, workResult.WorkKey)
		result.ProcessedWorkCount++
	}
	return result, runner.cancelled(ctx, id)
}

func (runner *shareWorkReexportRunner) cancelled(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if runner.exporter.tasks != nil && runner.exporter.tasks.IsCancelledContext(ctx, id) {
		return context.Canceled
	}
	return nil
}

func (runner *shareWorkReexportRunner) reexportMetadata(ctx context.Context, request domain.ShareWorkReexportRequest, id string, store shareWorkReexportStore, fingerprint string, prepared [][]domain.ShareWorkReexportSelection, sources map[string]domain.ShareStrmSource, output *StrmOutput) error {
	targets := []dao.WorkReexportTarget{}
	snapshots := []workReexportFile{}
	resources := []shareResource{}
	shares := []int{}
	for _, values := range prepared {
		for _, value := range values {
			source := sources[value.Selection.ItemKey]
			filename, err := NormalizeStrmOutputPath(filepath.Join(output.Root, value.Selection.RelativePath))
			if err != nil || !output.contains(filename) {
				return errors.Join(err, dao.ErrShareExportChanged)
			}
			file, err := readWorkReexportFile(filename)
			if err != nil {
				return err
			}
			targets = append(targets, dao.WorkReexportTarget{Selection: value.Selection, Source: source, URLMappingID: file.mapping})
			snapshots = append(snapshots, file)
			entry, err := domain.BuildShareStrmEntry(source)
			if err != nil {
				return err
			}
			shares = append(shares, source.ShareID)
			resources = append(resources, shareResource{key: shareKey(source.ShareID)}, shareResource{key: fmt.Sprintf("share-config:%d", source.ShareID)}, shareResource{key: fileKey(source.ID), exclusive: true}, shareResource{key: "mapping-code:" + entry.ShareCode}, shareResource{key: "mapping:" + entry.ID, exclusive: true}, shareResource{key: "mapping:" + file.mapping, exclusive: true}, shareResource{key: "identity:" + value.Selection.ItemKey, exclusive: true}, shareResource{key: "path:" + filename, exclusive: true})
		}
	}
	release, err := runner.exporter.Coordinator().acquire(ctx, shares, resources...)
	if err != nil {
		return err
	}
	defer release()
	for _, snapshot := range snapshots {
		unlock, err := output.Store.LockPath(ctx, snapshot.path)
		if err != nil {
			return err
		}
		defer unlock()
	}
	guard := func(ctx context.Context) error {
		if err := runner.cancelled(ctx, id); err != nil {
			return err
		}
		latest, err := runner.input(ctx, request)
		if err != nil {
			return err
		}
		current, err := shareExportFingerprint(latest)
		if err != nil || current != fingerprint {
			return errors.Join(err, dao.ErrShareExportChanged)
		}
		for workIndex, expectedWork := range prepared {
			values, err := store.WorkReexportSelections(ctx, request.WorkKeys[workIndex])
			if err != nil {
				return err
			}
			if len(values) != len(expectedWork) {
				return dao.ErrShareExportChanged
			}
			for index, value := range values {
				value.Selection.ExportedRevision = expectedWork[index].Selection.ExportedRevision
				if !reflect.DeepEqual(value, expectedWork[index]) {
					return dao.ErrShareExportChanged
				}
				fresh, err := store.GetStrmSource(ctx, value.SourceID)
				if err != nil {
					return err
				}
				if !reflect.DeepEqual(fresh, sources[value.Selection.ItemKey]) {
					return dao.ErrShareExportChanged
				}
			}
		}
		return nil
	}
	if err := guard(ctx); err != nil {
		return err
	}
	verify := func(ctx context.Context, receipts []dao.WorkReexportReceipt) error {
		if err := guard(ctx); err != nil {
			return err
		}
		for index, receipt := range receipts {
			file, err := readWorkReexportFile(snapshots[index].path)
			if err != nil {
				return err
			}
			if !os.SameFile(file.info, snapshots[index].info) || !file.info.ModTime().Equal(snapshots[index].info.ModTime()) || file.info.Mode() != snapshots[index].info.Mode() || !bytes.Equal(file.content, snapshots[index].content) || receipt.State.Path != file.path || receipt.State.Content != fmt.Sprintf("%x", sha256.Sum256(append([]byte(file.path+"\x00"), file.content...))) {
				return fmt.Errorf("文件与旧 state 凭证不一致 export_key=%q：%w", receipt.State.Key, dao.ErrShareExportChanged)
			}
			if !sameWorkReexportFile(receipt.Mapping, receipt.Entry) || !sameWorkReexportFile(receipt.URLMapping, receipt.Entry) || receipt.URLMapping.ID != file.mapping {
				return fmt.Errorf("旧 URL / mapping 与选中候选不是同一分享文件 export_key=%q：%w", receipt.State.Key, dao.ErrShareExportChanged)
			}
		}
		return nil
	}
	if err = output.Store.WorkReexportMetadata(ctx, output.Root, request.WorkKeys, request.ExpectedConfigRevision, id, targets, verify); err != nil {
		return err
	}
	for _, target := range targets {
		output.Seen[target.Selection.ItemKey] = true
	}
	return nil
}

type workReexportFile struct {
	path, mapping string
	content       []byte
	info          os.FileInfo
}

func readWorkReexportFile(filename string) (workReexportFile, error) {
	file := workReexportFile{path: filename}
	var err error
	file.info, err = os.Lstat(filename)
	if err != nil || !file.info.Mode().IsRegular() {
		return file, fmt.Errorf("重导要求既有普通 STRM 文件 %s：%w", filename, errors.Join(err, dao.ErrShareExportChanged))
	}
	if file.info.Size() > 8192 {
		return file, fmt.Errorf("STRM 超过播放 URL 长度上限：%w", dao.ErrShareExportChanged)
	}
	file.content, err = os.ReadFile(filename)
	if err != nil {
		return file, errors.Join(err, dao.ErrShareExportChanged)
	}
	parsed, err := url.Parse(strings.TrimSuffix(string(file.content), "\n"))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return file, fmt.Errorf("旧 STRM 不是可验证的分享播放 URL：%w", dao.ErrShareExportChanged)
	}
	marker := strings.LastIndex(parsed.Path, "/share-strm/")
	if marker < 0 {
		return file, dao.ErrShareExportChanged
	}
	file.mapping = parsed.Path[marker+len("/share-strm/"):]
	if _, err = uuid.Parse(file.mapping); err != nil {
		return file, fmt.Errorf("旧播放 ID 无效：%w", dao.ErrShareExportChanged)
	}
	return file, nil
}

func sameWorkReexportFile(previous, next domain.ShareStrmEntry) bool {
	return previous.ShareCode == next.ShareCode && previous.Password == next.Password && previous.FilePath == next.FilePath && previous.FileName == next.FileName && previous.FileID == next.FileID
}

// StartWorkReexportContext 创建显式有界的人工重导任务，复用任务取消与 owner 锁，不授权采纳或全量。
func (exporter *ShareStrmService) StartWorkReexportContext(parent context.Context, request domain.ShareWorkReexportRequest) (string, error) {
	if err := ValidateShareWorkReexport(request); err != nil {
		return "", err
	}
	if exporter == nil || exporter.tasks == nil || exporter.exportDB == nil || exporter.exportDB.Stats().MaxOpenConnections == 1 {
		return "", fmt.Errorf("作品重导需要已初始化的任务与至少两个数据库连接")
	}
	if _, ok := exporter.store.(shareWorkReexportStore); !ok {
		return "", fmt.Errorf("作品重导来源读取未初始化")
	}
	request.WorkKeys = append([]string(nil), request.WorkKeys...)
	generated, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	id := "share_work_reexport_" + generated.String()
	parent = logger.WithTaskID(context.WithoutCancel(parent), id)
	if err = exporter.tasks.CreateContext(parent, id, "strm_generate", "分享 STRM 显式作品重导"); err != nil {
		return "", err
	}
	metadata := map[string]interface{}{"share_export": true, "requested_mode": "work_reexport", "effective_mode": "work_reexport", "work_reexport_request": request, "confirmed_by": request.ActorUserID}
	if err = exporter.tasks.UpdateMetadataContext(parent, id, metadata); err != nil {
		_ = exporter.tasks.SetErrorContext(parent, id, err.Error())
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	exporter.tasks.RegisterCancel(id, cancel)
	go func() {
		defer cancel()
		defer exporter.tasks.RemoveCancel(id)
		var runErr error
		started := false
		defer func() {
			terminal, terminalCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer terminalCancel()
			if recovered := recover(); recovered != nil {
				_ = exporter.tasks.SetErrorContext(terminal, id, fmt.Sprint(recovered))
				return
			}
			if started && (ctx.Err() != nil || exporter.tasks.IsCancelledContext(terminal, id)) {
				_ = exporter.tasks.UpdateStatusContext(terminal, id, "cancelled")
			} else if runErr != nil {
				_ = exporter.tasks.SetErrorContext(terminal, id, runErr.Error())
			} else if err := exporter.tasks.UpdateStatusContext(terminal, id, "completed"); err != nil {
				_ = exporter.tasks.SetErrorContext(terminal, id, err.Error())
			}
		}()
		if runErr = exporter.tasks.UpdateStatusContext(ctx, id, "running"); runErr != nil {
			return
		}
		started = true
		runner := &shareWorkReexportRunner{exporter: exporter, checkpoints: dao.NewShareExportCheckpointDAO(exporter.exportDB), openOutput: func(ctx context.Context, root, taskID string) (*StrmOutput, error) {
			return waitStrmOutput(ctx, exporter.exportDB, root, "share:default", taskID, func() bool { return exporter.tasks.IsCancelledContext(ctx, taskID) }, false)
		}}
		_, runErr = runner.run(ctx, request, id)
	}()
	return id, nil
}
