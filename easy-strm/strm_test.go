package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseDirectoryTree 测试目录树解析
func TestParseDirectoryTree(t *testing.T) {
	// 创建测试目录树文件
	testTree := `{
		"cid": "1",
		"name": "根目录",
		"type": "dir",
		"files": [
			{
				"n": "test1.mp4",
				"m": 0,
				"cc": "fid1",
				"p": 1024
			},
			{
				"n": "test2.txt",
				"m": 0,
				"cc": "fid2",
				"p": 512
			}
		],
		"children": [
			{
				"cid": "2",
				"name": "子目录",
				"type": "dir",
				"files": [
					{
						"n": "test3.mkv",
						"m": 0,
						"cc": "fid3",
						"p": 2048
					}
				],
				"children": []
			}
		]
	}`
	
	// 写入测试文件
	testFile := filepath.Join(t.TempDir(), "test_tree.json")
	if err := os.WriteFile(testFile, []byte(testTree), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	
	// 解析目录树
	collection, err := ParseDirectoryTree(testFile)
	if err != nil {
		t.Fatalf("ParseDirectoryTree failed: %v", err)
	}
	
	// 验证结果
	if collection.Total != 2 {
		t.Errorf("Expected 2 videos, got %d", collection.Total)
	}
	
	// 验证视频文件
	expectedVideos := []string{"test1.mp4", "test3.mkv"}
	foundVideos := make([]string, 0)
	
	for _, video := range collection.Videos {
		foundVideos = append(foundVideos, video.Filename)
	}
	
	for _, expected := range expectedVideos {
		found := false
		for _, foundVideo := range foundVideos {
			if foundVideo == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected video %s not found", expected)
		}
	}
}

// TestStrmGenerator 测试STRM文件生成
func TestStrmGenerator(t *testing.T) {
	// 创建测试视频集合
	collection := &VideoCollection{
		Videos: []VideoFile{
			{
				Path:      "test/path",
				Filename:  "test1.mp4",
				CID:       "1",
				FID:       "fid1",
				Size:      1024,
				Extension: ".mp4",
			},
			{
				Path:      "test/path/sub",
				Filename:  "test2.mkv",
				CID:       "2",
				FID:       "fid2",
				Size:      2048,
				Extension: ".mkv",
			},
		},
		Total: 2,
	}
	
	// 创建临时输出目录
	outputDir := t.TempDir()
	
	// 创建STRM生成器
	generator := NewStrmGenerator(outputDir, "http://localhost:8080", ".strm")
	
	// 生成STRM文件
	if err := generator.GenerateStrmFiles(collection); err != nil {
		t.Fatalf("GenerateStrmFiles failed: %v", err)
	}
	
	// 验证STRM文件是否生成
	expectedStrmFiles := []string{
		filepath.Join(outputDir, "test", "path", "test1.strm"),
		filepath.Join(outputDir, "test", "path", "sub", "test2.strm"),
	}
	
	for _, expectedFile := range expectedStrmFiles {
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected STRM file %s not found", expectedFile)
		}
	}
	
	// 验证STRM文件内容
	for _, expectedFile := range expectedStrmFiles {
		content, err := os.ReadFile(expectedFile)
		if err != nil {
			t.Errorf("Failed to read STRM file %s: %v", expectedFile, err)
			continue
		}
		
		contentStr := string(content)
		if len(contentStr) == 0 {
			t.Errorf("STRM file %s is empty", expectedFile)
		}
		
		if !contains(contentStr, "http://localhost:8080/api/check") {
			t.Errorf("STRM file %s does not contain expected URL pattern", expectedFile)
		}
	}
	
	// 验证STRM文件
	if err := generator.ValidateStrmFiles(collection); err != nil {
		t.Errorf("ValidateStrmFiles failed: %v", err)
	}
	
	// 清理测试文件
	if err := generator.CleanupStrmFiles(); err != nil {
		t.Errorf("CleanupStrmFiles failed: %v", err)
	}
}

// TestEncodePath 测试路径编码
func TestEncodePath(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"test/path", "test/path"},
		{"test path", "test%20path"},
		{"test/path with spaces", "test/path%20with%20spaces"},
		{"test/path&with&symbols", "test/path%26with%26symbols"},
	}
	
	for _, tc := range testCases {
		result := EncodePath(tc.input)
		if result != tc.expected {
			t.Errorf("EncodePath(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// TestDecodePath 测试路径解码
func TestDecodePath(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{}
	
	for _, tc := range testCases {
		result, err := DecodePath(tc.input)
		if err != nil {
			t.Errorf("DecodePath(%q) error: %v", tc.input, err)
			continue
		}
		if result != tc.expected {
			t.Errorf("DecodePath(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// TestSanitizeFilename 测试文件名清理
func TestSanitizeFilename(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"test.txt", "test.txt"},
		{"test<file>.txt", "test_file_.txt"},
		{"test:file.txt", "test_file.txt"},
		{"test\"file\".txt", "test_file_.txt"},
		{"test|file.txt", "test_file.txt"},
		{"test?file.txt", "test_file.txt"},
		{"test*file.txt", "test_file.txt"},
	}
	
	for _, tc := range testCases {
		result := SanitizeFilename(tc.input)
		if result != tc.expected {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
