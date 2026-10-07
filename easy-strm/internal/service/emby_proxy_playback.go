package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type embyProxyPlaybackRequest struct {
	itemID      string
	prefix      string
	transcoding bool
}

// 先识别播放入口，再判断播放方式；转码和分片不能绕过 STRM 识别。
func embyPlaybackRequest(r *http.Request) (embyProxyPlaybackRequest, bool) {
	playback := embyProxyPlaybackRequest{}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return playback, false
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) > 0 && strings.EqualFold(parts[0], "emby") {
		playback.prefix = "/" + parts[0]
		parts = parts[1:]
	}
	if len(parts) < 3 || parts[1] == "" {
		return playback, false
	}
	group, action := strings.ToLower(parts[0]), strings.ToLower(parts[2])
	switch {
	case group == "items" && action == "download" && (len(parts) == 3 || len(parts) == 4):
	case group != "videos":
		return playback, false
	case (action == "hls" || action == "hls1"):
		playback.transcoding = true
	case len(parts) != 3:
		return playback, false
	case strings.HasSuffix(action, ".m3u8") || strings.HasSuffix(action, ".mpd"):
		playback.transcoding = true
	case action == "stream" || strings.HasPrefix(action, "stream.") || action == "original" || strings.HasPrefix(action, "original."):
	default:
		return playback, false
	}
	playback.itemID = parts[1]
	playback.transcoding = playback.transcoding || embyTranscodingQuery(r.URL.Query())
	return playback, true
}

func embyTranscodingQuery(query url.Values) bool {
	static := embyQuery(query, "Static")
	if strings.EqualFold(static, "false") || static == "0" || embyQuery(query, "TranscodeReasons") != "" || strings.EqualFold(embyQuery(query, "PlayMethod"), "Transcode") {
		return true
	}
	for _, key := range []string{"VideoCodec", "AudioCodec"} {
		if codec := embyQuery(query, key); codec != "" && !strings.EqualFold(codec, "copy") {
			return true
		}
	}
	return false
}

type embyProxyMediaSource struct {
	ID        string `json:"Id"`
	Path      string `json:"Path"`
	Protocol  string `json:"Protocol"`
	Container string `json:"Container"`
	IsRemote  bool   `json:"IsRemote"`
}

type embyProxyItem struct {
	ID           string                 `json:"Id"`
	Path         string                 `json:"Path"`
	IsShortcut   bool                   `json:"IsShortcut"`
	MediaSources []embyProxyMediaSource `json:"MediaSources"`
}

func (s *EmbyProxyService) playbackLocation(server *domain.EmbyServer, base *url.URL, incoming *http.Request, playback embyProxyPlaybackRequest) (location string, failure *embyProxyFailure) {
	source, failure := s.playbackSource(server, base, incoming, playback)
	if failure != nil {
		return "", failure
	}
	defer func() {
		if failure != nil {
			failure.mediaSourceID = source.ID
		}
	}()
	strm := embyProxySourceIsSTRM(source)
	if !strm {
		strm, failure = s.isSTRMItem(server, base, incoming, playback, source)
		if failure != nil {
			return "", failure
		}
	}
	if !strm {
		return "", nil
	}
	if playback.transcoding {
		return "", newEmbyProxyFailure("playback_policy", "strm_fallback_blocked", "STRM 禁止转码或 HLS 回退，已禁止回源", 200, nil)
	}
	if location, valid := embyProxySTRMLocation(source.Path); valid {
		return location, nil
	}
	return "", newEmbyProxyFailure("redirect", "invalid_strm_location", "STRM 未提供有效 HTTP(S) 播放地址，已禁止回源", 200, nil)
}

func (s *EmbyProxyService) playbackSource(server *domain.EmbyServer, base *url.URL, incoming *http.Request, playback embyProxyPlaybackRequest) (embyProxyMediaSource, *embyProxyFailure) {
	var info struct {
		MediaSources []embyProxyMediaSource `json:"MediaSources"`
		ErrorCode    string                 `json:"ErrorCode"`
	}
	path := playback.prefix + "/Items/" + url.PathEscape(playback.itemID) + "/PlaybackInfo"
	if failure := s.proxyMetadata(server, base, incoming, path, incoming.URL.Query(), "playback_info", &info); failure != nil {
		return embyProxyMediaSource{}, failure
	}
	if info.ErrorCode != "" || len(info.MediaSources) == 0 {
		return embyProxyMediaSource{}, newEmbyProxyFailure("media_source", "media_source_missing", "Emby 未提供可用媒体源，已禁止回源", 200, nil)
	}
	if id := embyQuery(incoming.URL.Query(), "MediaSourceId"); id != "" {
		for _, source := range info.MediaSources {
			if source.ID == id {
				return source, nil
			}
		}
		return embyProxyMediaSource{}, newEmbyProxyFailure("media_source", "media_source_missing", "Emby 未提供客户端选择的媒体源，已禁止回源", 200, nil)
	}
	if len(info.MediaSources) != 1 {
		return embyProxyMediaSource{}, newEmbyProxyFailure("media_source", "media_source_ambiguous", "存在多个播放媒体源，请指定 MediaSourceId，已禁止回源", 200, nil)
	}
	return info.MediaSources[0], nil
}

