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

const testMediaSourceColumns = "id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time"

func newMediaSourceRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled",
		"organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id",
		"create_time", "update_time",
	})
}

func expectMediaSourceByID(mock sqlmock.Sqlmock, sourceID int, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + testMediaSourceColumns + ` FROM t_media_source WHERE id = $1`)).
		WithArgs(sourceID).
		WillReturnRows(rows)
}

// ============================================
// RenameService 模锟斤拷锟斤拷锟斤拷锟斤拷锟?
// ============================================

// TestApplyTemplate_锟斤拷影模锟斤拷 锟斤拷锟皆碉拷影锟斤拷锟斤拷模锟斤拷谋锟斤拷锟斤拷婊?
func TestApplyTemplateMovie(t *testing.T) {
	svc := &RenameService{}

	cases := []struct {
		name     string
		template string
		title    string
		year     int
		want     string
	}{
		{
			name:     "默锟较碉拷影模锟斤拷",
			template: "{{ title }}{% if year %} ({{ year }}){% endif %}",
			title:    "Inception",
			year:     2010,
			want:     "Inception (2010)",
		},
		{
			name:     "锟斤拷锟斤拷锟斤拷锟斤拷模锟斤拷",
			template: "{{ title }}{% if year %} ({{ year }}){% endif %}{% if videoFormat %}.{{ videoFormat }}{% endif %}",
			title:    "Interstellar",
			year:     2014,
			want:     "Interstellar (2014)", // quality为锟斤拷时应锟斤拷锟斤拷
		},
		{
			name:     "锟斤拷锟斤拷锟斤拷模锟斤拷",
			template: "{{ title }}",
			title:    "锟斤拷锟剿碉拷锟斤拷",
			year:     2019,
			want:     "锟斤拷锟剿碉拷锟斤拷",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.applyTemplate(tc.template, tc.title, "", tc.year, 0, 0, "", "", "", "", 0)
			if err != nil {
				t.Fatalf("妯℃澘娓叉煋澶辫触: %v", err)
			}
			if got != tc.want {
				t.Errorf("模锟斤拷锟斤拷锟斤拷匹锟斤拷: got=%q, want=%q", got, tc.want)
			}
		})
	}
}

// TestApplyTemplate_锟界集模锟斤拷 锟斤拷锟皆剧集锟斤拷锟斤拷模锟斤拷谋锟斤拷锟斤拷婊?
func TestApplyTemplate_锟界集模锟斤拷(t *testing.T) {
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
			name:     "默锟较剧集模锟斤拷",
			template: `{{ title }}/S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}`,
			title:    "Breaking Bad",
			season:   1,
			episode:  2,
			want:     "Breaking Bad/S01E02",
		},
		{
            name:     "Season template with year",
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
				t.Errorf("模锟斤拷锟斤拷锟斤拷匹锟斤拷: got=%q, want=%q", got, tc.want)
			}
		})
	}
}

// TestGetDefaultTemplate_锟斤拷影 锟斤拷锟皆伙拷取默锟较碉拷影模锟斤拷
func TestGetDefaultTemplate_锟斤拷影(t *testing.T) {
	svc := &RenameService{}
	tmpl := svc.getDefaultTemplate("movie")
	want := defaultMovieTemplate
	if tmpl != want {
		t.Errorf("默锟较碉拷影模锟藉不匹锟斤拷: got=%q", tmpl)
	}
}

// TestGetDefaultTemplate_锟界集 锟斤拷锟皆伙拷取默锟较剧集模锟斤拷
func TestGetDefaultTemplate_锟界集(t *testing.T) {
	svc := &RenameService{}
	tmpl := svc.getDefaultTemplate("tv")
	want := defaultTVTemplate
	if tmpl != want {
		t.Errorf("默锟较剧集模锟藉不匹锟斤拷: got=%q", tmpl)
	}
}

