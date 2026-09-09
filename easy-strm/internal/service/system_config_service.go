package service

import (
	"fmt"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

// SystemConfigService 系统配置服务。
// 负责配置读取、批量更新与日志保留天数配置解析。
type SystemConfigService struct {
	systemConfigDAO *dao.SystemConfigDAO
}

// NewSystemConfigService 创建系统配置服务实例。
func NewSystemConfigService(systemConfigDAO *dao.SystemConfigDAO) *SystemConfigService {
	return &SystemConfigService{systemConfigDAO: systemConfigDAO}
}

// GetAll 获取所有系统配置。
func (s *SystemConfigService) GetAll() ([]*domain.SystemConfig, error) {
	configs, err := s.systemConfigDAO.GetAll()
	if err != nil {
		return nil, err
	}
	filtered := make([]*domain.SystemConfig, 0, len(configs))
	for _, config := range configs {
		if config != nil && !isRemovedSystemConfigKey(config.ConfigKey) && config.ConfigKey != "metatube_token" {
			filtered = append(filtered, config)
		}
	}
	return filtered, nil
}

// GetByKey 按 Key 获取系统配置。
func (s *SystemConfigService) GetByKey(key string) (*domain.SystemConfig, error) {
	if isRemovedSystemConfigKey(key) || key == "metatube_token" {
		return nil, nil
	}
	return s.systemConfigDAO.GetByKey(key)
}

// Upsert 更新或创建单个系统配置。
func (s *SystemConfigService) Upsert(key, value string) error {
	if isRemovedSystemConfigKey(key) {
		return fmt.Errorf("系统配置 %s 已停用", key)
	}
	return s.systemConfigDAO.Upsert(key, value)
}

// BatchUpsert 批量更新系统配置，返回成功写入的配置。
func (s *SystemConfigService) BatchUpsert(configs map[string]string) map[string]string {
	updated := make(map[string]string)
	for key, value := range configs {
		if isRemovedSystemConfigKey(key) {
			continue
		}
		if err := s.systemConfigDAO.Upsert(key, value); err == nil && key != "metatube_token" {
			updated[key] = value
		}
	}
	return updated
}

func isRemovedSystemConfigKey(key string) bool {
	// 全局 API 密钥只能通过专用接口访问，避免通用设置接口泄露明文。
	return key == AIRecognitionConfigKey || key == "alist_url" || key == "alist_token" || key == GlobalAPIKeyKey || key == GlobalAPIEnabledKey || key == GlobalAPIBaseURLKey
}

// GetLogSaveDayLimit 获取日志保留天数，配置不存在时返回默认值 1。
func (s *SystemConfigService) GetLogSaveDayLimit() (string, int, error) {
	config, err := s.systemConfigDAO.GetByKey("log_save_day_limit")
	if err != nil || config == nil {
		return "log_save_day_limit", 1, err
	}

	var days int
	fmt.Sscanf(config.ConfigVal, "%d", &days)
	return config.ConfigKey, days, nil
}

// UpdateLogSaveDayLimit 更新日志保留天数。
func (s *SystemConfigService) UpdateLogSaveDayLimit(days int) error {
	return s.systemConfigDAO.Upsert("log_save_day_limit", fmt.Sprintf("%d", days))
}
