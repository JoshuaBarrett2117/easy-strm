package controller

import (
	"reflect"
	"testing"

	"easy-strm/internal/domain"
)

func TestResolveCloud115CID(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		sourcePath string
		want       string
	}{
		{name: "uses explicit path", path: "123", sourcePath: "456", want: "123"},
		{name: "uses source root when path empty", path: "", sourcePath: "456", want: "456"},
		{name: "uses source root when path slash", path: "/", sourcePath: "456", want: "456"},
		{name: "falls back to cloud root", path: "", sourcePath: "/", want: "0"},
		{name: "falls back when both empty", path: "", sourcePath: "", want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveCloud115CID(tt.path, tt.sourcePath); got != tt.want {
				t.Fatalf("resolveCloud115CID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMediaSourceFileHelpers(t *testing.T) {
	controller := &MediaSourceController{}

	if got := controller.fileTypeFrom115("movie.mkv", false); got != "video" {
		t.Fatalf("fileTypeFrom115 video = %q", got)
	}
	if got := controller.fileTypeFrom115("folder", true); got != "dir" {
		t.Fatalf("fileTypeFrom115 dir = %q", got)
	}
	if got := getFileExt("archive.tar.gz"); got != ".gz" {
		t.Fatalf("getFileExt() = %q", got)
	}

	files := []domain.MediaFile{
		{ID: "2", Name: "Beta.mkv", Type: "video", Size: 20},
		{ID: "1", Name: "Alpha.srt", Type: "subtitle", Size: 5},
		{ID: "3", Name: "Folder", Type: "dir", IsDirectory: true},
	}

	filtered := controller.filterFiles(files, "video", "beta")
	if len(filtered) != 1 || filtered[0].ID != "2" {
		t.Fatalf("filterFiles() = %+v, want only Beta video", filtered)
	}

	sorted := controller.sortFiles(files, "name", "asc")
	gotOrder := []string{sorted[0].ID, sorted[1].ID, sorted[2].ID}
	wantOrder := []string{"3", "1", "2"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("sortFiles() order = %v, want %v", gotOrder, wantOrder)
	}

	breadcrumb := controller.buildBreadcrumb("Movies/Action")
	wantBreadcrumb := []domain.PathItem{
		{Name: "Movies", Path: "Movies"},
		{Name: "Action", Path: "Movies/Action"},
	}
	if !reflect.DeepEqual(breadcrumb, wantBreadcrumb) {
		t.Fatalf("buildBreadcrumb() = %+v, want %+v", breadcrumb, wantBreadcrumb)
	}
}
