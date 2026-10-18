package service

import (
	"easy-strm/internal/domain"
	"testing"
)

type directorySettings struct {
	value  string
	writes int
}

func (s *directorySettings) GetByKey(string) (*domain.SystemConfig, error) {
	return &domain.SystemConfig{ConfigVal: s.value}, nil
}
func (s *directorySettings) Upsert(string, string) error { s.writes++; return nil }

func TestShareTransferDirectory(t *testing.T) {
	store := &directorySettings{value: "/test_dir/"}
	for _, tc := range []struct{ explicit, account, want string }{{"", "", "/test_dir"}, {"/other", "/account", "/other"}, {"", "/other", "/other"}} {
		got, err := ResolveShareTransferDirectory(store, tc.explicit, tc.account)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %q %v", tc, got, err)
		}
	}
	store.value = "/latest"
	if got, _ := ResolveShareTransferDirectory(store, "", ""); got != "/latest" {
		t.Fatal(got)
	}
	for _, bad := range []string{"abc", "../x", "/a/../x", "/a\\b", "/a\n", "/a\x00"} {
		if _, err := NormalizeShareTransferDirectory(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	for _, tc := range []struct{ in, want string }{{"", "/EmbyCache"}, {" /test_dir/// ", "/test_dir"}} {
		if got, err := NormalizeShareTransferDirectory(tc.in); err != nil || got != tc.want {
			t.Fatalf("%q %v", got, err)
		}
	}
}
