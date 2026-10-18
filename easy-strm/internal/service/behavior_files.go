package service

import (
	"context"
	"crypto/sha256"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

func behaviorHash(content string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(content))) }

// strmContentPath 从STRM内容行解析播放地址中的 path 参数，解析失败返回空串。
func strmContentPath(content string) string {
	line := strings.TrimSpace(content)
	if line == "" {
		return ""
	}
	parsed, err := url.Parse(line)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("path")
}

// behaviorMovedContent 在源文件缺少SHA1时，用旧内容里的原扩展名重写新云盘路径。
// 旧内容 path 参数保存了含扩展名的原文件路径，移动后目录与文件名均可能变化，
// 因此取新相对路径去掉 .strm 后缀后拼接原扩展名，避免把 .strm 写进播放地址。
func behaviorMovedContent(oldContent, cloudPath, relative string) string {
	oldCloudPath := strmContentPath(oldContent)
	ext := path.Ext(oldCloudPath)
	base := strings.TrimSuffix(filepath.ToSlash(relative), ".strm")
	newCloudPath := path.Join(cloudPath, base) + ext
	line := strings.TrimSpace(oldContent)
	parsed, err := url.Parse(line)
	if err != nil {
		return oldContent
	}
	query := parsed.Query()
	query.Set("path", newCloudPath)
	parsed.RawQuery = query.Encode()
	return parsed.String() + "\n"
}

func behaviorWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func (run *behaviorRun) safePath(target string, file bool) (*behaviorScope, string, error) {
	normalized, err := NormalizeStrmOutputPath(target)
	if err != nil || normalized != target {
		return nil, "", fmt.Errorf("索引路径不安全或经过符号链接")
	}
	for _, root := range run.roots {
		if !behaviorWithin(root.LocalPath, target) {
			continue
		}
		relative, _ := filepath.Rel(root.LocalPath, target)
		if file && (relative == "." || filepath.Ext(target) != ".strm") {
			return nil, "", fmt.Errorf("拒绝操作非STRM文件")
		}
		return &root, relative, nil
	}
	return nil, "", fmt.Errorf("索引路径位于配置输出根目录之外")
}

func (run *behaviorRun) read(target string) (string, bool, error) {
	root, relative, err := run.safePath(target, true)
	if err != nil {
		return "", false, err
	}
	directory, err := os.OpenRoot(root.LocalPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer directory.Close()
	file, err := directory.Open(relative)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return "", false, fmt.Errorf("索引STRM不是普通小文件")
	}
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if len(data) > 1024*1024 {
		return "", false, fmt.Errorf("STRM超过大小限制")
	}
	return string(data), true, err
}

func (run *behaviorRun) publish(target, content string) error {
	root, relative, err := run.safePath(target, true)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(root.LocalPath, 0755); err != nil {
		return err
	}
	directory, err := os.OpenRoot(root.LocalPath)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err = directory.MkdirAll(filepath.Dir(relative), 0755); err != nil {
		return err
	}
	if _, _, err = run.safePath(target, true); err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(relative), ".behavior-"+uuid.NewString())
	file, err := directory.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer directory.Remove(temporary)
	_, writeErr := io.WriteString(file, content)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	return directory.Link(temporary, relative)
}

