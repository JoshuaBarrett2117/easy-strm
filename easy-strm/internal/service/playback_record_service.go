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
	"time"
)

// PlaybackRecordStore 定义记录保存和查询所需的数据访问能力。
type PlaybackRecordStore interface {
	Save(context.Context, domain.PlaybackRecord) error
	List(context.Context) ([]domain.PlaybackRecord, error)
	Metadata(context.Context, int, string, string) (string, string, error)
	ShareMetadata(context.Context, string) (string, string, error)
	GetLocation(context.Context, string) string
	SetLocation(context.Context, string, string) error
}

// PlaybackRecordService 编排调用记录及按需归属地补全。
type PlaybackRecordService struct {
	store  PlaybackRecordStore
	client *http.Client
}

// NewPlaybackRecordService 创建播放记录服务。
func NewPlaybackRecordService(store PlaybackRecordStore) *PlaybackRecordService {
	return &PlaybackRecordService{store: store, client: &http.Client{Timeout: 2 * time.Second}}
}

// Record 保存原始调用数据；不在播放请求上执行外部网络查询。
func (s *PlaybackRecordService) Record(filePath, pickcode string, account int, directURL, ip, method string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	name := path.Base(strings.ReplaceAll(filePath, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = pickcode
	}
	title, poster, err := s.store.Metadata(ctx, account, pickcode, name)
	if err != nil {
		logger.Warnf("播放记录媒体信息读取失败: %v", err)
		title = name
	}
	s.save(ctx, title, poster, directURL, ip, method)
}

// RecordShare 保存分享资源库 STRM 的成功解析记录，元数据按导出映射读取。
func (s *PlaybackRecordService) RecordShare(entryID, directURL, ip, method string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	name, poster, err := s.store.ShareMetadata(ctx, entryID)
	if err != nil {
		logger.Warnf("分享STRM播放记录媒体信息读取失败: %v", err)
		name = entryID
	}
	s.save(ctx, name, poster, directURL, ip, method)
}

func (s *PlaybackRecordService) save(ctx context.Context, name, poster, directURL, ip, method string) {
	record := domain.PlaybackRecord{ID: uuid.NewString(), Name: name, Poster: poster, URL: directURL, IP: ip, Method: method, Time: time.Now().UTC(), Location: "未知"}
	if strings.HasPrefix(record.Poster, "/") {
		record.Poster = "https://image.tmdb.org/t/p/w342" + record.Poster
	}
	if err := s.store.Save(ctx, record); err != nil {
		logger.Warnf("保存播放记录失败: %v", err)
	}
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
