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
	// RapidTransferFile 秒传文件（分享转存专用）
	RapidTransferFile(sourcePickCode string, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string) (newPickCode string, err error)
	// GetFileInfo 获取文件信息（分享转存专用）
	GetFileInfo(pickCode string, cloud115ID int, cookie string) (*driver.File, error)
	// GetShareSnap 获取分享快照（分享链接解析专用）
	// 调用 115 官方接口 GET https://115cdn.com/webapi/share/snap，
	// 由 115driver 内部完成 query 参数拼接与 Referer 头设置
	// queries: 可选的分页参数（driver.QueryLimit / driver.QueryOffset）
	GetShareSnap(shareCode, receiveCode, dirID string, queries ...driver.Query) (*driver.ShareSnapResp, error)
	// ReceiveShare 将分享文件转存到目标账号指定目录
	// 调用 115 官方接口 POST https://webapi.115.com/share/receive
	// shareCode: 分享码, receiveCode: 提取码, fileIDs: 逗号分隔的分享文件fid列表,
	// saveFolderID: 目标目录cid(根目录传"0"), targetCloud115ID: 目标账号ID, targetCookie: 目标账号Cookie
	ReceiveShare(shareCode, receiveCode, fileIDs, saveFolderID string, targetCloud115ID int, targetCookie string) error
}
