package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"easy-strm/internal/pkg/logger"
	driver "github.com/SheltonZhu/115driver/pkg/driver"

	"github.com/gin-gonic/gin"
)

// DirectLinkController 直链提取控制器
// 处理 /direct-link 路由，用于 STRM 播放，不需要认证
type DirectLinkController struct {
	recordPlayback func(string, string, int, string, string, string)
	// --- 回调依赖：main 包全局函数通过依赖注入解耦 ---

	// getCloud115ByID: 根据 ID 获取 115 账号（main.GetCloud115ByID）
	getCloud115ByID func(id int) (*Cloud115AccountBrief, error)
	// getAllCloud115: 获取所有 115 账号（main.GetAllCloud115）
	getAllCloud115 func(sortField, sortOrder string) ([]*Cloud115AccountBrief, error)
	// getPickCodeByPath: 根据路径获取 pickcode（main.Client.GetPickCodeByPath）
	getPickCodeByPath func(path string, cloud115ID int, cookie string) (string, error)
	// getFileDirectLink: 获取文件直链（main.Client.GetFileDirectLink）
	getFileDirectLink func(uid int, pickCode string, cloud115ID int, cookie string, userAgent string) (interface{}, error)
	// rapidTransferByMethod: 秒传（main.Client.RapidTransferByMethod）
	rapidTransferByMethod func(pickCode, decodedPath string, sourceID int, sourceCookie string, targetDirCID string, targetID int, targetCookie string, targetDir string, transferMethod string, alistUrl string, alistToken string) (string, error)
	// getCIDByPath: 根据路径获取 CID（main.Client.GetCIDByPath）
	getCIDByPath func(path string, cloud115ID int, cookie string) (string, error)
	// redisGet: 从 Redis 获取值
	redisGet func(key string) (string, error)
	// redisSet: 设置 Redis 键值对（带过期时间，单位秒）
	redisSet    func(key string, value string, expirationSec int) error
	getFileInfo func(pickCode string, cloud115ID int, cookie string) (*driver.File, error)
	// getDefaultUA: 获取默认 User-Agent
	getDefaultUA func() string
}

// SetRecordPlayback 注入成功直链调用的记录能力。
func (c *DirectLinkController) SetRecordPlayback(fn func(string, string, int, string, string, string)) {
	c.recordPlayback = fn
}

func NewDirectLinkController() *DirectLinkController {
	return &DirectLinkController{}
}

// --- 回调注入方法 ---

func (c *DirectLinkController) SetGetCloud115ByID(fn func(id int) (*Cloud115AccountBrief, error)) {
	c.getCloud115ByID = fn
}

func (c *DirectLinkController) SetGetAllCloud115(fn func(sortField, sortOrder string) ([]*Cloud115AccountBrief, error)) {
	c.getAllCloud115 = fn
}

func (c *DirectLinkController) SetGetPickCodeByPath(fn func(path string, cloud115ID int, cookie string) (string, error)) {
	c.getPickCodeByPath = fn
}

func (c *DirectLinkController) SetGetFileDirectLink(fn func(uid int, pickCode string, cloud115ID int, cookie string, userAgent string) (interface{}, error)) {
	c.getFileDirectLink = fn
}

func (c *DirectLinkController) SetRapidTransferByMethod(fn func(pickCode, decodedPath string, sourceID int, sourceCookie string, targetDirCID string, targetID int, targetCookie string, targetDir string, transferMethod string, alistUrl string, alistToken string) (string, error)) {
	c.rapidTransferByMethod = fn
}

func (c *DirectLinkController) SetGetCIDByPath(fn func(path string, cloud115ID int, cookie string) (string, error)) {
	c.getCIDByPath = fn
}

func (c *DirectLinkController) SetRedisGet(fn func(key string) (string, error)) {
	c.redisGet = fn
}

func (c *DirectLinkController) SetRedisSet(fn func(key string, value string, expirationSec int) error) {
	c.redisSet = fn
}

// SetGetFileInfo 设置获取115文件元数据的回调，用于成功转存后建立SHA1缓存。
func (c *DirectLinkController) SetGetFileInfo(fn func(pickCode string, cloud115ID int, cookie string) (*driver.File, error)) {
	c.getFileInfo = fn
}

func (c *DirectLinkController) SetGetDefaultUA(fn func() string) {
	c.getDefaultUA = fn
}

