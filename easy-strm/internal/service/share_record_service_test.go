package service

import "testing"

func TestParseShareRecordInput(t *testing.T) {
	tests := []struct{ name, input, wantURL, wantPassword string }{
		{"完整分享文案中的Markdown链接", `[https://115cdn.com/s/sws52hp3wcy?password=1111#](https://115cdn.com/s/sws52hp3wcy?password=1111#) 猎虎贰 (2026)等2个文件(夹) 访问码：1111 复制这段内容，可在115生活App中直接打开！`, "https://115cdn.com/s/sws52hp3wcy?password=1111#", "1111"},
		{"纯链接", "https://115.com/s/abc123?password=pA55#", "https://115.com/s/abc123?password=pA55#", "pA55"},
		{"文案访问码兜底", "https://115cdn.com/s/abc123 分享内容 提取码: ZX90", "https://115cdn.com/s/abc123", "ZX90"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotPassword, gotName, err := parseShareRecordInput(tt.input)
			if tt.name == "完整分享文案中的Markdown链接" && gotName != "猎虎贰 (2026)等2个文件(夹)" {
				t.Fatalf("名称解析错误 name=%q", gotName)
			}
			if err != nil || gotURL != tt.wantURL || gotPassword != tt.wantPassword {
				t.Fatalf("解析结果 url=%q password=%q err=%v", gotURL, gotPassword, err)
			}
		})
	}
}

func TestParseShareRecordInputRejectsInvalidText(t *testing.T) {
	if _, _, _, err := parseShareRecordInput("这不是分享链接"); err == nil {
		t.Fatal("无效文本应返回错误")
	}
}
