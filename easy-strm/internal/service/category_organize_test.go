package service

import (
	"easy-strm/internal/domain"
	"path/filepath"
	"strings"
	"testing"
)

// TestOrganizeService_MatchCategoryPath 测试基于规则关键词动态推断存放路径
func TestOrganizeService_MatchCategoryPath(t *testing.T) {
	svc := &OrganizeService{}

	// 模拟 TMDB 返回信息，模拟标题包含关键词
	identifyResult := &domain.TmdbIdentifyResult{
		Title:         "孤独摇滚",
		OriginalTitle: "Bocchi the Rock!",
		MediaType:     "tv",
	}

	// 模拟设定的三个分类
	categories := []*domain.MediaCategory{
		{
			Name:       "电影归档",
			MediaType:  "movie",
			Enabled:    true,
			MatchRules: []byte(`{"keywords": ["4k", "movie"]}`),
			TargetPath: "D:/Movies",
		},
		{
			// 我们期待命中这个
			Name:       "动漫归档",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"keywords": ["动漫", "bocchi"]}`),
			TargetPath: "D:/Anime",
		},
		{
			// 兜底类别为空的情况
			Name:       "未分类剧集兜底",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"keywords": []}`),
			TargetPath: "D:/TV_Shows",
		},
	}

	// 触发匹配逻辑
	targetPath := svc.matchCategoryPath(identifyResult, categories)

	// 检查归档目录是否计算正确
	if targetPath != "D:/Anime" {
		t.Errorf("分类动态路径匹配失败: 期待 %s, 得到 %s", "D:/Anime", targetPath)
	}
}

