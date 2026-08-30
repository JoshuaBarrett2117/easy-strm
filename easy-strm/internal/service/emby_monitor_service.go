package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	"github.com/go-redis/redis/v8"
)

const (
	embyMonitorSourcePlugin = "playback_reporting"
	embyMonitorSourceLocal  = "local_collector"
)

// EmbyMonitorService 聚合 Emby 实时会话、播放历史和最近入库数据。
type EmbyMonitorService struct {
	servers *dao.EmbyServerDAO
	monitor *dao.EmbyMonitorDAO
	http    *http.Client
	redis   *redis.Client
	now     func() time.Time
}

// NewEmbyMonitorService 创建 Emby 观影监控服务。
func NewEmbyMonitorService(servers *dao.EmbyServerDAO, monitor *dao.EmbyMonitorDAO, httpClient *http.Client, redisClient *redis.Client) *EmbyMonitorService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &EmbyMonitorService{servers: servers, monitor: monitor, http: httpClient, redis: redisClient, now: time.Now}
}

type embySessionResponse struct {
	ID             string `json:"Id"`
	UserID         string `json:"UserId"`
	UserName       string `json:"UserName"`
	Client         string `json:"Client"`
	DeviceName     string `json:"DeviceName"`
	NowPlayingItem *struct {
		ID           string            `json:"Id"`
		Name         string            `json:"Name"`
		Type         string            `json:"Type"`
		SeriesID     string            `json:"SeriesId"`
		SeriesName   string            `json:"SeriesName"`
		RunTimeTicks int64             `json:"RunTimeTicks"`
		ImageTags    map[string]string `json:"ImageTags"`
	} `json:"NowPlayingItem"`
	PlayState struct {
		IsPaused      bool   `json:"IsPaused"`
		PositionTicks int64  `json:"PositionTicks"`
		PlayMethod    string `json:"PlayMethod"`
	} `json:"PlayState"`
}

type embyItemsResponse struct {
	Items []struct {
		ID             string            `json:"Id"`
		Name           string            `json:"Name"`
		Type           string            `json:"Type"`
		SeriesName     string            `json:"SeriesName"`
		ProductionYear int               `json:"ProductionYear"`
		DateCreated    time.Time         `json:"DateCreated"`
		ImageTags      map[string]string `json:"ImageTags"`
	} `json:"Items"`
	TotalRecordCount int `json:"TotalRecordCount"`
}

type pluginPlaybackRow struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	Name     string `json:"name"`
	ItemID   string `json:"item_id"`
	Type     string `json:"type"`
	Duration int64  `json:"duration"`
}

type pluginUsageRow struct {
	UserID    string           `json:"user_id"`
	UserName  string           `json:"user_name"`
	UserUsage map[string]int64 `json:"user_usage"`
}

type pluginOverviewHistory struct {
	Summary domain.EmbyMonitorSummary `json:"summary"`
	StartAt *time.Time                `json:"start_at,omitempty"`
}

func (s *EmbyMonitorService) requireServer(id int) (*domain.EmbyServer, error) {
	server, err := s.servers.GetByID(id)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, fmt.Errorf("Emby 实例不存在")
	}
	if !server.Enabled {
		return nil, fmt.Errorf("Emby 实例已停用")
	}
	return server, nil
}

