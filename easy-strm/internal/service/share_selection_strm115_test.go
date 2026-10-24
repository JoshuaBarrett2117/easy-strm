package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type selection115Memory struct {
	*selectedFilesystemStore
	candidate  domain.ShareCandidate
	guardErr   error
	reads      int
	guardCalls int
	onGuard    func(int) error
	metadata   domain.ShareStrmSource
	queries    []domain.ShareLibraryQuery
}

func (store *selection115Memory) SelectionStrm115Candidate(context.Context, domain.ShareSelection) (domain.ShareCandidate, error) {
	store.reads++
	if !store.candidate.Available {
		return domain.ShareCandidate{}, dao.ErrShareSelectionUnavailable
	}
	return store.candidate, nil
}
func (store *selection115Memory) RecheckSelectionStrm115(context.Context, domain.ShareSelection) error {
	store.guardCalls++
	if store.onGuard != nil {
		return store.onGuard(store.guardCalls)
	}
	return store.guardErr
}

func (store *selection115Memory) Strm115SelectionSources(_ context.Context, query domain.ShareLibraryQuery, after string) ([]domain.Strm115SelectionSource, error) {
	store.queries = append(store.queries, query)
	if store.metadata.WorkKey == "" || after != "" {
		return nil, nil
	}
	if query.WorkKey != "" && query.WorkKey != store.selection.WorkKey {
		return nil, nil
	}
	if !store.candidate.Available {
		return nil, dao.ErrShareSelectionUnavailable
	}
	return []domain.Strm115SelectionSource{{Selection: store.selection, Source: store.metadata}}, nil
}

func TestSelection115SwapSamePathNoShareMappingOrFallback(t *testing.T) {
	source := filesystemSource(1, 7, 100, filesystemEpisode(1, 1))
	store := &selection115Memory{selectedFilesystemStore: &selectedFilesystemStore{strmMemory: &strmMemory{sources: []domain.ShareStrmSource{source}, entries: map[string]domain.ShareStrmEntry{}}, selection: domain.ShareSelection{ItemKey: shareStrmIdentity(source, source.Episodes[0]), WorkKey: source.WorkKey, Season: 1, Episode: 1, CandidateID: 11, SourceKind: "share", Revision: 1, Mode: "manual", Valid: true, RelativePath: "tv/既有路径.strm"}, chosen: 1}, candidate: domain.ShareCandidate{SourceKind: "strm115", ExternalRef: "0:5", Cloud115ID: 9, CloudPath: "/日番/剧/S01E01.mkv", Available: true}}
	service := NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, nil, nil, nil, nil)
	service.SetStrm115ServerURL(func() string { return "https://server.example.test" })
	database, mock, _ := sqlmock.New()
	defer database.Close()
	connection, err := database.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	root := t.TempDir()
	output := &StrmOutput{Store: &dao.StrmExportDAO{Conn: connection}, Root: root, Owner: "share:default", Run: "fixture", Paths: map[string]bool{}}
	ctx := context.WithValue(context.Background(), shareExportOutputContext{}, output)
	cfg := domain.ShareStrmSettings{OutputPath: root, BaseURL: "https://media.example.test"}
	expectSelectedFile(t, mock, store.selection, false, false)
	if result := service.exportLocalStrm(ctx, cfg, source, nil, map[string]bool{}); result.Err != nil || result.Written != 1 {
		t.Fatalf("%+v", result)
	}
	shareFiles := filesystemCollection(t, root)
	if len(shareFiles) != 1 || string(shareFiles[store.selection.RelativePath]) != string(filesystemContent(source)) {
		t.Fatal(shareFiles)
	}
	entriesBefore := len(store.entries)
	filesBefore := len(store.files)
	store.selection.SourceKind = "strm115"
	store.selection.ExternalRef = "0:5"
	store.selection.CandidateID = 0
	store.selection.Revision++
	store.chosen = 0
	expectSelectedFile(t, mock, store.selection, true, false)
	if result := service.exportLocalStrm(ctx, cfg, source, nil, map[string]bool{}); result.Err != nil || result.Written != 1 {
		t.Fatalf("%+v", result)
	}
	directoryFiles := filesystemCollection(t, root)
	entry, err := domain.BuildStrm115Entry("https://server.example.test", store.candidate)
	if err != nil || len(directoryFiles) != 1 || string(directoryFiles[store.selection.RelativePath]) != entry.Content || len(store.entries) != entriesBefore || len(store.files) != filesBefore {
		t.Fatalf("%v %v", directoryFiles, err)
	}
	store.candidate.Available = false
	if result := service.exportLocalStrm(ctx, cfg, source, nil, map[string]bool{}); !errors.Is(result.Err, dao.ErrShareSelectionUnavailable) || result.Written != 0 {
		t.Fatalf("%+v", result)
	}
	assertFilesystemCollection(t, root, directoryFiles)
	store.candidate.Available = true
	store.guardErr = dao.ErrShareExportChanged
	if result := service.exportLocalStrm(ctx, cfg, source, nil, map[string]bool{}); !errors.Is(result.Err, dao.ErrShareExportChanged) || result.Written != 0 {
		t.Fatalf("%+v", result)
	}
	assertFilesystemCollection(t, root, directoryFiles)
	store.guardErr = nil
	store.selection.SourceKind = "unknown"
	if result := service.exportLocalStrm(ctx, cfg, source, nil, map[string]bool{}); result.Err == nil {
		t.Fatal("unknown source accepted")
	}
	assertFilesystemCollection(t, root, directoryFiles)
	store.selection.SourceKind = "share"
	store.selection.ExternalRef = ""
	store.selection.CandidateID = 11
	store.selection.Revision++
	store.chosen = 1
	expectSelectedFile(t, mock, store.selection, true, false)
	if result := service.exportLocalStrm(ctx, cfg, source, nil, map[string]bool{}); result.Err != nil {
		t.Fatal(result.Err)
	}
	assertFilesystemCollection(t, root, shareFiles)
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, store.selection.RelativePath)); err != nil {
		t.Fatal(err)
	}
}

