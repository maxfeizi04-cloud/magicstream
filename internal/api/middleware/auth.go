package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
)

// contextKey 是 context.WithValue 使用的 key 类型。
//
// 使用自定义类型（非 string）的原因：
//
//	Go 的 context.Value(key) 通过 key 的 == 比较来查找值。
//	如果两个不同的包都用 string("userID") 作为 key，
//	它们的值会互相覆盖。
//	自定义类型 contextKey 即使底层值相同字符串，
//	但因为类型不同，== 比较总是 false，
//	不同包之间的 key 天然隔离。
type contextKey string

const (
	// CtxUserID 是 context 中存放用户 ID 的 key。
	CtxUserID contextKey = "userID"
	// CtxUserRole 是 context 中存放用户角色的 key。
	CtxUserRole contextKey = "userRole"
)

func AuthMiddleware(accessSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 提取 Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "缺少认证令牌",
			})
			return
		}

		// 2. 验证 Bearer 前缀
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "认证格式错误，应为 Bearer <token>",
			})
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "令牌不能为空",
			})
			return
		}

		// 3. 验证 token
		claims, err := util.ValidateAccessToken(tokenStr, accessSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "令牌无效或已过期",
			})
			return
		}

		// 4. 注入到 Go context
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, CtxUserID, claims.UserID)
		ctx = context.WithValue(ctx, CtxUserRole, claims.Role)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// RequireRole 创建角色验证中间件。
//
// 使用方式：
//
//	adminGroup := r.Group("/api/v1/admin")
//	adminGroup.Use(AuthMiddleware(secret), RequireRole("admin"))
//	{
//	    adminGroup.GET("/users", handler.ListAllUsers)
//	}
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Go context 获取角色（不是 c.Get！）
		role, ok := c.Request.Context().Value(CtxUserRole).(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "无法获取用户角色",
			})
			return
		}

		// 检查角色是否在白名单中
		for _, allowed := range roles {
			if role == allowed {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "权限不足",
		})
	}
}

// GetUserID 从 context 中提取用户 ID。
//
// 这是一个辅助函数，提供给 handler 和 service 层使用。
// 使用 context.Context（而非 *gin.Context），使 service 层无需依赖 Gin 框架。
func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(CtxUserID).(string)
	return userID, ok
}

// GetUserRole 从 context 中提取用户角色。
func GetUserRole(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(CtxUserRole).(string)
	return role, ok
}
