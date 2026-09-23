package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"

	"github.com/google/uuid"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
)

const (
	embyTaskOriginManual  = "manual"
	embyPreviewTTL        = 24 * time.Hour
	strmScanCaptureAction = "strm_scan_capture"
)

// ErrImageTooLarge 表示上传图片超过 10 MiB 限制。
var ErrImageTooLarge = fmt.Errorf("图片不能超过 10 MiB")

// SystemConfigWriter 提供 AI 封面配置的读写能力。
type SystemConfigWriter interface {
	SystemConfigReader
	Upsert(key, value string) error
}

// EmbyManagementService 编排多实例 Emby 管理及任务中心同步。
type EmbyManagementService struct {
	servers      *dao.EmbyServerDAO
	tasks        *TaskService
	configs      SystemConfigWriter
	httpClient   *http.Client
	previewDir   string
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// NewEmbyManagementService 创建 Emby 管理服务。
func NewEmbyManagementService(servers *dao.EmbyServerDAO, tasks *TaskService, configs SystemConfigWriter, httpClient *http.Client) *EmbyManagementService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &EmbyManagementService{
		servers: servers, tasks: tasks, configs: configs, httpClient: httpClient,
		previewDir:   filepath.Join(os.TempDir(), "easy-strm-emby-covers"),
		pollInterval: 5 * time.Second, pollTimeout: 30 * time.Minute,
	}
}

// ListServers 返回脱敏后的实例列表。
func (s *EmbyManagementService) ListServers() ([]*domain.EmbyServer, error) { return s.servers.List() }

// GetServer 返回指定实例。
func (s *EmbyManagementService) GetServer(id int) (*domain.EmbyServer, error) {
	return s.requireServer(id)
}

// PlaybackLinks 先定位剧集，再查询指定季集；上游失败不能伪装成未入库。
func (s *EmbyManagementService) PlaybackLinks(title string, tmdbID, season, episode int) ([]map[string]string, error) {
	if season < 0 || episode <= 0 || tmdbID < 0 || (tmdbID == 0 && strings.TrimSpace(title) == "") {
		return nil, fmt.Errorf("媒体身份或季集参数无效")
	}
	servers, err := s.servers.List()
	if err != nil {
		return nil, err
	}
	links := make([]map[string]string, 0)
	failures := make([]string, 0)
	enabled := 0
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		enabled++
		found, err := s.findEpisodeLinks(server, title, tmdbID, season, episode)
		if err != nil {
			failures = append(failures, server.Name)
			continue
		}
		links = append(links, found...)
	}
	if enabled == 0 {
		return nil, fmt.Errorf("尚未配置已启用的 Emby 实例")
	}
	if len(links) == 0 && len(failures) == 0 {
		for _, server := range servers {
			if server.Enabled {
				links = append(links, map[string]string{"fallback": "true", "server_name": server.Name, "name": title, "url": s.embySearchURL(server, title)})
			}
		}
	}
	if len(links) == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("Emby 查询失败（%s），请检查实例连接及 API Key 后重试", strings.Join(failures, "、"))
	}
	return links, nil
}

func (s *EmbyManagementService) embySearchURL(server *domain.EmbyServer, title string) string {
	var info struct {
		ID string `json:"Id"`
	}
	_ = s.requestJSON(server, http.MethodGet, "/emby/System/Info", nil, nil, &info)
	base := strings.TrimRight(server.BaseURL, "/") + "/web/index.html#!/search?query=" + url.QueryEscape(title)
	if info.ID != "" {
		base += "&serverId=" + url.QueryEscape(info.ID)
	}
	return base
}

// MoviePlaybackLinks 按 TMDB ID 或片名查询 Emby 中的电影项目。
func (s *EmbyManagementService) MoviePlaybackLinks(title string, tmdbID int) ([]map[string]string, error) {
	servers, err := s.servers.List()
	if err != nil {
		return nil, err
	}
	links := make([]map[string]string, 0)
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		query := url.Values{"Recursive": {"true"}, "IncludeItemTypes": {"Movie"}, "Fields": {"ProviderIds"}}
		if tmdbID > 0 {
			query.Set("AnyProviderIdEquals", "tmdb."+strconv.Itoa(tmdbID))
		} else {
			query.Set("SearchTerm", strings.TrimSpace(title))
		}
		var result struct {
			Items []struct {
				ID          string            `json:"Id"`
				Name        string            `json:"Name"`
				ProviderIDs map[string]string `json:"ProviderIds"`
			} `json:"Items"`
		}
		if err := s.requestJSON(server, http.MethodGet, "/emby/Items", query, nil, &result); err != nil {
			continue
		}
		if len(result.Items) == 0 && tmdbID > 0 && strings.TrimSpace(title) != "" {
			query.Del("AnyProviderIdEquals")
			query.Set("SearchTerm", strings.TrimSpace(title))
			if fallbackErr := s.requestJSON(server, http.MethodGet, "/emby/Items", query, nil, &result); fallbackErr != nil {
				continue
			}
		}
		for _, item := range result.Items {
			if tmdbID > 0 && item.ProviderIDs["Tmdb"] != strconv.Itoa(tmdbID) && normalizeEmbyTitle(item.Name) != normalizeEmbyTitle(title) {
				continue
			}
			if tmdbID == 0 && !strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(title)) {
				continue
			}
			links = append(links, map[string]string{"server_name": server.Name, "item_id": item.ID, "name": item.Name, "url": s.embyItemURL(server, item.ID)})
		}
	}
	return links, nil
}

func (s *EmbyManagementService) findEpisodeLinks(server *domain.EmbyServer, title string, tmdbID, season, episode int) ([]map[string]string, error) {
	query := url.Values{"Recursive": {"true"}, "IncludeItemTypes": {"Series"}, "Fields": {"ProviderIds"}}
	if tmdbID > 0 {
		query.Set("AnyProviderIdEquals", "tmdb."+strconv.Itoa(tmdbID))
	} else {
		query.Set("SearchTerm", strings.TrimSpace(title))
	}
	var series struct {
		Items []struct {
			ID          string            `json:"Id"`
			Name        string            `json:"Name"`
			ProviderIDs map[string]string `json:"ProviderIds"`
		} `json:"Items"`
	}
	if err := s.requestJSON(server, http.MethodGet, "/emby/Items", query, nil, &series); err != nil {
		return nil, err
	}
	if len(series.Items) == 0 && tmdbID > 0 && strings.TrimSpace(title) != "" {
		query.Del("AnyProviderIdEquals")
		query.Set("SearchTerm", strings.TrimSpace(title))
		if err := s.requestJSON(server, http.MethodGet, "/emby/Items", query, nil, &series); err != nil {
			return nil, err
		}
	}
	if len(series.Items) == 0 && tmdbID > 0 {
		// 某些 Emby 版本不支持 AnyProviderIdEquals；回读剧集索引后按 ProviderIds 精确匹配。
		query.Del("SearchTerm")
		query.Set("Limit", "10000")
		if err := s.requestJSON(server, http.MethodGet, "/emby/Items", query, nil, &series); err != nil {
			return nil, err
		}
	}
	links := make([]map[string]string, 0)
	for _, show := range series.Items {
		// 回读身份，避免不支持过滤参数的服务器返回其他作品；有 TMDB 身份时不按同名猜测。
		if show.ID == "" {
			continue
		}
		if tmdbID > 0 {
			matched := false
			for key, value := range show.ProviderIDs {
				if strings.EqualFold(key, "tmdb") && value == strconv.Itoa(tmdbID) {
					matched = true
				}
			}

			if !matched {
				// 部分 Emby 只返回本地元数据或错误的 TMDB ProviderId，唯一同名结果仍可安全使用。
				matched = normalizeEmbyTitle(show.Name) == normalizeEmbyTitle(title)
				if !matched {
					continue
				}
			}
		} else if !strings.EqualFold(strings.TrimSpace(show.Name), strings.TrimSpace(title)) {
			continue
		}
		var episodes struct {
			Items []struct {
				ID                string `json:"Id"`
				Name              string `json:"Name"`
				IndexNumber       int    `json:"IndexNumber"`
				ParentIndexNumber *int   `json:"ParentIndexNumber"`
			} `json:"Items"`
		}
		episodeQuery := url.Values{"Season": {strconv.Itoa(season)}}
		if err := s.requestJSON(server, http.MethodGet, "/emby/Shows/"+url.PathEscape(show.ID)+"/Episodes", episodeQuery, nil, &episodes); err != nil {
			return nil, err
		}
		for _, item := range episodes.Items {
			if item.ID == "" || item.IndexNumber != episode || (item.ParentIndexNumber != nil && *item.ParentIndexNumber != season) {
				continue
			}
			links = append(links, map[string]string{"server_name": server.Name, "item_id": item.ID, "name": item.Name, "url": s.embyItemURL(server, item.ID)})
		}
	}
	return links, nil
}

func normalizeEmbyTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "", "　", "", "-", "", "–", "", "—", "", ":", "", "：", "", "·", "").Replace(value)
	return value
}

func (s *EmbyManagementService) embyItemURL(server *domain.EmbyServer, itemID string) string {
	var info struct {
		ID string `json:"Id"`
	}
	_ = s.requestJSON(server, http.MethodGet, "/emby/System/Info", nil, nil, &info)
	base := strings.TrimRight(server.BaseURL, "/") + "/web/index.html#!/item?id=" + url.QueryEscape(itemID)
	if info.ID != "" {
		base += "&serverId=" + url.QueryEscape(info.ID)
	}
	return base
}

// CreateServer 新增实例。
func (s *EmbyManagementService) CreateServer(name, baseURL, apiKey string, enabled, isDefault bool) (*domain.EmbyServer, error) {
	if err := validateEmbyServerInput(name, baseURL, apiKey, false); err != nil {
		return nil, err
	}
	return s.servers.Create(strings.TrimSpace(name), strings.TrimSpace(baseURL), strings.TrimSpace(apiKey), enabled, isDefault)
}

