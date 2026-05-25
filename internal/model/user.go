package model

import (
	"time"

	"github.com/google/uuid"
)

// User 是认证系统的核心数据结构,映射到数据库 users 表
type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// 角色常量 —— 避免全项目散落 magic string
// 将来新增角色只需在此文件添加常量
// 所有引用处编译期即可检查
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// RegisterRequest 是注册接口的请求体
// 使用 binding tag 做输入校验,Gin 的ShouldBindJSON 会自动执行
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=128"`
}

// LoginRequest 是登录接口的请求体
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 是登录成功后的返回体
type LoginResponse struct {
	User        User   `json:"user"`
	AccessToken string `json:"access_token"`
}

// RefreshRequest 是 token 刷新接口的请求体
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RefreshResponse 是 token 刷新后的返回体
type RefreshResponse struct {
	AccessToken string `json:"access_token"`
}
