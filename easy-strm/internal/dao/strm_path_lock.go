package dao

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// LockStrmDeletePaths 原子获取待删路径的独占锁及祖先共享锁，失败时不保留部分占用。
func (d *ShareRecordDAO) LockStrmDeletePaths(ctx context.Context, paths []string) (func(), error) {
	return lockStrmPaths(ctx, d.db, paths)
}

func lockStrmPaths(ctx context.Context, db *sql.DB, paths []string) (func(), error) {
	if len(paths) == 0 {
		return func() {}, nil
	}
	keys := map[string]bool{}
	for _, p := range paths {
		var err error
		p, err = filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		p = filepath.Clean(p)
		if runtime.GOOS == "windows" {
			p = strings.ToLower(p)
		}
		keys[p] = true
		for q := filepath.Dir(p); ; q = filepath.Dir(q) {
			if _, ok := keys[q]; !ok {
				keys[q] = false
			}
			if filepath.Dir(q) == q {
				break
			}
		}
	}
	ordered := []string{}
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	c, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	store := &StrmExportDAO{Conn: c}
	for {
		busy := false
		for _, k := range ordered {
			fn := "pg_try_advisory_lock_shared"
			if keys[k] {
				fn = "pg_try_advisory_lock"
			}
			var ok bool
			err = c.QueryRowContext(ctx, "SELECT "+fn+"(hashtextextended($1, 34981))", k).Scan(&ok)
			if err != nil {
				store.Close()
				return nil, err
			}
			if !ok {
				busy = true
				break
			}
		}
		if !busy {
			return store.Close, nil
		}
		if _, err = c.ExecContext(ctx, "SELECT pg_advisory_unlock_all()"); err != nil {
			store.Close()
			return nil, fmt.Errorf("释放部分路径锁失败: %w", err)
		}
		select {
		case <-ctx.Done():
			store.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