// TestOrganizeService_MatchCategoryPathByCountryAndGenre 测试图中剧集分类的国家和类型组合规则
func TestOrganizeService_MatchCategoryPathByCountryAndGenre(t *testing.T) {
	svc := &OrganizeService{}

	identifyResult := &domain.TmdbIdentifyResult{
		Title:     "雾山五行",
		MediaType: "tv",
		GenreIDs:  []int{16},
		Countries: []string{"CN"},
		Language:  "zh",
	}

	categories := []*domain.MediaCategory{
		{
			Name:       "日番",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"genre_ids":[16],"countries":["JP"]}`),
			TargetPath: "/电视剧/日番",
		},
		{
			Name:       "国漫",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"genre_ids":[16],"countries":["CN","TW","HK"]}`),
			TargetPath: "/电视剧/国漫",
		},
		{
			Name:       "未分类",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"default":true}`),
			TargetPath: "/电视剧/未分类",
		},
	}

	targetPath := svc.matchCategoryPath(identifyResult, categories)
	if targetPath != "/电视剧/国漫" {
		t.Errorf("国家和类型组合规则匹配失败: 期待 %s, 得到 %s", "/电视剧/国漫", targetPath)
	}
}

// TestOrganizeService_MatchCategoryPathDefaultFallback 测试未命中图中分类时归到未分类
func TestOrganizeService_MatchCategoryPathDefaultFallback(t *testing.T) {
	svc := &OrganizeService{}

	identifyResult := &domain.TmdbIdentifyResult{
		Title:     "Unknown Show",
		MediaType: "tv",
		GenreIDs:  []int{18},
		Countries: []string{"BR"},
		Language:  "pt",
	}

	categories := []*domain.MediaCategory{
		{
			Name:       "国漫",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"genre_ids":[16],"countries":["CN","TW","HK"]}`),
			TargetPath: "/电视剧/国漫",
		},
		{
			Name:       "未分类",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"default":true}`),
			TargetPath: "/电视剧/未分类",
		},
	}

	targetPath := svc.matchCategoryPath(identifyResult, categories)
	if targetPath != "/电视剧/未分类" {
		t.Errorf("未分类兜底失败: 期待 %s, 得到 %s", "/电视剧/未分类", targetPath)
	}
}

func TestOrganizeService_PrependCategoryTargetPath(t *testing.T) {
	svc := &OrganizeService{}

	got := svc.prependCategoryTargetPath("D:/Output", "/电视剧/国漫")
	want := "D:/Output/国漫"
	if filepath.ToSlash(got) != want {
		t.Fatalf("分类目录应追加到目标目录前缀: got=%q, want=%q", filepath.ToSlash(got), want)
	}

	got = svc.prependCategoryTargetPath("D:/Output", "E:/Anime")
	want = "D:/Output/Anime"
	if filepath.ToSlash(got) != want {
		t.Fatalf("带盘符分类目录应转为相对前缀: got=%q, want=%q", filepath.ToSlash(got), want)
	}
}

func TestOrganizeService_动画电影分类目录前缀(t *testing.T) {
	svc := &OrganizeService{}

	identifyResult := &domain.TmdbIdentifyResult{
		Title:     "哆啦A梦：大雄的恐龙",
		MediaType: "movie",
		GenreIDs:  []int{16},
	}
	categories := []*domain.MediaCategory{
		{
			Name:       "动画电影",
			MediaType:  "movie",
			Enabled:    true,
			MatchRules: []byte(`{"genre_ids":[16]}`),
			TargetPath: "/电影/动画电影",
		},
		{
			Name:       "未分类",
			MediaType:  "movie",
			Enabled:    true,
			MatchRules: []byte(`{"default":true}`),
			TargetPath: "/电影/未分类",
		},
	}

	categoryPath := svc.matchCategoryPath(identifyResult, categories)
	if categoryPath != "/电影/动画电影" {
		t.Fatalf("动画电影分类未命中: got=%q", categoryPath)
	}

	got := svc.prependCategoryTargetPath("C:/debug/test", categoryPath)
	want := "C:/debug/test/动画电影"
	if filepath.ToSlash(got) != want {
		t.Fatalf("动画电影分类目录前缀不匹配: got=%q, want=%q", filepath.ToSlash(got), want)
	}
}

func TestOrganizeService_更具体的分类规则优先(t *testing.T) {
	svc := &OrganizeService{}

	identifyResult := &domain.TmdbIdentifyResult{
		Title:         "药屋少女的呢喃",
		OriginalTitle: "薬屋のひとりごと",
		MediaType:     "tv",
		GenreIDs:      []int{16, 18},
		Countries:     []string{"JP"},
		Language:      "ja",
	}

	categories := []*domain.MediaCategory{
		{
			Name:       "日韩剧",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"countries":["JP","KR"],"languages":["ja","ko"]}`),
			TargetPath: "/电视剧/日韩剧",
		},
		{
			Name:       "日番",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"genre_ids":[16],"countries":["JP"]}`),
			TargetPath: "/电视剧/日番",
		},
		{
			Name:       "未分类",
			MediaType:  "tv",
			Enabled:    true,
			MatchRules: []byte(`{"default":true}`),
			TargetPath: "/电视剧/未分类",
		},
	}

	targetPath := svc.matchCategoryPath(identifyResult, categories)
	if targetPath != "/电视剧/日番" {
		t.Fatalf("更具体的分类规则应优先命中日番: got=%q", targetPath)
	}
}

func TestOrganizeService_关键词规则仍需满足其他条件(t *testing.T) {
	svc := &OrganizeService{}

	identifyResult := &domain.TmdbIdentifyResult{
		Title:         "药屋少女的呢喃",
		OriginalTitle: "薬屋のひとりごと",
		MediaType:     "tv",
		GenreIDs:      []int{16},
		Countries:     []string{"JP"},
		Language:      "ja",
	}

	rule := &domain.CategoryMatchRule{
		Keywords:  []string{"药屋"},
		Countries: []string{"KR"},
	}

	if svc.matchCategoryRule(identifyResult, strings.ToLower(identifyResult.Title+" "+identifyResult.OriginalTitle), rule) {
		t.Fatal("关键词命中后仍应继续校验其它条件，国家不匹配时不应命中")
	}
}
