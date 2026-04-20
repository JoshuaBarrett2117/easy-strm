// EmbyService Emby 媒体服务器集成服务
// 负责与 Emby REST API 交互：连接检查、库列表、库刷新
package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// SystemConfigReader 系统配置读取接口
type SystemConfigReader interface {
	GetByKey(key string) (*domain.SystemConfig, error)
}

// EmbyService Emby 集成服务
type EmbyService struct {
	systemConfigDAO SystemConfigReader
	httpClient     *http.Client
}

// NewEmbyService 创建 Emby 服务实例
func NewEmbyService(systemConfigDAO SystemConfigReader, httpClient *http.Client) *EmbyService {
	return &EmbyService{
		systemConfigDAO: systemConfigDAO,
		httpClient:      httpClient,
	}
}

// --- Emby API 响应模型 ---

// EmbySystemInfo Emby 系统信息
type EmbySystemInfo struct {
	ID          string `json:"Id"`
	ServerName  string `json:"ServerName"`
	Version     string `json:"Version"`
	OperatingSystem string `json:"OperatingSystem"`
}

// EmbyVirtualFolderInfo Emby 虚拟文件夹（媒体库）信息
type EmbyVirtualFolderInfo struct {
	Name             string `json:"Name"`
	ItemID           string `json:"ItemId"`
	CollectionType   string `json:"CollectionType"`
	Path             string `json:"Path"`
	RefreshProgress  float64 `json:"RefreshProgress"`
	RefreshStatus    string `json:"RefreshStatus"`
}

// EmbyLibraryRefreshResult Emby 库刷新结果
type EmbyLibraryRefreshResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LibraryID string `json:"library_id,omitempty"`
	LibraryName string `json:"library_name,omitempty"`
}

// --- 配置读取 ---

// getEmbyConfig 从系统配置中读取 Emby 连接参数
// Returns: (embyURL, apiKey, enabled, error)
func (s *EmbyService) getEmbyConfig() (string, string, bool, error) {
	embyURL, err := s.getConfigValue("emby_url")
	if err != nil {
		return "", "", false, fmt.Errorf("读取Emby URL失败: %v", err)
	}

	apiKey, err := s.getConfigValue("emby_api_key")
	if err != nil {
		return "", "", false, fmt.Errorf("读取Emby API Key失败: %v", err)
	}

	enabledStr, err := s.getConfigValue("emby_enabled")
	if err != nil {
		return embyURL, apiKey, false, nil
	}

	enabled := enabledStr == "true" || enabledStr == "1"
	return embyURL, apiKey, enabled, nil
}

func (s *EmbyService) getConfigValue(key string) (string, error) {
	config, err := s.systemConfigDAO.GetByKey(key)
	if err != nil {
		return "", err
	}
	if config == nil {
		return "", nil
	}
	return config.ConfigVal, nil
}

// --- 公开方法 ---

// CheckConnection 检查 Emby 服务器连接状态
// Returns: (connected bool, info *EmbySystemInfo, error)
func (s *EmbyService) CheckConnection() (bool, *EmbySystemInfo, error) {
	embyURL, apiKey, _, err := s.getEmbyConfig()
	if err != nil {
		return false, nil, err
	}
	if embyURL == "" {
		return false, nil, fmt.Errorf("Emby URL 未配置")
	}

	// 调用 Emby System/Info API
	url := fmt.Sprintf("%s/emby/System/Info?api_key=%s", strings.TrimRight(embyURL, "/"), apiKey)
	resp, err := s.doRequest("GET", url, nil)
	if err != nil {
		logger.Warnf("EmbyService[CheckConnection] 连接失败: %v", err)
		return false, nil, fmt.Errorf("连接Emby服务器失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, nil, fmt.Errorf("Emby返回非200状态码: %d, body: %s", resp.StatusCode, string(body))
	}

	var info EmbySystemInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false, nil, fmt.Errorf("解析Emby响应失败: %v", err)
	}

	logger.Infof("EmbyService[CheckConnection] 连接成功: %s (%s)", info.ServerName, info.Version)
	return true, &info, nil
}

// IsEnabled 检查 Emby 集成是否启用
func (s *EmbyService) IsEnabled() bool {
	_, _, enabled, err := s.getEmbyConfig()
	if err != nil {
		return false
	}
	return enabled
}

// ListLibraries 获取 Emby 媒体库列表
func (s *EmbyService) ListLibraries() ([]EmbyVirtualFolderInfo, error) {
	embyURL, apiKey, _, err := s.getEmbyConfig()
	if err != nil {
		return nil, err
	}
	if embyURL == "" {
		return nil, fmt.Errorf("Emby URL 未配置")
	}

	url := fmt.Sprintf("%s/emby/Library/VirtualFolders?api_key=%s", strings.TrimRight(embyURL, "/"), apiKey)
	resp, err := s.doRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("获取Emby媒体库列表失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Emby返回非200状态码: %d, body: %s", resp.StatusCode, string(body))
	}

	var libraries []EmbyVirtualFolderInfo
	if err := json.NewDecoder(resp.Body).Decode(&libraries); err != nil {
		return nil, fmt.Errorf("解析Emby媒体库响应失败: %v", err)
	}

	logger.Infof("EmbyService[ListLibraries] 获取到 %d 个媒体库", len(libraries))
	return libraries, nil
}

