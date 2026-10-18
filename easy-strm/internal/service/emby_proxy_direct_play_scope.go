package service

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// force_direct_play 只作用于 STRM；直播和本地视频保留 Emby 的协商和转码能力。
// 改写前查询项目类型及生命周期字段，直播不写入判定缓存；类型未知时仅跳过协商改写，
// 实际流请求仍执行 playbackLocation 的授权和类型校验。

const (
	embyDirectPlayVerdictTTL      = 10 * time.Minute
	embyDirectPlayVerdictCapacity = 2048
)

type embyDirectPlayScopeKey struct{}

type embyDirectPlayVerdict struct {
	remote  bool
	expires time.Time
}

func withEmbyDirectPlayScope(request *http.Request, forced bool) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), embyDirectPlayScopeKey{}, forced))
}

func embyDirectPlayForced(request *http.Request) bool {
	if request == nil {
		return false
	}
	forced, _ := request.Context().Value(embyDirectPlayScopeKey{}).(bool)
	return forced
}

// embyDirectPlayPathPrefix 从请求路径还原反代前缀（/emby 或空）。
func embyDirectPlayPathPrefix(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 && strings.EqualFold(parts[0], "emby") {
		return "/" + parts[0]
	}
	return ""
}

// directPlayApplies 判定当前请求是否应当执行强制直接播放改写。
// 非 PlaybackInfo 请求沿用既有行为（交由 forceEmbyDirectPlay 自身判定）。
func (s *EmbyProxyService) directPlayApplies(server *domain.EmbyServer, base *url.URL, request *http.Request) bool {
	itemID := embyDirectPlayItemID(request)
	if itemID == "" {
		return true
	}
	key := strconv.Itoa(server.ID) + "/" + itemID
	if verdict, ok := s.loadDirectPlayVerdict(key); ok {
		return verdict
	}
	remote, live, failure := s.lookupDirectPlayVerdict(server, base, request, itemID)
	if failure != nil {
		logger.WithContext(request.Context(), "emby_proxy").Log(logger.WARN, "强制直接播放未改写，媒体类型未知", logger.Fields{
			"item_id": itemID,
			"reason":  failure.reason,
		}, nil)
		return false
	}
	if !live {
		s.storeDirectPlayVerdict(key, remote)
	}
	return remote
}

func (s *EmbyProxyService) loadDirectPlayVerdict(key string) (bool, bool) {
	s.directPlayMu.Lock()
	defer s.directPlayMu.Unlock()
	entry, ok := s.directPlayCache[key]
	if !ok || time.Now().After(entry.expires) {
		return false, false
	}
	return entry.remote, true
}

func (s *EmbyProxyService) storeDirectPlayVerdict(key string, remote bool) {
	s.directPlayMu.Lock()
	defer s.directPlayMu.Unlock()
	if s.directPlayCache == nil || len(s.directPlayCache) >= embyDirectPlayVerdictCapacity {
		s.directPlayCache = make(map[string]embyDirectPlayVerdict, embyDirectPlayVerdictCapacity)
	}
	s.directPlayCache[key] = embyDirectPlayVerdict{remote: remote, expires: time.Now().Add(embyDirectPlayVerdictTTL)}
}

func (s *EmbyProxyService) lookupDirectPlayVerdict(server *domain.EmbyServer, base *url.URL, incoming *http.Request, itemID string) (remote, live bool, failure *embyProxyFailure) {
	// 必须沿用客户端原始查询参数（UserId、api_key 等）：Emby 的 /Items 在用户身份下
	// 缺少 UserId 会返回空集合，导致 STRM 被判为「类型未知」而丢失强制直连。
	query := incoming.URL.Query()
	if query == nil {
		query = url.Values{}
	}
	for name := range query {
		if strings.EqualFold(name, "Ids") || strings.EqualFold(name, "Fields") || strings.EqualFold(name, "Limit") {
			delete(query, name)
		}
	}
	query.Set("Ids", itemID)
	query.Set("Fields", "Path,MediaSources")
	query.Set("Limit", "1")
	var info struct {
		Items []embyProxyItem `json:"Items"`
	}
	prefix := embyDirectPlayPathPrefix(incoming.URL.Path)
	if failure := s.proxyMetadata(server, base, incoming, prefix+"/Items", query, "direct_play_kind", &info); failure != nil {
		return false, false, failure
	}
	if len(info.Items) != 1 || info.Items[0].ID != itemID {
		return false, false, newEmbyProxyFailure("direct_play_kind", "media_kind_unknown", "Emby 未提供对应项目的媒体类型", 200, nil)
	}
	if embyProxyItemIsLive(info.Items[0], embyProxyMediaSource{}) {
		return false, true, nil
	}
	remote, failure = classifyEmbyProxyItem(info.Items[0], embyProxyMediaSource{})
	return remote, false, failure
}
