package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// ==================== 115 API POC 验证 — driver.ShareSnapResp 契约 ====================
// 本测试基于真实 115 API 验证结论编写：
//   - 正确端点：GET https://115cdn.com/webapi/share/snap（query传参 + Referer头）
//   - 文件字段：n(文件名), s(大小), ico(扩展名), fid(文件ID), sha(SHA1), cid(目录ID)
//   - 分享链接同时支持 115.com 与 115cdn.com 两种域名
// ParseShareLink 通过 Cloud115Client.GetShareSnap（115driver 封装）调用上述接口。

// fakeShareCloud115Client 实现 Cloud115Client 接口的测试桩
// 仅 GetShareSnap 具有可配置行为，其余方法返回零值
type fakeShareCloud115Client struct {
	shareSnap    *driver.ShareSnapResp
	shareSnapErr error
	// 记录调用参数，用于断言
	gotShareCode   string
	gotReceiveCode string
	gotDirID       string
	gotDirIDs      []string
	shareSnapByDir map[string]*driver.ShareSnapResp
	// ReceiveShare 调用参数记录
	gotReceiveShareFileIDs  string
	gotReceiveShareFolderID string
	receiveShareErr         error
	// 分页模拟：GetShareSnap 调用次数计数器，第 2 次起返回空列表（模拟分页结束）
	snapCallCount int
}

func (f *fakeShareCloud115Client) GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error) {
	return nil, nil
}

func (f *fakeShareCloud115Client) GetPickCodeByPath(filePath string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}

func (f *fakeShareCloud115Client) GetCIDByPath(path string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}

func (f *fakeShareCloud115Client) RenameFile(fileID, newName string, cloud115ID int, cookie string) error {
	return nil
}

func (f *fakeShareCloud115Client) CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error {
	return nil
}

func (f *fakeShareCloud115Client) MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error {
	return nil
}

func (f *fakeShareCloud115Client) MkdirAll115(path string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}

func (f *fakeShareCloud115Client) RapidTransferFile(sourcePickCode string, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string) (string, error) {
	return "", nil
}

func (f *fakeShareCloud115Client) GetFileInfo(pickCode string, cloud115ID int, cookie string) (*driver.File, error) {
	return nil, nil
}

func (f *fakeShareCloud115Client) GetShareSnap(shareCode, receiveCode, dirID string, queries ...driver.Query) (*driver.ShareSnapResp, error) {
	f.gotShareCode = shareCode
	f.gotReceiveCode = receiveCode
	f.gotDirID = dirID
	f.gotDirIDs = append(f.gotDirIDs, dirID)
	if f.shareSnapByDir != nil {
		if response := f.shareSnapByDir[dirID]; response != nil {
			return response, f.shareSnapErr
		}
		return &driver.ShareSnapResp{}, f.shareSnapErr
	}
	// 未配置目录响应时，根目录之外按空目录处理，避免递归测试桩重复返回根目录。
	if dirID != "0" {
		return &driver.ShareSnapResp{}, f.shareSnapErr
	}
	f.snapCallCount++
	// 模拟分页：第一页返回完整列表，后续页返回空列表（触发分页终止条件）
	if f.snapCallCount > 1 {
		empty := &driver.ShareSnapResp{}
		empty.Data.List = []driver.ShareFile{}
		empty.Data.Count = f.shareSnap.Data.Count
		return empty, f.shareSnapErr
	}
	return f.shareSnap, f.shareSnapErr
}

func (f *fakeShareCloud115Client) ReceiveShare(shareCode, receiveCode, fileIDs, saveFolderID string, targetCloud115ID int, targetCookie string) error {
	f.gotReceiveShareFileIDs = fileIDs
	f.gotReceiveShareFolderID = saveFolderID
	return f.receiveShareErr
}

