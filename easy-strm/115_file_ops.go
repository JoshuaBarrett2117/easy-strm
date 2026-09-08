package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/deadblue/elevengo"
)

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

	// Driver.GetFile 接收的是 file_id，不能传 pick_code，否则115返回990002参数错误。
	// 115搜索响应中的offset可能是字符串，不能直接使用Driver中固定为int的SearchResult。
	Info("Getting file info using Search API for pickcode: %s", pickCode)
	searchResult := struct {
		driver.BasicResp
		Files  []driver.FileInfo `json:"data"`
		Offset driver.IntString  `json:"offset"`
	}{}
	req := d.NewRequest().
		SetQueryParams(map[string]string{
			"aid": "7", "cid": "0", "offset": "0", "limit": "1",
			"pick_code": pickCode, "type": "0", "count_folders": "1",
		}).
		SetResult(&searchResult).
		ForceContentType("application/json;charset=UTF-8")
	resp, requestErr := req.Get(driver.ApiFileSearch)
	if err := driver.CheckErr(requestErr, &searchResult, resp); err != nil {
		Error("Failed to get file info for %s: %v", pickCode, err)
		return nil, fmt.Errorf("get file info failed: %v", err)
	}
	files := make([]driver.File, 0, len(searchResult.Files))
	for index := range searchResult.Files {
		files = append(files, *(&driver.File{}).From(&searchResult.Files[index]))
	}
	fileInfo := findFileByPickCode(files, pickCode)
	if fileInfo == nil {
		return nil, fmt.Errorf("get file info failed: pickcode不存在或文件已删除")
	}

	// 确保SHA1为大写（115官方要求）
	if fileInfo.Sha1 != "" {
		fileInfo.Sha1 = strings.ToUpper(fileInfo.Sha1)
	}

	Info("Got file info: name=%s, sha1=%s, pickcode=%s, size=%d", fileInfo.Name, fileInfo.Sha1, fileInfo.PickCode, fileInfo.Size)
	return fileInfo, nil
}

func findFileByPickCode(files []driver.File, pickCode string) *driver.File {
	for index := range files {
		if files[index].PickCode == pickCode {
			return &files[index]
		}
	}
	return nil
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

	Debug("Resolved CID: path=%s cid=%s", path, resp.CategoryID)
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

	Debug("Resolved CID: path=%s cid=%s", path, currentCID)
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

// RenameFile 重命名115云盘文件
// 参数:
//   - fileID: 文件ID
//   - newName: 新文件名
//   - cloud115ID: 115账号ID
//   - cookie: 115账号Cookie
//
// 返回:
//   - error: 错误信息
func (c *Client) RenameFile(fileID, newName string, cloud115ID int, cookie string) error {
	Debug("Renaming file %s to %s for cloud115_id: %d", fileID, newName, cloud115ID)

	// 使用 elevengo API 进行重命名
	cr := parseCookieToCredential(cookie)
	agent := elevengo.New()
	if err := agent.CredentialImport(cr); err != nil {
		return fmt.Errorf("import credential failed: %v", err)
	}

	// 调用 elevengo 的 FileRename 方法
	if err := agent.FileRename(fileID, newName); err != nil {
		Error("Failed to rename file %s to %s: %v", fileID, newName, err)
		return fmt.Errorf("rename file failed: %v", err)
	}

	Info("Successfully renamed file %s to %s", fileID, newName)
	return nil
}

// MoveFile115 移动文件或目录
// 参数:
//   - fileID: 需要移动的文件或目录ID
//   - targetDirID: 目标目录ID
//   - cloud115ID: 115账号ID
//   - cookie: 115账号cookie
//
// 返回:
//   - error: 错误信息
func (c *Client) MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error {
	Debug("Moving file %s to dir %s for cloud115_id: %d", fileID, targetDirID, cloud115ID)

	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return err
	}

	if targetDirID == "" {
		targetDirID = "0"
	}

	err = d.Move(targetDirID, fileID)
	if err != nil {
		Error("Failed to move file %s to dir %s: %v", fileID, targetDirID, err)
		return fmt.Errorf("move file failed: %v", err)
	}

	Info("Successfully moved file %s to dir %s", fileID, targetDirID)
	return nil
}

// MkdirAll115 递归创建目录树并返回最终 CID
func (c *Client) MkdirAll115(path string, cloud115ID int, cookie string) (string, error) {
	Debug("MkdirAll for path: %s", path)

	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return "", err
	}

	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		return "0", nil
	}

	parts := strings.Split(path, "/")
	currentCID := "0"

	for _, part := range parts {
		if part == "" {
			continue
		}

		// 检查目录是否存在
		var allFiles []driver.File
		offset := 0
		limit := 1000
		for {
			files, err := d.ListPage(currentCID, int64(offset), int64(limit))
			if err != nil {
				return "", fmt.Errorf("list directory failed at %s: %v", currentCID, err)
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

		found := false
		for _, f := range allFiles {
			if f.IsDir() && f.Name == part {
				currentCID = f.FileID
				found = true
				break
			}
		}

		if !found {
			// 未找到，需用 Mkdir 添加该级目录 (Mkdir 返回新目录的 JSON 结果但可能没有强封装返回子 CID。115driver 的 Mkdir 返回 string 类型的分类 ID)
			newDirID, err := d.Mkdir(currentCID, part)
			if err != nil {
				Error("Failed to create dir %s under %s: %v", part, currentCID, err)
				return "", fmt.Errorf("mkdir %s failed: %v", part, err)
			}

			// 为了防止 115 并发风控导致限流
			time.Sleep(200 * time.Millisecond)

			currentCID = newDirID
			Info("Created new directory %s with CID %s", part, currentCID)
		}
	}

	return currentCID, nil
}
