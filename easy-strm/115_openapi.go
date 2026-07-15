package main

import "fmt"

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
// TODO: implement after 115 open platform developer account is approved
// 注意：此方法为预留接口，需要115开放平台开发者账号申请通过后实现

func (c *Client) GetOpenAPIQRCode() (*OpenAPIQRCodeSession, error) {
	Warn("GetOpenAPIQRCode is not implemented yet, waiting for 115 open platform developer account approval")
	return nil, fmt.Errorf("115开放平台开发者账号暂未申请，功能暂不可用")
}

// CheckOpenAPILoginStatus 检查115开放平台扫码登录状态
// TODO: implement after 115 open platform developer account is approved
// 注意：此方法为预留接口，需要115开放平台开发者账号申请通过后实现

func (c *Client) CheckOpenAPILoginStatus(state string) (int, error) {
	Warn("CheckOpenAPILoginStatus is not implemented yet, waiting for 115 open platform developer account approval")
	return 0, fmt.Errorf("115开放平台开发者账号暂未申请，功能暂不可用")
}

// ConfirmOpenAPILogin 确认115开放平台扫码登录并获取Token
// TODO: implement after 115 open platform developer account is approved
// 注意：此方法为预留接口，需要115开放平台开发者账号申请通过后实现

func (c *Client) ConfirmOpenAPILogin(state string) (*OpenAPIToken, error) {
	Warn("ConfirmOpenAPILogin is not implemented yet, waiting for 115 open platform developer account approval")
	return nil, fmt.Errorf("115开放平台开发者账号暂未申请，功能暂不可用")
}
