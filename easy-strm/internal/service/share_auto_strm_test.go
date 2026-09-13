package service

import (
	"encoding/json"
	"testing"

	"easy-strm/internal/domain"
)

func TestShareStrmAutoExportOnlyStartsWhenConfigured(t *testing.T) {
	s, _, _ := strmFixture(t)
	s.settings = &shareSettingsMemory{}
	if id, started, err := s.StartAutoExport([]int{1}); err != nil || started || id != "" {
		t.Fatalf("未配置目录时不应自动导出：id=%q started=%v err=%v", id, started, err)
	}

	cfg := domain.ShareStrmSettings{OutputPath: t.TempDir(), BaseURL: "http://media.example", Cloud115ID: 7, TransferPath: "/播放"}
	raw, _ := json.Marshal(cfg)
	s.settings = &shareSettingsMemory{value: &domain.SystemConfig{ConfigVal: string(raw)}}
	if _, started, err := s.StartAutoExport(nil); err != nil || started {
		t.Fatalf("没有新识别文件时不应自动导出：started=%v err=%v", started, err)
	}
}
