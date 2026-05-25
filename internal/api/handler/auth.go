// Package handler 提供 Gin HTTP handler。
//
// handler 层的设计原则：
//
//  1. handler 只做协议转换（HTTP Request -> service 调用 -> HTTP Response）
//
//  2. handler 不包含任何业务逻辑（业务逻辑在 service 层）
//
//  3. handler 不直接操作数据库（数据库操作在 store 层）
//
//     handler 的职责：
//     - 解析请求参数（c.ShouldBindJSON）
//     - 调用 service 层
//     - 转换 service 返回值为 HTTP 响应（c.JSON、c.AbortWithStatusJSON）
//     - 日志记录（请求/响应摘要）
//
// Gin Handler 签名：func(c *gin.Context)
//
//	注意：这不是标准库的 http.Handler（签名 func(http.ResponseWriter, *http.Request)）。
//	Gin 的 c *gin.Context 同时包含了 http.ResponseWriter 和 *http.Request，
//	以及参数解析、JSON 序列化等便捷方法。
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxfeizi04-cloud/magicstream/internal/model"
	"github.com/maxfeizi04-cloud/magicstream/internal/service"
)

// AuthHandler 处理认证相关的 HTTP 请求
type AuthHandler struct {
	userService *service.UserService
}

// NewAuthHandler 创建认证 handler 实例
func NewAuthHandler(userService *service.UserService) *AuthHandler {
	return &AuthHandler{
		userService: userService,
	}
}

// HandlerRegister 处理用户注册请求
func (h *AuthHandler) HandlerRegister(c *gin.Context) {
	var req model.RegisterRequest

	// ShouldBindJSON 内部调用 json.NewDecoder + 反射 tag 校验
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "请求格式错误",
			"details": err.Error(),
		})
		return
	}

	// 调用 service 层执行业务逻辑
	user, err := h.userService.Register(
		c.Request.Context(),
		req.Username,
		req.Email,
		req.Password,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUsernameAlreadyExists):
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"error": "该用户名已被使用",
			})
		case errors.Is(err, service.ErrEmailAlreadyExists):
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"error": "该邮箱已被注册",
			})
		default:
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "服务器内部错误",
			})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// HandleLogin 处理用户登录请求
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "请求格式错误",
			"details": err.Error(),
		})
		return
	}

	user, pair, err := h.userService.Login(
		c.Request.Context(),
		req.Email,
		req.Password,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "邮箱或密码错误",
			})
		default:
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "服务器内部错误",
			})
		}
		return
	}
	maxAge := int(pair.RefreshExp.Sub(time.Now()).Seconds())
	c.SetCookie(
		"refresh_token",
		pair.RefreshToken,
		maxAge,
		"/api/v1/auth/refresh",
		"",
		false, // secure（开发环境 false，生产 true）
		true,  // httpOnly（始终 true）
	)
	// SameSite 需要通过 SetSameSite 单独设置
	c.SetSameSite(http.SameSiteLaxMode)

	c.JSON(http.StatusOK, gin.H{
		"user":         user,
		"access_token": pair.AccessToken,
		"expires_in":   maxAge,
	})
}

// HandleRefresh 处理 Token 刷新请求。
func (h *AuthHandler) HandleRefresh(c *gin.Context) {
	// 1. 优先从 httpOnly Cookie 获取 refresh token
	refreshTokenStr, err := c.Cookie("refresh_token")

	// 2.如果 Cookie 中没有，从 JSON body 获取（兼容移动端等不支持 Cookie 的客户端）
	if err != nil || refreshTokenStr == "" {
		var req model.RefreshRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "请提供 refresh_token",
			})
			return
		}
		refreshTokenStr = req.RefreshToken
	}

	// 3. 调用 service 验证并轮换 token
	pair, err := h.userService.RefreshToken(
		c.Request.Context(),
		refreshTokenStr,
	)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "登录已过期,请重新登录",
		})
		return
	}

	// 4. 设置新的 Refresh Token Cookie
	maxAge := int(pair.RefreshExp.Sub(time.Now()).Seconds())
	c.SetCookie(
		"refresh_token",
		pair.RefreshToken,
		maxAge,
		"/api/v1/auth/refresh",
		"",
		false,
		true,
	)
	c.SetSameSite(http.SameSiteLaxMode)
	c.JSON(http.StatusOK, gin.H{
		"access_token": pair.AccessToken,
		"expires_in":   maxAge,
	})
}
