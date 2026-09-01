package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

var driverCache = struct {
	sync.RWMutex
	drivers map[int]cached115Driver
}{
	drivers: make(map[int]cached115Driver),
}

const cloud115APITimeout = 2 * time.Minute

type cached115Driver struct {
	driver     *driver.Pan115Client
	cookieHash [sha256.Size]byte
}

// getOrCreateDriver 获取或创建指定账号的driver实例
// 使用缓存机制避免每次调用都创建新实例
func getOrCreateDriver(cloud115ID int, cookie string) (*driver.Pan115Client, error) {
	cookieHash := sha256.Sum256([]byte(cookie))
	// 先尝试读锁获取缓存
	driverCache.RLock()
	if cached, ok := driverCache.drivers[cloud115ID]; ok && cached.cookieHash == cookieHash {
		driverCache.RUnlock()
		Debug("Using cached driver for cloud115_id: %d", cloud115ID)
		return cached.driver, nil
	}
	driverCache.RUnlock()

	// 缓存不存在，加写锁创建新实例
	driverCache.Lock()
	defer driverCache.Unlock()

	// 双重检查，防止并发创建
	if cached, ok := driverCache.drivers[cloud115ID]; ok && cached.cookieHash == cookieHash {
		Debug("Using cached driver for cloud115_id: %d (double check)", cloud115ID)
		return cached.driver, nil
	}

	// 先注入项目统一的代理感知客户端，再设置115浏览器UA。
	// 115driver 的 SetHttpClient 会重建 Resty 客户端；若先设置UA，离线加密接口会因UA丢失返回 decode fail。
	cred := &driver.Credential{}
	if err := cred.FromCookie(cookie); err != nil {
		return nil, fmt.Errorf("parse cookie failed: %v", err)
	}
	// 115 Driver 默认使用无超时的 resty 客户端；网络异常时会让文件传输任务永久停留在 running。
	// 注入项目统一的代理感知客户端，确保移动、复制等 API 请求最终能够返回错误。
	d := driver.New(
		driver.WithClient(NewProxyAwareHTTPClient(cloud115APITimeout)),
		driver.UA(driver.UA115Browser),
	).
		ImportCredential(cred)
	driverCache.drivers[cloud115ID] = cached115Driver{driver: d, cookieHash: cookieHash}
	Debug("Created and cached new driver for cloud115_id: %d", cloud115ID)
	return d, nil
}

type Client struct {
	config     *Config
	httpClient *http.Client
	driver     *driver.Pan115Client
}

func NewClient(config *Config) *Client {
	return &Client{
		config:     config,
		httpClient: NewProxyAwareHTTPClient(30 * time.Second),
		driver:     driver.Default(),
	}
}

func (c *Client) GetQRCode() (*driver.QRCodeSession, error) {
	Debug("Getting login QR code")
	session, err := c.driver.QRCodeStart()
	if err != nil {
		Error("Failed to get QR code: %v", err)
		return nil, fmt.Errorf("get QR code failed: %v", err)
	}
	Info("Got QR code session with uid: %s", session.UID)
	return session, nil
}

func (c *Client) CheckLoginStatus(session *driver.QRCodeSession) (*driver.QRCodeStatus, error) {
	Debug("Checking login status for uid: %s", session.UID)
	status, err := c.driver.QRCodeStatus(session)
	if err != nil {
		Error("Failed to check login status: %v", err)
		return nil, fmt.Errorf("check login status failed: %v", err)
	}
	Info("Login status check completed with status: %d", status.Status)
	return status, nil
}

func (c *Client) QRCodeLogin(session *driver.QRCodeSession) (*driver.Credential, error) {
	Debug("Performing QR code login for uid: %s", session.UID)
	cred, err := c.driver.QRCodeLogin(session)
	if err != nil {
		Error("Failed to login with QR code: %v", err)
		return nil, fmt.Errorf("QR code login failed: %v", err)
	}
	Info("QR code login successful")
	return cred, nil
}

// QRCodeLoginWithApp 使用指定渠道进行二维码登录
// app: 登录渠道，可选值: web, android, ios, tv, alipaymini, wechatmini, qandroid
func (c *Client) QRCodeLoginWithApp(session *driver.QRCodeSession, app driver.LoginApp) (*driver.Credential, error) {
	Debug("Performing QR code login for uid: %s with app: %s", session.UID, app)
	cred, err := c.driver.QRCodeLoginWithApp(session, app)
	if err != nil {
		Error("Failed to login with QR code (app: %s): %v", app, err)
		return nil, fmt.Errorf("QR code login failed: %v", err)
	}
	Info("QR code login successful with app: %s", app)
	return cred, nil
}

func (c *Client) ImportCredential(cookie string) error {
	Debug("Importing credential from cookie")
	cred := &driver.Credential{}
	if err := cred.FromCookie(cookie); err != nil {
		Error("Failed to parse cookie: %v", err)
		return fmt.Errorf("parse cookie failed: %v", err)
	}
	c.driver = c.driver.ImportCredential(cred)
	Info("Credential imported successfully")
	return nil
}

