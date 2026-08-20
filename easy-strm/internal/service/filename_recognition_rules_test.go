package service

import (
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

type memoryFilenameRuleStore struct {
	value string
}

func (s *memoryFilenameRuleStore) GetByKey(string) (*domain.SystemConfig, error) {
	return &domain.SystemConfig{ConfigKey: filenameRecognitionRulesConfigKey, ConfigVal: s.value}, nil
}

func (s *memoryFilenameRuleStore) Upsert(_ string, value string) error {
	s.value = value
	return nil
}

func TestDefaultFilenameRecognitionRulesCoverCommonTVNames(t *testing.T) {
	tests := []struct {
		filename string
		title    string
		season   int
		episode  int
		ruleID   string
	}{
		{"Show.Name.S02E07.1080p.mkv", "Show Name", 2, 7, "tv_sxe"},
		{"Show.Name.S00E01.mkv", "Show Name", 0, 1, "tv_sxe"},
		{"Show Name - 2x07 - Episode title.mp4", "Show Name", 2, 7, "tv_x"},
		{"Show Name Season 2 Episode 7.mkv", "Show Name", 2, 7, "tv_words"},
		{"庆余年 第2季 第07集.mp4", "庆余年", 2, 7, "tv_chinese"},
		{"葬送的芙莉莲 EP07.mp4", "葬送的芙莉莲", 1, 7, "tv_episode"},
	}

	service := NewTmdbService("fake-key", nil)
	for _, test := range tests {
		t.Run(test.ruleID, func(t *testing.T) {
			parsed := service.ParseFilename(test.filename)
			if parsed.Title != test.title || parsed.MediaType != "tv" || parsed.Season != test.season || parsed.Episode != test.episode {
				t.Fatalf("unexpected parse result: %+v", parsed)
			}
			if parsed.MatchedRuleID != test.ruleID {
				t.Fatalf("expected rule %q, got %q", test.ruleID, parsed.MatchedRuleID)
			}
			if strings.Contains(test.filename, "1080p") && parsed.Quality != "1080p" {
				t.Fatalf("expected quality metadata to be retained, got %+v", parsed)
			}
		})
	}
}

func TestSaveFilenameRecognitionRulesUpdatesParserImmediately(t *testing.T) {
	store := &memoryFilenameRuleStore{}
	service := NewTmdbService("fake-key", nil)
	service.SetFilenameRecognitionRuleStore(store)
	rules := []FilenameRecognitionRule{{
		ID: "custom_tv", Name: "自定义 CxxPxx", MediaType: "tv", Enabled: true, Priority: 10,
		Pattern: `(?i)^(?P<title>.+?)\s+C(?P<season>\d{1,2})P(?P<episode>\d{1,3})$`,
		Example: "Custom Show C03P09.mkv",
	}}

	if _, err := service.SaveFilenameRecognitionRules(rules); err != nil {
		t.Fatalf("save rules failed: %v", err)
	}
	parsed := service.ParseFilename("Custom.Show.C03P09.mkv")
	if parsed.Title != "Custom Show" || parsed.Season != 3 || parsed.Episode != 9 || parsed.MatchedRuleID != "custom_tv" {
		t.Fatalf("unexpected custom parse result: %+v", parsed)
	}
	if !strings.Contains(store.value, "custom_tv") {
		t.Fatalf("expected persisted rule, got %q", store.value)
	}
}

func TestSaveFilenameRecognitionRulesRejectsInvalidPattern(t *testing.T) {
	service := NewTmdbService("fake-key", nil)
	service.SetFilenameRecognitionRuleStore(&memoryFilenameRuleStore{})
	_, err := service.SaveFilenameRecognitionRules([]FilenameRecognitionRule{{
		ID: "broken_rule", Name: "错误规则", MediaType: "tv", Enabled: true,
		Pattern: `(?P<title>.+`, Example: "Show S01E01.mkv",
	}})
	if err == nil || !strings.Contains(err.Error(), "正则无效") {
		t.Fatalf("expected invalid regex error, got %v", err)
	}
}

func TestSaveFilenameRecognitionRulesRejectsMissingEpisodeGroup(t *testing.T) {
	service := NewTmdbService("fake-key", nil)
	service.SetFilenameRecognitionRuleStore(&memoryFilenameRuleStore{})
	_, err := service.SaveFilenameRecognitionRules([]FilenameRecognitionRule{{
		ID: "missing_episode", Name: "缺少集数", MediaType: "tv", Enabled: true,
		Pattern: `^(?P<title>.+)$`, Example: "Show.mkv",
	}})
	if err == nil || !strings.Contains(err.Error(), "episode") {
		t.Fatalf("expected missing episode error, got %v", err)
	}
}
