package dao

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"easy-strm/internal/domain"
)

// EmbyMonitorDAO 封装 Emby 监控的播放历史、媒体发现和统计查询。
type EmbyMonitorDAO struct{ db *sql.DB }

// NewEmbyMonitorDAO 创建 Emby 监控 DAO。
func NewEmbyMonitorDAO(db *sql.DB) *EmbyMonitorDAO { return &EmbyMonitorDAO{db: db} }

// TouchPlayback 保存一次会话采样，并返回实际累计的秒数。
func (d *EmbyMonitorDAO) TouchPlayback(sample domain.EmbyPlaybackSample, now time.Time, maxDelta time.Duration) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	var itemID string
	var lastSeen time.Time
	var startedAt time.Time
	var previousPaused bool
	var ended sql.NullTime
	err = tx.QueryRow(`SELECT id, item_id, started_at, last_seen_at, is_paused, ended_at FROM t_emby_playback_event
		WHERE server_id=$1 AND session_id=$2 AND ended_at IS NULL FOR UPDATE`, sample.ServerID, sample.SessionID).
		Scan(&id, &itemID, &startedAt, &lastSeen, &previousPaused, &ended)
	if err == sql.ErrNoRows {
		_, err = tx.Exec(`INSERT INTO t_emby_playback_event(
			server_id,session_id,user_id,user_name,item_id,item_type,item_name,series_id,series_name,image_tag,
			client_name,device_name,playback_method,started_at,last_seen_at,is_paused)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14,$15)`,
			sample.ServerID, sample.SessionID, sample.UserID, sample.UserName, sample.ItemID, sample.ItemType,
			sample.ItemName, sample.SeriesID, sample.SeriesName, sample.ImageTag, sample.ClientName, sample.DeviceName,
			sample.PlaybackMethod, now, sample.Paused)
		if err != nil {
			return 0, fmt.Errorf("新增 Emby 播放记录失败: %w", err)
		}
		return 0, tx.Commit()
	}
	if err != nil {
		return 0, fmt.Errorf("读取 Emby 活跃播放失败: %w", err)
	}
	startHour := startedAt.In(now.Location()).Truncate(time.Hour)
	nowHour := now.Truncate(time.Hour)
	if itemID != sample.ItemID || !startHour.Equal(nowHour) {
		if _, err = tx.Exec(`UPDATE t_emby_playback_event SET ended_at=$2,last_seen_at=$2 WHERE id=$1`, id, now); err != nil {
			return 0, err
		}
		_, err = tx.Exec(`INSERT INTO t_emby_playback_event(
			server_id,session_id,user_id,user_name,item_id,item_type,item_name,series_id,series_name,image_tag,
			client_name,device_name,playback_method,started_at,last_seen_at,is_paused)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14,$15)`,
			sample.ServerID, sample.SessionID, sample.UserID, sample.UserName, sample.ItemID, sample.ItemType,
			sample.ItemName, sample.SeriesID, sample.SeriesName, sample.ImageTag, sample.ClientName, sample.DeviceName,
			sample.PlaybackMethod, now, sample.Paused)
		if err != nil {
			return 0, err
		}
		return 0, tx.Commit()
	}
	delta := now.Sub(lastSeen)
	if delta < 0 {
		delta = 0
	}
	if delta > maxDelta {
		delta = maxDelta
	}
	seconds := int64(delta / time.Second)
	if sample.Paused || previousPaused {
		seconds = 0
	}
	_, err = tx.Exec(`UPDATE t_emby_playback_event SET user_id=$2,user_name=$3,item_type=$4,item_name=$5,
		series_id=$6,series_name=$7,image_tag=$8,client_name=$9,device_name=$10,playback_method=$11,
		watched_seconds=watched_seconds+$12,last_seen_at=$13,is_paused=$14 WHERE id=$1`, id,
		sample.UserID, sample.UserName, sample.ItemType, sample.ItemName, sample.SeriesID, sample.SeriesName,
		sample.ImageTag, sample.ClientName, sample.DeviceName, sample.PlaybackMethod, seconds, now, sample.Paused)
	if err != nil {
		return 0, fmt.Errorf("更新 Emby 播放记录失败: %w", err)
	}
	return seconds, tx.Commit()
}

