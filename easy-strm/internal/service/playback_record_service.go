package service

import (
	"context"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

const playbackSessionIdleTimeout = 5 * time.Minute

// PlaybackRecordStore 定义记录保存和查询所需的数据访问能力。
type PlaybackRecordStore interface {
	Save(context.Context, domain.PlaybackRecord) error
	List(context.Context) ([]domain.PlaybackRecord, error)
	Metadata(context.Context, int, string, string) (domain.PlaybackMetadata, error)
	ShareMetadata(context.Context, string) (domain.PlaybackMetadata, error)
	GetLocation(context.Context, string) string
	SetLocation(context.Context, string, string) error
	UpdatePoster(context.Context, string, string) error
}

// PlaybackRecordService 编排播放会话记录、重复解析合并及按需归属地补全。
type PlaybackRecordService struct {
	store          PlaybackRecordStore
	client         *http.Client
	now            func() time.Time
	sessionMu      sync.Mutex
	sessions       map[string]time.Time
	posterResolver func(context.Context, string) (string, error)
	posterSlots    chan struct{}
	posterRetry    map[string]time.Time
}

// NewPlaybackRecordService 创建播放记录服务。
func NewPlaybackRecordService(store PlaybackRecordStore) *PlaybackRecordService {
	return &PlaybackRecordService{
		store:       store,
		client:      &http.Client{Timeout: 2 * time.Second},
		now:         time.Now,
		sessions:    make(map[string]time.Time),
		posterSlots: make(chan struct{}, 2),
		posterRetry: make(map[string]time.Time),
	}
}

// Record 记录普通 STRM 播放会话；不在播放请求上执行外部网络查询。
func (s *PlaybackRecordService) Record(filePath, pickcode string, account int, directURL, ip, method string) {
	identity := pickcode
	if identity == "" {
		identity = strings.ToLower(strings.ReplaceAll(filePath, "\\", "/"))
	}
	sessionKey := fmt.Sprintf("direct:%d:%s:%s", account, identity, ip)
	if !s.beginSessionCall(sessionKey) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	name := path.Base(strings.ReplaceAll(filePath, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = pickcode
	}
	metadata, err := s.store.Metadata(ctx, account, pickcode, name)
	if err != nil {
		logger.Warnf("播放记录媒体信息读取失败: %v", err)
		metadata.Title = name
	}
	if !s.save(ctx, playbackDisplayName(metadata), metadata.Poster, directURL, ip, method) {
		s.releaseSessionCall(sessionKey)
	}
}

// RecordShare 记录分享资源库 STRM 播放会话，元数据按导出映射读取。
func (s *PlaybackRecordService) RecordShare(entryID, directURL, ip, method string) {
	sessionKey := "share:" + entryID + ":" + ip
	if !s.beginSessionCall(sessionKey) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	metadata, err := s.store.ShareMetadata(ctx, entryID)
	if err != nil {
		logger.Warnf("分享STRM播放记录媒体信息读取失败: %v", err)
		metadata.Title = entryID
	}
	if !s.save(ctx, playbackDisplayName(metadata), metadata.Poster, directURL, ip, method) {
		s.releaseSessionCall(sessionKey)
	}
}

// playbackDisplayName 使用确定的文件级映射展示季集，未知季集不猜测。
func playbackDisplayName(metadata domain.PlaybackMetadata) string {
	parts := []string{metadata.Title}
	seen := map[domain.ShareEpisode]bool{}
	for _, episode := range metadata.Episodes {
		if episode.SeasonNumber < 0 || episode.EpisodeNumber <= 0 || seen[episode] {
			continue
		}
		seen[episode] = true
		parts = append(parts, fmt.Sprintf("第 %d 季 · 第 %d 集", episode.SeasonNumber, episode.EpisodeNumber))
	}
	return strings.Join(parts, " · ")
}

func (s *PlaybackRecordService) save(ctx context.Context, name, poster, directURL, ip, method string) bool {
	record := domain.PlaybackRecord{ID: uuid.NewString(), Name: name, Poster: poster, URL: directURL, IP: ip, Method: method, Time: s.now().UTC(), Location: "未知"}
	if strings.HasPrefix(record.Poster, "/") {
		record.Poster = "https://image.tmdb.org/t/p/w342" + record.Poster
	}
	if err := s.store.Save(ctx, record); err != nil {
		logger.Warnf("保存播放记录失败: %v", err)
		return false
	}
	return true
}

// beginSessionCall 将同一来源对同一文件的连续解析合并为一次播放会话。
// 每次重复解析都会延长会话，只有静默超过五分钟后再次解析才新增记录。
func (s *PlaybackRecordService) beginSessionCall(key string) bool {
	now := s.now()
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	for existingKey, lastCall := range s.sessions {
		if now.Sub(lastCall) > playbackSessionIdleTimeout {
			delete(s.sessions, existingKey)
		}
	}
	if lastCall, ok := s.sessions[key]; ok && now.Sub(lastCall) <= playbackSessionIdleTimeout {
		s.sessions[key] = now
		return false
	}
	s.sessions[key] = now
	return true
}

func (s *PlaybackRecordService) releaseSessionCall(key string) {
	s.sessionMu.Lock()
	delete(s.sessions, key)
	s.sessionMu.Unlock()
}

// List 分页返回记录；归属地查询共享三秒预算，不阻塞 STRM 播放。
func (s *PlaybackRecordService) List(ctx context.Context, offset, limit int) ([]domain.PlaybackRecord, int, error) {
	records, err := s.store.List(ctx)
	if err != nil {
		return nil, 0, err
	}
	total := len(records)
	if offset >= total {
		return []domain.PlaybackRecord{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	records = records[offset:end]
	geoCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for i := range records {
		s.completePoster(records[i])
		records[i].Location = s.location(geoCtx, records[i].IP)
	}
	return records, total, nil
}

func (s *PlaybackRecordService) location(ctx context.Context, value string) string {
	ip := net.ParseIP(value)
	if ip == nil {
		return "未知"
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return "内网"
	}
	if cached := s.store.GetLocation(ctx, value); cached != "" {
		return cached
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/"+ip.String(), nil)
	if err != nil {
		return "未知"
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "未知"
	}
	defer resp.Body.Close()
	var result struct {
		Success bool   `json:"success"`
		Country string `json:"country"`
		Region  string `json:"region"`
		City    string `json:"city"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&result) != nil || !result.Success {
		return "未知"
	}
	location := strings.TrimSpace(fmt.Sprintf("%s %s %s", result.Country, result.Region, result.City))
	if location == "" {
		return "未知"
	}
	if err := s.store.SetLocation(ctx, value, location); err != nil {
		logger.Warnf("缓存播放归属地失败: %v", err)
	}
	return location
}
