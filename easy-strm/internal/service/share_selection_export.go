package service

import (
	"context"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"fmt"
	"path/filepath"
	"strings"
)

type shareSelectionExportStore interface {
	ShareSelectionStore
	ResolveSelection(context.Context, string) (domain.ShareSelection, int, error)
	WorkSelectionKeys(context.Context, string) ([]string, error)
	GetStrmSource(context.Context, int) (domain.ShareStrmSource, error)
	AnchorSelectionPath(context.Context, string, string) (string, error)
}
type shareSelectionReceiptContext struct{}

func (s *ShareStrmService) prepareSelectionWork(ctx context.Context, store shareSelectionExportStore, work string) error {
	if err := store.CheckSelectionSchema(ctx); err != nil {
		return err
	}
	if err := store.SyncSelections(ctx, work); err != nil {
		return err
	}
	keys, err := store.WorkSelectionKeys(ctx, work)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, _, err = store.ResolveSelection(ctx, key); err != nil {
			return err
		}
	}
	return nil
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