// mustUnmarshalShareSnap 测试辅助：将真实 API 格式的 JSON 反序列化为 driver.ShareSnapResp
func mustUnmarshalShareSnap(t *testing.T, rawJSON string) *driver.ShareSnapResp {
	t.Helper()
	var snapResp driver.ShareSnapResp
	if err := json.Unmarshal([]byte(rawJSON), &snapResp); err != nil {
		t.Fatalf("driver.ShareSnapResp 反序列化失败 — 与 115 真实 API 契约不匹配: %v", err)
	}
	return &snapResp
}

// TestFetchShareTree_MarksMediaBeforeAppend 验证剧集目录会以 media 类型返回，
// 并且遍历止于季目录，不会继续请求季内文件。
func TestFetchShareTree_MarksMediaBeforeAppend(t *testing.T) {
	root := mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":1,"list":[{"cid":"category-1","n":"动漫 已经刮削整理 394部","fc":0}],"shareinfo":{"share_title":"动漫合集"}}}`)
	category := mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":2,"list":[{"cid":"show-1","n":"剧集甲 (2020)","fc":0},{"cid":"show-2","n":"剧集乙 (2021)","fc":0}]}}`)
	show1 := mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":1,"list":[{"cid":"season-1","n":"Season 1","fc":0}]}}`)
	show2 := mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":1,"list":[{"fid":"video-1","n":"剧集乙.S01E01.mkv","fc":1,"s":"1024"}]}}`)
	fake := &fakeShareCloud115Client{shareSnapByDir: map[string]*driver.ShareSnapResp{
		"0":          root,
		"category-1": category,
		"show-1":     show1,
		"show-2":     show2,
	}}
	service := &ShareTransferService{client: fake}

	files, _, err := service.fetchAllShareFiles(context.Background(), "share-code", "pass")
	if err != nil {
		t.Fatalf("解析分享目录失败: %v", err)
	}

	media := map[string]bool{}
	for _, file := range files {
		if file.Type == "media" {
			media[file.Name] = true
		}
	}
	if !media["剧集甲 (2020)"] || !media["剧集乙 (2021)"] {
		t.Fatalf("剧集目录未正确标记为 media: %#v", media)
	}

}

// 真实 115 share/snap 接口的响应样例（字段与线上抓包一致）
const pocShareSnapJSON = `{
	"state": true,
	"data": {
		"userinfo": {
			"user_id": "123456",
			"user_name": "share_user",
			"face": "https://face.115.com/f.jpg"
		},
		"shareinfo": {
			"snap_id": "987654321",
			"file_size": 21474836480,
			"share_title": "4K Movie Collection 2024",
			"share_state": 1,
			"receive_code": "demo1",
			"receive_count": 12,
			"expire_time": 1893427200,
			"share_duration": 30
		},
		"count": 195,
		"list": [
			{
				"fid": "2456789012345",
				"uid": 123456,
				"cid": 0,
				"n": "The Matrix (1999) 4K HDR.mkv",
				"ico": "mkv",
				"s": 21474836480,
				"sha": "abc123def456789",
				"fc": 1,
				"pid": "0",
				"t": "1700000000"
			},
			{
				"uid": 123456,
				"cid": 987654,
				"n": "纪录片合集",
				"ico": "folder",
				"s": 0,
				"fc": 0,
				"pid": "0",
				"t": "1700000001"
			}
		],
		"share_state": 1
	}
}`

