package service

import (
	"easy-strm/internal/domain"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateLibraryIncludesIDAndPreservesOptions(t *testing.T) {
	const id = "eef44ceeaf834d4293445eef5872ba74"
	saved := false
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/Library/VirtualFolders":
			json.NewEncoder(w).Encode([]map[string]interface{}{{"ItemId": id, "Name": "电影", "CollectionType": "movies", "Locations": []string{"/media"}, "LibraryOptions": map[string]interface{}{"EnablePhotos": true}}})
		case "/emby/Library/VirtualFolders/LibraryOptions":
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if body["Id"] != id {
				http.Error(w, "Unrecognized Guid format.", 500)
				return
			}
			options, _ := body["LibraryOptions"].(map[string]interface{})
			if options["EnablePhotos"] != true || options["PreferredMetadataLanguage"] != "zh-CN" {
				t.Errorf("配置未保留: %v", options)
			}
			saved = true
			w.WriteHeader(204)
		default:
			t.Errorf("意外请求: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	svc, mock, _, cleanup := newEmbyManagementTestService(t, func(http.ResponseWriter, *http.Request) {})
	defer cleanup()
	expectEmbyServer(t, mock, remote.URL)
	_, err := svc.UpdateLibrary(1, id, domain.EmbyLibraryInput{Name: "电影", CollectionType: "movies", Paths: []domain.EmbyMediaPath{{Path: "/media"}}, MetadataLanguage: "zh-CN"})
	if err != nil || !saved {
		t.Fatalf("保存失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