type selection115Checkpoint struct {
	*incrementalMemory
	selection      domain.ShareSelection
	keys           []string
	planned        []string
	completed      bool
	legacyComplete bool
}

func (store *selection115Checkpoint) ObserveWork(context.Context, string) ([]domain.ShareExportSourceState, error) {
	return []domain.ShareExportSourceState{{SourceFileID: 7, WorkKey: store.selection.WorkKey, State: "stale"}}, nil
}
func (store *selection115Checkpoint) WorkKeys(context.Context, string) ([]string, error) {
	return []string{store.selection.ItemKey}, nil
}
func (store *selection115Checkpoint) TargetSnapshot(context.Context, []string) (dao.ExportSnapshot, error) {
	return dao.ExportSnapshot{store.selection.ItemKey: "old-run"}, nil
}
func (store *selection115Checkpoint) RecordPlannedKeys(_ context.Context, _ string, _ int64, keys []string) error {
	store.planned = append(store.planned, keys...)
	return nil
}
func (store *selection115Checkpoint) CompleteWork(context.Context, domain.ShareExportDirty, int64, string, []domain.ShareExportSourceState, dao.ExportSnapshot) (int, error) {
	store.legacyComplete = true
	return 0, errors.New("directory keys must be preserved")
}
func (store *selection115Checkpoint) CompleteSelectionWork(_ context.Context, _ domain.ShareExportDirty, _ int64, _ string, states []domain.ShareExportSourceState, _ dao.ExportSnapshot, keys []string) (int, error) {
	store.keys = keys
	if !slices.Contains(keys, store.selection.ItemKey) || len(states) != 1 || states[0].State != "stale" {
		return 0, errors.New("false ack or lost directory output")
	}
	store.completed = true
	return 0, nil
}

