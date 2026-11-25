package service

import (
	"fmt"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type Cloud115Service struct {
	cloud115DAO *dao.Cloud115DAO
}

func NewCloud115Service(cloud115DAO *dao.Cloud115DAO) *Cloud115Service {
	return &Cloud115Service{
		cloud115DAO: cloud115DAO,
	}
}

// GetByID 根据ID获取115云账号
func (s *Cloud115Service) GetByID(id int) (*domain.Cloud115, error) {
	return s.cloud115DAO.GetByID(id)
}

// GetAll 获取所有115云账号
func (s *Cloud115Service) GetAll(sortField, sortOrder string) ([]*domain.Cloud115, error) {
	return s.cloud115DAO.GetAll(sortField, sortOrder)
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
			logger.Infof("Cloud115Service[CheckAndRecoverCoolingAccounts] 恢复账号 ID: %d - 无冷却开始时间", account.ID)
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
			logger.Infof("Cloud115Service[CheckAndRecoverCoolingAccounts] 恢复账号 ID: %d - 冷却时间 %.0f 分钟", account.ID, coolingElapsed.Minutes())
		}
	}

	if recoveredCount > 0 {
		logger.Infof("Cloud115Service[CheckAndRecoverCoolingAccounts] 本次共恢复 %d 个账号", recoveredCount)
	}

	return nil
}