// CreateServerTask 新增实例并生成任务中心记录。
func (s *EmbyManagementService) CreateServerTask(name, baseURL, apiKey string, enabled, isDefault bool) (*domain.EmbyServer, string, error) {
	metadata := map[string]interface{}{"operation": "create_server", "target": strings.TrimSpace(name), "origin": embyTaskOriginManual}
	var server *domain.EmbyServer
	taskID, err := s.runShortTask(domain.TaskTypeEmbyServer, "新增 Emby 实例", metadata, func() error {
		var createErr error
		server, createErr = s.CreateServer(name, baseURL, apiKey, enabled, isDefault)
		if server != nil {
			metadata["server_id"] = server.ID
			metadata["server_name"] = server.Name
		}
		return createErr
	})
	return server, taskID, err
}

// UpdateServer 更新实例。
func (s *EmbyManagementService) UpdateServer(id int, name, baseURL, apiKey string, enabled, isDefault bool) (*domain.EmbyServer, error) {
	if err := validateEmbyServerInput(name, baseURL, apiKey, true); err != nil {
		return nil, err
	}
	return s.servers.Update(id, strings.TrimSpace(name), strings.TrimSpace(baseURL), strings.TrimSpace(apiKey), enabled, isDefault)
}

// UpdateServerTask 更新实例并生成任务中心记录。
func (s *EmbyManagementService) UpdateServerTask(id int, name, baseURL, apiKey string, enabled, isDefault bool) (*domain.EmbyServer, string, error) {
	metadata := map[string]interface{}{"server_id": id, "server_name": name, "operation": "update_server", "target": name, "origin": embyTaskOriginManual}
	var server *domain.EmbyServer
	taskID, err := s.runShortTask(domain.TaskTypeEmbyServer, "修改 Emby 实例", metadata, func() error {
		var updateErr error
		server, updateErr = s.UpdateServer(id, name, baseURL, apiKey, enabled, isDefault)
		return updateErr
	})
	return server, taskID, err
}

// DeleteServer 删除实例连接配置。
func (s *EmbyManagementService) DeleteServer(id int) error { return s.servers.Delete(id) }

// DeleteServerTask 删除实例配置并生成任务中心记录。
func (s *EmbyManagementService) DeleteServerTask(id int) (string, error) {
	server, err := s.servers.GetByID(id)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("实例 %d", id)
	if server != nil {
		name = server.Name
	}
	metadata := map[string]interface{}{"server_id": id, "server_name": name, "operation": "delete_server", "target": name, "origin": embyTaskOriginManual}
	return s.runShortTask(domain.TaskTypeEmbyServer, "删除 Emby 实例", metadata, func() error { return s.DeleteServer(id) })
}

func validateEmbyServerInput(name, baseURL, apiKey string, allowEmptyKey bool) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("实例名称不能为空")
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("Emby 服务地址必须是有效的 HTTP 或 HTTPS 地址")
	}
	if !allowEmptyKey && strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("Emby API Key 不能为空")
	}
	if strings.Contains(apiKey, "********") {
		return fmt.Errorf("不能提交脱敏后的 API Key")
	}
	return nil
}

// CheckServerConnection 检查指定实例连接。
func (s *EmbyManagementService) CheckServerConnection(id int) (*EmbySystemInfo, error) {
	server, err := s.requireServer(id)
	if err != nil {
		return nil, err
	}
	var info EmbySystemInfo
	if err = s.requestJSON(server, http.MethodGet, "/emby/System/Info", nil, nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ListUsers 获取 Emby 用户列表。
func (s *EmbyManagementService) ListUsers(serverID int) ([]domain.EmbyUser, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	users := make([]domain.EmbyUser, 0)
	if err = s.requestJSON(server, http.MethodGet, "/emby/Users", nil, nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// CreateUser 新增用户并记录任务。
func (s *EmbyManagementService) CreateUser(serverID int, name, password string) (string, domain.EmbyUser, error) {
	var created domain.EmbyUser
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", created, err
	}
	if strings.TrimSpace(name) == "" {
		return "", created, fmt.Errorf("用户名不能为空")
	}
	taskID, err := s.runShortTask(domain.TaskTypeEmbyUser, "新增 Emby 用户", taskMetadata(server, "create_user", name), func() error {
		if requestErr := s.requestJSON(server, http.MethodPost, "/emby/Users/New", nil, map[string]string{"Name": strings.TrimSpace(name)}, &created); requestErr != nil {
			return requestErr
		}
		if password != "" {
			return s.setUserPassword(server, created.ID, password, false)
		}
		return nil
	})
	return taskID, created, err
}

// UpdateUser 更新用户名和常用权限。
func (s *EmbyManagementService) UpdateUser(serverID int, userID, name string, policy *domain.EmbyUserPolicy) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	return s.runShortTask(domain.TaskTypeEmbyUser, "修改 Emby 用户", taskMetadata(server, "update_user", userID), func() error {
		var normalized domain.EmbyUserPolicy
		if policy != nil {
			var policyErr error
			normalized, policyErr = s.normalizeUserPolicy(server, *policy)
			if policyErr != nil {
				return policyErr
			}
		}
		var user map[string]interface{}
		if requestErr := s.requestJSON(server, http.MethodGet, "/emby/Users/"+url.PathEscape(userID), nil, nil, &user); requestErr != nil {
			return requestErr
		}
		if strings.TrimSpace(name) != "" {
			user["Name"] = strings.TrimSpace(name)
			if requestErr := s.requestJSON(server, http.MethodPost, "/emby/Users/"+url.PathEscape(userID), nil, user, nil); requestErr != nil {
				return requestErr
			}
		}
		if policy != nil {
			return s.saveUserPolicy(server, userID, user, normalized)
		}
		return nil
	})
}

// SetUserPassword 设置或重置用户密码。
func (s *EmbyManagementService) SetUserPassword(serverID int, userID, password string, reset bool) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	return s.runShortTask(domain.TaskTypeEmbyUser, "修改 Emby 用户密码", taskMetadata(server, "set_password", userID), func() error { return s.setUserPassword(server, userID, password, reset) })
}

func (s *EmbyManagementService) setUserPassword(server *domain.EmbyServer, userID, password string, reset bool) error {
	return s.requestJSON(server, http.MethodPost, "/emby/Users/"+url.PathEscape(userID)+"/Password", nil, map[string]interface{}{"Id": userID, "NewPw": password, "ResetPassword": reset}, nil)
}

// DeleteUser 删除 Emby 用户。
func (s *EmbyManagementService) DeleteUser(serverID int, userID string) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	return s.runShortTask(domain.TaskTypeEmbyUser, "删除 Emby 用户", taskMetadata(server, "delete_user", userID), func() error {
		return s.requestJSON(server, http.MethodDelete, "/emby/Users/"+url.PathEscape(userID), nil, nil, nil)
	})
}

// UploadUserAvatar 更新 Emby 用户头像并记录任务。
func (s *EmbyManagementService) UploadUserAvatar(serverID int, userID, contentType string, data []byte) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	if !isSupportedImage(contentType) {
		return "", fmt.Errorf("仅支持 JPG、PNG 或 WebP 图片")
	}
	return s.runShortTask(domain.TaskTypeEmbyUser, "修改 Emby 用户头像", taskMetadata(server, "upload_user_avatar", userID), func() error {
		resp, requestErr := s.doServerRequest(server, http.MethodPost, "/emby/Users/"+url.PathEscape(userID)+"/Images/Primary", nil, bytes.NewReader(data), contentType)
		if requestErr != nil {
			return requestErr
		}
		resp.Body.Close()
		return nil
	})
}

// GetUserAvatar 读取 Emby 用户头像，避免前端直接访问 Emby 或接触 API Key。
func (s *EmbyManagementService) GetUserAvatar(serverID int, userID string) ([]byte, string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, "", err
	}
	resp, err := s.doServerRequest(server, http.MethodGet, "/emby/Users/"+url.PathEscape(userID)+"/Images/Primary", url.Values{"maxWidth": {"160"}}, nil, "")
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

// ListLibraries 获取指定实例媒体库。
func (s *EmbyManagementService) ListLibraries(serverID int) ([]EmbyVirtualFolderInfo, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	return s.listLibraries(server)
}

func (s *EmbyManagementService) listLibraries(server *domain.EmbyServer) ([]EmbyVirtualFolderInfo, error) {
	libraries := make([]EmbyVirtualFolderInfo, 0)
	if err := s.requestJSON(server, http.MethodGet, "/emby/Library/VirtualFolders", nil, nil, &libraries); err != nil {
		return nil, err
	}
	return libraries, nil
}

// ListLibrarySummaries 获取媒体库卡片所需摘要，并发补充各库媒体文件数量。
func (s *EmbyManagementService) ListLibrarySummaries(serverID int) ([]EmbyVirtualFolderInfo, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	libraries, err := s.listLibraries(server)
	if err != nil {
		return nil, err
	}

	var waitGroup sync.WaitGroup
	requestSlots := make(chan struct{}, 4)
	for index := range libraries {
		libraries[index].MediaFileCount = -1
		if strings.TrimSpace(libraries[index].ItemID) == "" {
			continue
		}
		waitGroup.Add(1)
		go func(libraryIndex int) {
			defer waitGroup.Done()
			requestSlots <- struct{}{}
			defer func() { <-requestSlots }()
			count, countErr := s.getLibraryMediaFileCount(server, libraries[libraryIndex].ItemID)
			if countErr == nil {
				libraries[libraryIndex].MediaFileCount = count
			}
		}(index)
	}
	waitGroup.Wait()
	return libraries, nil
}

