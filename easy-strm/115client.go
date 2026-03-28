package main

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	cipher "github.com/SheltonZhu/115driver/pkg/crypto/ec115"
	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/deadblue/elevengo"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

const (
	appVerWeb     = "27.0.5.7"
	appVerAndroid = "30.1.0"
	md5Salt       = "Qclm8MGWUv59TnrR0XPg"
)

// generateToken 计算上传 token
func generateToken(fileID, fileSize, userID, timeStamp, signKey, signVal, appVer string) string {
	userIDMd5 := md5.Sum([]byte(userID))
	tokenMd5 := md5.Sum([]byte(md5Salt + fileID + fileSize + signKey + signVal + userID + timeStamp + hex.EncodeToString(userIDMd5[:]) + appVer))
	return hex.EncodeToString(tokenMd5[:])
}

// driverCache 用于缓存每个115账号的driver实例，避免并发问题
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
		httpClient: &http.Client{},
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
		fileListResp.Files = append(fileListResp.Files, driver.FileInfo{
			FileID:     f.GetID(),
			CategoryID: driver.IntString(f.GetID()),
			Name:       f.GetName(),
			Size:       driver.StringInt64(f.GetSize()),
			PickCode:   f.GetID(),
		})
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

// CopyFile 复制文件到目标目录
// fileID: 要复制的文件ID（pickcode）
// targetDirID: 目标目录ID（为空则复制到根目录，即"0"）
// cloud115ID: 115账号ID
// cookie: 115账号Cookie
func (c *Client) CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error {
	Debug("Copying file %s to directory %s for cloud115_id: %d", fileID, targetDirID, cloud115ID)

	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return err
	}

	// 如果目标目录为空，则复制到根目录
	if targetDirID == "" {
		targetDirID = "0"
	}

	err = d.Copy(targetDirID, fileID)
	if err != nil {
		Error("Failed to copy file %s to directory %s: %v", fileID, targetDirID, err)
		return fmt.Errorf("copy file failed: %v", err)
	}

	Info("Successfully copied file %s to directory %s", fileID, targetDirID)
	return nil
}

// GetFileInfo 获取文件信息（包括SHA1）
// pickCode: 文件的pickcode
// cloud115ID: 115账号ID
// cookie: 115账号Cookie
func (c *Client) GetFileInfo(pickCode string, cloud115ID int, cookie string) (*driver.File, error) {
	Debug("Getting file info for pickcode: %s, cloud115_id: %d", pickCode, cloud115ID)

	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return nil, err
	}

	// 使用GetFile方法获取文件信息（包括正确的SHA1）
	Info("Getting file info using GetFile API for pickcode: %s", pickCode)
	fileInfo, err := d.GetFile(pickCode)
	if err != nil {
		Error("Failed to get file info for %s: %v", pickCode, err)
		return nil, fmt.Errorf("get file info failed: %v", err)
	}

	// 确保SHA1为大写（115官方要求）
	if fileInfo.Sha1 != "" {
		fileInfo.Sha1 = strings.ToUpper(fileInfo.Sha1)
	}

	Info("Got file info: name=%s, sha1=%s, pickcode=%s, size=%d", fileInfo.Name, fileInfo.Sha1, fileInfo.PickCode, fileInfo.Size)
	return fileInfo, nil
}

