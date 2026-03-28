package service

import (
	"fmt"
	"path/filepath"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type StrmService struct {
	strmConfigDAO *dao.StrmConfigDAO
	strmFileDAO   *dao.StrmFileDAO
	cronTaskDAO   *dao.CronTaskDAO
	scheduler     interface {
		AddTask(task *domain.CronTask) error
		RemoveTask(taskID int)
		UpdateTask(task *domain.CronTask) error
	}
}

func NewStrmService(strmConfigDAO *dao.StrmConfigDAO, strmFileDAO *dao.StrmFileDAO, cronTaskDAO *dao.CronTaskDAO) *StrmService {
	return &StrmService{
		strmConfigDAO: strmConfigDAO,
		strmFileDAO:   strmFileDAO,
		cronTaskDAO:   cronTaskDAO,
	}
}

func (s *StrmService) SetScheduler(scheduler interface {
	AddTask(task *domain.CronTask) error
	RemoveTask(taskID int)
	UpdateTask(task *domain.CronTask) error
}) {
	s.scheduler = scheduler
}

// GetConfigByID 根据ID获取STRM配置
func (s *StrmService) GetConfigByID(id int) (*domain.StrmConfig, error) {
	return s.strmConfigDAO.GetByID(id)
}

// GetAllConfig 获取所有STRM配置
func (s *StrmService) GetAllConfig(sortField, sortOrder string) ([]*domain.StrmConfig, error) {
	return s.strmConfigDAO.GetAll(sortField, sortOrder)
}

// CreateConfig 创建STRM配置
func (s *StrmService) CreateConfig(cloud115Id int, netDiskPath, localPath, cron, extension string) (*domain.StrmConfig, error) {
	cfg, err := s.strmConfigDAO.Create(cloud115Id, netDiskPath, localPath, cron, extension)
	if err != nil {
		logger.Errorf("StrmService[CreateConfig] 创建配置失败: %v", err)
		return nil, fmt.Errorf("创建配置失败: %v", err)
	}
	logger.Infof("StrmService[CreateConfig] 创建配置成功: ID %d", cfg.ID)

	if cron != "" {
		taskName := fmt.Sprintf("STRM全量生成-%s", filepath.Base(netDiskPath))
		cronTask, err := s.cronTaskDAO.Create(taskName, "full_generate", cloud115Id, cfg.ID, cron)
		if err != nil {
			logger.Warnf("StrmService[CreateConfig] 创建定时任务失败: %v", err)
		} else if s.scheduler != nil {
			if err := s.scheduler.AddTask(cronTask); err != nil {
				logger.Warnf("StrmService[CreateConfig] 添加定时任务到调度器失败: %v", err)
			}
		}
	}

	return cfg, nil
}

// UpdateConfig 更新STRM配置
func (s *StrmService) UpdateConfig(id, cloud115Id int, netDiskPath, localPath, cron, extension string) (*domain.StrmConfig, error) {
	cfg, err := s.strmConfigDAO.Update(id, cloud115Id, netDiskPath, localPath, cron, extension)
	if err != nil {
		logger.Errorf("StrmService[UpdateConfig] 更新配置失败: %v", err)
		return nil, fmt.Errorf("更新配置失败: %v", err)
	}
	logger.Infof("StrmService[UpdateConfig] 更新配置成功: ID %d", cfg.ID)

	existingTask, _ := s.cronTaskDAO.GetByStrmConfigID(cfg.ID)

	if cron != "" {
		taskName := fmt.Sprintf("STRM全量生成-%s", filepath.Base(netDiskPath))
		if existingTask != nil {
			_, err = s.cronTaskDAO.Update(existingTask.ID, taskName, "full_generate", cron, existingTask.Status)
			if err != nil {
				logger.Warnf("StrmService[UpdateConfig] 更新定时任务失败: %v", err)
			} else if s.scheduler != nil {
				updatedTask, _ := s.cronTaskDAO.GetByID(existingTask.ID)
				if updatedTask != nil {
					if err := s.scheduler.UpdateTask(updatedTask); err != nil {
						logger.Warnf("StrmService[UpdateConfig] 更新调度器任务失败: %v", err)
					}
				}
			}
		} else {
			cronTask, err := s.cronTaskDAO.Create(taskName, "full_generate", cloud115Id, cfg.ID, cron)
			if err != nil {
				logger.Warnf("StrmService[UpdateConfig] 创建定时任务失败: %v", err)
			} else if s.scheduler != nil {
				if err := s.scheduler.AddTask(cronTask); err != nil {
					logger.Warnf("StrmService[UpdateConfig] 添加定时任务到调度器失败: %v", err)
				}
			}
		}
	} else {
		if existingTask != nil {
			if s.scheduler != nil {
				s.scheduler.RemoveTask(existingTask.ID)
			}
			if err := s.cronTaskDAO.DeleteByName(existingTask.TaskName); err != nil {
				logger.Warnf("StrmService[UpdateConfig] 删除定时任务失败: %v", err)
			}
		}
	}

	return cfg, nil
}

// CreateConfigExt 创建STRM配置（扩展版，包含秒传同步字段）
func (s *StrmService) CreateConfigExt(cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*domain.StrmConfig, error) {
	cfg, err := s.strmConfigDAO.CreateExt(cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency)
	if err != nil {
		logger.Errorf("StrmService[CreateConfigExt] 创建配置失败: %v", err)
		return nil, fmt.Errorf("创建配置失败: %v", err)
	}
	logger.Infof("StrmService[CreateConfigExt] 创建配置成功: ID %d", cfg.ID)

	if cron != "" {
		taskName := fmt.Sprintf("STRM全量生成-%s", filepath.Base(netDiskPath))
		cronTask, err := s.cronTaskDAO.Create(taskName, "full_generate", cloud115Id, cfg.ID, cron)
		if err != nil {
			logger.Warnf("StrmService[CreateConfigExt] 创建定时任务失败: %v", err)
		} else if s.scheduler != nil {
			if err := s.scheduler.AddTask(cronTask); err != nil {
				logger.Warnf("StrmService[CreateConfigExt] 添加定时任务到调度器失败: %v", err)
			}
		}
	}

	return cfg, nil
}

// UpdateConfigExt 更新STRM配置（扩展版，包含秒传同步字段）
func (s *StrmService) UpdateConfigExt(id, cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*domain.StrmConfig, error) {
	cfg, err := s.strmConfigDAO.UpdateExt(id, cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency)
	if err != nil {
		logger.Errorf("StrmService[UpdateConfigExt] 更新配置失败: %v", err)
		return nil, fmt.Errorf("更新配置失败: %v", err)
	}
	logger.Infof("StrmService[UpdateConfigExt] 更新配置成功: ID %d", cfg.ID)

	existingTask, _ := s.cronTaskDAO.GetByStrmConfigID(cfg.ID)

	if cron != "" {
		taskName := fmt.Sprintf("STRM全量生成-%s", filepath.Base(netDiskPath))
		if existingTask != nil {
			_, err = s.cronTaskDAO.Update(existingTask.ID, taskName, "full_generate", cron, existingTask.Status)
			if err != nil {
				logger.Warnf("StrmService[UpdateConfigExt] 更新定时任务失败: %v", err)
			} else if s.scheduler != nil {
				updatedTask, _ := s.cronTaskDAO.GetByID(existingTask.ID)
				if updatedTask != nil {
					if err := s.scheduler.UpdateTask(updatedTask); err != nil {
						logger.Warnf("StrmService[UpdateConfigExt] 更新调度器任务失败: %v", err)
					}
				}
			}
		} else {
			cronTask, err := s.cronTaskDAO.Create(taskName, "full_generate", cloud115Id, cfg.ID, cron)
			if err != nil {
				logger.Warnf("StrmService[UpdateConfigExt] 创建定时任务失败: %v", err)
			} else if s.scheduler != nil {
				if err := s.scheduler.AddTask(cronTask); err != nil {
					logger.Warnf("StrmService[UpdateConfigExt] 添加定时任务到调度器失败: %v", err)
				}
			}
		}
	} else {
		if existingTask != nil {
			if s.scheduler != nil {
				s.scheduler.RemoveTask(existingTask.ID)
			}
			if err := s.cronTaskDAO.DeleteByName(existingTask.TaskName); err != nil {
				logger.Warnf("StrmService[UpdateConfigExt] 删除定时任务失败: %v", err)
			}
		}
	}

	return cfg, nil
}

// DeleteConfig 删除STRM配置
func (s *StrmService) DeleteConfig(id int) error {
	existingTask, _ := s.cronTaskDAO.GetByStrmConfigID(id)
	if existingTask != nil {
		if s.scheduler != nil {
			s.scheduler.RemoveTask(existingTask.ID)
		}
		if err := s.cronTaskDAO.DeleteByName(existingTask.TaskName); err != nil {
			logger.Warnf("StrmService[DeleteConfig] 删除定时任务失败: %v", err)
		}
	}

	if err := s.strmConfigDAO.Delete(id); err != nil {
		logger.Errorf("StrmService[DeleteConfig] 删除配置失败: %v", err)
		return fmt.Errorf("删除配置失败: %v", err)
	}
	logger.Infof("StrmService[DeleteConfig] 删除配置成功: ID %d", id)
	return nil
}

// GetFilesByConfigID 根据配置ID获取STRM文件记录
func (s *StrmService) GetFilesByConfigID(strmConfigID int) ([]*domain.StrmFile, error) {
	return s.strmFileDAO.GetByConfigID(strmConfigID)
}

// UpsertFile 创建或更新STRM文件记录
func (s *StrmService) UpsertFile(strmConfigID int, fileName, filePath, pickCode, sha1 string, fileSize int64, localStrmPath string) (*domain.StrmFile, error) {
	return s.strmFileDAO.Upsert(strmConfigID, fileName, filePath, pickCode, sha1, fileSize, localStrmPath)
}

// CountFilesByConfigID 统计指定配置的STRM文件数量
func (s *StrmService) CountFilesByConfigID(strmConfigID int) (int, error) {
	return s.strmFileDAO.CountByConfigID(strmConfigID)
}
