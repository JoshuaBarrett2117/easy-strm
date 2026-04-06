package service

import (
	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// Cloud115Client 115客户端接口
// 包含批量整理与常用文件操作所必需的 API
type Cloud115Client interface {
	GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error)
	GetPickCodeByPath(filePath string, cloud115ID int, cookie string) (string, error)
	GetCIDByPath(path string, cloud115ID int, cookie string) (string, error)
	RenameFile(fileID, newName string, cloud115ID int, cookie string) error
	CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error
	MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error
	MkdirAll115(path string, cloud115ID int, cookie string) (string, error)
}
