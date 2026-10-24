package domain

import "testing"

func TestParseStrm115Item(t *testing.T) {
	for _, test := range []struct {
		work, path      string
		season, episode int
		valid           bool
	}{
		{"tmdb:tv:36005", `日番\恶作剧之吻 (2008) [tmdbid=36005]\Season 1\恶作剧之吻.2008.S01E01.mkv`, 1, 1, true},
		{"tmdb:tv:102533", `剧 {tmdb-102533}/Season 0/剧.S00E02.mkv`, 0, 2, true},
		{"tmdb:movie:102533", `电影 {tmdb-102533}/电影.mkv`, 0, 0, true},
		{"tmdb:movie:36005", `电影 [tmdbid=36005]/电影.mkv`, 0, 0, true},
		{"tmdb:movie:36005", `Cars2 [tmdbid=36005]/House2.mkv`, 0, 0, true},
		{"tmdb:tv:36005", `House2 [tmdbid=36005]/House2.S01E01.mkv`, 1, 1, true},
		{"tmdb:tv:36005", `剧 [tmdbid=36005]/剧.S01E01~02.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=36005]/剧.S01E01+02.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=+36005]/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 {tmdb--36005}/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:movie:36005", `电影/电影.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=36005]/Season 2/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=36005]/Season 1/剧.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=360050]/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=36005] {tmdb-2}/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=36005]/剧.S01E01E02.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=36005]/剧.S01E01-02.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=36005]/剧.S01E01.S02E02.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=36005]/剧.S99999999999999999999E01.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=99999999999999999999]/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:36005", `剧 [tmdbid=-36005]/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:movie:36005", `电影 [tmdbid=36005]/Season 1/电影.mkv`, 0, 0, false},
		{"tmdb:movie:36005", `电影 [tmdbid=36005]/电影.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:0", `剧 [tmdbid=0]/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:99999999999999999999", `剧 [tmdbid=1]/剧.S01E01.mkv`, 0, 0, false},
		{"prefix:tmdb:tv:36005", `剧 [tmdbid=36005]/剧.S01E01.mkv`, 0, 0, false},
		{"tmdb:tv:36005:1", `剧 [tmdbid=36005]/剧.S01E01.mkv`, 0, 0, false},
	} {
		season, episode, valid := ParseStrm115Item(test.work, test.path)
		if valid != test.valid || season != test.season || episode != test.episode {
			t.Errorf("%q %q: %d/%d %v", test.work, test.path, season, episode, valid)
		}
	}
}

func TestBuildStrm115EntryAndReferences(t *testing.T) {
	candidate := ShareCandidate{SourceKind: "strm115", ExternalRef: "0:12", Cloud115ID: 8, CloudPath: "/日番/恶作剧之吻 (2008)/S01E01.mkv"}
	entry, err := BuildStrm115Entry("https://server.test/prefix/", candidate)
	want := "https://server.test/prefix/direct-link?path=%2F%E6%97%A5%E7%95%AA%2F%E6%81%B6%E4%BD%9C%E5%89%A7%E4%B9%8B%E5%90%BB+%282008%29%2FS01E01.mkv&cloud115_id=8\n"
	if err != nil || entry.Content != want {
		t.Fatalf("%q %v", entry.Content, err)
	}
	for _, ref := range []string{"-1:12", "1:0", "01:2", "1:02", "1:2:3", "1:+2", " 1:2", "1:999999999999999999999"} {
		if _, _, err := ParseStrm115Ref(ref); err == nil {
			t.Errorf("accepted %q", ref)
		}
	}
	for _, server := range []string{"", "/relative", "ftp://server", "https://user:pass@server", "https://server?q=1", "https://server/#frag", "https://server?", "https://server/#"} {
		if _, err := BuildStrm115Entry(server, candidate); err == nil {
			t.Errorf("accepted %q", server)
		}
	}
	for _, cloud := range []string{"relative", "/", "/../video", "/a/../video", "/a\\video", "/a\nvideo", "//server/video"} {
		invalid := candidate
		invalid.CloudPath = cloud
		if _, err := BuildStrm115Entry("https://server", invalid); err == nil {
			t.Errorf("accepted %q", cloud)
		}
	}
	for _, kind := range []string{"", "share", "unknown"} {
		invalid := candidate
		invalid.SourceKind = kind
		if _, err := BuildStrm115Entry("https://server", invalid); err == nil {
			t.Errorf("accepted %q", kind)
		}
	}
	candidate.Cloud115ID = 0
	if _, err = BuildStrm115Entry("https://server", candidate); err == nil {
		t.Fatal("accepted missing cloud ID")
	}
	change := ShareSelectionChange{CandidateID: 12}
	if err = ValidateShareSelectionChange(&change); err != nil || change.SourceKind != "share" {
		t.Fatal(change, err)
	}
	if NormalizeStrm115Path(`日番\剧集\file.mkv`) != "日番/剧集/file.mkv" {
		t.Fatal("Windows normalization")
	}
}
