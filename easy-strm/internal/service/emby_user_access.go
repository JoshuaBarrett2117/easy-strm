package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"easy-strm/internal/domain"
)

// ListUserLibraries 查询用户权限专用选项，不能将 VirtualFolders.ItemId 当作权限 Guid。
func (s *EmbyManagementService) ListUserLibraries(serverID int) ([]domain.EmbyUserLibrary, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	return s.listUserLibraries(server)
}

func (s *EmbyManagementService) listUserLibraries(server *domain.EmbyServer) ([]domain.EmbyUserLibrary, error) {
	var folders []struct {
		ID           string `json:"Id"`
		Guid         string `json:"Guid"`
		Name         string `json:"Name"`
		Configurable bool   `json:"IsUserAccessConfigurable"`
	}
	if err := s.requestJSON(server, http.MethodGet, "/emby/Library/SelectableMediaFolders", nil, nil, &folders); err != nil {
		return nil, err
	}
	result := make([]domain.EmbyUserLibrary, 0, len(folders))
	for _, folder := range folders {
		if !folder.Configurable {
			continue
		}
		if strings.TrimSpace(folder.Guid) == "" {
			return nil, fmt.Errorf("媒体库 %s 缺少权限 Guid", folder.Name)
		}
		result = append(result, domain.EmbyUserLibrary{ID: folder.Guid, ItemID: folder.ID, Name: folder.Name})
	}
	return result, nil
}

func (s *EmbyManagementService) normalizeUserPolicy(server *domain.EmbyServer, policy domain.EmbyUserPolicy) (domain.EmbyUserPolicy, error) {
	ids := policy.EnabledFolders
	policy.EnabledFolders = []string{}
	if policy.EnableAllFolders || len(ids) == 0 {
		return policy, nil
	}
	folders, err := s.listUserLibraries(server)
	if err != nil {
		return policy, err
	}
	aliases := make(map[string]string)
	for _, folder := range folders {
		aliases[folder.ID] = folder.ID
		if folder.ItemID != "" {
			aliases[folder.ItemID] = folder.ID
		}
	}
	seen := make(map[string]bool)
	for _, id := range ids {
		guid, ok := aliases[strings.TrimSpace(id)]
		if !ok {
			return policy, fmt.Errorf("媒体库权限标识 %q 无效，请刷新媒体库选项后重试", id)
		}
		if !seen[guid] {
			policy.EnabledFolders = append(policy.EnabledFolders, guid)
			seen[guid] = true
		}
	}
	return policy, nil
}

func (s *EmbyManagementService) saveUserPolicy(server *domain.EmbyServer, userID string, original map[string]interface{}, policy domain.EmbyUserPolicy) error {
	// Emby 更新策略接收完整对象，保留页面未编辑的直播、设备及子目录等设置。
	merged, _ := original["Policy"].(map[string]interface{})
	if merged == nil {
		return fmt.Errorf("读取 Emby 用户原有权限失败")
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &merged); err != nil {
		return err
	}
	path := "/emby/Users/" + url.PathEscape(userID)
	if err = s.requestJSON(server, http.MethodPost, path+"/Policy", nil, merged, nil); err != nil {
		return err
	}
	var saved domain.EmbyUser
	if err = s.requestJSON(server, http.MethodGet, path, nil, nil, &saved); err != nil {
		return fmt.Errorf("权限已提交但回读失败: %w", err)
	}
	if saved.Policy.EnableAllFolders != policy.EnableAllFolders {
		return fmt.Errorf("Emby 回读的全部媒体库开关与提交值不一致")
	}
	if !policy.EnableAllFolders {
		actual := make(map[string]bool)
		for _, id := range saved.Policy.EnabledFolders {
			actual[id] = true
		}
		if len(actual) != len(policy.EnabledFolders) {
			return fmt.Errorf("Emby 未应用所选媒体库权限，请重新加载后重试")
		}
		for _, id := range policy.EnabledFolders {
			if !actual[id] {
				return fmt.Errorf("Emby 未应用所选媒体库权限，请重新加载后重试")
			}
		}
	}
	return nil
}