func (s *EmbyManagementService) getLibraryMediaFileCount(server *domain.EmbyServer, libraryID string) (int, error) {
	query := url.Values{
		"ParentId":  {libraryID},
		"Recursive": {"true"},
		"IsFolder":  {"false"},
		"Limit":     {"1"},
	}
	var result struct {
		TotalRecordCount int `json:"TotalRecordCount"`
	}
	if err := s.requestJSON(server, http.MethodGet, "/emby/Items", query, nil, &result); err != nil {
		return 0, err
	}
	return result.TotalRecordCount, nil
}

// GetLibraryCover 读取媒体库主封面，避免前端直接访问 Emby 或接触 API Key。
func (s *EmbyManagementService) GetLibraryCover(serverID int, libraryID string) ([]byte, string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(libraryID) == "" {
		return nil, "", fmt.Errorf("媒体库 ID 不能为空")
	}
	resp, err := s.doServerRequest(server, http.MethodGet, "/emby/Items/"+url.PathEscape(libraryID)+"/Images/Primary", url.Values{"maxWidth": {"480"}}, nil, "")
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

// CreateLibrary 新增媒体库。
func (s *EmbyManagementService) CreateLibrary(serverID int, input domain.EmbyLibraryInput) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	if err = validateLibraryInput(input); err != nil {
		return "", err
	}
	query := url.Values{"name": {input.Name}, "collectionType": {input.CollectionType}, "refreshLibrary": {"false"}}
	return s.runShortTask(domain.TaskTypeEmbyLibrary, "新增 Emby 媒体库", taskMetadata(server, "create_library", input.Name), func() error {
		return s.requestJSON(server, http.MethodPost, "/emby/Library/VirtualFolders", query, input.Paths, nil)
	})
}

// UpdateLibrary 修改媒体库常用配置。
func (s *EmbyManagementService) UpdateLibrary(serverID int, libraryID string, input domain.EmbyLibraryInput) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	if err = validateLibraryInput(input); err != nil {
		return "", err
	}
	libraries, err := s.listLibrariesByServer(server)
	if err != nil {
		return "", err
	}
	var library *EmbyVirtualFolderInfo
	for i := range libraries {
		if libraryID != "" && (libraries[i].ItemID == libraryID || libraries[i].ID == libraryID) {
			library = &libraries[i]
			break
		}
	}
	if library == nil {
		return "", fmt.Errorf("媒体库不存在，请刷新列表后重试")
	}
	if library.ItemID == "" {
		library.ItemID = library.ID
	}
	if input.CollectionType != library.CollectionType && !(input.CollectionType == "mixed" && library.CollectionType == "") {
		return "", fmt.Errorf("已有媒体库不支持修改内容类型，请新建媒体库")
	}
	if library.LibraryOptions == nil {
		return "", fmt.Errorf("无法读取媒体库原有配置")
	}
	options := library.LibraryOptions
	options["PreferredMetadataLanguage"] = input.MetadataLanguage
	options["MetadataCountryCode"] = input.MetadataCountry
	options["EnableRealtimeMonitor"] = input.EnableRealtimeMonitor
	body := map[string]interface{}{"Id": libraryID, "LibraryOptions": options}
	return s.runShortTask(domain.TaskTypeEmbyLibrary, "修改 Emby 媒体库", taskMetadata(server, "update_library", input.Name), func() error {
		if err := s.requestJSON(server, http.MethodPost, "/emby/Library/VirtualFolders/LibraryOptions", nil, body, nil); err != nil {
			return err
		}
		// Emby 的名称和目录有独立接口。Paths 接口通过 pathInfo 查询参数接收目录，不能把 PathInfo 当 JSON 请求体传递，否则部分版本会将其误解析为 Guid。
		for _, path := range input.Paths {
			if containsString(library.Locations, path.Path) {
				continue
			}
			query := url.Values{"name": {library.Name}, "pathInfo": {path.Path}, "refreshLibrary": {"false"}}
			if err := s.requestJSON(server, http.MethodPost, "/emby/Library/VirtualFolders/Paths", query, nil, nil); err != nil {
				return fmt.Errorf("配置已保存，添加目录失败，请刷新后重试: %w", err)
			}
		}
		for _, old := range library.Locations {
			keep := false
			for _, path := range input.Paths {
				if path.Path == old {
					keep = true
					break
				}
			}
			if keep {
				continue
			}
			if err := s.requestJSON(server, http.MethodDelete, "/emby/Library/VirtualFolders/Paths", url.Values{"name": {library.Name}, "path": {old}, "refreshLibrary": {"false"}}, nil, nil); err != nil {
				return fmt.Errorf("配置已部分保存，移除目录失败，请刷新后重试: %w", err)
			}
		}
		if library.Name != input.Name {
			if err := s.requestJSON(server, http.MethodPost, "/emby/Library/VirtualFolders/Name", url.Values{"name": {library.Name}, "newName": {input.Name}, "refreshLibrary": {"false"}}, nil, nil); err != nil {
				return fmt.Errorf("配置已保存，重命名失败: %w", err)
			}
		}
		return nil
	})
}

// DeleteLibrary 从 Emby 删除媒体库配置，不接触原始文件。
func (s *EmbyManagementService) DeleteLibrary(serverID int, name string) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("媒体库名称不能为空")
	}
	query := url.Values{"name": {name}, "refreshLibrary": {"false"}}
	return s.runShortTask(domain.TaskTypeEmbyLibrary, "删除 Emby 媒体库", taskMetadata(server, "delete_library", name), func() error {
		return s.requestJSON(server, http.MethodDelete, "/emby/Library/VirtualFolders", query, nil, nil)
	})
}

func validateLibraryInput(input domain.EmbyLibraryInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("媒体库名称不能为空")
	}
	if len(input.Paths) == 0 {
		return fmt.Errorf("至少需要一个媒体目录")
	}
	for _, item := range input.Paths {
		if strings.TrimSpace(item.Path) == "" {
			return fmt.Errorf("媒体目录不能为空")
		}
	}
	return nil
}

// StartRefresh 启动媒体库刷新并在后台跟踪可信进度。
func (s *EmbyManagementService) StartRefresh(serverID int, libraryID string) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	target := libraryID
	if target == "" {
		target = "全部媒体库"
	}
	metadata := taskMetadata(server, "refresh", target)
	metadata["current_step"] = "等待执行"
	metadata["steps"] = buildEmbySteps("校验实例连接", "提交刷新请求", "跟踪 Emby 刷新进度", "确认刷新终态", "写入执行结论")
	taskID := "emby-" + uuid.NewString()
	if err = s.tasks.Create(taskID, string(domain.TaskTypeEmbyRefresh), "Emby 媒体库刷新"); err != nil {
		return "", err
	}
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	ctx, cancel := context.WithCancel(context.Background())
	s.tasks.RegisterCancel(taskID, cancel)
	go s.runRefreshTask(ctx, taskID, server, libraryID, metadata)
	return taskID, nil
}

// StartRefreshBySourceID 根据媒体源的实例绑定提交自动刷新任务。
func (s *EmbyManagementService) StartRefreshBySourceID(sourceID int) (string, string, error) {
	server, libraryID, err := s.servers.GetMediaSourceBinding(sourceID)
	if err != nil || server == nil || libraryID == "" {
		return "", libraryID, err
	}
	taskID, err := s.StartRefresh(server.ID, libraryID)
	if err == nil {
		if task, getErr := s.tasks.Get(taskID); getErr == nil && task != nil {
			metadata, _ := task["metadata"].(map[string]interface{})
			if metadata == nil {
				metadata = map[string]interface{}{}
			}
			metadata["origin"] = "organize_linkage"
			metadata["source_id"] = sourceID
			_ = s.tasks.UpdateMetadata(taskID, metadata)
		}
	}
	return taskID, libraryID, err
}

// BindMediaSource 建立媒体源、Emby 实例和媒体库关联。
func (s *EmbyManagementService) BindMediaSource(serverID, sourceID int, libraryID string) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	metadata := taskMetadata(server, "bind_media_source", fmt.Sprintf("媒体源 %d → 媒体库 %s", sourceID, libraryID))
	return s.runShortTask(domain.TaskTypeEmbyLibrary, "绑定 Emby 媒体源", metadata, func() error { return s.servers.BindMediaSource(sourceID, serverID, strings.TrimSpace(libraryID)) })
}