// GetDirectLink 根据路径获取文件直链（用于STRM播放，不需要认证）
// Route: GET /direct-link
// NOTE: 此处理器逻辑复杂，涉及秒传、转存等业务，
// 通过回调注入 main 包的 Client 方法和 Redis 操作
func (c *DirectLinkController) GetDirectLink(ctx *gin.Context) {
	defer beginDirectLinkRequest(ctx, "strm_playback")()
	logger.Debugf("DirectLinkController[GetDirectLink] 获取直链 from %s", ctx.ClientIP())

	// 获取查询参数
	pickcode := ctx.Query("pickcode")
	path := ctx.Query("path")
	cloud115IdStr := ctx.Query("cloud115_id")

	// 验证必要参数：pickcode 或 path 二选一
	if pickcode == "" && path == "" {
		logger.Warnf("DirectLinkController[GetDirectLink] 缺少必要参数 from %s", ctx.ClientIP())
		ctx.JSON(400, gin.H{"error": "pickcode or path is required"})
		return
	}

	// 获取指定的115云账号
	var cloud115 *Cloud115AccountBrief
	var err error
	cloud115Id := 0

	if cloud115IdStr != "" {
		fmt.Sscanf(cloud115IdStr, "%d", &cloud115Id)
		cloud115, err = c.getCloud115ByID(cloud115Id)
		if err != nil {
			logger.Errorf("DirectLinkController[GetDirectLink] 获取账号失败 ID %d: %v", cloud115Id, err)
			ctx.JSON(404, gin.H{"error": "Cloud115 account not found"})
			return
		}
	} else {
		cloud115List, err := c.getAllCloud115("", "")
		if err != nil {
			logger.Errorf("DirectLinkController[GetDirectLink] 获取账号列表失败: %v", err)
			ctx.JSON(500, gin.H{"error": "Failed to get cloud115 accounts"})
			return
		}
		if len(cloud115List) == 0 {
			ctx.JSON(404, gin.H{"error": "No cloud115 accounts found"})
			return
		}
		cloud115 = cloud115List[0]
		cloud115Id = cloud115.ID
	}

	// 解码路径
	decodedPath := path
	if path != "" {
		decodedPath, err = url.QueryUnescape(path)
		if err != nil {
			logger.Warnf("DirectLinkController[GetDirectLink] 路径解码失败: %v", err)
			decodedPath = path
		}
	}
	// 将Windows路径分隔符转换为正斜杠
	decodedPath = strings.ReplaceAll(decodedPath, "\\", "/")

	// 如果没有提供 pickcode，尝试从缓存或路径获取
	if pickcode == "" {
		pickcodeCacheKey := fmt.Sprintf("easy_strm:pickcode:%d:%s", cloud115Id, decodedPath)
		cachedPickcode, err := c.redisGet(pickcodeCacheKey)
		if err == nil && cachedPickcode != "" {
			pickcode = cachedPickcode
			logger.Debugf("DirectLinkController[GetDirectLink] 从缓存获取pickcode: %s", pickcode)
		} else {
			pickcode, err = c.getPickCodeByPath(decodedPath, cloud115.ID, cloud115.Cookie)
			if err != nil {
				logger.Errorf("DirectLinkController[GetDirectLink] 根据路径获取pickcode失败: %v", err)
				ctx.JSON(404, gin.H{"error": fmt.Sprintf("File not found: %v", err)})
				return
			}
			// 缓存 pickcode（24小时）
			c.redisSet(pickcodeCacheKey, pickcode, 86400)
		}
	}

	// 获取客户端 User-Agent
	defer func() {
		if c.recordPlayback != nil && ctx.Writer.Status() == http.StatusFound {
			c.recordPlayback(decodedPath, pickcode, cloud115Id, ctx.Writer.Header().Get("Location"), ctx.ClientIP(), ctx.Request.Method)
		}
	}()
	clientUA := ctx.GetHeader("User-Agent")
	if clientUA == "" && c.getDefaultUA != nil {
		clientUA = c.getDefaultUA()
	}

	// 检查是否配置了文件转存
	if cloud115.TransferAccountID > 0 {
		c.handleTransferAndRedirect(ctx, cloud115, pickcode, decodedPath, clientUA)
		return
	}

	// 没有配置转存，直接获取直链
	c.getDirectLinkAndRedirect(ctx, cloud115.ID, cloud115.Cookie, pickcode, clientUA)
}

