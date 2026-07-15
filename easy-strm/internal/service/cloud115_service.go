package service

import (
	"fmt"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type Cloud115Service struct {
	cloud115DAO            *dao.Cloud115DAO
	notificationConfigDAO  *dao.NotificationConfigDAO
	instantTransferService *InstantTransferService
}

func NewCloud115Service(cloud115DAO *dao.Cloud115DAO, notificationConfigDAO *dao.NotificationConfigDAO) *Cloud115Service {
	return &Cloud115Service{
		cloud115DAO:           cloud115DAO,
		notificationConfigDAO: notificationConfigDAO,
	}
}

// SetInstantTransferService 设置秒传服务实例
func (s *Cloud115Service) SetInstantTransferService(instantTransferService *InstantTransferService) {
	s.instantTransferService = instantTransferService
}

// GetByID 根据ID获取115云账号
func (s *Cloud115Service) GetByID(id int) (*domain.Cloud115, error) {
	return s.cloud115DAO.GetByID(id)
}

// GetAll 获取所有115云账号
func (s *Cloud115Service) GetAll(sortField, sortOrder string) ([]*domain.Cloud115, error) {
	return s.cloud115DAO.GetAll(sortField, sortOrder)
}

// Create 创建115云账号
func (s *Cloud115Service) Create(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, transferMethod string, alistUrl string, alistToken string) (*domain.Cloud115, error) {
	if name == "" {
		name = "115账号"
	}
	cloud115, err := s.cloud115DAO.Create(name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, transferMethod, alistUrl, alistToken)
	if err != nil {
		logger.Errorf("Cloud115Service[Create] 创建账号失败: %v", err)
		return nil, fmt.Errorf("创建账号失败: %v", err)
	}
	logger.Infof("Cloud115Service[Create] 创建账号成功: %s (ID: %d)", name, cloud115.ID)
	return cloud115, nil
}

// Update 更新115云账号
func (s *Cloud115Service) Update(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, status string, transferMethod string, alistUrl string, alistToken string) (*domain.Cloud115, error) {
	cloud115, err := s.cloud115DAO.Update(id, name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, status, transferMethod, alistUrl, alistToken)
	if err != nil {
		logger.Errorf("Cloud115Service[Update] 更新账号失败: %v", err)
		return nil, fmt.Errorf("更新账号失败: %v", err)
	}
	logger.Infof("Cloud115Service[Update] 更新账号成功: %s (ID: %d)", name, id)
	return cloud115, nil
}

// UpdateStatus 更新账号状态
func (s *Cloud115Service) UpdateStatus(id int, status string, coolingStartTime *time.Time) error {
	if err := s.cloud115DAO.UpdateStatus(id, status, coolingStartTime); err != nil {
		logger.Errorf("Cloud115Service[UpdateStatus] 更新状态失败: %v", err)
		return fmt.Errorf("更新账号状态失败: %v", err)
	}
	logger.Infof("Cloud115Service[UpdateStatus] 更新状态成功: ID %d, status: %s", id, status)
	return nil
}

// UpdateQuotaUsed 更新账号空间使用量
func (s *Cloud115Service) UpdateQuotaUsed(id int, quotaUsed int64) error {
	if err := s.cloud115DAO.UpdateQuotaUsed(id, quotaUsed); err != nil {
		logger.Errorf("Cloud115Service[UpdateQuotaUsed] 更新空间使用量失败: %v", err)
		return fmt.Errorf("更新空间使用量失败: %v", err)
	}
	return nil
}

// Delete 删除115云账号
func (s *Cloud115Service) Delete(id int) error {
	if err := s.cloud115DAO.Delete(id); err != nil {
		logger.Errorf("Cloud115Service[Delete] 删除账号失败: %v", err)
		return fmt.Errorf("删除账号失败: %v", err)
	}
	logger.Infof("Cloud115Service[Delete] 删除账号成功: ID %d", id)
	return nil
}

// GetAllNotificationConfig 获取所有通知配置
func (s *Cloud115Service) GetAllNotificationConfig() ([]*domain.NotificationConfig, error) {
	configs, err := s.notificationConfigDAO.GetAll()
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// GetNotificationConfigByChannel 根据渠道获取通知配置
func (s *Cloud115Service) GetNotificationConfigByChannel(channel string) (*domain.NotificationConfig, error) {
	return s.notificationConfigDAO.GetByChannel(channel)
}

// UpsertNotificationConfig 创建或更新通知配置
func (s *Cloud115Service) UpsertNotificationConfig(channel, configJSON string, enabled bool) (*domain.NotificationConfig, error) {
	return s.notificationConfigDAO.Upsert(channel, configJSON, enabled)
}

// DeleteNotificationConfig 删除通知配置
func (s *Cloud115Service) DeleteNotificationConfig(channel string) error {
	return s.notificationConfigDAO.Delete(channel)
}

// InstantTransfer 秒传文件
// sourceFile: 源文件信息（SHA1必须）
// sourceAccountID: 源账号ID
// targetAccountID: 目标账号ID
// targetDirectory: 目标目录（可选）
func (s *Cloud115Service) InstantTransfer(sourceFile *domain.FileInfo, sourceAccountID, targetAccountID int, targetDirectory string) (*TransferResult, error) {
	// 获取源账号
	sourceAccount, err := s.cloud115DAO.GetByID(sourceAccountID)
	if err != nil || sourceAccount == nil {
		logger.Errorf("Cloud115Service[InstantTransfer] 获取源账号失败: %v", err)
		return nil, fmt.Errorf("源账号不存在")
	}

	// 获取目标账号
	targetAccount, err := s.cloud115DAO.GetByID(targetAccountID)
	if err != nil || targetAccount == nil {
		logger.Errorf("Cloud115Service[InstantTransfer] 获取目标账号失败: %v", err)
		return nil, fmt.Errorf("目标账号不存在")
	}

	// 调用秒传服务
	if s.instantTransferService == nil {
		logger.Errorf("Cloud115Service[InstantTransfer] 秒传服务未初始化")
		return nil, fmt.Errorf("秒传服务未初始化")
	}

	result := s.instantTransferService.Transfer(sourceFile, sourceAccount, targetAccount, targetDirectory)
	logger.Infof("Cloud115Service[InstantTransfer] 秒传结果: success=%v, skip=%v, sha1=%s, message=%s",
		result.Success, result.Skip, result.SHA1, result.Message)

	return result, nil
}

// GetTransferCache 查询SHA1缓存
func (s *Cloud115Service) GetTransferCache(sha1 string) (bool, string, error) {
	if s.instantTransferService == nil {
		logger.Errorf("Cloud115Service[GetTransferCache] 秒传服务未初始化")
		return false, "", fmt.Errorf("秒传服务未初始化")
	}

	cachedCID, err := s.instantTransferService.GetCachedSHA1(sha1)
	if err != nil {
		logger.Errorf("Cloud115Service[GetTransferCache] 查询缓存失败: %v", err)
		return false, "", err
	}

	cached := cachedCID != ""
	logger.Infof("Cloud115Service[GetTransferCache] SHA1=%s, cached=%v, cid=%s", sha1, cached, cachedCID)

	return cached, cachedCID, nil
}

// CheckAndRecoverCoolingAccounts 检查并恢复超过冷却时间的账号
// 冷却时间阈值：5分钟
func (s *Cloud115Service) CheckAndRecoverCoolingAccounts() error {
	const coolingDuration = 5 * time.Minute

	// 获取所有cooling状态的账号
	coolingAccounts, err := s.cloud115DAO.GetByStatus(domain.AccountStatusCooling)
	if err != nil {
		logger.Errorf("Cloud115Service[CheckAndRecoverCoolingAccounts] 查询冷却账号失败: %v", err)
		return fmt.Errorf("查询冷却账号失败: %v", err)
	}

	if len(coolingAccounts) == 0 {
		logger.Debugf("Cloud115Service[CheckAndRecoverCoolingAccounts] 没有冷却中的账号")
		return nil
	}

	now := time.Now()
	recoveredCount := 0

	for _, account := range coolingAccounts {
		if account.CoolingStartTime == nil {
			// 如果没有冷却开始时间，视为已超过冷却时间，直接恢复
			if err := s.cloud115DAO.UpdateStatus(account.ID, domain.AccountStatusActive, nil); err != nil {
				logger.Errorf("Cloud115Service[CheckAndRecoverCoolingAccounts] 恢复账号 %d 失败: %v", account.ID, err)
				continue
			}
			recoveredCount++
			logger.Infof("Cloud115Service[CheckAndRecoverCoolingAccounts] 恢复账号 %s (ID: %d) - 无冷却开始时间", account.Name, account.ID)
			continue
		}

		// 检查冷却时间是否已超过5分钟
		coolingElapsed := now.Sub(*account.CoolingStartTime)
		if coolingElapsed >= coolingDuration {
			if err := s.cloud115DAO.UpdateStatus(account.ID, domain.AccountStatusActive, nil); err != nil {
				logger.Errorf("Cloud115Service[CheckAndRecoverCoolingAccounts] 恢复账号 %d 失败: %v", account.ID, err)
				continue
			}
			recoveredCount++
			logger.Infof("Cloud115Service[CheckAndRecoverCoolingAccounts] 恢复账号 %s (ID: %d) - 冷却时间 %.0f 分钟", account.Name, account.ID, coolingElapsed.Minutes())
		}
	}

	if recoveredCount > 0 {
		logger.Infof("Cloud115Service[CheckAndRecoverCoolingAccounts] 本次共恢复 %d 个账号", recoveredCount)
	}

	return nil
}
