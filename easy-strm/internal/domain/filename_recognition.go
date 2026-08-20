package domain

// FilenameRecognitionRule 描述一条有序的文件名识别规则。
// Pattern 使用 Go 正则，并针对去除扩展名、统一分隔符后的文件名匹配。
type FilenameRecognitionRule struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Pattern       string `json:"pattern"`
	MediaType     string `json:"media_type"`
	DefaultSeason int    `json:"default_season"`
	Example       string `json:"example"`
	Enabled       bool   `json:"enabled"`
	Priority      int    `json:"priority"`
}

// FilenameRecognitionRuleSet 是规则配置接口的稳定响应结构。
type FilenameRecognitionRuleSet struct {
	Rules          []FilenameRecognitionRule `json:"rules"`
	ConfigKey      string                    `json:"config_key"`
	InputTransform string                    `json:"input_transform"`
}
