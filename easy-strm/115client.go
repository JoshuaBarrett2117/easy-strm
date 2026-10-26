package main

import (
	"context"
	"crypto/sha256"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/go-resty/resty/v2"
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
		return nil, fmt.Errorf("parse cookie failed: error_type=%T", err)
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
	config          *Config
	httpClient      *http.Client
	driver          *driver.Pan115Client
	playbackContext context.Context
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
	Info("Got QR code session; identifier omitted")
	return session, nil
}

func (c *Client) CheckLoginStatus(session *driver.QRCodeSession) (*driver.QRCodeStatus, error) {
	Debug("Checking QR code login status")
	status, err := c.driver.QRCodeStatus(session)
	if err != nil {
		Error("Failed to check login status: %v", err)
		return nil, fmt.Errorf("check login status failed: %v", err)
	}
	Info("Login status check completed with status: %d", status.Status)
	return status, nil
}

func (c *Client) QRCodeLogin(session *driver.QRCodeSession) (*driver.Credential, error) {
	Debug("Performing QR code login")
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
	Debug("Performing QR code login with app: %s", app)
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
		Error("Failed to parse cookie: error_type=%T", err)
		return fmt.Errorf("parse cookie failed: error_type=%T", err)
	}
	c.driver = c.driver.ImportCredential(cred)
	Info("Credential imported successfully")
	return nil
}

func (c *Client) GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error) {
	Debug("Getting 115 cloud file list with cid: %d, show_dir: %d, offset: %d, limit: %d, cloud115_id: %d", cid, showDir, offset, limit, cloud115ID)

	d, metadata, err := c.playbackDriver(cloud115ID, cookie)
	if err != nil {
		return nil, err
	}

	files, err := d.ListPage(fmt.Sprintf("%d", cid), int64(offset), int64(limit))
	err = metadata.failure(err)
	if err != nil {
		if c.playbackContext != nil {
			return nil, err
		}
		Error("Failed to get file list: %v", err)
		return nil, fmt.Errorf("get file list failed: %v", err)
	}

	// 文件浏览请求频率高，结果详情仅在调试级别记录，避免系统日志被正常轮询淹没。
	Debug("Got file list response: cid=%d offset=%d limit=%d files=%d", cid, offset, limit, len(*files))

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
	Debug("[DirectLink] Getting 115 cloud file direct link, cloud115_id: %d", cloud115ID)

	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		Error("[DirectLink] Failed to get driver for cloud115_id %d: %v", cloud115ID, err)
		return nil, err
	}

	// 使用传入的 client UA，确保 115 CDN 的签名 (k=) 与重定向后的客户端匹配
	downloadInfo, err := d.DownloadWithUAByAndroidAPI(pickCode, ua)
	if err != nil {
		Error("[DirectLink] Failed to get download info from cloud115_id %d: %v", cloud115ID, err)
		return nil, fmt.Errorf("get download info failed: %w", err)
	}

	Debug("[DirectLink] Got download info for file: %s", downloadInfo.FileName)
	return downloadInfo, nil
}

type shareDirectLinkTransport struct {
	base   http.RoundTripper
	ctx    context.Context
	status int
	errno  int
}

func (transport *shareDirectLinkTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.status, transport.errno = 0, 0
	var ctx context.Context
	var cancel context.CancelFunc
	if deadline, exists := request.Context().Deadline(); exists {
		ctx, cancel = context.WithDeadline(transport.ctx, deadline)
	} else {
		ctx, cancel = context.WithCancel(transport.ctx)
	}
	stop := context.AfterFunc(request.Context(), cancel)
	cleanup := func() { stop(); cancel() }
	response, err := transport.base.RoundTrip(request.WithContext(ctx))
	if response != nil {
		transport.status = response.StatusCode
	}
	if err != nil || response == nil || response.Body == nil {
		cleanup()
	} else {
		response.Body = &shareDirectLinkBody{ReadCloser: response.Body, cleanup: cleanup}
	}
	return response, err
}

type shareDirectLinkBody struct {
	io.ReadCloser
	cleanup func()
}

func (body *shareDirectLinkBody) Close() error {
	defer body.cleanup()
	return body.ReadCloser.Close()
}

func (transport *shareDirectLinkTransport) captureResponse(_ *resty.Client, response *resty.Response) error {
	var result driver.BasicResp
	if json.Unmarshal(response.Body(), &result) == nil {
		transport.errno = int(result.Errno)
		if transport.errno == 0 {
			transport.errno = result.ErrNo
		}
	}
	return nil
}

func (transport *shareDirectLinkTransport) failure(err error) error {
	if transport == nil {
		return err
	}
	if err == nil && (transport.status < 200 || transport.status >= 300) {
		err = errors.New("115接口返回非成功HTTP状态")
	}
	if err != nil {
		return &domain.DirectLinkError{Errno: transport.errno, HTTPStatus: transport.status, Cause: err}
	}
	return nil
}

