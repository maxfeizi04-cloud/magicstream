// Package api 提供 HTTP 路由注册和中间件编排。
//
// Gin 路由的关键概念：
//
//  1. 路由分组 (RouterGroup)：
//     r.Group("/api/v1") 创建一个路径前缀为 /api/v1 的路由组。
//     组内所有路由自动继承此前缀，避免重复写路径。
//
//  2. 组级中间件：
//     group.Use(middleware) 让中间件只对该组内的路由生效。
//     Gin 的中间件执行顺序是洋葱模型：
//     Request → Recover → Logger → Auth → RateLimit → Handler → Response
//     后注册的中间件在内层，先执行 handler，然后反向执行。
//
//  3. Gin 路由方法命名：
//     Gin 使用大写首字母：r.GET, r.POST, r.PUT, r.DELETE
//     （chi 和其他路由器使用小写：r.Get, r.Post）
//
//  4. 路径参数：
//     :id —— 命名参数，通过 c.Param("id") 获取
//     *filepath —— 通配参数，通过 c.Param("filepath") 获取
package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxfeizi04-cloud/magicstream/internal/api/handler"
	"github.com/maxfeizi04-cloud/magicstream/internal/api/middleware"
	"github.com/maxfeizi04-cloud/magicstream/internal/service"
	magicredis "github.com/maxfeizi04-cloud/magicstream/internal/store/redis"
)

// Dependencies 是所有 handler 和中间件依赖项的集合。
//
// 集中管理依赖注入，避免在 router.go 中散落配置参数
// 将来新增 handler 只需在此结构体中添加字段
type Dependencies struct {
	// Config 相关
	AccessSecret  string
	RefreshSecret string
	AccessTTl     time.Duration
	RefreshTTl    time.Duration

	// Infrastructure
	RedisClient *magicredis.Client

	// Service
	UserService  *service.UserService
	VideoService *service.VideoService

	// Handlers
	AuthHandler  *handler.AuthHandler
	VideoHandler *handler.VideoHandler
}

// SetupRouter 创建并配置 Gin 路由引擎
func SetupRouter(deps *Dependencies) *gin.Engine {
	// 使用 gin.New() 而非 gin.Default()
	// 因为 gin.Default() 自带 Logger 和 Recovery 中间件
	r := gin.New()

	// --- 全局中间件 ---
	r.Use(middleware.Recover())
	r.Use(middleware.Logger())

	// --- 健康检查 ---
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// --- 公开路由: 认证接口 ---
	authGroup := r.Group("/api/v1/auth")

	// 注册接口 (3次/分钟，防止批量注册垃圾账户)
	authGroup.POST("/register",
		middleware.RateLimit(deps.RedisClient, 3, 1*time.Minute),
		deps.AuthHandler.HandlerRegister,
	)

	// 登录接口 (5次/分钟，防止暴力破解)
	authGroup.POST("/login",
		middleware.RateLimit(deps.RedisClient, 5, 1*time.Minute),
		deps.AuthHandler.HandleLogin,
	)

	// Refresh 接口
	authGroup.POST("/refresh", deps.AuthHandler.HandleRefresh)

	//--- 需认证路由组 ---
	protected := r.Group("/api/v1/")
	protected.Use(middleware.AuthMiddleware(deps.AccessSecret))
	{
		// 用户相关路由
		protected.GET("/users/me", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "用户信息接口 (待实现)"})
		})
		protected.PUT("/users/me", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "更新用户信息接口 (待实现)"})

		})
		videos := protected.Group("/videos")
		{
			videos.POST("", deps.VideoHandler.HandleCreate)
			videos.GET("", deps.VideoHandler.HandleList)
			videos.GET("/:id", deps.VideoHandler.HandleGet)
			videos.PUT("/:id", deps.VideoHandler.HandleUpdate)
			videos.DELETE("/:id", deps.VideoHandler.HandleDelete)
		}

		// --- 管理员路由组 ---
		admin := protected.Group("/admin")
		admin.Use(middleware.RequireRole("admin"))
		{
			admin.GET("/users", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "管理员用户列表 (待实现)"})
			})
		}
	}
	return r
}
