package domain

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// ErrStrm115MetadataUnsupported 阻止目录来源进入仅修复分享映射的流程，不降级或写文件。
var ErrStrm115MetadataUnsupported = errors.New("115目录选择不支持分享映射元数据重导或历史基线采纳；请使用来源选择的立即应用或正常增量导出")

var selectionWorkPattern = regexp.MustCompile(`^tmdb:(movie|tv):([1-9][0-9]*)$`)
var selectionRefPattern = regexp.MustCompile(`^(0|[1-9][0-9]*):([1-9][0-9]*)$`)
var selectionTagPattern = regexp.MustCompile(`(?i)\[tmdbid=([^\]]*)\]|\{tmdb-([^}]*)\}`)
var selectionEpisodePattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])S([0-9]+)E([0-9]+)(?:[^a-z0-9]|$)`)
var selectionSeasonPattern = regexp.MustCompile(`(?i)^Season\s+([0-9]+)$`)
var selectionMarkerPattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:s[0-9]+(?:e[0-9]+)*|e[0-9]+|season\s*[0-9]+|episode\s*[0-9]+)(?:$|[^a-z0-9])`)
var selectionRangePattern = regexp.MustCompile(`(?i)s[0-9]+e[0-9]+(?:[-+&~～]|[ ._-]*to[ ._-]*)[0-9]+`)
var selectionPositivePattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// NormalizeStrm115Path 显式归一化历史 Windows 分隔符，不依赖宿主操作系统。
func NormalizeStrm115Path(value string) string { return strings.ReplaceAll(value, "\\", "/") }

// ParseStrm115Ref 仅接受规范的非负配置 ID 和正数文件 ID，溢出或多余 token 均拒绝。
func ParseStrm115Ref(value string) (int64, int64, error) {
	parts := selectionRefPattern.FindStringSubmatch(value)
	if len(parts) != 3 {
		return 0, 0, fmt.Errorf("115目录引用无效")
	}
	configID, configErr := strconv.ParseInt(parts[1], 10, 64)
	fileID, fileErr := strconv.ParseInt(parts[2], 10, 64)
	if configErr != nil || fileErr != nil {
		return 0, 0, fmt.Errorf("115目录引用越界")
	}
	return configID, fileID, nil
}

// ValidateShareSelectionChange 保留旧候选载荷的分享默认值，按来源校验互斥引用。
func ValidateShareSelectionChange(change *ShareSelectionChange) error {
	if change.SourceKind == "" {
		change.SourceKind = "share"
	}
	switch change.SourceKind {
	case "share":
		if change.CandidateID > 0 && change.ExternalRef == "" {
			return nil
		}
	case "strm115":
		if change.CandidateID == 0 {
			_, _, err := ParseStrm115Ref(change.ExternalRef)
			return err
		}
	}
	return fmt.Errorf("来源类型或候选引用无效")
}

// ParseStrm115Item 精确匹配作品标签和唯一季集；识别失败绝不降级为电影。
func ParseStrm115Item(work, filename string) (int, int, bool) {
	key := selectionWorkPattern.FindStringSubmatch(work)
	if len(key) != 3 {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(key[2], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	filename = NormalizeStrm115Path(filename)
	tags := selectionTagPattern.FindAllStringSubmatch(filename, -1)
	if len(tags) == 0 {
		return 0, 0, false
	}
	remaining := strings.ToLower(selectionTagPattern.ReplaceAllString(filename, ""))
	if strings.Contains(remaining, "tmdbid=") || strings.Contains(remaining, "{tmdb-") {
		return 0, 0, false
	}
	for _, tag := range tags {
		value := tag[1]
		if value == "" {
			value = tag[2]
		}
		if !selectionPositivePattern.MatchString(value) {
			return 0, 0, false
		}
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil || parsed <= 0 || parsed != id {
			return 0, 0, false
		}
	}
	base := path.Base(filename)
	if key[1] == "movie" {
		return 0, 0, !selectionMarkerPattern.MatchString(filename)
	}
	matches := selectionEpisodePattern.FindAllStringSubmatch(base, -1)
	if len(matches) != 1 || selectionRangePattern.MatchString(base) {
		return 0, 0, false
	}
	markerText := selectionEpisodePattern.ReplaceAllString(base, " ")
	if selectionMarkerPattern.MatchString(markerText) {
		return 0, 0, false
	}
	season, seasonErr := strconv.Atoi(matches[0][1])
	episode, episodeErr := strconv.Atoi(matches[0][2])
	if seasonErr != nil || episodeErr != nil || season < 0 || episode <= 0 {
		return 0, 0, false
	}
	for _, part := range strings.Split(path.Dir(filename), "/") {
		if marker := selectionSeasonPattern.FindStringSubmatch(part); len(marker) > 0 {
			parsed, parseErr := strconv.Atoi(marker[1])
			if parseErr != nil || parsed != season {
				return 0, 0, false
			}
		} else if selectionMarkerPattern.MatchString(part) {
			return 0, 0, false
		}
	}
	return season, episode, true
}

// Strm115Entry 保存纯直链内容，不创建分享播放映射。
type Strm115Entry struct {
	Content     string `json:"content"`
	ExternalRef string `json:"external_ref"`
}

// Strm115SelectionSource 将目录选择绑定到既有作品元数据，不借用目录位置生成输出路径。
type Strm115SelectionSource struct {
	Selection ShareSelection
	Source    ShareStrmSource
}

// BuildStrm115Entry 校验服务地址及云端路径后构建直链，不执行任何 IO。
func BuildStrm115Entry(serverURL string, candidate ShareCandidate) (Strm115Entry, error) {
	if candidate.SourceKind != "strm115" || candidate.Cloud115ID <= 0 {
		return Strm115Entry{}, fmt.Errorf("115目录来源或账号无效")
	}
	if _, _, err := ParseStrm115Ref(candidate.ExternalRef); err != nil {
		return Strm115Entry{}, err
	}
	parsed, err := url.Parse(serverURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.ContainsAny(serverURL, "\r\n\\#") {
		return Strm115Entry{}, fmt.Errorf("ServerURL 无效")
	}
	cloud := candidate.CloudPath
	if !strings.HasPrefix(cloud, "/") || strings.HasPrefix(cloud, "//") || path.Clean(cloud) != cloud || cloud == "/" || strings.Contains(cloud, "\\") || strings.IndexFunc(cloud, unicode.IsControl) >= 0 {
		return Strm115Entry{}, fmt.Errorf("115云端路径无效")
	}
	return Strm115Entry{ExternalRef: candidate.ExternalRef, Content: strings.TrimRight(serverURL, "/") + "/direct-link?path=" + url.QueryEscape(cloud) + "&cloud115_id=" + strconv.Itoa(candidate.Cloud115ID) + "\n"}, nil
}