// CloseMissingSessions 结束本轮没有出现且超过失联窗口的活跃会话。
func (d *EmbyMonitorDAO) CloseMissingSessions(serverID int, active []string, before, now time.Time) error {
	query := `UPDATE t_emby_playback_event SET ended_at=$3 WHERE server_id=$1 AND ended_at IS NULL AND last_seen_at<$2`
	args := []interface{}{serverID, before, now}
	if len(active) > 0 {
		placeholders := make([]string, len(active))
		for i, value := range active {
			placeholders[i] = fmt.Sprintf("$%d", i+4)
			args = append(args, value)
		}
		query += ` AND session_id NOT IN (` + strings.Join(placeholders, ",") + `)`
	}
	_, err := d.db.Exec(query, args...)
	return err
}

// UpsertItems 保存媒体扫描结果，首次基线不会被视为新入库。
func (d *EmbyMonitorDAO) UpsertItems(serverID int, items []domain.EmbyMonitorItem, baseline bool, now time.Time) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range items {
		_, err = tx.Exec(`INSERT INTO t_emby_monitor_item(server_id,item_id,item_type,name,series_name,production_year,image_tag,date_created,first_seen_at,is_baseline)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT(server_id,item_id) DO UPDATE SET item_type=EXCLUDED.item_type,name=EXCLUDED.name,
			series_name=EXCLUDED.series_name,production_year=EXCLUDED.production_year,image_tag=EXCLUDED.image_tag,date_created=EXCLUDED.date_created`,
			serverID, item.ItemID, item.ItemType, item.Name, item.SeriesName, item.Year, item.ImageTag, item.DateCreated, now, baseline)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetState 查询实例监控状态。
func (d *EmbyMonitorDAO) GetState(serverID int) (*domain.EmbyMonitorState, error) {
	state := &domain.EmbyMonitorState{ServerID: serverID}
	err := d.db.QueryRow(`SELECT baseline_completed_at,last_session_poll_at,last_item_poll_at,plugin_available,last_error
		FROM t_emby_monitor_state WHERE server_id=$1`, serverID).
		Scan(&state.BaselineCompletedAt, &state.LastSessionPollAt, &state.LastItemPollAt, &state.PluginAvailable, &state.LastError)
	if err == sql.ErrNoRows {
		return state, nil
	}
	return state, err
}

// UpdateState 更新实例监控状态，只覆盖本次提供的采集时间。
func (d *EmbyMonitorDAO) UpdateState(serverID int, sessionAt, itemAt *time.Time, baseline bool, plugin bool, lastError string) error {
	_, err := d.db.Exec(`INSERT INTO t_emby_monitor_state(server_id,baseline_completed_at,last_session_poll_at,last_item_poll_at,plugin_available,last_error)
		VALUES($1,CASE WHEN $4 THEN CURRENT_TIMESTAMP END,$2,$3,$5,$6)
		ON CONFLICT(server_id) DO UPDATE SET
		baseline_completed_at=CASE WHEN $4 THEN COALESCE(t_emby_monitor_state.baseline_completed_at,CURRENT_TIMESTAMP) ELSE t_emby_monitor_state.baseline_completed_at END,
		last_session_poll_at=COALESCE($2,t_emby_monitor_state.last_session_poll_at),last_item_poll_at=COALESCE($3,t_emby_monitor_state.last_item_poll_at),
		plugin_available=$5,last_error=$6,update_time=CURRENT_TIMESTAMP`, serverID, sessionAt, itemAt, baseline, plugin, lastError)
	return err
}

// HistoryStart 返回本地自采历史起点。
func (d *EmbyMonitorDAO) HistoryStart(serverID int) (*time.Time, error) {
	var value sql.NullTime
	err := d.db.QueryRow(`SELECT MIN(started_at) FROM t_emby_playback_event WHERE server_id=$1`, serverID).Scan(&value)
	if err != nil || !value.Valid {
		return nil, err
	}
	return &value.Time, nil
}

// Summary 查询自然时间范围内的汇总数据。
func (d *EmbyMonitorDAO) Summary(serverID int, today, week, month time.Time) (domain.EmbyMonitorSummary, error) {
	var result domain.EmbyMonitorSummary
	err := d.db.QueryRow(`SELECT
		COALESCE(SUM(watched_seconds) FILTER(WHERE started_at >= $2),0),
		COALESCE(SUM(watched_seconds) FILTER(WHERE started_at >= $3),0),
		COALESCE(SUM(watched_seconds) FILTER(WHERE started_at >= $4),0),COALESCE(SUM(watched_seconds),0),
		COUNT(DISTINCT user_id) FILTER(WHERE started_at >= $2),COUNT(DISTINCT user_id) FILTER(WHERE started_at >= $3),
		COUNT(DISTINCT user_id) FILTER(WHERE started_at >= $4)
		FROM t_emby_playback_event WHERE server_id=$1`, serverID, today, week, month).Scan(
		&result.TodaySeconds, &result.WeekSeconds, &result.MonthSeconds, &result.TotalSeconds,
		&result.TodayActiveUsers, &result.WeekActiveUsers, &result.MonthActiveUsers)
	return result, err
}

// Trend 查询每小时活跃用户及观看时长。
func (d *EmbyMonitorDAO) Trend(serverID int, start, end time.Time) ([]domain.EmbyMonitorTrendPoint, error) {
	trendHour := `date_trunc('hour',started_at)`
	rows, err := d.db.Query(fmt.Sprintf(`SELECT %s,COUNT(DISTINCT user_id),COALESCE(SUM(watched_seconds),0)
		FROM t_emby_playback_event WHERE server_id=$1 AND started_at >= $2 AND started_at < $3
		GROUP BY %s ORDER BY %s`, trendHour, trendHour, trendHour), serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.EmbyMonitorTrendPoint, 0)
	for rows.Next() {
		var p domain.EmbyMonitorTrendPoint
		if err = rows.Scan(&p.Time, &p.ActiveUsers, &p.WatchedSeconds); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// Rankings 查询本地自采排行。
func (d *EmbyMonitorDAO) Rankings(serverID int, dimension, mediaType string, start, end time.Time) ([]domain.EmbyMonitorRankingItem, error) {
	label, id, subtitle, image := "user_name", "user_id", `''`, `''`
	switch dimension {
	case "media":
		label, id, subtitle, image = "CASE WHEN item_type='Episode' AND series_name<>'' THEN series_name ELSE item_name END", "CASE WHEN item_type='Episode' AND series_id<>'' THEN series_id ELSE item_id END", "item_type", "MAX(image_tag)"
	case "clients":
		label, id = "client_name", "client_name"
	}
	query := fmt.Sprintf(`SELECT %s,%s,%s,%s,COALESCE(SUM(watched_seconds),0),COUNT(*) FROM t_emby_playback_event
		WHERE server_id=$1 AND started_at >= $2 AND started_at < $3`, id, label, subtitle, image)
	args := []interface{}{serverID, start, end}
	if dimension == "media" && mediaType != "" {
		if mediaType == "movie" {
			query += ` AND item_type='Movie'`
		} else {
			query += ` AND item_type='Episode'`
		}
	}
	// 用户和客户端维度的 subtitle 是展示用空字符串常量，不能参与 GROUP BY；
	// PostgreSQL 会将其解析为非整数位置常量并返回「non-integer constant」错误。
	groupBy := []string{id, label}
	if dimension == "media" {
		groupBy = append(groupBy, subtitle)
	}
	query += fmt.Sprintf(` GROUP BY %s ORDER BY SUM(watched_seconds) DESC`, strings.Join(groupBy, ","))
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.EmbyMonitorRankingItem, 0)
	for rows.Next() {
		var item domain.EmbyMonitorRankingItem
		if err = rows.Scan(&item.ID, &item.Name, &item.Subtitle, &item.ImageTag, &item.WatchedSeconds, &item.PlayCount); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// Heatmap 查询用户在日期小时维度的观看时长。
func (d *EmbyMonitorDAO) Heatmap(serverID int, userID string, start, end time.Time) ([]domain.EmbyMonitorUserHeatmap, error) {
	query := `SELECT user_id,user_name,to_char(started_at AT TIME ZONE CURRENT_SETTING('TIMEZONE'),'YYYY-MM-DD'),EXTRACT(HOUR FROM started_at AT TIME ZONE CURRENT_SETTING('TIMEZONE'))::int,COALESCE(SUM(watched_seconds),0)
		FROM t_emby_playback_event WHERE server_id=$1 AND started_at >= $2 AND started_at < $3`
	args := []interface{}{serverID, start, end}
	if userID != "" {
		query += ` AND user_id=$4`
		args = append(args, userID)
	}
	dateExpr := `to_char(started_at AT TIME ZONE CURRENT_SETTING('TIMEZONE'),'YYYY-MM-DD')`
	hourExpr := `EXTRACT(HOUR FROM started_at AT TIME ZONE CURRENT_SETTING('TIMEZONE'))::int`
	query += fmt.Sprintf(` GROUP BY user_id,user_name,%s,%s ORDER BY user_name,%s,%s`, dateExpr, hourExpr, dateExpr, hourExpr)
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byUser := map[string]*domain.EmbyMonitorUserHeatmap{}
	order := []string{}
	for rows.Next() {
		var id, name, date string
		var hour int
		var seconds int64
		if err = rows.Scan(&id, &name, &date, &hour, &seconds); err != nil {
			return nil, err
		}
		u := byUser[id]
		if u == nil {
			u = &domain.EmbyMonitorUserHeatmap{UserID: id, UserName: name, Cells: []domain.EmbyMonitorHeatmapCell{}}
			byUser[id] = u
			order = append(order, id)
		}
		u.TotalSeconds += seconds
		u.Cells = append(u.Cells, domain.EmbyMonitorHeatmapCell{Date: date, Hour: hour, Seconds: seconds})
	}
	result := make([]domain.EmbyMonitorUserHeatmap, 0, len(order))
	for _, id := range order {
		result = append(result, *byUser[id])
	}
	return result, rows.Err()
}

// RecentItems 查询最近入库媒体。
func (d *EmbyMonitorDAO) RecentItems(serverID int, timeBasis, mediaType string, start, end time.Time) ([]domain.EmbyMonitorItem, error) {
	column := "date_created"
	if timeBasis == "first_seen" {
		column = "first_seen_at"
	}
	itemType := "Movie"
	if mediaType == "series" {
		itemType = "Series"
	}
	query := fmt.Sprintf(`SELECT server_id,item_id,item_type,name,series_name,production_year,image_tag,date_created,first_seen_at,is_baseline
		FROM t_emby_monitor_item WHERE server_id=$1 AND item_type=$2 AND %s >= $3 AND %s < $4`, column, column)
	if timeBasis == "first_seen" {
		query += ` AND is_baseline=false`
	}
	query += fmt.Sprintf(` ORDER BY %s DESC LIMIT 200`, column)
	rows, err := d.db.Query(query, serverID, itemType, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.EmbyMonitorItem{}
	for rows.Next() {
		var item domain.EmbyMonitorItem
		if err = rows.Scan(&item.ServerID, &item.ItemID, &item.ItemType, &item.Name, &item.SeriesName, &item.Year, &item.ImageTag, &item.DateCreated, &item.FirstSeenAt, &item.Baseline); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
