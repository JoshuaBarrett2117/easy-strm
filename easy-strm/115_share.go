package main

import (
	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// GetShareSnap 获取115分享快照（分享链接解析专用）
// 底层调用 115driver 的 Pan115Client.GetShareSnap，请求地址为
// GET https://115cdn.com/webapi/share/snap，参数通过 query 传递，
// 并由 driver 自动设置 Referer: https://115cdn.com/s/{shareCode}?password={receiveCode}
// 公开分享无需登录态，因此使用 driver.Default() 默认客户端
//
// 参数:
//   - shareCode: 分享码（从分享链接 /s/ 路径提取）
//   - receiveCode: 分享提取码（无密码分享传空字符串）
//   - dirID: 要列出的分享内目录ID，根目录传 "0"
//
// 返回:
//   - *driver.ShareSnapResp: 分享快照响应（文件列表在 Data.List 中）
//   - error: 分享不存在/密码错误等情况下由 driver 封装的 API 错误
func (c *Client) GetShareSnap(shareCode, receiveCode, dirID string, queries ...driver.Query) (*driver.ShareSnapResp, error) {
	Info("[ShareSnap] Getting share snap, shareCode: %s, dirID: %s", shareCode, dirID)

	resp, err := driver.Default().GetShareSnap(shareCode, receiveCode, dirID, queries...)
	if err != nil {
		Error("[ShareSnap] Failed to get share snap for shareCode %s: %v", shareCode, err)
		return nil, err
	}

	Info("[ShareSnap] Got share snap for shareCode %s, count: %d, files: %d",
		shareCode, resp.Data.Count, len(resp.Data.List))
	return resp, nil
}
