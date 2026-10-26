package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/google/uuid"
)

const (
	shareDirectLinkCapacity     = 256
	shareDirectLinkFlights      = 64
	shareDirectLinkWaiters      = 256
	shareDirectLinkLifetime     = 2 * time.Minute
	shareDirectLinkQueueTimeout = 10 * time.Second
	shareDirectLinkCallTimeout  = 20 * time.Second
)

var errShareDirectLinkBusy = errors.New("播放解析繁忙，请稍后重试")
var errShareDirectLinkChanged = errors.New("播放映射或转存配置已变化，请重试")
var errShareDirectLinkURL = errors.New("115直链无效、有效期未知或距过期不足一分钟")

type shareDirectLinkEntry struct {
	result shareDirectLinkResult
	until  time.Time
	timer  *time.Timer
}

type shareDirectLinkFlight struct {
	done      chan struct{}
	cancel    context.CancelFunc
	waiters   int
	result    shareDirectLinkResult
	err       error
	abandoned bool
}

type shareDirectLinkResult struct {
	link     string
	identity [32]byte
}

type shareDirectLinks struct {
	mu      sync.Mutex
	entries map[[32]byte]*shareDirectLinkEntry
	flights map[[32]byte]*shareDirectLinkFlight
	slots   chan struct{}
	logSlot chan struct{}
	now     func() time.Time
}

func newShareDirectLinks() *shareDirectLinks {
	return &shareDirectLinks{entries: make(map[[32]byte]*shareDirectLinkEntry), flights: make(map[[32]byte]*shareDirectLinkFlight), slots: make(chan struct{}, 2), logSlot: make(chan struct{}, 1), now: time.Now}
}

