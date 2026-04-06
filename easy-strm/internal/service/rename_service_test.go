package service

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

// ============================================
// RenameService 模板引擎测试
// ============================================

// TestApplyTemplate_电影模板 测试电影命名模板的变量替换
func TestApplyTemplate_电影模板(t *testing.T) {
	svc := &RenameService{}

	cases := []struct {
		name     string
		template string
		title    string
		year     int
		want     string
	}{
		{
			name:     "默认电影模板",
			template: "{{ title }}{% if year %} ({{ year }}){% endif %}",
			title:    "Inception",
			year:     2010,
			want:     "Inception (2010)",
		},
		{
			name:     "带质量的模板",
			template: "{{ title }}{% if year %} ({{ year }}){% endif %}{% if videoFormat %}.{{ videoFormat }}{% endif %}",
			title:    "Interstellar",
			year:     2014,
			want:     "Interstellar (2014)", // quality为空时应清理
		},
		{
			name:     "纯标题模板",
			template: "{{ title }}",
			title:    "流浪地球",
			year:     2019,
			want:     "流浪地球",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.applyTemplate(tc.template, tc.title, "", tc.year, 0, 0, "", "", "", "", 0)
			if err != nil {
				t.Fatalf("妯℃澘娓叉煋澶辫触: %v", err)
			}
			if got != tc.want {
				t.Errorf("模板结果不匹配: got=%q, want=%q", got, tc.want)
			}
		})
	}
}

// TestApplyTemplate_剧集模板 测试剧集命名模板的变量替换
func TestApplyTemplate_剧集模板(t *testing.T) {
	svc := &RenameService{}

	cases := []struct {
		name     string
		template string
		title    string
		season   int
		episode  int
		want     string
	}{
		{
			name:     "默认剧集模板",
			template: `{{ title }}/S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}`,
			title:    "Breaking Bad",
			season:   1,
			episode:  2,
			want:     "Breaking Bad/S01E02",
		},
		{
			name:     "带年份的剧集模板",
			template: `{{ title }}{% if year %} ({{ year }}){% endif %} S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}`,
			title:    "Friends",
			season:   3,
			episode:  14,
			want:     "Friends S03E14",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.applyTemplate(tc.template, tc.title, "", 0, tc.season, tc.episode, "", "", "", "", 0)
			if err != nil {
				t.Fatalf("妯℃澘娓叉煋澶辫触: %v", err)
			}
			if got != tc.want {
				t.Errorf("模板结果不匹配: got=%q, want=%q", got, tc.want)
			}
		})
	}
}

// TestGetDefaultTemplate_电影 测试获取默认电影模板
func TestGetDefaultTemplate_电影(t *testing.T) {
	svc := &RenameService{}
	tmpl := svc.getDefaultTemplate("movie")
	want := `{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}`
	if tmpl != want {
		t.Errorf("默认电影模板不匹配: got=%q", tmpl)
	}
}

// TestGetDefaultTemplate_剧集 测试获取默认剧集模板
func TestGetDefaultTemplate_剧集(t *testing.T) {
	svc := &RenameService{}
	tmpl := svc.getDefaultTemplate("tv")
	want := `{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}`
	if tmpl != want {
		t.Errorf("默认剧集模板不匹配: got=%q", tmpl)
	}
}

func TestApplyTemplate_Jinja2默认电影模板完整渲染(t *testing.T) {
	svc := &RenameService{}
	template := svc.getDefaultTemplate("movie")

	got, err := svc.applyTemplate(template, "盗梦空间", "Inception", 2010, 0, 0, "1080p", "", "", ".mkv", 27205)
	if err != nil {
		t.Fatalf("渲染 Jinja2 电影默认模板失败: %v", err)
	}

	want := "盗梦空间 (2010)/盗梦空间 - Inception (2010) [1080p].mkv"
	if got != want {
		t.Fatalf("Jinja2 电影默认模板渲染结果不匹配: got=%q, want=%q", got, want)
	}
}

func TestApplyTemplate_Jinja2默认剧集模板完整渲染(t *testing.T) {
	svc := &RenameService{}
	template := svc.getDefaultTemplate("tv")

	got, err := svc.applyTemplate(template, "绝命毒师", "Breaking Bad", 2008, 1, 2, "1080p", "", "", ".mkv", 1396)
	if err != nil {
		t.Fatalf("渲染 Jinja2 剧集默认模板失败: %v", err)
	}

	want := "绝命毒师 (2008)/Season 01/绝命毒师 - Breaking Bad - S01E02 [1080p].mkv"
	if got != want {
		t.Fatalf("Jinja2 剧集默认模板渲染结果不匹配: got=%q, want=%q", got, want)
	}
}

