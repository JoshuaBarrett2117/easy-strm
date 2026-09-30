package service

import (
	"context"
	"easy-strm/internal/domain"
	"testing"
)

// TestManualIdentifyRejectsInvalidSelection 验证空结果及不合法媒体类型不会写入数据库。
func TestManualIdentifyRejectsInvalidSelection(t *testing.T) {
	s := &ShareRecordService{}
	for _, m := range []domain.ShareMedia{{}, {ID: 1, Version: 1}, {ID: 1, Version: 1, Result: &domain.TmdbIdentifyResult{Title: "测试", TmdbID: 1, MediaType: "unknown"}}} {
		if s.ManualIdentify(context.Background(), m) == nil {
			t.Fatal("应拒绝无效选择")
		}
	}
}

func TestManualSeriesBatchKeepsOnlySameWorkEpisodes(t *testing.T) {
	s := &ShareRecordService{tmdb: NewTmdbService("fixture", nil)}
	base := "剧集/折腰（2025）/折腰.S01E"
	target := domain.ShareMedia{ID: 1, ShareID: 7, Version: 2, FileName: base + "36.mp4", Result: &domain.TmdbIdentifyResult{Success: true, TmdbID: 220269, Title: "折腰", Year: 2025, MediaType: "tv"}}
	records := []domain.ShareRecord{{ID: 7, Media: []domain.ShareMedia{
		{ID: 1, Version: 2, FileName: target.FileName, Status: "failed"},
		{ID: 2, Version: 3, FileName: base + "35.mp4", Status: "failed"},
		{ID: 3, Version: 1, FileName: "剧集/其他剧（2025）/折腰.S01E34.mp4", Status: "failed"},
		{ID: 4, Version: 1, FileName: "剧集/折腰（2025）/别的剧.S01E33.mp4", Status: "failed"},
		{ID: 5, Version: 1, FileName: "剧集/折腰（2025）/折腰.S02E01.mp4", Status: "failed"},
	}}}
	items, err := s.buildManualSeriesBatch(target, []domain.ShareEpisode{{SeasonNumber: 1, EpisodeNumber: 36}}, records)
	if err != nil || len(items) != 2 || items[1].ID != 2 || items[1].Episodes[0].EpisodeNumber != 35 || items[1].Result.TmdbID != 220269 {
		t.Fatalf("batch=%+v err=%v", items, err)
	}
	wrong := target
	wrong.Version = 1
	if _, err := s.buildManualSeriesBatch(wrong, []domain.ShareEpisode{{SeasonNumber: 1, EpisodeNumber: 36}}, records); err == nil {
		t.Fatal("旧版本不得批量保存")
	}
}
