package service

import (
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type CronService struct {
	cronTaskDAO *dao.CronTaskDAO
	scheduler   interface {
		AddTask(task *domain.CronTask) error
		RemoveTask(taskID int)
		UpdateTask(task *domain.CronTask) error
		GetNextRunTime(taskID int) *time.Time
	}
	cloud115Service *Cloud115Service
}

func NewCronService(cronTaskDAO *dao.CronTaskDAO) *CronService {
	return &CronService{
		cronTaskDAO: cronTaskDAO,
	}
}

func (s *CronService) SetScheduler(scheduler interface {
	AddTask(task *domain.CronTask) error
	RemoveTask(taskID int)
	UpdateTask(task *domain.CronTask) error
	GetNextRunTime(taskID int) *time.Time
}) {
	s.scheduler = scheduler
}

// SetCloud115Service 设置Cloud115Service实例，用于账号冷却恢复
func (s *CronService) SetCloud115Service(cloud115Service *Cloud115Service) {
	s.cloud115Service = cloud115Service
}

// RecoverCoolingAccounts 检查并恢复超过冷却时间的账号
// 由定时任务调用，每分钟执行一次
func (s *CronService) RecoverCoolingAccounts() {
	if s.cloud115Service == nil {
		logger.Errorf("CronService[RecoverCoolingAccounts] Cloud115Service未初始化，无法恢复冷却账号")
		return
	}

	logger.Infof("CronService[RecoverCoolingAccounts] 开始检查冷却账号...")
	if err := s.cloud115Service.CheckAndRecoverCoolingAccounts(); err != nil {
		logger.Errorf("CronService[RecoverCoolingAccounts] 恢复冷却账号失败: %v", err)
	} else {
		logger.Infof("CronService[RecoverCoolingAccounts] 冷却账号检查完成")
	}
}

// GetByID 根据ID获取定时任务
func (s *CronService) GetByID(id int) (*domain.CronTask, error) {
	return s.cronTaskDAO.GetByID(id)
}

// GetByName 根据任务名获取定时任务
func (s *CronService) GetByName(taskName string) (*domain.CronTask, error) {
	return s.cronTaskDAO.GetByName(taskName)
}

// GetAll 获取所有定时任务
func (s *CronService) GetAll() ([]*domain.CronTask, error) {
	return s.cronTaskDAO.GetAll()
}

// GetEnabled 获取所有启用的定时任务
func (s *CronService) GetEnabled() ([]*domain.CronTask, error) {
	return s.cronTaskDAO.GetEnabled()
}

// Create 创建定时任务
func (s *CronService) Create(taskName, taskType string, cloud115ID, strmConfigID int, cronExpr string) (*domain.CronTask, error) {
	task, err := s.cronTaskDAO.Create(taskName, taskType, cloud115ID, strmConfigID, cronExpr)
	if err != nil {
		logger.Errorf("CronService[Create] 创建定时任务失败: %v", err)
		return nil, err
	}
	logger.Infof("CronService[Create] 创建定时任务成功: %s (ID: %d)", taskName, task.ID)

	if s.scheduler != nil {
		if err := s.scheduler.AddTask(task); err != nil {
			logger.Warnf("CronService[Create] 添加到调度器失败: %v", err)
		}
	}

	return task, nil
}

// Update 更新定时任务
func (s *CronService) Update(id int, taskName, taskType, cronExpr, status string) (*domain.CronTask, error) {
	task, err := s.cronTaskDAO.Update(id, taskName, taskType, cronExpr, status)
	if err != nil {
		logger.Errorf("CronService[Update] 更新定时任务失败: %v", err)
		return nil, err
	}

	if s.scheduler != nil {
		if err := s.scheduler.UpdateTask(task); err != nil {
			logger.Warnf("CronService[Update] 更新调度器失败: %v", err)
		}
	}

	return task, nil
}

// UpdateRunInfo 更新定时任务执行信息
func (s *CronService) UpdateRunInfo(id int, lastRunTime, nextRunTime *time.Time, lastRunStatus, lastRunMessage string) error {
	if err := s.cronTaskDAO.UpdateRunInfo(id, lastRunTime, nextRunTime, lastRunStatus, lastRunMessage); err != nil {
		logger.Errorf("CronService[UpdateRunInfo] 更新执行信息失败: %v", err)
		return err
	}

	if s.scheduler != nil && nextRunTime != nil {
		if updatedTask, err := s.cronTaskDAO.GetByID(id); err == nil && updatedTask != nil {
			updatedTask.NextRunTime = s.scheduler.GetNextRunTime(id)
			if err := s.cronTaskDAO.UpdateRunInfo(id, lastRunTime, updatedTask.NextRunTime, lastRunStatus, lastRunMessage); err != nil {
				logger.Warnf("CronService[UpdateRunInfo] 更新下次执行时间失败: %v", err)
			}
		}
	}

	return nil
}

// Delete 删除定时任务
func (s *CronService) Delete(id int) error {
	if s.scheduler != nil {
		s.scheduler.RemoveTask(id)
	}

	if err := s.cronTaskDAO.Delete(id); err != nil {
		logger.Errorf("CronService[Delete] 删除定时任务失败: %v", err)
		return err
	}
	logger.Infof("CronService[Delete] 删除定时任务成功: ID %d", id)
	return nil
}

// LoadTasksFromDB 从数据库加载定时任务到调度器
func (s *CronService) LoadTasksFromDB() error {
	tasks, err := s.cronTaskDAO.GetEnabled()
	if err != nil {
		logger.Errorf("CronService[LoadTasksFromDB] 获取启用的定时任务失败: %v", err)
		return err
	}

	if s.scheduler != nil {
		for _, task := range tasks {
			if err := s.scheduler.AddTask(task); err != nil {
				logger.Warnf("CronService[LoadTasksFromDB] 添加任务到调度器失败: %v", err)
			} else {
				logger.Infof("CronService[LoadTasksFromDB] 加载定时任务: %s (ID: %d)", task.TaskName, task.ID)
			}
		}
	}

	return nil
}
