// Package main — MagicStream 启动入口
//
// 进程生命周期：
//  1. 加载配置（config.yaml + 环境变量覆盖）
//  2. 初始化基础设施（PostgreSQL 连接池、Redis 客户端）
//  3. 创建存储层（UserStore）
//  4. 创建服务层（UserService）
//  5. 创建 Handler（AuthHandler）
//  6. 注册路由（SetupRouter）
//  7. 启动 HTTP 服务器，监听信号
//  8. 收到 SIGINT/SIGTERM → 优雅关闭
//
// 依赖注入方向：main → infra → store → service → handler → router
// 每一层只依赖下一层的接口，不依赖具体实现
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maxfeizi04-cloud/magicstream/internal/api"
	"github.com/maxfeizi04-cloud/magicstream/internal/api/handler"
	"github.com/maxfeizi04-cloud/magicstream/internal/service"
	"github.com/maxfeizi04-cloud/magicstream/internal/store/pg"
	magicredis "github.com/maxfeizi04-cloud/magicstream/internal/store/redis"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
)

func main() {
	util.InitLogger(true)
	// 1. 加载配置
	cfg, err := util.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 2. 初始化数据库连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbpool, err := pg.Connect(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer dbpool.Close()

	if err := pg.Migrate(ctx, dbpool, "scripts/schema.sql"); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 3. 连接 Redis
	redisClient, err := magicredis.Connect(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
	)
	if err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}
	defer redisClient.Close()

	//  3. 依赖注入: 构建调用链
	userStore := pg.NewUserStore(dbpool)

	userService := service.NewUserService(userStore, service.UserServiceConfig{
		AccessSecret:  cfg.JWT.AccessSecret,
		RefreshSecret: cfg.JWT.RefreshSecret,
		AccessTTL:     cfg.JWT.AccessTTL,
		RefreshTTL:    cfg.JWT.RefreshTTL,
	})

	authHandler := handler.NewAuthHandler(userService)

	deps := api.Dependencies{
		AccessSecret:  cfg.JWT.AccessSecret,
		RefreshSecret: cfg.JWT.RefreshSecret,
		AccessTTl:     cfg.JWT.AccessTTL,
		RefreshTTl:    cfg.JWT.RefreshTTL,
		RedisClient:   redisClient,
		UserService:   userService,
		AuthHandler:   authHandler,
	}

	// 4. 注册路由
	router := api.SetupRouter(&deps)

	// 5. 启动 HTTP 服务器
	sev := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 在 goroutine 中启动服务器, 主 goroutine 等待信号
	go func() {
		log.Printf("MagicStream HTTP 服务器启动在 : %d", cfg.Server.HTTPPort)
		if err := sev.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务器异常退出: %v", err)
		}
	}()

	// 6. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("收到信号: %v,开始优雅关闭", sig)

	// 设置关闭超时
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sev.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 服务器强制关闭: %v", err)
	}

	log.Println("MagicStream 以安全关闭")

}
