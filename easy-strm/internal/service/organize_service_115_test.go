package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	driver "github.com/SheltonZhu/115driver/pkg/driver"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

type fakeOrganizeCloud115Client struct {
	fileList          *driver.FileListResp
	fileLists         map[int]*driver.FileListResp
	fileListSequences map[int][]*driver.FileListResp
	fileListCalls     map[int]int
	renameFile        string
	renameName        string
	renameHistory     []string
	renameNames       []string
	moveFile          string
	moveTarget        string
	copyFile          string
	copyTarget        string
	mkdirPath         string
}

func (f *fakeOrganizeCloud115Client) GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error) {
	if f.fileListSequences != nil {
		if sequence, ok := f.fileListSequences[cid]; ok && len(sequence) > 0 {
			if f.fileListCalls == nil {
				f.fileListCalls = make(map[int]int)
			}
			index := f.fileListCalls[cid]
			if index >= len(sequence) {
				index = len(sequence) - 1
			}
			f.fileListCalls[cid]++
			return sequence[index], nil
		}
	}
	if f.fileLists != nil {
		if resp, ok := f.fileLists[cid]; ok {
			return resp, nil
		}
	}
	if f.fileList == nil {
		return &driver.FileListResp{}, nil
	}
	return f.fileList, nil
}

func (f *fakeOrganizeCloud115Client) GetPickCodeByPath(filePath string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}

func (f *fakeOrganizeCloud115Client) GetCIDByPath(path string, cloud115ID int, cookie string) (string, error) {
	return "0", nil
}

func (f *fakeOrganizeCloud115Client) RenameFile(fileID, newName string, cloud115ID int, cookie string) error {
	f.renameFile = fileID
	f.renameName = newName
	f.renameHistory = append(f.renameHistory, fileID)
	f.renameNames = append(f.renameNames, newName)
	return nil
}

func (f *fakeOrganizeCloud115Client) CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error {
	f.copyFile = fileID
	f.copyTarget = targetDirID
	return nil
}

func (f *fakeOrganizeCloud115Client) MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error {
	f.moveFile = fileID
	f.moveTarget = targetDirID
	return nil
}

func (f *fakeOrganizeCloud115Client) MkdirAll115(path string, cloud115ID int, cookie string) (string, error) {
	f.mkdirPath = path
	return "target-cid", nil
}

func expectCloud115Account(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "name", "cookie", "refresh_token", "access_token", "expires_in",
		"transfer_account_id", "transfer_directory", "account_type", "quota_used", "priority",
		"status", "cooling_start_time", "transfer_method", "alist_url", "alist_token",
		"create_time", "update_time",
	}).AddRow(1, "115", "cookie", "", "", 0, 0, "", "resource", 0, 5, "active", nil, "115driver", "", "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(rows)
}