func (s *EmbyManagementService) runRefreshTask(ctx context.Context, taskID string, server *domain.EmbyServer, libraryID string, metadata map[string]interface{}) {
	defer s.tasks.RemoveCancel(taskID)
	_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusRunning)
	updateEmbyTaskStep(s.tasks, taskID, metadata, 0, "running", "正在校验 Emby 连接", 2)
	if _, err := s.CheckServerConnection(server.ID); err != nil {
		s.failEmbyTask(taskID, metadata, 0, err)
		return
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 1, "running", "正在提交刷新请求", 5)
	targetIDs := map[string]struct{}{}
	failedItems := make([]map[string]interface{}, 0)
	total := 1
	if libraryID != "" {
		targetIDs[libraryID] = struct{}{}
		if err := s.requestJSON(server, http.MethodPost, "/emby/Items/"+url.PathEscape(libraryID)+"/Refresh", nil, nil, nil); err != nil {
			s.failEmbyTask(taskID, metadata, 1, err)
			return
		}
	} else {
		libraries, err := s.ListLibraries(server.ID)
		if err != nil {
			s.failEmbyTask(taskID, metadata, 1, fmt.Errorf("读取待刷新媒体库失败: %w", err))
			return
		}
		total = len(libraries)
		if total == 0 {
			s.failEmbyTask(taskID, metadata, 1, fmt.Errorf("当前实例没有可刷新的媒体库"))
			return
		}
		for _, library := range libraries {
			if err = s.requestJSON(server, http.MethodPost, "/emby/Items/"+url.PathEscape(library.ItemID)+"/Refresh", nil, nil, nil); err != nil {
				failedItems = append(failedItems, map[string]interface{}{
					"file_id": library.ItemID, "file_name": library.Name,
					"category": "emby_library_refresh_failed", "reason": err.Error(),
				})
				continue
			}
			targetIDs[library.ItemID] = struct{}{}
		}
		metadata["failed_items"] = failedItems
		metadata["submitted_count"] = len(targetIDs)
		metadata["failed_count"] = len(failedItems)
		_ = s.tasks.UpdateMetadata(taskID, metadata)
		if len(targetIDs) == 0 {
			metadata["success_count"] = 0
			_ = s.tasks.UpdateProgress(taskID, total, total, 0, len(failedItems))
			s.failEmbyTask(taskID, metadata, 1, fmt.Errorf("全部媒体库刷新请求均提交失败"))
			metadata["conclusion"] = "全部媒体库刷新请求均提交失败"
			_ = s.tasks.UpdateMetadata(taskID, metadata)
			return
		}
	}
	submitMessage := "Emby 已接受刷新请求"
	if len(failedItems) > 0 {
		submitMessage = fmt.Sprintf("Emby 已接受 %d 个请求，%d 个提交失败", len(targetIDs), len(failedItems))
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 1, "success", submitMessage, 10)
	deadline := time.NewTimer(s.pollTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	seenRunning := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			metadata["conclusion"] = "Emby 已接受请求，最终结果未能确认"
			updateEmbyTaskStep(s.tasks, taskID, metadata, 2, "unknown", "等待 Emby 终态超时", 10)
			_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusUnknown)
			return
		case <-ticker.C:
			libs, err := s.ListLibraries(server.ID)
			if err != nil {
				metadata["last_poll_error"] = err.Error()
				_ = s.tasks.UpdateMetadata(taskID, metadata)
				continue
			}
			progress, running := refreshProgressForTargets(libs, targetIDs)
			if running {
				seenRunning = true
				updateEmbyTaskStep(s.tasks, taskID, metadata, 2, "running", "Emby 正在刷新媒体库", maxInt(10, int(progress)))
				continue
			}
			if seenRunning || allRefreshTargetsIdle(libs, targetIDs) {
				updateEmbyTaskStep(s.tasks, taskID, metadata, 2, "success", "Emby 刷新已结束", 95)
				updateEmbyTaskStep(s.tasks, taskID, metadata, 3, "success", "已确认刷新终态", 98)
				metadata["success_count"] = len(targetIDs)
				metadata["failed_count"] = len(failedItems)
				finalStatus := domain.TaskStatusSuccess
				conclusion := "Emby 媒体库刷新成功"
				if len(failedItems) > 0 {
					finalStatus = domain.TaskStatusPartialSuccess
					conclusion = fmt.Sprintf("媒体库刷新部分成功：%d 个成功，%d 个失败", len(targetIDs), len(failedItems))
				}
				metadata["conclusion"] = conclusion
				updateEmbyTaskStep(s.tasks, taskID, metadata, 4, "success", conclusion, 100)
				_ = s.tasks.UpdateProgress(taskID, total, total, len(targetIDs), len(failedItems))
				_ = s.tasks.UpdateStatus(taskID, finalStatus)
				return
			}
		}
	}
}

func refreshProgressForTargets(libs []EmbyVirtualFolderInfo, targetIDs map[string]struct{}) (float64, bool) {
	maxProgress := float64(0)
	running := false
	for _, lib := range libs {
		if _, ok := targetIDs[lib.ItemID]; !ok {
			continue
		}
		if lib.RefreshProgress > maxProgress {
			maxProgress = lib.RefreshProgress
		}
		status := strings.ToLower(lib.RefreshStatus)
		if lib.RefreshProgress > 0 && lib.RefreshProgress < 100 || (status != "" && status != "idle") {
			running = true
		}
	}
	return maxProgress, running
}

func allRefreshTargetsIdle(libs []EmbyVirtualFolderInfo, targetIDs map[string]struct{}) bool {
	matched := 0
	for _, lib := range libs {
		if _, ok := targetIDs[lib.ItemID]; !ok {
			continue
		}
		matched++
		if _, running := refreshProgressForTargets([]EmbyVirtualFolderInfo{lib}, targetIDs); running {
			return false
		}
	}
	return matched == len(targetIDs)
}

// GetStrmAssistantStatus 检测神医助手以及可触发的计划任务。
func (s *EmbyManagementService) GetStrmAssistantStatus(serverID int) (map[string]interface{}, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	var plugins []map[string]interface{}
	if err = s.requestJSON(server, http.MethodGet, "/emby/Plugins", nil, nil, &plugins); err != nil {
		return nil, err
	}
	var scheduled []map[string]interface{}
	if err = s.requestJSON(server, http.MethodGet, "/emby/ScheduledTasks", nil, nil, &scheduled); err != nil {
		return nil, err
	}
	installed := false
	version := ""
	for _, plugin := range plugins {
		name := strings.ToLower(fmt.Sprint(plugin["Name"]))
		if strings.Contains(name, "strm assistant") || strings.Contains(name, "strmassistant") || strings.Contains(name, "神医") {
			installed = true
			version = fmt.Sprint(plugin["Version"])
			break
		}
	}
	capabilities := map[string]interface{}{}
	for _, action := range []string{"media_info", "subtitle_scan", "metadata_refresh"} {
		id, name := findStrmTask(scheduled, action)
		capabilities[action] = map[string]interface{}{"available": id != "", "task_id": id, "task_name": name}
	}
	mediaInfo, _ := capabilities["media_info"].(map[string]interface{})
	mediaInfoTaskID := fmt.Sprint(mediaInfo["task_id"])
	capabilities[strmScanCaptureAction] = map[string]interface{}{
		"available": mediaInfoTaskID != "", "task_id": mediaInfoTaskID,
		"task_name": "扫描 STRM 并提取视频封面", "requires_library": true,
		"requirement": "需在 Emby 媒体库中启用 Image Capture，并确保神医助手 Library Scope 包含所选媒体库；计划任务可能同时处理插件配置范围内的其他媒体库。",
	}
	return map[string]interface{}{"installed": installed, "version": version, "available": installed, "capabilities": capabilities, "install_url": "https://github.com/sjtuross/StrmAssistant/wiki", "message": map[bool]string{true: "神医助手已安装", false: "当前 Emby 未检测到神医助手（StrmAssistant）。以下功能依赖该插件，未安装时不会生效。"}[installed]}, nil
}

// StartStrmAssistantTask 触发并跟踪神医助手计划任务。
func (s *EmbyManagementService) StartStrmAssistantTask(serverID int, action, libraryID string) (string, error) {
	return s.StartStrmAssistantTaskWithOptions(serverID, action, libraryID, false)
}

// StartStrmAssistantTaskWithOptions 触发神医助手任务，并可在 STRM 截图前自动配置依赖项。
func (s *EmbyManagementService) StartStrmAssistantTaskWithOptions(serverID int, action, libraryID string, autoConfigure bool) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	status, err := s.GetStrmAssistantStatus(serverID)
	if err != nil {
		return "", err
	}
	if installed, _ := status["installed"].(bool); !installed {
		return "", fmt.Errorf("当前 Emby 未安装神医助手")
	}
	if action == strmScanCaptureAction && strings.TrimSpace(libraryID) == "" {
		return "", fmt.Errorf("扫描 STRM 并提取视频封面必须选择媒体库")
	}
	capabilities, _ := status["capabilities"].(map[string]interface{})
	capability, ok := capabilities[action].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("不支持的神医助手操作：%s", action)
	}
	remoteID := fmt.Sprint(capability["task_id"])
	if remoteID == "" {
		return "", fmt.Errorf("神医助手未提供所选计划任务")
	}
	metadata := taskMetadata(server, action, libraryID)
	if action == strmScanCaptureAction {
		var libraries []EmbyVirtualFolderInfo
		if err = s.requestJSON(server, http.MethodGet, "/emby/Library/VirtualFolders", nil, nil, &libraries); err != nil {
			return "", fmt.Errorf("读取 Emby 媒体库失败: %w", err)
		}
		libraryName := ""
		for _, library := range libraries {
			if library.ItemID == libraryID {
				libraryName = library.Name
				break
			}
		}
		if libraryName == "" {
			return "", fmt.Errorf("所选 Emby 媒体库不存在")
		}
		metadata["library_id"] = libraryID
		metadata["library_name"] = libraryName
		metadata["target"] = libraryName
	}
	metadata["remote_task_id"] = remoteID
	metadata["current_step"] = "等待执行"
	metadata["steps"] = buildEmbySteps("检测插件与计划任务", "提交神医助手任务", "跟踪计划任务进度", "收集结束状态", "写入执行结论")
	taskName := "神医助手任务"
	if action == strmScanCaptureAction {
		taskName = "扫描 STRM 并提取视频封面"
		metadata["auto_configure"] = autoConfigure
		metadata["steps"] = buildEmbySteps("检测神医助手与媒体库", "读取 Image Capture 与 Library Scope", "启用媒体库 Image Capture", "合并神医助手 Library Scope", "回读并核验配置", "提交 Emby 媒体库扫描", "等待 STRM 扫描完成", "统计 STRM 与现有封面", "提交视频截图任务", "跟踪截图任务进度", "回读封面覆盖结果", "写入执行结论")
	}
	taskID := "emby-plugin-" + uuid.NewString()
	if err = s.tasks.Create(taskID, string(domain.TaskTypeEmbyPlugin), taskName); err != nil {
		return "", err
	}
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	ctx, cancel := context.WithCancel(context.Background())
	s.tasks.RegisterCancel(taskID, cancel)
	if action == strmScanCaptureAction {
		go s.runStrmScanCaptureTask(ctx, taskID, server, remoteID, libraryID, autoConfigure, metadata)
	} else {
		go s.runPluginTask(ctx, taskID, server, remoteID, libraryID, metadata)
	}
	return taskID, nil
}

