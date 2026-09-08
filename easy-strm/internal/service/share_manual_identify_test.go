package service

import (
	"context"
	"easy-strm/internal/domain"
	"testing"
)

// TestManualIdentifyRejectsInvalidSelection 验证空结果及不合法媒体类型不会写入数据库。
func TestManualIdentifyRejectsInvalidSelection(t *testing.T) {
	s := &ShareRecordService{}
	for _, m := range []domain.ShareMedia{{}, {ID: 1, Version: 1}, {ID: 1, Version: 1, Result: &domain.TmdbIdentifyResult{Title: "测试", TmdbID: 1, MediaType: "unknown"}}} {
		if s.ManualIdentify(context.Background(), m) == nil {
			t.Fatal("应拒绝无效选择")
		}
	}
}