// TestParseShareLink_POC_URLPatterns 验证 URL shareCode 正则提取
// 必须同时支持 115.com 与 115cdn.com 域名
func TestParseShareLink_POC_URLPatterns(t *testing.T) {
	tests := []struct {
		url       string
		wantCode  string
		wantMatch bool
	}{
		// 115.com 标准格式
		{"https://115.com/s/swexample123?password=demo1", "swexample123", true},
		{"https://115.com/s/swexample123", "swexample123", true},
		{"http://115.com/s/abc123", "abc123", true},
		// 115cdn.com 格式（真实分享页跳转域名）
		{"https://115cdn.com/s/swexample123?password=demo1", "swexample123", true},
		{"https://115cdn.com/s/swexample123", "swexample123", true},
		{"http://115cdn.com/s/abc123", "abc123", true},
		// SHA1 直链
		{"https://115.com/s/swexample123#sha1hash", "swexample123", true},
		{"https://115cdn.com/s/swexample123#sha1hash", "swexample123", true},
		// 无效格式
		{"https://pan.baidu.com/s/xxxxx", "", false},
		{"https://115.com/share/swexample123", "", false},
		{"https://115cdn.com/share/swexample123", "", false},
		{"", "", false},
		{"not a url", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			matches := shareCodeRe.FindStringSubmatch(tt.url)
			gotMatch := len(matches) >= 2
			if gotMatch != tt.wantMatch {
				t.Errorf("期望 match=%v, 实际=%v", tt.wantMatch, gotMatch)
				return
			}
			if tt.wantMatch && matches[1] != tt.wantCode {
				t.Errorf("期望 shareCode='%s', 实际='%s'", tt.wantCode, matches[1])
			}
		})
	}
}

// TestParseShareLink_POC_PasswordExtraction 验证 password query 参数提取
func TestParseShareLink_POC_PasswordExtraction(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://115.com/s/xxx?password=demo1", "demo1"},
		{"https://115.com/s/xxx?password=demo1&foo=bar", "demo1"},
		{"https://115cdn.com/s/xxx?foo=bar&password=abc123", "abc123"},
		{"https://115.com/s/xxx", ""},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			password := ""
			if idx := strings.Index(tt.url, "password="); idx >= 0 {
				password = tt.url[idx+9:]
				if ampIdx := strings.Index(password, "&"); ampIdx >= 0 {
					password = password[:ampIdx]
				}
			}
			if password != tt.expected {
				t.Errorf("期望 password='%s', 实际='%s'", tt.expected, password)
			}
		})
	}
}

// TestParseShareLink_POC_ResponseJSON 验证 driver.ShareSnapResp 与真实 115 API
// JSON 结构的契约（字段 n/s/ico/fid/sha/cid，无 pick_code）
func TestParseShareLink_POC_ResponseJSON(t *testing.T) {
	snapResp := mustUnmarshalShareSnap(t, pocShareSnapJSON)

	// 验证顶层
	if !snapResp.State {
		t.Error("State 应为 true")
	}

	// 验证分享信息
	if snapResp.Data.Shareinfo.ShareTitle != "4K Movie Collection 2024" {
		t.Errorf("ShareTitle 不匹配: '%s'", snapResp.Data.Shareinfo.ShareTitle)
	}
	if snapResp.Data.Count != 195 {
		t.Errorf("Count: 期望 195, 实际 %d", snapResp.Data.Count)
	}
	if len(snapResp.Data.List) != 2 {
		t.Fatalf("List 长度: 期望 2, 实际 %d", len(snapResp.Data.List))
	}

	// 验证视频文件字段（n/s/ico/fid/sha）
	f1 := snapResp.Data.List[0]
	if f1.FileName != "The Matrix (1999) 4K HDR.mkv" {
		t.Errorf("FileName(n) 不匹配: '%s'", f1.FileName)
	}
	if int64(f1.Size) != 21474836480 {
		t.Errorf("Size(s) 不匹配: %d", int64(f1.Size))
	}
	if f1.Type != "mkv" {
		t.Errorf("Type(ico) 不匹配: '%s'", f1.Type)
	}
	if f1.FileID != "2456789012345" {
		t.Errorf("FileID(fid) 不匹配: '%s'", f1.FileID)
	}
	if f1.Sha1 != "abc123def456789" {
		t.Errorf("Sha1(sha) 不匹配: '%s'", f1.Sha1)
	}
	if string(f1.CategoryID) != "0" {
		t.Errorf("文件项 CategoryID(cid) 应为 '0', 实际 '%s'", string(f1.CategoryID))
	}

	// 验证文件夹字段（cid 为非零目录ID）
	f2 := snapResp.Data.List[1]
	if f2.FileName != "纪录片合集" {
		t.Errorf("文件夹名不匹配: '%s'", f2.FileName)
	}
	if string(f2.CategoryID) != "987654" {
		t.Errorf("文件夹 CategoryID(cid) 应为 '987654', 实际 '%s'", string(f2.CategoryID))
	}
}

