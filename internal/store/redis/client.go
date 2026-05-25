// Package redis 提供 Redis 客户端初始化和连接管理。
//
// 为什么需要 Redis：
//  1. 速率限制：多实例部署时需要共享计数器，Redis 是天然的中心化计数器
//  2. 直播观众计数：SET + TTL 自动清理死连接（后续阶段）
//  3. Token 黑名单：Refresh Token 轮换时，旧 token 加入黑名单（后续阶段）
//  4. 会话缓存：高频读取的用户信息缓存（后续阶段）
//
// 使用 go-redis v9：
//   - 支持连接池、自动重连、Pipeline
//   - 支持 Redis Cluster 和 Sentinel（v1 用单机，但接口预留了扩展空间）
//   - 所有命令都支持 context.Context（超时、取消、链路追踪）
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client 是 Redis 客户端的包装
type Client struct {
	*redis.Client
}

// Connect 创建并验证 Redis 连接
func Connect(addr, password string, db int) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:            addr,
		Password:        password,
		DB:              db,
		MinIdleConns:    5,
		ConnMaxLifetime: 30 * time.Minute,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		PoolTimeout:     4 * time.Second,
	})

	// 启动时做一次 PING 验证
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis 连接失败: %w", err)
	}
	return &Client{rdb}, nil
}
