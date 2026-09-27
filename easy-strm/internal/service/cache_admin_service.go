package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"easy-strm/internal/dao"

	"github.com/go-redis/redis/v8"
)

// CacheGroupOverview 表示单类缓存的概览信息。
type CacheGroupOverview struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Storage     string `json:"storage"`
	Description string `json:"description"`
	Prefix      string `json:"prefix"`
	Count       int64  `json:"count"`
	Status      string `json:"status"`
	StatusText  string `json:"status_text"`
	TTLText     string `json:"ttl_text,omitempty"`
}

// CacheOverview 表示缓存总览信息。
type CacheOverview struct {
	RedisConnected bool                 `json:"redis_connected"`
	RedisTotalKeys int64                `json:"redis_total_keys"`
	RedisMemory    string               `json:"redis_memory"`
	Groups         []CacheGroupOverview `json:"groups"`
}

type cacheGroupDefinition struct {
	Key           string
	Name          string
	Storage       string
	Description   string
	DBTable       string
	DBWhere       string
	RedisPatterns []string
	TTLText       string
	Enabled       bool
}

// CacheDetail 表示 Redis 缓存条目的调试信息。
type CacheDetail struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

// CacheDetailsPage 表示缓存详情分页结果。
type CacheDetailsPage struct {
	Items    []CacheDetail `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// CacheAdminService 提供缓存管理能力。
type CacheAdminService struct {
	db          *sql.DB
	redisClient *redis.Client
}

// NewCacheAdminService 创建缓存管理服务。
func NewCacheAdminService(db *sql.DB, redisClient *redis.Client) *CacheAdminService {
	if db == nil {
		db = dao.DB
	}
	if redisClient == nil {
		redisClient = dao.GetGlobalRedisClient()
	}

	return &CacheAdminService{
		db:          db,
		redisClient: redisClient,
	}
}

func (s *CacheAdminService) groupDefinitions() []cacheGroupDefinition {
	return []cacheGroupDefinition{
		{
			Key:         "tmdb_cache",
			Name:        "TMDB 查询缓存",
			Storage:     "postgres",
			Description: "保存识别命中的 TMDB 查询结果，供识别和刮削复用。",
			DBTable:     "t_tmdb_cache",
			DBWhere:     "expire_at > NOW()",
			RedisPatterns: []string{
				"easy_strm:tmdb:query:*",
				"easy_strm:tmdb:id:*",
				"easy_strm:tmdb:search:*",
				"easy_strm:tmdb:detail:*",
				"easy_strm:share:work:*",
			},
			Enabled: true,
		},
		{
			Key:           "identify_cache",
			Name:          "识别结果缓存",
			Storage:       "mixed",
			Description:   "保存文件识别结果，覆盖整理链路里的快速回填。",
			DBTable:       "t_identify_cache",
			RedisPatterns: []string{"identify:cache:*"},
			Enabled:       true,
		},
		{
			Key:         "media_file_cache",
			Name:        "刮削源数据缓存",
			Storage:     "postgres",
			Description: "保存文件级 TMDB 数据，减少重复识别和重复刮削构建。",
			DBTable:     "t_media_file_cache",
			RedisPatterns: []string{
				"easy_strm:media_file:cache:*",
			},
			Enabled: true,
		},
		{
			Key:           "account_quota_cache",
			Name:          "115账号容量缓存",
			Storage:       "redis",
			Description:   "保存115账号已用容量与总容量，五分钟内优先复用。",
			RedisPatterns: []string{"easy_strm:dashboard:account_quota:*"},
			TTLText:       "5 分钟",
			Enabled:       true,
		},
		{
			Key:           "task_cache",
			Name:          "任务状态缓存",
			Storage:       "redis",
			Description:   "保存任务状态、取消标记和恢复进度。",
			RedisPatterns: []string{"easy_strm:task:*"},
			Enabled:       true,
		},
		{
			Key:           "pickcode_cache",
			Name:          "直链 PickCode 缓存",
			Storage:       "redis",
			Description:   "保存路径到 PickCode 的映射，减少 115 查询。",
			RedisPatterns: []string{"easy_strm:pickcode:*"},
			TTLText:       "24 小时",
			Enabled:       true,
		},
		{
			Key:           "transfer_pickcode_cache",
			Name:          "转存 PickCode 缓存",
			Storage:       "redis",
			Description:   "保存转存链路使用的 PickCode，减少重复探测。",
			RedisPatterns: []string{"easy_strm:transfer_pickcode:*"},
			TTLText:       "7 天",
			Enabled:       true,
		},
		{
			Key:           "sha1_cache",
			Name:          "SHA1 秒传缓存",
			Storage:       "redis",
			Description:   "保存 SHA1 到云端目录映射，提升秒传判断速度。",
			RedisPatterns: []string{"easy_strm:sha1:cache:*"},
			TTLText:       "7 天",
			Enabled:       true,
		},
	}
}

// GetOverview 获取缓存概览。
func (s *CacheAdminService) GetOverview() (*CacheOverview, error) {
	overview := &CacheOverview{
		RedisMemory: "未连接",
		Groups:      make([]CacheGroupOverview, 0),
	}

	if s.redisClient != nil {
		ctx := context.Background()
		if pong, err := s.redisClient.Ping(ctx).Result(); err == nil && strings.EqualFold(pong, "PONG") {
			overview.RedisConnected = true
			if totalKeys, err := s.redisClient.DBSize(ctx).Result(); err == nil {
				overview.RedisTotalKeys = totalKeys
			}
			if memoryText, err := s.redisClient.Info(ctx, "memory").Result(); err == nil {
				overview.RedisMemory = parseRedisInfoValue(memoryText, "used_memory_human")
				if overview.RedisMemory == "" {
					overview.RedisMemory = parseRedisInfoValue(memoryText, "used_memory")
				}
			}
		}
	}

	for _, group := range s.groupDefinitions() {
		count, err := s.countGroup(group)
		if err != nil {
			return nil, err
		}
		status, statusText := cacheGroupStatus(group, count, overview.RedisConnected)
		overview.Groups = append(overview.Groups, CacheGroupOverview{
			Key:         group.Key,
			Name:        group.Name,
			Storage:     group.Storage,
			Description: group.Description,
			Prefix:      strings.Join(group.RedisPatterns, ", "),
			Count:       count,
			Status:      status,
			StatusText:  statusText,
			TTLText:     group.TTLText,
		})
	}

	return overview, nil
}

// GetDetails 查询指定缓存分组的 Redis 条目，支持关键词筛选和分页。
func (s *CacheAdminService) GetDetails(scope, keyword string, page, pageSize int) (*CacheDetailsPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	var group *cacheGroupDefinition
	for _, definition := range s.groupDefinitions() {
		if definition.Key == strings.TrimSpace(scope) {
			copy := definition
			group = &copy
			break
		}
	}
	if group == nil {
		return nil, fmt.Errorf("不支持的缓存范围: %s", scope)
	}
	result := &CacheDetailsPage{Items: []CacheDetail{}, Page: page, PageSize: pageSize}
	if s.redisClient == nil || len(group.RedisPatterns) == 0 {
		return result, nil
	}
	var keys []string
	for _, pattern := range group.RedisPatterns {
		batch, err := s.scanKeys(pattern)
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
	}
	seen := make(map[string]struct{}, len(keys))
	filtered := make([]string, 0, len(keys))
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	ctx := context.Background()
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		value, err := s.redisClient.Get(ctx, key).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("CacheAdminService[GetDetails] 读取 Redis key 失败: %v", err)
		}
		if keyword == "" || strings.Contains(strings.ToLower(key), keyword) || strings.Contains(strings.ToLower(value), keyword) {
			filtered = append(filtered, key)
		}
	}
	sort.Strings(filtered)
	result.Total = len(filtered)
	start := (page - 1) * pageSize
	if start >= len(filtered) {
		return result, nil
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	for _, key := range filtered[start:end] {
		value, _ := s.redisClient.Get(ctx, key).Result()
		ttl, _ := s.redisClient.TTL(ctx, key).Result()
		result.Items = append(result.Items, CacheDetail{Key: key, Value: value, TTLSeconds: int64(ttl / time.Second)})
	}
	return result, nil
}

func cacheGroupStatus(group cacheGroupDefinition, count int64, redisConnected bool) (string, string) {
	if !group.Enabled {
		return "disabled", "未启用"
	}
	if count > 0 {
		return "active", "已启用"
	}
	if (group.Storage == "redis" || group.Storage == "mixed") && !redisConnected {
		return "unavailable", "Redis 不可用"
	}
	return "empty", "暂无有效缓存"
}

// ClearGroup 清理指定缓存分组。
func (s *CacheAdminService) ClearGroup(scope string) (int64, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return 0, fmt.Errorf("清理范围不能为空")
	}

	if scope == "all" {
		var deleted int64
		for _, group := range s.groupDefinitions() {
			currentDeleted, err := s.clearGroup(group)
			if err != nil {
				return deleted, err
			}
			deleted += currentDeleted
		}
		return deleted, nil
	}

	for _, group := range s.groupDefinitions() {
		if group.Key == scope {
			return s.clearGroup(group)
		}
	}

	return 0, fmt.Errorf("不支持的缓存范围: %s", scope)
}

func (s *CacheAdminService) countGroup(group cacheGroupDefinition) (int64, error) {
	var total int64

	if group.DBTable != "" {
		dbCount, err := s.countTableRows(group.DBTable, group.DBWhere)
		if err != nil {
			return 0, err
		}
		total += dbCount
	}

	if len(group.RedisPatterns) > 0 && s.redisClient != nil {
		redisCount, err := s.countRedisPatterns(group.RedisPatterns)
		if err != nil {
			return 0, err
		}
		total += redisCount
	}

	return total, nil
}

func (s *CacheAdminService) clearGroup(group cacheGroupDefinition) (int64, error) {
	var total int64

	if group.DBTable != "" {
		deleted, err := s.deleteTableRows(group.DBTable, group.DBWhere)
		if err != nil {
			return 0, err
		}
		total += deleted
	}

	if len(group.RedisPatterns) > 0 && s.redisClient != nil {
		deleted, err := s.deleteRedisPatterns(group.RedisPatterns)
		if err != nil {
			return 0, err
		}
		total += deleted
	}

	return total, nil
}

func (s *CacheAdminService) countTableRows(tableName, whereClause string) (int64, error) {
	if s.db == nil {
		return 0, nil
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	if strings.TrimSpace(whereClause) != "" {
		query += " WHERE " + whereClause
	}

	var count int64
	if err := s.db.QueryRow(query).Scan(&count); err != nil {
		return 0, fmt.Errorf("CacheAdminService[countTableRows] 查询 %s 失败: %v", tableName, err)
	}
	return count, nil
}

func (s *CacheAdminService) deleteTableRows(tableName, whereClause string) (int64, error) {
	if s.db == nil {
		return 0, nil
	}

	query := fmt.Sprintf("DELETE FROM %s", tableName)
	if strings.TrimSpace(whereClause) != "" {
		query += " WHERE " + whereClause
	}

	result, err := s.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("CacheAdminService[deleteTableRows] 清理 %s 失败: %v", tableName, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("CacheAdminService[deleteTableRows] 读取 %s 删除数量失败: %v", tableName, err)
	}

	return rowsAffected, nil
}

func (s *CacheAdminService) countRedisPatterns(patterns []string) (int64, error) {
	var total int64
	for _, pattern := range patterns {
		keys, err := s.scanKeys(pattern)
		if err != nil {
			return 0, err
		}
		total += int64(len(keys))
	}
	return total, nil
}

func (s *CacheAdminService) deleteRedisPatterns(patterns []string) (int64, error) {
	var total int64
	ctx := context.Background()

	for _, pattern := range patterns {
		keys, err := s.scanKeys(pattern)
		if err != nil {
			return 0, err
		}
		if len(keys) == 0 {
			continue
		}
		deleted, err := s.redisClient.Del(ctx, keys...).Result()
		if err != nil {
			return 0, fmt.Errorf("CacheAdminService[deleteRedisPatterns] 删除 Redis key 失败: %v", err)
		}
		total += deleted
	}

	return total, nil
}

func (s *CacheAdminService) scanKeys(pattern string) ([]string, error) {
	if s.redisClient == nil {
		return nil, nil
	}

	ctx := context.Background()
	var (
		cursor uint64
		keys   []string
	)

	for {
		batch, nextCursor, err := s.redisClient.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return nil, fmt.Errorf("CacheAdminService[scanKeys] 扫描 Redis key 失败: %v", err)
		}
		keys = append(keys, batch...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return keys, nil
}

func parseRedisInfoValue(infoText, key string) string {
	for _, line := range strings.Split(infoText, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, key+":"))
	}
	return ""
}
