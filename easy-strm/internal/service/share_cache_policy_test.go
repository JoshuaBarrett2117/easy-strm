package service

import (
	"context"
	"easy-strm/internal/domain"
	"testing"
)

func TestShareRetrySelectsFailuresOnly(t *testing.T) {
	for _, status := range []string{"pending", "failed", "identified", "masked", "ignored"} {
		if got := shouldIdentifyShareMedia(domain.ShareMedia{Status: status}, "auto", true, false); got != (status == "failed") {
			t.Errorf("status=%s selected=%v", status, got)
		}
	}
}

func TestShareRetryKeepsCaches(t *testing.T) {
	// retry 仅筛选失败项；缓存刷新必须显式通过独立上下文表达。
	if bypassShareRecognitionCache(withShareWorkRound(context.Background(), false)) {
		t.Fatal("retry bypassed caches")
	}
	if !bypassShareRecognitionCache(withShareWorkRound(context.Background(), true)) {
		t.Fatal("force refresh used caches")
	}
}
