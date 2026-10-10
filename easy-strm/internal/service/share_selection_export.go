package service

import (
	"context"
	"errors"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type shareSelectionExportStore interface {
	ShareSelectionStore
	ResolveSelection(context.Context, string) (domain.ShareSelection, int, error)
	ResolveSelections(context.Context, string) (int, error)
	GetStrmSource(context.Context, int) (domain.ShareStrmSource, error)
	AnchorSelectionPath(context.Context, string, string) (string, error)
}
type shareSelectionReceiptContext struct{}

func (s *ShareStrmService) prepareSelectionWork(ctx context.Context, store shareSelectionExportStore, work string) error {
	if err := s.preparationStage(ctx, "selection-schema", func(ctx context.Context, _ sharePreparationReport) error {
		return store.CheckSelectionSchema(ctx)
	}); err != nil {
		return err
	}
	if err := s.preparationStage(ctx, "selection-sync", func(ctx context.Context, _ sharePreparationReport) error {
		return store.SyncSelections(ctx, work)
	}); err != nil {
		return err
	}
	err := s.preparationStage(ctx, "selection-resolve", func(ctx context.Context, _ sharePreparationReport) error {
		_, err := store.ResolveSelections(ctx, work)
		return err
	})
	var invalid *dao.ShareSelectionPreparationError
	if id := logger.TaskID(ctx); id != "" && s.tasks != nil && errors.As(err, &invalid) {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err = errors.Join(err, s.updateExportMetadata(persistCtx, id, map[string]interface{}{"selection_preparation_report": invalid, "manual_invalid_count": len(invalid.ManualKeys), "auto_unavailable_count": len(invalid.AutoKeys)}))
	}
	return err
}

type shareSelectedItemContext struct{}
type shareSelectedItem struct {
	selection domain.ShareSelection
	relative  string
}

func (s *ShareStrmService) exportLocalStrm(ctx context.Context, cfg domain.ShareStrmSettings, source domain.ShareStrmSource, cats []*domain.MediaCategory, seen map[string]bool, conflicts ...map[string]*shareStrmConflict) strmExportResult {
	store, ok := s.store.(shareSelectionExportStore)
	if !ok {
		cfg.DedupeExport = true
		return s.exportLocalStrmSource(ctx, cfg, source, cats, seen, conflicts...)
	}
	result := strmExportResult{}
	episodes := source.Episodes
	if source.Result.MediaType == "movie" {
		episodes = []domain.ShareEpisode{{}}
	}
	for _, episode := range episodes {
		if err := ctx.Err(); err != nil {
			return strmExportResult{Written: result.Written, Err: err}
		}
		key := shareStrmIdentity(source, episode)
		if attempt, _ := ctx.Value(shareIncrementalAttemptContext{}).(*shareIncrementalAttempt); attempt != nil {
			attempt.keys[source.ID] = append(attempt.keys[source.ID], key)
		}
		if seen[key] {
			result.SkippedDedupe++
			continue
		}
		selection, sourceID, err := store.ResolveSelection(ctx, key)
		if err != nil {
			return strmExportResult{Written: result.Written, Err: err}
		}
		chosen, err := store.GetStrmSource(ctx, sourceID)
		if err != nil {
			return strmExportResult{Written: result.Written, Err: err}
		}
		if chosen.WorkKey != selection.WorkKey {
			return strmExportResult{Written: result.Written, Err: dao.ErrShareSelectionUnavailable}
		}
		chosen.Result.SeasonNumber = selection.Season
		chosen.Result.EpisodeNumber = selection.Episode
		relative := selection.RelativePath
		if relative == "" {
			relative, err = s.strmRelativePath(chosen, domain.ShareFileInfo{Path: chosen.FileName}, cats)
			if err != nil {
				return strmExportResult{Written: result.Written, Err: err}
			}

		}
		clean := filepath.Clean(relative)
		if filepath.IsAbs(relative) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return strmExportResult{Written: result.Written, Err: fmt.Errorf("持久输出路径越界")}
		}
		relative, err = store.AnchorSelectionPath(ctx, key, relative)
		if err != nil {
			return strmExportResult{Written: result.Written, Err: err}
		}
		clean = filepath.Clean(relative)
		if relative == "" || filepath.IsAbs(relative) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return strmExportResult{Written: result.Written, Err: fmt.Errorf("锚定输出路径越界或为空")}
		}
		selectedCtx := context.WithValue(ctx, shareSelectedItemContext{}, shareSelectedItem{selection: selection, relative: relative})
		if output, _ := ctx.Value(shareExportOutputContext{}).(*StrmOutput); output != nil {
			selectedCtx = context.WithValue(selectedCtx, shareSelectionReceiptContext{}, func(ctx context.Context, state dao.ExportState) error {
				return output.Store.SaveSelectionExport(ctx, state, selection, relative)
			})
		} else {
			return strmExportResult{Written: result.Written, Err: fmt.Errorf("持久来源导出必须配置安全输出凭证")}
		}
		part := s.exportLocalStrmSource(selectedCtx, cfg, chosen, cats, seen)
		result.Written += part.Written
		result.SkippedDedupe += part.SkippedDedupe
		if part.Err != nil {
			result.Err = part.Err
			return result
		}
	}
	return result
}
