package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

// JWTClaims 定义JWT声明
type JWTClaims struct {
	UserID int `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT token
func GenerateToken(userID int, secret string) (string, error) {
	// 创建声明
	claims := JWTClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24小时过期
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	// 创建token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 签名token
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// VerifyToken 验证JWT token
func VerifyToken(tokenString string, secret string) (*JWTClaims, error) {
	// 解析token
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	// 验证token有效性
	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// JWTMiddleware JWT验证中间件
func JWTMiddleware(config *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头获取token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			JSON(c, http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		// 移除Bearer前缀（如果存在）
		tokenString := authHeader
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		}

		// 验证token
		claims, err := VerifyToken(tokenString, config.JWTSecret)
		if err != nil {
			JSON(c, http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// 验证token是否存在于Redis
		tokenInRedis, err := GetToken(claims.UserID)
		if err != nil {
			Error("Failed to get token from Redis: %v", err)
			JSON(c, http.StatusUnauthorized, gin.H{"error": "Failed to validate token"})
			c.Abort()
			return
		}

		// 验证token是否匹配
		if tokenInRedis != tokenString {
			JSON(c, http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// 将用户ID存储到上下文
		c.Set("userID", claims.UserID)
		c.Next()
	}
}

// SetupAuthRoutes 设置认证相关路由
func SetupAuthRoutes(r *gin.Engine, config *Config, client *Client) {
	// 登录接口
	r.POST("/login", func(c *gin.Context) {
		var loginData struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}

		// 绑定请求体
		if err := c.ShouldBindJSON(&loginData); err != nil {
			Warn("Invalid login request body from %s: %v", c.ClientIP(), err)
			JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		// 验证用户名和密码
		user, err := GetUserByName(loginData.Name)
		if err != nil {
			Warn("User %s not found from %s", loginData.Name, c.ClientIP())
			JSON(c, http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
			return
		}
		Info("User %s found from %s", loginData.Name, c.ClientIP())
		// 验证密码
		err = VerifyPassword(user.Password, loginData.Password)
		if err != nil {
			Warn("Invalid password for user %s from %s", loginData.Name, c.ClientIP())
			JSON(c, http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
			return
		}

		// 生成JWT token
		token, err := GenerateToken(user.ID, config.JWTSecret)
		if err != nil {
			Error("Failed to generate token for user %d: %v", user.ID, err)
			JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		// 将token存储到Redis
		err = SetToken(user.ID, token)
		if err != nil {
			Error("Failed to store token for user %d: %v", user.ID, err)
			JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to store token"})
			return
		}

		Info("User %s logged in successfully from %s", loginData.Name, c.ClientIP())
		JSON(c, http.StatusOK, gin.H{
			"message": "Login successful",
			"token":   token,
			"user_id": user.ID,
			"name":    user.Name,
		})
	})

	// 根据路径获取文件直链（用于STRM播放，不需要认证）
	r.GET("/direct-link", func(c *gin.Context) {
		Debug("Get direct link API called from %s", c.ClientIP())

		// 获取查询参数
		pickcode := c.Query("pickcode")
		path := c.Query("path")
		cloud115IdStr := c.Query("cloud115_id")

		// 验证必要参数：pickcode 或 path 二选一
		if pickcode == "" && path == "" {
			Warn("Missing required parameter pickcode or path from %s", c.ClientIP())
			JSON(c, 400, gin.H{
				"error": "pickcode or path is required",
			})
			return
		}

		// 获取指定的115云账号
		var cloud115 *Cloud115
		var err error
		cloud115Id := 0

		if cloud115IdStr != "" {
			fmt.Sscanf(cloud115IdStr, "%d", &cloud115Id)
			cloud115, err = GetCloud115ByID(cloud115Id)
			if err != nil {
				Error("Failed to get cloud115 account by ID %d: %v", cloud115Id, err)
				JSON(c, 404, gin.H{
					"error": "Cloud115 account not found",
				})
				return
			}
		} else {
			cloud115List, err := GetAllCloud115()
			if err != nil {
				Error("Failed to get cloud115 accounts: %v", err)
				JSON(c, 500, gin.H{
					"error": "Failed to get cloud115 accounts",
				})
				return
			}

			if len(cloud115List) == 0 {
				Error("No cloud115 accounts found")
				JSON(c, 404, gin.H{
					"error": "No cloud115 accounts found",
				})
				return
			}

			cloud115 = cloud115List[0]
			cloud115Id = cloud115.ID
		}

		Debug("Using cloud115 account: %s (ID: %d)", cloud115.Name, cloud115Id)
		Debug("Cloud115 account details - ID: %d, Name: %s, Cookie length: %d, Cookie preview: %s..., AccessToken: %s..., RefreshToken: %s...",
			cloud115.ID,
			cloud115.Name,
			len(cloud115.Cookie),
			truncateString(cloud115.Cookie, 50),
			truncateString(cloud115.AccessToken, 20),
			truncateString(cloud115.RefreshToken, 20))

		// 解码路径
		decodedPath := path
		if path != "" {
			decodedPath, err = url.QueryUnescape(path)
			if err != nil {
				Warn("Failed to decode path: %v", err)
				decodedPath = path
			}
		}

		// 将Windows路径分隔符转换为正斜杠，确保与115网盘API兼容
		decodedPath = strings.ReplaceAll(decodedPath, "\\", "/")

		// 如果没有提供 pickcode，则尝试从缓存或路径获取 pickcode
		if pickcode == "" {
			// 尝试从 Redis 缓存获取 pickcode
			pickcodeCacheKey := fmt.Sprintf("pickcode:%d:%s", cloud115Id, decodedPath)
			cachedPickcode, err := redisClient.Get(ctx, pickcodeCacheKey).Result()
			if err == nil && cachedPickcode != "" {
				pickcode = cachedPickcode
				Debug("Got pickcode from cache: %s for path: %s", pickcode, decodedPath)
			} else {
				// 缓存中没有，调用 API 获取 pickcode
				Debug("Get pickcode from API for path: %s", decodedPath)

				pickcode, err = client.GetPickCodeByPath(decodedPath, cloud115.ID, cloud115.Cookie)
				if err != nil {
					Error("Failed to get pickcode by path: %v", err)
					JSON(c, 404, gin.H{
						"error": fmt.Sprintf("File not found: %v", err),
					})
					return
				}

				// 缓存 pickcode（缓存24小时）
				redisClient.Set(ctx, pickcodeCacheKey, pickcode, 24*time.Hour)
				Debug("Cached pickcode: %s for path: %s", pickcode, decodedPath)
			}
		}

		// 获取客户端 User-Agent。115 CDN 签名 (k=) 必须与访问时的 UA 匹配。
		clientUA := c.GetHeader("User-Agent")
		if clientUA == "" {
			clientUA = driver.UA115Disk // 兜底
		}

		// 检查是否配置了文件转存
		var targetCloud115 *Cloud115

		if cloud115.TransferAccountID > 0 {
			// 获取转存目标账号
			targetCloud115, err = GetCloud115ByID(cloud115.TransferAccountID)
			if err != nil {
				Error("Failed to get transfer target cloud115 account by ID %d: %v", cloud115.TransferAccountID, err)
				JSON(c, 500, gin.H{
					"error": fmt.Sprintf("Transfer target account not found: %v", err),
				})
				return
			}

			Info("Transfer configured: using target account %s (ID: %d) to get direct link", targetCloud115.Name, targetCloud115.ID)

			// 获取转存目录的CID
			var targetDirCID string = "0" // 默认为根目录
			if cloud115.TransferDirectory != "" {
				targetDirCID, err = client.GetCIDByPath(cloud115.TransferDirectory, targetCloud115.ID, targetCloud115.Cookie)
				if err != nil {
					Warn("Failed to get transfer directory CID, using root directory: %v", err)
					targetDirCID = "0"
				} else {
					Info("Transfer directory CID: %s for path: %s", targetDirCID, cloud115.TransferDirectory)
				}
			}

			// 尝试秒传文件到目标账号
			Debug("Attempting rapid transfer file %s to target account %d", pickcode, targetCloud115.ID)
			err = client.RapidTransferFile(pickcode, cloud115.ID, cloud115.Cookie, targetDirCID, targetCloud115.ID, targetCloud115.Cookie, "")
			if err != nil {
				Warn("Rapid transfer failed: %v, trying to get direct link from target account anyway", err)
			} else {
				Info("Rapid transfer successful, file is now available in target account")
			}

			// 使用目标账号获取直链
			directLink, err := client.GetFileDirectLink(0, pickcode, targetCloud115.ID, targetCloud115.Cookie, clientUA)
			if err != nil {
				Warn("File not found in target account %s, using source account %s instead", targetCloud115.Name, cloud115.Name)
				// 如果目标账号获取失败，回退到使用源账号
				directLink, err = client.GetFileDirectLink(0, pickcode, cloud115.ID, cloud115.Cookie, clientUA)
				if err != nil {
					Error("Failed to get direct link from source account: %v", err)
					JSON(c, 500, gin.H{"error": err.Error()})
					return
				}
			}

			directLinkURL := directLink.Url.Url
			Info("Got direct link for file: %s, URL: %s", directLink.FileName, directLinkURL)

			// 重定向到直链
			c.Redirect(http.StatusFound, directLinkURL)
			Info("Redirected to direct link: %s", directLinkURL)
		} else {
			// 没有配置转存，直接获取直链
			directLink, err := client.GetFileDirectLink(0, pickcode, cloud115.ID, cloud115.Cookie, clientUA)
			if err != nil {
				Error("Failed to get direct link: %v", err)
				JSON(c, 500, gin.H{"error": err.Error()})
				return
			}

			directLinkURL := directLink.Url.Url
			Info("Got direct link for file: %s, URL: %s", directLink.FileName, directLinkURL)

			// 重定向到直链 (使用 Android API 获取的链接通常更稳定，尝试让客户端直接访问)
			c.Redirect(http.StatusFound, directLinkURL)
			Info("Redirected to Android direct link: %s", directLinkURL)
		}
	})
}

// SetupAuthProtectedRoutes 设置需要认证的路由组
func SetupAuthProtectedRoutes(r *gin.Engine, config *Config, client *Client) {
	// 需要验证token的路由组
	auth := r.Group("/")
	auth.Use(JWTMiddleware(config))
	{
		// 用户信息接口
		auth.GET("/user/info", func(c *gin.Context) {
			// 从上下文获取用户ID
			userID, exists := c.Get("userID")
			if !exists {
				Error("Failed to get userID from context")
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
				return
			}

			// 根据用户ID获取用户信息
			user, err := GetUserByID(userID.(int))
			if err != nil {
				Error("Failed to get user by ID %d: %v", userID, err)
				JSON(c, http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}

			Info("Get user info successful for user ID %d", userID)
			JSON(c, http.StatusOK, gin.H{
				"data": gin.H{
					"id":          user.ID,
					"name":        user.Name,
					"create_time": user.CreateTime.Format("2006-01-02 15:04:05"),
					"update_time": user.UpdateTime.Format("2006-01-02 15:04:05"),
				},
			})
		})

		// 115 open扫码登录相关路由
		// 获取支持的登录渠道列表
		auth.GET("/115/login/channels", func(c *gin.Context) {
			Debug("Get login channels API called from %s", c.ClientIP())
			channels := []gin.H{
				{"value": "wechatmini", "label": "微信小程序", "description": "使用微信小程序扫码登录"},
				{"value": "web", "label": "网页版", "description": "使用115网页版扫码登录"},
				{"value": "android", "label": "安卓APP", "description": "使用115安卓APP扫码登录"},
				{"value": "ios", "label": "iOS APP", "description": "使用115 iOS APP扫码登录"},
				{"value": "tv", "label": "电视版", "description": "使用115电视版扫码登录"},
				{"value": "alipaymini", "label": "支付宝小程序", "description": "使用支付宝小程序扫码登录"},
				{"value": "qandroid", "label": "安卓Q版", "description": "使用115安卓Q版扫码登录"},
			}
			JSON(c, 200, gin.H{
				"state":   true,
				"code":    0,
				"message": "success",
				"data":    channels,
			})
		})

		// 获取登录二维码
		auth.GET("/115/qrcode", func(c *gin.Context) {
			Debug("Get QR code API called from %s", c.ClientIP())
			qrCodeResp, err := client.GetQRCode()
			if err != nil {
				Error("Failed to get QR code: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			Info("Generated QR code successfully")
			JSON(c, 200, gin.H{
				"state":   true,
				"code":    0,
				"message": "success",
				"data": gin.H{
					"uid":    qrCodeResp.UID,
					"time":   qrCodeResp.Time,
					"qrcode": qrCodeResp.QrcodeContent,
					"sign":   qrCodeResp.Sign,
				},
				"error": "",
				"errno": 0,
			})
		})

		// 检查登录状态
		auth.GET("/115/login/status", func(c *gin.Context) {
			uid := c.Query("uid")
			timeStr := c.Query("time")
			sign := c.Query("sign")

			if uid == "" || timeStr == "" || sign == "" {
				Warn("Missing required parameters for login status check from %s", c.ClientIP())
				JSON(c, 400, gin.H{
					"error": "uid, time and sign are required",
				})
				return
			}

			timeVal := int64(0)
			fmt.Sscanf(timeStr, "%d", &timeVal)

			Debug("Checking login status for uid %s", uid)
			session := &driver.QRCodeSession{
				UID:           uid,
				Time:          timeVal,
				Sign:          sign,
				QrcodeContent: "",
			}
			statusResp, err := client.CheckLoginStatus(session)
			if err != nil {
				Error("Failed to check login status: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			JSON(c, 200, gin.H{
				"state":   true,
				"code":    0,
				"message": "success",
				"data": gin.H{
					"status":  statusResp.Status,
					"msg":     statusResp.Msg,
					"version": statusResp.Version,
				},
			})
		})

		// 115登录确认（二维码登录成功后保存凭据）
		auth.POST("/115/login/confirm", func(c *gin.Context) {
			var loginData struct {
				UID     string `json:"uid" binding:"required"`
				Time    int64  `json:"time" binding:"required"`
				Sign    string `json:"sign" binding:"required"`
				Name    string `json:"name"`
				CloudID int    `json:"cloud_id"`
				App     string `json:"app"`
			}

			if err := c.ShouldBindJSON(&loginData); err != nil {
				Warn("Invalid login confirm request body from %s: %v", c.ClientIP(), err)
				JSON(c, 400, gin.H{
					"error": "Invalid request body",
				})
				return
			}

			Debug("Confirming 115 login for uid %s with app: %s", loginData.UID, loginData.App)
			session := &driver.QRCodeSession{
				UID:           loginData.UID,
				Time:          loginData.Time,
				Sign:          loginData.Sign,
				QrcodeContent: "",
			}

			var cred *driver.Credential
			var err error

			if loginData.App != "" {
				cred, err = client.QRCodeLoginWithApp(session, driver.LoginApp(loginData.App))
			} else {
				cred, err = client.QRCodeLogin(session)
			}
			if err != nil {
				Error("Failed to complete QR code login: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			cookie := cred.Cookie()
			name := loginData.Name
			if name == "" {
				name = "115账号"
			}

			if loginData.CloudID > 0 {
				cloud115, err := GetCloud115ByID(loginData.CloudID)
				if err != nil {
					Error("Failed to get cloud115 account: %v", err)
					JSON(c, 404, gin.H{
						"error": "Cloud115 account not found",
					})
					return
				}
				_, err = UpdateCloud115(cloud115.ID, name, cookie, cloud115.RefreshToken, cloud115.AccessToken, cloud115.ExpiresIn, cloud115.TransferAccountID, cloud115.TransferDirectory)
				if err != nil {
					Error("Failed to update cloud115 account: %v", err)
					JSON(c, 500, gin.H{
						"error": err.Error(),
					})
					return
				}
				Info("Updated 115 account: %s", name)
				JSON(c, 200, gin.H{
					"message": "115 account updated successfully",
					"cookie":  cookie,
				})
			} else {
				_, err := CreateCloud115(name, cookie, "", "", 0, 0, "")
				if err != nil {
					Error("Failed to create cloud115 account: %v", err)
					JSON(c, 500, gin.H{
						"error": err.Error(),
					})
					return
				}
				Info("Created new 115 account: %s", name)
				JSON(c, 200, gin.H{
					"message": "115 account created successfully",
					"cookie":  cookie,
				})
			}
		})

		// 115 Open API扫码登录相关路由（预留接口）
		// 获取Open API登录二维码
		auth.GET("/115/open/qrcode", func(c *gin.Context) {
			Debug("Get Open API QR code API called from %s", c.ClientIP())
			session, err := client.GetOpenAPIQRCode()
			if err != nil {
				Error("Failed to get Open API QR code: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			Info("Generated Open API QR code successfully")
			JSON(c, 200, gin.H{
				"state":   true,
				"code":    0,
				"message": "success",
				"data": gin.H{
					"qrcode_url": session.QRCodeUrl,
					"state":      session.State,
				},
			})
		})

		// 检查Open API登录状态
		auth.GET("/115/open/login/status", func(c *gin.Context) {
			state := c.Query("state")
			if state == "" {
				Warn("Missing state parameter for Open API login status check from %s", c.ClientIP())
				JSON(c, 400, gin.H{
					"error": "state is required",
				})
				return
			}

			Debug("Checking Open API login status for state %s", state)
			status, err := client.CheckOpenAPILoginStatus(state)
			if err != nil {
				Error("Failed to check Open API login status: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			JSON(c, 200, gin.H{
				"state":   true,
				"code":    0,
				"message": "success",
				"data": gin.H{
					"status": status,
				},
			})
		})

		// 确认Open API登录并保存Token
		auth.POST("/115/open/login/confirm", func(c *gin.Context) {
			var loginData struct {
				State   string `json:"state" binding:"required"`
				Name    string `json:"name"`
				CloudID int    `json:"cloud_id"`
			}

			if err := c.ShouldBindJSON(&loginData); err != nil {
				Warn("Invalid Open API login confirm request body from %s: %v", c.ClientIP(), err)
				JSON(c, 400, gin.H{
					"error": "Invalid request body",
				})
				return
			}

			Debug("Confirming Open API login for state %s", loginData.State)
			token, err := client.ConfirmOpenAPILogin(loginData.State)
			if err != nil {
				Error("Failed to complete Open API login: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			name := loginData.Name
			if name == "" {
				name = "115账号"
			}

			if loginData.CloudID > 0 {
				cloud115, err := GetCloud115ByID(loginData.CloudID)
				if err != nil {
					Error("Failed to get cloud115 account: %v", err)
					JSON(c, 404, gin.H{
						"error": "Cloud115 account not found",
					})
					return
				}
				_, err = UpdateCloud115(cloud115.ID, name, cloud115.Cookie, token.RefreshToken, token.AccessToken, token.ExpiresIn, cloud115.TransferAccountID, cloud115.TransferDirectory)
				if err != nil {
					Error("Failed to update cloud115 account: %v", err)
					JSON(c, 500, gin.H{
						"error": err.Error(),
					})
					return
				}
				Info("Updated 115 account with Open API tokens: %s", name)
				JSON(c, 200, gin.H{
					"message":       "115 account updated successfully",
					"access_token":  token.AccessToken,
					"refresh_token": token.RefreshToken,
				})
			} else {
				_, err := CreateCloud115(name, "", token.RefreshToken, token.AccessToken, token.ExpiresIn, 0, "")
				if err != nil {
					Error("Failed to create cloud115 account: %v", err)
					JSON(c, 500, gin.H{
						"error": err.Error(),
					})
					return
				}
				Info("Created new 115 account with Open API tokens: %s", name)
				JSON(c, 200, gin.H{
					"message":       "115 account created successfully",
					"access_token":  token.AccessToken,
					"refresh_token": token.RefreshToken,
				})
			}
		})

		// 115云账号管理相关路由
		// 获取所有115云账号
		auth.GET("/cloud115", func(c *gin.Context) {
			Debug("Get all cloud115 accounts API called from %s", c.ClientIP())
			cloud115List, err := GetAllCloud115()
			if err != nil {
				Error("Failed to get all cloud115 accounts: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			// 格式化返回数据的时间字段
			formattedCloud115List := make([]map[string]interface{}, len(cloud115List))
			for i, cloud115 := range cloud115List {
				formattedCloud115List[i] = map[string]interface{}{
					"id":                  cloud115.ID,
					"name":                cloud115.Name,
					"cookie":              cloud115.Cookie,
					"refresh_token":       cloud115.RefreshToken,
					"access_token":        cloud115.AccessToken,
					"expires_in":          cloud115.ExpiresIn,
					"transfer_account_id": cloud115.TransferAccountID,
					"transfer_directory":  cloud115.TransferDirectory,
					"create_time":         cloud115.CreateTime.Format("2006-01-02 15:04:05"),
					"update_time":         cloud115.UpdateTime.Format("2006-01-02 15:04:05"),
				}
			}

			JSON(c, 200, gin.H{
				"data": formattedCloud115List,
			})
		})

		// 根据ID获取115云账号
		auth.GET("/cloud115/:id", func(c *gin.Context) {
			idStr := c.Param("id")
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			Debug("Get cloud115 by ID API called from %s, ID: %d", c.ClientIP(), id)
			cloud115, err := GetCloud115ByID(id)
			if err != nil {
				Error("Failed to get cloud115 by ID %d: %v", id, err)
				JSON(c, 404, gin.H{
					"error": "Cloud115 account not found",
				})
				return
			}

			// 格式化返回数据的时间字段
			formattedCloud115 := map[string]interface{}{
				"id":                  cloud115.ID,
				"name":                cloud115.Name,
				"cookie":              cloud115.Cookie,
				"refresh_token":       cloud115.RefreshToken,
				"access_token":        cloud115.AccessToken,
				"expires_in":          cloud115.ExpiresIn,
				"transfer_account_id": cloud115.TransferAccountID,
				"transfer_directory":  cloud115.TransferDirectory,
				"create_time":         cloud115.CreateTime.Format("2006-01-02 15:04:05"),
				"update_time":         cloud115.UpdateTime.Format("2006-01-02 15:04:05"),
			}

			JSON(c, 200, gin.H{
				"data": formattedCloud115,
			})
		})

		// 创建115云账号
		auth.POST("/cloud115", func(c *gin.Context) {
			Debug("Create cloud115 account API called from %s", c.ClientIP())
			var cloud115Data struct {
				Name              string `json:"name" binding:"required"`
				Cookie            string `json:"cookie"`
				RefreshToken      string `json:"refresh_token"`
				AccessToken       string `json:"access_token"`
				ExpiresIn         int    `json:"expires_in"`
				TransferAccountID int    `json:"transfer_account_id"`
				TransferDirectory string `json:"transfer_directory"`
			}
			if err := c.ShouldBindJSON(&cloud115Data); err != nil {
				Warn("Invalid create cloud115 request body from %s: %v", c.ClientIP(), err)
				JSON(c, 400, gin.H{
					"error": "Invalid request body",
				})
				return
			}
			cloud115, err := CreateCloud115(cloud115Data.Name, cloud115Data.Cookie, cloud115Data.RefreshToken, cloud115Data.AccessToken, cloud115Data.ExpiresIn, cloud115Data.TransferAccountID, cloud115Data.TransferDirectory)
			if err != nil {
				Error("Failed to create cloud115 account: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			// 格式化返回数据的时间字段
			formattedCloud115 := map[string]interface{}{
				"id":                  cloud115.ID,
				"name":                cloud115.Name,
				"cookie":              cloud115.Cookie,
				"refresh_token":       cloud115.RefreshToken,
				"access_token":        cloud115.AccessToken,
				"expires_in":          cloud115.ExpiresIn,
				"transfer_account_id": cloud115.TransferAccountID,
				"transfer_directory":  cloud115.TransferDirectory,
				"create_time":         cloud115.CreateTime.Format("2006-01-02 15:04:05"),
				"update_time":         cloud115.UpdateTime.Format("2006-01-02 15:04:05"),
			}

			JSON(c, 201, gin.H{
				"message": "Cloud115 account created successfully",
				"data":    formattedCloud115,
			})
		})

		// 更新115云账号
		auth.PUT("/cloud115/:id", func(c *gin.Context) {
			idStr := c.Param("id")
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			Debug("Update cloud115 account API called from %s, ID: %d", c.ClientIP(), id)
			var cloud115Data struct {
				Name              string `json:"name" binding:"required"`
				Cookie            string `json:"cookie"`
				RefreshToken      string `json:"refresh_token"`
				AccessToken       string `json:"access_token"`
				ExpiresIn         int    `json:"expires_in"`
				TransferAccountID int    `json:"transfer_account_id"`
				TransferDirectory string `json:"transfer_directory"`
			}
			if err := c.ShouldBindJSON(&cloud115Data); err != nil {
				Warn("Invalid update cloud115 request body from %s: %v", c.ClientIP(), err)
				JSON(c, 400, gin.H{
					"error": "Invalid request body",
				})
				return
			}
			cloud115, err := UpdateCloud115(id, cloud115Data.Name, cloud115Data.Cookie, cloud115Data.RefreshToken, cloud115Data.AccessToken, cloud115Data.ExpiresIn, cloud115Data.TransferAccountID, cloud115Data.TransferDirectory)
			if err != nil {
				Error("Failed to update cloud115 account with ID %d: %v", id, err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			// 格式化返回数据的时间字段
			formattedCloud115 := map[string]interface{}{
				"id":                  cloud115.ID,
				"name":                cloud115.Name,
				"cookie":              cloud115.Cookie,
				"refresh_token":       cloud115.RefreshToken,
				"access_token":        cloud115.AccessToken,
				"expires_in":          cloud115.ExpiresIn,
				"transfer_account_id": cloud115.TransferAccountID,
				"transfer_directory":  cloud115.TransferDirectory,
				"create_time":         cloud115.CreateTime.Format("2006-01-02 15:04:05"),
				"update_time":         cloud115.UpdateTime.Format("2006-01-02 15:04:05"),
			}

			JSON(c, 200, gin.H{
				"message": "Cloud115 account updated successfully",
				"data":    formattedCloud115,
			})
		})

		// 删除115云账号
		auth.DELETE("/cloud115/:id", func(c *gin.Context) {
			idStr := c.Param("id")
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			Debug("Delete cloud115 account API called from %s, ID: %d", c.ClientIP(), id)
			err := DeleteCloud115(id)
			if err != nil {
				Error("Failed to delete cloud115 account with ID %d: %v", id, err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}
			JSON(c, 200, gin.H{
				"message": "Cloud115 account deleted successfully",
			})
		})

		// 获取115云文件列表
		auth.GET("/115/files", func(c *gin.Context) {
			Debug("Get 115 files API called from %s", c.ClientIP())

			// 获取查询参数，设置默认值
			cid := 0
			showDir := 1
			offset := 0
			limit := 100

			// 解析查询参数
			if c.Query("cid") != "" {
				fmt.Sscanf(c.Query("cid"), "%d", &cid)
			}
			if c.Query("show_dir") != "" {
				fmt.Sscanf(c.Query("show_dir"), "%d", &showDir)
			}
			if c.Query("offset") != "" {
				fmt.Sscanf(c.Query("offset"), "%d", &offset)
			}
			if c.Query("limit") != "" {
				fmt.Sscanf(c.Query("limit"), "%d", &limit)
			}
			cloud115IdStr := c.Query("cloud115_id")

			Debug("Get files with cid: %d, show_dir: %d, offset: %d, limit: %d, cloud115_id: %s", cid, showDir, offset, limit, cloud115IdStr)

			// 获取指定的115云账号
			var cloud115 *Cloud115
			var err error

			if cloud115IdStr != "" {
				var cloud115Id int
				fmt.Sscanf(cloud115IdStr, "%d", &cloud115Id)
				cloud115, err = GetCloud115ByID(cloud115Id)
				if err != nil {
					Error("Failed to get cloud115 account by ID %d: %v", cloud115Id, err)
					JSON(c, 404, gin.H{
						"error": "Cloud115 account not found",
					})
					return
				}
			} else {
				cloud115List, err := GetAllCloud115()
				if err != nil {
					Error("Failed to get cloud115 accounts: %v", err)
					JSON(c, 500, gin.H{
						"error": "Failed to get cloud115 accounts",
					})
					return
				}

				if len(cloud115List) == 0 {
					Error("No cloud115 accounts found")
					JSON(c, 404, gin.H{
						"error": "No cloud115 accounts found",
					})
					return
				}

				cloud115 = cloud115List[0]
			}

			Debug("Using cloud115 account: %s (ID: %d)", cloud115.Name, cloud115.ID)
			// 调用客户端获取文件列表
			fileList, err := client.GetFileList(cid, showDir, offset, limit, cloud115.ID, cloud115.Cookie)
			if err != nil {
				Error("Failed to get 115 files: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			Info("Get 115 files successful, count: %d", fileList.Count)
			JSON(c, 200, fileList)
		})

		// 测试115账号cookie是否有效
		auth.GET("/api/115/test", func(c *gin.Context) {
			Debug("Test 115 account API called from %s", c.ClientIP())

			// 获取id参数
			idStr := c.Query("id")
			if idStr == "" {
				Warn("Missing required parameter id from %s", c.ClientIP())
				JSON(c, 400, gin.H{"error": "id is required"})
				return
			}

			var id int
			fmt.Sscanf(idStr, "%d", &id)

			// 根据id获取115账号
			cloud115, err := GetCloud115ByID(id)
			if err != nil {
				Error("Failed to get cloud115 account by ID %d: %v", id, err)
				JSON(c, 404, gin.H{"error": "Cloud115 account not found"})
				return
			}

			Debug("Testing 115 account: %s (ID: %d)", cloud115.Name, cloud115.ID)

			// 测试获取根目录文件列表
			fileList, err := client.GetFileList(0, 1, 0, 20, cloud115.ID, cloud115.Cookie)
			if err != nil {
				Error("Failed to get 115 files for account %s: %v", cloud115.Name, err)
				JSON(c, 500, gin.H{
					"success":      false,
					"account_id":   cloud115.ID,
					"account_name": cloud115.Name,
					"error":        err.Error(),
				})
				return
			}

			Info("Test 115 account successful, account: %s, found %d files in root", cloud115.Name, fileList.Count)
			JSON(c, 200, gin.H{
				"success":      true,
				"account_id":   cloud115.ID,
				"account_name": cloud115.Name,
				"file_count":   fileList.Count,
				"files":        fileList.Files,
			})
		})

		// 获取115云文件直链
		auth.GET("/115/direct-link", func(c *gin.Context) {
			Debug("Get 115 direct link API called from %s", c.ClientIP())

			// 获取查询参数
			fid := c.Query("fid")
			cloud115IdStr := c.Query("cloud115_id")

			// 验证必要参数
			if fid == "" {
				Warn("Missing required parameter fid from %s", c.ClientIP())
				JSON(c, 400, gin.H{
					"error": "fid is required",
				})
				return
			}

			Debug("Get direct link with fid: %s, cloud115_id: %s", fid, cloud115IdStr)

			// 获取指定的115云账号
			var cloud115 *Cloud115
			var err error

			if cloud115IdStr != "" {
				// 使用指定的115云账号
				var cloud115Id int
				fmt.Sscanf(cloud115IdStr, "%d", &cloud115Id)
				cloud115, err = GetCloud115ByID(cloud115Id)
				if err != nil {
					Error("Failed to get cloud115 account by ID %d: %v", cloud115Id, err)
					JSON(c, 404, gin.H{
						"error": "Cloud115 account not found",
					})
					return
				}
			} else {
				// 使用第一个115云账号
				cloud115List, err := GetAllCloud115()
				if err != nil {
					Error("Failed to get cloud115 accounts: %v", err)
					JSON(c, 500, gin.H{
						"error": "Failed to get cloud115 accounts",
					})
					return
				}

				if len(cloud115List) == 0 {
					Error("No cloud115 accounts found")
					JSON(c, 404, gin.H{
						"error": "No cloud115 accounts found",
					})
					return
				}

				cloud115 = cloud115List[0]
			}

			Debug("Using cloud115 account: %s", cloud115.Name)
			// 获取客户端 UA (用于同步签名)
			clientUA := c.GetHeader("User-Agent")
			if clientUA == "" {
				clientUA = driver.UA115Disk
			}

			// 调用客户端获取文件直链
			directLink, err := client.GetFileDirectLink(0, fid, cloud115.ID, cloud115.Cookie, clientUA)
			if err != nil {
				Error("Failed to get 115 direct link: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			// 提取Header信息
			headers := make(map[string]string)
			for key, values := range directLink.Header {
				if len(values) > 0 {
					headers[key] = values[0]
				}
			}

			Info("Get 115 direct link successful for file: %s", directLink.FileName)
			JSON(c, 200, gin.H{
				"state":   true,
				"code":    0,
				"message": "success",
				"data": gin.H{
					"url":      directLink.Url.Url,
					"size":     directLink.FileSize,
					"name":     directLink.FileName,
					"pickcode": directLink.PickCode,
					"headers":  headers,
				},
			})
		})

		// 生成115目录树
		auth.POST("/115/export-dir", func(c *gin.Context) {
			Debug("Export 115 directory tree API called from %s", c.ClientIP())

			var exportData struct {
				CID        string `json:"cid" binding:"required"`
				Cloud115Id string `json:"cloud115_id"`
			}

			if err := c.ShouldBindJSON(&exportData); err != nil {
				Warn("Invalid export directory tree request body from %s: %v", c.ClientIP(), err)
				JSON(c, 400, gin.H{
					"error": "Invalid request body",
				})
				return
			}

			Debug("Export directory tree with cid: %s, cloud115_id: %s", exportData.CID, exportData.Cloud115Id)

			var cloud115 *Cloud115
			var err error

			if exportData.Cloud115Id != "" {
				var cloud115Id int
				fmt.Sscanf(exportData.Cloud115Id, "%d", &cloud115Id)
				cloud115, err = GetCloud115ByID(cloud115Id)
				if err != nil {
					Error("Failed to get cloud115 account by ID %d: %v", cloud115Id, err)
					JSON(c, 404, gin.H{
						"error": "Cloud115 account not found",
					})
					return
				}
			} else {
				cloud115List, err := GetAllCloud115()
				if err != nil {
					Error("Failed to get cloud115 accounts: %v", err)
					JSON(c, 500, gin.H{
						"error": "Failed to get cloud115 accounts",
					})
					return
				}

				if len(cloud115List) == 0 {
					Error("No cloud115 accounts found")
					JSON(c, 404, gin.H{
						"error": "No cloud115 accounts found",
					})
					return
				}

				cloud115 = cloud115List[0]
			}

			Debug("Using cloud115 account: %s", cloud115.Name)

			fileIds := exportData.CID
			target := fmt.Sprintf("U_1_%s", exportData.CID)

			exportResp, err := client.ExportDirectoryTree115(fileIds, target, cloud115.Cookie)
			if err != nil {
				Error("Failed to export 115 directory tree: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			Info("Export 115 directory tree successful with export_id: %s", exportResp.Data.ExportID.String())
			JSON(c, 200, exportResp)
		})

		// 查询115目录树生成状态
		auth.GET("/115/export-dir/status", func(c *gin.Context) {
			Debug("Get 115 export directory tree status API called from %s", c.ClientIP())

			exportId := c.Query("export_id")
			cloud115IdStr := c.Query("cloud115_id")

			if exportId == "" {
				Warn("Missing required parameter export_id from %s", c.ClientIP())
				JSON(c, 400, gin.H{
					"error": "export_id is required",
				})
				return
			}

			Debug("Get export directory tree status with export_id: %s, cloud115_id: %s", exportId, cloud115IdStr)

			var cloud115 *Cloud115
			var err error

			if cloud115IdStr != "" {
				var cloud115Id int
				fmt.Sscanf(cloud115IdStr, "%d", &cloud115Id)
				cloud115, err = GetCloud115ByID(cloud115Id)
				if err != nil {
					Error("Failed to get cloud115 account by ID %d: %v", cloud115Id, err)
					JSON(c, 404, gin.H{
						"error": "Cloud115 account not found",
					})
					return
				}
			} else {
				cloud115List, err := GetAllCloud115()
				if err != nil {
					Error("Failed to get cloud115 accounts: %v", err)
					JSON(c, 500, gin.H{
						"error": "Failed to get cloud115 accounts",
					})
					return
				}

				if len(cloud115List) == 0 {
					Error("No cloud115 accounts found")
					JSON(c, 404, gin.H{
						"error": "No cloud115 accounts found",
					})
					return
				}

				cloud115 = cloud115List[0]
			}

			Debug("Using cloud115 account: %s", cloud115.Name)

			statusResp, err := client.GetExportDirectoryTreeStatus(exportId, cloud115.Cookie)
			if err != nil {
				Error("Failed to get 115 export directory tree status: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			Info("Get 115 export directory tree status successful with status: %d", statusResp.GetFirstData().Status)
			JSON(c, 200, statusResp)
		})

		// STRM配置管理相关路由
		// 获取所有STRM配置
		auth.GET("/strm/config", func(c *gin.Context) {
			Debug("Get all strm config API called from %s", c.ClientIP())
			strmConfigList, err := GetAllStrmConfig()
			if err != nil {
				Error("Failed to get all strm config: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			// 格式化返回数据的时间字段
			formattedStrmConfigList := make([]map[string]interface{}, len(strmConfigList))
			for i, strmConfig := range strmConfigList {
				formattedStrmConfigList[i] = map[string]interface{}{
					"id":            strmConfig.ID,
					"cloud115_id":   strmConfig.Cloud115Id,
					"net_disk_path": strmConfig.NetDiskPath,
					"local_path":    strmConfig.LocalPath,
					"cron":          strmConfig.Cron,
					"extension":     strmConfig.Extension,
					"create_time":   strmConfig.CreateTime.Format("2006-01-02 15:04:05"),
					"update_time":   strmConfig.UpdateTime.Format("2006-01-02 15:04:05"),
				}
			}

			JSON(c, 200, gin.H{
				"data": formattedStrmConfigList,
			})
		})

		// 根据ID获取STRM配置
		auth.GET("/strm/config/:id", func(c *gin.Context) {
			idStr := c.Param("id")
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			Debug("Get strm config by ID API called from %s, ID: %d", c.ClientIP(), id)
			strmConfig, err := GetStrmConfigByID(id)
			if err != nil {
				Error("Failed to get strm config by ID %d: %v", id, err)
				JSON(c, 404, gin.H{
					"error": "STRM config not found",
				})
				return
			}

			// 格式化返回数据的时间字段
			formattedStrmConfig := map[string]interface{}{
				"id":            strmConfig.ID,
				"cloud115_id":   strmConfig.Cloud115Id,
				"net_disk_path": strmConfig.NetDiskPath,
				"local_path":    strmConfig.LocalPath,
				"cron":          strmConfig.Cron,
				"extension":     strmConfig.Extension,
				"create_time":   strmConfig.CreateTime.Format("2006-01-02 15:04:05"),
				"update_time":   strmConfig.UpdateTime.Format("2006-01-02 15:04:05"),
			}

			JSON(c, 200, gin.H{
				"data": formattedStrmConfig,
			})
		})

		// 创建STRM配置
		auth.POST("/strm/config", func(c *gin.Context) {
			Debug("Create strm config API called from %s", c.ClientIP())
			var strmConfigData struct {
				Cloud115Id  int    `json:"cloud115_id" binding:"required"`
				NetDiskPath string `json:"net_disk_path" binding:"required"`
				LocalPath   string `json:"local_path" binding:"required"`
				Cron        string `json:"cron"`
				Extension   string `json:"extension"`
			}
			if err := c.ShouldBindJSON(&strmConfigData); err != nil {
				Warn("Invalid create strm config request body from %s: %v", c.ClientIP(), err)
				JSON(c, 400, gin.H{
					"error": "Invalid request body",
				})
				return
			}
			strmConfig, err := CreateStrmConfig(strmConfigData.Cloud115Id, strmConfigData.NetDiskPath, strmConfigData.LocalPath, strmConfigData.Cron, strmConfigData.Extension)
			if err != nil {
				Error("Failed to create strm config: %v", err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			// 格式化返回数据的时间字段
			formattedStrmConfig := map[string]interface{}{
				"id":            strmConfig.ID,
				"cloud115_id":   strmConfig.Cloud115Id,
				"net_disk_path": strmConfig.NetDiskPath,
				"local_path":    strmConfig.LocalPath,
				"cron":          strmConfig.Cron,
				"extension":     strmConfig.Extension,
				"create_time":   strmConfig.CreateTime.Format("2006-01-02 15:04:05"),
				"update_time":   strmConfig.UpdateTime.Format("2006-01-02 15:04:05"),
			}

			JSON(c, 201, gin.H{
				"message": "STRM config created successfully",
				"data":    formattedStrmConfig,
			})
		})

		// 更新STRM配置
		auth.PUT("/strm/config/:id", func(c *gin.Context) {
			idStr := c.Param("id")
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			Debug("Update strm config API called from %s, ID: %d", c.ClientIP(), id)
			var strmConfigData struct {
				Cloud115Id  int    `json:"cloud115_id" binding:"required"`
				NetDiskPath string `json:"net_disk_path" binding:"required"`
				LocalPath   string `json:"local_path" binding:"required"`
				Cron        string `json:"cron"`
				Extension   string `json:"extension"`
			}
			if err := c.ShouldBindJSON(&strmConfigData); err != nil {
				Warn("Invalid update strm config request body from %s: %v", c.ClientIP(), err)
				JSON(c, 400, gin.H{
					"error": "Invalid request body",
				})
				return
			}
			strmConfig, err := UpdateStrmConfig(id, strmConfigData.Cloud115Id, strmConfigData.NetDiskPath, strmConfigData.LocalPath, strmConfigData.Cron, strmConfigData.Extension)
			if err != nil {
				Error("Failed to update strm config with ID %d: %v", id, err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}

			// 格式化返回数据的时间字段
			formattedStrmConfig := map[string]interface{}{
				"id":            strmConfig.ID,
				"cloud115_id":   strmConfig.Cloud115Id,
				"net_disk_path": strmConfig.NetDiskPath,
				"local_path":    strmConfig.LocalPath,
				"cron":          strmConfig.Cron,
				"extension":     strmConfig.Extension,
				"create_time":   strmConfig.CreateTime.Format("2006-01-02 15:04:05"),
				"update_time":   strmConfig.UpdateTime.Format("2006-01-02 15:04:05"),
			}

			JSON(c, 200, gin.H{
				"message": "STRM config updated successfully",
				"data":    formattedStrmConfig,
			})
		})

		// 删除STRM配置
		auth.DELETE("/strm/config/:id", func(c *gin.Context) {
			idStr := c.Param("id")
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			Debug("Delete strm config API called from %s, ID: %d", c.ClientIP(), id)
			err := DeleteStrmConfig(id)
			if err != nil {
				Error("Failed to delete strm config with ID %d: %v", id, err)
				JSON(c, 500, gin.H{
					"error": err.Error(),
				})
				return
			}
			JSON(c, 200, gin.H{
				"message": "STRM config deleted successfully",
			})
		})

		// 全量生成STRM文件（异步）
		auth.POST("/strm/config/:id/generate/full", func(c *gin.Context) {
			idStr := c.Param("id")
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			Debug("Full generate STRM files API called from %s, ID: %d", c.ClientIP(), id)

			// 获取STRM配置
			strmConfig, err := GetStrmConfigByID(id)
			if err != nil {
				Error("Failed to get strm config by ID %d: %v", id, err)
				JSON(c, 404, gin.H{
					"error": "STRM config not found",
				})
				return
			}
			Info("Found STRM config ID %d: cloud115_id=%d, net_disk_path=%s, local_path=%s",
				strmConfig.ID, strmConfig.Cloud115Id, strmConfig.NetDiskPath, strmConfig.LocalPath)

			// 获取115云账号
			cloud115, err := GetCloud115ByID(strmConfig.Cloud115Id)
			if err != nil {
				Error("Failed to get cloud115 account by ID %d: %v", strmConfig.Cloud115Id, err)
				JSON(c, 404, gin.H{
					"error": "Cloud115 account not found",
				})
				return
			}
			Info("Using cloud115 account ID %d: %s", cloud115.ID, cloud115.Name)

			// 创建任务ID和任务名称
			taskID := uuid.New().String()
			taskName := fmt.Sprintf("STRM生成 - %s", strmConfig.NetDiskPath)
			_, err = CreateTask(taskID, TaskTypeStrmGenerate, taskName)
			if err != nil {
				Error("Failed to create task: %v", err)
				JSON(c, 500, gin.H{
					"error": "Failed to create task",
				})
				return
			}

			// 复制必要的配置数据，避免在goroutine中访问可能被修改的数据
			configID := id
			netDiskPath := strmConfig.NetDiskPath
			localPath := strmConfig.LocalPath
			extension := strmConfig.Extension
			cloud115Cookie := cloud115.Cookie
			cloud115ID := cloud115.ID
			serverURL := config.ServerURL

			// 异步执行生成任务
			go func() {
				defer func() {
					if r := recover(); r != nil {
						Error("Panic in STRM generation task: %v", r)
						SetTaskError(taskID, fmt.Sprintf("Internal error: %v", r))
					}
				}()

				Info("Starting STRM generation task %s for config ID %d", taskID, configID)
				UpdateTaskStatus(taskID, TaskStatusRunning)

				var cid string
				var dirTree *DirectoryNode

				// 根据网盘路径获取CID
				var err error
				cid, err = client.GetCIDByPath(netDiskPath, cloud115ID, cloud115Cookie)
				if err != nil {
					Error("Failed to get CID by path %s: %v", netDiskPath, err)
					SetTaskError(taskID, fmt.Sprintf("Failed to get CID by path: %v", err))
					return
				}
				Info("Found CID %s for path: %s", cid, netDiskPath)

				// 将CID转换为int
				cidInt, err := strconv.Atoi(cid)
				if err != nil {
					Error("Failed to parse CID %s: %v", cid, err)
					SetTaskError(taskID, fmt.Sprintf("Failed to parse CID: %v", err))
					return
				}

				// 使用 115 目录导出接口获取完整的文件列表
				Info("Starting directory tree export for CID: %d", cidInt)

				// 获取配置的网盘路径的最后一级目录名称作为根节点名称
				rootName := filepath.Base(netDiskPath)
				if rootName == "" || rootName == "." || rootName == "/" {
					rootName = "根目录"
				}

				// 1. 发起导出目录树任务
				exportResp, err := client.ExportDirectoryTree115(fmt.Sprintf("%d", cidInt), fmt.Sprintf("U_1_%d", cidInt), cloud115Cookie)
				if err != nil {
					Error("Failed to trigger directory tree export: %v", err)
					SetTaskError(taskID, fmt.Sprintf("Failed to trigger directory tree export: %v", err))
					return
				}

				if !exportResp.State {
					Error("Export directory tree API returned false state: %s", exportResp.Message)
					SetTaskError(taskID, fmt.Sprintf("Export directory tree failed: %s", exportResp.Message))
					return
				}

				exportId := exportResp.Data.ExportID.String()
				Info("Directory tree export triggered, export_id: %s. Waiting for completion...", exportId)

				// 2. 轮询等待导出完成
				var pickCode string
				maxRetries := 60 // 最多等待 60 * 5 = 300 秒 (5分钟)
				for i := 0; i < maxRetries; i++ {
					statusResp, err := client.GetExportDirectoryTreeStatus(exportId, cloud115Cookie)
					if err != nil {
						Warn("Failed to get export status (retry %d): %v", i+1, err)
						time.Sleep(5 * time.Second)
						continue
					}

					data := statusResp.GetFirstData()
					if data == nil {
						Warn("Export status response has no data (retry %d)", i+1)
						time.Sleep(5 * time.Second)
						continue
					}

					// status 为 2 表示成功
					if data.Status == 2 && data.PickCode != "" {
						pickCode = data.PickCode
						Info("Directory tree export completed. PickCode: %s", pickCode)
						break
					} else if data.Status == 3 || data.Status == -1 { // 假设 3 或 -1 为失败状态，具体取决于实际 API
						Error("Directory tree export failed with status %d", data.Status)
						SetTaskError(taskID, fmt.Sprintf("Directory tree export failed with status %d", data.Status))
						return
					}

					Debug("Export in progress (status %d, progress %d%%). Waiting...", data.Status, data.Progress)
					time.Sleep(5 * time.Second)
				}

				if pickCode == "" {
					Error("Timed out waiting for directory tree export to complete")
					SetTaskError(taskID, "Timed out waiting for directory tree export")
					return
				}

				// 3. 下载目录树文本文件
				Info("Downloading directory tree file with PickCode: %s", pickCode)
				fileData, err := client.DownloadDirectoryTreeFile(pickCode, cloud115ID, cloud115Cookie)
				if err != nil {
					Error("Failed to download directory tree file: %v", err)
					SetTaskError(taskID, fmt.Sprintf("Failed to download directory tree file: %v", err))
					return
				}

				// 4. 解析文本为条目并构建树
				entries, err := Parse115DirTreeFile(fileData)
				if err != nil {
					Error("Failed to parse directory tree file: %v", err)
					SetTaskError(taskID, fmt.Sprintf("Failed to parse directory tree file: %v", err))
					return
				}

				dirTree = BuildTreeFromExport(entries, cid, rootName)
				Info("Directory tree built successfully from export, total entries: %d", len(entries))

				// 过滤指定后缀名的文件
				if extension == "" {
					extension = ".strm"
				}

				// 解析多个后缀名（逗号分隔）
				extensions := strings.Split(extension, ",")
				targetExts := make([]string, 0, len(extensions))
				for _, ext := range extensions {
					ext = strings.TrimSpace(ext)
					if ext != "" {
						targetExts = append(targetExts, strings.ToLower(strings.TrimPrefix(ext, ".")))
					}
				}
				Info("Filtering files with extensions: %v", targetExts)

				// 从目录树中提取视频文件
				collection := &VideoCollection{
					Videos: []VideoFile{},
				}
				// isRoot=true 表示当前是根节点，子目录使用自己的名称作为路径
				extractVideoFiles(dirTree, "", netDiskPath, cid, targetExts, collection, strmConfig.Cloud115Id, true)
				collection.Total = len(collection.Videos)
				Info("Found %d files matching extensions %v", collection.Total, targetExts)

				// 创建STRM生成器
				generator := NewStrmGeneratorWithServer(localPath, serverURL, ".strm")
				generator.ProgressCallback = func(totalFiles, processedFiles, successFiles, failedFiles int) {
					UpdateTaskProgress(taskID, totalFiles, processedFiles, successFiles, failedFiles)
				}

				// 生成STRM文件
				err = generator.GenerateStrmFilesAsync(collection, netDiskPath)
				if err != nil {
					Error("Failed to generate STRM files: %v", err)
					SetTaskError(taskID, fmt.Sprintf("Failed to generate STRM files: %v", err))
					return
				}

				Info("Full STRM generation completed for config ID %d, task %s", configID, taskID)
				UpdateTaskStatus(taskID, TaskStatusCompleted)
			}()

			// 立即返回任务ID
			JSON(c, 200, gin.H{
				"message":   "STRM generation task started",
				"task_id":   taskID,
				"config_id": configID,
			})
		})

		// 查询STRM生成任务状态
		auth.GET("/strm/task/:task_id", func(c *gin.Context) {
			taskID := c.Param("task_id")
			Debug("Get STRM task status API called from %s, task_id: %s", c.ClientIP(), taskID)

			task, err := GetTask(taskID)
			if err != nil {
				Error("Failed to get task %s: %v", taskID, err)
				JSON(c, 500, gin.H{
					"error": "Failed to get task status",
				})
				return
			}

			if task == nil {
				JSON(c, 404, gin.H{
					"error": "Task not found",
				})
				return
			}

			JSON(c, 200, gin.H{
				"data": task,
			})
		})

		// 增量生成STRM文件
		auth.POST("/strm/config/:id/generate/incremental", func(c *gin.Context) {
			idStr := c.Param("id")
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			Debug("Incremental generate STRM files API called from %s, ID: %d", c.ClientIP(), id)

			// 增量生成目前与全量生成相同，后续可以添加增量逻辑
			JSON(c, 200, gin.H{
				"message": "Incremental STRM generation is not yet implemented, please use full generation",
			})
		})

		// 日志查看相关路由（仅admin用户可访问）
		// 获取日志文件列表
		auth.GET("/logs", func(c *gin.Context) {
			Debug("Get log files API called from %s", c.ClientIP())

			// 从上下文获取用户ID
			userID, exists := c.Get("userID")
			if !exists {
				Error("Failed to get userID from context")
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
				return
			}

			// 根据用户ID获取用户信息
			user, err := GetUserByID(userID.(int))
			if err != nil {
				Error("Failed to get user by ID %d: %v", userID, err)
				JSON(c, http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}

			// 验证是否为admin用户
			if user.Name != "admin" {
				Warn("Non-admin user %s attempted to access logs", user.Name)
				JSON(c, http.StatusForbidden, gin.H{"error": "Permission denied"})
				return
			}

			// 获取日志目录
			logDir := logger.logDir
			if logDir == "" {
				logDir = "logs"
			}

			// 读取日志文件列表（支持info_*.log和debug_*.log格式）
			infoFiles, err := filepath.Glob(filepath.Join(logDir, "info_*.log"))
			if err != nil {
				Error("Failed to glob info log files: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get log files"})
				return
			}

			debugFiles, err := filepath.Glob(filepath.Join(logDir, "debug_*.log"))
			if err != nil {
				Error("Failed to glob debug log files: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get log files"})
				return
			}

			// 合并文件列表
			files := append(infoFiles, debugFiles...)

			// 构建返回数据
			logFiles := make([]map[string]interface{}, 0)
			for _, file := range files {
				info, err := os.Stat(file)
				if err != nil {
					continue
				}
				// 根据文件名判断日志类型
				fileName := filepath.Base(file)
				logType := "info"
				if strings.HasPrefix(fileName, "debug_") {
					logType = "debug"
				}
				logFiles = append(logFiles, map[string]interface{}{
					"name":     fileName,
					"size":     info.Size(),
					"mod_time": info.ModTime().Format("2006-01-02 15:04:05"),
					"type":     logType,
				})
			}

			// 按文件名降序排列（最新的在前面）
			for i, j := 0, len(logFiles)-1; i < j; i, j = i+1, j-1 {
				logFiles[i], logFiles[j] = logFiles[j], logFiles[i]
			}

			Info("Admin user %s accessed log files list", user.Name)
			JSON(c, http.StatusOK, gin.H{
				"data": logFiles,
			})
		})

		// 获取日志文件内容
		auth.GET("/logs/:filename", func(c *gin.Context) {
			filename := c.Param("filename")
			Debug("Get log file content API called from %s, filename: %s", c.ClientIP(), filename)

			// 从上下文获取用户ID
			userID, exists := c.Get("userID")
			if !exists {
				Error("Failed to get userID from context")
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
				return
			}

			// 根据用户ID获取用户信息
			user, err := GetUserByID(userID.(int))
			if err != nil {
				Error("Failed to get user by ID %d: %v", userID, err)
				JSON(c, http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}

			// 验证是否为admin用户
			if user.Name != "admin" {
				Warn("Non-admin user %s attempted to access log content", user.Name)
				JSON(c, http.StatusForbidden, gin.H{"error": "Permission denied"})
				return
			}

			// 安全检查：确保文件名格式正确（支持info_*.log和debug_*.log）
			if (!strings.HasPrefix(filename, "info_") && !strings.HasPrefix(filename, "debug_")) || !strings.HasSuffix(filename, ".log") {
				Warn("Invalid log filename format: %s", filename)
				JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid log filename"})
				return
			}

			// 获取日志目录
			logDir := logger.logDir
			if logDir == "" {
				logDir = "logs"
			}

			// 构建文件路径
			logPath := filepath.Join(logDir, filename)

			// 安全检查：确保路径在日志目录内
			absLogDir, err := filepath.Abs(logDir)
			if err != nil {
				Error("Failed to get absolute path of log dir: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get log file"})
				return
			}

			absLogPath, err := filepath.Abs(logPath)
			if err != nil {
				Error("Failed to get absolute path of log file: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get log file"})
				return
			}

			if !strings.HasPrefix(absLogPath, absLogDir) {
				Warn("Attempted path traversal attack: %s", filename)
				JSON(c, http.StatusForbidden, gin.H{"error": "Access denied"})
				return
			}

			// 获取请求参数
			lines := 500 // 默认返回最后500行
			if linesStr := c.Query("lines"); linesStr != "" {
				if parsedLines, err := strconv.Atoi(linesStr); err == nil && parsedLines > 0 {
					lines = parsedLines
				}
			}

			// 读取日志文件
			content, err := readLastNLines(logPath, lines)
			if err != nil {
				if os.IsNotExist(err) {
					JSON(c, http.StatusNotFound, gin.H{"error": "Log file not found"})
					return
				}
				Error("Failed to read log file: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to read log file"})
				return
			}

			Info("Admin user %s accessed log file: %s", user.Name, filename)
			JSON(c, http.StatusOK, gin.H{
				"data": map[string]interface{}{
					"filename": filename,
					"content":  content,
					"lines":    lines,
				},
			})
		})

		// 获取日志保留天数配置
		auth.GET("/logs/config", func(c *gin.Context) {
			Debug("Get log config API called from %s", c.ClientIP())

			// 从上下文获取用户ID
			userID, exists := c.Get("userID")
			if !exists {
				Error("Failed to get userID from context")
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
				return
			}

			// 根据用户ID获取用户信息
			user, err := GetUserByID(userID.(int))
			if err != nil {
				Error("Failed to get user by ID %d: %v", userID, err)
				JSON(c, http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}

			// 验证是否为admin用户
			if user.Name != "admin" {
				Warn("Non-admin user %s attempted to access log config", user.Name)
				JSON(c, http.StatusForbidden, gin.H{"error": "Permission denied"})
				return
			}

			// 获取日志保留天数配置
			config, err := GetSystemConfigByKey("log_save_day_limit")
			if err != nil {
				// 如果配置不存在，返回默认值
				JSON(c, http.StatusOK, gin.H{
					"data": map[string]interface{}{
						"key":   "log_save_day_limit",
						"value": 1,
					},
				})
				return
			}

			// 将配置值转换为整数
			var days int
			fmt.Sscanf(config.ConfigVal, "%d", &days)

			JSON(c, http.StatusOK, gin.H{
				"data": map[string]interface{}{
					"key":   config.ConfigKey,
					"value": days,
				},
			})
		})

		// 更新日志保留天数配置
		auth.PUT("/logs/config", func(c *gin.Context) {
			Debug("Update log config API called from %s", c.ClientIP())

			// 从上下文获取用户ID
			userID, exists := c.Get("userID")
			if !exists {
				Error("Failed to get userID from context")
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
				return
			}

			// 根据用户ID获取用户信息
			user, err := GetUserByID(userID.(int))
			if err != nil {
				Error("Failed to get user by ID %d: %v", userID, err)
				JSON(c, http.StatusNotFound, gin.H{"error": "User not found"})
				return
			}

			// 验证是否为admin用户
			if user.Name != "admin" {
				Warn("Non-admin user %s attempted to update log config", user.Name)
				JSON(c, http.StatusForbidden, gin.H{"error": "Permission denied"})
				return
			}

			// 解析请求体
			var configData struct {
				Value int `json:"value" binding:"required,min=1,max=365"`
			}
			if err := c.ShouldBindJSON(&configData); err != nil {
				Warn("Invalid log config request body from %s: %v", c.ClientIP(), err)
				JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid request body, value must be between 1 and 365"})
				return
			}

			// 更新配置
			config, err := UpsertSystemConfig("log_save_day_limit", fmt.Sprintf("%d", configData.Value))
			if err != nil {
				Error("Failed to update log config: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to update config"})
				return
			}

			// 更新日志记录器的保留天数
			if logger != nil {
				logger.keepDays = configData.Value
			}

			Info("Admin user %s updated log save day limit to %d", user.Name, configData.Value)
			JSON(c, http.StatusOK, gin.H{
				"message": "Log config updated successfully",
				"data": map[string]interface{}{
					"key":   config.ConfigKey,
					"value": configData.Value,
				},
			})
		})

		// 任务管理相关路由
		// 获取所有任务
		auth.GET("/tasks", func(c *gin.Context) {
			Debug("Get all tasks API called from %s", c.ClientIP())
			tasks, err := GetAllTasks()
			if err != nil {
				Error("Failed to get all tasks: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get tasks"})
				return
			}
			JSON(c, http.StatusOK, gin.H{"data": tasks})
		})

		// 获取所有定时任务
		auth.GET("/scheduled-tasks", func(c *gin.Context) {
			Debug("Get all scheduled tasks API called from %s", c.ClientIP())
			tasks, err := GetAllCronTasks()
			if err != nil {
				JSON(c, http.StatusOK, gin.H{"data": []interface{}{}})
				return
			}
			JSON(c, http.StatusOK, gin.H{"data": tasks})
		})

		// ========== Cron任务管理API ==========

		// 获取所有cron定时任务
		auth.GET("/cron/tasks", func(c *gin.Context) {
			Debug("Get all cron tasks API called from %s", c.ClientIP())
			tasks, err := GetAllCronTasks()
			if err != nil {
				Error("Failed to get all cron tasks: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to get cron tasks"})
				return
			}

			// 为每个任务添加下次执行时间
			result := make([]gin.H, 0)
			for _, task := range tasks {
				taskData := gin.H{
					"id":               task.ID,
					"task_name":        task.TaskName,
					"task_type":        task.TaskType,
					"cloud115_id":      task.Cloud115ID,
					"strm_config_id":   task.StrmConfigID,
					"cron_expr":        task.CronExpr,
					"status":           task.Status,
					"last_run_time":    task.LastRunTime,
					"next_run_time":    task.NextRunTime,
					"last_run_status":  task.LastRunStatus,
					"last_run_message": task.LastRunMessage,
					"create_time":      task.CreateTime,
					"update_time":      task.UpdateTime,
				}

				// 从调度器获取下次执行时间
				if scheduler != nil {
					nextRun := scheduler.GetNextRunTime(task.ID)
					if nextRun != nil {
						taskData["next_run_time"] = nextRun
					}
				}

				result = append(result, taskData)
			}

			JSON(c, http.StatusOK, gin.H{"data": result})
		})

		// 创建cron定时任务
		auth.POST("/cron/task", func(c *gin.Context) {
			Debug("Create cron task API called from %s", c.ClientIP())

			var req struct {
				Cloud115ID   int    `json:"cloud115_id" binding:"required"`
				StrmConfigID int    `json:"strm_config_id" binding:"required"`
				CronExpr     string `json:"cron_expr" binding:"required"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				Warn("Invalid cron task request body from %s: %v", c.ClientIP(), err)
				JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			// 验证STRM配置存在
			strmConfig, err := GetStrmConfigByID(req.StrmConfigID)
			if err != nil {
				Error("Failed to get strm config: %v", err)
				JSON(c, http.StatusNotFound, gin.H{"error": "STRM config not found"})
				return
			}

			// 验证STRM配置属于该115账号
			if strmConfig.Cloud115Id != req.Cloud115ID {
				JSON(c, http.StatusBadRequest, gin.H{"error": "STRM config does not belong to this 115 account"})
				return
			}

			// 生成任务名称
			taskName := fmt.Sprintf("%d增量更新任务", req.Cloud115ID)

			// 检查任务是否已存在
			existingTask, _ := GetCronTaskByName(taskName)
			if existingTask != nil {
				JSON(c, http.StatusBadRequest, gin.H{"error": "Cron task already exists for this account"})
				return
			}

			// 创建定时任务
			task, err := CreateCronTask(taskName, "incremental_sync", req.Cloud115ID, req.StrmConfigID, req.CronExpr)
			if err != nil {
				Error("Failed to create cron task: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to create cron task"})
				return
			}

			// 添加到调度器
			if scheduler != nil {
				if err := scheduler.AddTask(task); err != nil {
					Warn("Failed to add cron task to scheduler: %v", err)
				}
			}

			Info("Created cron task: %s (ID: %d)", taskName, task.ID)
			JSON(c, http.StatusOK, gin.H{
				"message": "Cron task created successfully",
				"data":    task,
			})
		})

		// 更新cron定时任务
		auth.PUT("/cron/task/:id", func(c *gin.Context) {
			Debug("Update cron task API called from %s", c.ClientIP())

			taskID := 0
			fmt.Sscanf(c.Param("id"), "%d", &taskID)
			if taskID == 0 {
				JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
				return
			}

			var req struct {
				CronExpr string `json:"cron_expr" binding:"required"`
				Status   string `json:"status"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				Warn("Invalid cron task update request body from %s: %v", c.ClientIP(), err)
				JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			// 获取现有任务
			task, err := GetCronTaskByID(taskID)
			if err != nil {
				Error("Failed to get cron task: %v", err)
				JSON(c, http.StatusNotFound, gin.H{"error": "Cron task not found"})
				return
			}

			// 更新状态
			status := task.Status
			if req.Status != "" {
				status = req.Status
			}

			// 更新数据库
			task, err = UpdateCronTask(taskID, req.CronExpr, status)
			if err != nil {
				Error("Failed to update cron task: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to update cron task"})
				return
			}

			// 更新调度器
			if scheduler != nil {
				if err := scheduler.UpdateTask(task); err != nil {
					Warn("Failed to update cron task in scheduler: %v", err)
				}
			}

			Info("Updated cron task ID: %d", taskID)
			JSON(c, http.StatusOK, gin.H{
				"message": "Cron task updated successfully",
				"data":    task,
			})
		})

		// 删除cron定时任务
		auth.DELETE("/cron/task/:id", func(c *gin.Context) {
			Debug("Delete cron task API called from %s", c.ClientIP())

			taskID := 0
			fmt.Sscanf(c.Param("id"), "%d", &taskID)
			if taskID == 0 {
				JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
				return
			}

			// 从调度器移除
			if scheduler != nil {
				scheduler.RemoveTask(taskID)
			}

			// 从数据库删除
			if err := DeleteCronTask(taskID); err != nil {
				Error("Failed to delete cron task: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "Failed to delete cron task"})
				return
			}

			Info("Deleted cron task ID: %d", taskID)
			JSON(c, http.StatusOK, gin.H{"message": "Cron task deleted successfully"})
		})

		// 立即执行cron任务
		auth.POST("/cron/task/:id/run", func(c *gin.Context) {
			Debug("Run cron task immediately API called from %s", c.ClientIP())

			taskID := 0
			fmt.Sscanf(c.Param("id"), "%d", &taskID)
			if taskID == 0 {
				JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
				return
			}

			// 获取任务
			task, err := GetCronTaskByID(taskID)
			if err != nil {
				Error("Failed to get cron task: %v", err)
				JSON(c, http.StatusNotFound, gin.H{"error": "Cron task not found"})
				return
			}

			// 异步执行任务
			go ExecuteCronTask(task)

			Info("Triggered cron task ID: %d to run immediately", taskID)
			JSON(c, http.StatusOK, gin.H{
				"message": "Cron task triggered successfully",
				"task_id": taskID,
			})
		})

		// 获取cron任务执行状态
		auth.GET("/cron/task/:id/status", func(c *gin.Context) {
			Debug("Get cron task status API called from %s", c.ClientIP())

			taskID := 0
			fmt.Sscanf(c.Param("id"), "%d", &taskID)
			if taskID == 0 {
				JSON(c, http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
				return
			}

			task, err := GetCronTaskByID(taskID)
			if err != nil {
				Error("Failed to get cron task: %v", err)
				JSON(c, http.StatusNotFound, gin.H{"error": "Cron task not found"})
				return
			}

			// 从调度器获取下次执行时间
			nextRun := task.NextRunTime
			if scheduler != nil {
				if nr := scheduler.GetNextRunTime(taskID); nr != nil {
					nextRun = nr
				}
			}

			JSON(c, http.StatusOK, gin.H{
				"data": gin.H{
					"id":               task.ID,
					"task_name":        task.TaskName,
					"status":           task.Status,
					"last_run_time":    task.LastRunTime,
					"next_run_time":    nextRun,
					"last_run_status":  task.LastRunStatus,
					"last_run_message": task.LastRunMessage,
				},
			})
		})
	}
}

// buildDirTreeFromEntries 从目录树文件条目构建目录树结构
func buildDirTreeFromEntries(entries []DirTreeEntry, rootName string) *DirectoryNode {
	Debug("Building directory tree from %d entries, root name: %s", len(entries), rootName)

	root := &DirectoryNode{
		CID:      "0",
		Name:     rootName,
		Type:     "dir",
		Files:    []driver.FileInfo{},
		Children: []*DirectoryNode{},
	}

	// 构建路径到节点的映射
	pathToNode := make(map[string]*DirectoryNode)
	pathToNode[""] = root

	// 首先创建所有目录节点
	for _, entry := range entries {
		if entry.IsDir {
			node := &DirectoryNode{
				CID:      "0",
				Name:     entry.Name,
				Type:     "dir",
				Files:    []driver.FileInfo{},
				Children: []*DirectoryNode{},
			}
			// 使用完整路径作为键
			fullPath := entry.Path
			if fullPath != "" {
				fullPath = filepath.Join(fullPath, entry.Name)
			} else {
				fullPath = entry.Name
			}
			pathToNode[fullPath] = node
			Debug("Added directory node: %s at path: %s", entry.Name, fullPath)
		}
	}

	// 然后处理文件和构建目录层级关系
	for _, entry := range entries {
		if !entry.IsDir {
			// 创建文件信息
			fileInfo := driver.FileInfo{
				Name:     entry.Name,
				Size:     driver.StringInt64(entry.Size),
				PickCode: entry.Pc,
				FileID:   entry.Fid,
				Sha1:     entry.Sha1,
			}

			// 找到父目录
			parentPath := entry.Path
			parentNode, exists := pathToNode[parentPath]
			if !exists {
				parentNode = root
			}
			parentNode.Files = append(parentNode.Files, fileInfo)
			Debug("Added file: %s to directory at path: %s", entry.Name, parentPath)
		}
	}

	// 构建目录层级关系
	for fullPath, node := range pathToNode {
		if fullPath == "" {
			continue
		}
		// 获取父目录路径
		parentPath := filepath.Dir(fullPath)
		if parentPath == "." {
			parentPath = ""
		}
		parentNode, exists := pathToNode[parentPath]
		if exists && parentNode != node {
			parentNode.Children = append(parentNode.Children, node)
			Debug("Added child: %s to parent at path: %s", node.Name, parentPath)
		}
	}

	return root
}

// extractVideoFiles 从目录树中递归提取视频文件
// 根节点的文件直接放在输出目录下，子目录使用自己的名称作为路径
func extractVideoFiles(node *DirectoryNode, currentPath string, netDiskPath string, cid string, targetExts []string, collection *VideoCollection, cloud115Id int, isRoot bool) {
	Debug("Extracting video files from directory: %s, currentPath: %s, isRoot: %v", node.Name, currentPath, isRoot)

	for _, file := range node.Files {
		fileExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.Name), "."))

		matched := false
		for _, targetExt := range targetExts {
			if fileExt == targetExt {
				matched = true
				break
			}
		}

		if matched {
			netDiskFullPath := filepath.Join(netDiskPath, currentPath, file.Name)

			// 优先使用 PickCode，PickCode 是用于下载文件的唯一标识
			filePickCode := file.PickCode
			if filePickCode == "" {
				filePickCode = file.FileID
			}

			// 计算相对路径
			relativePath := currentPath
			if relativePath == "" {
				relativePath = file.Name
			} else {
				relativePath = filepath.Join(currentPath, file.Name)
			}

			videoFile := VideoFile{
				Path:         currentPath,
				Filename:     file.Name,
				CID:          cid,
				FID:          filePickCode,
				Size:         int(file.Size),
				Extension:    filepath.Ext(file.Name),
				Sha1:         netDiskFullPath,
				Cloud115ID:   cloud115Id,
				RelativePath: relativePath,
				PickCode:     filePickCode,
				Name:         file.Name,
			}

			collection.Videos = append(collection.Videos, videoFile)
			Debug("Added video file: %s (PickCode: %s, Size: %d, cloud115_id: %d)", netDiskFullPath, filePickCode, file.Size, cloud115Id)
		}
	}

	for _, child := range node.Children {
		// 如果是根节点，子目录使用自己的名称作为路径
		// 如果不是根节点，子目录使用完整路径
		childPath := child.Name
		if !isRoot && currentPath != "" {
			childPath = filepath.Join(currentPath, child.Name)
		}
		extractVideoFiles(child, childPath, netDiskPath, cid, targetExts, collection, cloud115Id, false)
	}
}

// readLastNLines 读取文件的最后N行，返回倒序结果（最新的日志在最上面）
func readLastNLines(filePath string, n int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 获取文件大小
	stat, err := file.Stat()
	if err != nil {
		return "", err
	}
	fileSize := stat.Size()

	// 从文件末尾开始读取
	var lines []string
	var lineBuffer []byte
	var offset int64 = fileSize - 1
	newlineCount := 0

	for offset >= 0 && newlineCount < n {
		// 读取一个字节
		b := make([]byte, 1)
		_, err := file.ReadAt(b, offset)
		if err != nil {
			break
		}

		if b[0] == '\n' {
			if len(lineBuffer) > 0 {
				// 反转行缓冲区并添加到结果
				line := reverseBytes(lineBuffer)
				lines = append(lines, string(line))
				lineBuffer = lineBuffer[:0]
				newlineCount++
			}
		} else {
			lineBuffer = append(lineBuffer, b[0])
		}
		offset--
	}

	// 处理最后一行（如果文件不以换行符结尾）
	if len(lineBuffer) > 0 && newlineCount < n {
		line := reverseBytes(lineBuffer)
		lines = append(lines, string(line))
	}

	// 不反转行顺序，保持倒序（最新的日志在最上面）
	// 这样前端显示时，最新的日志会在最上面

	return strings.Join(lines, "\n"), nil
}

// reverseBytes 反转字节切片
func reverseBytes(b []byte) []byte {
	result := make([]byte, len(b))
	for i := range b {
		result[len(b)-1-i] = b[i]
	}
	return result
}

// truncateString 截断字符串到指定长度，用于日志输出时隐藏敏感信息
func truncateString(s string, maxLen int) string {
	if s == "" {
		return "<empty>"
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