func TestSelection115WithoutLiveShareNormalAndIncremental(t *testing.T) {
	for _, mode := range []string{"targeted", "filtered", "filtered-unavailable", "incremental", "unavailable", "unanchored"} {
		t.Run(mode, func(t *testing.T) {
			metadata := filesystemSource(1, 7, 100, filesystemEpisode(1, 1))
			metadata.Result.SeasonNumber = 1
			metadata.Result.EpisodeNumber = 1
			selection := domain.ShareSelection{ItemKey: shareStrmIdentity(metadata, metadata.Episodes[0]), WorkKey: metadata.WorkKey, Season: 1, Episode: 1, SourceKind: "strm115", ExternalRef: "0:5", Revision: 3, ExportedRevision: 2, RelativePath: "tv/稳定.strm", Valid: true, Mode: "manual"}
			if mode == "unanchored" {
				selection.RelativePath = ""
			}
			unavailable := mode == "unavailable" || mode == "filtered-unavailable"
			store := &selection115Memory{selectedFilesystemStore: &selectedFilesystemStore{strmMemory: &strmMemory{sources: []domain.ShareStrmSource{}, entries: map[string]domain.ShareStrmEntry{}}, selection: selection}, candidate: domain.ShareCandidate{SourceKind: "strm115", ExternalRef: "0:5", CloudPath: "/目录/原始布局/剧.S01E01.mkv", Cloud115ID: 9, Available: !unavailable}, metadata: metadata}
			if mode == "filtered-unavailable" {
				store.sources = []domain.ShareStrmSource{metadata}
			}
			store.metadata.ID = 0
			store.metadata.ShareID = 0
			store.metadata.FileName = ""
			service := NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, nil, func() ([]*domain.MediaCategory, error) { return nil, nil }, nil, nil)
			service.SetStrm115ServerURL(func() string { return "https://server.test" })
			database, mock, _ := sqlmock.New()
			defer database.Close()
			connection, err := database.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			root := t.TempDir()
			output := &StrmOutput{Store: &dao.StrmExportDAO{Conn: connection}, Root: root, Owner: "share:default", Run: "directory-only", Paths: map[string]bool{}}
			ctx := context.WithValue(context.Background(), shareExportOutputContext{}, output)
			cfg := domain.ShareStrmSettings{OutputPath: root, BaseURL: "https://share.test"}
			expected := selection
			if expected.RelativePath == "" {
				expected.RelativePath, err = service.strmRelativePath(metadata, domain.ShareFileInfo{}, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			filename := filepath.Join(root, expected.RelativePath)
			if err = os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filename, []byte("old share bytes\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before := filesystemCollection(t, root)
			if !unavailable {
				expectSelectedFile(t, mock, expected, true, false)
			}
			if mode == "incremental" || mode == "unavailable" {
				checkpoints := &selection115Checkpoint{selection: selection}
				incremental := NewShareStrmIncrementalService(service, checkpoints)
				processed, marked, result := incremental.consumeWork(ctx, "fixture", domain.ShareExportInput{Settings: cfg, ConfigRevision: 1}, domain.ShareExportDirty{WorkKey: selection.WorkKey, Revision: 3}, nil)
				if mode == "unavailable" {
					if !errors.Is(result.Err, dao.ErrShareSelectionUnavailable) || checkpoints.completed || checkpoints.legacyComplete || result.Written != 0 {
						t.Fatalf("%+v %+v", result, checkpoints)
					}
					assertFilesystemCollection(t, root, before)
				} else if result.Err != nil || result.Written != 1 || processed != 0 || marked != 0 || !checkpoints.completed || checkpoints.legacyComplete || !slices.Contains(checkpoints.planned, selection.ItemKey) {
					t.Fatalf("%d %d %+v %+v", processed, marked, result, checkpoints)
				}
			} else {
				query := domain.ShareLibraryQuery{WorkKey: selection.WorkKey}
				if mode == "filtered" || mode == "filtered-unavailable" {
					query = domain.ShareLibraryQuery{Keyword: "fixture", MediaType: "tv"}
				}
				err = service.export(ctx, cfg, query, "fixture")
				if mode == "filtered-unavailable" {
					if !errors.Is(err, dao.ErrShareSelectionUnavailable) || store.sourcePages != 0 || len(store.entries) != 0 || len(store.files) != 0 {
						t.Fatalf("preflight: %v pages=%d", err, store.sourcePages)
					}
					assertFilesystemCollection(t, root, before)
				} else if err != nil {
					t.Fatal(err)
				}
				if len(store.queries) == 0 || !reflect.DeepEqual(store.queries[0], query) {
					t.Fatalf("scope lost: %+v", store.queries)
				}
			}
			if !unavailable {
				content, err := os.ReadFile(filename)
				entry, _ := domain.BuildStrm115Entry("https://server.test", store.candidate)
				if err != nil || string(content) != entry.Content || len(store.entries) != 0 || len(store.files) != 0 {
					t.Fatalf("%q %v mappings=%v files=%v", content, err, store.entries, store.files)
				}
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelection115FinalAvailabilityGuardPreservesBytes(t *testing.T) {
	metadata := filesystemSource(1, 7, 100, filesystemEpisode(1, 1))
	selection := domain.ShareSelection{ItemKey: shareStrmIdentity(metadata, metadata.Episodes[0]), WorkKey: metadata.WorkKey, Season: 1, Episode: 1, SourceKind: "strm115", ExternalRef: "0:5", Revision: 3, RelativePath: "existing.strm", Mode: "manual", Valid: true}
	store := &selection115Memory{selectedFilesystemStore: &selectedFilesystemStore{strmMemory: &strmMemory{entries: map[string]domain.ShareStrmEntry{}}, selection: selection}, candidate: domain.ShareCandidate{SourceKind: "strm115", ExternalRef: "0:5", CloudPath: "/剧/S01E01.mkv", Cloud115ID: 9, Available: true}}
	store.onGuard = func(count int) error {
		if count == 2 {
			store.candidate.Available = false
		}
		return nil
	}
	service := NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, nil, nil, nil, nil)
	service.SetStrm115ServerURL(func() string { return "https://server.test" })
	database, mock, _ := sqlmock.New()
	defer database.Close()
	connection, err := database.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	root := t.TempDir()
	filename := filepath.Join(root, selection.RelativePath)
	if err = os.WriteFile(filename, []byte("unchanged original bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := filesystemCollection(t, root)
	output := &StrmOutput{Store: &dao.StrmExportDAO{Conn: connection}, Root: root, Owner: "share:default", Run: "guard", Paths: map[string]bool{}}
	ctx := context.WithValue(context.Background(), shareExportOutputContext{}, output)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	mock.ExpectExec("SELECT pg_advisory_lock\\(34983,1\\)").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	mock.ExpectQuery("SELECT owner_key,export_key").WillReturnRows(sqlmock.NewRows([]string{"owner", "key"}).AddRow("share:default", selection.ItemKey))
	mock.ExpectExec("INSERT INTO t_strm_export_history VALUES").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT pg_advisory_unlock\\(34983,1\\)").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 1))
	result := service.exportLocalStrm(ctx, domain.ShareStrmSettings{OutputPath: root}, metadata, nil, map[string]bool{})
	if !errors.Is(result.Err, dao.ErrShareSelectionUnavailable) || result.Written != 0 || len(output.Seen) != 0 {
		t.Fatalf("%+v seen=%v", result, output.Seen)
	}
	assertFilesystemCollection(t, root, before)
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelection115MetadataOnlyReexportUnsupported(t *testing.T) {
	fixture := newWorkReexportFixture(t)
	work := fixture.request.WorkKeys[0]
	fixture.store.works[work][0].Selection.SourceKind = "strm115"
	fixture.store.works[work][0].Selection.ExternalRef = "0:5"
	fixture.store.works[work][0].Selection.CandidateID = 0
	before := filesystemCollection(t, fixture.request.ExpectedOutputPath)
	fixture.expectClose()
	result, err := fixture.run(t, context.Background(), "unsupported-directory-metadata")
	if !errors.Is(err, domain.ErrStrm115MetadataUnsupported) || result.Written != 0 || len(fixture.store.savedSources) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	assertFilesystemCollection(t, fixture.request.ExpectedOutputPath, before)
	if err = fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
