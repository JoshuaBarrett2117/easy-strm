package service

import (
	"context"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"net/http"
	"path"
	"strings"
	"time"
)

// SetPosterResolver 注入缺失电影海报的后台识别能力，仅在服务初始化时调用。
func (s *PlaybackRecordService) SetPosterResolver(resolve func(context.Context, string) (string, error)) {
	s.posterResolver = resolve
}

func (s *PlaybackRecordService) completePoster(record domain.PlaybackRecord) {
	if record.Poster != "" || s.posterResolver == nil {
		return
	}
	s.sessionMu.Lock()
	now := s.now()
	for id, until := range s.posterRetry {
		if now.After(until) {
			delete(s.posterRetry, id)
		}
	}
	if _, ok := s.posterRetry[record.ID]; ok {
		s.sessionMu.Unlock()
		return
	}
	select {
	case s.posterSlots <- struct{}{}:
	default:
		s.sessionMu.Unlock()
		return
	}
	s.posterRetry[record.ID] = now.Add(10 * time.Minute)
	s.sessionMu.Unlock()
	go func() {
		defer func() { <-s.posterSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		poster, err := s.posterResolver(ctx, record.Name)
		if err == nil && poster != "" {
			if strings.HasPrefix(poster, "/") {
				poster = "https://image.tmdb.org/t/p/w342" + poster
			}
			err = s.store.UpdatePoster(ctx, record.ID, poster)
		}
		if err != nil {
			logger.Warnf("播放记录海报补全失败，record_id=%s: %v", record.ID, err)
		}
	}()
}

type playbackContextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t playbackContextTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req.Clone(t.ctx))
}

// ResolvePlaybackMoviePoster 复用文件名解析和 TMDB 搜索，只接受年份及片名均匹配的唯一电影。
// 使用独立请求客户端传递超时，不改动共享 TMDB 服务或在播放入口执行网络请求。
func (s *TmdbService) ResolvePlaybackMoviePoster(ctx context.Context, filename string) (string, error) {
	switch strings.ToLower(path.Ext(filename)) {
	case ".mkv", ".mp4", ".avi", ".mov", ".m2ts", ".ts", ".iso":
	default:
		return "", nil
	}
	parsed := s.parseFilename(filename)
	if parsed.MediaType != "movie" || parsed.Year <= 0 || parsed.Title == "" {
		return "", nil
	}
	scoped := NewTmdbService(s.apiKey, nil)
	scoped.baseURL, scoped.imageBaseURL, scoped.language = s.baseURL, s.imageBaseURL, s.language
	transport := s.httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	scoped.httpClient = &http.Client{Transport: playbackContextTransport{ctx: ctx, base: transport}, Timeout: 10 * time.Second}
	candidates, err := scoped.searchCandidatesWithFallback(parsed, domain.MetadataSourceTMDB)
	if err != nil {
		return "", err
	}
	normalized := strings.ToLower(compactSearchTitle(parsed.Title))
	matches := map[int]string{}
	for _, candidate := range candidates {
		if candidate.Year != parsed.Year || candidate.PosterPath == "" || candidate.TmdbID <= 0 {
			continue
		}
		for _, title := range []string{candidate.Title, candidate.OriginalTitle} {
			title = strings.ToLower(compactSearchTitle(title))
			if title != "" && (title == normalized || (len([]rune(title)) >= 6 && strings.Contains(normalized, title))) {
				matches[candidate.TmdbID] = candidate.PosterPath
			}
		}
	}
	if len(matches) == 1 {
		for _, poster := range matches {
			return poster, nil
		}
	}
	return "", nil
}