// TestGetDefaultTemplate_优先读取系统配置 测试系统配置中的模板优先级高于内置默认值
func TestGetDefaultTemplate_优先读取系统配置(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	svc := &RenameService{
		systemConfigDAO: dao.NewSystemConfigDAO(),
	}

	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "config_key", "config_val", "create_time", "update_time"}).
		AddRow(1, "movie_naming_template", "{{ title }} - {{ year }}", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, config_key, config_val, create_time, update_time 
		 FROM t_system_config WHERE config_key = $1`)).
		WithArgs("movie_naming_template").
		WillReturnRows(rows)

	tmpl := svc.getDefaultTemplate("movie")
	if tmpl != "{{ title }} - {{ year }}" {
		t.Fatalf("系统配置模板未生效: got=%q", tmpl)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}

// ============================================
// RenameService 季集解析测试
// ============================================

// TestParseSeasonEpisode_标准格式 测试标准季集格式解析
func TestParseSeasonEpisode_标准格式(t *testing.T) {
	svc := &RenameService{}

	cases := []struct {
		name        string
		filename    string
		wantSeason  int
		wantEpisode int
	}{
		{"S01E02", "Movie.S01E02.720p.mkv", 1, 2},
		{"S12E99", "Show.S12E99.1080p.mp4", 12, 99},
		{"1x05", "House.1x05.720p.mkv", 1, 5},
		{"EP03", "三体.EP03.mkv", 1, 3},
		{"无季集信息", "Movie.2020.1080p.mkv", 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := svc.parseSeasonEpisode(tc.filename)
			if parsed.Season != tc.wantSeason {
				t.Errorf("季数不匹配: got=%d, want=%d", parsed.Season, tc.wantSeason)
			}
			if parsed.Episode != tc.wantEpisode {
				t.Errorf("集数不匹配: got=%d, want=%d", parsed.Episode, tc.wantEpisode)
			}
		})
	}
}

// TestParseSeasonEpisode_质量来源编码 测试从文件名中提取质量、来源、编码
func TestParseSeasonEpisode_质量来源编码(t *testing.T) {
	svc := &RenameService{}

	parsed := svc.parseSeasonEpisode("Show.S01E01.1080p.BluRay.x264.mkv")
	if parsed.Quality != "1080p" {
		t.Errorf("质量不匹配: got=%q", parsed.Quality)
	}
	if parsed.Source != "BluRay" {
		t.Errorf("来源不匹配: got=%q", parsed.Source)
	}
	if parsed.Codec != "x264" {
		t.Errorf("编码不匹配: got=%q", parsed.Codec)
	}
}

// ============================================
// RenameService 标题清理测试
// ============================================

// TestCleanTitle_移除标签 测试标签清理功能
func TestCleanTitle_移除标签(t *testing.T) {
	svc := &RenameService{}

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "移除质量标签",
			input: "The Movie 1080p BluRay x264",
			want:  "The Movie",
		},
		{
			name:  "移除季集标签",
			input: "Breaking Bad S01E02",
			want:  "Breaking Bad",
		},
		{
			name:  "中文标题不受影响",
			input: "流浪地球",
			want:  "流浪地球",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.cleanTitle(tc.input)
			if got != tc.want {
				t.Errorf("清理结果不匹配: got=%q, want=%q", got, tc.want)
			}
		})
	}
}

// ============================================
// RenameService 预览与执行测试
// ============================================

// TestPreviewRename_本地文件_保留扩展名 测试预览时保留扩展名
func TestPreviewRename_本地文件_保留扩展名(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Movie.2020.1080p.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, nil, 10, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	req := &domain.RenamePreviewRequest{
		SourceID:  1,
		FileID:    "Movie.2020.1080p.mkv",
		MediaType: "movie",
		Template:  "{name} ({year})",
	}

	result, err := renameSvc.PreviewRename(req)
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}

	// 验证扩展名保留
	if filepath.Ext(result.NewName) != ".mkv" {
		t.Errorf("应保留扩展名.mkv, got=%q", result.NewName)
	}
	// 验证原始文件名
	if result.OriginalName != "Movie.2020.1080p.mkv" {
		t.Errorf("原始文件名不匹配: got=%q", result.OriginalName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}

// TestPreviewRename_空模板时使用系统配置 测试整理链路不传模板时会回退到系统配置中的规则
func TestPreviewRename_空模板时使用系统配置(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Movie.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, dao.NewSystemConfigDAO())

	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, nil, 10, true, now, now)
	configRows := sqlmock.NewRows([]string{"id", "config_key", "config_val", "create_time", "update_time"}).
		AddRow(1, "movie_naming_template", "{{ title }} - {{ year }}", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, config_key, config_val, create_time, update_time 
		 FROM t_system_config WHERE config_key = $1`)).
		WithArgs("movie_naming_template").
		WillReturnRows(configRows)

	req := &domain.RenamePreviewRequest{
		SourceID:  1,
		FileID:    "Movie.mkv",
		MediaType: "movie",
		Template:  "",
	}

	result, err := renameSvc.PreviewRename(req)
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}

	if result.NewName != "Movie - 0.mkv" {
		t.Fatalf("系统配置模板未用于生成新文件名: got=%q", result.NewName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}

// TestExecuteRename_本地文件_成功 测试本地文件更名执行成功
func TestExecuteRename_本地文件_成功(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	originalFile := filepath.Join(root, "old_name.mkv")
	if err := os.WriteFile(originalFile, []byte("video"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, nil, 10, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID:  1,
		FileID:    "old_name.mkv",
		NewName:   "New Name.mkv",
		Overwrite: false,
	}

	result, err := renameSvc.ExecuteRename(req)
	if err != nil {
		t.Fatalf("执行更名失败: %v", err)
	}
	if !result.Success {
		t.Fatalf("更名应当成功: %s", result.Message)
	}

	// 验证新文件存在
	newFile := filepath.Join(root, "New Name.mkv")
	if _, err := os.Stat(newFile); err != nil {
		t.Errorf("新文件应当存在: %v", err)
	}
	// 验证旧文件已不存在
	if _, err := os.Stat(originalFile); !os.IsNotExist(err) {
		t.Errorf("旧文件应当已不存在")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}

// TestExecuteRename_本地文件_自动补扩展名 测试没有扩展名时自动补齐
func TestExecuteRename_本地文件_自动补扩展名(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, nil, 10, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID: 1,
		FileID:   "movie.mp4",
		NewName:  "renamed_movie", // 不带扩展名
	}

	result, err := renameSvc.ExecuteRename(req)
	if err != nil {
		t.Fatalf("执行更名失败: %v", err)
	}
	if !result.Success {
		t.Fatalf("更名应当成功: %s", result.Message)
	}

	// 验证自动补了 .mp4 扩展名
	renamedFile := filepath.Join(root, "renamed_movie.mp4")
	if _, err := os.Stat(renamedFile); err != nil {
		t.Errorf("自动补扩展名后文件应存在: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}

// TestExecuteRename_目标已存在_不覆盖 测试目标文件已存在且不覆盖时返回错误
func TestExecuteRename_目标已存在_不覆盖(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.mkv"), []byte("source"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "target.mkv"), []byte("exists"), 0644); err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, nil, 10, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID:  1,
		FileID:    "source.mkv",
		NewName:   "target.mkv",
		Overwrite: false,
	}

	_, err := renameSvc.ExecuteRename(req)
	if err == nil {
		t.Fatal("目标文件已存在且不覆盖时应返回错误")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}

// TestExecuteRename_目标已存在_覆盖 测试目标文件已存在且覆盖时成功
func TestExecuteRename_目标已存在_覆盖(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.mkv"), []byte("new-content"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "target.mkv"), []byte("old-content"), 0644); err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, nil, 10, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID:  1,
		FileID:    "source.mkv",
		NewName:   "target.mkv",
		Overwrite: true,
	}

	result, err := renameSvc.ExecuteRename(req)
	if err != nil {
		t.Fatalf("覆盖更名应成功: %v", err)
	}
	if !result.Success {
		t.Fatalf("覆盖更名应成功: %s", result.Message)
	}

	// 验证文件内容已被覆盖
	content, _ := os.ReadFile(filepath.Join(root, "target.mkv"))
	if string(content) != "new-content" {
		t.Errorf("文件内容应当被覆盖: got=%q", string(content))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}

// TestExecuteRename_115云盘_不支持 测试115云盘更名返回不支持
func TestExecuteRename_115云盘_不支持(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cloud115ID := 1
	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "cloud", domain.SourceTypeCloud115, "0", &cloud115ID, 10, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID: 1,
		FileID:   "test.mkv",
		NewName:  "new.mkv",
	}

	_, err := renameSvc.ExecuteRename(req)
	if err == nil {
		t.Fatal("115云盘更名应返回不支持")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}

// TestExecuteRename_源文件不存在 测试源文件不存在时返回错误
func TestExecuteRename_源文件不存在(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, nil, 10, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID: 1,
		FileID:   "nonexistent.mkv",
		NewName:  "new.mkv",
	}

	_, err := renameSvc.ExecuteRename(req)
	if err == nil {
		t.Fatal("源文件不存在时应返回错误")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未满足的期望: %v", err)
	}
}