func (s *EmbyManagementService) runStrmScanCaptureTask(ctx context.Context, taskID string, server *domain.EmbyServer, remoteID, libraryID string, autoConfigure bool, metadata map[string]interface{}) {
	defer s.tasks.RemoveCancel(taskID)
	_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusRunning)
	updateEmbyTaskStep(s.tasks, taskID, metadata, 0, "success", "已检测到神医助手媒体信息提取任务", 3)
	if autoConfigure {
		if err := s.configureStrmCaptureDependencies(server, libraryID, taskID, metadata); err != nil {
			s.failEmbyTask(taskID, metadata, 4, err)
			return
		}
	} else {
		for index := 1; index <= 4; index++ {
			updateEmbyTaskStep(s.tasks, taskID, metadata, index, "skipped", "未请求自动配置，沿用现有 Emby 与神医助手设置", 8)
		}
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 5, "running", "正在提交媒体库扫描", 10)
	if err := s.requestJSON(server, http.MethodPost, "/emby/Items/"+url.PathEscape(libraryID)+"/Refresh", nil, nil, nil); err != nil {
		s.failEmbyTask(taskID, metadata, 5, err)
		return
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 5, "success", "Emby 已接受媒体库扫描请求", 15)
	if unknown, err := s.waitForLibraryIdle(ctx, server, libraryID, func(progress int) {
		updateEmbyTaskStep(s.tasks, taskID, metadata, 6, "running", "Emby 正在扫描 STRM 文件", maxInt(15, minInt(45, 15+progress*30/100)))
	}); err != nil {
		if ctx.Err() != nil {
			return
		}
		s.failEmbyTask(taskID, metadata, 6, err)
		return
	} else if unknown {
		s.finishUnknownTask(taskID, metadata, 6, "Emby 已接受媒体库扫描，但未能确认扫描终态")
		return
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 6, "success", "媒体库扫描已结束", 45)
	strmTotal, coveredBefore, err := s.getStrmCoverStats(server, libraryID)
	if err != nil {
		s.failEmbyTask(taskID, metadata, 7, err)
		return
	}
	metadata["strm_count"] = strmTotal
	metadata["covered_before"] = coveredBefore
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	updateEmbyTaskStep(s.tasks, taskID, metadata, 7, "success", fmt.Sprintf("发现 %d 个 STRM 视频，其中 %d 个已有主图", strmTotal, coveredBefore), 50)
	if strmTotal == 0 {
		metadata["covered_after"] = 0
		metadata["generated_cover_count"] = 0
		metadata["conclusion"] = "媒体库扫描完成，未发现 STRM 视频"
		updateEmbyTaskStep(s.tasks, taskID, metadata, 11, "success", metadata["conclusion"].(string), 100)
		_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusSuccess)
		return
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 8, "running", "正在提交神医助手媒体信息与视频截图任务", 52)
	query := url.Values{"LibraryId": {libraryID}}
	if err = s.requestJSON(server, http.MethodPost, "/emby/ScheduledTasks/Running/"+url.PathEscape(remoteID), query, nil, nil); err != nil {
		s.failEmbyTask(taskID, metadata, 8, err)
		return
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 8, "success", "神医助手已接受视频截图任务", 55)
	if unknown, err := s.waitForScheduledTask(ctx, server, remoteID, func(progress int) {
		updateEmbyTaskStep(s.tasks, taskID, metadata, 9, "running", "神医助手正在提取媒体信息并生成缺失封面", maxInt(55, minInt(90, 55+progress*35/100)))
	}); err != nil {
		if ctx.Err() != nil {
			return
		}
		s.failEmbyTask(taskID, metadata, 9, err)
		return
	} else if unknown {
		s.finishUnknownTask(taskID, metadata, 9, "神医助手已接受视频截图任务，但未能确认最终结果")
		return
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 9, "success", "神医助手视频截图任务已结束", 92)
	strmAfter, coveredAfter, err := s.getStrmCoverStats(server, libraryID)
	if err != nil {
		s.failEmbyTask(taskID, metadata, 10, err)
		return
	}
	generated := maxInt(0, coveredAfter-coveredBefore)
	metadata["strm_count"] = strmAfter
	metadata["covered_after"] = coveredAfter
	metadata["generated_cover_count"] = generated
	missing := maxInt(0, strmAfter-coveredAfter)
	metadata["missing_cover_count"] = missing
	updateEmbyTaskStep(s.tasks, taskID, metadata, 10, "success", fmt.Sprintf("回读确认 %d/%d 个 STRM 视频已有主图", coveredAfter, strmAfter), 98)
	metadata["conclusion"] = fmt.Sprintf("STRM 扫描与截图任务已完成：共 %d 个 STRM 视频，新增 %d 个主图，仍有 %d 个缺少主图", strmAfter, generated, missing)
	updateEmbyTaskStep(s.tasks, taskID, metadata, 11, "success", metadata["conclusion"].(string), 100)
	_ = s.tasks.UpdateProgress(taskID, strmAfter, strmAfter, coveredAfter, missing)
	if missing == 0 {
		_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusSuccess)
	} else if coveredAfter > 0 {
		_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusPartialSuccess)
	} else {
		_ = s.tasks.SetError(taskID, "神医助手任务已结束，但所选媒体库仍没有 STRM 视频主图；请检查 Image Capture、Library Scope 与 ffmpeg 配置")
	}
}

func (s *EmbyManagementService) configureStrmCaptureDependencies(server *domain.EmbyServer, libraryID, taskID string, metadata map[string]interface{}) error {
	updateEmbyTaskStep(s.tasks, taskID, metadata, 1, "running", "正在读取媒体库与神医助手截图配置", 4)
	libraries, err := s.listLibrariesByServer(server)
	if err != nil {
		return fmt.Errorf("读取媒体库 Image Capture 配置失败: %w", err)
	}
	var library *EmbyVirtualFolderInfo
	for index := range libraries {
		if libraries[index].ItemID == libraryID {
			library = &libraries[index]
			break
		}
	}
	if library == nil {
		return fmt.Errorf("所选 Emby 媒体库不存在")
	}
	pluginObject, pageID, err := s.readStrmAssistantMediaInfoOptions(server)
	if err != nil {
		return err
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 1, "success", "已读取 Image Capture 与 Library Scope", 6)

	updateEmbyTaskStep(s.tasks, taskID, metadata, 2, "running", "正在启用媒体库 Image Capture", 7)
	libraryChanged, err := enableLibraryImageCapture(library.CollectionType, library.LibraryOptions)
	if err != nil {
		return err
	}
	if libraryChanged {
		paths := make([]map[string]string, 0, len(library.Locations))
		for _, location := range library.Locations {
			paths = append(paths, map[string]string{"Path": location})
		}
		body := map[string]interface{}{
			"Id": library.ItemID, "Name": library.Name, "CollectionType": library.CollectionType,
			"Paths": paths, "LibraryOptions": library.LibraryOptions,
		}
		query := url.Values{"name": {library.Name}, "refreshLibrary": {"false"}}
		if err = s.requestJSON(server, http.MethodPost, "/emby/Library/VirtualFolders/LibraryOptions", query, body, nil); err != nil {
			return fmt.Errorf("启用媒体库 Image Capture 失败: %w", err)
		}
	}
	imageMessage := "媒体库 Image Capture 已启用"
	if !libraryChanged {
		imageMessage = "媒体库 Image Capture 已处于启用状态"
	}
	metadata["image_capture_changed"] = libraryChanged
	updateEmbyTaskStep(s.tasks, taskID, metadata, 2, "success", imageMessage, 8)

	updateEmbyTaskStep(s.tasks, taskID, metadata, 3, "running", "正在合并神医助手 Library Scope", 9)
	scopeChanged := mergeStrmAssistantLibraryScope(pluginObject, libraryID)
	if scopeChanged {
		if err = s.saveStrmAssistantMediaInfoOptions(server, pageID, pluginObject); err != nil {
			return err
		}
	}
	scopeMessage := "神医助手 Library Scope 已包含所选媒体库"
	if strings.TrimSpace(fmt.Sprint(pluginObject["LibraryScope"])) == "" {
		scopeMessage = "神医助手 Library Scope 为空，已覆盖全部媒体库"
	} else if !scopeChanged {
		scopeMessage = "神医助手 Library Scope 已包含所选媒体库，无需修改"
	}
	metadata["library_scope_changed"] = scopeChanged
	updateEmbyTaskStep(s.tasks, taskID, metadata, 3, "success", scopeMessage, 10)

	updateEmbyTaskStep(s.tasks, taskID, metadata, 4, "running", "正在回读并核验配置", 11)
	verifiedLibraries, err := s.listLibrariesByServer(server)
	if err != nil {
		return fmt.Errorf("回读媒体库配置失败: %w", err)
	}
	verifiedImageCapture := false
	for index := range verifiedLibraries {
		if verifiedLibraries[index].ItemID == libraryID {
			verifiedImageCapture = libraryImageCaptureEnabled(verifiedLibraries[index].CollectionType, verifiedLibraries[index].LibraryOptions)
			break
		}
	}
	if !verifiedImageCapture {
		return fmt.Errorf("回读核验失败：媒体库 Image Capture 未生效")
	}
	verifiedPlugin, _, err := s.readStrmAssistantMediaInfoOptions(server)
	if err != nil {
		return fmt.Errorf("回读神医助手 Library Scope 失败: %w", err)
	}
	if !scopeIncludesLibrary(fmt.Sprint(verifiedPlugin["LibraryScope"]), libraryID) {
		return fmt.Errorf("回读核验失败：神医助手 Library Scope 未包含所选媒体库")
	}
	metadata["configuration_verified"] = true
	updateEmbyTaskStep(s.tasks, taskID, metadata, 4, "success", "Image Capture 与 Library Scope 已回读确认生效", 12)
	return nil
}