func (s *EmbyProxyService) isSTRMItem(server *domain.EmbyServer, base *url.URL, incoming *http.Request, playback embyProxyPlaybackRequest, source embyProxyMediaSource) (bool, *embyProxyFailure) {
	path := strings.TrimSpace(source.Path)
	parsed, err := url.Parse(path)
	if path == "" || (source.IsRemote && (err != nil || parsed.Scheme == "")) {
		return false, newEmbyProxyFailure("media_kind", "media_kind_unknown", "无法确认播放媒体类型，已禁止回源", 200, nil)
	}
	query := incoming.URL.Query()
	for name := range query {
		if strings.EqualFold(name, "Ids") || strings.EqualFold(name, "Fields") || strings.EqualFold(name, "Limit") {
			delete(query, name)
		}
	}
	query.Set("Ids", playback.itemID)
	query.Set("Fields", "Path,MediaSources")
	query.Set("Limit", "1")
	var info struct {
		Items []embyProxyItem `json:"Items"`
	}
	if failure := s.proxyMetadata(server, base, incoming, playback.prefix+"/Items", query, "item_info", &info); failure != nil {
		return false, failure
	}
	if len(info.Items) != 1 || info.Items[0].ID != playback.itemID {
		return false, newEmbyProxyFailure("media_kind", "media_kind_unknown", "Emby 未提供对应项目的媒体类型，已禁止回源", 200, nil)
	}
	return classifyEmbyProxyItem(info.Items[0], source)
}

func classifyEmbyProxyItem(item embyProxyItem, selected embyProxyMediaSource) (bool, *embyProxyFailure) {
	if item.IsShortcut || embyProxySourceIsSTRM(embyProxyMediaSource{Path: item.Path}) {
		return true, nil
	}
	matched := len(item.MediaSources) == 0 || selected.ID == ""
	for _, source := range item.MediaSources {
		if selected.ID != "" && source.ID != selected.ID {
			continue
		}
		matched = true
		if embyProxySourceIsSTRM(source) {
			return true, nil
		}
	}
	if !matched || strings.TrimSpace(item.Path) == "" {
		return false, newEmbyProxyFailure("media_kind", "media_kind_unknown", "无法确认项目为非 STRM 媒体，已禁止回源", 200, nil)
	}
	return false, nil
}

func embyProxySourceIsSTRM(source embyProxyMediaSource) bool {
	path := strings.ToLower(strings.TrimSpace(source.Path))
	return strings.HasPrefix(path, "http:") || strings.HasPrefix(path, "https:") || strings.HasSuffix(path, ".strm") || strings.EqualFold(source.Protocol, "Http") || strings.EqualFold(source.Protocol, "Https") || strings.EqualFold(source.Container, "strm")
}

func embyProxySTRMLocation(path string) (string, bool) {
	if strings.ContainsAny(path, "\r\n") {
		return "", false
	}
	value := strings.TrimSpace(path)
	parsed, err := url.Parse(value)
	return value, err == nil && parsed.Hostname() != "" && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
}

func (s *EmbyProxyService) proxyMetadata(server *domain.EmbyServer, base *url.URL, incoming *http.Request, path string, query url.Values, stage string, output any) *embyProxyFailure {
	request, err := buildEmbyProxyMetadataRequest(server, base, incoming, path, query)
	if err != nil {
		return newEmbyProxyFailure(stage, "request_invalid", "无法构造 Emby 媒体信息请求，已禁止回源", 0, err)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return newEmbyProxyFailure(stage, "connection_failed", "Emby 媒体信息请求失败，已禁止回源", 0, err)
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		failure := newEmbyProxyFailure(stage, "authorization_failed", "Emby 拒绝当前客户端的播放授权，已禁止回源", response.StatusCode, nil)
		failure.status = response.StatusCode
		return failure
	}
	if response.StatusCode != 200 {
		return newEmbyProxyFailure(stage, "upstream_http_error", "Emby 媒体信息请求返回异常状态，已禁止回源", response.StatusCode, nil)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(output); err != nil {
		return newEmbyProxyFailure(stage, "invalid_json", "Emby 媒体信息解析失败，已禁止回源", response.StatusCode, err)
	}
	return nil
}

func buildEmbyProxyMetadataRequest(server *domain.EmbyServer, base *url.URL, incoming *http.Request, path string, query url.Values) (*http.Request, error) {
	relative, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	endpoint := *embyProxyTarget(base, relative.Path)
	escapedBase := strings.TrimRight(endpoint.EscapedPath(), "/")
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + relative.Path
	endpoint.RawPath = escapedBase + relative.EscapedPath()
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(incoming.Context(), http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"Authorization", "X-Emby-Authorization", "X-Emby-Token", "X-MediaBrowser-Token", "Cookie", "User-Agent"} {
		for _, value := range incoming.Header.Values(key) {
			request.Header.Add(key, value)
		}
	}
	// 沿用客户端身份；缺失时才使用实例 API Key，不透传 Range 到 JSON 查询。
	if !embyClientIdentity(incoming) {
		request.Header.Set("X-Emby-Token", server.APIKey)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Request-ID", logger.RequestID(incoming.Context()))
	return request, nil
}
