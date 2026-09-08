package service

import (
	"easy-strm/internal/domain"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	importLinkRE       = regexp.MustCompile(`(?i)(?:https?://|/)?(?:(?:www\.)?(?:115cdn\.com|115\.com)/s/|share\.115\.com/)([a-z0-9_]+)(?:\?[^\s\[\]()<>#]*)?(?:#[^\s\[\]()<>]*)?`)
	importMarkdownRE   = regexp.MustCompile(`\[([^\]\n]*)\]\((https?://[^\s)]+)\)`)
	importBareCodeRE   = regexp.MustCompile(`^\s*([A-Za-z0-9]{4})(?:\s+|$)`)
	importDecorationRE = regexp.MustCompile(`^(?:🔗\s*)?(?:【115】|\[115\])?\s*(?:大包[：:]\s*)?`)
	importHTTPRE       = regexp.MustCompile(`https?://\S+`)
)

type importOccurrence struct {
	entry  domain.ShareImportEntry
	line   int
	before string
	after  string
}

// ParseImport 解析混合分享文案；不调用网盘、不写库，歧义及冲突交给预览确认。
func (s *ShareRecordService) ParseImport(text string) (domain.ShareImportPreview, error) {
	out := domain.ShareImportPreview{Records: []domain.ShareImportEntry{}, Ignored: []string{}}
	if strings.TrimSpace(text) == "" {
		return out, fmt.Errorf("请输入分享内容")
	}
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\\&", "&", "\\_", "_", "\\(", "(", "\\)", ")").Replace(text)
	text = importMarkdownRE.ReplaceAllStringFunc(text, func(v string) string {
		m := importMarkdownRE.FindStringSubmatch(v)
		if importLinkRE.MatchString(m[1]) || strings.HasPrefix(m[1], "http") {
			return m[2]
		}
		return m[1] + " " + m[2]
	})
	lines := strings.Split(text, "\n")
	used := make([]bool, len(lines))
	hasLink := make([]bool, len(lines))
	occurrences := []importOccurrence{}
	for i, line := range lines {
		lines[i] = strings.TrimSpace(strings.TrimRight(line, "\\"))
		matches := importLinkRE.FindAllStringSubmatchIndex(lines[i], -1)
		for n, m := range matches {
			// 域名必须独立，避免把其他网址路径或域名后缀误判为分享。
			if m[0] > 0 && strings.ContainsAny(lines[i][m[0]-1:m[0]], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./_-") {
				continue
			}
			raw := lines[i][m[0]:m[1]]
			code := lines[i][m[2]:m[3]]
			raw = strings.TrimRight(raw, "，。；")
			if !strings.HasPrefix(strings.ToLower(raw), "http") {
				raw = "https://" + strings.TrimLeft(raw, "/")
			}
			parsed, err := url.Parse(raw)
			if err != nil {
				continue
			}
			entry := domain.ShareImportEntry{ShareCode: code, URL: "https://115.com/s/" + code, Lines: []int{i + 1}, Names: []string{}, Passwords: []string{}, Warnings: []string{}}
			entry.Passwords = appendImportCandidate(entry.Passwords, parsed.Query().Get("password"))
			before := ""
			if n == 0 {
				before = lines[i][:m[0]]
			}
			end := len(lines[i])
			if n+1 < len(matches) {
				end = matches[n+1][0]
			}
			after := lines[i][m[1]:end]
			occurrences = append(occurrences, importOccurrence{entry: entry, line: i, before: before, after: after})
			hasLink[i], used[i] = true, true
		}
	}
	// 先处理明确的同行名称和带访问码/复制提示的后置分享块，防止下一条抢走前一条名称。
	for k := range occurrences {
		o := &occurrences[k]
		end := len(lines)
		if k+1 < len(occurrences) {
			end = occurrences[k+1].line
		}
		body := o.after
		last := o.line
		for j := o.line + 1; j < end && lines[j] != ""; j++ {
			body += "\n" + lines[j]
			if shareAccessCodePattern.MatchString(lines[j]) || strings.Contains(lines[j], "复制这段") {
				last = j
			}
			if strings.Contains(lines[j], "复制这段") {
				break
			}
		}
		for _, m := range shareAccessCodePattern.FindAllStringSubmatch(body, -1) {
			o.entry.Passwords = appendImportCandidate(o.entry.Passwords, m[1])
		}
		after := o.after
		if m := importBareCodeRE.FindStringSubmatch(after); m != nil {
			o.entry.Passwords = appendImportCandidate(o.entry.Passwords, m[1])
			after = ""
		}
		o.entry.Name = cleanImportName(o.before)
		if o.entry.Name == "" {
			o.entry.Name = cleanImportName(after)
		}
		explicit := shareAccessCodePattern.MatchString(body) || strings.Contains(body, "复制这段")
		if explicit && last > o.line {
			for j := o.line + 1; j <= last; j++ {
				if o.entry.Name == "" {
					o.entry.Name = cleanImportName(lines[j])
				}
				used[j] = true
			}
		}
	}
	// 其次取未被消费的前置标题；没有前置标题时才尝试后置标题。
	ambiguousTitles := map[int]bool{}
	for k := range occurrences {
		o := &occurrences[k]
		prev := o.line - 1
		if o.entry.Name != "" {
			if prev >= 0 && !used[prev] && !hasLink[prev] {
				if name := cleanImportName(lines[prev]); name != "" && name != o.entry.Name {
					o.entry.Names = appendImportCandidate(o.entry.Names, o.entry.Name)
					o.entry.Names = appendImportCandidate(o.entry.Names, name)
					o.entry.Warnings = append(o.entry.Warnings, "链接前后均有名称，请核对原文")
					used[prev] = true
				}
			}
			continue
		}
		if prev >= 0 && !used[prev] && !hasLink[prev] {
			if name := cleanImportName(lines[prev]); name != "" {
				o.entry.Name = name
				if ambiguousTitles[prev] {
					o.entry.Warnings = append(o.entry.Warnings, "相邻标题归属不明确，请核对原文")
				}
				used[prev] = true
				continue
			}
		}
		next := o.line + 1
		if next < len(lines) && !used[next] && !hasLink[next] {
			if name := cleanImportName(lines[next]); name != "" {
				// 紧贴下一条链接时可能是下一条的前置标题，保留歧义而不擅自挪用。
				if next+1 < len(lines) && hasLink[next+1] {
					ambiguousTitles[next] = true
					o.entry.Warnings = append(o.entry.Warnings, "相邻标题归属不明确，请核对原文")
				} else {
					o.entry.Name = name
					used[next] = true
				}
			}
		}
	}
	byCode := map[string]int{}
	for _, o := range occurrences {
		e := o.entry
		e.Names = appendImportCandidate(e.Names, e.Name)
		if index, ok := byCode[e.ShareCode]; ok {
			target := &out.Records[index]
			target.Lines = append(target.Lines, e.Lines...)
			for _, name := range e.Names {
				target.Names = appendImportCandidate(target.Names, name)
			}
			for _, p := range e.Passwords {
				target.Passwords = appendImportCandidate(target.Passwords, p)
			}
			for _, w := range e.Warnings {
				target.Warnings = appendImportCandidate(target.Warnings, w)
			}
			out.Duplicates++
		} else {
			byCode[e.ShareCode] = len(out.Records)
			out.Records = append(out.Records, e)
		}
	}
	for i := range out.Records {
		e := &out.Records[i]
		if len(e.Names) > 0 {
			e.Name = e.Names[0]
		} else {
			e.Warnings = append(e.Warnings, "未确定名称，请补充")
		}
		if len(e.Passwords) > 0 {
			e.Password = e.Passwords[0]
		}
		if len(e.Names) > 1 {
			e.Warnings = append(e.Warnings, "同一分享有多个名称，请选择或修改")
		}
		if len(e.Passwords) > 1 {
			e.Warnings = append(e.Warnings, "访问码冲突，请核对")
		}
	}
	for i, line := range lines {
		if !used[i] && line != "" {
			out.Ignored = append(out.Ignored, fmt.Sprintf("第%d行：%s", i+1, line))
		}
	}
	if len(out.Records) == 0 {
		return out, fmt.Errorf("未识别到115分享链接")
	}
	return out, nil
}

func appendImportCandidate(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

func cleanImportName(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "复制这段") || strings.Contains(value, "随手转发") || strings.Contains(value, "全球顶级封装") || strings.Contains(value, "官方VIP") {
		return ""
	}
	value = importHTTPRE.ReplaceAllString(value, "")
	if m := shareAccessCodePattern.FindStringIndex(value); m != nil {
		value = value[:m[0]]
	}
	value = importDecorationRE.ReplaceAllString(value, "")
	value = strings.Trim(value, " \t：:")
	if value == "列表" || value == "有效" || value == "永久有效" {
		return ""
	}
	return value
}
