//go:build live
// +build live

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"
)

// TestParseShareLink_WithCookie 先访问分享页获取 Cookie，再调用 API
func TestParseShareLink_WithCookie(t *testing.T) {
	shareCode := os.Getenv("EASY_STRM_LIVE_SHARE_CODE")
	password := os.Getenv("EASY_STRM_LIVE_SHARE_PASSWORD")
	if shareCode == "" {
		t.Skip("未设置 EASY_STRM_LIVE_SHARE_CODE，跳过真实分享链接测试")
	}
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"

	// 使用 Cookie Jar 自动管理 cookie
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 15 * time.Second, Jar: jar}

	// Step 1: 先访问分享页面获取 Cookie
	t.Log("Step 1: 访问分享页面获取 Cookie...")
	sharePageURL := "https://115cdn.com/s/" + shareCode + "?password=" + password
	pageResp, err := client.Get(sharePageURL)
	if err != nil {
		t.Fatalf("访问分享页失败: %v", err)
	}
	pageBody, _ := io.ReadAll(pageResp.Body)
	pageResp.Body.Close()
	t.Logf("分享页 HTTP %d (%d 字节)", pageResp.StatusCode, len(pageBody))

	// 查看获取到的 Cookie
	cookies := jar.Cookies(pageResp.Request.URL)
	for _, c := range cookies {
		v := c.Value
		if len(v) > 16 {
			v = v[:16]
		}
		t.Logf("  Cookie: %s=%s", c.Name, v)
	}

	// Step 2: 用带 Cookie 的 client 调用 API
	t.Log("\nStep 2: 调用 API (GET + Referer + Cookie)...")
	apiURL := "https://115cdn.com/webapi/share/snap" +
		"?share_code=" + shareCode +
		"&receive_code=" + password +
		"&cid=0&limit=20&asc=0&offset=0&format=json"

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", sharePageURL+"&")

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	t.Logf("HTTP %d | %s | %d 字节", resp.StatusCode, elapsed, len(raw))

	if resp.StatusCode == 200 {
		// 美化输出
		var pretty map[string]interface{}
		json.Unmarshal(raw, &pretty)
		prettyJSON, _ := json.MarshalIndent(pretty, "", "  ")
		if len(prettyJSON) > 3000 {
			t.Logf("%s\n... (截断)", prettyJSON[:3000])
		} else {
			t.Logf("%s", prettyJSON)
		}

		// 提取关键信息
		state, _ := pretty["state"]
		errno, _ := pretty["errno"]
		t.Logf("\nstate=%v errno=%v", state, errno)

		if state == true || state == float64(1) {
			data := pretty["data"].(map[string]interface{})
			count, _ := data["count"]
			list, _ := data["list"].([]interface{})
			t.Logf("文件数: %v (实际返回: %d)", count, len(list))

			for i, item := range list {
				if i >= 3 {
					break
				}
				f := item.(map[string]interface{})
				t.Logf("  [%d] fid=%v cid=%v n=%v s=%v ico=%v sha=%v",
					i+1, f["fid"], f["cid"], f["n"], f["s"], f["ico"], f["sha"])
			}
		}
	} else {
		s := string(raw)
		if len(s) > 500 {
			s = s[:500]
		}
		t.Logf("响应: %s", s)
	}
}