func TestApplyTemplate_Jinja2默锟较碉拷影模锟斤拷锟斤拷锟斤拷锟斤拷染(t *testing.T) {
	svc := &RenameService{}
	template := svc.getDefaultTemplate("movie")

	got, err := svc.applyTemplate(template, "锟斤拷锟轿空硷拷", "Inception", 2010, 0, 0, "1080p", "", "", ".mkv", 27205)
	if err != nil {
		t.Fatalf("锟斤拷染 Jinja2 锟斤拷影默锟斤拷模锟斤拷失锟斤拷: %v", err)
	}

	want := "锟斤拷锟轿空硷拷 (2010)/锟斤拷锟轿空硷拷 (2010) [1080p].mkv"
	if got != want {
		t.Fatalf("Jinja2 锟斤拷影默锟斤拷模锟斤拷锟斤拷染锟斤拷锟斤拷锟狡ワ拷锟? got=%q, want=%q", got, want)
	}
}

func TestApplyTemplate_Jinja2默锟较剧集模锟斤拷锟斤拷锟斤拷锟斤拷染(t *testing.T) {
	svc := &RenameService{}
	template := svc.getDefaultTemplate("tv")

	got, err := svc.applyTemplate(template, "锟斤拷锟斤拷锟斤拷师", "Breaking Bad", 2008, 1, 2, "1080p", "", "", ".mkv", 1396)
	if err != nil {
		t.Fatalf("锟斤拷染 Jinja2 锟界集默锟斤拷模锟斤拷失锟斤拷: %v", err)
	}

	want := "锟斤拷锟斤拷锟斤拷师 (2008)/Season 01/锟斤拷锟斤拷锟斤拷师 - S01E02 [1080p].mkv"
	if got != want {
		t.Fatalf("Jinja2 锟界集默锟斤拷模锟斤拷锟斤拷染锟斤拷锟斤拷锟狡ワ拷锟? got=%q, want=%q", got, want)
	}
}

func TestNormalizeBuiltinTemplate_锟斤拷锟捷旧官凤拷模锟斤拷(t *testing.T) {
	svc := &RenameService{}

	if got := svc.normalizeBuiltinTemplate(legacyDefaultMovieTemplate, "movie"); got != defaultMovieTemplate {
		t.Fatalf("锟缴碉拷影锟劫凤拷模锟斤拷锟斤拷锟绞э拷锟? got=%q", got)
	}

	if got := svc.normalizeBuiltinTemplate(legacyDefaultTVTemplate, "tv"); got != defaultTVTemplate {
		t.Fatalf("锟缴剧集锟劫凤拷模锟斤拷锟斤拷锟绞э拷锟? got=%q", got)
	}
}

// TestGetDefaultTemplate_锟斤拷锟饺讹拷取系统锟斤拷锟斤拷 锟斤拷锟斤拷系统锟斤拷锟斤拷锟叫碉拷模锟斤拷锟斤拷锟饺硷拷锟斤拷锟斤拷锟斤拷锟斤拷默锟斤拷值
func TestGetDefaultTemplate_锟斤拷锟饺讹拷取系统锟斤拷锟斤拷(t *testing.T) {
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
		t.Fatalf("系统锟斤拷锟斤拷模锟斤拷未锟斤拷效: got=%q", tmpl)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

// ============================================
// RenameService 锟斤拷锟斤拷锟斤拷锟斤拷锟斤拷锟斤拷
// ============================================

// TestParseSeasonEpisode_锟斤拷准锟斤拷式 锟斤拷锟皆憋拷准锟斤拷锟斤拷锟斤拷式锟斤拷锟斤拷
func TestParseSeasonEpisode_锟斤拷准锟斤拷式(t *testing.T) {
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
		{"EP03", "锟斤拷锟斤拷.EP03.mkv", 1, 3},
		{"锟睫硷拷锟斤拷锟斤拷息", "Movie.2020.1080p.mkv", 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := svc.parseSeasonEpisode(tc.filename)
			if parsed.Season != tc.wantSeason {
				t.Errorf("锟斤拷锟斤拷锟斤拷匹锟斤拷: got=%d, want=%d", parsed.Season, tc.wantSeason)
			}
			if parsed.Episode != tc.wantEpisode {
				t.Errorf("锟斤拷锟斤拷锟斤拷匹锟斤拷: got=%d, want=%d", parsed.Episode, tc.wantEpisode)
			}
		})
	}
}