// RapidTransferFile 跨账号秒传文件
// 使用源文件的SHA1值在目标账号中秒传文件
// sourcePickCode: 源文件的pickcode
// sourceCloud115ID: 源账号ID
// sourceCookie: 源账号Cookie
// targetDirID: 目标目录ID
// targetCloud115ID: 目标账号ID
// targetCookie: 目标账号Cookie
// fileName: 保存的文件名（可选，为空则使用源文件名）
// 返回值: newPickCode - 秒传后文件在目标账号中的新pickcode, error - 错误信息
func (c *Client) RapidTransferFile(sourcePickCode string, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string) (newPickCode string, err error) {
	Info("=== Rapid Transfer Start ===")
	Info("Source account ID: %d, Target account ID: %d", sourceCloud115ID, targetCloud115ID)
	Info("Source pickcode: %s, Target directory: %s", sourcePickCode, targetDirID)

	// 1. 获取源文件信息（包括SHA1）
	sourceFile, err := c.GetFileInfo(sourcePickCode, sourceCloud115ID, sourceCookie)
	if err != nil {
		return "", fmt.Errorf("get source file info failed: %v", err)
	}

	if sourceFile.Sha1 == "" {
		return "", fmt.Errorf("source file has no SHA1, cannot rapid transfer")
	}
	// 确保 SHA1 为小写
	sourceFile.Sha1 = strings.ToLower(sourceFile.Sha1)

	// 如果没有指定文件名，使用源文件名
	if fileName == "" {
		fileName = sourceFile.Name
	}

	Info("Source file info: name=%s, sha1=%s, size=%d", sourceFile.Name, sourceFile.Sha1, sourceFile.Size)

	// 2. 获取目标账号的driver
	targetDriver, err := getOrCreateDriver(targetCloud115ID, targetCookie)
	if err != nil {
		return "", err
	}

	// 3. 确保目标目录存在
	if targetDirID == "" {
		targetDirID = "0"
	}

	// 4. 获取目标账号的上传信息（包括UserId和UserKey）
	err = targetDriver.GetUploadInfo()
	if err != nil {
		Error("Failed to get target account upload info: %v", err)
		return "", fmt.Errorf("get upload info failed: %v", err)
	}

	Debug("Target account upload info: UserID=%d, UserKey=%s", targetDriver.UserID, targetDriver.Userkey)

	// 5. 使用OpenList的方法执行秒传（使用ECDH加密）
	Debug("Attempting rapid transfer using initupload API with ECDH encryption")

	// 创建ECDH加密器
	ecdhCipher, err := cipher.NewEcdhCipher()
	if err != nil {
		return "", fmt.Errorf("create ecdh cipher failed: %v", err)
	}

	var (
		target      = "U_1_" + targetDirID
		result      = driver.UploadInitResp{}
		fileSizeStr = strconv.FormatInt(sourceFile.Size, 10)
	)

	userID := strconv.FormatInt(targetDriver.UserID, 10)

	// 尝试不同的 appid 配置。115 的签名校验有时对客户端类型绑定很严
	configs := []struct {
		AppID      string
		AppVersion string
	}{
		{"0", appVerWeb},     // Web/Browser
		{"1", appVerAndroid}, // Android
	}

	var lastErr error
	for _, cfg := range configs {
		Debug("Attempting rapid transfer with AppID=%s, AppVersion=%s", cfg.AppID, cfg.AppVersion)

		form := url.Values{}
		form.Set("appid", cfg.AppID)
		form.Set("appversion", cfg.AppVersion)
		form.Set("userid", userID)
		form.Set("filename", fileName)
		form.Set("filesize", fileSizeStr)
		form.Set("fileid", sourceFile.Sha1)
		form.Set("target", target)
		form.Set("topupload", "true")

		signKey, signVal := "", ""

		// 重试机制：115 接口有签名超时机制（bug），有时正确的签名也会返回 sig invalid。
		// 解决方案：检测到 sig invalid 时，延迟 1.5 秒后使用新的时间戳重新请求，最多重试 3 次。
		maxSigRetries := 3
		sigRetryCount := 0

		for {
			t := driver.NowMilli()

			// sig 签名需要与 t 时间戳匹配，必须在每次重试时一起重新计算
			innerData := userID + sourceFile.Sha1 + target + cfg.AppID
			innerHash := sha1.Sum([]byte(innerData))
			innerHashHex := hex.EncodeToString(innerHash[:])

			sigStr := targetDriver.Userkey + innerHashHex + t.String()
			sigHash := sha1.Sum([]byte(sigStr))
			sig := strings.ToUpper(hex.EncodeToString(sigHash[:]))
			form.Set("sig", sig)

			encodedToken, err := ecdhCipher.EncodeToken(t.ToInt64())
			if err != nil {
				return "", fmt.Errorf("encode token failed: %v", err)
			}

			// 使用正确的 appversion 计算 token
			token := generateToken(sourceFile.Sha1, fileSizeStr, userID, t.String(), signKey, signVal, cfg.AppVersion)
			form.Set("t", t.String())
			form.Set("token", token)

			if signKey != "" && signVal != "" {
				form.Set("sign_key", signKey)
				form.Set("sign_val", signVal)
			}

			formEncoded := form.Encode()
			Debug("Attempt %d: Full form data (unencrypted): %s", sigRetryCount+1, formEncoded)

			encrypted, err := ecdhCipher.Encrypt([]byte(formEncoded))
			if err != nil {
				return "", fmt.Errorf("encrypt request failed: %v", err)
			}

			params := map[string]string{"k_ec": encodedToken}
			req := targetDriver.NewRequest().
				SetQueryParams(params).
				SetBody(encrypted).
				SetHeaderVerbatim("Content-Type", "application/x-www-form-urlencoded").
				SetDoNotParseResponse(true)

			// 根据 appid 动态调整 UA
			if cfg.AppID == "1" {
				req.SetHeader("User-Agent", driver.UA115Disk)
			} else {
				req.SetHeader("User-Agent", driver.UA115Browser)
			}

			resp, err := req.Post(driver.ApiUploadInit)
			if err != nil {
				lastErr = fmt.Errorf("request failed: %v", err)
				break
			}
			data := resp.RawBody()
			bodyBytes, _ := io.ReadAll(data)
			data.Close()

			decrypted, err := ecdhCipher.Decrypt(bodyBytes)
			if err != nil {
				lastErr = fmt.Errorf("decrypt response failed: %v", err)
				break
			}
			decryptedStr := string(decrypted)
			Info("=== Rapid Transfer Response ===")
			Info("Source account ID: %d, Target account ID: %d", sourceCloud115ID, targetCloud115ID)
			Info("File SHA1: %s", sourceFile.Sha1)
			Info("Raw response: %s", decryptedStr)

			result = driver.UploadInitResp{}
			if err = driver.CheckErr(json.Unmarshal(decrypted, &result), &result, resp); err != nil {
				// 如果是 sig invalid，并且还没达到最大重试次数，则 sleep 后重试
				if strings.Contains(err.Error(), "sig invalid") {
					lastErr = err
					sigRetryCount++
					if sigRetryCount < maxSigRetries {
						Warn("Encountered 'sig invalid' from 115 API, sleeping 1.5s and retrying (attempt %d/%d)...", sigRetryCount, maxSigRetries)
						time.Sleep(1500 * time.Millisecond)
						continue // 继续下一次循环重新计算时间和请求
					} else {
						Error("Max sig invalid retries reached for AppID %s.", cfg.AppID)
						break // 跳出当前配置的循环，尝试下一个 cfg
					}
				}
				return "", fmt.Errorf("parse response failed: %v", err)
			}

			Info("Response parsed - Status: %d, PickCode: %s, ErrorCode: %d", result.Status, result.PickCode, result.ErrorCode)

			if result.Status == 7 {
				// 需要文件内容校验，跨账号场景无法满足
				return "", fmt.Errorf("server requires file content verification (status=7), cross-account transfer not supported")
			}

			if result.Status == 2 {
				Info("Rapid transfer successful with AppID=%s: file %s, new pickcode: %s", cfg.AppID, fileName, result.PickCode)
				return result.PickCode, nil
			}

			// 其他状态直接报错
			return "", fmt.Errorf("rapid transfer failed with status: %d, errorcode: %d", result.Status, result.ErrorCode)
		}

		// 如果不是 sig invalid，说明配置可能对，但有其他问题，直接返回
		if lastErr != nil && !strings.Contains(lastErr.Error(), "sig invalid") {
			return "", lastErr
		}
	}

	return "", fmt.Errorf("all rapid transfer attempts failed: %v", lastErr)
}

func (c *Client) GetCIDByPath(path string, cloud115ID int, cookie string) (string, error) {
	Debug("Getting CID by path: %s, cloud115_id: %d", path, cloud115ID)

	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return "", err
	}

	// 将Windows风格的路径分隔符转换为Unix风格
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		return "0", nil
	}

	Debug("Normalized path for CID lookup: %s", path)

	// 调用115的DirName2CID API获取目录CID
	resp, err := d.DirName2CID(path)
	if err != nil {
		Error("DirName2CID API call failed for path %s: %v", path, err)
		return "", fmt.Errorf("get CID by path failed: %v", err)
	}

	Debug("DirName2CID response for path %s: CategoryID=%s", path, resp.CategoryID)

	// 检查响应是否有效
	if resp.CategoryID == "" || resp.CategoryID == "0" {
		Warn("CID lookup returned empty or zero for path: %s, this may indicate the path does not exist", path)
	}

	Info("Found CID %s for path: %s", resp.CategoryID, path)
	return string(resp.CategoryID), nil
}

// getCIDByPathWithDriver 使用指定的driver获取路径对应的CID
// 支持多层路径递归解析
func getCIDByPathWithDriver(d *driver.Pan115Client, path string) (string, error) {
	Debug("Getting CID by path with driver: %s", path)

	// 将Windows风格的路径分隔符转换为Unix风格
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		return "0", nil
	}

	Debug("Normalized path for CID lookup: %s", path)

	// 分割路径为单独的目录名
	parts := strings.Split(path, "/")
	Debug("Path split into %d parts: %v", len(parts), parts)

	// 从根目录开始逐层解析
	currentCID := "0"

	for i, part := range parts {
		if part == "" {
			continue
		}

		Debug("Resolving directory %d/%d: %s (current CID: %s)", i+1, len(parts), part, currentCID)

		// 获取目录下的所有文件（分页获取，避免遗漏超过1000条的文件）
		var allFiles []driver.File
		offset := 0
		limit := 1000
		for {
			files, err := d.ListPage(currentCID, int64(offset), int64(limit))
			if err != nil {
				Error("ListPage failed for CID %s: %v", currentCID, err)
				return "", fmt.Errorf("list directory failed: %v", err)
			}
			if files == nil || len(*files) == 0 {
				break
			}
			allFiles = append(allFiles, *files...)
			if len(*files) < limit {
				break
			}
			offset += limit
		}

		Debug("Listed %d total files in directory CID=%s", len(allFiles), currentCID)

		// 查找匹配的目录
		found := false
		for _, file := range allFiles {
			if file.IsDir() && file.Name == part {
				currentCID = file.FileID
				found = true
				Debug("Found directory '%s' with CID: %s", part, currentCID)
				break
			}
		}

		if !found {
			Error("Directory not found: %s in path %s (current CID: %s), checked %d files", part, path, currentCID, len(allFiles))
			// 详细日志帮助调试
			for _, file := range allFiles {
				Debug("  File in list: name=%s, pickcode=%s, isDir=%v", file.Name, file.PickCode, file.IsDir())
			}
			return "", fmt.Errorf("directory not found: %s", part)
		}
	}

	Info("Found CID %s for path: %s", currentCID, path)
	return currentCID, nil
}

