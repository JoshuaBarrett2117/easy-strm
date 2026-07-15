package service

import (
	"encoding/json"
	"testing"
)

func TestScrapeSidecarNames(t *testing.T) {
	got := sidecarNames("Movies/Inception.2010.mkv", "-poster.jpg")
	if len(got) != 2 {
		t.Fatalf("expected base and generic poster names, got=%v", got)
	}
	if got[0] != "Inception.2010-poster.jpg" || got[1] != "poster.jpg" {
		t.Fatalf("unexpected sidecar names: %v", got)
	}
}

func TestExtractTmdbID(t *testing.T) {
	if got := extractTmdbID(json.RawMessage(`{"tmdb_id":123}`)); got != 123 {
		t.Fatalf("extractTmdbID(tmdb_id)=%d, want 123", got)
	}
	if got := extractTmdbID(json.RawMessage(`{"id":456}`)); got != 456 {
		t.Fatalf("extractTmdbID(id)=%d, want 456", got)
	}
	if got := extractTmdbID(json.RawMessage(`{bad json}`)); got != 0 {
		t.Fatalf("extractTmdbID(invalid)=%d, want 0", got)
	}
}

func TestExtractActorsOrdersAndLimits(t *testing.T) {
	cast := []tmdbCast{
		{Name: "Third", Character: "C", Order: 3},
		{Name: "First", Character: "A", Order: 1, ProfilePath: "/a.jpg"},
		{Name: "Second", Character: "B", Order: 2},
	}

	actors := extractActors(cast)
	if len(actors) != 3 {
		t.Fatalf("unexpected actor count: %d", len(actors))
	}
	if actors[0].Name != "First" || actors[0].Role != "A" {
		t.Fatalf("actors should be ordered by TMDB order, got=%+v", actors)
	}
	if actors[0].Thumb != "https://image.tmdb.org/t/p/original/a.jpg" {
		t.Fatalf("unexpected actor thumb: %s", actors[0].Thumb)
	}
}

func TestNeedsDetailRefresh(t *testing.T) {
	if !needsMovieDetailRefresh(&tmdbMovieDetail{ID: 1, PosterPath: "https://remote/poster.jpg"}) {
		t.Fatal("remote poster should require movie detail refresh")
	}
	if needsMovieDetailRefresh(&tmdbMovieDetail{
		ID:           1,
		BackdropPath: "/backdrop.jpg",
		Runtime:      120,
		Genres:       []tmdbGenre{{ID: 1, Name: "Drama"}},
		Credits:      &tmdbCredits{},
		PosterPath:   "/poster.jpg",
	}) {
		t.Fatal("complete movie detail should not require refresh")
	}
	if !needsTVDetailRefresh(&tmdbTVDetail{ID: 1, PosterPath: "/poster.jpg"}) {
		t.Fatal("missing tv backdrop/genres/credits should require refresh")
	}
}