func (s *EmbyMonitorService) requestJSON(server *domain.EmbyServer, path string, query url.Values, output interface{}) error {
	endpoint := strings.TrimRight(server.BaseURL, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Emby-Token", server.APIKey)
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("连接 Emby 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("Emby 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err = json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("解析 Emby 响应失败: %w", err)
	}
	return nil
}

func (s *EmbyMonitorService) sessions(server *domain.EmbyServer) ([]embySessionResponse, error) {
	var data []embySessionResponse
	err := s.requestJSON(server, "/emby/Sessions", url.Values{"ActiveWithinSeconds": {"120"}}, &data)
	return data, err
}

func normalizeMonitorMediaType(value string) string {
	if strings.EqualFold(value, "Episode") || strings.EqualFold(value, "Series") {
		return "series"
	}
	return "movie"
}

// CollectSessions 执行一次实时会话采集。
func (s *EmbyMonitorService) CollectSessions(serverID int) error {
	server, err := s.requireServer(serverID)
	if err != nil {
		return err
	}
	sessions, err := s.sessions(server)
	now := s.now()
	if err != nil {
		_ = s.monitor.UpdateState(serverID, nil, nil, false, false, err.Error())
		return err
	}
	active := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session.NowPlayingItem == nil || session.NowPlayingItem.ID == "" {
			continue
		}
		active = append(active, session.ID)
		image := session.NowPlayingItem.ImageTags["Primary"]
		_, touchErr := s.monitor.TouchPlayback(domain.EmbyPlaybackSample{ServerID: serverID, SessionID: session.ID, UserID: session.UserID, UserName: session.UserName, ItemID: session.NowPlayingItem.ID, ItemType: session.NowPlayingItem.Type, ItemName: session.NowPlayingItem.Name, SeriesID: session.NowPlayingItem.SeriesID, SeriesName: session.NowPlayingItem.SeriesName, ImageTag: image, ClientName: session.Client, DeviceName: session.DeviceName, PlaybackMethod: session.PlayState.PlayMethod, Paused: session.PlayState.IsPaused}, now, 45*time.Second)
		if touchErr != nil {
			return touchErr
		}
	}
	if err = s.monitor.CloseMissingSessions(serverID, active, now.Add(-90*time.Second), now); err != nil {
		return err
	}
	return s.monitor.UpdateState(serverID, &now, nil, false, false, "")
}

// CollectItems 执行一次媒体首次发现同步。
func (s *EmbyMonitorService) CollectItems(serverID int) error {
	server, err := s.requireServer(serverID)
	if err != nil {
		return err
	}
	state, err := s.monitor.GetState(serverID)
	if err != nil {
		return err
	}
	baseline := state.BaselineCompletedAt == nil
	start := 0
	now := s.now()
	all := []domain.EmbyMonitorItem{}
	for {
		var page embyItemsResponse
		query := url.Values{"Recursive": {"true"}, "IncludeItemTypes": {"Movie,Series"}, "Fields": {"DateCreated,ProductionYear,ImageTags"}, "SortBy": {"DateCreated"}, "SortOrder": {"Descending"}, "StartIndex": {strconv.Itoa(start)}, "Limit": {"500"}}
		if err = s.requestJSON(server, "/emby/Items", query, &page); err != nil {
			_ = s.monitor.UpdateState(serverID, nil, nil, false, false, err.Error())
			return err
		}
		for _, item := range page.Items {
			all = append(all, domain.EmbyMonitorItem{ServerID: serverID, ItemID: item.ID, ItemType: item.Type, Name: item.Name, SeriesName: item.SeriesName, Year: item.ProductionYear, ImageTag: item.ImageTags["Primary"], DateCreated: item.DateCreated})
		}
		start += len(page.Items)
		if len(page.Items) == 0 || start >= page.TotalRecordCount {
			break
		}
	}
	if err = s.monitor.UpsertItems(serverID, all, baseline, now); err != nil {
		return err
	}
	return s.monitor.UpdateState(serverID, nil, &now, true, false, "")
}

func timeBounds(period string, now time.Time) (time.Time, time.Time, error) {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	end := now.Add(time.Nanosecond)
	switch period {
	case "day", "today":
		return today, end, nil
	case "yesterday":
		return today.AddDate(0, 0, -1), today, nil
	case "week":
		weekday := (int(today.Weekday()) + 6) % 7
		return today.AddDate(0, 0, -weekday), end, nil
	case "month":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc), end, nil
	case "total":
		return time.Unix(0, 0).In(loc), end, nil
	case "7d":
		return today.AddDate(0, 0, -6), end, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("不支持的时间范围")
	}
}

func (s *EmbyMonitorService) meta(source string, history *time.Time, degraded string) domain.EmbyMonitorMeta {
	return domain.EmbyMonitorMeta{Source: source, GeneratedAt: s.now(), Timezone: s.now().Location().String(), HistoryStartAt: history, DegradedReason: degraded}
}

func (s *EmbyMonitorService) cacheGet(key string, output interface{}) bool {
	if s.redis == nil {
		return false
	}
	data, err := s.redis.Get(context.Background(), key).Bytes()
	return err == nil && json.Unmarshal(data, output) == nil
}

func (s *EmbyMonitorService) cacheSet(key string, value interface{}) {
	if s.redis == nil {
		return
	}
	data, err := json.Marshal(value)
	if err == nil {
		_ = s.redis.Set(context.Background(), key, data, time.Minute).Err()
	}
}

