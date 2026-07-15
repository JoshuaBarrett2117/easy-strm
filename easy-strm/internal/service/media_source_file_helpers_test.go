package service

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

func TestMediaSourceServiceFileHelpers(t *testing.T) {
	service := &MediaSourceService{}

	if got := service.joinRelativePath("Movies", "A.mkv"); got != "Movies/A.mkv" && got != "Movies\\A.mkv" {
		t.Fatalf("joinRelativePath() = %q", got)
	}
	if got := mediaFileTypeByExt(".mkv"); got != "video" {
		t.Fatalf("mediaFileTypeByExt video = %q", got)
	}
	if got := mediaFileTypeByExt(".unknown"); got != "file" {
		t.Fatalf("mediaFileTypeByExt file = %q", got)
	}
	if got := service.fileTypeFrom115("subtitle.srt", false); got != "subtitle" {
		t.Fatalf("fileTypeFrom115 subtitle = %q", got)
	}

	files := []domain.MediaFile{
		{ID: "b", Name: "Beta.mkv", Type: "video", Size: 20, ModifiedTime: time.Unix(20, 0)},
		{ID: "a", Name: "Alpha.mkv", Type: "video", Size: 10, ModifiedTime: time.Unix(10, 0)},
		{ID: "d", Name: "Directory", Type: "dir", IsDirectory: true, ModifiedTime: time.Unix(30, 0)},
	}

	filtered := service.filterFiles(files, "video", "alpha")
	if len(filtered) != 1 || filtered[0].ID != "a" {
		t.Fatalf("filterFiles() = %+v, want only alpha video", filtered)
	}

	sorted := service.sortFiles(files, "size", "desc")
	gotOrder := []string{sorted[0].ID, sorted[1].ID, sorted[2].ID}
	wantOrder := []string{"d", "b", "a"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("sortFiles() order = %v, want %v", gotOrder, wantOrder)
	}

	breadcrumb := service.buildBreadcrumb(filepath.Join("Movies", "Action"))
	if len(breadcrumb) == 0 || breadcrumb[len(breadcrumb)-1].Name != "Action" {
		t.Fatalf("buildBreadcrumb() = %+v, want Action tail", breadcrumb)
	}
}
