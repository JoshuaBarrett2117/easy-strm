package service

import (
	"testing"

	"easy-strm/internal/domain"
)

func TestBuildFullGenerateCronTaskNameUsesConfigID(t *testing.T) {
	t.Parallel()

	if actual := buildFullGenerateCronTaskName(23); actual != "STRM全量生成-23" {
		t.Fatalf("全量任务名称 = %q，期望 %q", actual, "STRM全量生成-23")
	}
}

func TestNormalizeStrmPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "空字符串", input: "", expected: ""},
		{name: "根路径", input: "/", expected: "/"},
		{name: "去除尾斜杠", input: "/影视资源/", expected: "/影视资源"},
		{name: "补齐前导斜杠", input: "影视资源/电影", expected: "/影视资源/电影"},
		{name: "转换反斜杠", input: "\\影视资源\\电影\\", expected: "/影视资源/电影"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if actual := normalizeStrmPath(tc.input); actual != tc.expected {
				t.Fatalf("normalizeStrmPath(%q) = %q, want %q", tc.input, actual, tc.expected)
			}
		})
	}
}

func TestHasStrmPathPrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		targetPath string
		configPath string
		expected   bool
	}{
		{name: "完全相等", targetPath: "/影视资源/电影", configPath: "/影视资源/电影", expected: true},
		{name: "父级前缀", targetPath: "/影视资源/电影/动画", configPath: "/影视资源/电影", expected: true},
		{name: "仅字符串前缀不算", targetPath: "/影视资源馆", configPath: "/影视资源", expected: false},
		{name: "无关路径", targetPath: "/视频资源", configPath: "/影视资源", expected: false},
		{name: "根路径仅匹配根路径", targetPath: "/影视资源", configPath: "/", expected: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if actual := hasStrmPathPrefix(tc.targetPath, tc.configPath); actual != tc.expected {
				t.Fatalf("hasStrmPathPrefix(%q, %q) = %v, want %v", tc.targetPath, tc.configPath, actual, tc.expected)
			}
		})
	}
}

func TestFindBestMatchingStrmConfig(t *testing.T) {
	t.Parallel()

	configs := []*domain.StrmConfig{
		{ID: 2, Cloud115Id: 3, NetDiskPath: "/视频资源"},
		{ID: 23, Cloud115Id: 3, NetDiskPath: "/影视资源"},
		{ID: 24, Cloud115Id: 3, NetDiskPath: "/影视资源/动画"},
		{ID: 30, Cloud115Id: 2, NetDiskPath: "/影视资源"},
	}

	cases := []struct {
		name       string
		cloud115ID int
		targetPath string
		expectedID int
	}{
		{name: "精确匹配优先", cloud115ID: 3, targetPath: "/影视资源", expectedID: 23},
		{name: "更长父级路径优先", cloud115ID: 3, targetPath: "/影视资源/动画/剧场版", expectedID: 24},
		{name: "无路径匹配时不回退", cloud115ID: 3, targetPath: "/综艺资源", expectedID: 0},
		{name: "不同账号不参与匹配", cloud115ID: 2, targetPath: "/影视资源", expectedID: 30},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := findBestMatchingStrmConfig(configs, tc.cloud115ID, tc.targetPath)
			if tc.expectedID == 0 {
				if cfg != nil {
					t.Fatalf("expected nil, got config ID %d", cfg.ID)
				}
				return
			}
			if cfg == nil {
				t.Fatalf("expected config ID %d, got nil", tc.expectedID)
			}
			if cfg.ID != tc.expectedID {
				t.Fatalf("expected config ID %d, got %d", tc.expectedID, cfg.ID)
			}
		})
	}
}
