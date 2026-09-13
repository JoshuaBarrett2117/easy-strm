package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

type strmMemory struct {
	sources []domain.ShareStrmSource
	entries map[string]domain.ShareStrmEntry
	files   map[string]domain.StrmFile
	fileErr error
	mu      sync.Mutex
}

func (m *strmMemory) StrmSources(_ context.Context, _ domain.ShareLibraryQuery, after int) ([]domain.ShareStrmSource, error) {
	if after > 0 {
		return nil, nil
	}
	return m.sources, nil
}
func (m *strmMemory) SaveStrmEntry(_ context.Context, v domain.ShareStrmEntry) error {
	m.entries[v.ID] = v
	return nil
}
func (m *strmMemory) SaveExportedStrmFile(_ context.Context, v domain.StrmFile) error {
	if m.fileErr != nil {
		return m.fileErr
	}
	if m.files == nil {
		m.files = map[string]domain.StrmFile{}
	}
	m.files[v.FilePath] = v
	return nil
}
func (m *strmMemory) GetStrmEntry(_ context.Context, id string) (domain.ShareStrmEntry, error) {
	v, ok := m.entries[id]
	if !ok {
		return v, sql.ErrNoRows
	}
	return v, nil
}
func (m *strmMemory) LockStrmPlayback(ctx context.Context, _ string) (func(), error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	m.mu.Lock()
	return m.mu.Unlock, nil
}

type strmClient struct {
	fakeShareCloud115Client
	present             bool
	receives            int
	listErr, receiveErr error
}

func (c *strmClient) MkdirAll115(string, int, string) (string, error) { return "123", nil }
func (c *strmClient) GetFileList(int, int, int, int, int, string) (*driver.FileListResp, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	v := &driver.FileListResp{}
	if c.present {
		v.Files = []driver.FileInfo{{Name: "Show.S02E03.mkv", FileID: "9", PickCode: "saved"}}
	}
	return v, nil
}
func (c *strmClient) ReceiveShare(code, password, ids, cid string, id int, cookie string) error {
	c.receives++
	if code != "share" || ids != "file-3" || cid != "123" || id != 7 {
		return errors.New("转存参数错误")
	}
	if c.receiveErr != nil {
		return c.receiveErr
	}
	c.present = true
	return nil
}

func strmFixture(t *testing.T) (*ShareStrmService, *strmMemory, *strmClient) {
	t.Helper()
	cfg := domain.ShareStrmSettings{OutputPath: t.TempDir(), BaseURL: "http://media.example", Cloud115ID: 7, TransferPath: "/播放"}
	raw, _ := json.Marshal(cfg)
	store := &strmMemory{entries: map[string]domain.ShareStrmEntry{"entry": {ID: "entry", ShareCode: "share", FileID: "file-3", FileName: "Show.S02E03.mkv"}}}
	client := &strmClient{}
	svc := NewShareStrmService(store, &shareSettingsMemory{value: &domain.SystemConfig{ConfigVal: string(raw)}}, client, NewTmdbService("", nil), &OrganizeService{}, nil, func() ([]*domain.MediaCategory, error) { return nil, nil }, func(id int) (*domain.Cloud115, error) { return &domain.Cloud115{ID: id, Cookie: "fixture"}, nil }, func(pick string, id int, cookie, ua string) (string, error) {
		if pick != "saved" || id != 7 || ua != "Player/1" {
			return "", errors.New("直链参数错误")
		}
		return "https://cdn.example/video", nil
	})
	return svc, store, client
}

func TestShareStrmPlaybackTransfersOnceAndPreservesUA(t *testing.T) {
	s, _, c := strmFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			link, err := s.Playback(context.Background(), "entry", "Player/1")
			if err != nil || link != "https://cdn.example/video" {
				t.Errorf("%s %v", link, err)
			}
		}()
	}
	wg.Wait()
	if c.receives != 1 {
		t.Fatalf("重复转存%d次", c.receives)
	}
	// 新服务实例仍检查115已有文件，不依赖内存直链缓存。
	restarted := NewShareStrmService(s.store, s.settings, c, s.tmdb, s.organizer, s.tasks, s.categories, s.account, s.directLink)
	if _, err := restarted.Playback(context.Background(), "entry", "Player/1"); err != nil || c.receives != 1 {
		t.Fatalf("重启复用失败：%v %d", err, c.receives)
	}
}

