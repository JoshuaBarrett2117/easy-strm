package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

var driverCache = struct {
	sync.RWMutex
	drivers map[int]*driver.Pan115Client
}{
	drivers: make(map[int]*driver.Pan115Client),
}

// getOrCreateDriver 获取或创建指定账号的driver实例
// 使用缓存机制避免每次调用都创建新实例
func getOrCreateDriver(cloud115ID int, cookie string) (*driver.Pan115Client, error) {
	// 先尝试读锁获取缓存
	driverCache.RLock()
	if d, ok := driverCache.drivers[cloud115ID]; ok {
		driverCache.RUnlock()
		Debug("Using cached driver for cloud115_id: %d", cloud115ID)
		return d, nil
	}
	driverCache.RUnlock()

	// 缓存不存在，加写锁创建新实例
	driverCache.Lock()
	defer driverCache.Unlock()

	// 双重检查，防止并发创建
	if d, ok := driverCache.drivers[cloud115ID]; ok {
		Debug("Using cached driver for cloud115_id: %d (double check)", cloud115ID)
		return d, nil
	}

	// 创建新的driver实例，默认设置 UA 为 115 浏览器，以匹配上传接口
	cred := &driver.Credential{}
	if err := cred.FromCookie(cookie); err != nil {
		return nil, fmt.Errorf("parse cookie failed: %v", err)
	}
	d := driver.New(driver.UA(driver.UA115Browser)).ImportCredential(cred)
	driverCache.drivers[cloud115ID] = d
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
			PickCode: f.GetID(),
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