func (c *Client) GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error) {
	Debug("Getting 115 cloud file list with cid: %d, show_dir: %d, offset: %d, limit: %d, cloud115_id: %d", cid, showDir, offset, limit, cloud115ID)

	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return nil, err
	}

	files, err := d.ListPage(fmt.Sprintf("%d", cid), int64(offset), int64(limit))
	if err != nil {
		Error("Failed to get file list: %v", err)
		return nil, fmt.Errorf("get file list failed: %v", err)
	}

	Info("Got file list response with %d files", len(*files))

	fileListResp := &driver.FileListResp{
		Count: len(*files),
		Files: make([]driver.FileInfo, 0),
	}
	for _, f := range *files {
		fileInfo := driver.FileInfo{
			Name:     f.GetName(),
			Size:     driver.StringInt64(f.GetSize()),
			PickCode: f.PickCode,
			Sha1:     f.Sha1,
		}

		if f.IsDir() {
			fileInfo.CategoryID = driver.IntString(f.GetID())
			fileInfo.Type = "folder"
		} else {
			fileInfo.FileID = f.GetID()
			fileInfo.CategoryID = driver.IntString(f.ParentID)
		}

		fileListResp.Files = append(fileListResp.Files, fileInfo)
	}

	return fileListResp, nil
}

func (c *Client) GetFileDirectLink(cid int, pickCode string, cloud115ID int, cookie string, ua string) (*driver.DownloadInfo, error) {
	Info("[DirectLink] Getting 115 cloud file direct link, cloud115_id: %d, pickcode: %s", cloud115ID, pickCode)

	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		Error("[DirectLink] Failed to get driver for cloud115_id %d: %v", cloud115ID, err)
		return nil, err
	}

	// 使用传入的 client UA，确保 115 CDN 的签名 (k=) 与重定向后的客户端匹配
	downloadInfo, err := d.DownloadWithUAByAndroidAPI(pickCode, ua)
	if err != nil {
		Error("[DirectLink] Failed to get download info for pickcode %s from cloud115_id %d: %v", pickCode, cloud115ID, err)
		return nil, fmt.Errorf("get download info failed: %v", err)
	}

	Info("[DirectLink] Got download info for file: %s", downloadInfo.FileName)
	return downloadInfo, nil
}

func (c *Client) GetUser(cookie string) (*driver.UserInfo, error) {
	Debug("Getting user info")

	if err := c.ImportCredential(cookie); err != nil {
		return nil, err
	}

	userInfo, err := c.driver.GetUser()
	if err != nil {
		Error("Failed to get user info: %v", err)
		return nil, fmt.Errorf("get user info failed: %v", err)
	}

	Info("Got user info for user: %s", userInfo.UserName)
	return userInfo, nil
}

// GetAccountStorage 获取指定115账号的已用容量和总容量，单位为字节。
func (c *Client) GetAccountStorage(cloud115ID int, cookie string) (int64, int64, error) {
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return 0, 0, fmt.Errorf("初始化115账号失败: %w", err)
	}

	info, err := d.GetInfo()
	if err != nil {
		return 0, 0, fmt.Errorf("获取115账号容量失败: %w", err)
	}
	return info.SpaceInfo.AllUse.Size, info.SpaceInfo.AllTotal.Size, nil
}

// ReceiveShare 将分享文件转存到目标账号指定目录
// 调用 115 官方接口 POST https://webapi.115.com/share/receive
// 基于分享文件的 file_id（fid）而非 pickcode，避免跨账号秒传的 status=7 内容校验问题
func (c *Client) ReceiveShare(shareCode, receiveCode, fileIDs, targetCID string, targetCloud115ID int, targetCookie string) error {
	masked := shareCode
	if len(masked) > 4 {
		masked = masked[:4] + "***"
	}
	Debug("[ReceiveShare] start | shareCode=%s | files=%s | targetCID=%s", masked, fileIDs, targetCID)

	d, err := getOrCreateDriver(targetCloud115ID, targetCookie)
	if err != nil {
		return err
	}

	form := buildReceiveShareForm(shareCode, receiveCode, fileIDs, targetCID)

	resp, err := d.NewRequest().
		SetHeaderVerbatim("Content-Type", "application/x-www-form-urlencoded").
		SetHeader("Referer", "https://115cdn.com/").
		SetBody(form.Encode()).
		SetDoNotParseResponse(true).
		Post("https://webapi.115.com/share/receive")
	if err != nil {
		return fmt.Errorf("share receive request failed: %v", err)
	}
	data := resp.RawBody()
	bodyBytes, _ := io.ReadAll(data)
	data.Close()

	var result struct {
		State  bool   `json:"state"`
		Errno  int    `json:"errno"`
		Errmsg string `json:"errmsg"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return fmt.Errorf("parse share receive response failed: %v (raw: %s)", err, string(bodyBytes))
	}
	if !result.State {
		return fmt.Errorf("share receive failed: %s (errno=%d)", result.Errmsg, result.Errno)
	}
	Info("[ReceiveShare] success | shareCode=%s | files=%s", masked, fileIDs)
	return nil
}

// buildReceiveShareForm 构造115分享转存表单。
// 目标目录必须使用官方share/receive接口定义的cid字段；其它字段名会被接口忽略。
func buildReceiveShareForm(shareCode, receiveCode, fileIDs, targetCID string) url.Values {
	form := url.Values{}
	form.Set("share_code", shareCode)
	form.Set("receive_code", receiveCode)
	form.Set("file_id", fileIDs)
	form.Set("cid", targetCID)
	return form
}