func finalizeRanking(items []domain.EmbyMonitorRankingItem) []domain.EmbyMonitorRankingItem {
	sort.SliceStable(items, func(i, j int) bool { return items[i].WatchedSeconds > items[j].WatchedSeconds })
	var top int64
	if len(items) > 0 {
		top = items[0].WatchedSeconds
	}
	for i := range items {
		items[i].Rank = i + 1
		if top > 0 {
			items[i].Percentage = float64(items[i].WatchedSeconds) * 100 / float64(top)
		}
	}
	return items
}

func (s *EmbyMonitorService) pluginAvailable(server *domain.EmbyServer) error {
	var data interface{}
	err := s.requestJSON(server, "/emby/user_usage_stats/user_list", nil, &data)
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	_ = s.monitor.UpdateState(server.ID, nil, nil, false, err == nil, reason)
	return err
}

func daysFor(start, end time.Time) int {
	days := int(end.Sub(start).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	if days > 36500 {
		days = 36500
	}
	return days
}

func (s *EmbyMonitorService) pluginRows(server *domain.EmbyServer, start, end time.Time) ([]pluginPlaybackRow, error) {
	var rows []pluginPlaybackRow
	q := url.Values{"days": {strconv.Itoa(daysFor(start, end))}, "end_date": {end.Format("2006-01-02")}, "aggregate_data": {"false"}}
	if err := s.requestJSON(server, "/emby/user_usage_stats/UserPlaylist", q, &rows); err != nil {
		return nil, err
	}
	filtered := rows[:0]
	for _, row := range rows {
		at, err := time.ParseInLocation("2006-01-02 15:04:05", row.Date+" "+row.Time, s.now().Location())
		if err == nil && at.Before(end) && !at.Before(start) {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func (s *EmbyMonitorService) pluginOverview(server *domain.EmbyServer, today, week, month time.Time) (pluginOverviewHistory, error) {
	cacheKey := fmt.Sprintf("easy_strm:emby_monitor:plugin_overview:%d", server.ID)
	cached := pluginOverviewHistory{}
	if s.cacheGet(cacheKey, &cached) {
		return cached, nil
	}
	var rows []pluginUsageRow
	query := url.Values{"days": {"36500"}, "end_date": {s.now().Format("2006-01-02")}, "filter": {"Movie,Episode"}, "data_type": {"time"}}
	if err := s.requestJSON(server, "/emby/user_usage_stats/PlayActivity", query, &rows); err != nil {
		return cached, err
	}
	result := pluginOverviewHistory{}
	todayUsers, weekUsers, monthUsers := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		if row.UserID == "labels_user" {
			continue
		}
		for date, seconds := range row.UserUsage {
			at, err := time.ParseInLocation("2006-01-02", date, s.now().Location())
			if err != nil {
				continue
			}
			if result.StartAt == nil || at.Before(*result.StartAt) {
				value := at
				result.StartAt = &value
			}
			result.Summary.TotalSeconds += seconds
			if !at.Before(month) {
				result.Summary.MonthSeconds += seconds
				monthUsers[row.UserID] = true
			}
			if !at.Before(week) {
				result.Summary.WeekSeconds += seconds
				weekUsers[row.UserID] = true
			}
			if !at.Before(today) {
				result.Summary.TodaySeconds += seconds
				todayUsers[row.UserID] = true
			}
		}
	}
	result.Summary.TodayActiveUsers = len(todayUsers)
	result.Summary.WeekActiveUsers = len(weekUsers)
	result.Summary.MonthActiveUsers = len(monthUsers)
	s.cacheSet(cacheKey, result)
	return result, nil
}

func rowsRanking(rows []pluginPlaybackRow, dimension, mediaType string) []domain.EmbyMonitorRankingItem {
	items := map[string]*domain.EmbyMonitorRankingItem{}
	for _, row := range rows {
		if dimension == "media" && normalizeMonitorMediaType(row.Type) != mediaType {
			continue
		}
		key, name, subtitle := row.UserID, row.UserName, ""
		if dimension == "media" {
			key, name, subtitle = row.ItemID, row.Name, row.Type
		}
		item := items[key]
		if item == nil {
			item = &domain.EmbyMonitorRankingItem{ID: key, Name: name, Subtitle: subtitle}
			items[key] = item
		}
		item.WatchedSeconds += row.Duration
		item.PlayCount++
	}
	result := make([]domain.EmbyMonitorRankingItem, 0, len(items))
	for _, item := range items {
		result = append(result, *item)
	}
	return finalizeRanking(result)
}

func (s *EmbyMonitorService) pluginBreakdown(server *domain.EmbyServer, dimension string, start, end time.Time) ([]domain.EmbyMonitorRankingItem, error) {
	kind := "ClientName"
	if dimension == "users" {
		kind = "UserId"
	}
	var rows []struct {
		Label string `json:"label"`
		Count int    `json:"count"`
		Time  int64  `json:"time"`
	}
	q := url.Values{"days": {strconv.Itoa(daysFor(start, end))}, "end_date": {end.Format("2006-01-02")}}
	if err := s.requestJSON(server, "/emby/user_usage_stats/"+kind+"/BreakdownReport", q, &rows); err != nil {
		return nil, err
	}
	result := make([]domain.EmbyMonitorRankingItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.EmbyMonitorRankingItem{ID: row.Label, Name: row.Label, WatchedSeconds: row.Time, PlayCount: row.Count})
	}
	return finalizeRanking(result), nil
}

func (s *EmbyMonitorService) pluginMediaReport(server *domain.EmbyServer, mediaType string, start, end time.Time) ([]domain.EmbyMonitorRankingItem, error) {
	path := "/emby/user_usage_stats/MoviesReport"
	if mediaType == "series" {
		path = "/emby/user_usage_stats/TvShowsReport"
	}
	var rows []struct {
		Label string `json:"label"`
		Count int    `json:"count"`
		Time  int64  `json:"time"`
	}
	query := url.Values{"days": {strconv.Itoa(daysFor(start, end))}, "end_date": {end.Format("2006-01-02")}}
	if err := s.requestJSON(server, path, query, &rows); err != nil {
		return nil, err
	}
	result := make([]domain.EmbyMonitorRankingItem, 0, len(rows))
	subtitle := "电影"
	if mediaType == "series" {
		subtitle = "剧集"
	}
	for _, row := range rows {
		result = append(result, domain.EmbyMonitorRankingItem{Name: row.Label, Subtitle: subtitle, WatchedSeconds: row.Time, PlayCount: row.Count})
	}
	return finalizeRanking(result), nil
}

// GetOverview 返回汇总、趋势和实时会话。
func (s *EmbyMonitorService) GetOverview(serverID int) (*domain.EmbyMonitorOverview, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	today, _, _ := timeBounds("day", now)
	week, _, _ := timeBounds("week", now)
	month, _, _ := timeBounds("month", now)
	trendStart, trendEnd, _ := timeBounds("7d", now)
	history, _ := s.monitor.HistoryStart(serverID)
	source := embyMonitorSourceLocal
	degraded := ""
	summary, err := s.monitor.Summary(serverID, today, week, month)
	if err != nil {
		return nil, err
	}
	trend, err := s.monitor.Trend(serverID, trendStart, trendEnd)
	if err != nil {
		return nil, err
	}
	if pluginErr := s.pluginAvailable(server); pluginErr == nil {
		source = embyMonitorSourcePlugin
		pluginHistory, historyErr := s.pluginOverview(server, today, week, month)
		rows, rowErr := s.pluginRows(server, trendStart, trendEnd)
		if historyErr == nil && rowErr == nil {
			summary = pluginHistory.Summary
			history = pluginHistory.StartAt
			trend = trendFromRows(rows, trendStart, trendEnd, now.Location())
		} else {
			source = embyMonitorSourceLocal
			if historyErr != nil {
				degraded = historyErr.Error()
			} else {
				degraded = rowErr.Error()
			}
		}
	} else {
		degraded = pluginErr.Error()
	}
	sessionsRaw, sessionErr := s.sessions(server)
	sessions := []domain.EmbyMonitorSession{}
	if sessionErr == nil {
		for _, session := range sessionsRaw {
			if session.NowPlayingItem == nil {
				continue
			}
			progress := float64(0)
			if session.NowPlayingItem.RunTimeTicks > 0 {
				progress = float64(session.PlayState.PositionTicks) * 100 / float64(session.NowPlayingItem.RunTimeTicks)
			}
			sessions = append(sessions, domain.EmbyMonitorSession{SessionID: session.ID, UserID: session.UserID, UserName: session.UserName, ItemID: session.NowPlayingItem.ID, ItemName: session.NowPlayingItem.Name, SeriesName: session.NowPlayingItem.SeriesName, ImageTag: session.NowPlayingItem.ImageTags["Primary"], ClientName: session.Client, DeviceName: session.DeviceName, PlaybackMethod: session.PlayState.PlayMethod, Paused: session.PlayState.IsPaused, Progress: progress})
		}
	}
	peak := 0
	for _, p := range trend {
		if p.ActiveUsers > peak {
			peak = p.ActiveUsers
		}
	}
	summary.PeakUsers = peak
	return &domain.EmbyMonitorOverview{Meta: s.meta(source, history, degraded), Summary: summary, Trend: trend, Sessions: sessions}, nil
}

func summaryFromRows(rows []pluginPlaybackRow, today, week, month time.Time) domain.EmbyMonitorSummary {
	r := domain.EmbyMonitorSummary{}
	tu, wu, mu := map[string]bool{}, map[string]bool{}, map[string]bool{}
	loc := today.Location()
	for _, row := range rows {
		at, e := time.ParseInLocation("2006-01-02 15:04:05", row.Date+" "+row.Time, loc)
		if e != nil {
			continue
		}
		r.TotalSeconds += row.Duration
		if !at.Before(month) {
			r.MonthSeconds += row.Duration
			mu[row.UserID] = true
		}
		if !at.Before(week) {
			r.WeekSeconds += row.Duration
			wu[row.UserID] = true
		}
		if !at.Before(today) {
			r.TodaySeconds += row.Duration
			tu[row.UserID] = true
		}
	}
	r.TodayActiveUsers = len(tu)
	r.WeekActiveUsers = len(wu)
	r.MonthActiveUsers = len(mu)
	return r
}

func trendFromRows(rows []pluginPlaybackRow, start, end time.Time, loc *time.Location) []domain.EmbyMonitorTrendPoint {
	type bucket struct {
		users   map[string]bool
		seconds int64
	}
	data := map[time.Time]*bucket{}
	for _, row := range rows {
		at, e := time.ParseInLocation("2006-01-02 15:04:05", row.Date+" "+row.Time, loc)
		if e != nil || at.Before(start) || !at.Before(end) {
			continue
		}
		splitPlaybackHours(at, row.Duration, func(hour time.Time, seconds int64) {
			if hour.Before(start) || !hour.Before(end) {
				return
			}
			b := data[hour]
			if b == nil {
				b = &bucket{users: map[string]bool{}}
				data[hour] = b
			}
			b.users[row.UserID] = true
			b.seconds += seconds
		})
	}
	result := []domain.EmbyMonitorTrendPoint{}
	for at, b := range data {
		result = append(result, domain.EmbyMonitorTrendPoint{Time: at, ActiveUsers: len(b.users), WatchedSeconds: b.seconds})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Time.Before(result[j].Time) })
	return result
}

func splitPlaybackHours(at time.Time, duration int64, add func(time.Time, int64)) {
	remaining := duration
	for remaining > 0 {
		hour := at.Truncate(time.Hour)
		seconds := int64(hour.Add(time.Hour).Sub(at) / time.Second)
		if seconds <= 0 || seconds > remaining {
			seconds = remaining
		}
		add(hour, seconds)
		remaining -= seconds
		at = at.Add(time.Duration(seconds) * time.Second)
	}
}

// GetRankings 返回用户、媒体或客户端排行。
func (s *EmbyMonitorService) GetRankings(serverID int, dimension, period, mediaType string) (*domain.EmbyMonitorRankingResponse, error) {
	if dimension != "users" && dimension != "media" && dimension != "clients" {
		return nil, fmt.Errorf("不支持的排行维度")
	}
	if dimension == "media" && mediaType != "movie" && mediaType != "series" {
		return nil, fmt.Errorf("不支持的媒体类型")
	}
	if dimension != "media" {
		mediaType = "movie"
	}
	start, end, err := timeBounds(period, s.now())
	if err != nil {
		return nil, err
	}
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	cacheKey := fmt.Sprintf("easy_strm:emby_monitor:ranking:%d:%s:%s:%s", serverID, dimension, period, mediaType)
	cached := &domain.EmbyMonitorRankingResponse{}
	if s.cacheGet(cacheKey, cached) {
		return cached, nil
	}
	history, _ := s.monitor.HistoryStart(serverID)
	source := embyMonitorSourceLocal
	degraded := ""
	var data []domain.EmbyMonitorRankingItem
	if pluginErr := s.pluginAvailable(server); pluginErr == nil {
		if dimension == "users" {
			rows, rowErr := s.pluginRows(server, start, end)
			if rowErr == nil {
				data = rowsRanking(rows, dimension, mediaType)
			} else {
				degraded = rowErr.Error()
			}
		} else if dimension == "media" {
			data, err = s.pluginMediaReport(server, mediaType, start, end)
			if err != nil {
				degraded = err.Error()
			}
		} else {
			data, err = s.pluginBreakdown(server, dimension, start, end)
			if err != nil {
				degraded = err.Error()
			}
		}
		if degraded == "" {
			source = embyMonitorSourcePlugin
		}
	} else {
		degraded = pluginErr.Error()
	}
	if source == embyMonitorSourceLocal {
		data, err = s.monitor.Rankings(serverID, dimension, mediaType, start, end)
		if err != nil {
			return nil, err
		}
		data = finalizeRanking(data)
	}
	result := &domain.EmbyMonitorRankingResponse{Meta: s.meta(source, history, degraded), Data: data, Total: len(data)}
	s.cacheSet(cacheKey, result)
	return result, nil
}

// GetHeatmap 返回用户小时热力图。
func (s *EmbyMonitorService) GetHeatmap(serverID int, rangeName, userID string) (*domain.EmbyMonitorHeatmapResponse, error) {
	start, end, err := timeBounds(rangeName, s.now())
	if err != nil {
		return nil, err
	}
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	cacheKey := fmt.Sprintf("easy_strm:emby_monitor:heatmap:%d:%s:%s", serverID, rangeName, userID)
	cached := &domain.EmbyMonitorHeatmapResponse{}
	if s.cacheGet(cacheKey, cached) {
		return cached, nil
	}
	history, _ := s.monitor.HistoryStart(serverID)
	source := embyMonitorSourceLocal
	degraded := ""
	var users []domain.EmbyMonitorUserHeatmap
	if pluginErr := s.pluginAvailable(server); pluginErr == nil {
		rows, rowErr := s.pluginRows(server, start, end)
		if rowErr == nil {
			users = heatmapFromRows(rows, userID, s.now().Location())
			source = embyMonitorSourcePlugin
		} else {
			degraded = rowErr.Error()
		}
	} else {
		degraded = pluginErr.Error()
	}
	if source == embyMonitorSourceLocal {
		users, err = s.monitor.Heatmap(serverID, userID, start, end)
		if err != nil {
			return nil, err
		}
	}
	result := &domain.EmbyMonitorHeatmapResponse{Meta: s.meta(source, history, degraded), Users: users}
	s.cacheSet(cacheKey, result)
	return result, nil
}

func heatmapFromRows(rows []pluginPlaybackRow, userID string, loc *time.Location) []domain.EmbyMonitorUserHeatmap {
	by := map[string]*domain.EmbyMonitorUserHeatmap{}
	cells := map[string]map[string]int64{}
	for _, row := range rows {
		if userID != "" && row.UserID != userID {
			continue
		}
		at, e := time.ParseInLocation("2006-01-02 15:04:05", row.Date+" "+row.Time, loc)
		if e != nil {
			continue
		}
		u := by[row.UserID]
		if u == nil {
			u = &domain.EmbyMonitorUserHeatmap{UserID: row.UserID, UserName: row.UserName}
			by[row.UserID] = u
			cells[row.UserID] = map[string]int64{}
		}
		u.TotalSeconds += row.Duration
		splitPlaybackHours(at, row.Duration, func(hour time.Time, seconds int64) {
			key := fmt.Sprintf("%s|%02d", hour.Format("2006-01-02"), hour.Hour())
			cells[row.UserID][key] += seconds
		})
	}
	result := []domain.EmbyMonitorUserHeatmap{}
	for id, u := range by {
		for key, seconds := range cells[id] {
			parts := strings.Split(key, "|")
			hour, _ := strconv.Atoi(parts[1])
			u.Cells = append(u.Cells, domain.EmbyMonitorHeatmapCell{Date: parts[0], Hour: hour, Seconds: seconds})
		}
		sort.Slice(u.Cells, func(i, j int) bool {
			if u.Cells[i].Date == u.Cells[j].Date {
				return u.Cells[i].Hour < u.Cells[j].Hour
			}
			return u.Cells[i].Date < u.Cells[j].Date
		})
		result = append(result, *u)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TotalSeconds > result[j].TotalSeconds })
	return result
}