// 115 Android 下载响应仅提供 URL；t 是 CDN 的 Unix 秒过期时间，不猜测其他字段或单位。
func shareDirectLinkDeadline(link string, now time.Time) (time.Time, error) {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.ContainsAny(link, "\r\n\x00") {
		return time.Time{}, errShareDirectLinkURL
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	values := query["t"]
	if err != nil || len(values) != 1 || len(values[0]) != 10 {
		return time.Time{}, errShareDirectLinkURL
	}
	for _, digit := range values[0] {
		if digit < '0' || digit > '9' {
			return time.Time{}, errShareDirectLinkURL
		}
	}
	seconds, err := strconv.ParseInt(values[0], 10, 64)
	until := time.Unix(seconds, 0).Add(-time.Minute)
	if err != nil || !now.Before(until) {
		return time.Time{}, errShareDirectLinkURL
	}
	if maximum := now.Add(time.Minute); maximum.Before(until) {
		until = maximum
	}
	return until, nil
}

func (cache *shareDirectLinks) remove(key [32]byte) {
	if entry := cache.entries[key]; entry != nil {
		entry.timer.Stop()
		delete(cache.entries, key)
	}
}

func (cache *shareDirectLinks) save(key [32]byte, result shareDirectLinkResult, until time.Time) {
	if len(cache.entries) >= shareDirectLinkCapacity {
		var oldest [32]byte
		var earliest time.Time
		for candidate, entry := range cache.entries {
			if earliest.IsZero() || entry.until.Before(earliest) {
				oldest, earliest = candidate, entry.until
			}
		}
		cache.remove(oldest)
	}
	cache.remove(key)
	entry := &shareDirectLinkEntry{result: result, until: until}
	entry.timer = time.AfterFunc(until.Sub(cache.now()), func() {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		if cache.entries[key] == entry {
			delete(cache.entries, key)
		}
	})
	cache.entries[key] = entry
}

func (cache *shareDirectLinks) resolve(ctx context.Context, key, identity [32]byte, resolve func(context.Context) (shareDirectLinkResult, error)) (shareDirectLinkResult, error) {
	ctx, cancelWait := context.WithTimeout(ctx, shareDirectLinkLifetime)
	defer cancelWait()
	if err := ctx.Err(); err != nil {
		return shareDirectLinkResult{}, err
	}
	cache.mu.Lock()
	for candidate, entry := range cache.entries {
		if !cache.now().Before(entry.until) {
			cache.remove(candidate)
		}
	}
	if entry := cache.entries[key]; entry != nil {
		if entry.result.identity == identity {
			result := entry.result
			cache.mu.Unlock()
			_, err := shareDirectLinkDeadline(result.link, cache.now())
			if err != nil {
				return shareDirectLinkResult{}, err
			}
			return result, nil
		}
		cache.remove(key)
	}
	flight := cache.flights[key]
	if flight == nil {
		if len(cache.flights) >= shareDirectLinkFlights {
			cache.mu.Unlock()
			return shareDirectLinkResult{}, errShareDirectLinkBusy
		}
		work, cancel := context.WithTimeout(context.WithoutCancel(ctx), shareDirectLinkLifetime)
		flight = &shareDirectLinkFlight{done: make(chan struct{}), cancel: cancel}
		cache.flights[key] = flight
		go cache.run(work, key, flight, resolve)
	}
	if flight.waiters >= shareDirectLinkWaiters || flight.abandoned {
		cache.mu.Unlock()
		return shareDirectLinkResult{}, errShareDirectLinkBusy
	}
	flight.waiters++
	cache.mu.Unlock()
	select {
	case <-ctx.Done():
		cache.mu.Lock()
		flight.waiters--
		if flight.waiters == 0 {
			flight.abandoned = true
			flight.cancel()
		}
		cache.mu.Unlock()
		return shareDirectLinkResult{}, ctx.Err()
	case <-flight.done:
		if err := ctx.Err(); err != nil {
			return shareDirectLinkResult{}, err
		}
		if flight.err != nil {
			return shareDirectLinkResult{}, flight.err
		}
		if _, err := shareDirectLinkDeadline(flight.result.link, cache.now()); err != nil {
			return shareDirectLinkResult{}, err
		}
		return flight.result, nil
	}
}

func (cache *shareDirectLinks) run(ctx context.Context, key [32]byte, flight *shareDirectLinkFlight, resolve func(context.Context) (shareDirectLinkResult, error)) {
	var result shareDirectLinkResult
	var err error
	defer func() {
		if recover() != nil {
			result, err = shareDirectLinkResult{}, errors.New("播放解析异常，请稍后重试")
		}
		func() {
			cache.mu.Lock()
			defer cache.mu.Unlock()
			defer flight.cancel()
			if err == nil {
				err = ctx.Err()
			}
			if err == nil {
				var until time.Time
				until, err = shareDirectLinkDeadline(result.link, cache.now())
				if err == nil {
					cache.save(key, result, until)
				}
			}
			if err != nil {
				result = shareDirectLinkResult{}
			}
			flight.result, flight.err = result, err
			delete(cache.flights, key)
			close(flight.done)
		}()
		if err != nil {
			// 先交付再记录；慢日志最多占用一个工作协程，其余诊断丢弃。
			select {
			case cache.logSlot <- struct{}{}:
				defer func() { <-cache.logSlot }()
				logShareDirectLinkFailure(ctx, err)
			default:
			}
		}
	}()
	result, err = resolve(ctx)
}

func (cache *shareDirectLinks) apply(ctx context.Context, apply func(context.Context) (string, error)) (string, error) {
	queue, cancelQueue := context.WithTimeout(ctx, shareDirectLinkQueueTimeout)
	defer cancelQueue()
	select {
	case cache.slots <- struct{}{}:
	case <-queue.Done():
		return "", queue.Err()
	}
	defer func() { <-cache.slots }()
	if err := queue.Err(); err != nil {
		return "", err
	}
	call, cancelCall := context.WithTimeout(ctx, shareDirectLinkCallTimeout)
	defer cancelCall()
	// 同步等待 SDK 真正退出后才释放槽位，不以等待者的超时冒充请求完成。
	link, err := apply(call)
	if callErr := call.Err(); callErr != nil {
		if err == nil {
			err = callErr
		}
		return "", err
	}
	return link, err
}

type sharePlaybackSnapshot struct {
	id       string
	revision string
	config   domain.ShareStrmSettings
	entry    domain.ShareStrmEntry
	account  domain.Cloud115
}

type sharePlaybackSettings struct {
	ShareTaskSettingsStore
	ctx context.Context
}

func (settings sharePlaybackSettings) GetByKey(key string) (*domain.SystemConfig, error) {
	if reader, ok := settings.ShareTaskSettingsStore.(interface {
		GetByKeyContext(context.Context, string) (*domain.SystemConfig, error)
	}); ok {
		return reader.GetByKeyContext(settings.ctx, key)
	}
	return settings.ShareTaskSettingsStore.GetByKey(key)
}

// 按路径合并首次 FileID 回填，完整身份仍保存在结果中，外部修改 FileID 不复用旧链接。
func (snapshot sharePlaybackSnapshot) key(ua string) [32]byte {
	snapshot.revision = ""
	if snapshot.entry.FilePath != "" {
		snapshot.entry.FileID = ""
	}
	return snapshot.identity(ua)
}

func (snapshot sharePlaybackSnapshot) identity(ua string) [32]byte {
	class := "other"
	if strings.HasPrefix(ua, "Lavf/") {
		class = "lavf"
	} else if strings.HasPrefix(ua, "Mozilla/") {
		class = "browser"
	}
	identity := []any{snapshot.id, snapshot.revision, snapshot.config, snapshot.entry, snapshot.account.ID, snapshot.account.Cookie, snapshot.account.TransferDirectory, snapshot.account.TransferAccountID, snapshot.account.Status, snapshot.account.UpdateTime, class, sha256.Sum256([]byte(ua))}
	raw, _ := json.Marshal(identity)
	return sha256.Sum256(raw)
}

func (service *ShareStrmService) playbackSnapshot(ctx context.Context, id string) (sharePlaybackSnapshot, error) {
	snapshot := sharePlaybackSnapshot{id: id}
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	var err error
	snapshot.config, err = service.settingsContext(ctx)
	if err != nil {
		return snapshot, err
	}
	if err = validateShareStrmSettings(&snapshot.config); err != nil {
		return snapshot, err
	}
	snapshot.entry, snapshot.revision, err = service.playbackEntry(ctx, id)
	if err != nil {
		return snapshot, err
	}
	var account *domain.Cloud115
	if service.playbackAccount != nil {
		account, err = service.playbackAccount(ctx, snapshot.config.Cloud115ID)
	} else {
		account, err = service.account(snapshot.config.Cloud115ID)
	}
	if err != nil {
		return snapshot, err
	}
	if account == nil {
		return snapshot, errors.New("115转存账号不存在")
	}
	snapshot.account = *account
	snapshot.config.TransferPath, err = ResolveShareTransferDirectory(sharePlaybackSettings{ShareTaskSettingsStore: service.settings, ctx: ctx}, snapshot.config.TransferPath, account.TransferDirectory)
	return snapshot, err
}

func (service *ShareStrmService) playbackEntry(ctx context.Context, id string) (domain.ShareStrmEntry, string, error) {
	if reader, ok := service.store.(interface {
		GetStrmPlaybackEntry(context.Context, string) (domain.ShareStrmEntry, string, error)
	}); ok {
		return reader.GetStrmPlaybackEntry(ctx, id)
	}
	entry, err := service.store.GetStrmEntry(ctx, id)
	return entry, "", err
}

// SetPlaybackDirectLink 注入携带真实上游上下文的取链能力，仅用于分享播放。
func (service *ShareStrmService) SetPlaybackDirectLink(resolve func(context.Context, string, int, string, string) (string, error)) {
	service.playbackDirectLink = resolve
}

// SetPlaybackClient 注入带任务上下文的115适配器，使建目录、转存及分页同样可取消。
func (service *ShareStrmService) SetPlaybackClient(factory func(context.Context) Cloud115Client) {
	service.playbackClient = factory
}

// SetPlaybackAccountReader 注入可取消的账号读取，避免分离任务超出数据库等待预算。
func (service *ShareStrmService) SetPlaybackAccountReader(reader func(context.Context, int) (*domain.Cloud115, error)) {
	service.playbackAccount = reader
}

func (service *ShareStrmService) directLinks() *shareDirectLinks {
	service.directLinksOnce.Do(func() { service.playbackLinks = newShareDirectLinks() })
	return service.playbackLinks
}

// Playback 在映射串行锁之前合并取链；命中缓存仍检查映射、配置与账号，不访问115目录。
func (service *ShareStrmService) Playback(ctx context.Context, id, ua string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, shareDirectLinkLifetime)
	defer cancel()
	if strings.TrimSpace(ua) == "" {
		ua = ""
	} else {
		ua = strings.Trim(ua, " \t")
	}
	if parsed, err := uuid.Parse(id); err == nil {
		id = parsed.String()
	}
	snapshot, err := service.playbackSnapshot(ctx, id)
	if err != nil {
		return "", err
	}
	key := snapshot.key(ua)
	result, err := service.directLinks().resolve(ctx, key, snapshot.identity(ua), func(work context.Context) (shareDirectLinkResult, error) {
		var resolved sharePlaybackSnapshot
		link, resolveErr := service.resolvePlayback(work, id, ua, key, &resolved)
		return shareDirectLinkResult{link: link, identity: resolved.identity(ua)}, resolveErr
	})
	if err != nil {
		return "", err
	}
	release, err := service.Coordinator().acquire(ctx, nil, shareResource{key: "mapping-code:" + snapshot.entry.ShareCode}, shareResource{key: "mapping:" + id})
	if err != nil {
		return "", err
	}
	defer release()
	current, err := service.playbackSnapshot(ctx, id)
	if err != nil {
		return "", err
	}
	if current.key(ua) != key || current.identity(ua) != result.identity {
		return "", errShareDirectLinkChanged
	}
	if _, err = shareDirectLinkDeadline(result.link, service.directLinks().now()); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return result.link, nil
}