func (run *behaviorRun) unlink(item domain.BehaviorIndex) error {
	root, relative, err := run.safePath(item.Path, true)
	if err != nil {
		return err
	}
	content, exists, err := run.read(item.Path)
	if err != nil || !exists {
		return err
	}
	if !item.Owned || item.ContentHash == "" || behaviorHash(content) != item.ContentHash {
		return fmt.Errorf("文件归属或指纹不匹配，保留原文件")
	}
	directory, err := os.OpenRoot(root.LocalPath)
	if err != nil {
		return err
	}
	defer directory.Close()
	err = directory.Remove(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (run *behaviorRun) write(ctx context.Context, node BehaviorNode, root behaviorScope, relative string) error {
	target := filepath.Join(root.LocalPath, strings.TrimSuffix(relative, filepath.Ext(relative))+".strm")
	if _, _, err := run.safePath(target, true); err != nil {
		return err
	}
	old, err := run.store.Get(ctx, node.ID)
	if err != nil {
		return err
	}
	if old != nil && old.IsDir {
		return fmt.Errorf("文件身份与目录索引冲突")
	}
	if old != nil && old.Path != target {
		if !old.Owned {
			run.stats.Skipped++
			return nil
		}
		return run.moveFile(ctx, *old, target, node.ParentID)
	}
	content, exists, err := run.read(target)
	if err != nil {
		return err
	}
	if exists {
		if old == nil {
			old = &domain.BehaviorIndex{CloudID: run.account, FileID: node.ID, ParentID: node.ParentID, Path: target, Size: node.Size, SHA1: node.SHA1}
		}
		if old.Owned && behaviorHash(content) != old.ContentHash {
			return fmt.Errorf("已索引文件被外部修改，拒绝接管")
		}
		if err = run.store.Put(ctx, *old); err != nil {
			return err
		}
		run.stats.Skipped++
		return nil
	}
	cloudPath := path.Join(root.CloudPath, filepath.ToSlash(relative))
	content = BuildCloud115StrmContent(run.service.serverURL(), cloudPath, node.SHA1, run.account)
	item := domain.BehaviorIndex{CloudID: run.account, FileID: node.ID, ParentID: node.ParentID, Path: target, Size: node.Size, SHA1: node.SHA1, Owned: true, ContentHash: behaviorHash(content)}
	if err = run.store.Put(ctx, item); err != nil {
		return err
	}
	if err = run.publish(target, content); err != nil {
		return err
	}
	run.stats.Generated++
	run.changed[target] = true
	return nil
}

func (run *behaviorRun) recoverPending(ctx context.Context) error {
	items, err := run.store.Pending(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err = run.budget(ctx); err != nil {
			return err
		}
		if err = run.finishMove(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (run *behaviorRun) moveFile(ctx context.Context, item domain.BehaviorIndex, target, parent string) error {
	if !item.Owned {
		return fmt.Errorf("未归属文件不能移动")
	}
	root, relative, err := run.safePath(target, true)
	if err != nil {
		return err
	}
	if item.Path == target {
		item.ParentID = parent
		return run.store.Put(ctx, item)
	}
	if _, exists, readErr := run.read(target); readErr != nil {
		return readErr
	} else if exists {
		return fmt.Errorf("目标路径已存在，拒绝覆盖")
	}
	item.PendingPath = target
	item.PendingContent = BuildCloud115StrmContent(run.service.serverURL(), path.Join(root.CloudPath, strings.TrimSuffix(filepath.ToSlash(relative), ".strm")+path.Ext(filepath.ToSlash(item.Path))), item.SHA1, run.account)
	if item.SHA1 == "" {
		content, exists, readErr := run.read(item.Path)
		if readErr != nil {
			return readErr
		}
		if !exists {
			return fmt.Errorf("移动源STRM不存在")
		}
		item.PendingContent = behaviorMovedContent(content, root.CloudPath, relative)
	}
	item.ParentID = parent
	if err = run.store.Put(ctx, item); err != nil {
		return err
	}
	return run.finishMove(ctx, item)
}

func (run *behaviorRun) finishMove(ctx context.Context, item domain.BehaviorIndex) error {
	if !item.Owned || item.PendingPath == "" || item.PendingContent == "" {
		return fmt.Errorf("移动日志无效，保留索引")
	}
	if _, _, err := run.safePath(item.Path, true); err != nil {
		return err
	}
	if _, _, err := run.safePath(item.PendingPath, true); err != nil {
		return err
	}
	old, oldExists, err := run.read(item.Path)
	if err != nil {
		return err
	}
	if oldExists && behaviorHash(old) != item.ContentHash {
		return fmt.Errorf("移动源文件指纹不匹配")
	}
	current, newExists, err := run.read(item.PendingPath)
	if err != nil {
		return err
	}
	if newExists && current != item.PendingContent {
		return fmt.Errorf("移动目标冲突，拒绝覆盖")
	}
	if !oldExists && !newExists {
		return fmt.Errorf("移动源和目标均不存在")
	}
	if !newExists {
		if err = run.publish(item.PendingPath, item.PendingContent); err != nil {
			return err
		}
	}
	if oldExists {
		if err = run.unlink(item); err != nil {
			return err
		}
	}
	run.changed[item.Path] = true
	run.changed[item.PendingPath] = true
	item.Path = item.PendingPath
	item.ContentHash = behaviorHash(item.PendingContent)
	item.PendingPath = ""
	item.PendingContent = ""
	return run.store.Put(ctx, item)
}

func (run *behaviorRun) remove(ctx context.Context, id string, event domain.BehaviorEvent) error {
	items, err := run.store.Tree(ctx, id)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		run.skip(ctx, event, "删除事件缺少索引，禁止扫描或猜测本地路径")
		return nil
	}
	sort.Slice(items, func(first, second int) bool { return len(items[first].Path) > len(items[second].Path) })
	for _, item := range items {
		if err = run.budget(ctx); err != nil {
			return err
		}
		if _, _, safeErr := run.safePath(item.Path, !item.IsDir); safeErr != nil {
			run.skip(ctx, event, "索引路径不安全或已超出配置，保留文件和索引")
			continue
		}
		if !item.IsDir {
			if !item.Owned {
				run.skip(ctx, event, "未登记写入归属的历史STRM，保留文件和索引")
				continue
			}
			if err = run.unlink(item); err != nil {
				return err
			}
			run.changed[item.Path] = true
		}
		if err = run.store.Delete(ctx, item.FileID); err != nil {
			return err
		}
	}
	return nil
}

func (run *behaviorRun) relocate(ctx context.Context, event domain.BehaviorEvent) error {
	item, err := run.store.Get(ctx, event.FileID)
	if err != nil {
		return err
	}
	if item == nil {
		run.skip(ctx, event, "移动或重命名事件缺少旧索引")
		return nil
	}
	if _, _, err = run.safePath(item.Path, !item.IsDir); err != nil {
		run.skip(ctx, event, "旧索引已超出配置或不安全，保留文件")
		return nil
	}
	node := BehaviorNode{ID: event.FileID, ParentID: event.ParentID, Name: event.Name, IsDir: item.IsDir}
	root, ancestors, relative, err := run.locate(ctx, node)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		run.skip(ctx, event, "目标目录解析失败，不能当作移出范围删除")
		return nil
	}
	if root == nil {
		behaviorLog(ctx, "文件移出配置子树，仅删除已归属且有索引的STRM", event)
		return run.remove(ctx, event.FileID, event)
	}
	for position, ancestor := range ancestors {
		parts := []string{root.LocalPath}
		for _, child := range ancestors[1 : position+1] {
			parts = append(parts, child.Name)
		}
		if err = run.rememberDirectory(ctx, domain.BehaviorIndex{CloudID: run.account, FileID: ancestor.ID, ParentID: ancestor.ParentID, Path: filepath.Join(parts...), IsDir: true}); err != nil {
			return err
		}
	}
	target := filepath.Join(root.LocalPath, relative)
	if !item.IsDir {
		if !item.Owned {
			run.skip(ctx, event, "历史未归属STRM不能移动")
			return nil
		}
		target = strings.TrimSuffix(target, filepath.Ext(target)) + ".strm"
		return run.moveFile(ctx, *item, target, event.ParentID)
	}
	if _, _, err = run.safePath(target, false); err != nil {
		return err
	}
	if target != item.Path && behaviorWithin(item.Path, target) {
		return fmt.Errorf("拒绝将目录移动到自身子目录")
	}
	items, err := run.store.Tree(ctx, event.FileID)
	if err != nil {
		return err
	}
	sort.Slice(items, func(first, second int) bool { return len(items[first].Path) > len(items[second].Path) })
	for _, child := range items {
		if err = run.budget(ctx); err != nil {
			return err
		}
		if child.FileID == item.FileID {
			continue
		}
		if !behaviorWithin(item.Path, child.Path) {
			if behaviorWithin(target, child.Path) {
				continue
			}
			return fmt.Errorf("目录子项索引前缀不一致")
		}
		if !child.IsDir && !child.Owned {
			run.skip(ctx, event, "目录内未归属STRM保留在原位")
			continue
		}
		suffix, _ := filepath.Rel(item.Path, child.Path)
		newPath := filepath.Join(target, suffix)
		if child.IsDir {
			child.Path = newPath
			err = run.store.Put(ctx, child)
		} else {
			err = run.moveFile(ctx, child, newPath, child.ParentID)
		}
		if err != nil {
			return err
		}
	}
	item.Path = target
	item.ParentID = event.ParentID
	return run.store.Put(ctx, *item)
}
