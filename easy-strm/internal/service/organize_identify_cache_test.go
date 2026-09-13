package service

import (
	"easy-strm/internal/domain"
	"testing"
)

func TestBuildIdentifyCacheKeepsPoster(t *testing.T) {
	result := &domain.TmdbIdentifyResult{MediaType: "movie", TmdbID: 10, Title: "影片", PosterPath: "/poster.jpg"}
	cache := buildIdentifyCache("hash", "影片.mkv", result, 7, true)
	if cache.PosterPath != "/poster.jpg" || cache.TmdbID != 10 || cache.SourceID != 7 || !cache.IsManual {
		t.Fatalf("识别缓存字段丢失: %+v", cache)
	}
}