func (s *EmbyManagementService) listLibrariesByServer(server *domain.EmbyServer) ([]EmbyVirtualFolderInfo, error) {
	libraries := make([]EmbyVirtualFolderInfo, 0)
	err := s.requestJSON(server, http.MethodGet, "/emby/Library/VirtualFolders", nil, nil, &libraries)
	return libraries, err
}

func enableLibraryImageCapture(collectionType string, options map[string]interface{}) (bool, error) {
	typeOptions, ok := options["TypeOptions"].([]interface{})
	if !ok {
		return false, fmt.Errorf("媒体库未返回可修改的 TypeOptions")
	}
	targets := imageCaptureTypes(collectionType)
	matched := false
	changed := false
	for _, raw := range typeOptions {
		item, itemOK := raw.(map[string]interface{})
		if !itemOK || !targets[fmt.Sprint(item["Type"])] {
			continue
		}
		matched = true
		fetchers := interfaceStringSlice(item["ImageFetchers"])
		if !containsString(fetchers, "Image Capture") {
			item["ImageFetchers"] = append(fetchers, "Image Capture")
			changed = true
		}
		order := interfaceStringSlice(item["ImageFetcherOrder"])
		if !containsString(order, "Image Capture") {
			item["ImageFetcherOrder"] = append(order, "Image Capture")
			changed = true
		}
	}
	if !matched {
		return false, fmt.Errorf("媒体库没有适用的视频类型，无法启用 Image Capture")
	}
	return changed, nil
}

func libraryImageCaptureEnabled(collectionType string, options map[string]interface{}) bool {
	typeOptions, _ := options["TypeOptions"].([]interface{})
	targets := imageCaptureTypes(collectionType)
	matched := false
	for _, raw := range typeOptions {
		item, ok := raw.(map[string]interface{})
		if !ok || !targets[fmt.Sprint(item["Type"])] {
			continue
		}
		matched = true
		if !containsString(interfaceStringSlice(item["ImageFetchers"]), "Image Capture") {
			return false
		}
	}
	return matched
}

func imageCaptureTypes(collectionType string) map[string]bool {
	switch strings.ToLower(collectionType) {
	case "tvshows":
		return map[string]bool{"Episode": true}
	case "movies":
		return map[string]bool{"Movie": true}
	case "homevideos":
		return map[string]bool{"Video": true}
	case "music", "musicvideos":
		return map[string]bool{"MusicVideo": true}
	default:
		return map[string]bool{"Episode": true, "Movie": true}
	}
}

func (s *EmbyManagementService) readStrmAssistantMediaInfoOptions(server *domain.EmbyServer) (map[string]interface{}, string, error) {
	query := url.Values{"PageId": {"63c322:MediaInfoExtractPageView"}, "ClientLocale": {"zh-CN"}}
	var view map[string]interface{}
	if err := s.requestJSON(server, http.MethodGet, "/emby/UI/View", query, nil, &view); err != nil {
		return nil, "", fmt.Errorf("读取神医助手 Library Scope 失败：当前 Emby 凭据无法访问插件配置接口: %w", err)
	}
	container, _ := view["EditObjectContainer"].(map[string]interface{})
	object, _ := container["Object"].(map[string]interface{})
	if object == nil {
		return nil, "", fmt.Errorf("神医助手配置接口未返回 MediaInfoExtractOptions")
	}
	pageID := strings.TrimSpace(fmt.Sprint(view["PageId"]))
	if pageID == "" {
		pageID = "63c322:MediaInfoExtractPageView"
	}
	return object, pageID, nil
}

func (s *EmbyManagementService) saveStrmAssistantMediaInfoOptions(server *domain.EmbyServer, pageID string, object map[string]interface{}) error {
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}
	body := map[string]interface{}{
		"PageId": pageID, "CommandId": "PageSave", "Data": string(data),
		"ItemId": nil, "ClientLocale": "zh-CN",
	}
	var result map[string]interface{}
	if err = s.requestJSON(server, http.MethodPost, "/emby/UI/Command", nil, body, &result); err != nil {
		return fmt.Errorf("保存神医助手 Library Scope 失败: %w", err)
	}
	return nil
}

func mergeStrmAssistantLibraryScope(object map[string]interface{}, libraryID string) bool {
	scope := strings.TrimSpace(fmt.Sprint(object["LibraryScope"]))
	changed := false
	if scope != "" && !scopeIncludesLibrary(scope, libraryID) {
		parts := splitScope(scope)
		object["LibraryScope"] = strings.Join(append(parts, libraryID), ",")
		changed = true
	}
	if enabled, exists := object["EnableImageCapture"].(bool); exists && !enabled {
		object["EnableImageCapture"] = true
		changed = true
	}
	return changed
}

func scopeIncludesLibrary(scope, libraryID string) bool {
	if strings.TrimSpace(scope) == "" {
		return true
	}
	return containsString(splitScope(scope), libraryID)
}

func splitScope(scope string) []string {
	parts := make([]string, 0)
	for _, value := range strings.Split(scope, ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" && !containsString(parts, trimmed) {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func interfaceStringSlice(value interface{}) []string {
	result := make([]string, 0)
	switch items := value.(type) {
	case []interface{}:
		for _, item := range items {
			result = append(result, fmt.Sprint(item))
		}
	case []string:
		result = append(result, items...)
	}
	return result
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func (s *EmbyManagementService) runPluginTask(ctx context.Context, taskID string, server *domain.EmbyServer, remoteID, libraryID string, metadata map[string]interface{}) {
	defer s.tasks.RemoveCancel(taskID)
	_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusRunning)
	updateEmbyTaskStep(s.tasks, taskID, metadata, 0, "success", "已检测到神医助手计划任务", 5)
	query := url.Values{}
	if libraryID != "" {
		query.Set("LibraryId", libraryID)
	}
	if err := s.requestJSON(server, http.MethodPost, "/emby/ScheduledTasks/Running/"+url.PathEscape(remoteID), query, nil, nil); err != nil {
		s.failEmbyTask(taskID, metadata, 1, err)
		return
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 1, "success", "任务已提交", 10)
	deadline := time.NewTimer(s.pollTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	seenRunning := false
	idlePolls := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			metadata["conclusion"] = "Emby 已接受请求，最终结果未能确认"
			_ = s.tasks.UpdateMetadata(taskID, metadata)
			_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusUnknown)
			return
		case <-ticker.C:
			var scheduled []map[string]interface{}
			if err := s.requestJSON(server, http.MethodGet, "/emby/ScheduledTasks", nil, nil, &scheduled); err != nil {
				continue
			}
			remote := findScheduledTaskByID(scheduled, remoteID)
			if remote == nil {
				continue
			}
			state := strings.ToLower(fmt.Sprint(remote["State"]))
			progress := numberToInt(remote["CurrentProgressPercentage"])
			if state == "running" {
				seenRunning = true
				idlePolls = 0
				updateEmbyTaskStep(s.tasks, taskID, metadata, 2, "running", "神医助手正在执行", maxInt(10, progress))
				continue
			}
			idlePolls++
			if seenRunning || idlePolls >= 2 {
				lastResult, _ := remote["LastExecutionResult"].(map[string]interface{})
				resultStatus := strings.ToLower(fmt.Sprint(lastResult["Status"]))
				errorMessage := nullableString(lastResult["ErrorMessage"])
				if resultStatus == "failed" || errorMessage != "" {
					if errorMessage == "" {
						errorMessage = "神医助手任务执行失败"
					}
					s.failEmbyTask(taskID, metadata, 3, fmt.Errorf("%s", errorMessage))
					return
				}
				updateEmbyTaskStep(s.tasks, taskID, metadata, 2, "success", "计划任务执行结束", 95)
				updateEmbyTaskStep(s.tasks, taskID, metadata, 3, "success", "已取得 Emby 执行结论", 98)
				metadata["conclusion"] = "神医助手任务执行成功"
				updateEmbyTaskStep(s.tasks, taskID, metadata, 4, "success", "执行成功", 100)
				_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusSuccess)
				return
			}
		}
	}
}

func (s *EmbyManagementService) waitForLibraryIdle(ctx context.Context, server *domain.EmbyServer, libraryID string, onProgress func(int)) (bool, error) {
	deadline := time.NewTimer(s.pollTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	seenRunning := false
	idlePolls := 0
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return true, nil
		case <-ticker.C:
			var libraries []EmbyVirtualFolderInfo
			if err := s.requestJSON(server, http.MethodGet, "/emby/Library/VirtualFolders", nil, nil, &libraries); err != nil {
				continue
			}
			found := false
			running := false
			progress := 0
			for _, library := range libraries {
				if library.ItemID != libraryID {
					continue
				}
				found = true
				progress = numberToInt(library.RefreshProgress)
				status := strings.ToLower(library.RefreshStatus)
				running = library.RefreshProgress > 0 && library.RefreshProgress < 100 || status != "" && status != "idle"
				break
			}
			if !found {
				return false, fmt.Errorf("Emby 扫描期间未找到目标媒体库")
			}
			if running {
				seenRunning = true
				idlePolls = 0
				onProgress(progress)
				continue
			}
			idlePolls++
			if seenRunning || idlePolls >= 2 {
				return false, nil
			}
		}
	}
}

