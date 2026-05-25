package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	magicredis "github.com/maxfeizi04-cloud/magicstream/internal/store/redis"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
)

// rateLimitScript 是 Redis Lua 脚本，原子化执行：
// 1. 移除窗口外的旧记录
// 2. 统计窗口内请求数
// 3. 若未超限，添加本次请求记录并设置 key 过期时间
const rateLimitScript = `
local key = KEYS[1]
local window_start = ARGV[1]
local now = ARGV[2]
local member = ARGV[3]
local limit = tonumber(ARGV[4])
local window_ttl = ARGV[5]

redis.call("ZREMRANGEBYSCORE", key, "0", window_start)
local count = redis.call("ZCARD", key)
if count < limit then
    redis.call("ZADD", key, now, member)
    redis.call("EXPIRE", key, window_ttl)
end
return count
`

// RateLimit 创建速率限制中间件
func RateLimit(rdb *magicredis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		endpoint := c.FullPath()
		if endpoint == "" {
			endpoint = c.Request.URL.Path
		}
		key := fmt.Sprintf("ratelimit:%s:%s", clientIP, endpoint)

		now := time.Now()
		nowNano := now.UnixNano()
		windowStart := now.Add(-window).UnixNano()
		member := fmt.Sprintf("%d", nowNano)
		windowSeconds := int(window.Seconds()) + 10

		ctx := c.Request.Context()

		count, err := rdb.Eval(ctx, rateLimitScript,
			[]string{key},
			fmt.Sprintf("%d", windowStart),
			fmt.Sprintf("%d", nowNano),
			member,
			fmt.Sprintf("%d", limit),
			fmt.Sprintf("%d", windowSeconds),
		).Int64()

		if err != nil {
			util.Logger.Error().
				Err(err).
				Str("key", key).
				Str("client_ip", clientIP).
				Str("endpoint", endpoint).
				Msg("限流器 Redis 操作失败，请求被放行(fail-open)")
			c.Next()
			return
		}

		if count >= int64(limit) {
			retryAfter := int(window.Seconds())
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "请求过于频繁,请稍后再试",
				"retry_after": retryAfter,
			})
			return
		}

		c.Next()
	}
}