// TestParseSeasonEpisode_锟斤拷锟斤拷锟斤拷源锟斤拷锟斤拷 锟斤拷锟皆达拷锟侥硷拷锟斤拷锟斤拷锟斤拷取锟斤拷锟斤拷锟斤拷锟斤拷源锟斤拷锟斤拷锟斤拷
func TestParseSeasonEpisode_锟斤拷锟斤拷锟斤拷源锟斤拷锟斤拷(t *testing.T) {
	svc := &RenameService{}

	parsed := svc.parseSeasonEpisode("Show.S01E01.1080p.BluRay.x264.mkv")
	if parsed.Quality != "1080p" {
		t.Errorf("锟斤拷锟斤拷锟斤拷匹锟斤拷: got=%q", parsed.Quality)
	}
	if parsed.Source != "BluRay" {
		t.Errorf("锟斤拷源锟斤拷匹锟斤拷: got=%q", parsed.Source)
	}
	if parsed.Codec != "x264" {
		t.Errorf("锟斤拷锟诫不匹锟斤拷: got=%q", parsed.Codec)
	}
}

// ============================================
// RenameService 锟斤拷锟斤拷锟斤拷锟斤拷锟斤拷锟?
// ============================================

// TestCleanTitle_锟狡筹拷锟斤拷签 锟斤拷锟皆憋拷签锟斤拷锟斤拷锟斤拷锟?
func TestCleanTitle_锟狡筹拷锟斤拷签(t *testing.T) {
	svc := &RenameService{}

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "锟狡筹拷锟斤拷锟斤拷锟斤拷签",
			input: "The Movie 1080p BluRay x264",
			want:  "The Movie",
		},
		{
			name:  "锟狡筹拷锟斤拷锟斤拷锟斤拷签",
			input: "Breaking Bad S01E02",
			want:  "Breaking Bad",
		},
		{
			name:  "锟斤拷锟侥憋拷锟解不锟斤拷影锟斤拷",
			input: "锟斤拷锟剿碉拷锟斤拷",
			want:  "锟斤拷锟剿碉拷锟斤拷",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.cleanTitle(tc.input)
			if got != tc.want {
				t.Errorf("锟斤拷锟斤拷锟斤拷锟斤拷匹锟斤拷: got=%q, want=%q", got, tc.want)
			}
		})
	}
}

// ============================================
// RenameService 预锟斤拷锟斤拷执锟叫诧拷锟斤拷
// ============================================

