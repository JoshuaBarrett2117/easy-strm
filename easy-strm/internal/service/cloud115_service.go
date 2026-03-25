package service

import (
	"fmt"

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

// Create 创建115云账号
func (s *Cloud115Service) Create(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string) (*domain.Cloud115, error) {
	if name == "" {
		name = "115账号"
	}
	cloud115, err := s.cloud115DAO.Create(name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory)
	if err != nil {
		logger.Errorf("Cloud115Service[Create] 创建账号失败: %v", err)
		return nil, fmt.Errorf("创建账号失败: %v", err)
	}
	logger.Infof("Cloud115Service[Create] 创建账号成功: %s (ID: %d)", name, cloud115.ID)
	return cloud115, nil
}

// Update 更新115云账号
func (s *Cloud115Service) Update(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string) (*domain.Cloud115, error) {
	cloud115, err := s.cloud115DAO.Update(id, name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory)
	if err != nil {
		logger.Errorf("Cloud115Service[Update] 更新账号失败: %v", err)
		return nil, fmt.Errorf("更新账号失败: %v", err)
	}
	logger.Infof("Cloud115Service[Update] 更新账号成功: %s (ID: %d)", name, id)
	return cloud115, nil
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
