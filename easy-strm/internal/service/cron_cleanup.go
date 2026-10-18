package service

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const cleanupPageSize = 100

// CleanupEntry 是115即时目录列表中的直属子项，父ID用于删除前核验。
type CleanupEntry struct {
	ID, ParentID, Name string
	Directory          bool
}

// CleanupDirectoryClient 提供可取消的即时目录读取和批量删除，不接受缓存CID。
type CleanupDirectoryClient interface {
	List(context.Context, *domain.Cloud115, string, int, int) ([]CleanupEntry, error)
	Delete(context.Context, *domain.Cloud115, []string) error
}

// DirectoryCleanupService 串行清空多个账号的目录内容，保留每个目标目录。
type DirectoryCleanupService struct {
	client   CleanupDirectoryClient
	account  func(int) (*domain.Cloud115, error)
	interval time.Duration
	timeout  time.Duration
}

// NewDirectoryCleanupService 创建限速、有界执行的目录清空服务。
func NewDirectoryCleanupService(client CleanupDirectoryClient, account func(int) (*domain.Cloud115, error)) *DirectoryCleanupService {
	return &DirectoryCleanupService{client: client, account: account, interval: 500 * time.Millisecond, timeout: 30 * time.Minute}
}

func validateCleanupTargets(targets []domain.CronCleanupTarget, account func(int) error) error {
	if len(targets) == 0 || len(targets) > 100 {
		return fmt.Errorf("请配置1至100个清空目标")
	}
	for i := range targets {
		target := &targets[i]
		if strings.TrimSpace(target.DirPath) == "" {
			return fmt.Errorf("清空目录不能为空")
		}
		value, err := NormalizeShareTransferDirectory(target.DirPath)
		if err != nil {
			return err
		}
		if value == "/" {
			return fmt.Errorf("禁止清空根目录")
		}
		target.DirPath, target.DirCID = value, ""
		if target.Cloud115ID <= 0 {
			return fmt.Errorf("请选择有效115账号")
		}
		for _, other := range targets[:i] {
			if other.Cloud115ID == target.Cloud115ID && (other.DirPath == value || strings.HasPrefix(value, other.DirPath+"/") || strings.HasPrefix(other.DirPath, value+"/")) {
				return fmt.Errorf("同账号清空目录不能重复或互为父子目录: %s 与 %s", other.DirPath, value)
			}
		}
	}
	for _, target := range targets {
		if err := account(target.Cloud115ID); err != nil {
			return err
		}
	}
	return nil
}

func cleanupID(id string) bool {
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
}

func (s *DirectoryCleanupService) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(s.interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *DirectoryCleanupService) list(ctx context.Context, account *domain.Cloud115, cid string, offset int) ([]CleanupEntry, error) {
	if err := s.wait(ctx); err != nil {
		return nil, err
	}
	entries, err := s.client.List(ctx, account, cid, offset, cleanupPageSize)
	if err != nil {
		return nil, err
	}
	if len(entries) > cleanupPageSize {
		return nil, fmt.Errorf("目录响应超过分页上限")
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !cleanupID(entry.ID) || entry.ID == cid || entry.ParentID != cid || seen[entry.ID] {
			return nil, fmt.Errorf("目录列表包含无效ID、重复项或非直属子项，停止删除")
		}
		seen[entry.ID] = true
	}
	return entries, nil
}

func (s *DirectoryCleanupService) resolve(ctx context.Context, account *domain.Cloud115, directory string) (string, error) {
	cid := "0"
	ancestors := map[string]bool{"0": true}
	for _, name := range strings.Split(strings.TrimPrefix(directory, "/"), "/") {
		found := ""
		for page := 0; page < 10000; page++ {
			entries, err := s.list(ctx, account, cid, page*cleanupPageSize)
			if err != nil {
				return "", err
			}
			for _, entry := range entries {
				if entry.Directory && entry.Name == name {
					if found != "" || ancestors[entry.ID] {
						return "", fmt.Errorf("目录名称不唯一或目录ID指向上级")
					}
					found = entry.ID
				}
			}
			if len(entries) < cleanupPageSize {
				break
			}
			if page == 9999 {
				return "", fmt.Errorf("目录解析超过分页上限")
			}
		}
		if found == "" {
			return "", fmt.Errorf("目录不存在: %s", directory)
		}
		cid = found
		ancestors[cid] = true
	}
	return cid, nil
}

func (s *DirectoryCleanupService) clear(ctx context.Context, account *domain.Cloud115, directory string) (int, error) {
	cid, err := s.resolve(ctx, account, directory)
	if err != nil {
		return 0, err
	}
	count, stalled := 0, 0
	pending := map[string]bool{}
	for batch := 0; batch < 10000; batch++ {
		// 删除会改变偏移，始终重新读取第一页。
		entries, err := s.list(ctx, account, cid, 0)
		if err != nil {
			return count, err
		}
		current := map[string]bool{}
		for _, entry := range entries {
			current[entry.ID] = true
		}
		progress := 0
		for id := range pending {
			if !current[id] {
				progress++
				delete(pending, id)
			}
		}
		count += progress
		if len(entries) == 0 {
			return count, nil
		}
		if len(pending) > 0 && progress == 0 {
			stalled++
		} else {
			stalled = 0
		}
		if stalled >= 3 {
			return count, fmt.Errorf("连续3次删除未取得进展，停止清空")
		}
		ids := make([]string, 0, len(entries))
		for _, entry := range entries {
			ids = append(ids, entry.ID)
		}
		if err := s.wait(ctx); err != nil {
			return count, err
		}
		if err := s.client.Delete(ctx, account, ids); err != nil {
			return count, err
		}
		for _, id := range ids {
			pending[id] = true
		}
	}
	return count, fmt.Errorf("清空超过批次数量上限")
}

// Execute 返回包含成功与失败明细的JSON摘要；任一目标失败时整体失败。
func (s *DirectoryCleanupService) Execute(ctx context.Context, task *domain.CronTask, _ string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	targets := append([]domain.CronCleanupTarget(nil), task.CleanupTargets...)
	if err := validateCleanupTargets(targets, func(int) error { return nil }); err != nil {
		return "", err
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].Cloud115ID < targets[j].Cloud115ID })
	type result struct {
		AccountID int    `json:"cloud115_id"`
		Account   string `json:"account"`
		Path      string `json:"path"`
		Count     int    `json:"deleted_count"`
		Error     string `json:"error,omitempty"`
	}
	results := []result{}
	var failures []error
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		r := result{AccountID: target.Cloud115ID, Path: target.DirPath}
		err := ctx.Err()
		if err == nil {
			var account *domain.Cloud115
			account, err = s.account(target.Cloud115ID)
			if err == nil && account == nil {
				err = fmt.Errorf("115账号不存在")
			}
			if err == nil {
				r.Account = account.Name
				r.Count, err = s.clear(ctx, account, target.DirPath)
			}
		}
		if err != nil {
			r.Error = err.Error()
			failures = append(failures, err)
		}
		results = append(results, r)
	}
	raw, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	return string(raw), errors.Join(failures...)
}
