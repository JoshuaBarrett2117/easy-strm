package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type selectedFilesystemStore struct {
	*strmMemory
	selection  domain.ShareSelection
	chosen     int
	resolveErr error
	syncs      int
}

func (store *selectedFilesystemStore) CheckSelectionSchema(context.Context) error { return nil }
func (store *selectedFilesystemStore) ListSelections(context.Context, domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error) {
	return nil, 0, nil
}
func (store *selectedFilesystemStore) SelectionDetail(context.Context, string) (domain.ShareSelectionDetail, error) {
	return domain.ShareSelectionDetail{Selection: store.selection}, nil
}
func (store *selectedFilesystemStore) ChangeSelection(context.Context, domain.ShareSelectionChange) error {
	return nil
}
func (store *selectedFilesystemStore) SyncSelections(context.Context, string) error {
	store.syncs++
	return nil
}
func (store *selectedFilesystemStore) ResolveSelection(context.Context, string) (domain.ShareSelection, int, error) {
	return store.selection, store.chosen, store.resolveErr
}
func (store *selectedFilesystemStore) WorkSelectionKeys(context.Context, string) ([]string, error) {
	return []string{store.selection.ItemKey}, nil
}
func (store *selectedFilesystemStore) AnchorSelectionPath(_ context.Context, key, relative string) (string, error) {
	if store.selection.RelativePath == "" {
		store.selection.RelativePath = relative
	}
	return store.selection.RelativePath, nil
}
func (store *selectedFilesystemStore) GetStrmSource(_ context.Context, id int) (domain.ShareStrmSource, error) {
	for _, source := range store.sources {
		if source.ID == id {
			return source, nil
		}
	}
	return domain.ShareStrmSource{}, sql.ErrNoRows
}

