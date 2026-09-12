package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
)

const filenameRecognitionRulesConfigKey = "filename_recognition_rules"

const legacySxePattern = `(?i)^(?P<title>.+?)\s+S(?P<season>\d{1,2})E(?P<episode>\d{1,3})(?:\s+.*)?$`

var filenameEpisodeToken = regexp.MustCompile(`(?i)E(\d{1,3})`)

// FilenameRecognitionRuleStore 定义文件名识别规则所需的配置读写能力。
type FilenameRecognitionRuleStore interface {
	GetByKey(key string) (*domain.SystemConfig, error)
	Upsert(key, value string) error
}

// FilenameRecognitionRule 是领域层规则结构的服务层别名。
type FilenameRecognitionRule = domain.FilenameRecognitionRule

type compiledFilenameRecognitionRule struct {
	rule       FilenameRecognitionRule
	expression *regexp.Regexp
}

// FilenameRecognitionRuleSet 是领域层规则集结构的服务层别名。
type FilenameRecognitionRuleSet = domain.FilenameRecognitionRuleSet

// SetFilenameRecognitionRuleStore 注入规则配置存储，并清空旧缓存。
func (s *TmdbService) SetFilenameRecognitionRuleStore(store FilenameRecognitionRuleStore) {
	s.filenameRuleMu.Lock()
	defer s.filenameRuleMu.Unlock()
	s.filenameRuleStore = store
	s.filenameRulesLoaded = false
	s.filenameRules = nil
	s.compiledFilenameRules = nil
}

// GetFilenameRecognitionRules 返回当前生效的文件名识别规则。
func (s *TmdbService) GetFilenameRecognitionRules() FilenameRecognitionRuleSet {
	rules, _ := s.loadFilenameRecognitionRules()
	return FilenameRecognitionRuleSet{
		Rules:          cloneFilenameRecognitionRules(rules),
		ConfigKey:      filenameRecognitionRulesConfigKey,
		InputTransform: "去除扩展名，将点号、下划线和连字符转换为空格，再合并连续空白",
	}
}

// SaveFilenameRecognitionRules 校验、持久化并立即启用规则。
func (s *TmdbService) SaveFilenameRecognitionRules(rules []FilenameRecognitionRule) (FilenameRecognitionRuleSet, error) {
	normalized, compiled, err := validateAndCompileFilenameRecognitionRules(rules)
	if err != nil {
		return FilenameRecognitionRuleSet{}, err
	}
	if s.filenameRuleStore == nil {
		return FilenameRecognitionRuleSet{}, fmt.Errorf("文件名识别规则存储未初始化")
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		return FilenameRecognitionRuleSet{}, fmt.Errorf("序列化文件名识别规则失败: %v", err)
	}
	if err := s.filenameRuleStore.Upsert(filenameRecognitionRulesConfigKey, string(payload)); err != nil {
		return FilenameRecognitionRuleSet{}, fmt.Errorf("保存文件名识别规则失败: %v", err)
	}

	s.filenameRuleMu.Lock()
	s.filenameRules = normalized
	s.compiledFilenameRules = compiled
	s.filenameRulesLoaded = true
	s.filenameRuleMu.Unlock()

	return s.GetFilenameRecognitionRules(), nil
}

// ResetFilenameRecognitionRules 恢复并保存内置常用模板。
func (s *TmdbService) ResetFilenameRecognitionRules() (FilenameRecognitionRuleSet, error) {
	return s.SaveFilenameRecognitionRules(DefaultFilenameRecognitionRules())
}

