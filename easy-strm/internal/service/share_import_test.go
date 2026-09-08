package service

import (
	"strings"
	"testing"
)

// TestShareImportFormats 覆盖用户提供的混合排版，使用虚构分享编号避免保存真实访问凭据。
func TestShareImportFormats(t *testing.T) {
	tests := []struct{ name, text, title, code, password string }{
		{"前置Markdown", "🔗 【115】大包：动漫 394部 14T\r\n[https://115cdn.com/s/test1?password=0000#](https://115cdn.com/s/test1?password=0000#)", "动漫 394部 14T", "test1", "0000"},
		{"同行", "整理好的圆盘 2.24PB：[https://115.com/s/test2?password=abcd#](https://115.com/s/test2?password=abcd#)", "整理好的圆盘 2.24PB", "test2", "abcd"},
		{"后置", "https://115.com/s/test3\n演唱会原盘 820T 永久有效\n访问码：5089\n复制这段内容，可在115App中直接打开！", "演唱会原盘 820T 永久有效", "test3", "5089"},
		{"简写", "/share.115.com/test4\n纪录片 21T\n访问码：n9f5", "纪录片 21T", "test4", "n9f5"},
		{"同行裸码", "经典台剧 [https://115.com/s/test5#](https://115.com/s/test5#) faf5 4T 有效", "经典台剧", "test5", "faf5"},
		{"后置无标签", "https://115cdn.com/s/test6?password=9527#\n2025年电影更新至9月 6.6T\nhttps://115.com/u/promo", "2025年电影更新至9月 6.6T", "test6", "9527"},
		{"转义", "音乐 [https://115.com/s/test7?password=8888&#](https://115.com/s/test7?password=8888\\&#)\\", "音乐", "test7", "8888"},
		{"保留括号", "[周星驰] [原盘电影集 国粤语] https://share.115.com/test8?password=gd41#", "[周星驰] [原盘电影集 国粤语]", "test8", "gd41"},
		{"Markdown标题", "[电影合集](https://115.com/s/test9) 密码：1111", "电影合集", "test9", "1111"},
	}
	s := &ShareRecordService{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := s.ParseImport(tt.text)
			if err != nil || len(p.Records) != 1 {
				t.Fatalf("preview=%+v err=%v", p, err)
			}
			r := p.Records[0]
			if r.Name != tt.title || r.ShareCode != tt.code || r.Password != tt.password || r.URL != "https://115.com/s/"+tt.code {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestShareImportMixedAndConflicts(t *testing.T) {
	text := strings.Join([]string{
		"宣传，全球顶级封装", "列表：", "电影一", "https://115.com/s/one?password=1111",
		"电影二", "https://115cdn.com/s/two?password=2222", "",
		"https://115.com/s/three", "电影三", "访问码：3333", "复制这段内容，可直接打开！",
		"https://115.com/s/four", "电影四", "访问码：4444", "复制这段内容，可直接打开！", "",
		"不同名称 https://share.115.com/one?password=9999",
		"电影二 https://115.com/s/two?password=2222#",
		"https://115.com/u/promotional", "https://115.com/s/unknown",
	}, "\n")
	p, err := (&ShareRecordService{}).ParseImport(text)
	if err != nil || len(p.Records) != 5 || p.Duplicates != 2 {
		t.Fatalf("%+v %v", p, err)
	}
	for i, name := range []string{"电影一", "电影二", "电影三", "电影四", ""} {
		if p.Records[i].Name != name {
			t.Fatalf("row%d: %+v", i, p.Records[i])
		}
	}
	if len(p.Records[0].Names) != 2 || len(p.Records[0].Passwords) != 2 || len(p.Records[0].Warnings) != 2 {
		t.Fatalf("%+v", p.Records[0])
	}
	if len(p.Records[4].Warnings) == 0 || len(p.Ignored) != 3 {
		t.Fatalf("%+v", p)
	}
}

func TestShareImportInvalidAndMultiple(t *testing.T) {
	s := &ShareRecordService{}
	for _, text := range []string{"", "普通文案", "https://115.com/u/promo", "https://evil115.com/s/abc", "https://example.com/115.com/s/abc"} {
		if _, err := s.ParseImport(text); err == nil {
			t.Fatalf("应拒绝 %q", text)
		}
	}
	p, err := s.ParseImport("https://115.com/s/a https://115.com/s/b")
	if err != nil || len(p.Records) != 2 {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = s.ParseImport("电影 https://115.com/s/a?password=1111 密码：2222")
	if err != nil || len(p.Records[0].Passwords) != 2 {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestShareImportBlockBoundary(t *testing.T) {
	text := "电影一 https://115.com/s/first 密码：1111\n电影二\nhttps://115.com/s/second\n电影三\n访问码：3333\n复制这段内容，可直接打开！\n电影四\nhttps://115.com/s/fourth"
	p, err := (&ShareRecordService{}).ParseImport(text)
	if err != nil || len(p.Records) != 3 {
		t.Fatalf("%+v %v", p, err)
	}
	if p.Records[2].Name != "电影四" {
		t.Fatalf("不应跨结束标记消费下一条标题：%+v", p)
	}
	p, err = (&ShareRecordService{}).ParseImport("电影一 https://115.com/s/first 密码：1111\n电影二\nhttps://115.com/s/second")
	if err != nil || p.Records[1].Name != "电影二" {
		t.Fatalf("同行访问码不应消费下一条标题：%+v %v", p, err)
	}
}
