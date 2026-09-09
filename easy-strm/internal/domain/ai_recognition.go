package domain

// AIRecognitionConfig 保存OpenAI兼容识别配置；公开响应不返回已有密钥。
type AIRecognitionConfig struct {
	Enabled        bool     `json:"enabled"`
	BaseURL        string   `json:"base_url"`
	APIKey         string   `json:"api_key"`
	HasAPIKey      bool     `json:"has_api_key"`
	ClearAPIKey    bool     `json:"clear_api_key"`
	Model          string   `json:"model"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Scenes         []string `json:"scenes"`
	Prompt         string   `json:"prompt"`
}

// AIRecognitionHint 只包含待核对的查询信息，不接受AI编造的媒体ID或海报。
type AIRecognitionHint struct {
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Year          int    `json:"year"`
	MediaType     string `json:"media_type"`
}