// GetRecentItems 返回最近入库媒体。
func (s *EmbyMonitorService) GetRecentItems(serverID int, timeBasis, rangeName, mediaType string) (*domain.EmbyMonitorRecentResponse, error) {
	if timeBasis != "emby" && timeBasis != "first_seen" {
		return nil, fmt.Errorf("不支持的时间依据")
	}
	if mediaType != "movie" && mediaType != "series" {
		return nil, fmt.Errorf("不支持的媒体类型")
	}
	start, end, err := timeBounds(rangeName, s.now())
	if err != nil {
		return nil, err
	}
	if _, err = s.requireServer(serverID); err != nil {
		return nil, err
	}
	cacheKey := fmt.Sprintf("easy_strm:emby_monitor:recent:%d:%s:%s:%s", serverID, timeBasis, rangeName, mediaType)
	cached := &domain.EmbyMonitorRecentResponse{}
	if s.cacheGet(cacheKey, cached) {
		return cached, nil
	}
	data, err := s.monitor.RecentItems(serverID, timeBasis, mediaType, start, end)
	if err != nil {
		return nil, err
	}
	history, _ := s.monitor.HistoryStart(serverID)
	result := &domain.EmbyMonitorRecentResponse{Meta: s.meta(embyMonitorSourceLocal, history, ""), Data: data, Total: len(data)}
	s.cacheSet(cacheKey, result)
	return result, nil
}

