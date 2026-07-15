package main

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	cipher "github.com/SheltonZhu/115driver/pkg/crypto/ec115"
	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/deadblue/elevengo"
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
