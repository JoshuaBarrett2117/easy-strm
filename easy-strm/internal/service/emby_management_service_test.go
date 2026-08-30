package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

const testEmbyServerSelect = `SELECT id, name, base_url, api_key, enabled, is_default, create_time, update_time FROM t_emby_server WHERE id=$1`

func newEmbyManagementTestService(t *testing.T, handler http.HandlerFunc) (*EmbyManagementService, sqlmock.Sqlmock, *TaskService, func()) {
	t.Helper()
	database, sqlMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	dao.InitTaskRedisDAO(redisClient)
	taskService := NewTaskService(dao.NewTaskRedisDAO(redisClient))
	server := httptest.NewServer(handler)
	management := NewEmbyManagementService(dao.NewEmbyServerDAO(database), taskService, &mockSystemConfigDAO{configs: map[string]string{}}, server.Client())
	cleanup := func() { server.Close(); redisClient.Close(); mini.Close(); database.Close() }
	return management, sqlMock, taskService, cleanup
}

func expectEmbyServer(t *testing.T, mock sqlmock.Sqlmock, endpoint string) {
	t.Helper()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(testEmbyServerSelect)).WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}).
			AddRow(1, "家庭 Emby", endpoint, "test-key", true, true, now, now))
}

func TestEmbyManagementListUsersUsesSelectedServer(t *testing.T) {
	management, mock, _, cleanup := newEmbyManagementTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Users" {
			t.Errorf("请求路径错误: %s", r.URL.Path)
		}
		if r.Header.Get("X-Emby-Token") != "test-key" {
			t.Errorf("未携带实例 API Key")
		}
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"Id": "u1", "Name": "Alice", "Policy": map[string]interface{}{"EnableAllFolders": true}}})
	}))
	defer remote.Close()
	management.httpClient = remote.Client()
	expectEmbyServer(t, mock, remote.URL)
	users, err := management.ListUsers(1)
	if err != nil {
		t.Fatalf("查询用户失败: %v", err)
	}
	if len(users) != 1 || users[0].Name != "Alice" || !users[0].Policy.EnableAllFolders {
		t.Fatalf("用户响应不符合预期: %#v", users)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbyRefreshTaskTracksProgressAndConclusion(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/System/Info":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"Id": "s1", "ServerName": "Test", "Version": "4.8.10"})
		case "/emby/Items/lib-1/Refresh":
			w.WriteHeader(http.StatusNoContent)
		case "/emby/Library/VirtualFolders":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"Name": "Movies", "ItemId": "lib-1", "RefreshStatus": "Idle", "RefreshProgress": 0}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	management, mock, tasks, cleanup := newEmbyManagementTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()
	management.httpClient = remote.Client()
	management.pollInterval = 10 * time.Millisecond
	management.pollTimeout = time.Second
	for i := 0; i < 3; i++ {
		expectEmbyServer(t, mock, remote.URL)
	}
	taskID, err := management.StartRefresh(1, "lib-1")
	if err != nil {
		t.Fatalf("提交刷新任务失败: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, getErr := tasks.Get(taskID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if task != nil && task["status"] == "success" {
			if int(task["progress"].(float64)) != 100 {
				t.Fatalf("任务进度不是 100: %#v", task)
			}
			metadata := task["metadata"].(map[string]interface{})
			if metadata["conclusion"] != "Emby 媒体库刷新成功" {
				t.Fatalf("缺少最终结论: %#v", metadata)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("刷新任务未在期限内结束")
}

func TestEmbyRefreshAllRecordsPartialSuccess(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/System/Info":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"Id": "s1", "ServerName": "Test", "Version": "4.8.10"})
		case "/emby/Library/VirtualFolders":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Name": "Movies", "ItemId": "lib-1", "RefreshStatus": "Idle", "RefreshProgress": 0},
				{"Name": "Shows", "ItemId": "lib-2", "RefreshStatus": "Idle", "RefreshProgress": 0},
			})
		case "/emby/Items/lib-1/Refresh":
			w.WriteHeader(http.StatusNoContent)
		case "/emby/Items/lib-2/Refresh":
			http.Error(w, "refresh rejected", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	management, mock, tasks, cleanup := newEmbyManagementTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()
	management.httpClient = remote.Client()
	management.pollInterval = 10 * time.Millisecond
	management.pollTimeout = time.Second
	for i := 0; i < 4; i++ {
		expectEmbyServer(t, mock, remote.URL)
	}
	taskID, err := management.StartRefresh(1, "")
	if err != nil {
		t.Fatalf("提交全部刷新任务失败: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, getErr := tasks.Get(taskID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if task != nil && task["status"] == "partial_success" {
			metadata := task["metadata"].(map[string]interface{})
			if metadata["conclusion"] != "媒体库刷新部分成功：1 个成功，1 个失败" {
				t.Fatalf("部分成功结论不符合预期: %#v", metadata)
			}
			failedItems, ok := metadata["failed_items"].([]interface{})
			if !ok || len(failedItems) != 1 {
				t.Fatalf("失败媒体库明细不符合预期: %#v", metadata["failed_items"])
			}
			if task["success_files"] != float64(1) || task["failed_files"] != float64(1) {
				t.Fatalf("任务成功/失败计数不符合预期: %#v", task)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("全部刷新任务未在期限内得到部分成功终态")
}

func TestStrmScanCaptureWorkflowTracksCoverResult(t *testing.T) {
	var scheduledReads atomic.Int32
	var itemReads atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/Plugins":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"Name": "Strm Assistant", "Version": "2.3.0"}})
		case "/emby/ScheduledTasks":
			read := scheduledReads.Add(1)
			state := "Idle"
			progress := 100
			if read == 2 {
				state = "Running"
				progress = 50
			}
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"Id": "media-info-task", "Name": "Extract MediaInfo", "Category": "Strm Assistant",
				"State": state, "CurrentProgressPercentage": progress,
				"LastExecutionResult": map[string]interface{}{"Status": "Completed"},
			}})
		case "/emby/Items/lib-1/Refresh", "/emby/ScheduledTasks/Running/media-info-task":
			w.WriteHeader(http.StatusNoContent)
		case "/emby/Library/VirtualFolders":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"Name": "Movies", "ItemId": "lib-1", "RefreshStatus": "Idle", "RefreshProgress": 0}})
		case "/emby/Items":
			covered := itemReads.Add(1) > 1
			secondTags := map[string]string{}
			if covered {
				secondTags["Primary"] = "new-cover"
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"TotalRecordCount": 2,
				"Items": []map[string]interface{}{
					{"Path": "/media/a.strm", "ImageTags": map[string]string{"Primary": "old-cover"}},
					{"Path": "/media/b.strm", "ImageTags": secondTags},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	management, mock, tasks, cleanup := newEmbyManagementTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()
	management.httpClient = remote.Client()
	management.pollInterval = 10 * time.Millisecond
	management.pollTimeout = time.Second
	for i := 0; i < 2; i++ {
		expectEmbyServer(t, mock, remote.URL)
	}
	taskID, err := management.StartStrmAssistantTask(1, strmScanCaptureAction, "lib-1")
	if err != nil {
		t.Fatalf("提交 STRM 扫描截图任务失败: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, getErr := tasks.Get(taskID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if task != nil && task["status"] == "success" {
			metadata := task["metadata"].(map[string]interface{})
			if metadata["strm_count"] != float64(2) || metadata["covered_before"] != float64(1) || metadata["covered_after"] != float64(2) || metadata["generated_cover_count"] != float64(1) {
				t.Fatalf("STRM 封面统计不符合预期: %#v", metadata)
			}
			if metadata["conclusion"] != "STRM 扫描与截图任务已完成：共 2 个 STRM 视频，新增 1 个主图，仍有 0 个缺少主图" {
				t.Fatalf("任务结论不符合预期: %#v", metadata)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("STRM 扫描截图任务未在期限内结束")
}

func TestEnableLibraryImageCaptureOnlyChangesApplicableVideoType(t *testing.T) {
	options := map[string]interface{}{
		"PreferredMetadataLanguage": "zh-CN",
		"TypeOptions": []interface{}{
			map[string]interface{}{"Type": "Series", "ImageFetchers": []interface{}{"TheMovieDb"}, "ImageFetcherOrder": []interface{}{"TheMovieDb"}},
			map[string]interface{}{"Type": "Episode", "ImageFetchers": []interface{}{"TheMovieDb"}, "ImageFetcherOrder": []interface{}{"TheMovieDb", "Image Capture"}},
		},
	}
	changed, err := enableLibraryImageCapture("tvshows", options)
	if err != nil || !changed {
		t.Fatalf("启用 Image Capture 失败: changed=%v err=%v", changed, err)
	}
	typeOptions := options["TypeOptions"].([]interface{})
	series := typeOptions[0].(map[string]interface{})
	episode := typeOptions[1].(map[string]interface{})
	if containsString(interfaceStringSlice(series["ImageFetchers"]), "Image Capture") {
		t.Fatalf("不应修改 Series 的 ImageFetchers: %#v", series)
	}
	if !containsString(interfaceStringSlice(episode["ImageFetchers"]), "Image Capture") {
		t.Fatalf("Episode 的 Image Capture 未启用: %#v", episode)
	}
	if options["PreferredMetadataLanguage"] != "zh-CN" {
		t.Fatalf("修改时丢失了无关媒体库设置: %#v", options)
	}
	changed, err = enableLibraryImageCapture("tvshows", options)
	if err != nil || changed {
		t.Fatalf("重复启用应保持幂等: changed=%v err=%v", changed, err)
	}
}

func TestMergeStrmAssistantLibraryScopePreservesBlankAndAvoidsDuplicates(t *testing.T) {
	allLibraries := map[string]interface{}{"LibraryScope": "", "EnableImageCapture": true, "CustomProbePath": "/opt/ffprobe"}
	if mergeStrmAssistantLibraryScope(allLibraries, "lib-3") {
		t.Fatalf("空范围代表全部媒体库，不应收窄范围: %#v", allLibraries)
	}
	if allLibraries["LibraryScope"] != "" || allLibraries["CustomProbePath"] != "/opt/ffprobe" {
		t.Fatalf("空范围或无关设置被意外修改: %#v", allLibraries)
	}

	selected := map[string]interface{}{"LibraryScope": "lib-1, lib-2", "EnableImageCapture": false, "CustomProbePath": "/opt/ffprobe"}
	if !mergeStrmAssistantLibraryScope(selected, "lib-3") {
		t.Fatal("应合并所选媒体库并启用插件截图能力")
	}
	if selected["LibraryScope"] != "lib-1,lib-2,lib-3" || selected["EnableImageCapture"] != true {
		t.Fatalf("Library Scope 合并结果错误: %#v", selected)
	}
	if selected["CustomProbePath"] != "/opt/ffprobe" {
		t.Fatalf("合并时丢失了无关插件设置: %#v", selected)
	}
	if mergeStrmAssistantLibraryScope(selected, "lib-3") {
		t.Fatalf("已存在的媒体库不应重复加入: %#v", selected)
	}
}

func TestConfigureStrmCaptureDependenciesWritesAndReadsBack(t *testing.T) {
	var imageCaptureEnabled atomic.Bool
	pluginScope := "lib-1,lib-2"
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/Library/VirtualFolders":
			fetchers := []string{"TheMovieDb"}
			if imageCaptureEnabled.Load() {
				fetchers = append(fetchers, "Image Capture")
			}
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"Name": "Movies", "ItemId": "lib-3", "CollectionType": "movies", "Locations": []string{"/media/movies"},
				"LibraryOptions": map[string]interface{}{
					"PreferredMetadataLanguage": "zh-CN",
					"TypeOptions":               []map[string]interface{}{{"Type": "Movie", "ImageFetchers": fetchers, "ImageFetcherOrder": []string{"TheMovieDb"}}},
				},
			}})
		case "/emby/Library/VirtualFolders/LibraryOptions":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("解析媒体库配置请求失败: %v", err)
			}
			options := body["LibraryOptions"].(map[string]interface{})
			if options["PreferredMetadataLanguage"] != "zh-CN" {
				t.Errorf("媒体库完整配置未保留: %#v", options)
			}
			imageCaptureEnabled.Store(libraryImageCaptureEnabled("movies", options))
			w.WriteHeader(http.StatusNoContent)
		case "/emby/UI/View":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"PageId": "63c322:MediaInfoExtractPageView",
				"EditObjectContainer": map[string]interface{}{"Object": map[string]interface{}{
					"LibraryScope": pluginScope, "EnableImageCapture": true, "CustomProbePath": "/opt/ffprobe",
				}},
			})
		case "/emby/UI/Command":
			var command map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
				t.Errorf("解析插件配置请求失败: %v", err)
			}
			var options map[string]interface{}
			if err := json.Unmarshal([]byte(command["Data"].(string)), &options); err != nil {
				t.Errorf("解析插件配置 Data 失败: %v", err)
			}
			if options["CustomProbePath"] != "/opt/ffprobe" {
				t.Errorf("插件无关配置未保留: %#v", options)
			}
			pluginScope = options["LibraryScope"].(string)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"PageId": command["PageId"]})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	management, _, _, cleanup := newEmbyManagementTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()
	management.httpClient = remote.Client()
	server := &domain.EmbyServer{ID: 1, Name: "Test", BaseURL: remote.URL, APIKey: "test-key", Enabled: true}
	metadata := map[string]interface{}{"steps": buildEmbySteps("检测", "读取", "Image Capture", "Library Scope", "核验")}
	if err := management.configureStrmCaptureDependencies(server, "lib-3", "not-persisted", metadata); err != nil {
		t.Fatalf("自动配置失败: %v", err)
	}
	if !imageCaptureEnabled.Load() || pluginScope != "lib-1,lib-2,lib-3" || metadata["configuration_verified"] != true {
		t.Fatalf("自动配置或回读结果错误: image=%v scope=%s metadata=%#v", imageCaptureEnabled.Load(), pluginScope, metadata)
	}
}
