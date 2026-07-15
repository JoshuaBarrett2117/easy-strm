package main

import (
	"encoding/json"
	"fmt"
	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

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
