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
	if len(c.MatchRules) > 0 {
		_ = json.Unmarshal(c.MatchRules, rule)
	}
	return rule
}
