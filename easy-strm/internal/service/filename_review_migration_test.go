package service

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestReviewMigrationRules(t *testing.T) {
	data, err := os.ReadFile("../../migrations/migrate_v39_filename_review_rules.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(data), "$rules$")
	if len(parts) != 3 {
		t.Fatal("迁移缺少规则数据")
	}
	var additions []FilenameRecognitionRule
	if err := json.Unmarshal([]byte(parts[1]), &additions); err != nil {
		t.Fatal(err)
	}
	rules := append(DefaultFilenameRecognitionRules(), additions...)
	store := &memoryFilenameRuleStore{}
	s := NewTmdbService("", nil)
	s.SetFilenameRecognitionRuleStore(store)
	if _, err := s.SaveFilenameRecognitionRules(rules); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, title     string
		season, episode int
	}{
		{"塞外奇侠传第15集.mp4", "塞外奇侠传", 1, 15},
		{"戴拿奥特曼第24话 湖中的吸血鬼.mkv", "戴拿奥特曼", 1, 24},
		{"如来神掌.2002.EP02.DVDRip.mkv", "如来神掌", 1, 2},
		{"勇士之城EP14.mp4", "勇士之城", 1, 14},
		{"Douluo.Dalu.S01.EP072.2019.2160p.mkv", "Douluo Dalu", 1, 72},
		{"Show.S00.E02.mkv", "Show", 0, 2},
		{"[UHA-WINGS][Kimetsu no Yaiba][01][x264 1080p][CHS].mp4", "Kimetsu no Yaiba", 1, 1},
		{"[爱の夏字幕组][戴拿奥特曼][24][湖中的吸血鬼][BDrip].mkv", "戴拿奥特曼", 1, 24},
		{"九阴真经 (18).mkv", "九阴真经", 1, 18},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := s.ParseFilename(tt.name)
			if p.Title != tt.title || p.Season != tt.season || p.Episode != tt.episode || p.MediaType != "tv" {
				t.Fatalf("解析不符: %+v", p)
			}
			q := s.analyzeShareQuery(tt.name, "auto")
			if q.MediaType != "tv" || len(q.Titles) == 0 {
				t.Fatalf("分享未消费规则: %+v", q)
			}
		})
	}
	_, compiled, err := validateAndCompileFilenameRecognitionRules(additions)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"1917.mkv", "08.mp4", "00017.m2ts", "S8D1.iso", "电影 (2020).mkv", "电影 1080p.mkv", "Show S01E01E02.mkv"} {
		if p, ok := matchFilenameRecognitionRule(normalizeFilenameRecognitionInput(name, true), compiled); ok {
			t.Fatalf("不应匹配 %s: %+v", name, p)
		}
	}
}

// TestReviewNumericRuleDoesNotReadAudioAsEpisode 复现分享电影的声道和数字片名被旧规则识别成集号。
func TestReviewNumericRuleDoesNotReadAudioAsEpisode(t *testing.T) {
	data, err := os.ReadFile("../../migrations/migrate_v40_numeric_episode_boundary.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(data), "$pattern$")
	if len(parts) != 3 {
		t.Fatal("迁移缺少规则表达式")
	}
	rules := append(DefaultFilenameRecognitionRules(), FilenameRecognitionRule{
		ID: "tv_number_episode", Name: "数字集号", Enabled: true, Priority: 78, MediaType: "tv", DefaultSeason: 1,
		Pattern: parts[1], Example: "海贼王 112.mkv",
	})
	s := NewTmdbService("", nil)
	s.SetFilenameRecognitionRuleStore(&memoryFilenameRuleStore{})
	if _, err := s.SaveFilenameRecognitionRules(rules); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{
		"插翅难飞 [2025][4K DIY 简繁+双语字幕][HDR10 Atmos 7.1][sh@CHDBits][58.61G].iso",
		"丑陋的继姐 [2025][4K 德版原盘][Dolby Vision HDR10 DTS-HDMA 5.1][75.05G].iso",
		"编号17 [2025][4K 美版原盘 DIY 国语DD.2.0][Dolby Vision HDR10 Atmos 7.1].iso",
		"梅根2.0.M3GAN.2.0.2025.2160p.WEB-DL.DDP5.1.Atmos.SDR.H265-AOC.mkv",
		"电影.Atmos.7.1.mkv", "电影.DTS-HD.MA.5.1.mkv", "电影.2025.mkv",
	} {
		p := s.ParseFilename(file)
		q := s.analyzeShareQuery(file, "auto")
		if p.Episode != 0 || q.MediaType == "tv" {
			t.Errorf("电影被误判为剧集 %s: %+v / %+v", file, p, q)
		}
	}
	for _, file := range []string{"海贼王 112.mkv", "海贼王 112 1080p.mkv", "Show S02E03.mkv"} {
		p := s.ParseFilename(file)
		if p.MediaType != "tv" || p.Episode == 0 {
			t.Errorf("丢失剧集解析 %s: %+v", file, p)
		}
	}
}
