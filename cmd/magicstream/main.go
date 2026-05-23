package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
)

func main() {
	// 1. 加载配置
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

	// 2. 初始化数据库连接

	// 3. 注册路由
	r := gin.Default()

	// 4. 健康检查点
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// 5. 启动服务器
	addr := ":8080"
	log.Printf("MagicStream 服务启动,监听地址: %s", addr)
	log.Printf("健康检查: http://localhost%s/health", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