// GetFileDirectLinkContext 为分享播放隔离 SDK 的可变请求状态，并将取消传递到真实 HTTP 请求。
func (c *Client) GetFileDirectLinkContext(ctx context.Context, pickCode string, cookie string, ua string) (*driver.DownloadInfo, error) {
	credential := &driver.Credential{}
	if err := credential.FromCookie(cookie); err != nil {
		return nil, &domain.DirectLinkError{Cause: err}
	}
	client := *c.httpClient
	client.Timeout = 20 * time.Second
	transport := &shareDirectLinkTransport{base: client.Transport, ctx: ctx}
	if transport.base == nil {
		transport.base = http.DefaultTransport
	}
	client.Transport = transport
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	upstream := driver.New(driver.WithClient(&client), driver.UA(driver.UA115Browser)).ImportCredential(credential)
	upstream.Client.SetRetryCount(0)
	upstream.Client.OnAfterResponse(transport.captureResponse)
	info, err := upstream.DownloadWithUAByAndroidAPI(pickCode, ua)
	err = transport.failure(err)
	if err != nil {
		return nil, fmt.Errorf("获取115直链失败：%w", err)
	}
	return info, nil
}

// WithPlaybackContext 为单次分享解析创建客户端，不修改共享 SDK 或其他端点的请求上下文。
func (c *Client) WithPlaybackContext(ctx context.Context) *Client {
	return &Client{config: c.config, httpClient: c.httpClient, playbackContext: ctx}
}

func (c *Client) playbackDriver(accountID int, cookie string) (*driver.Pan115Client, *shareDirectLinkTransport, error) {
	if c.playbackContext == nil {
		upstream, err := getOrCreateDriver(accountID, cookie)
		return upstream, nil, err
	}
	if err := c.playbackContext.Err(); err != nil {
		return nil, nil, err
	}
	client := *c.httpClient
	client.Timeout = 20 * time.Second
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	metadata := &shareDirectLinkTransport{base: base, ctx: c.playbackContext}
	client.Transport = metadata
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	upstream := driver.New(driver.WithClient(&client), driver.UA(driver.UA115Browser))
	upstream.Client.SetRetryCount(0)
	upstream.Client.OnAfterResponse(metadata.captureResponse)
	if cookie != "" {
		credential := &driver.Credential{}
		if err := credential.FromCookie(cookie); err != nil {
			return nil, nil, &domain.DirectLinkError{Cause: err}
		}
		upstream.ImportCredential(credential)
	}
	return upstream, metadata, nil
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

	Info("Got user info")
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
	Debug("[ReceiveShare] start | target_id=%d", targetCloud115ID)

	d, _, err := c.playbackDriver(targetCloud115ID, targetCookie)
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
		return fmt.Errorf("share receive request failed: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("share receive response is missing")
	}
	data := resp.RawBody()
	if data == nil {
		return fmt.Errorf("share receive response body is missing")
	}
	if status := resp.StatusCode(); status < http.StatusOK || status >= http.StatusMultipleChoices {
		// 非成功响应可能含代理错误页或敏感信息，只返回状态码并释放响应体。
		if closeErr := data.Close(); closeErr != nil {
			return fmt.Errorf("share receive response HTTP %d: close error_type=%T", status, closeErr)
		}
		return fmt.Errorf("share receive response HTTP %d", status)
	}
	const maxReceiveShareResponseBytes = 1 << 20
	bodyBytes, readErr := io.ReadAll(io.LimitReader(data, maxReceiveShareResponseBytes+1))
	closeErr := data.Close()
	if readErr != nil {
		var timeout net.Error
		if errors.As(readErr, &timeout) && timeout.Timeout() {
			return fmt.Errorf("read share receive response timeout: error_type=%T", readErr)
		}
		return fmt.Errorf("read share receive response failed: error_type=%T", readErr)
	}
	if len(bodyBytes) > maxReceiveShareResponseBytes {
		return fmt.Errorf("share receive response exceeds %d bytes", maxReceiveShareResponseBytes)
	}
	if closeErr != nil {
		return fmt.Errorf("close share receive response failed: error_type=%T", closeErr)
	}

	var result struct {
		State   bool   `json:"state"`
		Errno   int    `json:"errno"`
		Errmsg  string `json:"errmsg"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return fmt.Errorf("parse share receive response failed: error_type=%T", err)
	}
	if !result.State {
		message := result.Errmsg
		if message == "" {
			message = result.Error
		}
		if message == "" {
			message = result.Message
		}
		return &domain.ShareReceiveError{Errno: result.Errno, Message: message}
	}
	Info("[ReceiveShare] success | target_id=%d", targetCloud115ID)
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
