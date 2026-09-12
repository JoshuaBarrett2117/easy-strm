package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"errors"
	"fmt"
	"github.com/robfig/cron/v3"
	"sort"
	"strings"
	"sync"
	"time"
)

// CronParameter 描述处理器表单参数。
type CronParameter struct {
	Type    string `json:"type,omitempty"`
	Key     string `json:"key"`
	Label   string `json:"label"`
	Default int    `json:"default"`
}

// CronHandler 将稳定标识绑定到实际代码方法。
type CronHandler struct {
	Key        string                                                          `json:"key"`
	Name       string                                                          `json:"name"`
	Parameters []CronParameter                                                 `json:"parameters"`
	Execute    func(context.Context, *domain.CronTask, string) (string, error) `json:"-"`
}

// CronService 统一配置、调度和执行状态。
type CronService struct {
	cronTaskDAO *dao.CronTaskDAO
	engine      *cron.Cron
	mu          sync.Mutex
	entries     map[int]cron.EntryID
	running     map[string]string
	handlers    map[string]CronHandler
	tasks       *TaskService
}

// NewCronService 创建未启动的调度服务。
func NewCronService(d *dao.CronTaskDAO) *CronService {
	return &CronService{cronTaskDAO: d, engine: cron.New(), entries: map[int]cron.EntryID{}, running: map[string]string{}, handlers: map[string]CronHandler{}}
}

// SetTaskService 注入任务中心。
func (s *CronService) SetTaskService(t *TaskService) { s.tasks = t }

// Register 在启动前注册处理器。
func (s *CronService) Register(h CronHandler) { s.handlers[h.Key] = h }

// Handlers 返回稳定顺序的配置目录。
func (s *CronService) Handlers() []CronHandler {
	out := make([]CronHandler, 0, len(s.handlers))
	for _, h := range s.handlers {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ParseCron 接受五段或六段表达式，按配置时区解释。
func ParseCron(expr, zone string) (cron.Schedule, error) {
	if zone == "" {
		zone = "Local"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("时区无效: %w", err)
	}
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		expr = "0 " + expr
	} else if len(fields) != 6 {
		return nil, fmt.Errorf("Cron需要五段或六段")
	}
	schedule, err := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("Cron无效: %w", err)
	}
	schedule.(*cron.SpecSchedule).Location = loc
	return schedule, nil
}

