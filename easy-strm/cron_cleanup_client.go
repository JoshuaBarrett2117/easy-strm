package main

import (
	"context"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"fmt"
	"net/http"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// cleanup115Client 为破坏性任务使用独立的可取消客户端，避免修改共享驱动状态。
type cleanup115Client struct{}
type cleanupTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t cleanupTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req.Clone(t.ctx))
}

func newCleanupDriver(ctx context.Context, account *domain.Cloud115) (*driver.Pan115Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	credential := &driver.Credential{}
	if err := credential.FromCookie(account.Cookie); err != nil {
		return nil, fmt.Errorf("115账号登录信息无效")
	}
	client := NewProxyAwareHTTPClient(30 * time.Second)
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = cleanupTransport{ctx: ctx, base: base}
	return driver.New(driver.WithClient(client), driver.UA(driver.UA115Browser)).ImportCredential(credential), nil
}

// List 即时列出目录的直属子项，保留服务端父目录ID供业务层核验。
func (cleanup115Client) List(ctx context.Context, account *domain.Cloud115, cid string, offset, limit int) ([]service.CleanupEntry, error) {
	d, err := newCleanupDriver(ctx, account)
	if err != nil {
		return nil, err
	}
	files, err := d.ListPage(cid, int64(offset), int64(limit))
	if err != nil {
		return nil, fmt.Errorf("115目录读取失败: %w", err)
	}
	if files == nil {
		return nil, fmt.Errorf("115目录响应为空")
	}
	entries := make([]service.CleanupEntry, 0, len(*files))
	for _, file := range *files {
		entries = append(entries, service.CleanupEntry{ID: file.GetID(), ParentID: file.ParentID, Name: file.GetName(), Directory: file.IsDir()})
	}
	return entries, nil
}

// Delete 在任务上下文和HTTP超时约束下删除指定子项。
func (cleanup115Client) Delete(ctx context.Context, account *domain.Cloud115, ids []string) error {
	d, err := newCleanupDriver(ctx, account)
	if err != nil {
		return err
	}
	return d.Delete(ids...)
}
