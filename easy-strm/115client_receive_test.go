package main

import "testing"

// TestBuildReceiveShareFormUsesCID 验证115分享转存使用官方接口要求的cid字段传递目标目录。
// 若误用save_folder_id，115会忽略目标目录并把文件放入“最近接收”。
func TestBuildReceiveShareFormUsesCID(t *testing.T) {
	form := buildReceiveShareForm("share-code", "receive-code", "file-1", "target-cid")

	if got := form.Get("cid"); got != "target-cid" {
		t.Fatalf("cid = %q，期望 target-cid", got)
	}
	if got := form.Get("save_folder_id"); got != "" {
		t.Fatalf("不应发送115不识别的save_folder_id字段，实际值为 %q", got)
	}
	if got := form.Get("share_code"); got != "share-code" {
		t.Fatalf("share_code = %q，期望 share-code", got)
	}
	if got := form.Get("receive_code"); got != "receive-code" {
		t.Fatalf("receive_code = %q，期望 receive-code", got)
	}
	if got := form.Get("file_id"); got != "file-1" {
		t.Fatalf("file_id = %q，期望 file-1", got)
	}
}