// CronInt 读取已校验的整数参数。
func CronInt(p map[string]interface{}, key string) int {
	switch n := p[key].(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// ValidateDefinition 在写库前验证完整配置。
func (s *CronService) ValidateDefinition(t *domain.CronTask) error {
	if strings.TrimSpace(t.TaskName) == "" {
		return fmt.Errorf("请输入任务名称")
	}
	h, ok := s.handlers[t.Handler]
	if !ok {
		return fmt.Errorf("处理器未注册")
	}
	if t.Timezone == "" {
		t.Timezone = "Local"
	}
	if t.Status == "" {
		t.Status = "enabled"
	}
	if t.Status != "enabled" && t.Status != "disabled" {
		return fmt.Errorf("任务状态无效")
	}
	if _, err := ParseCron(t.CronExpr, t.Timezone); err != nil {
		return err
	}
	if t.Params == nil {
		t.Params = map[string]interface{}{}
	}
	allowed := map[string]bool{}
	for _, p := range h.Parameters {
		allowed[p.Key] = true
		if p.Type == "boolean" {
			if v, ok := t.Params[p.Key]; ok {
				if _, valid := v.(bool); !valid {
					return fmt.Errorf("%s必须为布尔值", p.Label)
				}
			} else {
				t.Params[p.Key] = false
			}
			continue
		}
		raw, exists := t.Params[p.Key]
		if !exists {
			raw = float64(p.Default)
			t.Params[p.Key] = raw
		}
		var n float64
		switch v := raw.(type) {
		case float64:
			n = v
		case int:
			n = float64(v)
		default:
			return fmt.Errorf("%s必须是正整数", p.Label)
		}
		if n < 1 || n > 2147483647 || n != float64(int(n)) {
			return fmt.Errorf("%s必须是正整数", p.Label)
		}
	}
	for key := range t.Params {
		if !allowed[key] {
			return fmt.Errorf("未知处理器参数: %s", key)
		}
	}
	t.Cloud115ID = 0
	t.StrmConfigID = 0
	if t.Handler == "full_generate" || t.Handler == "incremental_sync" {
		t.Cloud115ID = CronInt(t.Params, "cloud115_id")
		t.StrmConfigID = CronInt(t.Params, "strm_config_id")
		if err := s.cronTaskDAO.ValidateStrm(t.StrmConfigID, t.Cloud115ID); err != nil {
			return err
		}
	}
	t.TaskType = t.Handler
	return nil
}

// Save 保存配置并立即替换调度。
func (s *CronService) Save(t *domain.CronTask) (*domain.CronTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.ID > 0 {
		old, err := s.GetByID(t.ID)
		if err != nil {
			return nil, err
		}
		if old == nil {
			return nil, fmt.Errorf("任务不存在")
		}
		t.TaskKey = old.TaskKey
		t.Builtin = old.Builtin
		t.LastRunTime = old.LastRunTime
		t.LastRunStatus = old.LastRunStatus
		t.LastRunMessage = old.LastRunMessage
		if old.Builtin && old.Handler != t.Handler {
			return nil, fmt.Errorf("内置任务不能更换处理器")
		}
	}
	if err := s.ValidateDefinition(t); err != nil {
		return nil, err
	}
	if t.TaskKey == "" {
		t.TaskKey = fmt.Sprintf("task:%d", time.Now().UnixNano())
	}
	if err := s.cronTaskDAO.SaveDefinition(t); err != nil {
		return nil, err
	}
	if err := s.replaceLocked(t); err != nil {
		return nil, err
	}
	return t, nil
}
func (s *CronService) replaceLocked(t *domain.CronTask) error {
	schedule, err := ParseCron(t.CronExpr, t.Timezone)
	if err != nil {
		return err
	}
	if _, ok := s.handlers[t.Handler]; !ok {
		return fmt.Errorf("处理器未注册: %s", t.Handler)
	}
	if entry, ok := s.entries[t.ID]; ok {
		s.engine.Remove(entry)
		delete(s.entries, t.ID)
	}
	t.NextRunTime = nil
	if t.Status == "enabled" {
		id := t.ID
		s.entries[id] = s.engine.Schedule(schedule, cron.FuncJob(func() {
			if _, err := s.Run(id, "scheduled"); err != nil {
				logger.Errorf("任务%d启动失败: %v", id, err)
			}
		}))
		next := schedule.Next(time.Now())
		t.NextRunTime = &next
	}
	return s.cronTaskDAO.UpdateNextRun(t.ID, t.NextRunTime)
}

// AddTask 同步STRM旧入口写入的任务。
func (s *CronService) AddTask(t *domain.CronTask) error {
	if err := s.cronTaskDAO.Hydrate(t); err != nil {
		return err
	}
	if t.Handler == "" {
		t.Handler = t.TaskType
		t.Params = map[string]interface{}{"cloud115_id": t.Cloud115ID, "strm_config_id": t.StrmConfigID}
		_, err := s.Save(t)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceLocked(t)
}

// UpdateTask 重新装载任务配置。
func (s *CronService) UpdateTask(t *domain.CronTask) error { return s.AddTask(t) }

// RemoveTask 移除后续调度，不终止当前执行。
func (s *CronService) RemoveTask(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(id)
}

// removeLocked 移除调度；调用方必须持有 mu。
func (s *CronService) removeLocked(id int) {
	if entry, ok := s.entries[id]; ok {
		s.engine.Remove(entry)
		delete(s.entries, id)
	}
}

// reloadConfigLocked 重新装载某个 STRM 配置关联的任务；调用方必须持有 mu。
func (s *CronService) reloadConfigLocked(configID int) error {
	ids, err := s.cronTaskDAO.GetByConfig(configID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		t, err := s.GetByID(id)
		if err != nil {
			return err
		}
		if err := s.replaceLocked(t); err != nil {
			return err
		}
	}
	return nil
}

// GetNextRunTime 获取下次执行时间。
func (s *CronService) GetNextRunTime(id int) *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entries[id]; ok {
		n := s.engine.Entry(entry).Next
		if !n.IsZero() {
			return &n
		}
	}
	return nil
}

// LoadTasksFromDB 装载配置并启动，不补跑停机期间的周期。
func (s *CronService) LoadTasksFromDB() error {
	if err := s.cronTaskDAO.RecoverRuns(); err != nil {
		return err
	}
	list, err := s.GetAll()
	if err != nil {
		return err
	}
	var loadErrors []error
	for _, t := range list {
		if err = s.AddTask(t); err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("任务%d装载失败: %w", t.ID, err))
			if e := s.cronTaskDAO.UpdateRunInfo(t.ID, t.LastRunTime, nil, "failed", err.Error()); e != nil {
				loadErrors = append(loadErrors, e)
			}
		}
	}
	s.engine.Start()
	return errors.Join(loadErrors...)
}

