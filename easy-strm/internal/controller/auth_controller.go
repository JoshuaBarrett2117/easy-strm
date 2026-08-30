package controller

import (
	"net/http"
	"strings"
	"time"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// UserInfo 用户信息（用于回调注入时传递用户数据）
type UserInfo struct {
	ID         int
	Name       string
	Password   string
	CreateTime time.Time
	UpdateTime time.Time
}

type AuthController struct {
	authService      *service.AuthService
	globalAPIService *service.GlobalAPIService
	redisClient      interface {
		Set(key string, value interface{}, expiration int) error
		Get(key string) (string, error)
		Del(key string) error
	}

	// --- 回调依赖：main 包全局函数通过依赖注入解耦 ---
	// NOTE: 以下函数属于 main 包，无法在 controller 层直接引用，
	// 因此通过回调模式注入，与 CronController/LogController 保持一致

	// getUserByName: 根据用户名获取用户（main.GetUserByName）
	getUserByName func(name string) (*UserInfo, error)
	// getUserByID: 根据ID获取用户（main.GetUserByID）
	getUserByID func(id int) (*UserInfo, error)
	// verifyPassword: 验证密码（main.VerifyPassword）
	verifyPassword func(hashedPassword, password string) error
	// generateToken: 生成JWT token（main.GenerateToken）
	generateToken func(userID int, secret string) (string, error)
	// setToken: 将token存储到Redis（main.SetToken）
	setToken func(userID int, token string) error
	// getToken: 从Redis获取token（main.GetToken）
	getToken func(userID int) (string, error)
	// jwtSecret: JWT密钥（来自 config.JWTSecret）
	jwtSecret string
	// verifyTokenAndReturnUserID: 验证token并返回userID（main.VerifyToken 的包装）
	verifyTokenAndReturnUserID func(tokenString string, secret string) (int, error)
}

// SetGlobalAPIService 注入全局 API Key 校验服务。
func (c *AuthController) SetGlobalAPIService(s *service.GlobalAPIService) { c.globalAPIService = s }

func NewAuthController(authService *service.AuthService) *AuthController {
	return &AuthController{
		authService: authService,
	}
}

// --- 回调注入方法 ---

func (c *AuthController) SetRedisClient(client interface {
	Set(key string, value interface{}, expiration int) error
	Get(key string) (string, error)
	Del(key string) error
}) {
	c.redisClient = client
}

func (c *AuthController) SetGetUserByName(fn func(name string) (*UserInfo, error)) {
	c.getUserByName = fn
}

func (c *AuthController) SetGetUserByID(fn func(id int) (*UserInfo, error)) {
	c.getUserByID = fn
}

func (c *AuthController) SetVerifyPassword(fn func(hashedPassword, password string) error) {
	c.verifyPassword = fn
}

func (c *AuthController) SetGenerateToken(fn func(userID int, secret string) (string, error)) {
	c.generateToken = fn
}

func (c *AuthController) SetSetToken(fn func(userID int, token string) error) {
	c.setToken = fn
}

func (c *AuthController) SetGetToken(fn func(userID int) (string, error)) {
	c.getToken = fn
}

func (c *AuthController) SetJWTSecret(secret string) {
	c.jwtSecret = secret
}

// --- 路由处理器方法 ---

// Login 登录处理器
// Route: POST /login, POST /auth/login
// 保持与原 auth.go 内联处理器一致的响应格式
func (c *AuthController) Login(ctx *gin.Context) {
	var loginData struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	if err := ctx.ShouldBindJSON(&loginData); err != nil {
		logger.Warnf("AuthController[Login] 无效的登录请求体 from %s: %v", ctx.ClientIP(), err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if c.getUserByName == nil || c.verifyPassword == nil || c.generateToken == nil || c.setToken == nil {
		logger.Error("AuthController[Login] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// 验证用户名
	user, err := c.getUserByName(loginData.Name)
	if err != nil {
		logger.Warnf("AuthController[Login] 用户 %s 未找到 from %s", loginData.Name, ctx.ClientIP())
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}
	logger.Infof("AuthController[Login] 找到用户 %s from %s", loginData.Name, ctx.ClientIP())

	// 验证密码
	err = c.verifyPassword(user.Password, loginData.Password)
	if err != nil {
		logger.Warnf("AuthController[Login] 用户 %s 密码错误 from %s", loginData.Name, ctx.ClientIP())
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	// 生成JWT token
	token, err := c.generateToken(user.ID, c.jwtSecret)
	if err != nil {
		logger.Errorf("AuthController[Login] 生成token失败 user %d: %v", user.ID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// 将token存储到Redis
	err = c.setToken(user.ID, token)
	if err != nil {
		logger.Errorf("AuthController[Login] 存储token失败 user %d: %v", user.ID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store token"})
		return
	}

	logger.Infof("AuthController[Login] 用户 %s 登录成功 from %s", loginData.Name, ctx.ClientIP())
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token":   token,
		"user_id": user.ID,
		"name":    user.Name,
	})
}

// GetUserInfo 获取用户信息
// Route: GET /user/info
func (c *AuthController) GetUserInfo(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		logger.Error("AuthController[GetUserInfo] 无法从上下文获取userID")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
		return
	}

	if c.getUserByID == nil {
		logger.Error("AuthController[GetUserInfo] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// 根据用户ID获取用户信息
	user, err := c.getUserByID(userID.(int))
	if err != nil {
		logger.Errorf("AuthController[GetUserInfo] 获取用户失败 ID %d: %v", userID, err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	logger.Infof("AuthController[GetUserInfo] 获取用户信息成功 ID %d", userID)
	ctx.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":          user.ID,
			"name":        user.Name,
			"create_time": user.CreateTime.Format("2006-01-02 15:04:05"),
			"update_time": user.UpdateTime.Format("2006-01-02 15:04:05"),
		},
	})
}

// JWTMiddleware JWT验证中间件
// 保持与原 auth.go 中间件一致的逻辑
func (c *AuthController) JWTMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if c.globalAPIService != nil {
			candidate := ctx.GetHeader("X-API-Key")
			if candidate == "" {
				a := ctx.GetHeader("Authorization")
				if strings.HasPrefix(a, "ApiKey ") {
					candidate = strings.TrimSpace(strings.TrimPrefix(a, "ApiKey "))
				}
			}
			if candidate != "" {
				valid, err := c.globalAPIService.ValidateAPIKey(candidate)
				if err != nil {
					ctx.JSON(http.StatusUnauthorized, gin.H{"error": "API key validation failed"})
					ctx.Abort()
					return
				}
				if valid {
					if c.getUserByName == nil {
						ctx.JSON(http.StatusUnauthorized, gin.H{"error": "API key user unavailable"})
						ctx.Abort()
						return
					}
					admin, err := c.getUserByName("admin")
					if err != nil || admin == nil {
						ctx.JSON(http.StatusUnauthorized, gin.H{"error": "API key user unavailable"})
						ctx.Abort()
						return
					}
					ctx.Set("userID", admin.ID)
					ctx.Set("authType", "api_key")
					ctx.Next()
					return
				}
				ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
				ctx.Abort()
				return
			}
		}
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			ctx.Abort()
			return
		}

		tokenString := authHeader
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		}

		// 使用 main 包的 VerifyToken 验证
		if c.generateToken == nil || c.getToken == nil {
			// 回调未注入时，回退到 service 层验证
			claims, err := c.authService.VerifyToken(tokenString)
			if err != nil {
				ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				ctx.Abort()
				return
			}
			ctx.Set("userID", int(claims.UserID))
			ctx.Next()
			return
		}

		// 通过 main 包的 VerifyToken 验证（注入的回调）
		// NOTE: 这里需要注入 verifyToken 回调来解析 JWT
		// 由于 VerifyToken 返回 *JWTClaims（main 包类型），
		// 我们通过注入一个 verifyTokenAndReturnUserID 回调来简化
		if c.verifyTokenAndReturnUserID != nil {
			userID, err := c.verifyTokenAndReturnUserID(tokenString, c.jwtSecret)
			if err != nil {
				ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				ctx.Abort()
				return
			}

			// 验证token是否存在于Redis
			tokenInRedis, err := c.getToken(userID)
			if err != nil {
				logger.Errorf("AuthController[JWTMiddleware] 从Redis获取token失败: %v", err)
				ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Failed to validate token"})
				ctx.Abort()
				return
			}

			if tokenInRedis != tokenString {
				ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				ctx.Abort()
				return
			}

			ctx.Set("userID", userID)
			ctx.Next()
			return
		}

		// 兜底：使用 service 层
		claims, err := c.authService.VerifyToken(tokenString)
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			ctx.Abort()
			return
		}
		ctx.Set("userID", int(claims.UserID))
		ctx.Next()
	}
}

// SetVerifyTokenAndReturnUserID 注入验证token并返回userID的回调
func (c *AuthController) SetVerifyTokenAndReturnUserID(fn func(tokenString string, secret string) (int, error)) {
	c.verifyTokenAndReturnUserID = fn
}

// UpdatePassword 更新密码
func (c *AuthController) UpdatePassword(ctx *gin.Context) {
	var data struct {
		Name    string `json:"name"`
		NewPass string `json:"new_password"`
	}
	if err := ctx.ShouldBindJSON(&data); err != nil {
		ctx.JSON(400, gin.H{"error": "参数错误"})
		return
	}
	if data.Name == "" || data.NewPass == "" {
		ctx.JSON(400, gin.H{"error": "用户名和新密码不能为空"})
		return
	}
	if err := c.authService.UpdatePassword(data.Name, data.NewPass); err != nil {
		ctx.JSON(500, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(200, gin.H{"message": "密码更新成功"})
}

// --- 通用响应辅助函数 ---

type Response struct {
	State   bool        `json:"state"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error"`
	Errno   int         `json:"errno"`
}

func SuccessResp(ctx *gin.Context, data interface{}) {
	ctx.JSON(http.StatusOK, Response{
		State:   true,
		Code:    0,
		Message: "success",
		Data:    data,
		Error:   "",
		Errno:   0,
	})
}

func ErrorResp(ctx *gin.Context, httpStatus int, errMsg string) {
	ctx.JSON(httpStatus, gin.H{"error": errMsg})
}

func InfoResp(ctx *gin.Context, httpStatus int, message string, data interface{}) {
	ctx.JSON(httpStatus, Response{
		State:   true,
		Code:    0,
		Message: message,
		Data:    data,
		Error:   "",
		Errno:   0,
	})
}