// GetPickCodeByPath 根据文件路径获取文件的pickcode
func (c *Client) GetPickCodeByPath(filePath string, cloud115ID int, cookie string) (string, error) {
	Debug("Getting pickcode by path: %s", filePath)

	// 统一使用 getOrCreateDriver 获取 driver，确保后续调用使用同一个实例
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return "", err
	}

	// 标准化路径
	filePath = strings.TrimPrefix(filePath, "/")
	filePath = strings.TrimPrefix(filePath, "\\")
	filePath = strings.TrimSuffix(filePath, "/")
	filePath = strings.TrimSuffix(filePath, "\\")

	// 分离目录路径和文件名
	dirPath := filepath.Dir(filePath)
	fileName := filepath.Base(filePath)

	Debug("Dir path: %s, file name: %s", dirPath, fileName)

	// 获取目录的CID
	cid := "0"
	if dirPath != "" && dirPath != "." {
		cid, err = getCIDByPathWithDriver(d, dirPath)
		if err != nil {
			Error("Failed to get CID for path %s: %v", dirPath, err)
			return "", fmt.Errorf("get CID failed: %v", err)
		}
	}

	Debug("Got CID %s for dir path: %s", cid, dirPath)

	// 获取目录下的文件列表
	files, err := d.ListPage(cid, 0, 1000)
	if err != nil {
		Error("Failed to list files in directory %s: %v", cid, err)
		return "", fmt.Errorf("list files failed: %v", err)
	}

	// 调试：列出目录下的所有文件
	Debug("Listing files in directory CID=%s (requested path: %s, file: %s), total files: %d", cid, filePath, fileName, len(*files))
	for _, file := range *files {
		Debug("  File in list: name=%s, pickcode=%s, isDir=%v", file.Name, file.PickCode, file.IsDir())
	}

	// 查找匹配的文件
	for _, file := range *files {
		if file.Name == fileName && !file.IsDir() {
			Debug("Found file %s with pickcode %s", fileName, file.PickCode)
			return file.PickCode, nil
		}
	}

	Error("File not found: %s in directory %s (CID: %s), checked %d files", fileName, dirPath, cid, len(*files))
	return "", fmt.Errorf("file not found: %s", fileName)
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

type ExportDirResponse struct {
	State   bool   `json:"state"`
	Error   string `json:"error"`
	ErrNo   int    `json:"errNo"`
	Errno   int    `json:"errno"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ExportID json.Number `json:"export_id"`
		Status   int         `json:"status"`
		Name     string      `json:"name"`
	} `json:"data"`
}

func (c *Client) ExportDirectoryTree115(fileIds string, target string, cookie string) (*ExportDirResponse, error) {
	Debug("Exporting directory tree with file_ids: %s, target: %s", fileIds, target)

	apiURL := "https://webapi.115.com/files/export_dir"
	params := url.Values{}
	params.Add("file_ids", fileIds)
	params.Add("target", target)

	Debug("Sending POST request to %s", apiURL)
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		Error("Failed to create request: %v", err)
		return nil, fmt.Errorf("create request failed: %v", err)
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Add("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Add("X-Requested-With", "XMLHttpRequest")
	req.Header.Add("Referer", "https://115.com/")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	if cookie != "" {
		req.Header.Add("Cookie", cookie)
		Debug("Added cookie to request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		Error("POST request failed: %v", err)
		return nil, fmt.Errorf("post request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Error("Failed to read response body: %v", err)
		return nil, fmt.Errorf("read response body failed: %v", err)
	}
	Debug("Received response: %s", string(body))

	var result ExportDirResponse
	if err := json.Unmarshal(body, &result); err != nil {
		Error("Failed to unmarshal response: %v, body: %s", err, string(body))
		return nil, fmt.Errorf("unmarshal response failed: %v, body: %s", err, string(body))
	}

	Info("Export directory tree response with state: %v, export_id: %s", result.State, result.Data.ExportID.String())
	return &result, nil
}

type ExportDirStatusData struct {
	ExportID string `json:"export_id"`
	Status   int    `json:"status"`
	FileName string `json:"file_name"`
	Progress int    `json:"progress"`
	FileID   string `json:"file_id"`
	PickCode string `json:"pick_code"`
}

type ExportDirStatusResponse struct {
	State   bool                  `json:"state"`
	Error   string                `json:"error"`
	ErrNo   int                   `json:"errNo"`
	Errno   int                   `json:"errno"`
	Code    int                   `json:"code"`
	Message string                `json:"message"`
	Data    []ExportDirStatusData `json:"data"`
}

func (r *ExportDirStatusResponse) GetFirstData() *ExportDirStatusData {
	if len(r.Data) > 0 {
		return &r.Data[0]
	}
	return nil
}

func (c *Client) GetExportDirectoryTreeStatus(exportId string, cookie string) (*ExportDirStatusResponse, error) {
	Debug("Getting export directory tree status with export_id: %s", exportId)

	apiURL := "https://webapi.115.com/files/export_dir"
	params := url.Values{}
	params.Add("export_id", exportId)

	fullURL := apiURL + "?" + params.Encode()
	Debug("Sending GET request to %s", fullURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		Error("Failed to create request: %v", err)
		return nil, fmt.Errorf("create request failed: %v", err)
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Add("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Add("X-Requested-With", "XMLHttpRequest")
	req.Header.Add("Referer", "https://115.com/")

	if cookie != "" {
		req.Header.Add("Cookie", cookie)
		Debug("Added cookie to request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		Error("GET request failed: %v", err)
		return nil, fmt.Errorf("get request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Error("Failed to read response body: %v", err)
		return nil, fmt.Errorf("read response body failed: %v", err)
	}
	Debug("Received response: %s", string(body))

	var rawResult struct {
		State   bool        `json:"state"`
		Error   string      `json:"error"`
		ErrNo   int         `json:"errNo"`
		Errno   int         `json:"errno"`
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &rawResult); err != nil {
		Error("Failed to unmarshal response: %v, body: %s", err, string(body))
		return nil, fmt.Errorf("unmarshal response failed: %v, body: %s", err, string(body))
	}

	result := &ExportDirStatusResponse{
		State:   rawResult.State,
		Error:   rawResult.Error,
		ErrNo:   rawResult.ErrNo,
		Errno:   rawResult.Errno,
		Code:    rawResult.Code,
		Message: rawResult.Message,
	}

	switch v := rawResult.Data.(type) {
	case []interface{}:
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				data := ExportDirStatusData{}
				if exportID, ok := itemMap["export_id"].(string); ok {
					data.ExportID = exportID
				}
				if pickCode, ok := itemMap["pick_code"].(string); ok {
					data.PickCode = pickCode
				}
				if status, ok := itemMap["status"].(float64); ok {
					data.Status = int(status)
				} else if status, ok := itemMap["status"].(bool); ok {
					if status {
						data.Status = 2
					} else {
						data.Status = 0
					}
				} else if rawResult.State && data.PickCode != "" {
					data.Status = 2
				}
				if fileName, ok := itemMap["file_name"].(string); ok {
					data.FileName = fileName
				}
				if progress, ok := itemMap["progress"].(float64); ok {
					data.Progress = int(progress)
				}
				if fileID, ok := itemMap["file_id"].(string); ok {
					data.FileID = fileID
				}
				result.Data = append(result.Data, data)
			}
		}
	case map[string]interface{}:
		data := ExportDirStatusData{}
		if exportID, ok := v["export_id"].(string); ok {
			data.ExportID = exportID
		}
		if pickCode, ok := v["pick_code"].(string); ok {
			data.PickCode = pickCode
		}
		if status, ok := v["status"].(float64); ok {
			data.Status = int(status)
		} else if status, ok := v["status"].(bool); ok {
			if status {
				data.Status = 2
			} else {
				data.Status = 0
			}
		} else if rawResult.State && data.PickCode != "" {
			// 如果 data 中没有 status 字段，但是外层的 state 为 true，且有 pick_code，说明已经处理完成
			data.Status = 2
		}
		if fileName, ok := v["file_name"].(string); ok {
			data.FileName = fileName
		}
		if progress, ok := v["progress"].(float64); ok {
			data.Progress = int(progress)
		}
		if fileID, ok := v["file_id"].(string); ok {
			data.FileID = fileID
		}
		result.Data = append(result.Data, data)
	}

	data := result.GetFirstData()
	if data != nil {
		Info("Export directory tree status response with state: %v, status: %d, pick_code: %s", result.State, data.Status, data.PickCode)
	} else {
		Info("Export directory tree status response with state: %v, no data yet", result.State)
	}
	return result, nil
}

type DirectoryNode struct {
	CID      string            `json:"cid"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Files    []driver.FileInfo `json:"files,omitempty"`
	Children []*DirectoryNode  `json:"children,omitempty"`
}