// GetItemImage 代理读取媒体主图。
func (s *EmbyMonitorService) GetItemImage(serverID int, itemID string) ([]byte, string, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, "", err
	}
	path := "/emby/Items/" + url.PathEscape(itemID) + "/Images/Primary"
	endpoint := strings.TrimRight(server.BaseURL, "/") + path + "?maxWidth=500"
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("X-Emby-Token", server.APIKey)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("媒体图片不存在")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	return data, resp.Header.Get("Content-Type"), err
}

// EmbyMonitorCollector 周期采集所有已启用 Emby 实例。
type EmbyMonitorCollector struct {
	service         *EmbyMonitorService
	sessionInterval time.Duration
	itemInterval    time.Duration
	mu              sync.Mutex
	cancel          context.CancelFunc
}

// NewEmbyMonitorCollector 创建后台采集器。
func NewEmbyMonitorCollector(service *EmbyMonitorService) *EmbyMonitorCollector {
	return &EmbyMonitorCollector{service: service, sessionInterval: 30 * time.Second, itemInterval: 5 * time.Minute}
}

// Start 启动采集器；重复调用不会创建额外协程。
func (c *EmbyMonitorCollector) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.run(ctx)
}

// Stop 停止采集器。
func (c *EmbyMonitorCollector) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.mu.Unlock()
}

func (c *EmbyMonitorCollector) run(ctx context.Context) {
	c.collectSessions()
	c.collectItems()
	sessions := time.NewTicker(c.sessionInterval)
	items := time.NewTicker(c.itemInterval)
	defer sessions.Stop()
	defer items.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sessions.C:
			c.collectSessions()
		case <-items.C:
			c.collectItems()
		}
	}
}

func (c *EmbyMonitorCollector) collectSessions() {
	servers, err := c.service.servers.List()
	if err != nil {
		logger.Warnf("[EmbyMonitorCollector] 查询实例失败: %v", err)
		return
	}
	for _, server := range servers {
		if server.Enabled {
			if err = c.service.CollectSessions(server.ID); err != nil {
				logger.Warnf("[EmbyMonitorCollector] 采集实例 %s 会话失败: %v", server.Name, err)
			}
		}
	}
}
func (c *EmbyMonitorCollector) collectItems() {
	servers, err := c.service.servers.List()
	if err != nil {
		return
	}
	for _, server := range servers {
		if server.Enabled {
			if err = c.service.CollectItems(server.ID); err != nil {
				logger.Warnf("[EmbyMonitorCollector] 同步实例 %s 媒体失败: %v", server.Name, err)
			}
		}
	}
}
