package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupStrmFilesRemovesAllContentsAndKeepsOutputDirectory(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	files := []string{
		filepath.Join(outputDir, "old.strm"),
		filepath.Join(outputDir, ".stale"),
		filepath.Join(outputDir, "电影", "旧文件.strm"),
	}
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatalf("创建测试目录失败: %v", err)
		}
		if err := os.WriteFile(file, []byte("stale"), 0644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
	}

	generator := NewStrmGeneratorWithServer(outputDir, "http://localhost:8082", ".strm")
	if err := generator.CleanupStrmFiles(); err != nil {
		t.Fatalf("清空 STRM 目录失败: %v", err)
	}

	info, err := os.Stat(outputDir)
	if err != nil {
		t.Fatalf("目标目录应继续存在: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("目标路径应为目录")
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("读取目标目录失败: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("目标目录应为空，实际仍有 %d 项", len(entries))
	}
}

func TestCleanupStrmFilesRejectsEmptyOutputDirectory(t *testing.T) {
	t.Parallel()

	invalidPaths := []string{"", ".", filepath.VolumeName(t.TempDir()) + string(filepath.Separator)}
	for _, invalidPath := range invalidPaths {
		generator := NewStrmGeneratorWithServer(invalidPath, "http://localhost:8082", ".strm")
		if err := generator.CleanupStrmFiles(); err == nil {
			t.Fatalf("危险目标目录 %q 应返回错误", invalidPath)
		}
	}
}
