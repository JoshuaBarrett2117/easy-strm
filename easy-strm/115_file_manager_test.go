package main

import (
	"net/http"
	"testing"
)

func TestNormalize115DownloadCookiesRemovesResponseAttributes(t *testing.T) {
	header := http.Header{}
	header.Set("Cookie", "UID=user; acw_tc=token; Path=/; Max-Age=3600; HttpOnly; Secure")
	normalize115DownloadCookies(header)

	request := &http.Request{Header: header}
	cookies := request.Cookies()
	if len(cookies) != 2 || cookies[0].Name != "UID" || cookies[1].Name != "acw_tc" {
		t.Fatalf("规范化后的 Cookie 不正确: %q", header.Get("Cookie"))
	}
}
