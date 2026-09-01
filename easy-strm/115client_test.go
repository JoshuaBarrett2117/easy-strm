package main

import (
	"testing"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

func TestGetOrCreateDriverConfiguresTimeoutAndRefreshesChangedCookie(t *testing.T) {
	driverCache.Lock()
	driverCache.drivers = make(map[int]cached115Driver)
	driverCache.Unlock()
	t.Cleanup(func() {
		driverCache.Lock()
		driverCache.drivers = make(map[int]cached115Driver)
		driverCache.Unlock()
	})

	first, err := getOrCreateDriver(115, "UID=1;CID=2;SEID=3;KID=4")
	if err != nil {
		t.Fatalf("创建115 Driver失败: %v", err)
	}
	if first.Client.GetClient().Timeout != cloud115APITimeout {
		t.Fatalf("115 Driver超时=%s，期望=%s", first.Client.GetClient().Timeout, cloud115APITimeout)
	}
	if userAgent := first.Client.Header.Get("User-Agent"); userAgent != driver.UA115Browser {
		t.Fatalf("115 Driver User-Agent=%q，期望=%q；缺失浏览器UA会导致离线接口返回decode fail", userAgent, driver.UA115Browser)
	}

	reused, err := getOrCreateDriver(115, "UID=1;CID=2;SEID=3;KID=4")
	if err != nil {
		t.Fatalf("复用115 Driver失败: %v", err)
	}
	if reused != first {
		t.Fatal("相同Cookie应复用已有115 Driver")
	}

	refreshed, err := getOrCreateDriver(115, "UID=1;CID=2;SEID=changed;KID=4")
	if err != nil {
		t.Fatalf("刷新115 Driver失败: %v", err)
	}
	if refreshed == first {
		t.Fatal("Cookie变化后必须重建115 Driver")
	}
}
