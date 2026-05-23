package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxfeizi04-cloud/magicstream/internal/api/middleware"
	"github.com/maxfeizi04-cloud/magicstream/internal/store/pg"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
)

func main() {
	// 1. 初始化日志（必须放在最前面——其他组件可能依赖 Logger 记录启动信息）
	util.InitLogger(true) // true = 开发环境，彩色 ConsoleWriter 输出
	util.Logger.Info().Msg("MagicStream 启动中...")

	// 2. 加载配置
	cfg, err := util.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	// 打印非敏感的配置信息，用于启动时确认配置正确
	// 注意：不要打印 password、secret 等敏感字段
	log.Printf("配置加载成功")
	log.Printf("  HTTP 端口: %d", cfg.Server.HTTPPort)
	log.Printf("  RTMP 端口: %d", cfg.Server.RTMPPort)
	log.Printf("  数据库: %s@%s:%d/%s (sslmode=%s)",
		cfg.Database.User, cfg.Database.Host,
		cfg.Database.Port, cfg.Database.DBName,
		cfg.Database.SSLMode)
	log.Printf("  Redis: %s (db=%d)", cfg.Redis.Addr, cfg.Redis.DB)
	log.Printf("  数据目录: %s", cfg.Storage.DataDir)

	// 3. 初始化数据库连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Println("正在连接数据库...")
	pool, err := pg.Connect(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer pool.Close()
	log.Printf("数据库连接成功")

	log.Println("正在执行数据库迁移...")
	if err := pg.Migrate(ctx, pool, "scripts/schema.sql"); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}
	log.Printf("数据库迁移完成")

	// 4. 创建 Gin 引擎（使用自定义中间件）
	// r := gin.Default()
	// 中间件的注册顺序决定执行顺序（洋葱模型从外到内）：
	//   Recover（最外层）→ Logger → Auth → Handler
	//   Recover 在最外层确保它能捕获所有内层中间件和 handler 的 panic
	r := gin.New()

	// 注册全局中间件
	// 注意：Recover 写在 Logger 之前——因为 Recover 要捕获一切
	// 包括 Logger 中间件可能发生的 panic
	r.Use(middleware.Recover(), middleware.Logger())

	// 5.注册路由, 健康检查点
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	r.GET("panic-test", func(c *gin.Context) {
		panic("panic 测试")
	})

	// 6. 启动服务器
	addr := ":8080"
	log.Printf("MagicStream 服务启动,监听地址: %s", addr)
	log.Printf("健康检查: http://localhost%s/health", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
