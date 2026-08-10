//go:build live
// +build live

package main

import (
	"os"
	"testing"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// TestLiveShareParsePagination 用真实分享链接验证分页拉取能力
// 验证 GetShareSnap 支持 QueryLimit/QueryOffset，能拉全 26 个文件（之前默认只返回 20）
// 运行方式: go test -tags=live -run TestLiveShareParsePagination -v . -timeout 60s
func TestLiveShareParsePagination(t *testing.T) {
	client := NewClient(&Config{})

	shareCode := os.Getenv("EASY_STRM_LIVE_SHARE_CODE")
	password := os.Getenv("EASY_STRM_LIVE_SHARE_PASSWORD")
	if shareCode == "" {
		t.Skip("未设置 EASY_STRM_LIVE_SHARE_CODE，跳过真实分享链接测试")
	}
	const pageSize = 200

	var allFiles []driver.ShareFile
	offset := 0
	totalCount := 0
	start := time.Now()

	for {
		resp, err := client.GetShareSnap(shareCode, password, "0",
			driver.QueryLimit(pageSize), driver.QueryOffset(offset))
		if err != nil {
			t.Fatalf("❌ GetShareSnap 失败 (offset=%d): %v", offset, err)
		}
		if offset == 0 {
			totalCount = resp.Data.Count
			t.Logf("ShareTitle: %s, API Count: %d", resp.Data.Shareinfo.ShareTitle, totalCount)
		}
		allFiles = append(allFiles, resp.Data.List...)
		if len(allFiles) >= resp.Data.Count || len(resp.Data.List) == 0 {
			break
		}
		offset += pageSize
	}
	elapsed := time.Since(start)

	t.Logf("✅ 分页拉取完成 (%s): 实际拿到 %d 个文件, API 声明 %d 个", elapsed, len(allFiles), totalCount)
	if len(allFiles) != totalCount {
		t.Errorf("⚠️ 实际文件数(%d) 与 API 声明数(%d) 不一致，可能分页未拉全", len(allFiles), totalCount)
	}
	for i, f := range allFiles {
		if i >= 3 {
			t.Logf("  ... +%d 个文件", len(allFiles)-i)
			break
		}
		t.Logf("  [%d] fid=%s n=%s s=%v ico=%s sha=%s", i+1, f.FileID, f.FileName, f.Size, f.Type, f.Sha1)
	}
}
