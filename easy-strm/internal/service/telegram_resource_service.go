package service

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
)

const (
	telegramResourceShare   = "share"
	telegramResourceOffline = "offline"
)

// TelegramResourceRequest 表示从机器人文本中解析出的115资源操作。
type TelegramResourceRequest struct {
	Kind        string
	URL         string
	AccountName string
	Directory   string
}

type telegramShareTransfer interface {
	ParseShareLink(ctx context.Context, url string, password string) (*domain.ParseShareResponse, error)
	SubmitTransfer(ctx context.Context, req domain.TransferRequest) (*domain.TransferResponse, error)
}

type telegramOfflineDownload interface {
	Submit(ctx context.Context, req domain.OfflineDownloadSubmitRequest) (*domain.OfflineDownloadSubmitResponse, error)
}

type telegramAccountStore interface {
	GetAll(sortField, sortOrder string) ([]*domain.Cloud115, error)
}

// TelegramResourceService 编排机器人发起的115分享转存和云下载。
type TelegramResourceService struct {
	shares   telegramShareTransfer
	offline  telegramOfflineDownload
	accounts telegramAccountStore
}

// NewTelegramResourceService 创建机器人115资源操作服务。
func NewTelegramResourceService(shares telegramShareTransfer, offline telegramOfflineDownload, accounts telegramAccountStore) *TelegramResourceService {
	return &TelegramResourceService{shares: shares, offline: offline, accounts: accounts}
}

// Execute 解析并提交机器人文本；handled=false 表示文本不是支持的资源链接。
func (s *TelegramResourceService) Execute(ctx context.Context, text string) (card NotificationCard, handled bool, err error) {
	lines := splitTelegramResourceLines(text)
	if len(lines) == 0 {
		return NotificationCard{}, false, nil
	}
	if len(lines) > 1 {
		return s.executeBatch(ctx, lines)
	}
	request, handled, err := parseTelegramResourceRequest(lines[0])
	if err != nil || !handled {
		return NotificationCard{}, handled, err
	}
	if s == nil || s.accounts == nil {
		return NotificationCard{}, true, fmt.Errorf("115资源操作服务未初始化")
	}
	accounts, err := s.accounts.GetAll("", "")
	if err != nil {
		return NotificationCard{}, true, fmt.Errorf("读取115账号失败: %v", err)
	}
	return s.executeRequest(ctx, request, accounts)
}

func (s *TelegramResourceService) executeRequest(ctx context.Context, request TelegramResourceRequest, accounts []*domain.Cloud115) (NotificationCard, bool, error) {
	account, err := selectTelegramResourceAccount(accounts, request.AccountName, request.Kind)
	if err != nil {
		return NotificationCard{}, true, err
	}

	switch request.Kind {
	case telegramResourceShare:
		return s.submitShare(ctx, request, account)
	case telegramResourceOffline:
		return s.submitOffline(ctx, request, account)
	default:
		return NotificationCard{}, false, nil
	}
}

