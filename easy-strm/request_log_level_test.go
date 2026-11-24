package main

import (
	"testing"
	"time"

	pkglogger "easy-strm/internal/pkg/logger"
)

func TestRequestLogLevel(t *testing.T) {
	cases := []struct {
		status  int
		elapsed time.Duration
		want    pkglogger.Level
	}{
		{200, 10 * time.Millisecond, pkglogger.DEBUG},
		{302, 10 * time.Millisecond, pkglogger.DEBUG},
		{200, 3 * time.Second, pkglogger.INFO},
		{401, 10 * time.Millisecond, pkglogger.DEBUG},
		{404, 10 * time.Millisecond, pkglogger.WARN},
		{500, 10 * time.Millisecond, pkglogger.ERROR},
		{503, 3 * time.Second, pkglogger.ERROR},
	}
	for _, c := range cases {
		if got := requestLogLevel(c.status, c.elapsed); got != c.want {
			t.Errorf("status=%d elapsed=%s got=%v want=%v", c.status, c.elapsed, got, c.want)
		}
	}
}