func expectCloud115MediaSourceByID(t *testing.T, mock sqlmock.Sqlmock, sourceID int, cloud115ID int, root string) {
	t.Helper()

	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(sourceID, "cloud", domain.SourceTypeCloud115, root, root, &cloud115ID, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(sourceID).
		WillReturnRows(rows)
}

func TestOrganizeService_BuildTargetPathDoesNotCreateSameNameFolder(t *testing.T) {
	svc := &OrganizeService{}

	finalTargetPath, newName, newPath := svc.buildOrganizeTargetPath("D:/Target", "BROWSER-Title.mp4")
	if filepath.ToSlash(finalTargetPath) != "D:/Target" {
		t.Fatalf("expected target path to stay unchanged, got %q", finalTargetPath)
	}
	if newName != "BROWSER-Title.mp4" {
		t.Fatalf("expected file name to stay unchanged, got %q", newName)
	}
	if filepath.ToSlash(newPath) != "D:/Target/BROWSER-Title.mp4" {
		t.Fatalf("expected file to be placed directly in target path, got %q", newPath)
	}
}

func TestOrganizeService_BuildTargetPathUsesTemplateDirectory(t *testing.T) {
	svc := &OrganizeService{}

	finalTargetPath, newName, newPath := svc.buildOrganizeTargetPath("D:/Target", "Movies/BROWSER-Title.mp4")
	if filepath.ToSlash(finalTargetPath) != "D:/Target/Movies" {
		t.Fatalf("expected template directory to become parent path, got %q", finalTargetPath)
	}
	if newName != "BROWSER-Title.mp4" {
		t.Fatalf("expected file name without template directory, got %q", newName)
	}
	if filepath.ToSlash(newPath) != "D:/Target/Movies/BROWSER-Title.mp4" {
		t.Fatalf("expected file to be placed in template directory, got %q", newPath)
	}
}

func TestOrganizeService_BuildCloud115TargetPathUsesSlashAndTemplateDirectory(t *testing.T) {
	svc := &OrganizeService{}

	finalTargetPath, newName, newPath := svc.buildCloud115OrganizeTargetPath("0", "Movies/BROWSER-Title.mp4")
	if finalTargetPath != "0/Movies" {
		t.Fatalf("expected cloud target path to keep slash format, got %q", finalTargetPath)
	}
	if newName != "BROWSER-Title.mp4" {
		t.Fatalf("expected cloud file name without template directory, got %q", newName)
	}
	if newPath != "0/Movies/BROWSER-Title.mp4" {
		t.Fatalf("expected cloud file path to use slash format, got %q", newPath)
	}
}

func TestOrganizeService_Resolve115CIDSupportsNumericCID(t *testing.T) {
	client := &fakeOrganizeCloud115Client{}
	svc := &OrganizeService{client: client}

	cid, err := svc.resolve115CID("12345", 1, "cookie")
	if err != nil {
		t.Fatalf("expected numeric cid to resolve without error: %v", err)
	}
	if cid != "12345" {
		t.Fatalf("expected numeric cid to stay unchanged, got %q", cid)
	}
	if client.mkdirPath != "" {
		t.Fatalf("expected numeric cid not to trigger mkdir, got %q", client.mkdirPath)
	}
}

func TestOrganizeService_OrganizeCloud115CopyRenamesCopiedFileOnly(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	expectCloud115Account(t, mock)

	targetCID := 2001
	client := &fakeOrganizeCloud115Client{
		fileListSequences: map[int][]*driver.FileListResp{
			targetCID: {
				{Files: []driver.FileInfo{}},
				{
					Files: []driver.FileInfo{
						{
							CategoryID: driver.IntString(strconv.Itoa(targetCID)),
							FileID:     "copied-file-id",
							Name:       "Doraemon.mp4",
							Type:       "mp4",
						},
					},
				},
			},
		},
	}

	svc := &OrganizeService{
		cloud115DAO: dao.NewCloud115DAO(),
		client:      client,
	}

	result, err := svc.organizeCloud115File(&domain.MediaSource{
		ID:         1,
		SourceType: domain.SourceTypeCloud115,
		Cloud115ID: intPtr(1),
	}, OrganizePreview{
		FileID:     "movies/Doraemon.mp4",
		CloudID:    "source-file-id",
		FileName:   "Doraemon.mp4",
		NewName:    "Doraemon - COPY.mkv",
		TargetPath: strconv.Itoa(targetCID),
		NewPath:    strconv.Itoa(targetCID) + "/Doraemon - COPY.mkv",
	}, "skip", organizeOperationCopy)
	if err != nil {
		t.Fatalf("expected copy organize to succeed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful organize result, got %+v", result)
	}
	if client.copyFile != "source-file-id" || client.copyTarget != strconv.Itoa(targetCID) {
		t.Fatalf("expected copy to use source file id, got file=%q target=%q", client.copyFile, client.copyTarget)
	}
	if len(client.renameHistory) != 1 {
		t.Fatalf("expected one rename after copy, got %d", len(client.renameHistory))
	}
	if client.renameHistory[0] != "copied-file-id" {
		t.Fatalf("expected copied file to be renamed, got %q", client.renameHistory[0])
	}
	if client.renameNames[0] != "Doraemon - COPY.mkv" {
		t.Fatalf("expected copied file rename target, got %q", client.renameNames[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizeService_ScanCloud115MatchesSelectedFileID(t *testing.T) {
	client := &fakeOrganizeCloud115Client{
		fileList: &driver.FileListResp{
			Files: []driver.FileInfo{
				{
					CategoryID: driver.IntString("0"),
					FileID:     "fid-1",
					Name:       "Movie.mkv",
					Type:       "mkv",
				},
			},
		},
	}
	svc := &OrganizeService{client: client}

	files, err := svc.scanCloud115Recursive("0", "", 1, "cookie", "all", []string{"fid-1"})
	if err != nil {
		t.Fatalf("expected scan to succeed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected selected 115 file to be scanned, got %d files", len(files))
	}
	if files[0].CID != "fid-1" {
		t.Fatalf("expected cloud file id to be preserved, got %q", files[0].CID)
	}
}

func TestOrganizeService_ScanCloud115UsesShortTermDirectoryCache(t *testing.T) {
	client := &fakeOrganizeCloud115Client{
		fileListSequences: map[int][]*driver.FileListResp{
			0: {
				{
					Files: []driver.FileInfo{
						{
							CategoryID: driver.IntString("0"),
							FileID:     "fid-1",
							Name:       "Movie.mkv",
							Type:       "mkv",
						},
					},
				},
				{
					Files: []driver.FileInfo{
						{
							CategoryID: driver.IntString("0"),
							FileID:     "fid-2",
							Name:       "Changed.mkv",
							Type:       "mkv",
						},
					},
				},
			},
		},
	}
	svc := &OrganizeService{
		client:            client,
		cloud115ListCache: make(map[string]cloud115ListCacheEntry),
	}

	first, err := svc.scanCloud115Recursive("0", "", 1, "cookie", "all", []string{"fid-1"})
	if err != nil {
		t.Fatalf("expected first scan to succeed: %v", err)
	}
	second, err := svc.scanCloud115Recursive("0", "", 1, "cookie", "all", []string{"fid-1"})
	if err != nil {
		t.Fatalf("expected second scan to succeed: %v", err)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected cached scans to return one file each, got first=%d second=%d", len(first), len(second))
	}
	if first[0].CID != "fid-1" || second[0].CID != "fid-1" {
		t.Fatalf("expected cached result to preserve first response, got first=%q second=%q", first[0].CID, second[0].CID)
	}
	if calls := client.fileListCalls[0]; calls != 1 {
		t.Fatalf("expected cached scan to hit remote once, got %d", calls)
	}
}

func TestOrganizeService_FilterFilesByIDsMatchesCloudFileID(t *testing.T) {
	svc := &OrganizeService{}
	files := []domain.MediaFile{
		{
			ID:   "movies/Doraemon.mp4",
			CID:  "fid-1",
			Name: "Doraemon.mp4",
			Path: "movies/Doraemon.mp4",
		},
	}

	filtered := svc.filterFilesByIDs(files, []string{"fid-1"})
	if len(filtered) != 1 {
		t.Fatalf("expected cloud file id filter to keep the file, got %d", len(filtered))
	}
}

func TestOrganizeService_ListOrganizeCandidatesCloud115SelectedDirectoryIncludesChildren(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	expectCloud115MediaSourceByID(t, mock, 1, 1, "0")
	expectCloud115Account(t, mock)

	client := &fakeOrganizeCloud115Client{
		fileLists: map[int]*driver.FileListResp{
			0: {
				Files: []driver.FileInfo{
					{
						CategoryID: driver.IntString("1001"),
						Name:       "Movies",
						Type:       "folder",
					},
				},
			},
			1001: {
				Files: []driver.FileInfo{
					{
						CategoryID: driver.IntString("1001"),
						FileID:     "fid-1",
						Name:       "Movie.mp4",
						Type:       "mp4",
					},
				},
			},
		},
	}
	svc := &OrganizeService{
		mediaSourceService: NewMediaSourceService(dao.NewMediaSourceDAO(), nil),
		cloud115DAO:        dao.NewCloud115DAO(),
		client:             client,
	}

	candidates, err := svc.ListOrganizeCandidates(1, "0", "all", []string{"1001"})
	if err != nil {
		t.Fatalf("expected 115 directory candidate listing to succeed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate from selected 115 directory, got %d", len(candidates))
	}
	if candidates[0].CloudID != "fid-1" {
		t.Fatalf("expected selected child file cloud id to be preserved, got %q", candidates[0].CloudID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizeService_GetCachedIdentifyResultPrefersManualCloudIDCache(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	rawData, err := json.Marshal(domain.TmdbSearchResult{
		TmdbID:    1148677,
		Title:     "鍝嗗暒A姊︼細澶ч泟鐨勫湴鐞冧氦鍝嶄箰",
		MediaType: "movie",
		GenreIDs:  []int{16},
		Countries: []string{"JP"},
		Language:  "ja",
	})
	if err != nil {
		t.Fatalf("failed to marshal raw data: %v", err)
	}

	now := time.Now()
	cacheRows := sqlmock.NewRows([]string{
		"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path",
		"overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number",
		"raw_data", "expire_at", "create_time", "update_time",
	}).AddRow(
		1, "2979491657162553316", "movie", 1148677,
		"Doraemon Earth Symphony", "Doraemon Earth Symphony",
		2024, nil, nil, 0, "2024-03-01", nil, nil, nil,
		rawData, now.Add(24*time.Hour), now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, query_key, media_type, tmdb_id, title, original_title, year, poster_path, 
		        overview, vote_average, release_date, first_air_date, season_number, episode_number, 
		        raw_data, expire_at, create_time, update_time
		 FROM t_tmdb_cache 
		 WHERE query_key = $1 AND media_type = $2 AND expire_at > NOW()`)).
		WithArgs("movie.mp4", "movie").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path",
			"overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number",
			"raw_data", "expire_at", "create_time", "update_time",
		}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, query_key, media_type, tmdb_id, title, original_title, year, poster_path, 
		        overview, vote_average, release_date, first_air_date, season_number, episode_number, 
		        raw_data, expire_at, create_time, update_time
		 FROM t_tmdb_cache 
		 WHERE query_key = $1 AND media_type = $2 AND expire_at > NOW()`)).
		WithArgs("movie.mp4", "tv").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path",
			"overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number",
			"raw_data", "expire_at", "create_time", "update_time",
		}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, query_key, media_type, tmdb_id, title, original_title, year, poster_path, 
		        overview, vote_average, release_date, first_air_date, season_number, episode_number, 
		        raw_data, expire_at, create_time, update_time
		 FROM t_tmdb_cache 
		 WHERE query_key = $1 AND media_type = $2 AND expire_at > NOW()`)).
		WithArgs("2979491657162553316", "movie").
		WillReturnRows(cacheRows)

	svc := &OrganizeService{
		tmdbCacheDAO: dao.NewTmdbCacheDAO(),
		tmdbService:  NewTmdbService("", dao.NewTmdbCacheDAO()),
	}

	result := svc.getCachedIdentifyResult(domain.MediaFile{
		ID:   "movie.mp4",
		CID:  "2979491657162553316",
		Name: "movie.mp4",
	})
	if result == nil {
		t.Fatal("expected cached identify result")
	}
	if result.TmdbID != 1148677 || result.Title != "Doraemon Earth Symphony" {
		t.Fatalf("unexpected cached identify result: %+v", result)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizeService_OrganizeCloud115MovesFile(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectCloud115Account(t, mock)

	client := &fakeOrganizeCloud115Client{}
	cloudID := 1
	svc := &OrganizeService{
		cloud115DAO:       dao.NewCloud115DAO(),
		client:            client,
		cloud115ListCache: make(map[string]cloud115ListCacheEntry),
	}
	svc.setCloud115CachedFileList("1:stale-cid", []domain.MediaFile{{ID: "old-file", Name: "Old.mkv"}})

	result, err := svc.organizeCloud115File(&domain.MediaSource{
		ID:         1,
		SourceType: domain.SourceTypeCloud115,
		Cloud115ID: &cloudID,
	}, OrganizePreview{
		FileID:     "Movie.mkv",
		CloudID:    "fid-1",
		FileName:   "Movie.mkv",
		FilePath:   "Movie.mkv",
		NewName:    "BROWSER-Movie.mkv",
		NewPath:    "/library/movies/BROWSER-Movie.mkv",
		TargetPath: "/library/movies",
	}, "skip", organizeOperationMove)
	if err != nil {
		t.Fatalf("expected 115 organize to succeed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}
	if client.mkdirPath != "/library/movies" {
		t.Fatalf("expected target directory to be created, got %q", client.mkdirPath)
	}
	if client.renameFile != "fid-1" || client.renameName != "BROWSER-Movie.mkv" {
		t.Fatalf("expected rename by 115 file id, got file=%q name=%q", client.renameFile, client.renameName)
	}
	if client.moveFile != "fid-1" || client.moveTarget != "target-cid" {
		t.Fatalf("expected move by 115 file id, got file=%q target=%q", client.moveFile, client.moveTarget)
	}
	if client.copyFile != "" || client.copyTarget != "" {
		t.Fatalf("expected move mode not to copy, got file=%q target=%q", client.copyFile, client.copyTarget)
	}
	if len(svc.cloud115ListCache) != 0 {
		t.Fatalf("expected successful 115 organize to invalidate account cache, got %#v", svc.cloud115ListCache)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizeService_OrganizeCloud115RejectsLinkModes(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectCloud115Account(t, mock)

	client := &fakeOrganizeCloud115Client{}
	cloudID := 1
	svc := &OrganizeService{
		cloud115DAO: dao.NewCloud115DAO(),
		client:      client,
	}

	_, err := svc.organizeCloud115File(&domain.MediaSource{
		ID:         1,
		SourceType: domain.SourceTypeCloud115,
		Cloud115ID: &cloudID,
	}, OrganizePreview{
		FileID:     "Movie.mkv",
		CloudID:    "fid-1",
		FileName:   "Movie.mkv",
		FilePath:   "Movie.mkv",
		NewName:    "Movie.mkv",
		NewPath:    "/library/movies/Movie.mkv",
		TargetPath: "/library/movies",
	}, "skip", organizeOperationHardLink)
	if err == nil {
		t.Fatal("expected hard link mode to be rejected for 115 cloud")
	}
	if client.moveFile != "" || client.copyFile != "" {
		t.Fatalf("expected no 115 file operation on unsupported link mode, got move=%q copy=%q", client.moveFile, client.copyFile)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOrganizeService_GetPreferredIdentifyResultPrefersManualOverride(t *testing.T) {
	svc := &OrganizeService{}

	result, err := svc.getPreferredIdentifyResult(domain.MediaFile{
		ID:   "movie.mp4",
		CID:  "fid-1",
		Name: "movie.mp4",
	}, &domain.OrganizeManualOverride{
		FileID:    "movie.mp4",
		CloudID:   "fid-1",
		MediaType: "tv",
		TmdbID:    95299,
		Title:     "The Office",
		Year:      2005,
		Season:    1,
		Episode:   2,
	}, 1)
	if err != nil {
		t.Fatalf("expected manual override to succeed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected manual override result, got %+v", result)
	}
	if result.MediaType != "tv" || result.TmdbID != 95299 || result.Title != "The Office" {
		t.Fatalf("unexpected manual override result: %+v", result)
	}
	if result.SeasonNumber != 1 || result.EpisodeNumber != 2 {
		t.Fatalf("expected season/episode from manual override, got %+v", result)
	}
}

func TestOrganizeService_MatchOrganizeManualOverrideSupportsCloudID(t *testing.T) {
	svc := &OrganizeService{}
	overrideMap := svc.buildOrganizeManualOverrideMap([]domain.OrganizeManualOverride{
		{
			FileID:    "Movies/Movie.mp4",
			CloudID:   "fid-1",
			MediaType: "movie",
			TmdbID:    603,
			Title:     "The Matrix",
			Year:      1999,
		},
	})

	match := svc.matchOrganizeManualOverride(domain.MediaFile{
		ID:   "Movies/Movie.mp4",
		CID:  "fid-1",
		Name: "Movie.mp4",
	}, overrideMap)
	if match == nil {
		t.Fatal("expected to match manual override by cloud id")
	}
	if match.TmdbID != 603 || match.Title != "The Matrix" {
		t.Fatalf("unexpected override match: %+v", match)
	}
}

func TestOrganizeService_BuildPreviewErrorResultKeepsCloudID(t *testing.T) {
	svc := &OrganizeService{}

	result := svc.buildPreviewErrorResult(domain.MediaFile{
		ID:   "电影/示例.mp4",
		CID:  "cloud-file-id",
		Name: "示例.mp4",
		Path: "电影/示例.mp4",
	}, fmt.Errorf("识别失败"))

	if result.FileID != "电影/示例.mp4" {
		t.Fatalf("expected file_id to be preserved, got %q", result.FileID)
	}
	if result.CloudID != "cloud-file-id" {
		t.Fatalf("expected cloud_id to be preserved, got %q", result.CloudID)
	}
	if result.IdentifyError != "识别失败" {
		t.Fatalf("expected identify error to be preserved, got %q", result.IdentifyError)
	}
}

func TestResolveCloud115ScanRootCID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		sourcePath string
		expected   string
	}{
		{name: "空路径回退根目录", sourcePath: "", expected: "0"},
		{name: "斜杠回退根目录", sourcePath: "/", expected: "0"},
		{name: "保留媒体源CID", sourcePath: "2977269469445485999", expected: "2977269469445485999"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if actual := resolveCloud115ScanRootCID(tc.sourcePath); actual != tc.expected {
				t.Fatalf("resolveCloud115ScanRootCID(%q) = %q, want %q", tc.sourcePath, actual, tc.expected)
			}
		})
	}
}

func intPtr(value int) *int {
	return &value
}