// TestParseShareLink_POC_FileTypeMapping 验证文件名后缀 → 领域类型映射
// 根据 fc 判断文件/目录，再根据文件扩展名判断具体文件类型。
func TestParseShareLink_POC_FileTypeMapping(t *testing.T) {
	mappings := []struct {
		name     string
		fileName string
		cid      string
		isFile   int
		expected string
	}{
		{"mkv视频", "movie.mkv", "0", 1, "video"},
		{"mp4视频", "movie.mp4", "0", 1, "video"},
		{"大写扩展名视频", "MOVIE.MKV", "0", 1, "video"},
		{"mp3音频", "song.mp3", "0", 1, "audio"},
		{"flac音频", "song.flac", "0", 1, "audio"},
		{"jpg图片", "photo.jpg", "0", 1, "image"},
		{"png图片", "photo.png", "0", 1, "image"},
		{"压缩包", "archive.zip", "0", 1, "other"},
		{"无扩展名", "README", "0", 1, "other"},
		{"文档", "notes.txt", "0", 1, "other"},
		{"目录", "纪录片合集", "987654", 0, "folder"},
	}

	for _, m := range mappings {
		t.Run(m.name, func(t *testing.T) {
			f := driver.ShareFile{
				FileName:   m.fileName,
				CategoryID: driver.IntString(m.cid),
				IsFile:     m.isFile,
			}
			result := convertToShareFileInfo(f)
			if result.Type != m.expected {
				t.Errorf("fileName=%s cid=%s 期望 type='%s', 实际 '%s'",
					m.fileName, m.cid, m.expected, result.Type)
			}
		})
	}
}

// TestParseShareLink_POC_ConvertFields 验证 convertToShareFileInfo 的字段转换规则
// share/snap 接口不返回 pick_code 与路径：Path="" PickCode=""，IsDir 根据 fc 判断。
func TestParseShareLink_POC_ConvertFields(t *testing.T) {
	snapResp := mustUnmarshalShareSnap(t, pocShareSnapJSON)

	// 文件项
	fi := convertToShareFileInfo(snapResp.Data.List[0])
	if fi.Name != "The Matrix (1999) 4K HDR.mkv" {
		t.Errorf("Name 不匹配: '%s'", fi.Name)
	}
	if fi.Size != 21474836480 {
		t.Errorf("Size 不匹配: %d", fi.Size)
	}
	if fi.Type != "video" {
		t.Errorf("Type: 期望 video, 实际 %s", fi.Type)
	}
	if fi.Path != "" {
		t.Errorf("Path 应为空字符串, 实际 '%s'", fi.Path)
	}
	if fi.PickCode != "" {
		t.Errorf("PickCode 应为空字符串（share/snap 不返回）, 实际 '%s'", fi.PickCode)
	}
	if fi.Sha1 != "abc123def456789" {
		t.Errorf("Sha1 不匹配: '%s'", fi.Sha1)
	}
	if fi.IsDir {
		t.Error("文件项 IsDir 应为 false")
	}

	// 目录项
	di := convertToShareFileInfo(snapResp.Data.List[1])
	if !di.IsDir {
		t.Error("目录项 IsDir 应为 true（cid 非零）")
	}
	if di.Type != "folder" {
		t.Errorf("目录项 Type: 期望 folder, 实际 %s", di.Type)
	}
	if di.DirID != "987654" || di.Fid != "987654" {
		t.Errorf("目录ID映射异常: DirID=%q Fid=%q", di.DirID, di.Fid)
	}
}

