package domain

// TaskTypeInfo 描述任务中心可见的一种任务类型。
type TaskTypeInfo struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Group string `json:"group"`
}

// 任务分组（前端筛选下拉按组展示）。
const (
	TaskGroupStrm     = "STRM"
	TaskGroupOrganize = "媒体整理"
	TaskGroupShare    = "115 与分享"
	TaskGroupEmby     = "Emby"
	TaskGroupSystem   = "系统"
)

// TaskTypeCatalog 是所有会写入任务中心的任务类型的唯一中文名映射（顺序即展示顺序）。
// 新增任务类型时在此登记；前端通过 GET /tasks/types 读取，通知文案也从这里取名。
var TaskTypeCatalog = []TaskTypeInfo{
	{"strm_generate", "STRM 生成", TaskGroupStrm},
	{"incremental_sync", "STRM 增量同步", TaskGroupStrm},
	{"share_baseline_adopt", "分享 STRM 基线采纳", TaskGroupStrm},
	{"organize", "媒体整理", TaskGroupOrganize},
	{"watch_auto_organize", "自动整理", TaskGroupOrganize},
	{"share_transfer", "115 分享转存", TaskGroupShare},
	{"share_transfer_organize", "转存后整理", TaskGroupShare},
	{"share_transfer_scrape", "转存后刮削", TaskGroupShare},
	{"share_sync", "分享同步", TaskGroupShare},
	{"share_identify", "分享媒体识别", TaskGroupShare},
	{"share_delete", "删除分享", TaskGroupShare},
	{"share_clear", "清空分享文件记录", TaskGroupShare},
	{"share_media_delete", "删除分享文件记录", TaskGroupShare},
	{"offline_download", "115 云下载", TaskGroupShare},
	{"file_transfer", "文件传输", TaskGroupShare},
	{"emby_refresh", "Emby 库刷新", TaskGroupEmby},
	{"emby_server", "Emby 实例管理", TaskGroupEmby},
	{"emby_user", "Emby 用户管理", TaskGroupEmby},
	{"emby_library", "Emby 媒体库管理", TaskGroupEmby},
	{"emby_cover", "Emby 媒体库封面", TaskGroupEmby},
	{"emby_plugin", "神医助手任务", TaskGroupEmby},
	{"emby_scheduled", "Emby 定时任务触发", TaskGroupEmby},
	{"cleanup", "定时清理", TaskGroupSystem},
}

var taskTypeLabels = func() map[string]string {
	labels := make(map[string]string, len(TaskTypeCatalog))
	for _, item := range TaskTypeCatalog {
		labels[item.Value] = item.Label
	}
	return labels
}()

// TaskTypeLabel 返回任务类型中文名；未登记的类型原样返回。
func TaskTypeLabel(taskType string) string {
	if label := taskTypeLabels[taskType]; label != "" {
		return label
	}
	return taskType
}
