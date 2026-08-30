package service

import (
	"context"
	"testing"

	"easy-strm/internal/domain"
)

type fakeTelegramShareTransfer struct {
	parsed      *domain.ParseShareResponse
	request     domain.TransferRequest
	parseURL    string
	parseSecret string
}

func (f *fakeTelegramShareTransfer) ParseShareLink(_ context.Context, rawURL, password string) (*domain.ParseShareResponse, error) {
	f.parseURL = rawURL
	f.parseSecret = password
	return f.parsed, nil
}

func (f *fakeTelegramShareTransfer) SubmitTransfer(_ context.Context, request domain.TransferRequest) (*domain.TransferResponse, error) {
	f.request = request
	return &domain.TransferResponse{TaskId: "share-task", TotalFiles: len(request.Files)}, nil
}

type fakeTelegramOfflineDownload struct {
	request domain.OfflineDownloadSubmitRequest
}

func (f *fakeTelegramOfflineDownload) Submit(_ context.Context, request domain.OfflineDownloadSubmitRequest) (*domain.OfflineDownloadSubmitResponse, error) {
	f.request = request
	return &domain.OfflineDownloadSubmitResponse{TaskId: "offline-task", Total: len(request.Urls), Accepted: len(request.Urls)}, nil
}

type fakeTelegramAccountStore struct {
	accounts []*domain.Cloud115
}

func (f *fakeTelegramAccountStore) GetAll(string, string) ([]*domain.Cloud115, error) {
	return f.accounts, nil
}

func TestParseTelegramResourceRequest(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		handled   bool
		kind      string
		account   string
		directory string
	}{
		{name: "分享链接", text: "https://115cdn.com/s/swssl7n3h94?password=gcb8#", handled: true, kind: telegramResourceShare},
		{name: "分享目录", text: "https://115cdn.com/s/swssl7n3h94?password=gcb8# /自动转存", handled: true, kind: telegramResourceShare, directory: "/自动转存"},
		{name: "分享账号目录", text: "https://115cdn.com/s/swssl7n3h94?password=gcb8# 115主号 /自动转存", handled: true, kind: telegramResourceShare, account: "115主号", directory: "/自动转存"},
		{name: "分享账号", text: "https://115cdn.com/s/swssl7n3h94?password=gcb8# 115 主号", handled: true, kind: telegramResourceShare, account: "115 主号"},
		{name: "云下载", text: "magnet:?xt=urn:btih:abc VIP观影号 /下载", handled: true, kind: telegramResourceOffline, account: "VIP观影号", directory: "/下载"},
		{name: "普通文本", text: "查看下载", handled: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, handled, err := parseTelegramResourceRequest(test.text)
			if err != nil {
				t.Fatal(err)
			}
			if handled != test.handled || request.Kind != test.kind || request.AccountName != test.account || request.Directory != test.directory {
				t.Fatalf("解析结果异常: handled=%v request=%#v", handled, request)
			}
		})
	}
}

func TestSelectTelegramResourceAccount(t *testing.T) {
	accounts := []*domain.Cloud115{
		{ID: 4, Name: "资源低", AccountType: domain.AccountTypeResource, Priority: 5, Status: domain.AccountStatusActive, Cookie: "cookie"},
		{ID: 3, Name: "资源同级大ID", AccountType: domain.AccountTypeResource, Priority: 10, Status: domain.AccountStatusActive, Cookie: "cookie"},
		{ID: 2, Name: "资源同级小ID", AccountType: domain.AccountTypeResource, Priority: 10, Status: domain.AccountStatusActive, Cookie: "cookie"},
		{ID: 5, Name: "兼顾", AccountType: domain.AccountTypeBoth, Priority: 10, Status: domain.AccountStatusActive, Cookie: "cookie"},
		{ID: 6, Name: "VIP", AccountType: domain.AccountTypeVIP, Priority: 1, Status: domain.AccountStatusActive, Cookie: "cookie"},
		{ID: 1, Name: "停用VIP", AccountType: domain.AccountTypeVIP, Priority: 10, Status: domain.AccountStatusDisabled, Cookie: "cookie"},
	}
	shareAccount, err := selectTelegramResourceAccount(accounts, "", telegramResourceShare)
	if err != nil || shareAccount.ID != 2 {
		t.Fatalf("分享默认账号应按优先级降序、ID升序选择资源号: account=%#v err=%v", shareAccount, err)
	}
	offlineAccount, err := selectTelegramResourceAccount(accounts, "", telegramResourceOffline)
	if err != nil || offlineAccount.ID != 6 {
		t.Fatalf("云下载应优先选择VIP账号: account=%#v err=%v", offlineAccount, err)
	}
	specified, err := selectTelegramResourceAccount(accounts, "兼顾", telegramResourceShare)
	if err != nil || specified.ID != 5 {
		t.Fatalf("显式指定账号不应受默认类型限制: account=%#v err=%v", specified, err)
	}
}

func TestTelegramResourceServiceSubmitsShareAndOffline(t *testing.T) {
	share := &fakeTelegramShareTransfer{parsed: &domain.ParseShareResponse{
		ShareCode: "swssl7n3h94",
		Files: []domain.ShareFileInfo{
			{Fid: "fid-1", Name: "目录", IsDir: true},
			{Name: "无ID文件"},
		},
	}}
	offline := &fakeTelegramOfflineDownload{}
	accounts := &fakeTelegramAccountStore{accounts: []*domain.Cloud115{
		{ID: 8, Name: "资源号", AccountType: domain.AccountTypeResource, Priority: 8, Status: domain.AccountStatusActive, Cookie: "cookie", TransferDirectory: "/默认转存"},
		{ID: 9, Name: "VIP号", AccountType: domain.AccountTypeVIP, Priority: 1, Status: domain.AccountStatusActive, Cookie: "cookie"},
	}}
	service := NewTelegramResourceService(share, offline, accounts)

	shareCard, handled, err := service.Execute(context.Background(), "https://115cdn.com/s/swssl7n3h94?password=gcb8#")
	if err != nil || !handled {
		t.Fatalf("提交分享转存失败: handled=%v err=%v", handled, err)
	}
	if share.request.TargetCloud115Id != 8 || share.request.TargetDirectory != "/默认转存" || share.request.Password != "gcb8" || len(share.request.Files) != 1 {
		t.Fatalf("分享转存请求异常: %#v", share.request)
	}
	if shareCard.Title != "115 分享转存已提交" {
		t.Fatalf("分享提交卡片异常: %#v", shareCard)
	}

	offlineCard, handled, err := service.Execute(context.Background(), "magnet:?xt=urn:btih:abc /指定下载")
	if err != nil || !handled {
		t.Fatalf("提交云下载失败: handled=%v err=%v", handled, err)
	}
	if offline.request.Cloud115ID != 9 || offline.request.Directory != "/指定下载" || len(offline.request.Urls) != 1 {
		t.Fatalf("云下载请求异常: %#v", offline.request)
	}
	if offlineCard.Title != "115 云下载已提交" {
		t.Fatalf("云下载提交卡片异常: %#v", offlineCard)
	}
}
