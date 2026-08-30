package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	telegram "github.com/go-telegram/bot"
	telegrammodels "github.com/go-telegram/bot/models"
)

// TelegramBotStatus 描述机器人当前生命周期状态。
type TelegramBotStatus struct {
	Enabled        bool   `json:"enabled"`
	Running        bool   `json:"running"`
	BotUsername    string `json:"bot_username"`
	ChatID         string `json:"chat_id"`
	LastUpdateTime string `json:"last_update_time"`
	LastError      string `json:"last_error"`
}

// TelegramBotService 管理 Telegram 长轮询和运维命令。
type TelegramBotService struct {
	configService *NotificationConfigService
	dashboard     *DashboardService
	tasks         *TaskService
	cron          *CronService
	emby          *EmbyService
	resources     *TelegramResourceService
	httpClient    *http.Client
	retryTask     func(taskID string) error
	runCronTask   func(task *domain.CronTask)

	mu     sync.RWMutex
	bot    *telegram.Bot
	cancel context.CancelFunc
	config TelegramConfig
	status TelegramBotStatus
}

// NewTelegramBotService 创建 Telegram 机器人服务。
func NewTelegramBotService(
	configService *NotificationConfigService,
	dashboard *DashboardService,
	tasks *TaskService,
	cron *CronService,
	emby *EmbyService,
	httpClient *http.Client,
) *TelegramBotService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &TelegramBotService{
		configService: configService,
		dashboard:     dashboard,
		tasks:         tasks,
		cron:          cron,
		emby:          emby,
		httpClient:    httpClient,
	}
}

// SetRetryTask 注入具备真实执行链路的任务重试函数。
func (s *TelegramBotService) SetRetryTask(retry func(taskID string) error) {
	s.retryTask = retry
}

// SetRunCronTask 注入定时任务立即执行函数。
func (s *TelegramBotService) SetRunCronTask(run func(task *domain.CronTask)) {
	s.runCronTask = run
}

// SetResourceService 注入 Telegram 发起的115分享转存与云下载服务。
func (s *TelegramBotService) SetResourceService(resources *TelegramResourceService) {
	s.resources = resources
}

