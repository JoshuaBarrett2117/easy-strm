package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"testing"
)

// TestMaskedOnlyShareTaskCompletes 验证全脱敏分享不调用识别服务，并以完成状态结束。
func TestMaskedOnlyShareTaskCompletes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	dao.InitTaskRedisDAO(client)
	tasks := NewTaskService(dao.NewTaskRedisDAO(client))
	if err := tasks.Create("masked-only", "share_identify", "脱敏测试"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT count\(\*\) FROM t_share_record`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM \(SELECT .* FROM t_share_record`).WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "mid", "file_name", "source", "status", "result", "error", "mversion"}).AddRow(7, "auto", "合集", "https://115.com/s/test", "", "", 1, "now", "now", false, 1, "名***", "auto", "failed", nil, "旧失败", 1))
	s := &ShareRecordService{dao: dao.NewShareRecordDAO(db), tasks: tasks}
	s.runBatchIdentify(context.Background(), "masked-only", nil, true, []int{7})
	task, err := tasks.Get("masked-only")
	if err != nil {
		t.Fatal(err)
	}
	if task["status"] != "completed" {
		t.Fatalf("全脱敏任务未正常完成: %+v", task)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestMaskedShareDirectories 验证脱敏目录不会进入解析结果，也不会产生子目录请求。
func TestMaskedShareDirectories(t *testing.T) {
	fake := &fakeShareCloud115Client{shareSnap: mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":3,"list":[{"cid":"1","n":"名********","fc":0},{"cid":"2","n":"名＊＊＊","fc":0},{"fid":"3","n":"正常电影 (2020).mkv","fc":1}]}}`)}
	s := &ShareTransferService{client: fake}
	files, _, err := s.fetchAllShareFiles(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "正常电影 (2020).mkv" {
		t.Fatalf("错误的解析结果: %+v", files)
	}
	if len(fake.gotDirIDs) != 1 || fake.gotDirIDs[0] != "0" {
		t.Fatalf("不应请求脱敏目录: %v", fake.gotDirIDs)
	}
	for _, name := range []string{"名********", `合集\名***\Season 1`, "名＊＊＊"} {
		if !isMaskedSharePath(name) {
			t.Errorf("未识别脱敏路径: %s", name)
		}
	}
	for _, name := range []string{"命运石之门 (2011)", "猫之茗 (2021)", "星*物语"} {
		if isMaskedSharePath(name) {
			t.Errorf("不应忽略正常名称: %s", name)
		}
	}
}

// TestNormalizeMaskedMedia 验证历史失败和已识别项均独立计数，正常星号名称保持不变。
func TestNormalizeMaskedMedia(t *testing.T) {
	r := domain.ShareRecord{Media: []domain.ShareMedia{
		{FileName: "合集/名***", Status: "failed"},
		{FileName: `合集\名＊＊＊\Season 1`, Status: "identified", Result: &domain.TmdbIdentifyResult{Success: true}},
		{FileName: "星*物语", Status: "pending"},
		{FileName: "正常电影", Status: "failed"},
	}}
	for i := 0; i < 2; i++ {
		normalizeShareMaskedMedia(&r)
		if r.MaskedCount != 2 || r.Media[0].Status != "masked" || r.Media[1].Result != nil || r.Media[2].Status != "pending" || r.Media[3].Status != "failed" {
			t.Fatalf("分类统计错误: %+v", r)
		}
	}
	s := &ShareRecordService{}
	if _, err := s.identifyOne(context.Background(), r.Media[0], true); err == nil {
		t.Fatal("脱敏项应跳过识别")
	}
}

// TestParseShareRetainsMaskedStatistics 验证同名脱敏目录逐个保留用于统计，但不进入可操作文件列表。
func TestParseShareRetainsMaskedStatistics(t *testing.T) {
	fake := &fakeShareCloud115Client{shareSnap: mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":3,"list":[{"cid":"1","n":"名********","fc":0},{"cid":"2","n":"名********","fc":0},{"cid":"3","n":"名＊＊＊","fc":0}]}}`)}
	s := &ShareTransferService{client: fake}
	r, err := s.ParseShareLink(context.Background(), "https://115.com/s/test", "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r)
	var response struct {
		MaskedDirectories []struct {
			DirID string `json:"dir_id"`
		} `json:"masked_directories"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.MaskedDirectories) != 3 || len(r.Files) != 0 || r.TotalFiles != 0 || len(fake.gotDirIDs) != 1 {
		t.Fatalf("脱敏目录统计或跳过行为错误: %s requests=%v", b, fake.gotDirIDs)
	}
}
