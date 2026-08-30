package domain

import "testing"

func TestParseCloud115CookieIdentity(t *testing.T) {
	tests := []struct {
		name       string
		cookie     string
		wantOK     bool
		wantUserID string
		wantSSOEnt string
		wantSource string
	}{
		{name: "支付宝小程序", cookie: "CID=cid; UID=102071024_R2_1700000000; SEID=seid", wantOK: true, wantUserID: "102071024", wantSSOEnt: "R2", wantSource: "支付宝小程序"},
		{name: "微信小程序且忽略字段大小写", cookie: "uid=88_r1_1700000001; CID=cid", wantOK: true, wantUserID: "88", wantSSOEnt: "R1", wantSource: "微信小程序"},
		{name: "A1 保留歧义", cookie: `UID="99_A1_1700000002"; CID=cid`, wantOK: true, wantUserID: "99", wantSSOEnt: "A1", wantSource: "网页版 / 115 浏览器"},
		{name: "未知设备码", cookie: "UID=77_Z9_1700000003", wantOK: true, wantUserID: "77", wantSSOEnt: "Z9", wantSource: "未知渠道"},
		{name: "旧式无设备码 UID", cookie: "UID=123", wantOK: false},
		{name: "非法用户 ID", cookie: "UID=user_R2_1700000000", wantOK: false},
		{name: "缺少 UID", cookie: "CID=cid; SEID=seid", wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseCloud115CookieIdentity(test.cookie)
			if ok != test.wantOK {
				t.Fatalf("ok = %v，期望 %v", ok, test.wantOK)
			}
			if !ok {
				return
			}
			if got.UserID != test.wantUserID || got.SSOEnt != test.wantSSOEnt || got.Source != test.wantSource {
				t.Fatalf("解析结果 = %+v", got)
			}
		})
	}
}

func TestCloud115CookieSourceByApp(t *testing.T) {
	if got := Cloud115CookieSourceByApp(" alipaymini "); got != "支付宝小程序" {
		t.Fatalf("支付宝渠道 = %q", got)
	}
	if got := Cloud115CookieSourceByApp(""); got != "网页版 / 115 浏览器" {
		t.Fatalf("默认渠道 = %q", got)
	}
}