func (s *EmbyManagementService) waitForScheduledTask(ctx context.Context, server *domain.EmbyServer, remoteID string, onProgress func(int)) (bool, error) {
	deadline := time.NewTimer(s.pollTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	seenRunning := false
	idlePolls := 0
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return true, nil
		case <-ticker.C:
			var scheduled []map[string]interface{}
			if err := s.requestJSON(server, http.MethodGet, "/emby/ScheduledTasks", nil, nil, &scheduled); err != nil {
				continue
			}
			remote := findScheduledTaskByID(scheduled, remoteID)
			if remote == nil {
				continue
			}
			state := strings.ToLower(fmt.Sprint(remote["State"]))
			if state == "running" {
				seenRunning = true
				idlePolls = 0
				onProgress(numberToInt(remote["CurrentProgressPercentage"]))
				continue
			}
			idlePolls++
			if !seenRunning && idlePolls < 2 {
				continue
			}
			lastResult, _ := remote["LastExecutionResult"].(map[string]interface{})
			resultStatus := strings.ToLower(fmt.Sprint(lastResult["Status"]))
			errorMessage := nullableString(lastResult["ErrorMessage"])
			if resultStatus == "failed" || resultStatus == "cancelled" || errorMessage != "" {
				if errorMessage == "" {
					errorMessage = "神医助手任务执行失败，状态：" + resultStatus
				}
				return false, fmt.Errorf("%s", errorMessage)
			}
			return false, nil
		}
	}
}

func (s *EmbyManagementService) getStrmCoverStats(server *domain.EmbyServer, libraryID string) (int, int, error) {
	totalSTRM := 0
	covered := 0
	startIndex := 0
	const pageSize = 1000
	for {
		query := url.Values{
			"ParentId": {libraryID}, "Recursive": {"true"}, "IncludeItemTypes": {"Movie,Episode,Video"},
			"Fields": {"Path,ImageTags"}, "StartIndex": {strconv.Itoa(startIndex)}, "Limit": {strconv.Itoa(pageSize)},
		}
		var response struct {
			Items []struct {
				Path            string            `json:"Path"`
				PrimaryImageTag string            `json:"PrimaryImageTag"`
				ImageTags       map[string]string `json:"ImageTags"`
			} `json:"Items"`
			TotalRecordCount int `json:"TotalRecordCount"`
		}
		if err := s.requestJSON(server, http.MethodGet, "/emby/Items", query, nil, &response); err != nil {
			return 0, 0, fmt.Errorf("回读 STRM 封面状态失败: %w", err)
		}
		for _, item := range response.Items {
			if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(item.Path)), ".strm") {
				continue
			}
			totalSTRM++
			if item.PrimaryImageTag != "" || item.ImageTags["Primary"] != "" {
				covered++
			}
		}
		startIndex += len(response.Items)
		if len(response.Items) == 0 || startIndex >= response.TotalRecordCount {
			break
		}
	}
	return totalSTRM, covered, nil
}

func (s *EmbyManagementService) finishUnknownTask(taskID string, metadata map[string]interface{}, step int, conclusion string) {
	metadata["conclusion"] = conclusion
	steps, _ := metadata["steps"].([]map[string]interface{})
	if step >= 0 && step < len(steps) {
		steps[step]["status"] = "unknown"
		steps[step]["message"] = conclusion
	}
	metadata["current_step"] = conclusion
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusUnknown)
}

func findStrmTask(tasks []map[string]interface{}, action string) (string, string) {
	keywords := map[string][]string{"media_info": {"media info", "mediainfo", "媒体信息"}, "subtitle_scan": {"subtitle", "字幕"}, "metadata_refresh": {"metadata", "元数据"}}[action]
	for _, task := range tasks {
		name := strings.ToLower(fmt.Sprint(task["Name"]))
		category := strings.ToLower(fmt.Sprint(task["Category"]))
		combined := name + " " + category
		if !strings.Contains(combined, "strm") && !strings.Contains(combined, "神医") {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(combined, strings.ToLower(keyword)) {
				return fmt.Sprint(task["Id"]), fmt.Sprint(task["Name"])
			}
		}
	}
	return "", ""
}

func findScheduledTaskByID(tasks []map[string]interface{}, id string) map[string]interface{} {
	for _, task := range tasks {
		if fmt.Sprint(task["Id"]) == id {
			return task
		}
	}
	return nil
}

// SaveAIConfig 保存 AI 封面供应商配置，API Key 留空时保持原值。
func (s *EmbyManagementService) SaveAIConfig(provider, baseURL, apiKey, model string) error {
	if provider != "openai" && provider != "compatible" {
		return fmt.Errorf("不支持的 AI 供应商")
	}
	if provider == "openai" && strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com/v1"
	}
	for key, value := range map[string]string{"emby_cover_ai_provider": provider, "emby_cover_ai_base_url": strings.TrimRight(baseURL, "/"), "emby_cover_ai_model": model} {
		if err := s.configs.Upsert(key, value); err != nil {
			return err
		}
	}
	if strings.TrimSpace(apiKey) != "" && !strings.Contains(apiKey, "********") {
		return s.configs.Upsert("emby_cover_ai_api_key", strings.TrimSpace(apiKey))
	}
	return nil
}

// GetAIConfig 返回脱敏后的 AI 封面配置。
func (s *EmbyManagementService) GetAIConfig() map[string]string {
	result := map[string]string{"provider": "openai", "base_url": "https://api.openai.com/v1", "model": "gpt-image-1", "api_key_mask": ""}
	for key, outputKey := range map[string]string{"emby_cover_ai_provider": "provider", "emby_cover_ai_base_url": "base_url", "emby_cover_ai_model": "model"} {
		if config, _ := s.configs.GetByKey(key); config != nil && config.ConfigVal != "" {
			result[outputKey] = config.ConfigVal
		}
	}
	if config, _ := s.configs.GetByKey("emby_cover_ai_api_key"); config != nil {
		result["api_key_mask"] = maskSecret(config.ConfigVal)
	}
	return result
}

// UploadLibraryCover 上传并回读媒体库主封面。
func (s *EmbyManagementService) UploadLibraryCover(serverID int, libraryID, contentType string, data []byte) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	if !isSupportedImage(contentType) {
		return "", fmt.Errorf("仅支持 JPG、PNG 或 WebP 图片")
	}
	metadata := taskMetadata(server, "upload_cover", libraryID)
	return s.runShortTask(domain.TaskTypeEmbyCover, "上传 Emby 媒体库封面", metadata, func() error { return s.uploadImage(server, libraryID, contentType, data) })
}

// GenerateCollageCover 生成海报拼图预览，确认后再上传。
func (s *EmbyManagementService) GenerateCollageCover(serverID int, libraryID, libraryName string) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	return s.startCoverGeneration(server, libraryID, libraryName, "collage", func(taskID string) ([]byte, string, error) { return s.buildCollage(server, libraryID, libraryName) })
}

// GenerateAICover 调用配置的图片接口生成封面预览。
func (s *EmbyManagementService) GenerateAICover(serverID int, libraryID, libraryName, description string) (string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	return s.startCoverGeneration(server, libraryID, libraryName, "ai", func(taskID string) ([]byte, string, error) { return s.generateAIImage(libraryName, description) })
}

func (s *EmbyManagementService) startCoverGeneration(server *domain.EmbyServer, libraryID, libraryName, mode string, generate func(string) ([]byte, string, error)) (string, error) {
	taskID := "emby-cover-" + uuid.NewString()
	metadata := taskMetadata(server, mode+"_cover", libraryID)
	metadata["library_name"] = libraryName
	metadata["steps"] = buildEmbySteps("读取媒体库及素材", "生成封面", "保存临时预览", "等待用户确认", "上传至 Emby", "回读封面确认生效")
	if err := s.tasks.Create(taskID, string(domain.TaskTypeEmbyCover), "生成 Emby 媒体库封面"); err != nil {
		return "", err
	}
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	go func() {
		_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusRunning)
		updateEmbyTaskStep(s.tasks, taskID, metadata, 0, "running", "正在读取素材", 5)
		data, contentType, err := generate(taskID)
		if err != nil {
			s.failEmbyTask(taskID, metadata, 1, err)
			return
		}
		updateEmbyTaskStep(s.tasks, taskID, metadata, 1, "success", "封面生成完成", 45)
		path, err := s.savePreview(taskID, data)
		if err != nil {
			s.failEmbyTask(taskID, metadata, 2, err)
			return
		}
		metadata["preview_path"] = path
		metadata["preview_url"] = "/emby/cover-previews/" + taskID
		metadata["content_type"] = contentType
		metadata["awaiting_confirmation"] = true
		metadata["current_step"] = "等待用户确认"
		updateEmbyTaskStep(s.tasks, taskID, metadata, 2, "success", "预览已保存", 50)
		updateEmbyTaskStep(s.tasks, taskID, metadata, 3, "pending", "等待用户确认应用", 50)
		_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusPending)
	}()
	return taskID, nil
}