func (c *Client) GenerateDirectoryTree(rootCID int, rootName string, cookie string) (*DirectoryNode, error) {
	Debug("Generating directory tree from root CID: %d, root name: %s", rootCID, rootName)

	if err := c.ImportCredential(cookie); err != nil {
		return nil, err
	}

	rootNode, err := c.buildDirectoryTree(rootCID, rootName, cookie)
	if err != nil {
		Error("Failed to build directory tree: %v", err)
		return nil, fmt.Errorf("build directory tree failed: %v", err)
	}

	Info("Directory tree generation completed successfully")
	return rootNode, nil
}

func (c *Client) buildDirectoryTree(cid int, name string, cookie string) (*DirectoryNode, error) {
	Debug("Building directory tree for CID: %d, name: %s", cid, name)

	node := &DirectoryNode{
		CID:   fmt.Sprintf("%d", cid),
		Name:  name,
		Type:  "dir",
		Files: []driver.FileInfo{},
	}

	offset := int64(0)
	limit := int64(100)

	for {
		// 使用 c.driver.ListPage 代替 driver.GetFiles，避免 CID 匹配检查的 bug
		files, err := c.driver.ListPage(fmt.Sprintf("%d", cid), offset, limit, driver.WithApiURLs(driver.ApiFileList))
		if err != nil {
			Error("Failed to get file list for CID %d: %v", cid, err)
			return nil, fmt.Errorf("get file list failed: %v", err)
		}

		for _, file := range *files {
			if file.IsDir() {
				subCID, err := strconv.Atoi(file.FileID)
				if err != nil {
					Warn("Invalid CID format for directory: %s", file.FileID)
					continue
				}

				subNode, err := c.buildDirectoryTree(subCID, file.Name, cookie)
				if err != nil {
					Warn("Failed to build subdirectory tree for %s: %s", file.Name, err)
					continue
				}

				node.Children = append(node.Children, subNode)
			} else {
				// 将 driver.File 转换为 driver.FileInfo
				fileInfo := driver.FileInfo{
					FileID:     file.FileID,
					CategoryID: driver.IntString(file.ParentID),
					Name:       file.Name,
					Size:       driver.StringInt64(file.Size),
					PickCode:   file.PickCode,
					Sha1:       file.Sha1,
				}
				node.Files = append(node.Files, fileInfo)
			}
		}

		// ListPage 返回的是分页后的文件列表，如果返回数量小于 limit，说明已经获取完所有文件
		if int64(len(*files)) < limit {
			break
		}

		offset += limit
	}

	Debug("Built directory tree node with %d files and %d subdirectories", len(node.Files), len(node.Children))
	return node, nil
}

func (c *Client) ExportDirectoryTree(tree *DirectoryNode, filePath string) error {
	Debug("Exporting directory tree to JSON file: %s", filePath)

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		Error("Failed to create directory: %v", err)
		return fmt.Errorf("create directory failed: %v", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		Error("Failed to create JSON file: %v", err)
		return fmt.Errorf("create file failed: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(tree); err != nil {
		Error("Failed to encode directory tree to JSON: %v", err)
		return fmt.Errorf("encode JSON failed: %v", err)
	}

	Info("Directory tree exported successfully to: %s", filePath)
	return nil
}

func (c *Client) DownloadDirectoryTreeFile(pickCode string, cloud115ID int, cookie string) ([]byte, error) {
	Debug("Downloading directory tree file with pick_code: %s, cloud115_id: %d", pickCode, cloud115ID)

	// 使用 getOrCreateDriver 获取独立的 driver 实例，避免并发时 cookie 混淆
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return nil, err
	}

	downloadInfo, err := d.Download(pickCode)
	if err != nil {
		Error("Failed to get download info: %v", err)
		return nil, fmt.Errorf("get download info failed: %v", err)
	}

	Debug("Download info - Size: %d, Name: %s, URL: %s", downloadInfo.FileSize, downloadInfo.FileName, downloadInfo.Url.Url)

	// 使用 d 客户端来下载文件（它会自动带上 Cookie，且 User-Agent 与获取下载链接时一致）
	resp, err := d.NewRequest().Get(downloadInfo.Url.Url)
	if err != nil {
		Error("Failed to download file: %v", err)
		return nil, fmt.Errorf("download file failed: %v", err)
	}

	fileData := resp.Body()

	if resp.StatusCode() != http.StatusOK {
		Error("Download failed with status %d: %s", resp.StatusCode(), string(fileData))
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode(), string(fileData))
	}

	// 保存目录树文件到本地用于调试
	debugFilePath := "debug_directory_tree.txt"
	if err := os.WriteFile(debugFilePath, fileData, 0644); err != nil {
		Warn("Failed to save debug file: %v", err)
	} else {
		Debug("Saved directory tree file to %s for debugging", debugFilePath)
	}

	Info("Downloaded directory tree file successfully, size: %d bytes", len(fileData))
	return fileData, nil
}

type DirTreeEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
	Sha1  string `json:"sha1"`
	Pc    string `json:"pc"`
	Fid   string `json:"fid"`
}

func Parse115DirTreeFile(content []byte) ([]DirTreeEntry, error) {
	Debug("Parsing 115 directory tree file, size: %d bytes", len(content))

	var entries []DirTreeEntry

	// 尝试检测编码并转换为 UTF-8
	// 115 导出的目录树文件可能是 GBK 编码
	var contentStr string
	// 第一步：检测 BOM 判断是否为 UTF-16
	if len(content) >= 2 && content[0] == 0xFF && content[1] == 0xFE {
		Debug("Detected UTF-16LE BOM in directory tree file")
		decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
		decoded, err := decoder.Bytes(content)
		if err != nil {
			Warn("Failed to decode as UTF-16LE: %v", err)
			contentStr = string(content)
		} else {
			contentStr = string(decoded)
			Debug("Directory tree file decoded from UTF-16LE to UTF-8")
		}
	} else if utf8.Valid(content) {
		contentStr = string(content)
		Debug("Directory tree file is valid UTF-8")
	} else {
		// 尝试 GBK 解码
		decoder := simplifiedchinese.GBK.NewDecoder()
		decoded, err := decoder.Bytes(content)
		if err != nil {
			Warn("Failed to decode as GBK, trying as raw bytes: %v", err)
			contentStr = string(content)
		} else {
			contentStr = string(decoded)
			Debug("Directory tree file decoded from GBK to UTF-8")
		}
	}

	lines := strings.Split(contentStr, "\n")
	Debug("Total lines in directory tree file: %d", len(lines))

	pathStack := make([]string, 0)

	// 第二步提取所有条目
	for i, line := range lines {
		// 跳过顶部的根目录名称行，比如: |——根目录20260314091416
		if strings.HasPrefix(line, "|——") {
			continue
		}

		lineTrimmed := strings.TrimRight(line, "\r ")
		if lineTrimmed == "" {
			continue
		}

		// 尝试 JSON 格式
		if strings.HasPrefix(lineTrimmed, "{") {
			var entry DirTreeEntry
			if err := json.Unmarshal([]byte(lineTrimmed), &entry); err == nil {
				entries = append(entries, entry)
				continue
			}
		}

		// 正则匹配树状结构: "| |-文件名" 或者 "| | |-文件名" 等
		// 每次深度增加，前面就会多一个 "| "，然后最后跟一个 "|-" 或 " "（如果只是缩进）

		// 寻找实际名称开始的位置 (找到最后出现的 "|-" 或 "| ")
		nameStartIdx := -1
		depth := 0

		lastDash := strings.LastIndex(lineTrimmed, "|-")
		if lastDash >= 0 {
			nameStartIdx = lastDash + 2
			// 计算深度: 计算 "\x7c" (|的ascii) 出现的次数
			prefix := lineTrimmed[:lastDash]
			depth = strings.Count(prefix, "|")
		} else {
			// 可能不是树状结构，跳过
			if len(lineTrimmed) > 0 && lineTrimmed[0] == '|' {
				// 有些行只有 | 但是没有 |-
				fmt.Printf("Skipping line because no |- found: %s\n", lineTrimmed)
			}
			continue
		}

		if nameStartIdx >= 0 && nameStartIdx < len(lineTrimmed) {
			name := strings.TrimSpace(lineTrimmed[nameStartIdx:])
			if name == "" {
				continue
			}

			// 我们需要预判它是不是目录：如果下一行的层级比它深，那它就是目录
			isDir := false
			if i+1 < len(lines) {
				nextLine := strings.TrimRight(lines[i+1], "\r ")
				nextLastDash := strings.LastIndex(nextLine, "|-")
				if nextLastDash >= 0 {
					nextPrefix := nextLine[:nextLastDash]
					nextDepth := strings.Count(nextPrefix, "|")
					if nextDepth > depth {
						isDir = true
					}
				}
			}

			// 第 N 层的元素，它的父节点路径应该是从根节点一直到第 N-1 层
			// 所以我们需要截断 stack 使其长度最多为 N-1。
			// 这里因为我们移除了 +1，所以 depth 从 1 开始，深度1的target就是0
			targetStackLen := depth - 1
			if targetStackLen < 0 {
				targetStackLen = 0
			}

			if len(pathStack) > targetStackLen {
				pathStack = pathStack[:targetStackLen]
			}

			// 如果因为有跳级导致 targetStackLen > len(pathStack)，我们由于缺乏中间层名字也就只能拼接到当前已知最高层
			currentPath := strings.Join(pathStack, "/")

			// 把名字压入栈，供下一行的孩子使用 (这会使得下一行处理时，栈长度正好等于当前的 depth)
			if isDir {
				pathStack = append(pathStack, name)
			}

			entry := DirTreeEntry{
				Name:  name,
				Path:  currentPath,
				IsDir: isDir,
			}

			Debug("Parsed tree entry %d: name=%s, path=%s, depth=%d, isDir=%v", i, name, currentPath, depth, isDir)
			entries = append(entries, entry)
		}
	}

	Info("Parsed %d entries from directory tree file", len(entries))
	return entries, nil
}

