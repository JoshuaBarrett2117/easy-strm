package main

import (
	"encoding/json"

	"easy-strm/internal/dao"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware 处理跨域请求的中间件
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// JSONEscapeMiddleware JSON转义中间件，禁用Unicode转义
func JSONEscapeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 使用自定义的JSON响应方法
		c.Next()
	}
}

// JSON gin.Context的JSON响应辅助函数，禁用Unicode转义
func JSON(c *gin.Context, code int, obj interface{}) {
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(code)
	encoder := json.NewEncoder(c.Writer)
	encoder.SetEscapeHTML(false)
	encoder.Encode(obj)
}

func main() {
	// 初始化Windows控制台UTF-8编码
	initWindowsConsole()

	// 加载配置
	config := LoadConfig()

	// 设置gin为debug模式
	gin.SetMode(gin.DebugMode)
	// 创建gin引擎
	r := gin.Default()

	// 添加CORS中间件
	r.Use(CORSMiddleware())

	// 初始化日志系统
	InitLogger(config)
	Info("Loaded configuration successfully")

	// 创建115 open客户端
	client := NewClient(config)
	Info("Created 115 open client")

	// 初始化数据库连接
	err := InitDB(config)
	if err != nil {
		Error("Failed to initialize database: %v", err)
		return
	}
	Info("Database connected successfully")

	// 初始化Redis连接
	err = InitRedis(config)
	if err != nil {
		Error("Failed to initialize Redis: %v", err)
		return
	}
	Info("Redis connected successfully")

	// 迁移旧的Redis key到新格式
	MigrateRedisKeys()

	// 确保除了admin用户存在
	if err := SeedAdmin(); err != nil {
		Error("Failed to seed admin user: %v", err)
	}

	// 初始化 DAO 层（在所有路由设置之前）
	dao.Init(getDBInstance(), getRedisClientInstance())
	dao.InitDAO(getDBInstance())

	// 初始化cron调度器
	if err := InitCronScheduler(); err != nil {
		Error("Failed to initialize cron scheduler: %v", err)
	}

	// 设置认证相关路由（登录等）
	SetupAuthRoutes(r, config, client)

	// 设置需要认证的路由（所有业务API）
	SetupAuthProtectedRoutes(r, config, client)

	// 使用RouterSetup注册需要认证的路由（包含正确的transfer实现和所有业务API）
	// rs := router.NewRouterSetup(r)
	// rs.InitDAO(GetDB(), GetRedisClient())
	// authService, _, _, _ := rs.InitServices(config.JWTSecret)
	// rs.SetupRoutes(authService)

	// 启动服务器
	Info("Gin server starting on http://localhost:8082")
	r.Run(":8082")
}

// GetDB 获取全局数据库连接（供 router 使用）
func GetDB() interface{} {
	return getDBInstance()
}

// GetRedisClient 获取全局Redis客户端（供 router 使用）
func GetRedisClient() interface{} {
	return getRedisClientInstance()
}

// 需要在 db.go 或 redis.go 中实现这些函数
// 由于 router 包需要 interface{} 类型，需要提供适配
