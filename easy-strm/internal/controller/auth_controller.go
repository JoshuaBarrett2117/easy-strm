package controller

import (
	"fmt"
	"net/http"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthController struct {
	authService *service.AuthService
	redisClient interface {
		Set(key string, value interface{}, expiration int) error
		Get(key string) (string, error)
		Del(key string) error
	}
}

func NewAuthController(authService *service.AuthService) *AuthController {
	return &AuthController{
		authService: authService,
	}
}

func (c *AuthController) SetRedisClient(client interface {
	Set(key string, value interface{}, expiration int) error
	Get(key string) (string, error)
	Del(key string) error
}) {
	c.redisClient = client
}

func (c *AuthController) Login(ctx *gin.Context) {
	var loginData struct {
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&loginData); err != nil {
		logger.Warnf("AuthController[Login] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	user, token, err := c.authService.Login(loginData.Name, loginData.Password)
	if err != nil {
		logger.Warnf("AuthController[Login] 登录失败: %s, error: %v", loginData.Name, err)
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	if c.redisClient != nil {
		if err := c.redisClient.Set(fmt.Sprintf("user:token:%d", user.ID), token, 0); err != nil {
			logger.Errorf("AuthController[Login] 保存token到Redis失败: %v", err)
		}
	}

	logger.Infof("AuthController[Login] 用户登录成功: %s", loginData.Name)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "登录成功",
		"token":   token,
		"user_id": user.ID,
		"name":    user.Name,
	})
}

func (c *AuthController) GetUserInfo(ctx *gin.Context) {
	userID, exists := ctx.Get("userID")
	if !exists {
		logger.Error("AuthController[GetUserInfo] 无法获取用户ID")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "无法获取用户信息"})
		return
	}

	typedUserID, ok := userID.(int64)
	if !ok {
		logger.Errorf("AuthController[GetUserInfo] userID类型错误: %T, value: %v", userID, userID)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("userID类型错误: %T", userID)})
		return
	}

	logger.Warnf("AuthController[GetUserInfo] 开始获取用户信息, typedUserID: %d", typedUserID)

	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("AuthController[GetUserInfo] 发生panic: %v", r)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("内部错误: %v", r)})
		}
	}()

	user, err := c.authService.GetUserByID(int(typedUserID))
	logger.Warnf("AuthController[GetUserInfo] GetUserByID返回, user: %v, err: %v", user, err)
	if err != nil {
		logger.Errorf("AuthController[GetUserInfo] 获取用户信息失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取用户信息失败: %v", err)})
		return
	}
	if user == nil {
		logger.Errorf("AuthController[GetUserInfo] 用户不存在, typedUserID: %d", typedUserID)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":          user.ID,
			"name":        user.Name,
			"create_time": user.CreateTime.Format("2006-01-02 15:04:05"),
			"update_time": user.UpdateTime.Format("2006-01-02 15:04:05"),
		},
	})
}

func (c *AuthController) JWTMiddleware(secret string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
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

		claims, err := c.authService.VerifyToken(tokenString)
		if err != nil {
			logger.Warnf("AuthController[JWTMiddleware] Token验证失败: %v", err)
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "无效的token"})
			ctx.Abort()
			return
		}

		if c.redisClient != nil {
			tokenInRedis, err := c.redisClient.Get(fmt.Sprintf("user:token:%d", claims.UserID))
			if err != nil || tokenInRedis != tokenString {
				logger.Warnf("AuthController[JWTMiddleware] Redis中token不匹配")
				ctx.JSON(http.StatusUnauthorized, gin.H{"error": "无效的token"})
				ctx.Abort()
				return
			}
		}

		ctx.Set("userID", claims.UserID)
		ctx.Next()
	}
}

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