func (s *TelegramResourceService) executeBatch(ctx context.Context, lines []string) (NotificationCard, bool, error) {
	type parsedLine struct {
		request TelegramResourceRequest
		handled bool
		err     error
	}
	parsed := make([]parsedLine, 0, len(lines))
	hasResource := false
	for _, line := range lines {
		request, handled, err := parseTelegramResourceRequest(line)
		parsed = append(parsed, parsedLine{request: request, handled: handled, err: err})
		if handled {
			hasResource = true
		}
	}
	if !hasResource {
		return NotificationCard{}, false, nil
	}
	if s == nil || s.accounts == nil {
		return NotificationCard{}, true, fmt.Errorf("115资源操作服务未初始化")
	}
	accounts, err := s.accounts.GetAll("", "")
	if err != nil {
		return NotificationCard{}, true, fmt.Errorf("读取115账号失败: %v", err)
	}

	successCount := 0
	allOffline := true
	allShare := true
	details := make([]string, 0, len(lines))
	for index, item := range parsed {
		label := fmt.Sprintf("第%d行", index+1)
		if item.handled && item.err == nil {
			label = telegramResourceRequestLabel(item.request, index+1)
			if item.request.Kind != telegramResourceOffline {
				allOffline = false
			}
			if item.request.Kind != telegramResourceShare {
				allShare = false
			}
		}
		if item.err != nil {
			details = append(details, fmt.Sprintf("%s：失败（%s）", label, item.err.Error()))
			continue
		}
		if !item.handled {
			details = append(details, fmt.Sprintf("%s：失败（不支持的资源链接）", label))
			continue
		}
		resultCard, _, executeErr := s.executeRequest(ctx, item.request, accounts)
		if executeErr != nil {
			details = append(details, fmt.Sprintf("%s：失败（%s）", label, executeErr.Error()))
			continue
		}
		successCount++
		taskID := notificationCardField(resultCard, "任务 ID")
		if taskID == "" {
			details = append(details, fmt.Sprintf("%s：已创建", label))
		} else {
			details = append(details, fmt.Sprintf("%s：已创建，任务 ID %s", label, taskID))
		}
	}

	failureCount := len(lines) - successCount
	title := "115 资源批量提交完成"
	if allOffline {
		title = "115 云下载批量提交完成"
	} else if allShare {
		title = "115 分享转存批量提交完成"
	}
	status := "☁️"
	if failureCount > 0 && successCount > 0 {
		title = strings.TrimSuffix(title, "完成") + "部分完成"
		status = "⚠️"
	} else if successCount == 0 {
		title = strings.TrimSuffix(title, "完成") + "失败"
		status = "❌"
	}
	card := NotificationCard{
		Title:  title,
		Status: status,
		Fields: [][2]string{
			{"总数", strconv.Itoa(len(lines))},
			{"已创建", strconv.Itoa(successCount)},
			{"失败", strconv.Itoa(failureCount)},
		},
		Detail: strings.Join(details, "\n"),
	}
	if successCount > 0 {
		card.Actions = [][]NotificationAction{{{Text: "查看最近任务", Data: "tasks"}}}
	}
	return card, true, nil
}

func splitTelegramResourceLines(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	rawLines := strings.Split(normalized, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func telegramResourceRequestLabel(request TelegramResourceRequest, lineNumber int) string {
	if request.Kind == telegramResourceOffline {
		if parsed, err := url.Parse(request.URL); err == nil && strings.EqualFold(parsed.Scheme, "magnet") {
			if name := strings.TrimSpace(parsed.Query().Get("dn")); name != "" {
				return fmt.Sprintf("第%d行 %s", lineNumber, truncateRunes(name, 36))
			}
			if hash := strings.TrimPrefix(parsed.Query().Get("xt"), "urn:btih:"); hash != "" {
				return fmt.Sprintf("第%d行 %s", lineNumber, truncateRunes(hash, 16))
			}
		}
	}
	return fmt.Sprintf("第%d行 %s", lineNumber, truncateRunes(request.URL, 36))
}

func notificationCardField(card NotificationCard, name string) string {
	for _, field := range card.Fields {
		if field[0] == name {
			return strings.TrimSpace(field[1])
		}
	}
	return ""
}

func (s *TelegramResourceService) submitShare(ctx context.Context, request TelegramResourceRequest, account *domain.Cloud115) (NotificationCard, bool, error) {
	if s.shares == nil {
		return NotificationCard{}, true, fmt.Errorf("115分享转存服务未初始化")
	}
	parsed, err := s.shares.ParseShareLink(ctx, request.URL, "")
	if err != nil {
		return NotificationCard{}, true, err
	}
	files := make([]domain.ShareTransferFileItem, 0, len(parsed.Files))
	for _, file := range parsed.Files {
		if strings.TrimSpace(file.Fid) == "" {
			continue
		}
		files = append(files, domain.ShareTransferFileItem{Fid: file.Fid, PickCode: file.PickCode, Name: file.Name, Size: file.Size})
	}
	if len(files) == 0 {
		return NotificationCard{}, true, fmt.Errorf("分享中没有可转存的文件或目录")
	}
	directory := request.Directory
	if directory == "" {
		directory = account.TransferDirectory
	}
	directory, err = normalizeCloud115DirectoryPath(directory, "目标目录", true)
	if err != nil {
		return NotificationCard{}, true, err
	}
	result, err := s.shares.SubmitTransfer(ctx, domain.TransferRequest{
		ShareCode:        parsed.ShareCode,
		Password:         extractSharePassword(request.URL),
		TargetCloud115Id: account.ID,
		TargetDirectory:  directory,
		Files:            files,
		ConflictStrategy: "skip",
	})
	if err != nil {
		return NotificationCard{}, true, err
	}
	return NotificationCard{
		Title:  "115 分享转存已提交",
		Status: "📥",
		Fields: [][2]string{
			{"账号", account.Name}, {"目录", displayTelegramDirectory(directory, "/")},
			{"项目数", strconv.Itoa(result.TotalFiles)}, {"任务 ID", result.TaskId},
		},
		Detail:  "转存完成后会发送任务成功通知。",
		Actions: [][]NotificationAction{{{Text: "查看任务", Data: "task:detail:" + result.TaskId}}},
	}, true, nil
}

func (s *TelegramResourceService) submitOffline(ctx context.Context, request TelegramResourceRequest, account *domain.Cloud115) (NotificationCard, bool, error) {
	if s.offline == nil {
		return NotificationCard{}, true, fmt.Errorf("115云下载服务未初始化")
	}
	directory, err := normalizeCloud115DirectoryPath(request.Directory, "目标目录", true)
	if err != nil {
		return NotificationCard{}, true, err
	}
	result, err := s.offline.Submit(ctx, domain.OfflineDownloadSubmitRequest{
		Cloud115ID: account.ID,
		Directory:  directory,
		Urls:       []string{request.URL},
	})
	if err != nil {
		return NotificationCard{}, true, err
	}
	return NotificationCard{
		Title:  "115 云下载已提交",
		Status: "☁️",
		Fields: [][2]string{
			{"账号", account.Name}, {"目录", displayTelegramDirectory(directory, offlineDefaultDir)},
			{"链接数", strconv.Itoa(result.Total)}, {"任务 ID", result.TaskId},
		},
		Detail:  "云下载完成后会发送任务成功通知。",
		Actions: [][]NotificationAction{{{Text: "查看任务", Data: "task:detail:" + result.TaskId}}},
	}, true, nil
}

func parseTelegramResourceRequest(text string) (TelegramResourceRequest, bool, error) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return TelegramResourceRequest{}, false, nil
	}
	rawURL := strings.Trim(fields[0], "<>")
	kind := ""
	if shareCodeRe.MatchString(rawURL) {
		kind = telegramResourceShare
	} else if isSupportedOfflineUrl(rawURL) {
		kind = telegramResourceOffline
	} else {
		return TelegramResourceRequest{}, false, nil
	}

	request := TelegramResourceRequest{Kind: kind, URL: rawURL}
	rest := fields[1:]
	directoryIndex := -1
	for index, field := range rest {
		if strings.HasPrefix(field, "/") {
			directoryIndex = index
			break
		}
	}
	if directoryIndex < 0 {
		request.AccountName = strings.TrimSpace(strings.Join(rest, " "))
		return request, true, nil
	}
	request.AccountName = strings.TrimSpace(strings.Join(rest[:directoryIndex], " "))
	request.Directory = strings.TrimSpace(strings.Join(rest[directoryIndex:], " "))
	normalized, err := normalizeCloud115DirectoryPath(request.Directory, "目标目录", false)
	if err != nil {
		return TelegramResourceRequest{}, true, err
	}
	request.Directory = normalized
	return request, true, nil
}