func shareDirectLinkFailureFields(err error) logger.Fields {
	info := DescribeSharePlaybackError(err)
	fields := logger.Fields{"event": "share_strm_directlink_singleflight_v1", "source": "share_strm_playback", "upstream_errno": "unknown", "upstream_http_status": "unknown", "risk_control": "unknown", "timeout": false, "canceled": errors.Is(err, context.Canceled), "error_type": "resolution_failed", "detail": logger.SanitizeText(info.Category)}
	if info.Errno != "" {
		fields["upstream_errno"] = info.Errno
	}
	var upstream *domain.DirectLinkError
	if errors.As(err, &upstream) {
		fields["error_type"] = "upstream_error"
		fields["detail"] = logger.SanitizeText(info.Category + "；" + upstream.Error())
		if upstream.HTTPStatus != 0 {
			fields["upstream_http_status"] = upstream.HTTPStatus
		}
	}
	fields["cause_type"] = shareDirectLinkCauseType(err)
	if isSharePlaybackTimeout(err) {
		fields["timeout"], fields["error_type"] = true, "timeout"
	} else if errors.Is(err, context.Canceled) {
		fields["error_type"] = "canceled"
	} else if errors.Is(err, errShareDirectLinkURL) {
		fields["error_type"] = "invalid_url"
	} else if errors.Is(err, errShareDirectLinkBusy) {
		fields["error_type"] = "busy"
	}
	if errors.Is(err, driver.ErrLoginTwoStepVerify) || info.Errno == "40101010" {
		fields["risk_control"] = "verification_required"
	}
	return fields
}

func shareDirectLinkCauseType(err error) string {
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case isSharePlaybackTimeout(err):
		return "timeout"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected_eof"
	case errors.As(err, &network):
		return "network_error"
	case errors.Is(err, driver.ErrPasswordIncorrect):
		return "password_incorrect"
	case errors.Is(err, driver.ErrLoginTwoStepVerify):
		return "verification_required"
	default:
		return "unknown"
	}
}

func logShareDirectLinkFailure(ctx context.Context, err error) {
	// SDK 错误链可能携带完整响应体；仅记录已归类字段，不记录原始链或 URL。
	logger.WithContext(ctx, "directlink").Log(logger.WARN, "分享直链解析失败", shareDirectLinkFailureFields(err), nil)
}

func shareTransferPickCodeKey(snapshot sharePlaybackSnapshot) string {
	return fmt.Sprintf("%sv2:%x", transferPickCodeKeyPrefix, snapshot.identity(""))
}