func TestShareStrmPlaybackFailures(t *testing.T) {
	for _, kind := range []string{"missing", "list", "receive", "link", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			s, _, c := strmFixture(t)
			id := "entry"
			ctx := context.Background()
			switch kind {
			case "missing":
				id = "absent"
			case "list":
				c.listErr = errors.New("网络故障")
			case "receive":
				c.receiveErr = errors.New("分享失效")
			case "link":
				s.directLink = func(string, int, string, string) (string, error) { return "", errors.New("直链失败") }
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			link, err := s.Playback(ctx, id, "Player/1")
			if err == nil || link != "" {
				t.Fatalf("错误时仍返回链接：%q %v", link, err)
			}
			if (kind == "missing" || kind == "list" || kind == "cancel") && c.receives != 0 {
				t.Fatal("前置失败不能转存")
			}
		})
	}
}

func TestShareStrmPathsAndValidation(t *testing.T) {
	s, _, _ := strmFixture(t)
	source := domain.ShareStrmSource{WorkKey: "tmdb:tv:10", FileName: "Show", Result: domain.TmdbIdentifyResult{Success: true, Title: "剧/名", Year: 2024, TmdbID: 10, MediaType: "tv", Countries: []string{"CN"}}}
	cats := []*domain.MediaCategory{{Enabled: true, MediaType: "tv", TargetPath: "/媒体/国产剧", MatchRules: json.RawMessage(`{"countries":["CN"]}`)}}
	for _, tt := range []struct {
		file            string
		season, episode int
		want            string
	}{
		{"Show/Show.S02E03.mkv", 2, 3, "Season 02/剧 名 - S02E03.strm"},
		{"Show/Show.S00E01.mkv", 0, 1, "Season 00/剧 名 - S00E01.strm"},
		{"Show/Season 02/EP03.mkv", 2, 3, "Season 02/剧 名 - S02E03.strm"},
	} {
		source.Result.SeasonNumber, source.Result.EpisodeNumber = tt.season, tt.episode
		got, err := s.strmRelativePath(source, domain.ShareFileInfo{Path: tt.file}, cats)
		if err != nil || !strings.HasPrefix(filepath.ToSlash(got), "国产剧/") || !strings.HasSuffix(filepath.ToSlash(got), tt.want) {
			t.Fatalf("%s %v", got, err)
		}
	}
	source.Result.EpisodeNumber = 0
	if _, err := s.strmRelativePath(source, domain.ShareFileInfo{Path: "Show/unknown.mkv"}, cats); err == nil {
		t.Fatal("未知集数不能导出")
	}
	source.FileName = "Show/Season 02/EP03.mkv"
	source.Result.SeasonNumber, source.Result.EpisodeNumber = 1, 3
	got, err := s.strmRelativePath(source, domain.ShareFileInfo{Path: source.FileName}, cats)
	if err != nil || !strings.Contains(filepath.ToSlash(got), "Season 01/") {
		t.Fatalf("持久化映射未生效：%s %v", got, err)
	}
	source.Result.Message = "手动识别"
	source.Result.SeasonNumber = 4
	got, err = s.strmRelativePath(source, domain.ShareFileInfo{Path: source.FileName}, cats)
	if err != nil || !strings.Contains(filepath.ToSlash(got), "Season 04/") {
		t.Fatalf("手动修正未生效：%s %v", got, err)
	}
	source.Result.MediaType = "movie"
	if got, err := s.strmRelativePath(source, domain.ShareFileInfo{}, nil); err != nil || !strings.HasPrefix(got, "movie") {
		t.Fatalf("%s %v", got, err)
	}
	cfg, _ := s.Settings()
	cfg.BaseURL = "javascript:bad"
	if s.SaveSettings(cfg) == nil {
		t.Fatal("无效地址通过")
	}
	cfg, _ = s.Settings()
	cfg.OutputPath = "relative"
	if s.SaveSettings(cfg) == nil {
		t.Fatal("相对目录通过")
	}
}

