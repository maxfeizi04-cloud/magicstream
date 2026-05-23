package pg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate 读取并执行 schema.sql 中的 DDL 语句
func Migrate(ctx context.Context, pool *pgxpool.Pool, schemaPath string) error {
	// --- 读取 SQL 文件 ---
	sqlBytes, err := os.ReadFile(filepath.Clean(schemaPath))
	if err != nil {
		absPath, _ := filepath.Abs(schemaPath)
		return fmt.Errorf("读取 Schema 文件失败: %s (绝对路径: %s) :%w", schemaPath, absPath, err)
	}

	sql := string(sqlBytes)
	if len(sql) == 0 {
		return fmt.Errorf("schema 文件 %s 内容为空", schemaPath)
	}

	// --- 执行 SQL ---
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("执行 schema 迁移失败: %w", err)
	}
	return nil
}
