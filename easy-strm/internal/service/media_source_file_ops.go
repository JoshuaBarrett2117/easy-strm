package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// ValidateLocalPath 验证本地路径是否存在且可访问
// 参数:
//   - path: 本地路径
//
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) ValidateLocalPath(path string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}

	// 检查路径是否存在
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("路径不存在: %s", path)
		}
		return fmt.Errorf("无法访问路径: %v", err)
	}

	// 检查是否为目录
	if !info.IsDir() {
		return fmt.Errorf("路径不是目录: %s", path)
	}

	return nil
}

// FormatFileSize 格式化文件大小显示
// 参数:
//   - size: 文件大小（字节）
//
// 返回:
//   - string: 格式化后的文件大小
func (s *MediaSourceService) FormatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	} else if size < unit*unit {
		return fmt.Sprintf("%.1f KB", float64(size)/float64(unit))
	} else if size < unit*unit*unit {
		return fmt.Sprintf("%.1f MB", float64(size)/float64(unit*unit))
	} else if size < unit*unit*unit*unit {
		return fmt.Sprintf("%.1f GB", float64(size)/float64(unit*unit*unit))
	}
	return fmt.Sprintf("%.1f TB", float64(size)/float64(unit*unit*unit*unit))
}

// CopyFile 复制文件（支持大文件流式复制）
// 参数:
//   - src: 源文件路径
//   - dst: 目标文件路径
//
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) CopyFile(src, dst string) error {
	// 打开源文件
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %v", err)
	}
	defer sourceFile.Close()

	// 创建目标文件
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %v", err)
	}
	defer dstFile.Close()

	// 流式复制
	buffer := make([]byte, 32*1024) // 32KB 缓冲区
	_, err = io.CopyBuffer(dstFile, sourceFile, buffer)
	if err != nil {
		return fmt.Errorf("复制文件失败: %v", err)
	}

	// 复制文件权限
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("获取源文件信息失败: %v", err)
	}
	return os.Chmod(dst, info.Mode())
}

// MoveFile 移动文件
// 参数:
//   - src: 源文件路径
//   - dst: 目标文件路径
//
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) MoveFile(src, dst string) error {
	// 首先尝试重命名（同一文件系统）
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// 如果重命名失败（跨文件系统），则复制后删除
	if err := s.CopyFile(src, dst); err != nil {
		return fmt.Errorf("复制文件失败: %v", err)
	}

	// 删除源文件
	if err := os.Remove(src); err != nil {
		// 尝试删除目标文件
		os.Remove(dst)
		return fmt.Errorf("删除源文件失败: %v", err)
	}

	return nil
}

// DeleteFile 删除文件
// 参数:
//   - path: 文件路径
//
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) DeleteFile(path string) error {
	return os.Remove(path)
}

// RenameFile 重命名文件
// 参数:
//   - oldPath: 旧文件路径
//   - newPath: 新文件路径
//
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) RenameFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// CreateHardLink 创建硬链接
func (s *MediaSourceService) CreateHardLink(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if err := os.Link(src, dst); err != nil {
		return fmt.Errorf("创建硬链接失败: %w", err)
	}
	return nil
}

// CreateSymbolicLink 创建软链接
func (s *MediaSourceService) CreateSymbolicLink(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if err := os.Symlink(absSrc, dst); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("Windows 创建软链接需要管理员权限或启用开发者模式: %w", err)
		}
		return fmt.Errorf("创建软链接失败: %w", err)
	}
	return nil
}

// CreateDirectory 创建目录
// 参数:
//   - path: 目录路径
//
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

// FileExists 检查文件是否存在
// 参数:
//   - path: 文件路径
//
// 返回:
//   - bool: 是否存在
func (s *MediaSourceService) FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// IsDirectory 检查是否为目录
// 参数:
//   - path: 文件路径
//
// 返回:
//   - bool: 是否为目录
func (s *MediaSourceService) IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