func TestShareStrmExportCreatesEpisodesWithoutTransfer(t *testing.T) {
	s, store, c := strmFixture(t)
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer redisClient.Close()
	dao.InitTaskRedisDAO(redisClient)
	s.tasks = NewTaskService(dao.NewTaskRedisDAO(redisClient))
	store.sources = []domain.ShareStrmSource{{ID: 1, MediaID: 88, WorkKey: "tmdb:tv:10", URL: "https://115.com/s/share", FileName: "Show", Result: domain.TmdbIdentifyResult{Success: true, Title: "Show", Year: 2024, MediaType: "tv", TmdbID: 10, PosterPath: "/show.jpg"}}}
	store.sources[0].FileName = "Show/Show.S02E03E04.mkv"
	store.sources[0].Episodes = []domain.ShareEpisode{{SeasonNumber: 2, EpisodeNumber: 3}, {SeasonNumber: 2, EpisodeNumber: 4}}
	s.client = nil // 导出必须完全不依赖115客户端。
	id, err := s.StartExport(domain.ShareLibraryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		task, err := s.tasks.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if task["status"] == "completed" {
			break
		}
		if task["status"] == "failed" || time.Now().After(deadline) {
			t.Fatalf("%+v", task)
		}
		time.Sleep(time.Millisecond * 10)
	}
	cfg, _ := s.Settings()
	files, err := filepath.Glob(filepath.Join(cfg.OutputPath, "tv", "*", "Season 02", "*.strm"))
	if err != nil || len(files) != 2 {
		t.Fatalf("%v %v", files, err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil || !strings.HasPrefix(string(raw), "http://media.example/share-strm/") {
			t.Fatalf("%s %v", raw, err)
		}
	}
	if c.receives != 0 {
		t.Fatal("导出时不应转存")
	}
	for key, entry := range store.entries {
		if key != "entry" && (entry.FilePath == "" || entry.FileID != "") {
			t.Fatalf("导出映射必须仅保存本地路径：%+v", entry)
		}
		if key != "entry" && (entry.MediaID != 88 || entry.Title != "Show" || entry.PosterPath != "/show.jpg") {
			t.Fatalf("导出映射缺少播放记录元数据：%+v", entry)
		}
		if key != "entry" && (len(entry.Episodes) != 2 || entry.Episodes[0].SeasonNumber != 2 || entry.Episodes[1].EpisodeNumber != 4) {
			t.Fatalf("导出映射缺少季集快照：%+v", entry)
		}
	}
}

func TestShareStrmExportAddsShareNameOnlyForDifferentSources(t *testing.T) {
	s, store, _ := strmFixture(t)
	mini := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer rc.Close()
	dao.InitTaskRedisDAO(rc)
	s.tasks = NewTaskService(dao.NewTaskRedisDAO(rc))
	base := domain.ShareStrmSource{MediaID: 88, WorkKey: "tmdb:movie:10", Result: domain.TmdbIdentifyResult{Success: true, Title: "名侦探柯南剧场版M13", Year: 2009, MediaType: "movie", TmdbID: 10}}
	first := base
	first.ID, first.ShareID, first.ShareName, first.URL, first.FileName = 1, 7, "动画电影9.73TB", "https://115.com/s/first", "Movie/M13.mkv"
	duplicateInSameShare := first
	duplicateInSameShare.ID, duplicateInSameShare.FileName = 2, "Movie/M13-remux.mkv"
	second := base
	second.ID, second.ShareID, second.ShareName, second.URL, second.FileName = 3, 9, "高码电影/合集", "https://115.com/s/second", "M13.mkv"
	store.sources = []domain.ShareStrmSource{first, duplicateInSameShare, second}

	id, err := s.StartExport(domain.ShareLibraryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	waitShareTask(t, s.tasks, id)
	cfg, _ := s.Settings()
	files, err := filepath.Glob(filepath.Join(cfg.OutputPath, "movie", "*", "*.strm"))
	if err != nil || len(files) != 2 {
		t.Fatalf("应按两个不同分享来源各生成一个STRM：%v %v", files, err)
	}
	names := []string{filepath.Base(files[0]), filepath.Base(files[1])}
	joined := strings.Join(names, "|")
	if !strings.Contains(joined, "名侦探柯南剧场版M13 (2009) {tmdb-10}-动画电影9.73TB.strm") || !strings.Contains(joined, "名侦探柯南剧场版M13 (2009) {tmdb-10}-高码电影 合集.strm") {
		t.Fatalf("分享来源名称未正确追加或清理：%v", names)
	}
}

func waitShareTask(t *testing.T, tasks *TaskService, id string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		task, err := tasks.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if task["status"] == "completed" {
			return
		}
		if task["status"] == "failed" || time.Now().After(deadline) {
			t.Fatalf("任务未成功：%+v", task)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestShareStrmFirstPlaybackResolvesOnlyStoredPath(t *testing.T) {
	s, store, c := strmFixture(t)
	entry := store.entries["entry"]
	entry.FileID = ""
	entry.FilePath = "Show/Season 02/Show.S02E03.mkv"
	store.entries["entry"] = entry
	c.shareSnapByDir = map[string]*driver.ShareSnapResp{
		"0":  mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":2,"list":[{"cid":"10","n":"Show","fc":0},{"cid":"99","n":"Other","fc":0}]}}`),
		"10": mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":1,"list":[{"cid":"20","n":"Season 02","fc":0}]}}`),
		"20": mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":1,"list":[{"fid":"file-3","n":"Show.S02E03.mkv","fc":1}]}}`),
	}
	link, err := s.Playback(context.Background(), "entry", "Player/1")
	if err != nil || link != "https://cdn.example/video" {
		t.Fatalf("%s %v", link, err)
	}
	if strings.Join(c.gotDirIDs, ",") != "0,10,20" || store.entries["entry"].FileID != "file-3" {
		t.Fatalf("定位范围或缓存错误：%v %+v", c.gotDirIDs, store.entries["entry"])
	}
	c.present = false // 模拟网盘文件删除后的重新转存，仍复用已保存ID。
	if _, err = s.Playback(context.Background(), "entry", "Player/1"); err != nil || len(c.gotDirIDs) != 3 || c.receives != 2 {
		t.Fatalf("未复用文件ID：%v %v", err, c.gotDirIDs)
	}
}

func TestShareStrmLocalDirectoryDoesNotTriggerScan(t *testing.T) {
	s, _, _ := strmFixture(t)
	s.client = nil
	cfg, _ := s.Settings()
	created, err := s.exportLocalStrm(context.Background(), cfg, domain.ShareStrmSource{ID: 1, FileName: "Show/Season 02"}, nil, map[string]bool{})
	if created || err == nil || !strings.Contains(err.Error(), "缺少具体视频") {
		t.Fatalf("目录不能补扫：%v %v", created, err)
	}
}

func TestShareStrmMissingPlaybackPathDoesNotTransfer(t *testing.T) {
	s, store, c := strmFixture(t)
	entry := store.entries["entry"]
	entry.FileID = ""
	entry.FilePath = "missing.mkv"
	store.entries["entry"] = entry
	c.shareSnapByDir = map[string]*driver.ShareSnapResp{"0": mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":0,"list":[]}}`)}
	if link, err := s.Playback(context.Background(), "entry", "Player/1"); err == nil || link != "" || c.receives != 0 {
		t.Fatalf("未定位成功不应转存：%s %v", link, err)
	}
}

func TestShareStrmExportKeepsValidFilesAfterUnknownEpisode(t *testing.T) {
	s, store, c := strmFixture(t)
	mini := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer rc.Close()
	dao.InitTaskRedisDAO(rc)
	s.tasks = NewTaskService(dao.NewTaskRedisDAO(rc))
	if err := s.tasks.Create("partial", "strm_generate", "部分失败"); err != nil {
		t.Fatal(err)
	}
	store.sources = []domain.ShareStrmSource{{ID: 1, WorkKey: "tmdb:tv:10", URL: "https://115.com/s/share", FileName: "Show", Result: domain.TmdbIdentifyResult{Success: true, Title: "Show", Year: 2024, MediaType: "tv", TmdbID: 10}}}
	store.sources[0].FileName = "Show/unknown.mkv"
	second := store.sources[0]
	second.ID = 2
	second.FileName = "Show/Show.S02E03.mkv"
	second.Episodes = []domain.ShareEpisode{{SeasonNumber: 2, EpisodeNumber: 3}}
	store.sources = append(store.sources, second)
	s.client = nil
	cfg, _ := s.Settings()
	if err := s.export(context.Background(), cfg, domain.ShareLibraryQuery{}, "partial"); err == nil {
		t.Fatal("失败来源未报告")
	}
	files, _ := filepath.Glob(filepath.Join(cfg.OutputPath, "tv", "*", "Season 02", "*.strm"))
	if len(files) != 1 || c.receives != 0 {
		t.Fatalf("有效单集未保留：%v", files)
	}
	// 重复导出更新已有STRM，不增加后缀文件或产生不稳定URL。
	first, _ := os.ReadFile(files[0])
	_ = s.export(context.Background(), cfg, domain.ShareLibraryQuery{}, "partial")
	secondContent, _ := os.ReadFile(files[0])
	if string(first) != string(secondContent) {
		t.Fatal("重复导出改变播放标识")
	}
}

type strmCancelStore struct {
	*strmMemory
	started chan struct{}
}

func (p strmCancelStore) StrmSources(ctx context.Context, _ domain.ShareLibraryQuery, _ int) ([]domain.ShareStrmSource, error) {
	close(p.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestShareStrmExportCancellation(t *testing.T) {
	s, store, c := strmFixture(t)
	mini := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer rc.Close()
	dao.InitTaskRedisDAO(rc)
	s.tasks = NewTaskService(dao.NewTaskRedisDAO(rc))
	store.sources = []domain.ShareStrmSource{{ID: 1, URL: "https://115.com/s/share"}}
	started := make(chan struct{})
	s.store = strmCancelStore{strmMemory: store, started: started}
	id, err := s.StartExport(domain.ShareLibraryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("任务未开始")
	}
	if _, err = s.StartExport(domain.ShareLibraryQuery{}); err == nil {
		t.Fatal("重复运行未拦截")
	}
	s.tasks.CancelContext(id)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if s.exportMu.TryLock() {
			s.exportMu.Unlock()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("取消后任务未退出")
		}
		time.Sleep(time.Millisecond * 10)
	}
	task, err := s.tasks.Get(id)
	if err != nil || task["status"] != "cancelled" || c.receives != 0 {
		t.Fatalf("%+v %v", task, err)
	}
}
