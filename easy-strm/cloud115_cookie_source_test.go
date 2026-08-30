package main

import "testing"

func TestCloud115CookieSourceLabel(t *testing.T) {
	tests := []struct {
		name string
		app  string
		want string
	}{
		{name: "微信小程序", app: "wechatmini", want: "微信小程序"},
		{name: "支付宝小程序", app: "alipaymini", want: "支付宝小程序"},
		{name: "忽略大小写和空格", app: " Android ", want: "115 生活安卓端"},
		{name: "未知渠道回退网页版", app: "", want: "网页版 / 115 浏览器"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cloud115CookieSourceLabel(test.app); got != test.want {
				t.Fatalf("cloud115CookieSourceLabel(%q) = %q，期望 %q", test.app, got, test.want)
			}
		})
	}
}
