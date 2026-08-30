package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestFormatCloud115IncludesCookieSource(t *testing.T) {
	now := time.Now()
	formatted := formatCloud115(&Cloud115AccountBrief{
		ID:           1,
		Name:         "主账号",
		Cookie:       "UID=test",
		CookieSource: "R2 支付宝小程序",
		CreateTime:   now,
		UpdateTime:   now,
	}, false)

	if got := formatted["cookie_source"]; got != "R2 支付宝小程序" {
		t.Fatalf("cookie_source = %v，期望 R2 支付宝小程序", got)
	}
}

func TestFormatCloud115InfersHistoricalCookieSource(t *testing.T) {
	now := time.Now()
	formatted := formatCloud115(&Cloud115AccountBrief{
		ID:         2,
		Name:       "历史账号",
		Cookie:     "UID=102071024_R2_1700000000; CID=cid; SEID=seid",
		CreateTime: now,
		UpdateTime: now,
	}, false)

	if formatted["cookie_uid"] != "102071024" || formatted["cookie_ssoent"] != "R2" {
		t.Fatalf("UID 信息解析错误：%v", formatted)
	}
	if formatted["cookie_source"] != "支付宝小程序" || formatted["cookie_source_inferred"] != true {
		t.Fatalf("历史 Cookie 来源未自动推断：%v", formatted)
	}
}

func TestFormatCloud115PreservesManualSource(t *testing.T) {
	now := time.Now()
	formatted := formatCloud115(&Cloud115AccountBrief{
		ID:           3,
		Name:         "人工来源账号",
		Cookie:       "UID=102071024_R2_1700000000; CID=cid; SEID=seid",
		CookieSource: "R2 专用资源号",
		CreateTime:   now,
		UpdateTime:   now,
	}, false)

	if formatted["cookie_source"] != "R2 专用资源号" || formatted["cookie_source_inferred"] != false {
		t.Fatalf("人工来源不应被覆盖：%v", formatted)
	}
}

func TestNormalizeCookieSource(t *testing.T) {
	got, valid := normalizeCookieSource("  R2 支付宝小程序  ")
	if !valid || got != "R2 支付宝小程序" {
		t.Fatalf("normalizeCookieSource() = %q, %v", got, valid)
	}

	tooLong := make([]rune, 101)
	for index := range tooLong {
		tooLong[index] = '来'
	}
	if _, valid = normalizeCookieSource(string(tooLong)); valid {
		t.Fatal("101 个字符的 Cookie 来源应被拒绝")
	}
}

func TestCloud115CreateForwardsCookieSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := NewCloud115Controller(nil)
	capturedSource := ""
	controller.SetCreateCloud115(func(name, cookie, cookieSource, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, transferMethod string, alistUrl string, alistToken string) (*Cloud115AccountBrief, error) {
		capturedSource = cookieSource
		now := time.Now()
		return &Cloud115AccountBrief{ID: 1, Name: name, Cookie: cookie, CookieSource: cookieSource, CreateTime: now, UpdateTime: now}, nil
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/cloud115", strings.NewReader(`{"name":"主账号","cookie":"UID=test","cookie_source":"  R2 支付宝小程序  "}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	controller.Create(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d，响应：%s", recorder.Code, recorder.Body.String())
	}
	if capturedSource != "R2 支付宝小程序" {
		t.Fatalf("传给持久化层的 cookieSource = %q", capturedSource)
	}
}

func TestConfirmLoginForwardsSelectedChannelAndAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := NewCloud115Controller(nil)
	controller.SetQRCodeLogin(func(session interface{}, name string, cloudID int) (interface{}, error) {
		return map[string]interface{}{}, nil
	})

	capturedApp := ""
	capturedName := ""
	capturedID := 0
	controller.SetQRCodeLoginWithApp(func(session interface{}, app, name string, cloudID int) (interface{}, error) {
		capturedApp, capturedName, capturedID = app, name, cloudID
		return map[string]interface{}{"state": true}, nil
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/115/login/confirm", strings.NewReader(`{"uid":"uid","time":1,"sign":"sign","app":"alipaymini","name":"主账号","cloud_id":8}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	controller.ConfirmLogin(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d，响应：%s", recorder.Code, recorder.Body.String())
	}
	if capturedApp != "alipaymini" || capturedName != "主账号" || capturedID != 8 {
		t.Fatalf("扫码参数未完整转发: app=%q name=%q cloudID=%d", capturedApp, capturedName, capturedID)
	}
}
