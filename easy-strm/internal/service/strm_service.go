package service

import (
	"fmt"
	"path/filepath"
	"strings"

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

// FindMatchingConfigByCloud115ID 根据媒体源关联的 115 账号 ID 查找匹配的 STRM 配置
// 匹配逻辑：优先匹配 net_disk_path 为目标路径前缀的配置，否则返回该账号下的第一个配置
// Args:
//   - cloud115ID: 115 账号 ID
//   - targetPath: 整理后的目标路径（如 /已整理/电影/xxx）
// Returns:
//   - *domain.StrmConfig: 匹配到的 STRM 配置，未找到返回 nil
//   - error: 查询错误
func (s *StrmService) FindMatchingConfigByCloud115ID(cloud115ID int, targetPath string) (*domain.StrmConfig, error) {
	configs, err := s.strmConfigDAO.GetAll("id", "asc")
	if err != nil {
		return nil, fmt.Errorf("查询STRM配置列表失败: %v", err)
	}

	// 筛选同一 115 账号下的配置
	var matched []*domain.StrmConfig
	for _, cfg := range configs {
		if cfg.Cloud115Id == cloud115ID {
			matched = append(matched, cfg)
		}
	}

	if len(matched) == 0 {
		return nil, nil
	}

	// 优先匹配 net_disk_path 为 targetPath 前缀的配置（路径最精确的优先）
	var bestMatch *domain.StrmConfig
	bestLen := 0
	for _, cfg := range matched {
		if strings.HasPrefix(targetPath, cfg.NetDiskPath) {
			if len(cfg.NetDiskPath) > bestLen {
				bestMatch = cfg
				bestLen = len(cfg.NetDiskPath)
			}
		}
	}

	if bestMatch != nil {
		return bestMatch, nil
	}

	// 无前缀匹配时返回该账号下第一个配置
	return matched[0], nil
}

// UpdateNetDiskPath 更新 STRM 配置的网盘路径
// 整理完成后自动将目标目录写入 STRM 配置的 net_disk_path
func (s *StrmService) UpdateNetDiskPath(configID int, netDiskPath string) error {
	cfg, err := s.strmConfigDAO.GetByID(configID)
	if err != nil {
		return fmt.Errorf("获取STRM配置失败: %v", err)
	}
	if cfg == nil {
		return fmt.Errorf("STRM配置不存在: ID %d", configID)
	}

	_, err = s.strmConfigDAO.UpdateExt(
		cfg.ID, cfg.Cloud115Id, netDiskPath, cfg.LocalPath,
		cfg.Cron, cfg.Extension, cfg.SyncMode, cfg.SourceAccount,
		cfg.TargetAccount, cfg.TargetDirectory, cfg.AutoCleanup,
		cfg.CleanupThreshold, cfg.CleanupPolicy, cfg.MaxConcurrency,
	)
	if err != nil {
		return fmt.Errorf("更新STRM配置网盘路径失败: %v", err)
	}

	logger.Infof("StrmService[UpdateNetDiskPath] 已更新配置 ID %d 的网盘路径为: %s", configID, netDiskPath)
	return nil
}