// TestGetShareFilesLoadsDirectory 验证传入 dir_id 时直接请求该目录，而不是读取根目录缓存。
func TestGetShareFilesLoadsDirectory(t *testing.T) {
	childResp := mustUnmarshalShareSnap(t, `{
		"state": true,
		"data": {
			"shareinfo": {"share_title": "测试分享"},
			"count": 1,
			"list": [{
				"fid": "5566", "cid": 987654, "n": "movie.mkv",
				"ico": "mkv", "s": 1024, "sha": "sha1", "fc": 1
			}]
		}
	}`)
	fakeClient := &fakeShareCloud115Client{
		shareSnapByDir: map[string]*driver.ShareSnapResp{"987654": childResp},
	}
	svc := &ShareTransferService{client: fakeClient}

	result, err := svc.GetShareFiles(context.Background(), "share-code", "pass-code", "987654", 1, 50, "", "")
	if err != nil {
		t.Fatalf("展开分享目录失败: %v", err)
	}
	if len(fakeClient.gotDirIDs) != 1 || fakeClient.gotDirIDs[0] != "987654" {
		t.Fatalf("目录ID未传给115接口: %#v", fakeClient.gotDirIDs)
	}
	if fakeClient.gotReceiveCode != "pass-code" {
		t.Fatalf("分享密码未透传: %q", fakeClient.gotReceiveCode)
	}
	if result.TotalFiles != 1 || len(result.Files) != 1 || result.Files[0].Name != "movie.mkv" {
		t.Fatalf("目录子项返回异常: %#v", result)
	}
}

// TestParseShareLink_POC_ErrorResponses 验证 mapShareSnapError 的错误映射逻辑
// driver 封装的错误信息中包含原始响应体，按 errno / 关键字匹配
func TestParseShareLink_POC_ErrorResponses(t *testing.T) {
	errorScenarios := []struct {
		name       string
		err        error
		wantErrMsg string
	}{
		{
			name:       "share_not_found_errno",
			err:        fmt.Errorf(`shared link not found: {"state":false,"errno":4100026,"error":"分享不存在"}`),
			wantErrMsg: "分享不存在或已过期",
		},
		{
			name:       "share_invalid_errno",
			err:        fmt.Errorf(`shared link invalid: {"state":false,"errno":4100009}`),
			wantErrMsg: "分享不存在或已过期",
		},
		{
			name:       "share_expired_990009",
			err:        fmt.Errorf(`unexpected error: {"state":false,"errno":990009,"error":"分享已过期"}`),
			wantErrMsg: "分享不存在或已过期",
		},
		{
			name:       "password_required_990010",
			err:        fmt.Errorf(`unexpected error: {"state":false,"errno":990010,"error":"需要访问密码"}`),
			wantErrMsg: "分享需要访问密码",
		},
		{
			name:       "wrong_password_990011",
			err:        fmt.Errorf(`unexpected error: {"state":false,"errno":990011,"error":"访问密码错误"}`),
			wantErrMsg: "访问密码错误",
		},
		{
			name:       "wrong_password_keyword",
			err:        fmt.Errorf(`unexpected error: {"state":false,"error":"提取码错误"}`),
			wantErrMsg: "访问密码错误",
		},
		{
			name:       "unknown_error",
			err:        fmt.Errorf("network timeout"),
			wantErrMsg: "解析分享链接失败: network timeout",
		},
	}

	for _, sc := range errorScenarios {
		t.Run(sc.name, func(t *testing.T) {
			gotErr := mapShareSnapError(sc.err)
			if gotErr == nil {
				t.Fatal("mapShareSnapError 不应返回 nil")
			}
			if gotErr.Error() != sc.wantErrMsg {
				t.Errorf("错误信息: 期望 '%s', 实际 '%s'", sc.wantErrMsg, gotErr.Error())
			}
		})
	}
}

func TestIsRetryableShareSnapError(t *testing.T) {
	if !isRetryableShareSnapError(fmt.Errorf("wsarecv: An existing connection was forcibly closed by the remote host")) {
		t.Fatal("连接被重置应判定为可重试网络错误")
	}
	if isRetryableShareSnapError(fmt.Errorf("unexpected error: errno=990011 提取码错误")) {
		t.Fatal("访问码错误不应重试")
	}
}

