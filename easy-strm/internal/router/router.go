package router

import (
	"database/sql"
	"net/http"

	"easy-strm/internal/controller"
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

type Router struct {
	engine            *gin.Engine
	authController    *controller.AuthController
	cloud115Controller *controller.Cloud115Controller
	taskController    *controller.TaskController
	strmController     *controller.StrmController
	authService       *service.AuthService
	cloud115Service   *service.Cloud115Service
	taskService       *service.TaskService
	strmService       *service.StrmService
}

func NewRouter(e *gin.Engine) *Router {
	return &Router{
		engine: e,
	}
}

func (r *Router) SetControllers(
	authController *controller.AuthController,
	cloud115Controller *controller.Cloud115Controller,
	taskController *controller.TaskController,
	strmController *controller.StrmController,
) {
	r.authController = authController
	r.cloud115Controller = cloud115Controller
	r.taskController = taskController
	r.strmController = strmController
}

func (r *Router) SetServices(
	authService *service.AuthService,
	cloud115Service *service.Cloud115Service,
	taskService *service.TaskService,
	strmService *service.StrmService,
) {
	r.authService = authService
	r.cloud115Service = cloud115Service
	r.taskService = taskService
	r.strmService = strmService
}

func (r *Router) SetScheduler(scheduler interface {
	AddTask(task interface{}) error
	RemoveTask(taskID int)
	UpdateTask(task interface{}) error
}) {
	if r.strmService != nil {
		r.strmService.SetScheduler(scheduler)
	}
}

func (r *Router) Setup() {
	r.setupAuthRoutes()
	r.setupCloud115Routes()
	r.setupStrmRoutes()
	r.setupTaskRoutes()
}

func (r *Router) setupAuthRoutes() {
	auth := r.engine.Group("/auth")
	{
		auth.POST("/login", r.authController.Login)
	}
}

func (r *Router) setupCloud115Routes() {
	auth := r.engine.Group("/auth")
	auth.Use(r.authController.JWTMiddleware(""))
	{
		auth.GET("/cloud115/list", r.cloud115Controller.GetList)
		auth.GET("/cloud115/:id", r.cloud115Controller.GetByID)
		auth.POST("/cloud115", r.cloud115Controller.Create)
		auth.PUT("/cloud115/:id", r.cloud115Controller.Update)
		auth.DELETE("/cloud115/:id", r.cloud115Controller.Delete)
		auth.GET("/cloud115/login/channels", r.cloud115Controller.GetLoginChannels)
		auth.GET("/cloud115/login/qrcode", r.cloud115Controller.GetQRCode)
		auth.GET("/cloud115/login/status", r.cloud115Controller.CheckLoginStatus)
		auth.POST("/cloud115/login/confirm", r.cloud115Controller.ConfirmLogin)
	}
}

func (r *Router) setupStrmRoutes() {
	auth := r.engine.Group("/auth")
	auth.Use(r.authController.JWTMiddleware(""))
	{
		auth.GET("/strm/config", r.strmController.GetConfigList)
		auth.GET("/strm/config/:id", r.strmController.GetConfigByID)
		auth.POST("/strm/config", r.strmController.CreateConfig)
		auth.PUT("/strm/config/:id", r.strmController.UpdateConfig)
		auth.DELETE("/strm/config/:id", r.strmController.DeleteConfig)
		auth.GET("/strm/config/:id/files", r.strmController.GetFilesByConfigID)
	}
}

func (r *Router) setupTaskRoutes() {
	auth := r.engine.Group("/auth")
	auth.Use(r.authController.JWTMiddleware(""))
	{
		auth.GET("/strm/task/all", r.taskController.GetAll)
		auth.GET("/strm/task/:task_id", r.taskController.Get)
		auth.DELETE("/strm/task/:task_id", r.taskController.Delete)
	}
}

type RouterSetup struct {
	Engine              *gin.Engine
	AuthController     *controller.AuthController
	Cloud115Controller *controller.Cloud115Controller
	TaskController     *controller.TaskController
	StrmController     *controller.StrmController
}

func NewRouterSetup(e *gin.Engine) *RouterSetup {
	return &RouterSetup{
		Engine: e,
	}
}

func (rs *RouterSetup) InitDAO(database interface{}, redis interface{}) {
	dao.Init(database.(*sql.DB), redis.(*redis.Client))
}

func (rs *RouterSetup) InitServices() (*service.AuthService, *service.Cloud115Service, *service.TaskService, *service.StrmService) {
	userDAO := dao.NewUserDAO()
	cloud115DAO := dao.NewCloud115DAO()
	strmConfigDAO := dao.NewStrmConfigDAO()
	strmFileDAO := dao.NewStrmFileDAO()
	cronTaskDAO := dao.NewCronTaskDAO()
	taskRedisDAO := dao.NewTaskRedisDAO()

	authService := service.NewAuthService(userDAO, "")
	cloud115Service := service.NewCloud115Service(cloud115DAO)
	taskService := service.NewTaskService(taskRedisDAO)
	strmService := service.NewStrmService(strmConfigDAO, strmFileDAO, cronTaskDAO)

	rs.AuthController = controller.NewAuthController(authService)
	rs.Cloud115Controller = controller.NewCloud115Controller(cloud115Service)
	rs.TaskController = controller.NewTaskController(taskService)
	rs.StrmController = controller.NewStrmController(strmService)

	return authService, cloud115Service, taskService, strmService
}

func (rs *RouterSetup) SetupRoutes(authService *service.AuthService) {
	rs.Engine.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	rs.Engine.POST("/auth/login", rs.AuthController.Login)

	auth := rs.Engine.Group("/auth")
	auth.Use(func(ctx *gin.Context) {
		token := ctx.GetHeader("Authorization")
		if token != "" {
			claims, err := authService.VerifyToken(token)
			if err == nil {
				ctx.Set("userID", claims.UserID)
			}
		}
		ctx.Next()
	})
	{
		auth.GET("/cloud115/list", rs.Cloud115Controller.GetList)
		auth.GET("/cloud115/:id", rs.Cloud115Controller.GetByID)
		auth.POST("/cloud115", rs.Cloud115Controller.Create)
		auth.PUT("/cloud115/:id", rs.Cloud115Controller.Update)
		auth.DELETE("/cloud115/:id", rs.Cloud115Controller.Delete)
		auth.GET("/strm/config", rs.StrmController.GetConfigList)
		auth.GET("/strm/config/:id", rs.StrmController.GetConfigByID)
		auth.POST("/strm/config", rs.StrmController.CreateConfig)
		auth.PUT("/strm/config/:id", rs.StrmController.UpdateConfig)
		auth.DELETE("/strm/config/:id", rs.StrmController.DeleteConfig)
		auth.GET("/strm/task/all", rs.TaskController.GetAll)
		auth.GET("/strm/task/:task_id", rs.TaskController.Get)
		auth.DELETE("/strm/task/:task_id", rs.TaskController.Delete)
	}
}

func (rs *RouterSetup) GetStrmService() *service.StrmService {
	return rs.StrmController.strmService
}