// TestPreviewRename_锟斤拷锟斤拷锟侥硷拷_锟斤拷锟斤拷锟斤拷展锟斤拷 锟斤拷锟斤拷预锟斤拷时锟斤拷锟斤拷锟斤拷展锟斤拷
func TestPreviewRename_锟斤拷锟斤拷锟侥硷拷_锟斤拷锟斤拷锟斤拷展锟斤拷(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Movie.2020.1080p.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("锟斤拷锟斤拷锟斤拷锟斤拷锟侥硷拷失锟斤拷: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := newMediaSourceRows().
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 0, "", now, now)
	expectMediaSourceByID(mock, 1, sourceRows)

	req := &domain.RenamePreviewRequest{
		SourceID:  1,
		FileID:    "Movie.2020.1080p.mkv",
		MediaType: "movie",
		Template:  "{name} ({year})",
	}

	result, err := renameSvc.PreviewRename(req)
	if err != nil {
		t.Fatalf("预锟斤拷失锟斤拷: %v", err)
	}

	// 锟斤拷证锟斤拷展锟斤拷锟斤拷锟斤拷
	if filepath.Ext(result.NewName) != ".mkv" {
		t.Errorf("应锟斤拷锟斤拷锟斤拷展锟斤拷.mkv, got=%q", result.NewName)
	}
	// 锟斤拷证原始锟侥硷拷锟斤拷
	if result.OriginalName != "Movie.2020.1080p.mkv" {
		t.Errorf("原始锟侥硷拷锟斤拷锟斤拷匹锟斤拷: got=%q", result.OriginalName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

// TestPreviewRename_锟斤拷模锟斤拷时使锟斤拷系统锟斤拷锟斤拷 锟斤拷锟斤拷锟斤拷锟斤拷锟斤拷路锟斤拷锟斤拷模锟斤拷时锟斤拷锟斤拷说锟较低筹拷锟斤拷锟斤拷械墓锟斤拷锟?
func TestPreviewRename_锟斤拷模锟斤拷时使锟斤拷系统锟斤拷锟斤拷(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Movie.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("锟斤拷锟斤拷锟斤拷锟斤拷锟侥硷拷失锟斤拷: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, dao.NewSystemConfigDAO())

	now := time.Now()
	sourceRows := newMediaSourceRows().
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 0, "", now, now)
	configRows := sqlmock.NewRows([]string{"id", "config_key", "config_val", "create_time", "update_time"}).
		AddRow(1, "movie_naming_template", "{{ title }} - {{ year }}", now, now)
	expectMediaSourceByID(mock, 1, sourceRows)

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
		t.Fatalf("预锟斤拷失锟斤拷: %v", err)
	}

	if result.NewName != "Movie - 0.mkv" {
		t.Fatalf("系统锟斤拷锟斤拷模锟斤拷未锟斤拷锟斤拷锟斤拷锟斤拷锟斤拷锟侥硷拷锟斤拷: got=%q", result.NewName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

// TestExecuteRename_锟斤拷锟斤拷锟侥硷拷_锟缴癸拷 锟斤拷锟皆憋拷锟斤拷锟侥硷拷锟斤拷锟斤拷执锟叫成癸拷
func TestExecuteRename_锟斤拷锟斤拷锟侥硷拷_锟缴癸拷(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	originalFile := filepath.Join(root, "old_name.mkv")
	if err := os.WriteFile(originalFile, []byte("video"), 0644); err != nil {
		t.Fatalf("锟斤拷锟斤拷锟斤拷锟斤拷锟侥硷拷失锟斤拷: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := newMediaSourceRows().
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 0, "", now, now)
	expectMediaSourceByID(mock, 1, sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID:  1,
		FileID:    "old_name.mkv",
		NewName:   "New Name.mkv",
		Overwrite: false,
	}

	result, err := renameSvc.ExecuteRename(req)
	if err != nil {
		t.Fatalf("执锟叫革拷锟斤拷失锟斤拷: %v", err)
	}
	if !result.Success {
		t.Fatalf("锟斤拷锟斤拷应锟斤拷锟缴癸拷: %s", result.Message)
	}

	// 锟斤拷证锟斤拷锟侥硷拷锟斤拷锟斤拷
	newFile := filepath.Join(root, "New Name.mkv")
	if _, err := os.Stat(newFile); err != nil {
		t.Errorf("锟斤拷锟侥硷拷应锟斤拷锟斤拷锟斤拷: %v", err)
	}
	// 锟斤拷证锟斤拷锟侥硷拷锟窖诧拷锟斤拷锟斤拷
	if _, err := os.Stat(originalFile); !os.IsNotExist(err) {
		t.Errorf("锟斤拷锟侥硷拷应锟斤拷锟窖诧拷锟斤拷锟斤拷")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

// TestExecuteRename_锟斤拷锟斤拷锟侥硷拷_锟皆讹拷锟斤拷锟斤拷展锟斤拷 锟斤拷锟斤拷没锟斤拷锟斤拷展锟斤拷时锟皆讹拷锟斤拷锟斤拷
func TestExecuteRename_锟斤拷锟斤拷锟侥硷拷_锟皆讹拷锟斤拷锟斤拷展锟斤拷(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("锟斤拷锟斤拷锟斤拷锟斤拷锟侥硷拷失锟斤拷: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := newMediaSourceRows().
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 0, "", now, now)
	expectMediaSourceByID(mock, 1, sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID: 1,
		FileID:   "movie.mp4",
		NewName:  "renamed_movie", // 锟斤拷锟斤拷锟斤拷展锟斤拷
	}

	result, err := renameSvc.ExecuteRename(req)
	if err != nil {
		t.Fatalf("执锟叫革拷锟斤拷失锟斤拷: %v", err)
	}
	if !result.Success {
		t.Fatalf("锟斤拷锟斤拷应锟斤拷锟缴癸拷: %s", result.Message)
	}

	// 锟斤拷证锟皆讹拷锟斤拷锟斤拷 .mp4 锟斤拷展锟斤拷
	renamedFile := filepath.Join(root, "renamed_movie.mp4")
	if _, err := os.Stat(renamedFile); err != nil {
		t.Errorf("锟皆讹拷锟斤拷锟斤拷展锟斤拷锟斤拷锟侥硷拷应锟斤拷锟斤拷: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

// TestExecuteRename_目锟斤拷锟窖达拷锟斤拷_锟斤拷锟斤拷锟斤拷 锟斤拷锟斤拷目锟斤拷锟侥硷拷锟窖达拷锟斤拷锟揭诧拷锟斤拷锟斤拷时锟斤拷锟截达拷锟斤拷
func TestExecuteRename_目锟斤拷锟窖达拷锟斤拷_锟斤拷锟斤拷锟斤拷(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.mkv"), []byte("source"), 0644); err != nil {
		t.Fatalf("锟斤拷锟斤拷源锟侥硷拷失锟斤拷: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "target.mkv"), []byte("exists"), 0644); err != nil {
		t.Fatalf("锟斤拷锟斤拷目锟斤拷锟侥硷拷失锟斤拷: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := newMediaSourceRows().
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 0, "", now, now)
	expectMediaSourceByID(mock, 1, sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID:  1,
		FileID:    "source.mkv",
		NewName:   "target.mkv",
		Overwrite: false,
	}

	_, err := renameSvc.ExecuteRename(req)
	if err == nil {
		t.Fatal("目锟斤拷锟侥硷拷锟窖达拷锟斤拷锟揭诧拷锟斤拷锟斤拷时应锟斤拷锟截达拷锟斤拷")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

// TestExecuteRename_目锟斤拷锟窖达拷锟斤拷_锟斤拷锟斤拷 锟斤拷锟斤拷目锟斤拷锟侥硷拷锟窖达拷锟斤拷锟揭革拷锟斤拷时锟缴癸拷
func TestExecuteRename_目锟斤拷锟窖达拷锟斤拷_锟斤拷锟斤拷(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.mkv"), []byte("new-content"), 0644); err != nil {
		t.Fatalf("锟斤拷锟斤拷源锟侥硷拷失锟斤拷: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "target.mkv"), []byte("old-content"), 0644); err != nil {
		t.Fatalf("锟斤拷锟斤拷目锟斤拷锟侥硷拷失锟斤拷: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := newMediaSourceRows().
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 0, "", now, now)
	expectMediaSourceByID(mock, 1, sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID:  1,
		FileID:    "source.mkv",
		NewName:   "target.mkv",
		Overwrite: true,
	}

	result, err := renameSvc.ExecuteRename(req)
	if err != nil {
		t.Fatalf("锟斤拷锟角革拷锟斤拷应锟缴癸拷: %v", err)
	}
	if !result.Success {
		t.Fatalf("锟斤拷锟角革拷锟斤拷应锟缴癸拷: %s", result.Message)
	}

	// 锟斤拷证锟侥硷拷锟斤拷锟斤拷锟窖憋拷锟斤拷锟斤拷
	content, _ := os.ReadFile(filepath.Join(root, "target.mkv"))
	if string(content) != "new-content" {
		t.Errorf("锟侥硷拷锟斤拷锟斤拷应锟斤拷锟斤拷锟斤拷锟斤拷: got=%q", string(content))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

// TestExecuteRename_115锟斤拷锟斤拷_锟斤拷支锟斤拷 锟斤拷锟斤拷115锟斤拷锟教革拷锟斤拷锟斤拷锟截诧拷支锟斤拷
func TestExecuteRename_115锟斤拷锟斤拷_锟斤拷支锟斤拷(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cloud115ID := 1
	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := newMediaSourceRows().
		AddRow(1, "cloud", domain.SourceTypeCloud115, "0", "0", &cloud115ID, 10, true, "", "all", "skip", "move", false, false, 0, "", now, now)
	expectMediaSourceByID(mock, 1, sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID: 1,
		FileID:   "test.mkv",
		NewName:  "new.mkv",
	}

	_, err := renameSvc.ExecuteRename(req)
	if err == nil {
		t.Fatal("115锟斤拷锟教革拷锟斤拷应锟斤拷锟截诧拷支锟斤拷")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

// TestExecuteRename_源锟侥硷拷锟斤拷锟斤拷锟斤拷 锟斤拷锟斤拷源锟侥硷拷锟斤拷锟斤拷锟斤拷时锟斤拷锟截达拷锟斤拷
func TestExecuteRename_源锟侥硷拷锟斤拷锟斤拷锟斤拷(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	renameSvc := NewRenameService(mediaSvc, nil, nil, nil)

	now := time.Now()
	sourceRows := newMediaSourceRows().
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 0, "", now, now)
	expectMediaSourceByID(mock, 1, sourceRows)

	req := &domain.RenameExecuteRequest{
		SourceID: 1,
		FileID:   "nonexistent.mkv",
		NewName:  "new.mkv",
	}

	_, err := renameSvc.ExecuteRename(req)
	if err == nil {
		t.Fatal("源锟侥硷拷锟斤拷锟斤拷锟斤拷时应锟斤拷锟截达拷锟斤拷")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("未锟斤拷锟斤拷锟斤拷锟斤拷锟? %v", err)
	}
}

func TestRenameServiceNormalizeGeneratedNameRemovesDuplicatedTailAfterExtension(t *testing.T) {
	svc := &RenameService{}

	got := svc.normalizeGeneratedName(
		"锟斤拷锟斤拷A锟轿ｏ拷锟斤拷锟桔碉拷锟铰匡拷锟斤拷 (2020).mp4锟斤拷锟斤拷A锟轿ｏ拷锟斤拷锟桔碉拷锟铰匡拷锟斤拷",
		"Doraemon.2020.1080p.mkv",
		".mp4",
	)

	if got != "锟斤拷锟斤拷A锟轿ｏ拷锟斤拷锟桔碉拷锟铰匡拷锟斤拷 (2020).mp4" {
		t.Fatalf("unexpected normalized name: %q", got)
	}
}

func TestRenameServiceNormalizeGeneratedNameRemovesDuplicatedTailAfterTemplateProvidedExtension(t *testing.T) {
	svc := &RenameService{}

	got := svc.normalizeGeneratedName(
		"Mobile.Movie.2024..mp4Mobile.Movie.2024.",
		"Mobile.Movie.2024.1080p.mkv",
		".mkv",
	)

	if got != "Mobile.Movie.2024..mp4" {
		t.Fatalf("unexpected normalized name with template extension: %q", got)
	}
}




