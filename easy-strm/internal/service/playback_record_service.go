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
	UpdateCaller(context.Context, string, string) error
}

// playbackSession 记录一次播放会话：最近调用时间、首条记录 ID 与当前来源地址。
// 会话按「账号 + 文件」聚合，不按 IP 聚合：Emby 容器与真实播放器会分别解析同一 STRM，
// 若把 IP 纳入 key，两边永远无法合并，每次播放都会落两条记录。
type playbackSession struct {
	lastCall time.Time
	recordID string
	ip       string
}

// PlaybackRecordService 编排播放会话记录、重复解析合并及按需归属地补全。
type PlaybackRecordService struct {
	store          PlaybackRecordStore
	client         *http.Client
	now            func() time.Time
	sessionMu      sync.Mutex
	sessions       map[string]playbackSession
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
		sessions:    make(map[string]playbackSession),
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
	sessionKey := fmt.Sprintf("direct:%d:%s", account, identity)
	if session, merged := s.beginSessionCall(sessionKey); merged {
		s.refreshSessionCaller(sessionKey, session, ip)
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
	recordID, saved := s.save(playbackDisplayName(metadata), metadata.Poster, directURL, ip, method)
	if !saved {
		s.releaseSessionCall(sessionKey)
		return
	}
	s.bindSessionRecord(sessionKey, recordID, ip)
}

// RecordShare 记录分享资源库 STRM 播放会话，元数据按导出映射读取。
func (s *PlaybackRecordService) RecordShare(entryID, directURL, ip, method string) {
	sessionKey := "share:" + entryID
	if session, merged := s.beginSessionCall(sessionKey); merged {
		s.refreshSessionCaller(sessionKey, session, ip)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	metadata, err := s.store.ShareMetadata(ctx, entryID)
	if err != nil {
		logger.Warnf("分享STRM播放记录媒体信息读取失败: %v", err)
		if metadata.Title == "" {
			metadata.Title = entryID
		}
	}
	recordID, saved := s.save(playbackDisplayName(metadata), metadata.Poster, directURL, ip, method)
	if !saved {
		s.releaseSessionCall(sessionKey)
		return
	}
	s.bindSessionRecord(sessionKey, recordID, ip)
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

func (s *PlaybackRecordService) save(name, poster, directURL, ip, method string) (string, bool) {
	// 元数据只用于补全展示，超时后保存仍需独立预算，不能复用已失效的查询上下文。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	record := domain.PlaybackRecord{ID: uuid.NewString(), Name: name, Poster: poster, URL: directURL, IP: ip, Method: method, Time: s.now().UTC(), Location: "未知"}
	if strings.HasPrefix(record.Poster, "/") {
		record.Poster = "https://image.tmdb.org/t/p/w342" + record.Poster
	}
	if err := s.store.Save(ctx, record); err != nil {
		logger.Warnf("保存播放记录失败: %v", err)
		return "", false
	}
	return record.ID, true
}

// beginSessionCall 将同一来源对同一文件的连续解析合并为一次播放会话。
// 每次重复解析都会延长会话，只有静默超过五分钟后再次解析才新增记录。
// 返回既有会话时表示应合并，不再新增记录。
func (s *PlaybackRecordService) beginSessionCall(key string) (playbackSession, bool) {
	now := s.now()
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	for existingKey, session := range s.sessions {
		if now.Sub(session.lastCall) > playbackSessionIdleTimeout {
			delete(s.sessions, existingKey)
		}
	}
	if session, ok := s.sessions[key]; ok && now.Sub(session.lastCall) <= playbackSessionIdleTimeout {
		session.lastCall = now
		s.sessions[key] = session
		return session, true
	}
	return playbackSession{lastCall: now}, false
}

// bindSessionRecord 把新建的记录挂到会话上，后续重复解析据此合并。
func (s *PlaybackRecordService) bindSessionRecord(key, recordID, ip string) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	s.sessions[key] = playbackSession{lastCall: s.now(), recordID: recordID, ip: ip}
}

// refreshSessionCaller 合并时用更可信的来源地址更新首条记录：
// Emby 容器会先回源一次，真实播放器随后才到，展示应以真实客户端为准。
func (s *PlaybackRecordService) refreshSessionCaller(key string, session playbackSession, ip string) {
	if !preferCallerIP(ip, session.ip) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.store.UpdateCaller(ctx, session.recordID, ip); err != nil {
		logger.Warnf("更新播放记录来源失败: %v", err)
		return
	}
	s.sessionMu.Lock()
	if current, ok := s.sessions[key]; ok {
		current.ip = ip
		s.sessions[key] = current
	}
	s.sessionMu.Unlock()
}

func (s *PlaybackRecordService) releaseSessionCall(key string) {
	s.sessionMu.Lock()
	delete(s.sessions, key)
	s.sessionMu.Unlock()
}

// preferCallerIP 优先保留真实客户端地址；Docker 网桥地址（Emby 容器回源）不覆盖真实地址。
func preferCallerIP(next, current string) bool {
	if next == "" || next == current {
		return false
	}
	if current == "" {
		return true
	}
	return isDockerBridgeIP(current) && !isDockerBridgeIP(next)
}

func isDockerBridgeIP(value string) bool {
	ip := net.ParseIP(value)
	if ip == nil {
		return false
	}
	_, block, err := net.ParseCIDR("172.16.0.0/12")
	return err == nil && block.Contains(ip)
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
		s.completePosterContext(ctx, records[i])
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