func expectSelectedFile(t *testing.T, mock sqlmock.Sqlmock, selection domain.ShareSelection, own bool, foreign bool) {
	t.Helper()
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	rows := sqlmock.NewRows([]string{"owner", "key"})
	if own {
		rows.AddRow("share:default", selection.ItemKey)
	}
	if foreign {
		rows.AddRow("cloud115:7", "other")
	}
	mock.ExpectQuery("SELECT owner_key,export_key").WillReturnRows(rows)
	if !foreign {
		mock.ExpectExec("INSERT INTO t_strm_export_history VALUES").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectBegin()
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT selection_revision,stable_relative_path").WillReturnRows(sqlmock.NewRows([]string{"revision", "path"}).AddRow(selection.Revision, selection.RelativePath))
		mock.ExpectExec("INSERT INTO t_strm_export_history SELECT").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO t_strm_export_state").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE t_share_media_selection SET stable_relative_path").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestSelectedExportFirstDiscoveryManualReplaceAndForeignGuard(t *testing.T) {
	first := filesystemSource(1, 7, 100, filesystemEpisode(1, 1))
	second := filesystemSource(2, 8, 9999, filesystemEpisode(1, 1))
	store := &selectedFilesystemStore{strmMemory: &strmMemory{sources: []domain.ShareStrmSource{first, second}, entries: map[string]domain.ShareStrmEntry{}}, selection: domain.ShareSelection{ItemKey: shareStrmIdentity(first, first.Episodes[0]), WorkKey: first.WorkKey, Season: 1, Episode: 1, CandidateID: 11, Revision: 1, Mode: "auto", Valid: true}, chosen: 1}
	service := NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, nil, func() ([]*domain.MediaCategory, error) { return nil, nil }, nil, nil)
	database, mock, _ := sqlmock.New()
	defer database.Close()
	connection, err := database.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	root := t.TempDir()
	cfg := domain.ShareStrmSettings{OutputPath: root, BaseURL: "https://media.example.test", DedupeExport: false}
	output := &StrmOutput{Store: &dao.StrmExportDAO{Conn: connection}, Root: root, Owner: "share:default", Run: "selected", Paths: map[string]bool{}, Snapshot: dao.ExportSnapshot{}}
	ctx := context.WithValue(context.Background(), shareExportOutputContext{}, output)
	expectSelectedFile(t, mock, store.selection, false, false)
	if err = service.export(ctx, cfg, domain.ShareLibraryQuery{FileIDs: []int{2}}, "first-discovery"); err != nil {
		t.Fatal(err)
	}
	files := filesystemCollection(t, root)
	if len(files) != 1 {
		t.Fatalf("fanout: %v", files)
	}
	var relative string
	for name, content := range files {
		relative = name
		if string(content) != string(filesystemContent(first)) {
			t.Fatal("largest file replaced first discovery")
		}
		if strings.Contains(name, "分享") || strings.Contains(name, "文件") {
			t.Fatalf("source suffix: %s", name)
		}
	}
	store.selection.RelativePath = relative
	store.selection.Mode = "manual"
	store.selection.CandidateID = 12
	store.selection.Revision = 2
	store.chosen = 2
	store.sources[1].Result.Title = "Changed metadata title"
	expectSelectedFile(t, mock, store.selection, true, false)
	if err = service.export(ctx, cfg, domain.ShareLibraryQuery{WorkKey: first.WorkKey}, "manual-choice"); err != nil {
		t.Fatal(err)
	}
	files = filesystemCollection(t, root)
	if len(files) != 1 || string(files[relative]) != string(filesystemContent(second)) {
		t.Fatalf("manual replacement changed path or content: %v", files)
	}
	store.resolveErr = dao.ErrShareSelectionUnavailable
	if err = service.export(ctx, cfg, domain.ShareLibraryQuery{WorkKey: first.WorkKey}, "invalid-manual"); !errors.Is(err, dao.ErrShareSelectionUnavailable) {
		t.Fatal(err)
	}
	assertFilesystemCollection(t, root, files)
	store.resolveErr = nil
	store.chosen = 1
	store.selection.Revision++
	expectSelectedFile(t, mock, store.selection, false, true)
	if result := service.exportLocalStrm(ctx, cfg, first, nil, map[string]bool{}); result.Err == nil || result.Written != 0 {
		t.Fatalf("foreign file overwritten: %+v", result)
	}
	assertFilesystemCollection(t, root, files)
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionPathTraversalAndIdentityCollision(t *testing.T) {
	source := filesystemSource(1, 7, 100, filesystemEpisode(1, 1))
	store := &selectedFilesystemStore{strmMemory: &strmMemory{sources: []domain.ShareStrmSource{source}, entries: map[string]domain.ShareStrmEntry{}}, selection: domain.ShareSelection{WorkKey: source.WorkKey, Season: 1, Episode: 1, RelativePath: "../foreign.strm"}, chosen: 1}
	service := NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, nil, nil, nil, nil)
	if result := service.exportLocalStrm(context.Background(), domain.ShareStrmSettings{OutputPath: t.TempDir()}, source, nil, map[string]bool{}); result.Err == nil {
		t.Fatal("traversal accepted")
	}
	source.Result.TmdbID = 0
	source.Result.SeasonNumber = 1
	source.Result.EpisodeNumber = 1
	source.WorkKey = "identity-a"
	first, err := service.strmRelativePath(source, domain.ShareFileInfo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	source.WorkKey = "identity-b"
	second, err := service.strmRelativePath(source, domain.ShareFileInfo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Ext(first) != ".strm" {
		t.Fatal("identified identities collide")
	}
}

func TestAtomicShareStrmSameDirectoryPublish(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "movie.strm")
	if err := writeShareStrm(target, "first"); err != nil {
		t.Fatal(err)
	}
	if err := writeShareStrm(target, "second"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", "first\nsecond", "first\rsecond"} {
		if err := writeShareStrm(target, invalid); err == nil {
			t.Fatalf("无效 STRM 内容被发布: %q", invalid)
		}
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != "second\n" {
		t.Fatalf("raw=%q err=%v", raw, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file leaked: %v %v", entries, err)
	}
}

func TestSelectedExport24MultiSourceTitlesStableCleanPathsAndLegacyUntouched(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	connection, err := database.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy-分享7-文件1.strm")
	if err := os.WriteFile(legacy, []byte("legacy-sentinel\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for title := 1; title <= 24; title++ {
		first := filesystemSource(title*2, 7, 100, filesystemEpisode(1+title%2, title))
		first.WorkKey = fmt.Sprintf("tmdb:tv:%d", 9000+title)
		first.Result.Title = fmt.Sprintf("测试作品%02d", title)
		first.Result.TmdbID = 9000 + title
		if title%2 == 0 {
			first.Result.MediaType = "movie"
			first.WorkKey = fmt.Sprintf("tmdb:movie:%d", 9000+title)
			first.Episodes = nil
		}
		second := first
		second.ID++
		second.ShareID = 8
		second.URL = "https://115.com/s/share8"
		second.FileSize = 9999
		second.FileName = fmt.Sprintf("remux/%d.mkv", title)
		episode := domain.ShareEpisode{}
		if len(first.Episodes) > 0 {
			episode = first.Episodes[0]
		}
		selection := domain.ShareSelection{ItemKey: shareStrmIdentity(first, episode), WorkKey: first.WorkKey, Season: episode.SeasonNumber, Episode: episode.EpisodeNumber, CandidateID: 11, Revision: 1, Mode: "auto", Valid: true}
		store := &selectedFilesystemStore{strmMemory: &strmMemory{sources: []domain.ShareStrmSource{first, second}, entries: map[string]domain.ShareStrmEntry{}}, selection: selection, chosen: first.ID}
		service := NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, nil, nil, nil, nil)
		cfg := domain.ShareStrmSettings{OutputPath: root, BaseURL: "https://media.example.test"}
		output := &StrmOutput{Store: &dao.StrmExportDAO{Conn: connection}, Root: root, Owner: "share:default", Run: "24-fixture", Paths: map[string]bool{}, Snapshot: dao.ExportSnapshot{}}
		ctx := context.WithValue(context.Background(), shareExportOutputContext{}, output)
		expectSelectedFile(t, mock, selection, false, false)
		if result := service.exportLocalStrm(ctx, cfg, second, nil, map[string]bool{}); result.Err != nil || result.Written != 1 {
			t.Fatalf("first title=%d result=%+v", title, result)
		}
		first.Result.SeasonNumber = episode.SeasonNumber
		first.Result.EpisodeNumber = episode.EpisodeNumber
		relative, err := service.strmRelativePath(first, domain.ShareFileInfo{Path: first.FileName}, nil)
		if err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil || string(content) != string(filesystemContent(first)) {
			t.Fatalf("未使用首次来源或不是干净名: %s content=%s err=%v", relative, content, err)
		}
		store.selection.RelativePath = relative
		store.selection.Revision++
		store.selection.Mode = "manual"
		store.selection.CandidateID = 12
		store.chosen = second.ID
		store.sources[1].Result.Title = "后续元数据改名"
		expectSelectedFile(t, mock, store.selection, true, false)
		if result := service.exportLocalStrm(ctx, cfg, first, nil, map[string]bool{}); result.Err != nil || result.Written != 1 {
			t.Fatalf("switch title=%d result=%+v", title, result)
		}
		content, err = os.ReadFile(filepath.Join(root, relative))
		if err != nil || string(content) != string(filesystemContent(second)) {
			t.Fatalf("换源路径/内容错误: %s", relative)
		}
	}
	files := filesystemCollection(t, root)
	if len(files) != 25 || string(files["legacy-分享7-文件1.strm"]) != "legacy-sentinel\n" {
		t.Fatalf("重复文件或触碰遗留文件: count=%d", len(files))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	t.Log("isolated temporary filesystem: 24 two-source movie/episode titles, one clean path each after switch; legacy sentinel preserved; persistence via sqlmock")
}
