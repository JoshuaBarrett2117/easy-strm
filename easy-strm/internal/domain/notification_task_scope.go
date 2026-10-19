package domain

// NotificationScopeTask 描述可选择的调度任务，TaskKey 不使用数据库自增编号。
type NotificationScopeTask struct {
	TaskKey  string `json:"task_key"`
	TaskName string `json:"task_name"`
	Handler  string `json:"handler"`
	Status   string `json:"status"`
}

// NotificationTaskScope 返回有效选择及当前全部调度任务，空切片始终编码为数组。
type NotificationTaskScope struct {
	Selected []string                `json:"selected"`
	Tasks    []NotificationScopeTask `json:"tasks"`
}