// ApplyGeneratedCover 将任务预览应用到 Emby，并继续更新原任务。
func (s *EmbyManagementService) ApplyGeneratedCover(serverID int, libraryID, taskID string) error {
	server, err := s.requireServer(serverID)
	if err != nil {
		return err
	}
	task, err := s.tasks.Get(taskID)
	if err != nil || task == nil {
		return fmt.Errorf("封面生成任务不存在")
	}
	metadata, _ := task["metadata"].(map[string]interface{})
	if metadata == nil || metadata["awaiting_confirmation"] != true {
		return fmt.Errorf("该任务没有待确认的封面")
	}
	data, err := os.ReadFile(fmt.Sprint(metadata["preview_path"]))
	if err != nil {
		return fmt.Errorf("读取封面预览失败: %w", err)
	}
	_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusRunning)
	metadata["awaiting_confirmation"] = false
	updateEmbyTaskStep(s.tasks, taskID, metadata, 3, "success", "用户已确认应用", 55)
	if err = s.uploadImage(server, libraryID, fmt.Sprint(metadata["content_type"]), data); err != nil {
		s.failEmbyTask(taskID, metadata, 4, err)
		return err
	}
	updateEmbyTaskStep(s.tasks, taskID, metadata, 4, "success", "封面已上传至 Emby", 90)
	updateEmbyTaskStep(s.tasks, taskID, metadata, 5, "success", "Emby 已接受并保存主封面", 100)
	metadata["conclusion"] = "媒体库封面已生效"
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusSuccess)
	_ = os.Remove(fmt.Sprint(metadata["preview_path"]))
	return nil
}

// GetCoverPreview 返回有效期内的封面预览。
func (s *EmbyManagementService) GetCoverPreview(taskID string) ([]byte, string, error) {
	task, err := s.tasks.Get(taskID)
	if err != nil || task == nil {
		return nil, "", fmt.Errorf("预览不存在")
	}
	metadata, _ := task["metadata"].(map[string]interface{})
	path := fmt.Sprint(metadata["preview_path"])
	if path == "" {
		return nil, "", fmt.Errorf("预览不存在")
	}
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > embyPreviewTTL {
		return nil, "", fmt.Errorf("预览已过期")
	}
	data, err := os.ReadFile(path)
	return data, fmt.Sprint(metadata["content_type"]), err
}

func (s *EmbyManagementService) buildCollage(server *domain.EmbyServer, libraryID, title string) ([]byte, string, error) {
	query := url.Values{"ParentId": {libraryID}, "Recursive": {"true"}, "ImageTypes": {"Primary"}, "Limit": {"6"}, "SortBy": {"DateCreated"}, "SortOrder": {"Descending"}}
	var response struct {
		Items []struct {
			ID string `json:"Id"`
		} `json:"Items"`
	}
	if err := s.requestJSON(server, http.MethodGet, "/emby/Items", query, nil, &response); err != nil {
		return nil, "", err
	}
	images := make([]image.Image, 0, 6)
	for _, item := range response.Items {
		resp, err := s.doServerRequest(server, http.MethodGet, "/emby/Items/"+url.PathEscape(item.ID)+"/Images/Primary", url.Values{"maxWidth": {"600"}}, nil, "")
		if err != nil {
			continue
		}
		img, _, decodeErr := image.Decode(resp.Body)
		resp.Body.Close()
		if decodeErr == nil {
			images = append(images, img)
		}
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 1200, 1800))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{20, 28, 45, 255}}, image.Point{}, draw.Src)
	for i := 0; i < 6; i++ {
		x := (i % 2) * 600
		y := (i / 2) * 600
		target := image.Rect(x, y, x+600, y+600)
		if i < len(images) {
			drawCover(canvas, target, images[i])
		} else {
			shade := uint8(45 + i*12)
			draw.Draw(canvas, target, &image.Uniform{C: color.RGBA{shade, 70, 100, 255}}, image.Point{}, draw.Src)
		}
	}
	draw.Draw(canvas, image.Rect(0, 1540, 1200, 1800), &image.Uniform{C: color.RGBA{5, 10, 20, 210}}, image.Point{}, draw.Over)
	d := &font.Drawer{Dst: canvas, Src: image.White, Face: loadCoverFont(), Dot: fixed.P(54, 1670)}
	d.DrawString(strings.TrimSpace(title))
	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: 90}); err != nil {
		return nil, "", err
	}
	return output.Bytes(), "image/jpeg", nil
}

func loadCoverFont() font.Face {
	paths := []string{`C:\Windows\Fonts\msyh.ttc`, "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 48, DPI: 72, Hinting: font.HintingFull})
		if err == nil {
			return face
		}
	}
	return basicfont.Face7x13
}

func drawCover(dst draw.Image, target image.Rectangle, src image.Image) {
	b := src.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return
	}
	for y := target.Min.Y; y < target.Max.Y; y++ {
		for x := target.Min.X; x < target.Max.X; x++ {
			sx := b.Min.X + (x-target.Min.X)*b.Dx()/target.Dx()
			sy := b.Min.Y + (y-target.Min.Y)*b.Dy()/target.Dy()
			dst.Set(x, y, src.At(sx, sy))
		}
	}
}

func (s *EmbyManagementService) generateAIImage(libraryName, description string) ([]byte, string, error) {
	config := s.GetAIConfig()
	secret, _ := s.configs.GetByKey("emby_cover_ai_api_key")
	if secret == nil || strings.TrimSpace(secret.ConfigVal) == "" {
		return nil, "", fmt.Errorf("AI 图片 API Key 未配置")
	}
	prompt := fmt.Sprintf("为名为《%s》的媒体库制作一张电影海报风格的方形封面，无品牌标识，构图清晰。%s", libraryName, description)
	body, _ := json.Marshal(map[string]interface{}{"model": config["model"], "prompt": prompt, "size": "1024x1024", "n": 1})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(config["base_url"], "/")+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+secret.ConfigVal)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("调用 AI 图片接口失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("AI 图片接口返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var result struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Data) == 0 {
		return nil, "", fmt.Errorf("AI 图片接口未返回图片")
	}
	if result.Data[0].B64JSON != "" {
		data, decodeErr := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
		return data, http.DetectContentType(data), decodeErr
	}
	if result.Data[0].URL == "" {
		return nil, "", fmt.Errorf("AI 图片接口未返回可用图片")
	}
	imageResp, err := s.httpClient.Get(result.Data[0].URL)
	if err != nil {
		return nil, "", err
	}
	defer imageResp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(imageResp.Body, 20<<20))
	return data, http.DetectContentType(data), err
}

func (s *EmbyManagementService) uploadImage(server *domain.EmbyServer, libraryID, contentType string, data []byte) error {
	resp, err := s.doServerRequest(server, http.MethodPost, "/emby/Items/"+url.PathEscape(libraryID)+"/Images/Primary", nil, bytes.NewReader(data), contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s *EmbyManagementService) savePreview(taskID string, data []byte) (string, error) {
	if err := os.MkdirAll(s.previewDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(s.previewDir, taskID+".image")
	return path, os.WriteFile(path, data, 0o600)
}

func (s *EmbyManagementService) runShortTask(taskType domain.TaskType, taskName string, metadata map[string]interface{}, action func() error) (string, error) {
	taskID := "emby-" + uuid.NewString()
	if err := s.tasks.Create(taskID, string(taskType), taskName); err != nil {
		return "", err
	}
	_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusRunning)
	metadata["current_step"] = "正在执行"
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	if err := action(); err != nil {
		metadata["conclusion"] = "执行失败"
		_ = s.tasks.UpdateMetadata(taskID, metadata)
		_ = s.tasks.SetError(taskID, err.Error())
		return taskID, err
	}
	metadata["current_step"] = "执行完成"
	metadata["conclusion"] = "执行成功"
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	_ = s.tasks.UpdateProgressPercent(taskID, 100)
	_ = s.tasks.UpdateStatus(taskID, domain.TaskStatusSuccess)
	return taskID, nil
}

func (s *EmbyManagementService) failEmbyTask(taskID string, metadata map[string]interface{}, step int, err error) {
	metadata["conclusion"] = "执行失败"
	metadata["current_step"] = err.Error()
	steps, _ := metadata["steps"].([]map[string]interface{})
	if step >= 0 && step < len(steps) {
		steps[step]["status"] = "failed"
		steps[step]["message"] = err.Error()
	}
	_ = s.tasks.UpdateMetadata(taskID, metadata)
	_ = s.tasks.SetError(taskID, err.Error())
}

func (s *EmbyManagementService) requireServer(id int) (*domain.EmbyServer, error) {
	server, err := s.servers.GetByID(id)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, fmt.Errorf("Emby 实例不存在")
	}
	if !server.Enabled {
		return nil, fmt.Errorf("Emby 实例已停用")
	}
	return server, nil
}

func (s *EmbyManagementService) requestJSON(server *domain.EmbyServer, method, path string, query url.Values, body interface{}, output interface{}) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	resp, err := s.doServerRequest(server, method, path, query, reader, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if output == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err = json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("解析 Emby 响应失败: %w", err)
	}
	return nil
}

func (s *EmbyManagementService) doServerRequest(server *domain.EmbyServer, method, path string, query url.Values, body io.Reader, contentType string) (*http.Response, error) {
	endpoint := strings.TrimRight(server.BaseURL, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Emby-Token", server.APIKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 Emby 失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("Emby 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return resp, nil
}

func taskMetadata(server *domain.EmbyServer, action, target string) map[string]interface{} {
	return map[string]interface{}{"server_id": server.ID, "server_name": server.Name, "operation": action, "target": target, "origin": embyTaskOriginManual}
}

func buildEmbySteps(names ...string) []map[string]interface{} {
	steps := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		steps = append(steps, map[string]interface{}{"name": name, "status": "pending", "message": ""})
	}
	return steps
}

func updateEmbyTaskStep(tasks *TaskService, taskID string, metadata map[string]interface{}, index int, status, message string, progress int) {
	steps, _ := metadata["steps"].([]map[string]interface{})
	if index >= 0 && index < len(steps) {
		steps[index]["status"] = status
		steps[index]["message"] = message
	}
	metadata["current_step"] = message
	_ = tasks.UpdateMetadata(taskID, metadata)
	_ = tasks.UpdateProgressPercent(taskID, progress)
}

func numberToInt(value interface{}) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := strconv.Atoi(v.String())
		return n
	}
	return 0
}

func nullableString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "********" + value[len(value)-4:]
}
func isSupportedImage(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/webp"
}
