package domain

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var shareStrmCodePattern = regexp.MustCompile(`(?:115cdn\.com|115\.com)/s/(\w+)`)

// BuildShareStrmEntry 按导出语义构造来源映射，路径参与稳定 ID，显式密码优先于 URL 提取码；不访问外部系统。
func BuildShareStrmEntry(source ShareStrmSource) (ShareStrmEntry, error) {
	matches := shareStrmCodePattern.FindStringSubmatch(source.URL)
	if len(matches) < 2 {
		return ShareStrmEntry{}, fmt.Errorf("本地记录的115分享地址无效")
	}
	filePath := path.Clean(strings.ReplaceAll(source.FileName, "\\", "/"))
	entry := ShareStrmEntry{
		ID:         uuid.NewSHA1(uuid.NameSpaceURL, []byte(matches[1]+":path:"+filePath)).String(),
		ShareCode:  matches[1],
		Password:   source.Password,
		FileID:     source.RemoteFileID,
		FileName:   path.Base(filePath),
		FilePath:   filePath,
		MediaID:    source.MediaID,
		Title:      source.Result.Title,
		PosterPath: source.Result.PosterPath,
		Episodes:   source.Episodes,
	}
	if entry.Password == "" {
		if parsed, err := url.Parse(source.URL); err == nil {
			entry.Password = strings.TrimSpace(parsed.Query().Get("password"))
		}
	}
	return entry, nil
}
