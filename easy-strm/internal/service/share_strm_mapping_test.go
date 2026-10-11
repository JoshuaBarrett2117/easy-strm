package service

import (
	"path"
	"reflect"
	"testing"

	"easy-strm/internal/domain"
	"github.com/google/uuid"
)

func TestShareStrmMappingBuilderPreservesExportSemantics(test *testing.T) {
	for _, scenario := range []struct {
		name     string
		url      string
		password string
		fileName string
	}{
		{"explicit-password", "https://115.com/s/selected?password=ignored", " selected-code ", "Show/episode.mkv"},
		{"url-password", "https://115cdn.com/s/selected?password=%20url%2Bcode%20", "", `Show\unused\..\episode.mkv`},
		{"empty-password", "https://115.com/s/selected", "", "Show//./episode.mkv"},
		{"duplicate-password", "https://115.com/s/selected?password=first&password=second", "", "episode.mkv"},
		{"malformed-query", "https://115.com/s/selected?password=%zz", "", "episode.mkv"},
		{"invalid-link", "https://example.com/s/selected", "", "episode.mkv"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			source := domain.ShareStrmSource{URL: scenario.url, Password: scenario.password, FileName: scenario.fileName, RemoteFileID: "selected-file", MediaID: 7, Result: domain.TmdbIdentifyResult{Title: "Show", PosterPath: "/poster.jpg"}, Episodes: []domain.ShareEpisode{{SeasonNumber: 1, EpisodeNumber: 1}}}
			actual, err := domain.BuildShareStrmEntry(source)
			matches := shareCodeRe.FindStringSubmatch(source.URL)
			if len(matches) < 2 {
				if err == nil {
					test.Fatal("invalid original share code accepted")
				}
				return
			}
			filePath := shareCandidatePath(source.FileName)
			expected := domain.ShareStrmEntry{
				ID:        uuid.NewSHA1(uuid.NameSpaceURL, []byte(matches[1]+":path:"+filePath)).String(),
				ShareCode: matches[1], Password: source.Password, FileID: source.RemoteFileID,
				FileName: path.Base(filePath), FilePath: filePath, MediaID: source.MediaID,
				Title: source.Result.Title, PosterPath: source.Result.PosterPath, Episodes: source.Episodes,
			}
			if expected.Password == "" {
				expected.Password = extractSharePassword(source.URL)
			}
			if err != nil || !reflect.DeepEqual(actual, expected) {
				test.Fatalf("shared builder differs from original export semantics: %v", err)
			}
		})
	}
}
