package service

import (
	"easy-strm/internal/domain"
	"testing"
)

func TestShareGalleryGroupsTVIdentity(t *testing.T) {
	result := func(id int, kind string) *domain.TmdbIdentifyResult {
		return &domain.TmdbIdentifyResult{Success: true, MediaType: kind, TmdbID: id, Title: "同名"}
	}
	record := domain.ShareRecord{Media: []domain.ShareMedia{
		{ID: 1, Status: "identified", Result: result(1, "tv")},
		{ID: 2, Status: "identified", Result: result(1, "tv")},
		{ID: 3, Status: "identified", Result: result(2, "tv")},
		{ID: 4, Status: "identified", Result: result(1, "movie")},
		{ID: 5, Status: "identified", Result: result(1, "movie")},
		{ID: 6, Status: "identified", Result: result(0, "tv")},
		{ID: 7, Status: "failed", Result: result(1, "tv")},
	}}
	normalizeShareGallery(&record)
	for i, m := range record.Media {
		if m.GalleryDuplicate != (i == 1) {
			t.Fatalf("row=%d %+v", i, m)
		}
	}
	record.Media[1].Result = result(3, "tv")
	normalizeShareGallery(&record)
	if record.Media[1].GalleryDuplicate {
		t.Fatal("手动修正后应重新出现")
	}
	other := domain.ShareRecord{Media: record.Media[:1]}
	normalizeShareGallery(&other)
	if other.Media[0].GalleryDuplicate {
		t.Fatal("不同分享不可相互折叠")
	}
}

func TestShareCandidatesSkipCoveredEpisodes(t *testing.T) {
	files := []domain.ShareFileInfo{
		{Name: "一路繁花 (2025)", Path: "综艺/一路繁花 (2025)", IsDir: true, Type: "media"},
		{Name: "一路繁花.S01E05.Extra.mp4", Path: "综艺\\一路繁花 (2025)\\一路繁花.S01E05.Extra.mp4", Type: "video"},
		{Name: "一路繁花.S01E06.mp4", Path: "综艺/一路繁花 (2025)/一路繁花.S01E06.mp4", Type: "video"},
		{Name: "别的.S01E01.mp4", Path: "别的.S01E01.mp4", Type: "video"},
		{Name: "电影.mp4", Path: "综艺/一路繁花 (2025)/电影.mp4", Type: "video"},
	}
	got := selectShareIdentifyCandidates(files)
	if len(got) != 3 || got[0].Name != files[0].Name || got[1].Name != files[3].Name || got[2].Name != files[4].Name {
		t.Fatalf("%+v", got)
	}
}

func TestShareCandidatesCoverSeriesDiscsButKeepMovieISOs(t *testing.T) {
	files := []domain.ShareFileInfo{
		{Name: "七龙珠153集全", Path: "七龙珠153集全", IsDir: true, Type: "media"},
		{Name: "Dragonball.1986.D11.iso", Path: "七龙珠153集全/Dragonball.1986.D11.iso", Type: "video"},
		{Name: "电影ISO-01", Path: "电影ISO-01", IsDir: true, Type: "media"},
		{Name: "电影.2013.iso", Path: "电影ISO-01/电影.2013.iso", Type: "video"},
	}
	got := selectShareIdentifyCandidates(files)
	if len(got) != 2 || got[0].Name != "七龙珠153集全" || got[1].Name != "电影.2013.iso" {
		t.Fatalf("%+v", got)
	}
}
