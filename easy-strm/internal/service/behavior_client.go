package service

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// BehaviorNode 是列表返回的文件或目录身份，不把数字ID转成浮点数。
type BehaviorNode struct {
	ID, ParentID, Name, SHA1 string
	Size                     int64
	IsDir                    bool
}

// BehaviorListing 包含用于验证祖先链的服务端目录路径。
type BehaviorListing struct {
	Nodes, Ancestors []BehaviorNode
	Count            int
}

// BehaviorRemote 为行为同步限定只读115能力，所有调用均接受取消。
type BehaviorRemote interface {
	Events(context.Context, string, int) (domain.BehaviorPage, error)
	Resolve(context.Context, string, string) (string, error)
	List(context.Context, string, string, int) (BehaviorListing, error)
}

// BehaviorHTTPClient 用账号Cookie访问webapi，统一超时和至少一秒的请求间隔。
type BehaviorHTTPClient struct {
	http *http.Client
	base string
	gate sync.Mutex
	last time.Time
}

// NewBehaviorHTTPClient 创建共享限流客户端，不接受proapi令牌。
func NewBehaviorHTTPClient(client *http.Client) *BehaviorHTTPClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &BehaviorHTTPClient{http: &copyClient, base: "https://webapi.115.com"}
}

func validBehaviorID(id string) bool {
	if id == "" {
		return false
	}
	for _, character := range id {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (client *BehaviorHTTPClient) get(ctx context.Context, cookie, endpoint string, query url.Values, result any) error {
	if strings.TrimSpace(cookie) == "" {
		return fmt.Errorf("115行为同步缺少账号Cookie")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client.gate.Lock()
	defer client.gate.Unlock()
	if wait := time.Until(client.last.Add(time.Second)); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.base+endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("构造115请求失败")
	}
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Referer", "https://115.com/")
	client.last = time.Now()
	response, err := client.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("115请求失败 (%s)", endpoint)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("115接口%s返回HTTP %d", endpoint, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil || len(data) > 8*1024*1024 {
		return fmt.Errorf("115响应读取失败或超过大小限制")
	}
	var envelope struct {
		State bool `json:"state"`
	}
	if json.Unmarshal(data, &envelope) != nil || !envelope.State {
		return fmt.Errorf("115接口%s失败或Cookie失效", endpoint)
	}
	if err = json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("115接口%s响应格式无效", endpoint)
	}
	return nil
}

// Events 固定每页1000条，从offset=0开始消费降序事件。
func (client *BehaviorHTTPClient) Events(ctx context.Context, cookie string, offset int) (domain.BehaviorPage, error) {
	var response struct {
		State    bool            `json:"state"`
		Count    int             `json:"count"`
		NextPage bool            `json:"next_page"`
		Data     json.RawMessage `json:"data"`
	}
	if err := client.get(ctx, cookie, "/user/behavior/detail", url.Values{"limit": {"1000"}, "offset": {strconv.Itoa(offset)}}, &response); err != nil {
		return domain.BehaviorPage{}, err
	}
	if len(response.Data) == 0 {
		return domain.BehaviorPage{}, fmt.Errorf("115行为响应缺少data")
	}
	var page domain.BehaviorPage
	var err error
	switch response.Data[0] {
	case '{':
		err = json.Unmarshal(response.Data, &page)
	case '[':
		page.Count, page.NextPage = response.Count, response.NextPage
		err = json.Unmarshal(response.Data, &page.List)
	default:
		return domain.BehaviorPage{}, fmt.Errorf("115行为响应data必须是分页对象或事件数组")
	}
	if err != nil {
		return domain.BehaviorPage{}, fmt.Errorf("115行为分页响应格式无效: %w", err)
	}
	if page.List == nil {
		return domain.BehaviorPage{}, fmt.Errorf("115行为响应缺少有效list数组")
	}
	for _, event := range page.List {
		if event.ID <= 0 {
			return domain.BehaviorPage{}, fmt.Errorf("115行为响应事件ID无效")
		}
	}
	page.State = response.State
	return page, nil
}

// Resolve 只解析明确配置的网盘路径；非根路径返回0时视为失败。
func (client *BehaviorHTTPClient) Resolve(ctx context.Context, cookie, cloudPath string) (string, error) {
	if cloudPath == "/" {
		return "0", nil
	}
	var result struct {
		ID driver.IntString `json:"id"`
	}
	err := client.get(ctx, cookie, "/files/getid", url.Values{"path": {strings.Trim(cloudPath, "/")}}, &result)
	if err != nil {
		return "", err
	}
	if !validBehaviorID(string(result.ID)) || result.ID == "0" {
		return "", fmt.Errorf("配置路径无法解析为有效CID")
	}
	return string(result.ID), nil
}

// List 复用现有115driver的文件字段定义，并校验返回的CID，拒绝静默回退根目录。
func (client *BehaviorHTTPClient) List(ctx context.Context, cookie, cid string, offset int) (BehaviorListing, error) {
	if !validBehaviorID(cid) {
		return BehaviorListing{}, fmt.Errorf("目录CID无效")
	}
	var result struct {
		driver.FileListResp
		Path []struct {
			ID     driver.IntString `json:"cid"`
			Parent driver.IntString `json:"pid"`
			Name   string           `json:"name"`
		} `json:"path"`
	}
	err := client.get(ctx, cookie, "/files", url.Values{"aid": {"1"}, "cid": {cid}, "offset": {strconv.Itoa(offset)}, "limit": {"1000"}, "show_dir": {"1"}, "o": {"file_id"}, "asc": {"1"}, "format": {"json"}, "record_open_time": {"0"}}, &result)
	if err != nil {
		return BehaviorListing{}, err
	}
	if string(result.CategoryID) != cid {
		return BehaviorListing{}, fmt.Errorf("115目录响应CID不匹配")
	}
	listing := BehaviorListing{Count: result.Count}
	for _, item := range result.Path {
		listing.Ancestors = append(listing.Ancestors, BehaviorNode{ID: string(item.ID), ParentID: string(item.Parent), Name: item.Name, IsDir: true})
	}
	for _, item := range result.Files {
		file := new(driver.File).From(&item)
		if !validBehaviorID(file.FileID) || file.ParentID != cid {
			return BehaviorListing{}, fmt.Errorf("115子项身份或父目录不匹配")
		}
		listing.Nodes = append(listing.Nodes, BehaviorNode{ID: file.FileID, ParentID: file.ParentID, Name: file.Name, SHA1: file.Sha1, Size: file.Size, IsDir: file.IsDirectory})
	}
	return listing, nil
}
