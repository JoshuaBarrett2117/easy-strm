package service

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"easy-strm/internal/pkg/logger"
)

func forceEmbyDirectPlayResponse(response *http.Response) {
	if response.Request == nil || response.Request.URL == nil {
		return
	}
	itemID := embyDirectPlayItemID(response.Request)
	if itemID == "" {
		return
	}
	log := logger.WithContext(response.Request.Context(), "emby_proxy")
	warn := func(reason string) {
		log.Log(logger.WARN, "强制直接播放未改写，保留原始播放信息响应", logger.Fields{"item_id": itemID, "reason": reason}, nil)
	}
	if response.Uncompressed {
		warn("encoded_body")
		return
	}
	for _, encoding := range response.Header.Values("Content-Encoding") {
		if encoding != "" && !strings.EqualFold(strings.TrimSpace(encoding), "identity") {
			warn("encoded_body")
			return
		}
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/json" && !(strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))) {
		warn("non_json_response")
		return
	}
	if response.Body == nil || response.Body == http.NoBody {
		warn("empty_body")
		return
	}
	if response.ContentLength > embyDirectPlayBodyLimit {
		warn("body_too_large")
		return
	}
	original := response.Body
	body, err := readEmbyDirectPlayBody(original)
	response.Body = &embyDirectPlayReplayBody{Reader: io.MultiReader(bytes.NewReader(body), original), Closer: original}
	if err != nil {
		warn("body_read_failed")
		return
	}
	if len(body) > embyDirectPlayBodyLimit {
		warn("body_too_large")
		return
	}
	rewritten, changed, localSources, reason := rewriteEmbyDirectPlayResponseJSON(body)
	if reason != "" {
		warn(reason)
		return
	}
	if err := original.Close(); err != nil {
		log.Log(logger.WARN, "强制直接播放已改写，原始响应体关闭失败", logger.Fields{"item_id": itemID, "reason": "body_close_failed"}, nil)
	}
	response.Body = io.NopCloser(bytes.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	response.TransferEncoding = nil
	response.Header.Del("Transfer-Encoding")
	for _, index := range localSources {
		log.Log(logger.WARN, "强制直接播放媒体源非远程，保留 IsRemote", logger.Fields{"item_id": itemID, "source_index": index, "reason": "is_remote_false"}, nil)
	}
	log.Log(logger.INFO, "强制直接播放已改写播放信息响应", logger.Fields{"item_id": itemID, "changed_fields": changed}, nil)
}

func rewriteEmbyDirectPlayResponseJSON(body []byte) ([]byte, []string, []int, string) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil, nil, "empty_body"
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil, nil, nil, "invalid_json"
	}
	rawSources, exists := root["MediaSources"]
	if !exists {
		return nil, nil, nil, "missing_media_sources"
	}
	var sources []json.RawMessage
	if err := json.Unmarshal(rawSources, &sources); err != nil || sources == nil {
		return nil, nil, nil, "invalid_media_sources"
	}
	if len(sources) == 0 {
		return nil, nil, nil, "empty_media_sources"
	}
	// 即使前置判定与响应之间媒体状态发生变化，也不能删除直播的转码或生命周期信息。
	var item embyProxyItem
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, nil, nil, "invalid_media_source"
	}
	if embyProxyItemIsLive(item, embyProxyMediaSource{}) {
		return nil, nil, nil, "live_media_source"
	}
	changed := make([]string, 0)
	var localSources []int
	for index, rawSource := range sources {
		var source map[string]json.RawMessage
		if err := json.Unmarshal(rawSource, &source); err != nil || source == nil {
			return nil, nil, nil, "invalid_media_source"
		}
		prefix := "MediaSources[" + strconv.Itoa(index) + "]."
		for _, field := range []struct{ name, value string }{
			{"SupportsDirectStream", "false"},
			{"SupportsTranscoding", "false"},
			{"SupportsDirectPlay", "true"},
		} {
			if string(bytes.TrimSpace(source[field.name])) != field.value {
				source[field.name] = json.RawMessage(field.value)
				changed = append(changed, prefix+field.name)
			}
		}
		for _, field := range []string{"DirectStreamUrl", "TranscodingUrl"} {
			if _, present := source[field]; present {
				delete(source, field)
				changed = append(changed, prefix+field)
			}
		}
		if bytes.Equal(bytes.TrimSpace(source["IsRemote"]), []byte("false")) {
			localSources = append(localSources, index)
		}
		encodedSource, err := json.Marshal(source)
		if err != nil {
			return nil, nil, nil, "json_encode_failed"
		}
		sources[index] = encodedSource
	}
	encodedSources, err := json.Marshal(sources)
	if err != nil {
		return nil, nil, nil, "json_encode_failed"
	}
	root["MediaSources"] = encodedSources
	rewritten, err := json.Marshal(root)
	if err != nil {
		return nil, nil, nil, "json_encode_failed"
	}
	return rewritten, changed, localSources, ""
}
