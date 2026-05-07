package domain

import (
	"encoding/json"
	"time"
)

// MediaCategory 媒体分类模型
type MediaCategory struct {
	ID         int             `json:"id"`
	Name       string          `json:"name"`
	MediaType  string          `json:"media_type"`  // movie | tv
	TargetPath string          `json:"target_path"` // 目标存储目录
	MatchRules json.RawMessage `json:"match_rules"` // 匹配规则序列化数据
	Enabled    bool            `json:"enabled"`
	CreateTime time.Time       `json:"create_time"`
	UpdateTime time.Time       `json:"update_time"`
}

// CategoryMatchRule 分类匹配规则结构体
type CategoryMatchRule struct {
	Genres    []string `json:"genres"`    // TMDB 类型名称
	GenreIDs  []int    `json:"genre_ids"` // TMDB 类型 ID
	Countries []string `json:"countries"` // TMDB 国家/地区代码
	Languages []string `json:"languages"` // TMDB 语种代码
	Years     []int    `json:"years"`     // 年份
	Keywords  []string `json:"keywords"`  // 文件名/标题关键字
	Default   bool     `json:"default"`   // 未命中其他规则时兜底
}

// GetMatchRule 解析并返回匹配规则
func (c *MediaCategory) GetMatchRule() *CategoryMatchRule {
	rule := &CategoryMatchRule{}
	if normalized := normalizeCategoryMatchRules(c.MatchRules); len(normalized) > 0 {
		_ = json.Unmarshal(normalized, rule)
	}
	return rule
}

// NormalizeMatchRules 统一兼容对象型和字符串型 match_rules，避免历史脏数据失效。
func (c *MediaCategory) NormalizeMatchRules() {
	c.MatchRules = normalizeCategoryMatchRules(c.MatchRules)
}

func normalizeCategoryMatchRules(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}

	var object map[string]interface{}
	if err := json.Unmarshal(raw, &object); err == nil {
		normalized, marshalErr := json.Marshal(object)
		if marshalErr == nil {
			return normalized
		}
		return raw
	}

	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil && encoded != "" {
		var decoded map[string]interface{}
		if decodeErr := json.Unmarshal([]byte(encoded), &decoded); decodeErr == nil {
			normalized, marshalErr := json.Marshal(decoded)
			if marshalErr == nil {
				return normalized
			}
		}
	}

	return raw
}
