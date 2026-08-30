package service

import (
	"testing"
	"time"
)

func TestEmbyMonitorTimeBoundsUsesNaturalPeriods(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 8, 30, 15, 30, 0, 0, loc)
	tests := []struct {
		period string
		want   time.Time
	}{
		{"day", time.Date(2026, 8, 30, 0, 0, 0, 0, loc)},
		{"yesterday", time.Date(2026, 8, 29, 0, 0, 0, 0, loc)},
		{"week", time.Date(2026, 8, 24, 0, 0, 0, 0, loc)},
		{"month", time.Date(2026, 8, 1, 0, 0, 0, 0, loc)},
		{"7d", time.Date(2026, 8, 24, 0, 0, 0, 0, loc)},
	}
	for _, tt := range tests {
		start, _, err := timeBounds(tt.period, now)
		if err != nil || !start.Equal(tt.want) {
			t.Fatalf("period=%s start=%v err=%v", tt.period, start, err)
		}
	}
	if _, _, err := timeBounds("invalid", now); err == nil {
		t.Fatal("非法时间范围应返回错误")
	}
}

func TestEmbyMonitorRowsRankingAndHeatmap(t *testing.T) {
	rows := []pluginPlaybackRow{
		{Date: "2026-08-30", Time: "10:00:00", UserID: "u1", UserName: "甲", ItemID: "m1", Name: "电影", Type: "Movie", Duration: 120},
		{Date: "2026-08-30", Time: "11:00:00", UserID: "u1", UserName: "甲", ItemID: "m1", Name: "电影", Type: "Movie", Duration: 60},
		{Date: "2026-08-30", Time: "11:30:00", UserID: "u2", UserName: "乙", ItemID: "e1", Name: "剧集", Type: "Episode", Duration: 90},
	}
	ranking := rowsRanking(rows, "media", "movie")
	if len(ranking) != 1 || ranking[0].WatchedSeconds != 180 || ranking[0].PlayCount != 2 || ranking[0].Percentage != 100 {
		t.Fatalf("媒体排行聚合错误: %#v", ranking)
	}
	heatmap := heatmapFromRows(rows, "u1", time.FixedZone("CST", 8*3600))
	if len(heatmap) != 1 || heatmap[0].TotalSeconds != 180 || len(heatmap[0].Cells) != 2 {
		t.Fatalf("热力图聚合错误: %#v", heatmap)
	}
}

func TestSplitPlaybackHoursCrossesHourBoundary(t *testing.T) {
	start := time.Date(2026, 8, 30, 10, 59, 30, 0, time.UTC)
	values := map[int]int64{}
	splitPlaybackHours(start, 90, func(hour time.Time, seconds int64) { values[hour.Hour()] += seconds })
	if values[10] != 30 || values[11] != 60 {
		t.Fatalf("跨小时切分错误: %#v", values)
	}
}

func TestEmbyMonitorSummaryCountsDistinctUsers(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	today := time.Date(2026, 8, 30, 0, 0, 0, 0, loc)
	week := time.Date(2026, 8, 24, 0, 0, 0, 0, loc)
	month := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	rows := []pluginPlaybackRow{
		{Date: "2026-08-30", Time: "09:00:00", UserID: "u1", Duration: 60},
		{Date: "2026-08-30", Time: "10:00:00", UserID: "u1", Duration: 30},
		{Date: "2026-08-29", Time: "10:00:00", UserID: "u2", Duration: 90},
	}
	result := summaryFromRows(rows, today, week, month)
	if result.TodaySeconds != 90 || result.WeekSeconds != 180 || result.TodayActiveUsers != 1 || result.WeekActiveUsers != 2 {
		t.Fatalf("汇总统计错误: %#v", result)
	}
}
