package dao

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

type systemConfigSnapshotKey struct {
	database *sql.DB
	keys     string
}

type systemConfigSnapshot struct {
	done        chan struct{}
	expires     time.Time
	values      map[string]string
	err         error
	invalidated bool
}

var hotSystemConfigs = struct {
	sync.Mutex
	entries map[systemConfigSnapshotKey]*systemConfigSnapshot
}{entries: make(map[systemConfigSnapshotKey]*systemConfigSnapshot)}

// InvalidateSystemConfigCache 在写入成功后废弃该连接池的所有热配置，阻止在途旧查询重新发布。
func InvalidateSystemConfigCache(database *sql.DB) {
	hotSystemConfigs.Lock()
	defer hotSystemConfigs.Unlock()
	for key, entry := range hotSystemConfigs.entries {
		if key.database == database {
			entry.invalidated = true
			delete(hotSystemConfigs.entries, key)
		}
	}
}

// ReadSystemConfigSnapshot 一次查询关联热配置并缓存两秒，返回值为独立副本；故障不回退旧值。
func ReadSystemConfigSnapshot(ctx context.Context, database *sql.DB, keys []string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if database == nil || len(keys) == 0 || len(keys) > 16 {
		return nil, fmt.Errorf("热配置查询参数或数据库未初始化")
	}
	keys = append([]string(nil), keys...)
	sort.Strings(keys)
	for _, key := range keys {
		if key == "share_strm_incremental_watermark" || strings.ContainsRune(key, '\x00') {
			return nil, fmt.Errorf("热配置查询键无效")
		}
	}
	cacheKey := systemConfigSnapshotKey{database: database, keys: strings.Join(keys, "\x00")}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hotSystemConfigs.Lock()
		entry := hotSystemConfigs.entries[cacheKey]
		reused := entry != nil && (entry.expires.IsZero() || time.Now().Before(entry.expires))
		if !reused {
			entry = &systemConfigSnapshot{done: make(chan struct{})}
			hotSystemConfigs.entries[cacheKey] = entry
		}
		hotSystemConfigs.Unlock()
		if reused {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-entry.done:
			}
		} else {
			values, err := readSystemConfigValues(ctx, database, keys)
			hotSystemConfigs.Lock()
			entry.values, entry.err = values, err
			entry.expires = time.Now().Add(2 * time.Second)
			if err != nil && hotSystemConfigs.entries[cacheKey] == entry {
				delete(hotSystemConfigs.entries, cacheKey)
			}
			close(entry.done)
			hotSystemConfigs.Unlock()
		}
		hotSystemConfigs.Lock()
		if entry.invalidated {
			hotSystemConfigs.Unlock()
			continue
		}
		values := make(map[string]string, len(entry.values))
		for key, value := range entry.values {
			values[key] = value
		}
		err := entry.err
		hotSystemConfigs.Unlock()
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		return values, nil
	}
}

func readSystemConfigValues(ctx context.Context, database *sql.DB, keys []string) (map[string]string, error) {
	rows, err := database.QueryContext(ctx, `SELECT config_key,config_val FROM t_system_config WHERE config_key=ANY($1::text[])`, pq.Array(keys))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make(map[string]string, len(keys))
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
