package service

import (
	"easy-strm/internal/domain"
	"testing"
)

func TestValidateLibraryQueryNormalizesPaging(t *testing.T) {
	q := domain.ShareLibraryQuery{}
	if err := ValidateLibraryQuery(&q); err != nil {
		t.Fatal(err)
	}
	if q.Page != 1 || q.PageSize != 24 {
		t.Fatalf("unexpected defaults: %+v", q)
	}
}

func TestValidateLibraryQueryRejectsUnsafeFilters(t *testing.T) {
	bad := []domain.ShareLibraryQuery{{PageSize: 101}, {MediaType: "music"}, {RatingMin: float64Ptr(-1)}, {Genres: "28,x"}, {Countries: "CHN"}, {Sort: "random"}}
	for _, q := range bad {
		if err := ValidateLibraryQuery(&q); err == nil {
			t.Fatalf("expected validation failure for %+v", q)
		}
	}
}

func float64Ptr(v float64) *float64 { return &v }