// DefaultFilenameRecognitionRules 返回参考常见媒体命名约定的内置模板。
func DefaultFilenameRecognitionRules() []FilenameRecognitionRule {
	return []FilenameRecognitionRule{
		{ID: "tv_sxe_compact", Name: "紧凑型 SxxExx 合并剧集", MediaType: "tv", Enabled: true, Priority: 9,
			Pattern: `(?i)^(?P<title>.+?)\s*S(?P<season>\d+)E(?P<episode>\d{1,3})(?P<episodes>(?:\s*E\d{1,3})*)\s*$`,
			Example: "举重妖精金福珠S01E01.mkv", Description: "支持中文标题紧贴 S01E01，以及 S01E01E02 连续合并集。"},
		{
			ID: "tv_sxe", Name: "SxxExx 标准剧集", MediaType: "tv", Enabled: true, Priority: 10,
			Pattern: `(?i)^(?P<title>.+?)\s*S(?P<season>\d+)E(?P<episode>\d{1,3})(?P<episodes>(?:\s*E\d{1,3})*)(?:\s+.*)?$`,
			Example: "你是谁 - S01E01E02 .mp4", Description: "支持片名紧贴S01E01、特别篇S00及S01E01E02合并集；episodes保留后续全部集号。",
		},
		{
			ID: "tv_x", Name: "数字 x 数字剧集", MediaType: "tv", Enabled: true, Priority: 20,
			Pattern: `(?i)^(?P<title>.+?)\s+(?P<season>\d{1,2})x(?P<episode>\d{1,3})(?:\s+.*)?$`,
			Example: "Breaking Bad - 1x02 - Cat's in the Bag.mkv", Description: "适用于 1x02、02x15 等命名。",
		},
		{
			ID: "tv_words", Name: "Season Episode 剧集", MediaType: "tv", Enabled: true, Priority: 30,
			Pattern: `(?i)^(?P<title>.+?)\s+Season\s*(?P<season>\d{1,2})\s+Episode\s*(?P<episode>\d{1,3})(?:\s+.*)?$`,
			Example: "The Office Season 2 Episode 3.mp4", Description: "适用于英文 Season 2 Episode 3 命名。",
		},
		{
			ID: "tv_chinese", Name: "中文季集", MediaType: "tv", Enabled: true, Priority: 40,
			Pattern: `^(?P<title>.+?)\s*第(?P<season>\d{1,2})季\s*第(?P<episode>\d{1,3})集(?:\s+.*)?$`,
			Example: "庆余年 第2季 第05集.mp4", Description: "适用于第2季第5集等中文命名，季数需使用阿拉伯数字。",
		},
		{
			ID: "tv_episode", Name: "EP 单集", MediaType: "tv", Enabled: true, Priority: 50, DefaultSeason: 1,
			Pattern: `(?i)^(?P<title>.+?)\s+(?:EP?|Episode)\s*(?P<episode>\d{1,3})(?:\s+.*)?$`,
			Example: "葬送的芙莉莲 EP08.mp4", Description: "仅包含集数时按默认季识别，默认第 1 季。",
		},
		{
			ID: "movie_year", Name: "电影标题与年份", MediaType: "movie", Enabled: true, Priority: 60,
			Pattern: `^(?P<title>.+?)\s+(?P<year>19\d{2}|20\d{2})(?:\s+.*)?$`,
			Example: "Inception.2010.1080p.BluRay.mkv", Description: "适用于标题后包含四位发行年份的电影命名。",
		},
		{
			ID: "anime_number", Name: "动漫纯集数", MediaType: "tv", Enabled: false, Priority: 70, DefaultSeason: 1,
			Pattern: `^(?P<title>.+?)\s+(?P<episode>\d{1,4})(?:v\d+)?(?:\s+.*)?$`,
			Example: "海贼王 - 1120.mp4", Description: "可能与年份或标题数字冲突，确认资源命名稳定后再启用。",
		},
	}
}

func (s *TmdbService) loadFilenameRecognitionRules() ([]FilenameRecognitionRule, []compiledFilenameRecognitionRule) {
	s.filenameRuleMu.RLock()
	if s.filenameRulesLoaded {
		rules := cloneFilenameRecognitionRules(s.filenameRules)
		compiled := append([]compiledFilenameRecognitionRule(nil), s.compiledFilenameRules...)
		s.filenameRuleMu.RUnlock()
		return rules, compiled
	}
	s.filenameRuleMu.RUnlock()

	rules := DefaultFilenameRecognitionRules()
	if s.filenameRuleStore != nil {
		if config, err := s.filenameRuleStore.GetByKey(filenameRecognitionRulesConfigKey); err == nil && config != nil && strings.TrimSpace(config.ConfigVal) != "" {
			var stored []FilenameRecognitionRule
			if json.Unmarshal([]byte(config.ConfigVal), &stored) == nil {
				// 仅升级未修改过的旧内置表达式，保留用户的开关、顺序及自定义规则。
				for i := range stored {
					for _, builtin := range rules {
						if stored[i].ID != builtin.ID {
							continue
						}
						oldPattern := strings.ReplaceAll(builtin.Pattern, `(?P<season>\d+)`, `(?P<season>\d{1,2})`)
						if stored[i].Pattern == oldPattern || (stored[i].ID == "tv_sxe" && stored[i].Pattern == legacySxePattern) {
							stored[i].Pattern = builtin.Pattern
							stored[i].Description = builtin.Description
						}
					}
				}
				if normalized, _, validateErr := validateAndCompileFilenameRecognitionRules(stored); validateErr == nil {
					rules = normalized
				}
			}
		}
	}
	normalized, compiled, _ := validateAndCompileFilenameRecognitionRules(rules)

	s.filenameRuleMu.Lock()
	if !s.filenameRulesLoaded {
		s.filenameRules = normalized
		s.compiledFilenameRules = compiled
		s.filenameRulesLoaded = true
	}
	rules = cloneFilenameRecognitionRules(s.filenameRules)
	compiled = append([]compiledFilenameRecognitionRule(nil), s.compiledFilenameRules...)
	s.filenameRuleMu.Unlock()
	return rules, compiled
}

func (s *TmdbService) compiledRulesForFilenameParsing() []compiledFilenameRecognitionRule {
	_, compiled := s.loadFilenameRecognitionRules()
	return compiled
}

