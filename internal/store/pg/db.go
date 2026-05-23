package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
)

// Connect 创建 PostgreSQL 连接池并验证连通性
func Connect(ctx context.Context, cfg util.DatabaseConfig) (*pgxpool.Pool, error) {
	// --- 解析连接字符串并创建连接池配置 ---
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("解析数据库连接字符串失败: %w", err)
	}

	// --- 配置连接池参数 ---
	poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	poolConfig.MinConns = int32(cfg.MaxIdleConns)
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.HealthCheckPeriod = 30 * time.Second

	// --- 创建连接池 ---
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("创建数据库连接池失败: %w", err)
	}

	// --- 验证连通性 ---
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("Ping PostgreSQL 失败: %w", err)
	}

	return pool, nil
}