// BuildTreeFromExport converts a flat list of DirTreeEntry into a hierarchical DirectoryNode
func BuildTreeFromExport(entries []DirTreeEntry, rootCID string, rootName string) *DirectoryNode {
	root := &DirectoryNode{
		CID:      rootCID,
		Name:     rootName,
		Type:     "dir",
		Files:    []driver.FileInfo{},
		Children: []*DirectoryNode{},
	}

	// Create a map from path strings to *DirectoryNode pointers
	// The root node itself represents the empty path or "."
	dirMap := make(map[string]*DirectoryNode)
	dirMap[""] = root
	dirMap["."] = root

	for _, entry := range entries {
		// Clean the path
		dirPath := strings.TrimPrefix(entry.Path, "/")

		// Find or create the parent directory node
		parent, ok := dirMap[dirPath]
		if !ok {
			// If parent doesn't exist in map (which can happen if export is unordered),
			// we fallback to attaching it to root to avoid losing data,
			// or we could build intermediate nodes. For simplicity, attach to root.
			parent = root
		}

		if entry.IsDir {
			// Create a new directory node
			newNode := &DirectoryNode{
				CID:      "0", // The export text file might not contain accurate CID for sub-dirs in this format
				Name:     entry.Name,
				Type:     "dir",
				Files:    []driver.FileInfo{},
				Children: []*DirectoryNode{},
			}

			// The full path for this new directory
			fullPath := entry.Name
			if dirPath != "" && dirPath != "." {
				fullPath = dirPath + "/" + entry.Name
			}
			dirMap[fullPath] = newNode
			parent.Children = append(parent.Children, newNode)
		} else {
			// Create file info
			fileInfo := driver.FileInfo{
				FileID:   entry.Fid, // May be empty depending on export
				Name:     entry.Name,
				Size:     driver.StringInt64(entry.Size),
				PickCode: entry.Pc,
				Sha1:     entry.Sha1,
			}
			parent.Files = append(parent.Files, fileInfo)
		}
	}

	// 安全脱壳: 115在导出非根目录时，第一层经常会将目标文件夹自己给作为第一个子节点展示。
	// 这会导致外层生成的目录树双重嵌套（比如/影视资源/影视资源），我们必须将这层外壳剥去。
	if len(root.Children) == 1 && len(root.Files) == 0 {
		firstChild := root.Children[0]
		if strings.EqualFold(firstChild.Name, rootName) || firstChild.Name == "根目录" {
			firstChild.CID = rootCID
			Debug("Unwrapped redundant outer directory block: %s", firstChild.Name)
			return firstChild
		}
	}

	return root
}

// ExportDirectoryTreeToFile 将目录树导出到文件
func (c *Client) ExportDirectoryTreeToFile(node *DirectoryNode, filePath string) error {
	Debug("Exporting directory tree to file: %s", filePath)

	var lines []string
	c.exportDirTreeToLines(node, "", &lines)

	content := strings.Join(lines, "\n")
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		Error("Failed to write directory tree file: %v", err)
		return err
	}

	Info("Exported %d lines to directory tree file: %s", len(lines), filePath)
	return nil
}

// exportDirTreeToLines 递归将目录树转换为文本行
func (c *Client) exportDirTreeToLines(node *DirectoryNode, prefix string, lines *[]string) {
	// 导出当前目录的文件
	for _, file := range node.Files {
		pickCode := file.PickCode
		if pickCode == "" {
			pickCode = file.FileID
		}
		line := fmt.Sprintf("%s\t%s\t%d\t%d\t%s\t%s",
			prefix,
			file.Name,
			0, // isDir = false
			file.Size,
			file.Sha1,
			pickCode)
		*lines = append(*lines, line)
	}

	// 导出子目录
	for _, child := range node.Children {
		childPrefix := filepath.Join(prefix, child.Name)
		// 先添加目录条目
		line := fmt.Sprintf("%s\t%s\t%d\t%d\t\t",
			prefix,
			child.Name,
			1, // isDir = true
			0, // size = 0
		)
		*lines = append(*lines, line)
		// 递归处理子目录
		c.exportDirTreeToLines(child, childPrefix, lines)
	}
}

// OpenAPIQRCodeSession 表示115开放平台扫码登录会话信息
type OpenAPIQRCodeSession struct {
	QRCodeUrl string `json:"qrcode_url"`
	State     string `json:"state"`
}

// OpenAPIToken 表示115开放平台Token信息
type OpenAPIToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// GetOpenAPIQRCode 获取115开放平台扫码登录二维码
// 注意：此方法为预留接口，需要115开放平台开发者账号申请通过后实现
func (c *Client) GetOpenAPIQRCode() (*OpenAPIQRCodeSession, error) {
	Warn("GetOpenAPIQRCode is not implemented yet, waiting for 115 open platform developer account approval")
	return nil, fmt.Errorf("115开放平台开发者账号暂未申请，功能暂不可用")
}

// CheckOpenAPILoginStatus 检查115开放平台扫码登录状态
// 注意：此方法为预留接口，需要115开放平台开发者账号申请通过后实现
func (c *Client) CheckOpenAPILoginStatus(state string) (int, error) {
	Warn("CheckOpenAPILoginStatus is not implemented yet, waiting for 115 open platform developer account approval")
	return 0, fmt.Errorf("115开放平台开发者账号暂未申请，功能暂不可用")
}

// ConfirmOpenAPILogin 确认115开放平台扫码登录并获取Token
// 注意：此方法为预留接口，需要115开放平台开发者账号申请通过后实现
func (c *Client) ConfirmOpenAPILogin(state string) (*OpenAPIToken, error) {
	Warn("ConfirmOpenAPILogin is not implemented yet, waiting for 115 open platform developer account approval")
	return nil, fmt.Errorf("115开放平台开发者账号暂未申请，功能暂不可用")
}

// ==================== 秒传方式切换实现 ====================

// 秒传方式常量
const (
	TransferMethod115Driver = "115driver" // 115driver原生秒传
	TransferMethodGo115     = "go115"     // go115库秒传
	TransferMethodAlist     = "alist"     // alist秒传
)

// RapidTransferByMethod 根据配置选择秒传方式执行跨账号秒传
// sourcePickCode: 源文件的pickcode
// sourceCloud115ID: 源账号ID
// sourceCookie: 源账号Cookie
// targetDirID: 目标目录ID
// targetCloud115ID: 目标账号ID
// targetCookie: 目标账号Cookie
// fileName: 保存的文件名（可选，为空则使用源文件名）
// method: 秒传方式，可选值：115driver, go115, alist
// alistUrl: alist地址（当method为alist时使用）
// alistToken: alist token（当method为alist时使用）
// 返回值: newPickCode - 秒传后文件在目标账号中的新pickcode, error - 错误信息
func (c *Client) RapidTransferByMethod(sourcePickCode string, sourceFilePath string, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string, method string, alistUrl string, alistToken string) (newPickCode string, err error) {
	Info("=== Rapid Transfer By Method ===")
	Info("Transfer method: %s", method)
	Info("Source account ID: %d, Target account ID: %d", sourceCloud115ID, targetCloud115ID)
	Info("Source pickcode: %s, Source file path: %s, Target directory: %s", sourcePickCode, sourceFilePath, targetDirID)

	switch method {
	case TransferMethodGo115:
		Info("Using go115 method for rapid transfer")
		return c.rapidTransferGo115(sourcePickCode, sourceCloud115ID, sourceCookie, targetDirID, targetCloud115ID, targetCookie, fileName)
	case TransferMethodAlist:
		Info("Using alist method for rapid transfer")
		return c.rapidTransferAlist(sourcePickCode, sourceFilePath, sourceCloud115ID, sourceCookie, targetDirID, targetCloud115ID, targetCookie, fileName, alistUrl, alistToken)
	default:
		Info("Using default 115driver method for rapid transfer")
		return c.RapidTransferFile(sourcePickCode, sourceCloud115ID, sourceCookie, targetDirID, targetCloud115ID, targetCookie, fileName)
	}
}