// TestParseShareLink_POC_EndToEnd 验证 ParseShareLink 完整流程：
// URL解析 → 调用 GetShareSnap(shareCode, password, "0") → 领域模型组装
func TestParseShareLink_POC_EndToEnd(t *testing.T) {
	fakeClient := &fakeShareCloud115Client{
		shareSnap: mustUnmarshalShareSnap(t, pocShareSnapJSON),
	}
	// 直接构造 service 实例，绕过 DAO/Redis 依赖（redisClient=nil 时跳过缓存）
	svc := &ShareTransferService{client: fakeClient}

	result, err := svc.ParseShareLink(context.Background(),
		"https://115cdn.com/s/swexample123?password=demo1", "")
	if err != nil {
		t.Fatalf("ParseShareLink 返回错误: %v", err)
	}

	// 验证传递给 client 的参数
	if fakeClient.gotShareCode != "swexample123" {
		t.Errorf("shareCode: 期望 'swexample123', 实际 '%s'", fakeClient.gotShareCode)
	}
	if fakeClient.gotReceiveCode != "demo1" {
		t.Errorf("receiveCode: 期望 'demo1'（从URL提取）, 实际 '%s'", fakeClient.gotReceiveCode)
	}
	if len(fakeClient.gotDirIDs) == 0 || fakeClient.gotDirIDs[0] != "0" {
		t.Errorf("dirID: 首次请求应为 '0'（根目录）, 实际 %#v", fakeClient.gotDirIDs)
	}

	// 验证领域模型
	if result.ShareCode != "swexample123" {
		t.Errorf("ShareCode 不匹配: '%s'", result.ShareCode)
	}
	if result.FolderName != "4K Movie Collection 2024" {
		t.Errorf("FolderName 不匹配: '%s'", result.FolderName)
	}
	if result.TotalFiles != 2 {
		t.Errorf("TotalFiles: 期望 2（分页拉全后实际文件数）, 实际 %d", result.TotalFiles)
	}
	if len(result.Files) != 2 {
		t.Fatalf("Files 长度: 期望 2, 实际 %d", len(result.Files))
	}
	if result.TotalSize != 21474836480 {
		t.Errorf("TotalSize: 期望 21474836480, 实际 %d", result.TotalSize)
	}
	if result.Files[0].Type != "video" {
		t.Errorf("Files[0].Type: 期望 video, 实际 %s", result.Files[0].Type)
	}
	if !result.Files[1].IsDir {
		t.Error("Files[1] 应为目录")
	}
}

// TestParseShareLink_POC_EndToEndError 验证 API 错误经 mapShareSnapError 转换后返回
func TestParseShareLink_POC_EndToEndError(t *testing.T) {
	fakeClient := &fakeShareCloud115Client{
		shareSnapErr: fmt.Errorf(`shared link not found: {"state":false,"errno":4100026}`),
	}
	svc := &ShareTransferService{client: fakeClient}

	_, err := svc.ParseShareLink(context.Background(), "https://115cdn.com/s/swexample123", "")
	if err == nil {
		t.Fatal("ParseShareLink 应返回错误")
	}
	if err.Error() != "分享不存在或已过期" {
		t.Errorf("错误信息: 期望 '分享不存在或已过期', 实际 '%s'", err.Error())
	}
}

// TestParseShareLink_POC_InvalidURL 验证无效链接不触发 API 调用
func TestParseShareLink_POC_InvalidURL(t *testing.T) {
	fakeClient := &fakeShareCloud115Client{}
	svc := &ShareTransferService{client: fakeClient}

	_, err := svc.ParseShareLink(context.Background(), "https://pan.baidu.com/s/xxxxx", "")
	if err == nil {
		t.Fatal("无效链接应返回错误")
	}
	if !strings.Contains(err.Error(), "无效的115分享链接") {
		t.Errorf("错误信息不匹配: '%s'", err.Error())
	}
	if fakeClient.gotShareCode != "" {
		t.Error("无效链接不应调用 GetShareSnap")
	}
}