// RefreshLibrary 刷新指定的 Emby 媒体库
// libraryID: Emby 媒体库的 ItemId
func (s *EmbyService) RefreshLibrary(libraryID string) (*EmbyLibraryRefreshResult, error) {
	embyURL, apiKey, _, err := s.getEmbyConfig()
	if err != nil {
		return nil, err
	}
	if embyURL == "" {
		return &EmbyLibraryRefreshResult{Success: false, Message: "Emby URL 未配置"}, nil
	}

	// Emby 刷新指定媒体库的 API
	url := fmt.Sprintf("%s/emby/Items/%s/Refresh?api_key=%s", strings.TrimRight(embyURL, "/"), libraryID, apiKey)
	resp, err := s.doRequest("POST", url, nil)
	if err != nil {
		errMsg := fmt.Sprintf("刷新Emby媒体库失败: %v", err)
		logger.Errorf("EmbyService[RefreshLibrary] %s", errMsg)
		return &EmbyLibraryRefreshResult{Success: false, Message: errMsg}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("Emby返回非200状态码: %d, body: %s", resp.StatusCode, string(body))
		return &EmbyLibraryRefreshResult{Success: false, Message: errMsg}, nil
	}

	// 查找媒体库名称
	libraryName := libraryID
	libraries, libErr := s.ListLibraries()
	if libErr == nil {
		for _, lib := range libraries {
			if lib.ItemID == libraryID {
				libraryName = lib.Name
				break
			}
		}
	}

	logger.Infof("EmbyService[RefreshLibrary] 刷新媒体库成功: %s (%s)", libraryName, libraryID)
	return &EmbyLibraryRefreshResult{
		Success:     true,
		Message:     "刷新请求已发送",
		LibraryID:   libraryID,
		LibraryName: libraryName,
	}, nil
}

// RefreshAll 刷新所有 Emby 媒体库
func (s *EmbyService) RefreshAll() ([]EmbyLibraryRefreshResult, error) {
	embyURL, apiKey, _, err := s.getEmbyConfig()
	if err != nil {
		return nil, err
	}
	if embyURL == "" {
		return nil, fmt.Errorf("Emby URL 未配置")
	}

	url := fmt.Sprintf("%s/emby/Library/Refresh?api_key=%s", strings.TrimRight(embyURL, "/"), apiKey)
	resp, err := s.doRequest("POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("刷新Emby所有媒体库失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Emby返回非200状态码: %d, body: %s", resp.StatusCode, string(body))
	}

	logger.Infof("EmbyService[RefreshAll] 全部媒体库刷新请求已发送")
	return []EmbyLibraryRefreshResult{
		{Success: true, Message: "全部媒体库刷新请求已发送"},
	}, nil
}

// RefreshLibraryBySourceID 根据媒体源ID刷新绑定的Emby媒体库
// 当媒体源绑定了 emby_library_id 时，自动触发该库的刷新
func (s *EmbyService) RefreshLibraryBySourceID(mediaSourceDAO *dao.MediaSourceDAO, sourceID int) *EmbyLibraryRefreshResult {
	if !s.IsEnabled() {
		return nil
	}

	source, err := mediaSourceDAO.GetByID(sourceID)
	if err != nil || source == nil {
		logger.Warnf("EmbyService[RefreshLibraryBySourceID] 获取媒体源失败: source_id=%d, error=%v", sourceID, err)
		return nil
	}

	if source.EmbyLibraryID == "" {
		logger.Debugf("EmbyService[RefreshLibraryBySourceID] 媒体源未绑定Emby库: source_id=%d", sourceID)
		return nil
	}

	result, err := s.RefreshLibrary(source.EmbyLibraryID)
	if err != nil {
		logger.Errorf("EmbyService[RefreshLibraryBySourceID] 刷新失败: source_id=%d, library_id=%s, error=%v",
			sourceID, source.EmbyLibraryID, err)
		return &EmbyLibraryRefreshResult{
			Success:     false,
			Message:     fmt.Sprintf("刷新失败: %v", err),
			LibraryID:   source.EmbyLibraryID,
		}
	}

	return result
}

// --- 内部方法 ---

// doRequest 发送 HTTP 请求到 Emby 服务器
func (s *EmbyService) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// Emby API 需要 api_key 参数，已在 URL 中携带
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	return client.Do(req)
}