// Reload 根据持久化配置停止旧实例并启动新实例。
func (s *TelegramBotService) Reload() error {
	s.stop()
	config, view, err := s.configService.GetTelegramConfig()
	if err != nil {
		s.setError(err)
		return err
	}
	s.mu.Lock()
	s.config = config
	s.status.Enabled = view.Enabled
	s.status.ChatID = view.ChatID
	s.status.LastUpdateTime = time.Now().Format(time.RFC3339)
	s.mu.Unlock()
	if !view.Enabled {
		return nil
	}
	if err := config.Validate(); err != nil {
		s.setError(err)
		return err
	}

	instance, err := telegram.New(
		config.BotToken,
		telegram.WithHTTPClient(10*time.Second, s.httpClient),
		telegram.WithAllowedUpdates(telegram.AllowedUpdates{
			telegrammodels.AllowedUpdateMessage,
			telegrammodels.AllowedUpdateCallbackQuery,
		}),
		telegram.WithDefaultHandler(s.handleUpdate),
		telegram.WithErrorsHandler(func(err error) {
			logger.Warnf("[TelegramBotService] 长轮询异常: %v", err)
			s.setError(err)
		}),
	)
	if err != nil {
		s.setError(err)
		return fmt.Errorf("启动Telegram机器人失败: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	me, err := instance.GetMe(ctx)
	if err != nil {
		cancel()
		s.setError(err)
		return fmt.Errorf("获取Telegram机器人信息失败: %v", err)
	}
	_, _ = instance.SetMyCommands(ctx, &telegram.SetMyCommandsParams{Commands: []telegrammodels.BotCommand{
		{Command: "status", Description: "查看系统概览"},
		{Command: "tasks", Description: "查看最近任务"},
		{Command: "cron", Description: "查看定时任务"},
		{Command: "emby", Description: "管理 Emby 媒体库"},
		{Command: "help", Description: "查看机器人帮助"},
	}})

	s.mu.Lock()
	s.bot = instance
	s.cancel = cancel
	s.status.Running = true
	s.status.BotUsername = me.Username
	s.status.LastError = ""
	s.status.LastUpdateTime = time.Now().Format(time.RFC3339)
	s.mu.Unlock()
	go func(current *telegram.Bot) {
		current.Start(ctx)
		s.mu.Lock()
		if s.bot == current {
			s.status.Running = false
			s.status.LastUpdateTime = time.Now().Format(time.RFC3339)
		}
		s.mu.Unlock()
	}(instance)
	logger.Infof("[TelegramBotService] 机器人 @%s 已启动", me.Username)
	return nil
}

// Stop 停止机器人长轮询。
func (s *TelegramBotService) Stop() {
	s.stop()
}

func (s *TelegramBotService) stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = nil
	s.bot = nil
	s.status.Running = false
	s.status.LastUpdateTime = time.Now().Format(time.RFC3339)
	s.mu.Unlock()
}

// Status 返回机器人状态快照。
func (s *TelegramBotService) Status() TelegramBotStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// TestConnection 使用已保存配置验证机器人并发送测试卡片。
func (s *TelegramBotService) TestConnection(ctx context.Context) (string, error) {
	config, _, err := s.configService.GetTelegramConfig()
	if err != nil {
		return "", err
	}
	if err := config.Validate(); err != nil {
		return "", err
	}
	instance, err := telegram.New(config.BotToken, telegram.WithHTTPClient(10*time.Second, s.httpClient))
	if err != nil {
		return "", fmt.Errorf("连接Telegram失败: %v", err)
	}
	me, err := instance.GetMe(ctx)
	if err != nil {
		return "", fmt.Errorf("获取机器人信息失败: %v", err)
	}
	card := NotificationCard{
		Title:   "Easy-STRM Telegram 测试",
		Status:  "✅",
		Fields:  [][2]string{{"机器人", "@" + me.Username}, {"状态", "连接和消息发送正常"}, {"时间", time.Now().Format("2006-01-02 15:04:05")}},
		Actions: [][]NotificationAction{{{Text: "查看系统状态", Data: "status"}}},
	}
	if err := sendTelegramCard(ctx, instance, config.ChatID, card); err != nil {
		return "", err
	}
	return me.Username, nil
}

func (s *TelegramBotService) handleUpdate(ctx context.Context, instance *telegram.Bot, update *telegrammodels.Update) {
	if update.Message != nil {
		if !s.authorizedChat(update.Message.Chat.ID, update.Message.Chat.Type) {
			return
		}
		fields := strings.Fields(update.Message.Text)
		if len(fields) == 0 {
			return
		}
		command := strings.ToLower(strings.Split(fields[0], "@")[0])
		if strings.HasPrefix(command, "/") {
			s.handleCommand(ctx, instance, command)
			return
		}
		if s.resources != nil {
			go s.handleResourceMessage(instance, update.Message.Text)
			return
		}
		s.sendHelp(ctx, instance)
		return
	}
	if update.CallbackQuery != nil {
		if !s.authorizedUser(update.CallbackQuery.From.ID) {
			return
		}
		_, _ = instance.AnswerCallbackQuery(ctx, &telegram.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
		s.handleCallback(ctx, instance, update.CallbackQuery.Data)
	}
}

func (s *TelegramBotService) handleCommand(ctx context.Context, instance *telegram.Bot, command string) {
	switch command {
	case "/start", "/help":
		s.sendHelp(ctx, instance)
	case "/status":
		s.sendStatus(ctx, instance)
	case "/tasks":
		s.sendTasks(ctx, instance)
	case "/cron":
		s.sendCron(ctx, instance)
	case "/emby":
		s.sendEmby(ctx, instance)
	default:
		s.sendHelp(ctx, instance)
	}
}

func (s *TelegramBotService) handleCallback(ctx context.Context, instance *telegram.Bot, data string) {
	switch {
	case data == "status":
		s.sendStatus(ctx, instance)
	case data == "tasks":
		s.sendTasks(ctx, instance)
	case data == "cron":
		s.sendCron(ctx, instance)
	case data == "emby":
		s.sendEmby(ctx, instance)
	case data == "emby:all":
		s.refreshEmby(ctx, instance, "")
	case strings.HasPrefix(data, "emby:"):
		s.refreshEmby(ctx, instance, strings.TrimPrefix(data, "emby:"))
	case strings.HasPrefix(data, "task:detail:"):
		s.sendTaskDetail(ctx, instance, strings.TrimPrefix(data, "task:detail:"))
	case strings.HasPrefix(data, "task:cancel:"):
		s.cancelTask(ctx, instance, strings.TrimPrefix(data, "task:cancel:"))
	case strings.HasPrefix(data, "task:retry:"):
		s.retryTaskAction(ctx, instance, strings.TrimPrefix(data, "task:retry:"))
	case strings.HasPrefix(data, "cron:run:"):
		s.runCron(ctx, instance, strings.TrimPrefix(data, "cron:run:"))
	}
}

func (s *TelegramBotService) sendHelp(ctx context.Context, instance *telegram.Bot) {
	card := NotificationCard{
		Title:  "Easy-STRM 运维机器人",
		Status: "🤖",
		Detail: "可查看系统状态与任务，并直接取消运行任务、重试自动整理、运行定时任务或刷新 Emby。\n\n发送115分享链接可自动转存，发送 magnet、ed2k、HTTP、HTTPS 或 FTP 链接可创建115云下载。链接后可追加“账号名称 /目标目录”，也可只追加其中一项。",
		Actions: [][]NotificationAction{
			{{Text: "系统状态", Data: "status"}, {Text: "最近任务", Data: "tasks"}},
			{{Text: "定时任务", Data: "cron"}, {Text: "Emby", Data: "emby"}},
		},
	}
	s.send(ctx, instance, card)
}

func (s *TelegramBotService) handleResourceMessage(instance *telegram.Bot, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	card, handled, err := s.resources.Execute(ctx, text)
	if !handled {
		s.sendHelp(ctx, instance)
		return
	}
	if err != nil {
		s.sendError(ctx, instance, "115资源操作失败", err)
		return
	}
	s.send(ctx, instance, card)
}

func (s *TelegramBotService) sendStatus(ctx context.Context, instance *telegram.Bot) {
	stats, err := s.dashboard.GetDashboardStats()
	if err != nil {
		s.sendError(ctx, instance, "读取系统状态失败", err)
		return
	}
	monitor, _ := s.dashboard.GetDashboardResourceMonitor()
	fields := [][2]string{
		{"115 账号", fmt.Sprintf("%d/%d 活跃", stats.Accounts.Active, stats.Accounts.Total)},
		{"媒体源", fmt.Sprintf("%d/%d 启用", stats.MediaSources.Enabled, stats.MediaSources.Total)},
		{"任务", fmt.Sprintf("运行 %d · 今日成功 %d · 失败 %d", stats.Tasks.Running, stats.Tasks.CompletedToday, stats.Tasks.FailedToday)},
		{"STRM 文件", strconv.Itoa(stats.StrmFiles.Total)},
	}
	if monitor != nil {
		fields = append(fields, [2]string{"运行资源", fmt.Sprintf("Goroutine %d · 内存 %.1f MB", monitor.Goroutines, float64(monitor.MemoryBytes)/1024/1024)})
	}
	s.send(ctx, instance, NotificationCard{Title: "系统概览", Status: "📊", Fields: fields, Actions: [][]NotificationAction{{{Text: "刷新", Data: "status"}, {Text: "最近任务", Data: "tasks"}}}})
}

func (s *TelegramBotService) sendTasks(ctx context.Context, instance *telegram.Bot) {
	tasks, err := s.tasks.GetUnified()
	if err != nil {
		s.sendError(ctx, instance, "读取任务失败", err)
		return
	}
	if len(tasks) == 0 {
		s.send(ctx, instance, NotificationCard{Title: "最近任务", Status: "📭", Detail: "暂无任务记录"})
		return
	}
	limit := len(tasks)
	if limit > 5 {
		limit = 5
	}
	for _, task := range tasks[:limit] {
		s.send(ctx, instance, buildTaskCard(task, false))
	}
}

func (s *TelegramBotService) sendTaskDetail(ctx context.Context, instance *telegram.Bot, taskID string) {
	task, err := s.tasks.Get(taskID)
	if err != nil || task == nil {
		if err == nil {
			err = fmt.Errorf("任务不存在")
		}
		s.sendError(ctx, instance, "读取任务详情失败", err)
		return
	}
	s.send(ctx, instance, buildTaskCard(task, true))
}

func (s *TelegramBotService) cancelTask(ctx context.Context, instance *telegram.Bot, taskID string) {
	if err := s.tasks.Cancel(taskID); err != nil {
		s.sendError(ctx, instance, "取消任务失败", err)
		return
	}
	s.send(ctx, instance, NotificationCard{Title: "任务已取消", Status: "⏹", Fields: [][2]string{{"任务 ID", taskID}}, Actions: [][]NotificationAction{{{Text: "最近任务", Data: "tasks"}}}})
}

func (s *TelegramBotService) retryTaskAction(ctx context.Context, instance *telegram.Bot, taskID string) {
	if s.retryTask == nil {
		s.sendError(ctx, instance, "重试任务失败", fmt.Errorf("重试执行器未初始化"))
		return
	}
	task, err := s.tasks.Get(taskID)
	if err != nil || task == nil || fmt.Sprint(task["task_type"]) != "watch_auto_organize" {
		s.sendError(ctx, instance, "重试任务失败", fmt.Errorf("该任务不支持重试"))
		return
	}
	if err := s.retryTask(taskID); err != nil {
		s.sendError(ctx, instance, "重试任务失败", err)
		return
	}
	s.send(ctx, instance, NotificationCard{Title: "自动整理已重新执行", Status: "🔄", Fields: [][2]string{{"任务 ID", taskID}}, Actions: [][]NotificationAction{{{Text: "查看任务", Data: "task:detail:" + taskID}}}})
}

func (s *TelegramBotService) sendCron(ctx context.Context, instance *telegram.Bot) {
	tasks, err := s.cron.GetAll()
	if err != nil {
		s.sendError(ctx, instance, "读取定时任务失败", err)
		return
	}
	if len(tasks) == 0 {
		s.send(ctx, instance, NotificationCard{Title: "定时任务", Status: "📭", Detail: "暂无定时任务"})
		return
	}
	limit := len(tasks)
	if limit > 8 {
		limit = 8
	}
	for _, task := range tasks[:limit] {
		card := NotificationCard{
			Title:   task.TaskName,
			Status:  "⏰",
			Fields:  [][2]string{{"状态", task.Status}, {"Cron", task.CronExpr}, {"上次结果", task.LastRunStatus}},
			Actions: [][]NotificationAction{{{Text: "立即运行", Data: fmt.Sprintf("cron:run:%d", task.ID)}}},
		}
		s.send(ctx, instance, card)
	}
}

func (s *TelegramBotService) runCron(ctx context.Context, instance *telegram.Bot, rawID string) {
	id, err := strconv.Atoi(rawID)
	if err != nil {
		s.sendError(ctx, instance, "运行定时任务失败", fmt.Errorf("任务 ID 无效"))
		return
	}
	task, err := s.cron.GetByID(id)
	if err != nil || task == nil || s.runCronTask == nil {
		if err == nil {
			err = fmt.Errorf("定时任务不存在或执行器未初始化")
		}
		s.sendError(ctx, instance, "运行定时任务失败", err)
		return
	}
	go s.runCronTask(task)
	s.send(ctx, instance, NotificationCard{Title: "定时任务已触发", Status: "▶️", Fields: [][2]string{{"任务", task.TaskName}}})
}

func (s *TelegramBotService) sendEmby(ctx context.Context, instance *telegram.Bot) {
	connected, info, err := s.emby.CheckConnection()
	if err != nil || !connected {
		if err == nil {
			err = fmt.Errorf("Emby 未连接")
		}
		s.sendError(ctx, instance, "Emby 状态异常", err)
		return
	}
	fields := [][2]string{{"服务器", info.ServerName}, {"版本", info.Version}}
	actions := [][]NotificationAction{{{Text: "刷新全部媒体库", Data: "emby:all"}}}
	libraries, listErr := s.emby.ListLibraries()
	if listErr == nil {
		for _, library := range libraries {
			data := "emby:" + library.ItemID
			if len(data) <= 64 {
				actions = append(actions, []NotificationAction{{Text: "刷新 " + truncateRunes(library.Name, 24), Data: data}})
			}
		}
	}
	s.send(ctx, instance, NotificationCard{Title: "Emby 媒体库", Status: "🎞", Fields: fields, Actions: actions})
}

func (s *TelegramBotService) refreshEmby(ctx context.Context, instance *telegram.Bot, libraryID string) {
	if libraryID == "" {
		results, err := s.emby.RefreshAll()
		if err != nil {
			s.sendError(ctx, instance, "刷新 Emby 失败", err)
			return
		}
		s.send(ctx, instance, NotificationCard{Title: "Emby 刷新已提交", Status: "✅", Fields: [][2]string{{"媒体库数量", strconv.Itoa(len(results))}}})
		return
	}
	result, err := s.emby.RefreshLibrary(libraryID)
	if err != nil || result == nil || !result.Success {
		if err == nil {
			err = fmt.Errorf("%s", result.Message)
		}
		s.sendError(ctx, instance, "刷新 Emby 失败", err)
		return
	}
	s.send(ctx, instance, NotificationCard{Title: "Emby 刷新已提交", Status: "✅", Fields: [][2]string{{"媒体库", result.LibraryName}}})
}

func (s *TelegramBotService) authorizedChat(chatID int64, chatType telegrammodels.ChatType) bool {
	if chatType != telegrammodels.ChatTypePrivate {
		return false
	}
	return s.authorizedUser(chatID)
}

func (s *TelegramBotService) authorizedUser(userID int64) bool {
	s.mu.RLock()
	configured := s.config.ChatID
	s.mu.RUnlock()
	chatID, err := ParseTelegramChatID(configured)
	return err == nil && chatID == userID
}

func (s *TelegramBotService) send(ctx context.Context, instance *telegram.Bot, card NotificationCard) {
	s.mu.RLock()
	chatID := s.config.ChatID
	s.mu.RUnlock()
	if err := sendTelegramCard(ctx, instance, chatID, card); err != nil {
		logger.Warnf("[TelegramBotService] 发送响应失败: %v", err)
		s.setError(err)
	}
}

func (s *TelegramBotService) sendError(ctx context.Context, instance *telegram.Bot, title string, err error) {
	s.send(ctx, instance, NotificationCard{Title: title, Status: "❌", Detail: err.Error()})
}

func (s *TelegramBotService) setError(err error) {
	s.mu.Lock()
	s.status.LastError = truncateRunes(err.Error(), 500)
	s.status.LastUpdateTime = time.Now().Format(time.RFC3339)
	s.mu.Unlock()
}

func sendTelegramCard(ctx context.Context, instance *telegram.Bot, chatID string, card NotificationCard) error {
	params := &telegram.SendMessageParams{ChatID: chatID, Text: card.TelegramText(), ParseMode: telegrammodels.ParseModeHTML}
	if markup := card.TelegramMarkup(); markup != nil {
		params.ReplyMarkup = markup
	}
	if _, err := instance.SendMessage(ctx, params); err != nil {
		return fmt.Errorf("发送Telegram消息失败: %v", err)
	}
	return nil
}

func buildTaskCard(task map[string]interface{}, detailed bool) NotificationCard {
	status := fmt.Sprint(task["status"])
	statusIcon := map[string]string{"completed": "✅", "failed": "❌", "partial_failed": "⚠️", "cancelled": "⏹", "running": "🔄", "pending": "⏳"}[status]
	if statusIcon == "" {
		statusIcon = "ℹ️"
	}
	taskID := fmt.Sprint(task["task_id"])
	taskType := fmt.Sprint(task["task_type"])
	fields := [][2]string{
		{"类型", notificationTaskTypeName(taskType)},
		{"状态", status},
		{"进度", fmt.Sprintf("%v%% · 成功 %v · 失败 %v", task["progress"], task["success_files"], task["failed_files"])},
	}
	if duration := taskDurationText(fmt.Sprint(task["create_time"]), fmt.Sprint(task["update_time"])); duration != "" {
		fields = append(fields, [2]string{"耗时", duration})
	}
	if detailed {
		fields = append(fields, [2]string{"任务 ID", taskID}, [2]string{"更新时间", fmt.Sprint(task["update_time"])})
	}
	actions := [][]NotificationAction{{{Text: "详情", Data: "task:detail:" + taskID}}}
	if (status == "pending" || status == "running") && len("task:cancel:"+taskID) <= 64 {
		actions = append(actions, []NotificationAction{{Text: "取消任务", Data: "task:cancel:" + taskID}})
	}
	if taskType == "watch_auto_organize" && (status == "failed" || status == "cancelled") && len("task:retry:"+taskID) <= 64 {
		actions = append(actions, []NotificationAction{{Text: "重试自动整理", Data: "task:retry:" + taskID}})
	}
	return NotificationCard{
		Title:   fmt.Sprint(task["task_name"]),
		Status:  statusIcon,
		Fields:  fields,
		Detail:  fmt.Sprint(task["error_message"]),
		Actions: actions,
	}
}

func taskDurationText(createTime, updateTime string) string {
	const layout = "2006-01-02 15:04:05"
	created, createErr := time.ParseInLocation(layout, createTime, time.Local)
	updated, updateErr := time.ParseInLocation(layout, updateTime, time.Local)
	if createErr != nil || updateErr != nil || updated.Before(created) {
		return ""
	}
	duration := updated.Sub(created)
	if duration < time.Minute {
		return fmt.Sprintf("%d 秒", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%d 分 %d 秒", int(duration.Minutes()), int(duration.Seconds())%60)
	}
	return fmt.Sprintf("%d 小时 %d 分", int(duration.Hours()), int(duration.Minutes())%60)
}