func validateAndCompileFilenameRecognitionRules(rules []FilenameRecognitionRule) ([]FilenameRecognitionRule, []compiledFilenameRecognitionRule, error) {
	if len(rules) == 0 {
		return nil, nil, fmt.Errorf("至少需要保留一条文件名识别规则")
	}
	if len(rules) > 50 {
		return nil, nil, fmt.Errorf("文件名识别规则不能超过 50 条")
	}

	seen := make(map[string]struct{}, len(rules))
	normalized := cloneFilenameRecognitionRules(rules)
	for index := range normalized {
		rule := &normalized[index]
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Pattern = strings.TrimSpace(rule.Pattern)
		rule.MediaType = strings.ToLower(strings.TrimSpace(rule.MediaType))
		rule.Example = strings.TrimSpace(rule.Example)
		rule.Description = strings.TrimSpace(rule.Description)
		if rule.Priority <= 0 {
			rule.Priority = (index + 1) * 10
		}
		if !regexp.MustCompile(`^[a-z][a-z0-9_-]{1,39}$`).MatchString(rule.ID) {
			return nil, nil, fmt.Errorf("第 %d 条规则 ID 仅支持小写字母、数字、下划线和连字符，且长度为 2-40", index+1)
		}
		if _, exists := seen[rule.ID]; exists {
			return nil, nil, fmt.Errorf("规则 ID %q 重复", rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if rule.Name == "" || rule.Pattern == "" {
			return nil, nil, fmt.Errorf("规则 %q 的名称和正则不能为空", rule.ID)
		}
		if rule.MediaType != "movie" && rule.MediaType != "tv" {
			return nil, nil, fmt.Errorf("规则 %q 的媒体类型必须为 movie 或 tv", rule.ID)
		}
		if rule.DefaultSeason < 0 || rule.DefaultSeason > 99 {
			return nil, nil, fmt.Errorf("规则 %q 的默认季必须在 0-99 之间", rule.ID)
		}
		expression, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, nil, fmt.Errorf("规则 %q 的正则无效: %v", rule.ID, err)
		}
		groups := make(map[string]bool)
		for _, name := range expression.SubexpNames() {
			groups[name] = true
		}
		if !groups["title"] {
			return nil, nil, fmt.Errorf("规则 %q 必须包含命名捕获组 (?P<title>...)", rule.ID)
		}
		if rule.MediaType == "tv" && !groups["episode"] {
			return nil, nil, fmt.Errorf("剧集规则 %q 必须包含命名捕获组 (?P<episode>...)", rule.ID)
		}
		if rule.Enabled && rule.Example != "" && expression.FindStringSubmatch(normalizeFilenameRecognitionInput(rule.Example, true)) == nil {
			return nil, nil, fmt.Errorf("规则 %q 无法匹配其示例文件名", rule.ID)
		}
	}

	sort.SliceStable(normalized, func(i, j int) bool { return normalized[i].Priority < normalized[j].Priority })
	compiled := make([]compiledFilenameRecognitionRule, 0, len(normalized))
	for _, rule := range normalized {
		if !rule.Enabled {
			continue
		}
		expression, _ := regexp.Compile(rule.Pattern)
		compiled = append(compiled, compiledFilenameRecognitionRule{rule: rule, expression: expression})
	}
	if len(compiled) == 0 {
		return nil, nil, fmt.Errorf("至少需要启用一条文件名识别规则")
	}
	return normalized, compiled, nil
}

func matchFilenameRecognitionRule(input string, rules []compiledFilenameRecognitionRule) (*ParsedFilename, bool) {
	for _, compiled := range rules {
		matches := compiled.expression.FindStringSubmatch(input)
		if matches == nil {
			continue
		}
		values := make(map[string]string)
		for index, name := range compiled.expression.SubexpNames() {
			if index > 0 && name != "" && index < len(matches) {
				values[name] = strings.TrimSpace(matches[index])
			}
		}
		season := compiled.rule.DefaultSeason
		if values["season"] != "" {
			season, _ = strconv.Atoi(values["season"])
		}
		episode, _ := strconv.Atoi(values["episode"])
		episodes := []int{}
		if episode > 0 {
			episodes = append(episodes, episode)
		}
		valid := true
		for _, token := range filenameEpisodeToken.FindAllStringSubmatch(values["episodes"], -1) {
			n, _ := strconv.Atoi(token[1])
			if n <= episode {
				valid = false
				break
			}
			episodes = append(episodes, n)
			episode = n
		}
		if !valid {
			continue
		}
		episode, _ = strconv.Atoi(values["episode"])
		year, _ := strconv.Atoi(values["year"])
		return &ParsedFilename{
			Title:           values["title"],
			Year:            year,
			MediaType:       compiled.rule.MediaType,
			Season:          season,
			Episode:         episode,
			Episodes:        episodes,
			MatchedRuleID:   compiled.rule.ID,
			MatchedRuleName: compiled.rule.Name,
		}, true
	}
	return nil, false
}

func normalizeFilenameRecognitionInput(input string, trimExt bool) string {
	name := strings.TrimSpace(input)
	if trimExt {
		if ext := regexp.MustCompile(`(?i)\.[a-z0-9]{1,8}$`).FindString(name); ext != "" {
			name = strings.TrimSuffix(name, ext)
		}
	}
	name = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(name)
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(name, " "))
}

func cloneFilenameRecognitionRules(rules []FilenameRecognitionRule) []FilenameRecognitionRule {
	return append([]FilenameRecognitionRule(nil), rules...)
}
