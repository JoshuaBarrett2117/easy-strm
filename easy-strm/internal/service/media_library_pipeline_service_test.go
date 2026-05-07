package service

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPipelineStrmPath(t *testing.T) {
	path := buildPipelineStrmPath(`D:\strm`, "Movies/Smoke/Movie.Smoke.2026.mkv", "Movie.Smoke.2026.mkv")
	expectedSuffix := filepath.Join("Movies", "Smoke", "Movie.Smoke.2026.strm")
	if !strings.HasSuffix(path, expectedSuffix) {
		t.Fatalf("expected suffix %s, got %s", expectedSuffix, path)
	}
}

func TestNormalizePipelineMediaType(t *testing.T) {
	if got := normalizePipelineMediaType("anime"); got != "tv" {
		t.Fatalf("expected anime to map to tv, got %s", got)
	}
	if got := normalizePipelineMediaType(""); got != "movie" {
		t.Fatalf("expected empty media type to map to movie, got %s", got)
	}
}