func selectTelegramResourceAccount(accounts []*domain.Cloud115, specifiedName, kind string) (*domain.Cloud115, error) {
	available := make([]*domain.Cloud115, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.Status != domain.AccountStatusActive || strings.TrimSpace(account.Cookie) == "" {
			continue
		}
		if specifiedName != "" {
			if account.Name == specifiedName {
				available = append(available, account)
			}
			continue
		}
		if kind == telegramResourceShare && account.AccountType != domain.AccountTypeResource {
			continue
		}
		available = append(available, account)
	}
	if len(available) == 0 {
		if specifiedName != "" {
			return nil, fmt.Errorf("指定的115账号不存在或当前不可用: %s", specifiedName)
		}
		if kind == telegramResourceShare {
			return nil, fmt.Errorf("没有可用的115资源号")
		}
		return nil, fmt.Errorf("没有可用的115云下载账号")
	}
	sort.SliceStable(available, func(i, j int) bool {
		if kind == telegramResourceOffline && available[i].AccountType != available[j].AccountType {
			return telegramOfflineAccountTypeRank(available[i].AccountType) < telegramOfflineAccountTypeRank(available[j].AccountType)
		}
		if available[i].Priority != available[j].Priority {
			return available[i].Priority > available[j].Priority
		}
		return available[i].ID < available[j].ID
	})
	return available[0], nil
}

func telegramOfflineAccountTypeRank(accountType string) int {
	switch accountType {
	case domain.AccountTypeVIP:
		return 0
	case domain.AccountTypeBoth:
		return 1
	case domain.AccountTypeResource:
		return 2
	default:
		return 3
	}
}

func displayTelegramDirectory(directory, fallback string) string {
	if strings.TrimSpace(directory) == "" {
		return fallback
	}
	return directory
}