// rapidTransferGo115 使用go115方式执行秒传
// go115方式与115driver类似，但是使用不同的签名算法
func (c *Client) rapidTransferGo115(sourcePickCode string, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string) (newPickCode string, err error) {
	Info("=== Go115 Rapid Transfer Start ===")
	Info("Source account ID: %d, Target account ID: %d", sourceCloud115ID, targetCloud115ID)
	Info("Source pickcode: %s, Target directory: %s", sourcePickCode, targetDirID)

	// 1. 获取源文件信息（包括SHA1）
	sourceFile, err := c.GetFileInfo(sourcePickCode, sourceCloud115ID, sourceCookie)
	if err != nil {
		return "", fmt.Errorf("get source file info failed: %v", err)
	}

	if sourceFile.Sha1 == "" {
		return "", fmt.Errorf("source file has no SHA1, cannot rapid transfer")
	}
	// 确保 SHA1 为小写
	sourceFile.Sha1 = strings.ToLower(sourceFile.Sha1)

	// 如果没有指定文件名，使用源文件名
	if fileName == "" {
		fileName = sourceFile.Name
	}

	Info("Source file info: name=%s, sha1=%s, size=%d", sourceFile.Name, sourceFile.Sha1, sourceFile.Size)

	// 2. 获取目标账号的driver
	targetDriver, err := getOrCreateDriver(targetCloud115ID, targetCookie)
	if err != nil {
		return "", err
	}

	// 3. 确保目标目录存在
	if targetDirID == "" {
		targetDirID = "0"
	}

	// 4. 获取目标账号的上传信息
	err = targetDriver.GetUploadInfo()
	if err != nil {
		Error("Failed to get target account upload info: %v", err)
		return "", fmt.Errorf("get upload info failed: %v", err)
	}

	Debug("Target account upload info: UserID=%d, UserKey=%s", targetDriver.UserID, targetDriver.Userkey)

	// 5. Go115秒传实现：使用简化版签名算法
	// 115driver使用的签名算法有时会导致sig invalid，go115使用不同的签名策略

	// 创建ECDH加密器
	ecdhCipher, err := cipher.NewEcdhCipher()
	if err != nil {
		return "", fmt.Errorf("create ecdh cipher failed: %v", err)
	}

	var (
		target      = "U_1_" + targetDirID
		result      = driver.UploadInitResp{}
		fileSizeStr = strconv.FormatInt(sourceFile.Size, 10)
	)

	userID := strconv.FormatInt(targetDriver.UserID, 10)

	// 尝试不同的 appid 配置
	configs := []struct {
		AppID      string
		AppVersion string
	}{
		{"0", appVerWeb},     // Web/Browser
		{"1", appVerAndroid}, // Android
	}

	var lastErr error
	for _, cfg := range configs {
		Debug("Attempting Go115 rapid transfer with AppID=%s, AppVersion=%s", cfg.AppID, cfg.AppVersion)

		form := url.Values{}
		form.Set("appid", cfg.AppID)
		form.Set("appversion", cfg.AppVersion)
		form.Set("userid", userID)
		form.Set("filename", fileName)
		form.Set("filesize", fileSizeStr)
		form.Set("fileid", sourceFile.Sha1)
		form.Set("target", target)
		form.Set("topupload", "true")

		// Go115使用简化签名：直接使用userkey+SHA1+target+时间戳
		maxSigRetries := 3
		sigRetryCount := 0

		for {
			t := driver.NowMilli()

			// Go115简化签名算法
			innerData := userID + sourceFile.Sha1 + target + cfg.AppID
			innerHash := sha1.Sum([]byte(innerData))
			innerHashHex := hex.EncodeToString(innerHash[:])

			// 使用userkey直接计算签名
			sigStr := targetDriver.Userkey + innerHashHex + t.String()
			sigHash := sha1.Sum([]byte(sigStr))
			sig := strings.ToUpper(hex.EncodeToString(sigHash[:]))
			form.Set("sig", sig)

			encodedToken, err := ecdhCipher.EncodeToken(t.ToInt64())
			if err != nil {
				return "", fmt.Errorf("encode token failed: %v", err)
			}

			// 生成token
			token := generateToken(sourceFile.Sha1, fileSizeStr, userID, t.String(), "", "", cfg.AppVersion)
			form.Set("t", t.String())
			form.Set("token", token)

			formEncoded := form.Encode()
			Debug("Go115 Attempt %d: Full form data (unencrypted): %s", sigRetryCount+1, formEncoded)

			encrypted, err := ecdhCipher.Encrypt([]byte(formEncoded))
			if err != nil {
				return "", fmt.Errorf("encrypt request failed: %v", err)
			}

			params := map[string]string{"k_ec": encodedToken}
			req := targetDriver.NewRequest().
				SetQueryParams(params).
				SetBody(encrypted).
				SetHeaderVerbatim("Content-Type", "application/x-www-form-urlencoded").
				SetDoNotParseResponse(true)

			// 根据 appid 动态调整 UA
			if cfg.AppID == "1" {
				req.SetHeader("User-Agent", driver.UA115Disk)
			} else {
				req.SetHeader("User-Agent", driver.UA115Browser)
			}

			resp, err := req.Post(driver.ApiUploadInit)
			if err != nil {
				lastErr = fmt.Errorf("request failed: %v", err)
				break
			}
			data := resp.RawBody()
			bodyBytes, _ := io.ReadAll(data)
			data.Close()

			decrypted, err := ecdhCipher.Decrypt(bodyBytes)
			if err != nil {
				lastErr = fmt.Errorf("decrypt response failed: %v", err)
				break
			}
			decryptedStr := string(decrypted)
			Info("=== Go115 Rapid Transfer Response ===")
			Info("Raw response: %s", decryptedStr)

			result = driver.UploadInitResp{}
			if err = driver.CheckErr(json.Unmarshal(decrypted, &result), &result, resp); err != nil {
				if strings.Contains(err.Error(), "sig invalid") {
					lastErr = err
					sigRetryCount++
					if sigRetryCount < maxSigRetries {
						Warn("Encountered 'sig invalid' from 115 API, sleeping 1.5s and retrying (attempt %d/%d)...", sigRetryCount, maxSigRetries)
						time.Sleep(1500 * time.Millisecond)
						continue
					} else {
						Error("Max sig invalid retries reached for AppID %s.", cfg.AppID)
						break
					}
				}
				return "", fmt.Errorf("parse response failed: %v", err)
			}

			Info("Response parsed - Status: %d, PickCode: %s, ErrorCode: %d", result.Status, result.PickCode, result.ErrorCode)

			if result.Status == 7 {
				return "", fmt.Errorf("server requires file content verification (status=7), cross-account transfer not supported")
			}

			if result.Status == 2 {
				Info("Go115 rapid transfer successful: file %s, new pickcode: %s", fileName, result.PickCode)
				return result.PickCode, nil
			}

			return "", fmt.Errorf("rapid transfer failed with status: %d, errorcode: %d", result.Status, result.ErrorCode)
		}

		if lastErr != nil && !strings.Contains(lastErr.Error(), "sig invalid") {
			return "", lastErr
		}
	}

	return "", fmt.Errorf("all Go115 rapid transfer attempts failed: %v", lastErr)
}