// handleTransferAndRedirect 处理转存逻辑并重定向到直链
func (c *DirectLinkController) handleTransferAndRedirect(ctx *gin.Context, cloud115 *Cloud115AccountBrief, pickcode, decodedPath, clientUA string) {
	targetCloud115, err := c.getCloud115ByID(cloud115.TransferAccountID)
	if err != nil {
		logger.Errorf("DirectLinkController[handleTransfer] 获取转存目标账号失败 ID %d: %v", cloud115.TransferAccountID, err)
		ctx.JSON(500, gin.H{"error": fmt.Sprintf("Transfer target account not found: %v", err)})
		return
	}

	// 获取转存目录的CID
	targetDirCID := "0"
	if cloud115.TransferDirectory != "" {
		targetDirCID, err = c.getCIDByPath(cloud115.TransferDirectory, targetCloud115.ID, targetCloud115.Cookie)
		if err != nil {
			logger.Warnf("DirectLinkController[handleTransfer] 获取转存目录CID失败: %v", err)
			targetDirCID = "0"
		}
	}

	transferMethod := cloud115.TransferMethod
	if transferMethod != "" {
		// 尝试从缓存获取秒传后的新pickcode
		transferCacheKey := fmt.Sprintf("easy_strm:transfer_pickcode:%d:%s", cloud115.ID, decodedPath)
		var newPickCode string
		cachedPickcode, err := c.redisGet(transferCacheKey)
		if err == nil && cachedPickcode != "" {
			newPickCode = cachedPickcode
		} else {
			// 执行秒传
			newPickCode, err = c.rapidTransferByMethod(pickcode, decodedPath, cloud115.ID, cloud115.Cookie, targetDirCID, targetCloud115.ID, targetCloud115.Cookie, "", transferMethod, "", "")
			if err != nil {
				logger.Warnf("DirectLinkController[handleTransfer] 秒传失败: %v", err)
				// 从源账号获取直链
				c.getDirectLinkAndRedirect(ctx, cloud115.ID, cloud115.Cookie, pickcode, clientUA)
				return
			}
			// 缓存新的pickcode（30分钟）
			if newPickCode != "" {
				c.redisSet(transferCacheKey, newPickCode, 1800)
				if c.getFileInfo != nil && c.redisSet != nil {
					if sourceFile, infoErr := c.getFileInfo(pickcode, cloud115.ID, cloud115.Cookie); infoErr == nil && sourceFile != nil && strings.TrimSpace(sourceFile.Sha1) != "" {
						sha1Key := "easy_strm:sha1:cache:" + strings.ToLower(strings.TrimSpace(sourceFile.Sha1))
						if cacheErr := c.redisSet(sha1Key, newPickCode, 7*24*60*60); cacheErr != nil {
							logger.Warnf("DirectLinkController[handleTransfer] 写入SHA1缓存失败: %v", cacheErr)
						}
					}
				}
			}
		}

		if newPickCode == "" {
			// 从源账号获取直链
			c.getDirectLinkAndRedirect(ctx, cloud115.ID, cloud115.Cookie, pickcode, clientUA)
			return
		}

		// 使用目标账号获取直链
		c.getDirectLinkAndRedirect(ctx, targetCloud115.ID, targetCloud115.Cookie, newPickCode, clientUA)
		return
	}

	// 没有配置秒传方式，直接从源账号获取直链
	c.getDirectLinkAndRedirect(ctx, cloud115.ID, cloud115.Cookie, pickcode, clientUA)
}

// getDirectLinkAndRedirect 获取直链并重定向
// NOTE: 返回值类型为 interface{}，由回调函数负责返回具体的直链数据结构
// 回调函数应返回包含 Url.Url 字段的结构体，或直接返回 gin.H 格式
func (c *DirectLinkController) getDirectLinkAndRedirect(ctx *gin.Context, cloud115ID int, cookie, pickcode, clientUA string) {
	logger.Infof("[DirectLink] event=resolve request_id=%s source=strm_playback cloud115_id=%d pickcode=%q effective_ua=%q", logger.RequestID(ctx.Request.Context()), cloud115ID, pickcode, clientUA)
	result, err := c.getFileDirectLink(0, pickcode, cloud115ID, cookie, clientUA)
	if err != nil {
		logger.Errorf("DirectLinkController[getDirectLink] 获取直链失败: %v", err)
		ctx.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// result 由回调函数返回，可能是原始类型或 gin.H
	// 回调函数负责类型转换和URL提取
	if redirectURL, ok := result.(string); ok {
		// 如果回调返回的是 URL 字符串，直接重定向
		if redirectURL == "" || !strings.HasPrefix(redirectURL, "http") {
			logger.Errorf("DirectLinkController[getDirectLink] 无效的直链URL: %s", redirectURL)
			ctx.JSON(500, gin.H{"error": "Failed to get valid direct link"})
			return
		}
		ctx.Redirect(http.StatusFound, redirectURL)
		return
	}

	// 否则直接返回结果
	ctx.JSON(http.StatusOK, result)
}
