package main

import (
	"fmt"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// AddOfflineTasks 向115离线下载（云下载）批量提交下载链接
// 支持 ed2k、magnet、http/https、ftp 链接；saveDirID 为保存目录cid。
// 返回与 uris 顺序一一对应的 info_hash 列表，空字符串表示115未接受该链接。
func (c *Client) AddOfflineTasks(uris []string, saveDirID string, cloud115ID int, cookie string) ([]string, error) {
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return nil, err
	}

	hashes, err := d.AddOfflineTaskURIs(uris, saveDirID)
	if err != nil {
		Error("[OfflineDownload] Failed to add offline tasks for cloud115_id %d: %v", cloud115ID, err)
		return nil, fmt.Errorf("添加115离线下载任务失败: %w", err)
	}

	Info("[OfflineDownload] Added %d offline tasks for cloud115_id %d, saveDirID=%s", len(hashes), cloud115ID, saveDirID)
	return hashes, nil
}

// ListOfflineTasks 查询指定账号的115离线下载任务列表
// page 从1开始；返回结构包含分页信息与任务明细。
func (c *Client) ListOfflineTasks(page int64, cloud115ID int, cookie string) (*driver.OfflineTaskResp, error) {
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return nil, err
	}

	resp, err := d.ListOfflineTask(page)
	if err != nil {
		return nil, fmt.Errorf("查询115离线下载任务列表失败: %w", err)
	}
	return &resp, nil
}

// DeleteOfflineTasks 删除115离线下载任务
// deleteFiles 为 true 时同时删除已下载完成的文件，false 仅移除任务记录。
func (c *Client) DeleteOfflineTasks(hashes []string, deleteFiles bool, cloud115ID int, cookie string) error {
	d, err := getOrCreateDriver(cloud115ID, cookie)
	if err != nil {
		return err
	}

	if err := d.DeleteOfflineTasks(hashes, deleteFiles); err != nil {
		return fmt.Errorf("删除115离线下载任务失败: %w", err)
	}
	return nil
}
