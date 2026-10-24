package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

type selectionStrm115Store interface {
	SelectionStrm115Candidate(context.Context, domain.ShareSelection) (domain.ShareCandidate, error)
	RecheckSelectionStrm115(context.Context, domain.ShareSelection) error
}

type selectionPublishGuardContext struct{}

type selection115Sources interface {
	Strm115SelectionSources(context.Context, domain.ShareLibraryQuery, string) ([]domain.Strm115SelectionSource, error)
}

func (s *ShareStrmService) preflightStrm115Scope(ctx context.Context, query domain.ShareLibraryQuery) error {
	store, ok := s.store.(selection115Sources)
	if !ok {
		return nil
	}
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		values, err := store.Strm115SelectionSources(ctx, query, after)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			return nil
		}
		for _, value := range values {
			if value.Selection.ItemKey <= after {
				return dao.ErrShareExportChanged
			}
			after = value.Selection.ItemKey
		}
	}
}

func (s *ShareStrmService) exportRemainingStrm115(ctx context.Context, cfg domain.ShareStrmSettings, query domain.ShareLibraryQuery, cats []*domain.MediaCategory, seen map[string]bool) strmExportResult {
	store, ok := s.store.(selection115Sources)
	if !ok {
		return strmExportResult{}
	}
	result := strmExportResult{}
	after := ""
	for {
		values, err := store.Strm115SelectionSources(ctx, query, after)
		if err != nil {
			result.Err = err
			return result
		}
		if len(values) == 0 {
			return result
		}
		for _, value := range values {
			if value.Selection.ItemKey <= after {
				result.Err = dao.ErrShareExportChanged
				return result
			}
			after = value.Selection.ItemKey
			if seen[value.Selection.ItemKey] {
				continue
			}
			part := s.exportLocalStrm(ctx, cfg, value.Source, cats, seen)
			result.Written += part.Written
			if part.Err != nil {
				result.Err = part.Err
				return result
			}
		}
	}
}

func (s *ShareStrmService) exportSelectionStrm115(ctx context.Context, cfg domain.ShareStrmSettings, source domain.ShareStrmSource, selected shareSelectedItem, seen map[string]bool) (result strmExportResult) {
	store, ok := s.store.(selectionStrm115Store)
	output, _ := ctx.Value(shareExportOutputContext{}).(*StrmOutput)
	if !ok || output == nil || s.strm115ServerURL == nil {
		return strmExportResult{Err: fmt.Errorf("115目录安全导出未配置")}
	}
	selection := selected.selection
	candidate, err := store.SelectionStrm115Candidate(ctx, selection)
	if err != nil {
		return strmExportResult{Err: err}
	}
	if !candidate.Available {
		return strmExportResult{Err: dao.ErrShareSelectionUnavailable}
	}
	serverURL := s.strm115ServerURL()
	entry, err := domain.BuildStrm115Entry(serverURL, candidate)
	if err != nil {
		return strmExportResult{Err: err}
	}
	filename, err := NormalizeStrmOutputPath(filepath.Join(cfg.OutputPath, selected.relative))
	if err != nil {
		return strmExportResult{Err: err}
	}
	release, err := s.Coordinator().acquire(ctx, nil, shareResource{key: "identity:" + selection.ItemKey, exclusive: true}, shareResource{key: "path:" + filename, exclusive: true})
	if err != nil {
		return strmExportResult{Err: err}
	}
	defer release()
	guard := func(ctx context.Context) error {
		if guard, ok := ctx.Value(shareWorkReexportGuardContext{}).(func(context.Context) error); ok {
			if err := guard(ctx); err != nil {
				return err
			}
		}
		snapshot := selection
		snapshot.RelativePath = selected.relative
		if err := store.RecheckSelectionStrm115(ctx, snapshot); err != nil {
			return err
		}
		current, err := store.SelectionStrm115Candidate(ctx, snapshot)
		if err != nil {
			return err
		}
		if !current.Available || current != candidate || s.strm115ServerURL() != serverURL {
			return dao.ErrShareExportChanged
		}
		return nil
	}
	ctx = context.WithValue(ctx, selectionPublishGuardContext{}, guard)
	if err = guard(ctx); err != nil {
		return strmExportResult{Err: err}
	}
	unlockPath, err := output.Store.LockPath(ctx, filename)
	if err != nil {
		return strmExportResult{Err: err}
	}
	defer unlockPath()
	unlockSelection, err := output.Store.LockSelectionPublication(ctx)
	if err != nil {
		return strmExportResult{Err: err}
	}
	defer func() { result.Err = errors.Join(result.Err, unlockSelection()) }()
	if attempt, _ := ctx.Value(shareIncrementalAttemptContext{}).(*shareIncrementalAttempt); attempt != nil {
		if err = attempt.record(ctx, selection.ItemKey); err != nil {
			return strmExportResult{Err: err}
		}
	} else if recovery, ok := ctx.Value(shareExportRecoveryContext{}).(*dao.ShareExportCheckpointDAO); ok {
		if err = recovery.RecordExternalPlannedKeys(ctx, source.WorkKey, []string{selection.ItemKey}); err != nil {
			return strmExportResult{Err: err}
		}
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return strmExportResult{Err: err}
	}
	changed, err := output.Write(ctx, selection.ItemKey, filename, entry.Content, string(raw), "")
	result = strmExportResult{Err: err}
	if changed {
		result.Written = 1
	}
	if err == nil {
		seen[selection.ItemKey] = true
	}
	return result
}