// Stop 停止后续调度。
func (s *CronService) Stop() { s.engine.Stop() }

// AcquireStrmExecution 让配置页直接生成、任务恢复和定时处理器共用配置互斥。
// 同一执行ID由调度服务预先占用时不重复释放，由外层执行器负责收尾。
func (s *CronService) AcquireStrmExecution(configID int, taskID string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("strm:%d", configID)
	if owner := s.running[key]; owner != "" {
		if owner == taskID {
			return func() {}, nil
		}
		return nil, fmt.Errorf("该STRM配置正在执行，请等待当前任务结束")
	}
	s.running[key] = taskID
	return func() {
		s.mu.Lock()
		if s.running[key] == taskID {
			delete(s.running, key)
		}
		s.mu.Unlock()
	}, nil
}

// Run 统一手动和定时触发，返回任务中心执行ID。
func (s *CronService) Run(id int, trigger string) (string, error) {
	s.mu.Lock()
	t, err := s.GetByID(id)
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	if t == nil {
		s.mu.Unlock()
		return "", fmt.Errorf("任务不存在")
	}
	if trigger == "scheduled" && t.Status != "enabled" {
		s.mu.Unlock()
		return "", nil
	}
	h, ok := s.handlers[t.Handler]
	if !ok || h.Execute == nil {
		s.mu.Unlock()
		return "", fmt.Errorf("处理器不可用")
	}
	if s.tasks == nil {
		s.mu.Unlock()
		return "", fmt.Errorf("任务中心未初始化")
	}
	taskID := fmt.Sprintf("cron_%d_%d", id, time.Now().UnixNano())
	keys := []string{fmt.Sprintf("task:%d", id)}
	if t.StrmConfigID > 0 {
		keys = append(keys, fmt.Sprintf("strm:%d", t.StrmConfigID))
	}
	busy := false
	for _, key := range keys {
		busy = busy || s.running[key] != ""
	}
	if !busy {
		for _, key := range keys {
			s.running[key] = taskID
		}
	}
	s.mu.Unlock()
	release := func() {
		s.mu.Lock()
		for _, key := range keys {
			delete(s.running, key)
		}
		s.mu.Unlock()
	}
	if busy {
		if err = s.cronTaskDAO.StartRun(id, taskID, trigger, "skipped"); err != nil {
			return "", err
		}
		if err = s.cronTaskDAO.FinishRun(taskID, "skipped", "同一任务或STRM配置正在运行，跳过此次触发"); err != nil {
			return "", err
		}
		if err = s.tasks.Create(taskID, "cleanup", t.TaskName+"（跳过）"); err != nil {
			return "", err
		}
		if err = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"cron_task_id": id, "outcome": "skipped", "message": "同一任务或STRM配置正在运行"}); err != nil {
			return "", err
		}
		return taskID, s.tasks.UpdateStatus(taskID, "completed")
	}
	if err = s.cronTaskDAO.StartRun(id, taskID, trigger, "pending"); err != nil {
		release()
		return "", err
	}
	kind := "cleanup"
	if t.Handler == "full_generate" {
		kind = "strm_generate"
	} else if t.Handler == "share_strm_incremental_export" {
		kind = "strm_generate"
	} else if t.Handler == "incremental_sync" {
		kind = "incremental_sync"
	}
	if err = s.tasks.Create(taskID, kind, t.TaskName); err != nil {
		release()
		_ = s.cronTaskDAO.FinishRun(taskID, "failed", err.Error())
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.tasks.RegisterCancel(taskID, cancel)
	if e := s.tasks.UpdateMetadata(taskID, map[string]interface{}{"cron_task_id": id, "trigger_type": trigger}); e != nil {
		logger.Warnf("保存调度元数据失败: %v", e)
	}
	go func() {
		defer cancel()
		defer s.tasks.RemoveCancel(taskID)
		defer release()
		now := time.Now()
		status, message := "success", ""
		defer func() {
			if v := recover(); v != nil {
				status = "failed"
				message = fmt.Sprint(v)
			}
			if ctx.Err() != nil || s.tasks.IsCancelled(taskID) {
				status = "cancelled"
				message = "任务已取消"
			}
			if status == "failed" {
				_ = s.tasks.SetError(taskID, message)
			} else if status == "cancelled" {
				_ = s.tasks.UpdateStatus(taskID, "cancelled")
			} else {
				_ = s.tasks.UpdateStatus(taskID, "completed")
			}
			if e := s.cronTaskDAO.FinishRun(taskID, status, message); e != nil {
				logger.Errorf("保存执行记录失败: %v", e)
			}
			if e := s.cronTaskDAO.UpdateRunInfo(id, &now, s.GetNextRunTime(id), status, message); e != nil {
				logger.Errorf("保存调度状态失败: %v", e)
			}
		}()

		if ctx.Err() != nil || s.tasks.IsCancelled(taskID) {
			return
		}
		if e := s.cronTaskDAO.MarkRunStarted(taskID); e != nil {
			status = "failed"
			message = e.Error()
			return
		}
		if e := s.tasks.UpdateStatus(taskID, "running"); e != nil {
			status = "failed"
			message = e.Error()
			return
		}
		_ = s.cronTaskDAO.UpdateRunInfo(id, &now, s.GetNextRunTime(id), "running", "")
		if s.tasks.IsCancelled(taskID) {
			return
		}
		var runErr error
		message, runErr = h.Execute(ctx, t, taskID)
		if runErr != nil {
			status = "failed"
			message = runErr.Error()
		}
	}()
	return taskID, nil
}

