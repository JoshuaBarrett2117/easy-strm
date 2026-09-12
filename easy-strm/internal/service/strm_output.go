package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"easy-strm/internal/dao"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// StrmOutput 统一输出目录边界、文件归属和增量写入。
type StrmOutput struct {
	Store                              *dao.StrmExportDAO
	Root, Owner, Run                   string
	Paths                              map[string]bool
	Added, Updated, Skipped, Conflicts int
}

// NormalizeStrmOutputPath 返回本机规范路径，拒绝符号链接祖先。
func NormalizeStrmOutputPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("输出路径不能为空")
	}
	p, e := filepath.Abs(p)
	if e != nil {
		return "", e
	}
	p = filepath.Clean(p)
	for q := p; ; q = filepath.Dir(q) {
		info, e := os.Lstat(q)
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("输出路径不能经过符号链接：%s", q)
		}
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if filepath.Dir(q) == q {
			break
		}
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p, nil
}

// NewStrmOutput 锁定目录并迁移历史文件归属，不触碰实际文件。
func NewStrmOutput(ctx context.Context, db *sql.DB, root, owner, run string) (*StrmOutput, error) {
	if filepath.Clean(strings.TrimSpace(root)) == "." {
		return nil, fmt.Errorf("输出目录不能是当前工作目录")
	}
	root, e := NormalizeStrmOutputPath(root)
	if e != nil {
		return nil, e
	}
	chain := []string{}
	for p := root; ; p = filepath.Dir(p) {
		chain = append([]string{p}, chain...)
		if filepath.Dir(p) == p {
			break
		}
	}
	store, e := dao.LockStrmOutput(ctx, db, chain)
	if e != nil {
		return nil, e
	}
	s := &StrmOutput{Store: store, Root: root, Owner: owner, Run: run, Paths: map[string]bool{}}
	legacy, e := store.LegacyPaths(ctx)
	if e != nil {
		store.Close()
		return nil, e
	}
	for raw, owners := range legacy {
		p, pe := NormalizeStrmOutputPath(raw)
		if pe != nil || !s.contains(p) {
			continue
		}
		s.Paths[p] = true
		for _, o := range owners {
			if e = store.RememberLegacy(ctx, p, o); e != nil {
				store.Close()
				return nil, e
			}
		}
	}
	known, e := store.KnownPaths(ctx)
	if e != nil {
		store.Close()
		return nil, e
	}
	for _, p := range known {
		if s.contains(p) {
			s.Paths[p] = true
		}
	}
	return s, nil
}
func (s *StrmOutput) contains(p string) bool {
	r, e := filepath.Rel(s.Root, p)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

// Write 仅修改本归属的文件，成功写入后登记指纹。
func (s *StrmOutput) Write(ctx context.Context, key, p, content, mapping, playback string) (bool, error) {
	p, e := NormalizeStrmOutputPath(p)
	if e != nil {
		return false, e
	}
	if !s.contains(p) {
		return false, fmt.Errorf("输出路径超出配置目录")
	}
	own, e := s.Store.CheckPath(ctx, p, s.Owner, key)
	if e != nil {
		s.Conflicts++
		return false, e
	}
	old, readErr := os.ReadFile(p)
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, readErr
	}
	if readErr == nil && !own {
		s.Conflicts++
		return false, fmt.Errorf("目标文件未登记归属，保留原文件：%s", p)
	}
	changed := readErr != nil || string(old) != content
	if e = s.Store.Reserve(ctx, s.Owner, p); e != nil {
		return false, e
	}
	if changed {
		if e = writeShareStrm(p, strings.TrimSuffix(content, "\n")); e != nil {
			return false, e
		}
		if os.IsNotExist(readErr) {
			s.Added++
		} else {
			s.Updated++
		}
	} else {
		s.Skipped++
	}
	state := dao.ExportState{Owner: s.Owner, Key: key, Path: p, Content: fmt.Sprintf("%x", sha256.Sum256([]byte(p+"\x00"+content))), Mapping: fmt.Sprintf("%x", sha256.Sum256([]byte(mapping))), Playback: playback, Run: s.Run}
	if e = s.Store.Save(ctx, state); e != nil {
		return changed, fmt.Errorf("文件已处理但登记失败：%w", e)
	}
	s.Paths[p] = true
	return changed, nil
}

// Clear 根据用户明确选择清理内容，已完成清理的同一任务不重复清空。
func (s *StrmOutput) Clear(ctx context.Context) error {
	if filepath.Dir(s.Root) == s.Root {
		return fmt.Errorf("不能清空磁盘根目录")
	}
	done, e := s.Store.ClearStarted(ctx, s.Run, s.Root)
	if e != nil || done {
		return e
	}
	entries, e := os.ReadDir(s.Root)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	for _, entry := range entries {
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = os.RemoveAll(filepath.Join(s.Root, entry.Name())); e != nil {
			return e
		}
	}
	paths := []string{}
	for p := range s.Paths {
		paths = append(paths, p)
	}
	return s.Store.MarkMissing(ctx, paths, s.Run)
}
