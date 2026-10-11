package dao

import (
	"strings"
	"testing"
)

func TestShareBaselineAdoptionRetainsAllLiveContributorKeys(test *testing.T) {
	for _, fragment := range []string{"LEFT JOIN LATERAL", "f.available AND NOT r.share_cancelled AND f.status='identified'", "s.work_key=m.work_key", "e.file_id=f.id AND e.season_number=s.season_number AND e.episode_number=s.episode_number"} {
		if !strings.Contains(shareBaselineSourcesSQL, fragment) {
			test.Fatalf("adoption must checkpoint every live source's represented media items, not only selected candidates: missing %s", fragment)
		}
	}
	if !strings.Contains(shareBaselineCountsSQL, "m.media_type NOT IN ('movie','tv')") || !strings.Contains(shareBaselineCountsSQL, "s.work_key=m.work_key AND s.season_number=e.season_number AND s.episode_number=e.episode_number") {
		test.Fatal("active source episodes without a represented selection must refuse adoption")
	}
}
