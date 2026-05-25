package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	magicredis "github.com/maxfeizi04-cloud/magicstream/internal/store/redis"
)

// RateLimit 创建速率限制中间件
func RateLimit(rdb *magicredis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 构造限流 key
		clientIP := c.ClientIP()
		endpoint := c.FullPath() // Gin 注册时的路径模板，如 "/api/v1/auth/login"
		if endpoint == "" {
			endpoint = c.Request.URL.Path
		}
		key := fmt.Sprintf("ratelimit:%s:%s", clientIP, endpoint)

		now := time.Now()
		nowNano := now.UnixNano()
		windowStart := now.Add(-window).UnixNano()

		// 生成唯一 member ID
		member := fmt.Sprintf("%d", nowNano)

		ctx := c.Request.Context()

		// 1. 移除窗口外的旧记录
		if err := rdb.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart)).Err(); err != nil {
			c.Next()
			return
		}

		// 2. 统计窗口内的请求数
		count, err := rdb.ZCard(ctx, key).Result()
		if err != nil {
			c.Next()
			return
		}

		// 3. 超时检查
		if count >= int64(limit) {
			retryAfter := int(window.Seconds())
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "请求过于频繁,请稍后再试",
				"retry_after": retryAfter,
			})
			return
		}

		// 4. 记录本次请求
		pipe := rdb.Pipeline()
		pipe.ZAdd(ctx, key, redis.Z{
			Score:  float64(nowNano),
			Member: member,
		})

		// EXPIRE 设置 key 的过期时间
		pipe.Expire(ctx, key, window+10*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			c.Next()
		}
		c.Next()

	}
}