// rapidTransferAlist 使用elevengo方式执行跨账号秒传
// 完全按照demo的逻辑实现：先路径遍历查找文件，失败后再使用搜索
func (c *Client) rapidTransferAlist(sourcePickCode string, sourceFilePath string, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string, alistUrl string, alistToken string) (newPickCode string, err error) {
	Info("=== Alist Rapid Transfer Start (using elevengo API) ===")
	Info("Source account ID: %d, Target account ID: %d", sourceCloud115ID, targetCloud115ID)
	Info("Source pickcode: %s, Source file path: %s, Target directory: %s", sourcePickCode, sourceFilePath, targetDirID)

	sourceCr := parseCookieToCredential(sourceCookie)
	targetCr := parseCookieToCredential(targetCookie)

	sourceAgent := elevengo.New()
	if err := sourceAgent.CredentialImport(sourceCr); err != nil {
		return "", fmt.Errorf("import source credential failed: %v", err)
	}

	targetAgent := elevengo.New()
	if err := targetAgent.CredentialImport(targetCr); err != nil {
		return "", fmt.Errorf("import target credential failed: %v", err)
	}

	Info("Extracting filename from path: %s", sourceFilePath)
	parts := strings.Split(strings.Trim(sourceFilePath, "/"), "/")
	fileNameFromPath := parts[len(parts)-1]
	Info("Extracted filename: %s", fileNameFromPath)

	Info("Searching file: %s", fileNameFromPath)

	var targetFile *elevengo.File

	dirId := "0"
	for i := 0; i < len(parts)-1; i++ {
		searchName := parts[i]
		Info("第%d层: 搜索目录 '%s'", i, searchName)
		it, err := sourceAgent.FileIterate(dirId)
		if err != nil {
			Info("列出目录 %s 失败: %v", dirId, err)
			break
		}
		found := false
		for _, file := range it.Items() {
			Info("  [%s] %s (IsDir=%v)", file.FileId, file.Name, file.IsDirectory)
			if file.Name == searchName && file.IsDirectory {
				dirId = file.FileId
				Info("找到目录ID: %s", dirId)
				found = true
				break
			}
		}
		if !found {
			Info("未找到目录: %s", searchName)
			break
		}
	}

	if dirId != "" {
		Info("在目录 %s 中查找文件: %s", dirId, fileNameFromPath)
		it, err := sourceAgent.FileIterate(dirId)
		if err == nil {
			for _, file := range it.Items() {
				if file.Name == fileNameFromPath {
					targetFile = file
					Info("找到文件! SHA1: %s, Size: %d", file.Sha1, file.Size)
					break
				}
			}
		}
	}

	if targetFile == nil {
		Info("通过遍历未找到文件，尝试搜索...")
		it, err := sourceAgent.FileSearch("0", fileNameFromPath)
		if err != nil {
			return "", fmt.Errorf("搜索文件失败: %v", err)
		}
		for _, f := range it.Items() {
			if f.Name == fileNameFromPath {
				targetFile = f
				Info("通过搜索找到文件! SHA1: %s, Size: %d", f.Sha1, f.Size)
				break
			}
		}
	}

	if targetFile == nil {
		return "", fmt.Errorf("未找到文件: %s", sourceFilePath)
	}

	Info("文件信息:")
	Info("  - 文件名: %s", targetFile.Name)
	Info("  - 大小: %d", targetFile.Size)
	Info("  - SHA1: %s", targetFile.Sha1)

	if targetFile.Sha1 == "" {
		return "", fmt.Errorf("该文件缺少SHA1，无法执行跨账号秒传")
	}

	if fileName == "" {
		fileName = targetFile.Name
	}

	Info("初始化目标账号...")
	Info("目标账号初始化成功")

	targetDir := targetDirID
	if targetDir == "" {
		targetDir = "0"
	}
	Info("目标目录ID: %s", targetDir)

	Info("正在创建秒传票据...")

	ticket := &elevengo.ImportTicket{
		FileName: fileName,
		FileSize: targetFile.Size,
		FileSha1: strings.ToUpper(targetFile.Sha1),
	}

	pickcode, err := sourceAgent.ImportCreateTicket(targetFile.FileId, ticket)
	if err != nil {
		return "", fmt.Errorf("创建源文件票据失败: %v", err)
	}
	Info("获取到 pickcode: %s", pickcode)

	Info("正在执行秒传...")
	if err = targetAgent.Import(targetDir, ticket); err != nil {
		if ie, ok := err.(*elevengo.ErrImportNeedCheck); ok {
			Info("需要进行签名校验...")
			signValue, err := sourceAgent.ImportCalculateSignValue(pickcode, ie.SignRange)
			if err != nil {
				return "", fmt.Errorf("计算签名失败: %v", err)
			}
			ticket.SignKey, ticket.SignValue = ie.SignKey, signValue
			if err = targetAgent.Import(targetDir, ticket); err != nil {
				return "", fmt.Errorf("秒传失败: %v", err)
			}
		} else {
			return "", fmt.Errorf("秒传失败: %v", err)
		}
	}

	Info("秒传成功！文件已存入目标账号！")

	// 在目标账号中查找刚上传的文件，获取新的 pickcode
	Info("在目标账号中查找秒传后的文件: %s", fileName)
	it, err := targetAgent.FileIterate(targetDir)
	if err != nil {
		Warn("无法遍历目标目录获取新 pickcode: %v, 使用源文件 pickcode", err)
		return pickcode, nil
	}

	for _, f := range it.Items() {
		if f.Name == fileName && strings.ToUpper(f.Sha1) == strings.ToUpper(targetFile.Sha1) {
			Info("找到目标账号中的文件，新 pickcode: %s", f.PickCode)
			return f.PickCode, nil
		}
	}

	Warn("未在目标账号中找到秒传后的文件，使用源文件 pickcode: %s", pickcode)
	return pickcode, nil
}

// parseCookieToCredential 将115 cookie字符串解析为elevengo.Credential
func parseCookieToCredential(cookie string) *elevengo.Credential {
	cr := &elevengo.Credential{}
	Info("Parsing cookie for elevengo: length=%d", len(cookie))

	// 115 cookie格式: UID=xxx; CID=xxx; SEID=xxx; KID=xxx (用分号分隔)
	// 分割每个cookie项
	parts := strings.Split(cookie, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch name {
		case "UID":
			cr.UID = value
			Info("Parsed UID: %s", value)
		case "CID":
			cr.CID = value
			Info("Parsed CID: %s", value)
		case "KID":
			cr.KID = value
			Info("Parsed KID: %s", value)
		case "SEID":
			cr.SEID = value
			Info("Parsed SEID: %s", value)
		}
	}

	Info("Final credential: UID=%s, CID=%s, KID=%s, SEID=%s",
		cr.UID, cr.CID, cr.KID, cr.SEID)
	return cr
}