// GetByID 返回通用配置。
func (s *CronService) GetByID(id int) (*domain.CronTask, error) {
	t, err := s.cronTaskDAO.GetByID(id)
	if err != nil || t == nil {
		return t, err
	}
	err = s.cronTaskDAO.Hydrate(t)
	return t, err
}

// GetByName 查询名称匹配的任务。
func (s *CronService) GetByName(name string) (*domain.CronTask, error) {
	t, e := s.cronTaskDAO.GetByName(name)
	if e != nil || t == nil {
		return t, e
	}
	e = s.cronTaskDAO.Hydrate(t)
	return t, e
}

// GetAll 返回任务配置列表。
func (s *CronService) GetAll() ([]*domain.CronTask, error) {
	list, err := s.cronTaskDAO.GetAll()
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		if err = s.cronTaskDAO.Hydrate(t); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// GetEnabled 返回启用的配置。
func (s *CronService) GetEnabled() ([]*domain.CronTask, error) {
	all, e := s.GetAll()
	out := []*domain.CronTask{}
	for _, t := range all {
		if t.Status == "enabled" {
			out = append(out, t)
		}
	}
	return out, e
}

// Create 为STRM旧入口建立通用定义。
func (s *CronService) Create(name, kind string, cloud, config int, expr string) (*domain.CronTask, error) {
	return s.Save(&domain.CronTask{TaskName: name, Handler: kind, Params: map[string]interface{}{"cloud115_id": cloud, "strm_config_id": config}, CronExpr: expr, Status: "enabled", Timezone: "Local"})
}

// Update 为旧入口更新同一任务。
func (s *CronService) Update(id int, name, kind, expr, status string) (*domain.CronTask, error) {
	t, e := s.GetByID(id)
	if e != nil {
		return nil, e
	}
	if t == nil {
		return nil, fmt.Errorf("任务不存在")
	}
	t.TaskName = name
	t.Handler = kind
	t.CronExpr = expr
	t.Status = status
	return s.Save(t)
}

// UpdateRunInfo 保存运行摘要。
func (s *CronService) UpdateRunInfo(id int, last, next *time.Time, status, msg string) error {
	return s.cronTaskDAO.UpdateRunInfo(id, last, next, status, msg)
}

// Delete 删除用户任务，内置任务仅允许停用。
func (s *CronService) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.GetByID(id)
	if e != nil {
		return e
	}
	if t == nil {
		return fmt.Errorf("任务不存在")
	}
	if t.Builtin {
		return fmt.Errorf("内置任务只能停用，不能删除")
	}
	if e = s.cronTaskDAO.Delete(id); e != nil {
		return e
	}
	s.removeLocked(id)
	return nil
}

// Runs 返回执行历史分页。
func (s *CronService) Runs(id, page, size int) (domain.ShareLibraryPage, error) {
	return s.cronTaskDAO.Runs(id, page, size)
}
