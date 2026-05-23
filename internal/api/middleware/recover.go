package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
	"github.com/rs/zerolog"
)

// Recover 是一个 Gin 中间件，捕获 handler 中发生的 panic
// 记录完整堆栈，返回 500 给客户端，而不是让整个进程崩溃
func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		// defer 保证这个函数在 Recover 返回前一定执行
		defer func() {
			if r := recover(); r != nil {
				// 检查是否客户端已断开
				if c.Request.Context().Err() != nil {
					util.Logger.Warn().
						Str("method", c.Request.Method).
						Str("path", c.Request.URL.Path).
						Interface("panic_value", r).
						Msg("Panic 发生但客户端已断开连接")
					// 不需要 c.About() -- 连接已断开
					return
				}

				// 获取完整堆栈信息
				stack := debug.Stack()

				// 记录 Error 级别日志
				logger := util.Logger
				if l, exists := c.Get("logger"); exists {
					if zl, ok := l.(zerolog.Logger); ok {
						logger = zl
					}
				}
				// 如果有 request_id,加上去
				if rid, exists := c.Get("request_id"); exists {
					if ridStr, ok := rid.(string); ok {
						logger = logger.With().Str("request_id", ridStr).Logger()
					}
				}

				logger.Error().
					Str("method", c.Request.Method).
					Str("path", c.Request.URL.Path).
					Interface("panic_value", r).
					Str("stack", string(stack)).
					Msg("HTTP handler 中发生 panic,已恢复")

				// 返回 500 给客户端
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "服务器内部错误",
				})
			}
		}()
		// c.Next() 执行后续中间件和 handler
		// 如果 handler 中发生 panic，Go runtime 会沿调用栈寻找 defer/recover
		// 上面 defer 中的 recover() 就会捕获它
		c.Next()
	}
}
