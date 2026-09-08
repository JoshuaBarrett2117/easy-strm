package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"easy-strm/internal/domain"
)

func TestEmbyUserAccessRoundTrip(t *testing.T) {
	const guid = "eef44ceeaf834d4293445eef5872ba74"
	for _, tc := range []struct {
		name      string
		all       bool
		ids       []string
		ignore    bool
		wantError bool
	}{
		{"数字ID映射到权限Guid", false, []string{"127953"}, false, false},
		{"权限Guid直接保存", false, []string{guid}, false, false},
		{"全库开关覆盖历史选择", true, []string{guid}, false, false},
		{"取消全部选择", false, []string{}, false, false},
		{"Emby忽略写入必须失败", false, []string{guid}, true, true},
		{"未知ID不应写入", false, []string{"unknown"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := map[string]interface{}{"EnableAllFolders": true, "EnabledFolders": []string{}, "EnableLiveTvAccess": true}
			posts := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/emby/Library/SelectableMediaFolders":
					json.NewEncoder(w).Encode([]map[string]interface{}{{"Name": "电影", "Id": "127953", "Guid": guid, "IsUserAccessConfigurable": true}})
				case r.URL.Path == "/emby/Users/u1" && r.Method == http.MethodGet:
					json.NewEncoder(w).Encode(map[string]interface{}{"Id": "u1", "Name": "115tv", "Policy": state})
				case r.URL.Path == "/emby/Users/u1" && r.Method == http.MethodPost:
					w.WriteHeader(http.StatusNoContent)
				case r.URL.Path == "/emby/Users/u1/Policy" && r.Method == http.MethodPost:
					posts++
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["EnableLiveTvAccess"] != true {
						t.Error("未保留原有非编辑权限")
					}
					expected := []interface{}{}
					if !tc.all && len(tc.ids) > 0 {
						expected = []interface{}{guid}
					}
					if !reflect.DeepEqual(body["EnabledFolders"], expected) {
						t.Errorf("权限ID错误: %#v", body["EnabledFolders"])
					}
					if !tc.ignore {
						state = body
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					http.NotFound(w, r)
				}
			}))
			defer remote.Close()
			service, mock, _, cleanup := newEmbyManagementTestService(t, func(w http.ResponseWriter, r *http.Request) {})
			defer cleanup()
			expectEmbyServer(t, mock, remote.URL)
			_, err := service.UpdateUser(1, "u1", "115tv", &domain.EmbyUserPolicy{EnableAllFolders: tc.all, EnabledFolders: tc.ids})
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v，wantError=%v", err, tc.wantError)
			}
			if tc.name == "未知ID不应写入" && posts != 0 {
				t.Fatal("未知ID已提交")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestListUserLibrariesUsesPermissionGuid(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		status    int
		wantError bool
	}{
		{"名称和Guid映射", `[{"Id":"127953","Guid":"eef44ceeaf834d4293445eef5872ba74","Name":"电影","IsUserAccessConfigurable":true},{"Id":"2","Guid":"hidden","Name":"系统库","IsUserAccessConfigurable":false}]`, 200, false},
		{"缺少Guid不能回退数字ID", `[{"Id":"127953","Name":"电影","IsUserAccessConfigurable":true}]`, 200, true},
		{"上游请求失败", `{"error":"unavailable"}`, 502, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/emby/Library/SelectableMediaFolders" || r.Header.Get("X-Emby-Token") != "test-key" {
					t.Errorf("错误的权限请求: %s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer remote.Close()
			service, mock, _, cleanup := newEmbyManagementTestService(t, func(w http.ResponseWriter, r *http.Request) {})
			defer cleanup()
			expectEmbyServer(t, mock, remote.URL)
			folders, err := service.ListUserLibraries(1)
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v", err)
			}
			if !tc.wantError && (len(folders) != 1 || folders[0].ID != "eef44ceeaf834d4293445eef5872ba74" || folders[0].ItemID != "127953" || folders[0].Name != "电影") {
				t.Fatalf("错误的权限选项: %#v", folders)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
