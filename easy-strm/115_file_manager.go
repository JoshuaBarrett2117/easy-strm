package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// CreateDirectory115 在指定115父目录中创建目录并返回新目录CID。
func (c *Client) CreateDirectory115(parentID, name string, cloud115ID int, cookie string) (string, error) {
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return "", err
	}
	if parentID == "" {
		parentID = "0"
	}
	const pageSize = 1000
	for offset := int64(0); ; offset += pageSize {
		entries, listErr := d.ListPage(parentID, offset, pageSize)
		if listErr != nil {
			return "", fmt.Errorf("list 115 parent directory failed: %w", listErr)
		}
		if entries == nil || len(*entries) == 0 {
			break
		}
		for _, entry := range *entries {
			if entry.IsDir() && entry.Name == name {
				return entry.FileID, nil
			}
		}
		if len(*entries) < pageSize {
			break
		}
	}
	cid, err := d.Mkdir(parentID, name)
	if err != nil {
		return "", fmt.Errorf("create 115 directory failed: %w", err)
	}
	return cid, nil
}

// DeleteFiles115 批量删除115文件或目录。
func (c *Client) DeleteFiles115(fileIDs []string, cloud115ID int, cookie string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return err
	}
	if err := d.Delete(fileIDs...); err != nil {
		return fmt.Errorf("delete 115 files failed: %w", err)
	}
	return nil
}

// UploadLocalFile115 将本地文件上传到指定115目录。
func (c *Client) UploadLocalFile115(filePath, targetDirID, fileName string, cloud115ID int, cookie string) error {
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open upload file failed: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat upload file failed: %w", err)
	}
	if targetDirID == "" {
		targetDirID = "0"
	}
	if fileName == "" {
		fileName = info.Name()
	}
	if err := d.UploadFastOrByMultipart(targetDirID, fileName, info.Size(), file); err != nil {
		return fmt.Errorf("upload file to 115 failed: %w", err)
	}
	return nil
}

// DownloadFile115 将115文件下载到本地目标路径，成功后原子替换目标文件。
func (c *Client) DownloadFile115(pickCode, targetPath string, cloud115ID int, cookie string) error {
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return err
	}
	info, err := d.DownloadWithUAByAndroidAPI(pickCode, "")
	if err != nil {
		return fmt.Errorf("get 115 download link failed: %w", err)
	}
	request, err := http.NewRequest(http.MethodGet, info.Url.Url, nil)
	if err != nil {
		return err
	}
	request.Header = info.Header.Clone()
	normalize115DownloadCookies(request.Header)
	request.Header.Set("Range", "bytes=0-")
	downloader := *c.httpClient
	downloader.Timeout = 0
	downloader.CheckRedirect = func(redirected *http.Request, _ []*http.Request) error {
		redirected.Header = request.Header.Clone()
		return nil
	}
	response, err := downloader.Do(request)
	if err != nil {
		return fmt.Errorf("download 115 file request failed: %w", err)
	}
	body := response.Body
	defer body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(body, 1024))
		return fmt.Errorf("download 115 file failed: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(detail)))
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(targetPath), ".easy-strm-download-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	completed := false
	defer func() {
		temp.Close()
		if !completed {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := io.Copy(temp, body); err != nil {
		return fmt.Errorf("write downloaded file failed: %w", err)
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return err
	}
	completed = true
	return nil
}

// normalize115DownloadCookies 将驱动返回的响应 Cookie 转换为合法的请求 Cookie。
func normalize115DownloadCookies(header http.Header) {
	request := &http.Request{Header: header}
	validCookies := make([]*http.Cookie, 0)
	for _, cookie := range request.Cookies() {
		if cookie.Value == "" || isCookieAttribute(cookie.Name) {
			continue
		}
		validCookies = append(validCookies, &http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	header.Del("Cookie")
	for _, cookie := range validCookies {
		request.AddCookie(cookie)
	}
}

func isCookieAttribute(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "path", "domain", "expires", "max-age", "secure", "httponly", "samesite", "partitioned":
		return true
	default:
		return false
	}
}
