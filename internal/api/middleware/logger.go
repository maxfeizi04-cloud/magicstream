// Package middleware 包含 MagicStream 的 Gin 中间件集合
// logger.go 实现 HTTP 请求的结构化日志记录
package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
	"github.com/rs/zerolog"
)

// Logger 是一个 Gin 中间件，记录每个 HTTP 请求的详细信息
//
// 记录的内容：
//   - request_id：唯一请求标识，串联同一请求的所有日志
//   - method：HTTP 方法（GET/POST/PUT/DELETE）
//   - path：请求路径（如 /api/v1/videos）
//   - status：HTTP 响应状态码（200/404/500 等）
//   - duration：请求处理耗时（毫秒）
//   - remote_addr：客户端 IP 地址
//   - body_size：响应体大小（字节）
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 生成/复用 Request ID
		requestID := c.GetHeader("X-Request-Id")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// 2. 将 Request ID 注入到各处
		// HTTP 响应头 -- 让客户端也能拿到这个 ID
		c.Header("X-Request-Id", requestID)

		// Gin context —— handler 层可以通过 c.GetString("request_id") 获取
		c.Set("request_id", requestID)

		// Go context —— service/store 层可以通过 ctx.Value("request_id") 获取
		// 使用自定义类型作为 key 而不是 string，避免 context key 冲突
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, requestIDKey, requestID)
		c.Request = c.Request.WithContext(ctx)

		// 3. 创建带 request_id 的子 Logger
		reqLogger := util.WithRequestID(requestID)
		c.Set("logger", reqLogger)

		// 4. 记录请求开始
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		reqLogger.Info().
			Str("method", c.Request.Method).
			Str("path", path).
			Str("query", rawQuery).
			Str("remote_ip", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Msg("HTTP 请求开始")

		// 6. 记录请求完成
		duration := time.Since(start)
		status := c.Writer.Status()

		// 根据状态码选择日志级别：
		//   5xx → Error 级别（服务端错误，需要告警关注）
		//   4xx → Warn 级别（客户端错误，通常不需告警但值得注意）
		//   2xx/3xx → Info 级别（正常流量)
		var logEvevt *zerolog.Event
		switch {
		case status >= 200 && status < 400:
			logEvevt = reqLogger.Info()
		case status >= 400 && status < 500:
			logEvevt = reqLogger.Warn()
		case status >= 500:
			logEvevt = reqLogger.Error()
		default:
			logEvevt = reqLogger.Info()
		}
		logEvevt.
			Int("status", status).
			Dur("duration", duration).
			Int("body_size", c.Writer.Size()).
			Msg("HTTP 请求完成")
	}
}

// requestIDKey 是 context 中 request_id 的 key 类型。
// 使用自定义类型（而非 string）防止不同包之间的 context key 冲突。
// 如果两个包都用 string "request_id" 作为 key，后写入的会覆盖先写入的
type requestIDKeyType string

const requestIDKey requestIDKeyType = "request_id"
