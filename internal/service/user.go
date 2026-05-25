// Package service 是业务逻辑层。
//
// service 层的职责：
//  1. 编排多个 store 操作（如注册 = 查重 + 哈希 + 创建）
//  2. 实现业务规则（如密码强度校验、用户名唯一性）
//  3. 返回业务错误（如 "用户名已存在"、"密码错误"）
//
// service 层不依赖 HTTP 框架（Gin/标准库都不应该出现在这里）。
// 所有参数通过 Go context 和普通类型传递，
// 这样 service 可以被 CLI 工具、gRPC 服务、消息队列消费者等复用。
//
// UserService 的每个方法接收 context.Context 作为第一个参数：
//   - 支持超时控制（调用方可以设置 deadline）
//   - 支持请求链路追踪（从 ctx 获取 trace ID）
//   - 支持取消传播（HTTP 连接断开时，下游的 DB 查询也能感知到并提前中止）
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/maxfeizi04-cloud/magicstream/internal/model"
	"github.com/maxfeizi04-cloud/magicstream/internal/store/pg"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
)

// 业务错误定义
// service 层返回的这些错误,handler 层将其映射为 HTTP 状态码
var (
	ErrEmailAlreadyExists    = errors.New("该邮箱已被注册")
	ErrUsernameAlreadyExists = errors.New("该用户名已被使用")
	ErrInvalidCredentials    = errors.New("邮箱或密码错误")
	ErrUserNotFound          = errors.New("用户不存在")
)

// UserServiceConfig 是 UserService 的运行时配置
// 使用值类型（非指针）传递，避免被修改
type UserServiceConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

// UserService 是用户认证的业务逻辑层
// 持有 UserStore 接口（不是具体实现）
// 方便单元测试时注入 mock store
type UserService struct {
	store  pg.UserStore
	config UserServiceConfig
}

// NewUserService 创建用户服务实例
func NewUserService(store pg.UserStore, config UserServiceConfig) *UserService {
	return &UserService{
		store:  store,
		config: config,
	}
}

// Register 注册新用户
func (s *UserService) Register(ctx context.Context, username, email, password string) (*model.User, error) {
	// 1. 检查用户名唯一性
	existing, err := s.store.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUsernameAlreadyExists
	}

	// 2. 检查邮箱唯一性
	existing, err = s.store.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailAlreadyExists
	}

	// 3. 哈希密码
	hash, err := util.HashPassword(password)
	if err != nil {
		return nil, err
	}

	// 4. 创建用户
	user := &model.User{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		Role:         model.RoleUser, // 默认角色
		AvatarURL:    "",             // 用户可在设置中上传头像
	}

	if err := s.store.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Login 用户登录,验证凭据并返回 token pair
// 返回值：
//   - user: 用户信息（不含 PasswordHash，因为 json:"-"）
//   - accessToken / refreshToken: token pair
//   - error: 业务错误
func (s *UserService) Login(ctx context.Context, email, password string) (*model.User, *util.TokenPair, error) {
	// 1. 查找用户
	user, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		// 用户不存在 — 但不告知调用方具体原因
		return nil, nil, ErrInvalidCredentials
	}

	// 2. 验证密码
	if !util.CheckPassword(user.PasswordHash, password) {
		// 密码错误 — 同样不告知具体原因
		return nil, nil, ErrInvalidCredentials
	}

	// 3. 生成 token pair
	accessToken, accessExp, err := util.GenerateAccessToken(
		user.ID, user.Role, s.config.AccessSecret, s.config.AccessTTL,
	)
	if err != nil {
		return nil, nil, err
	}

	refreshToken, refreshExp, err := util.GenerateRefreshToken(
		user.ID, s.config.RefreshSecret, s.config.RefreshTTL,
	)
	if err != nil {
		return nil, nil, err
	}

	pair := &util.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		AccessExp:    accessExp,
		RefreshExp:   refreshExp,
	}
	return user, pair, nil
}

// RefreshToken 使用 Refresh Token 换取新的 token pair
func (s *UserService) RefreshToken(ctx context.Context, refreshTokenStr string) (*util.TokenPair, error) {
	// 1. 验证旧 refresh token
	claims, err := util.ValidateRefreshToken(refreshTokenStr, s.config.RefreshSecret)
	if err != nil {
		return nil, err
	}

	// 2. 从数据库重新读取用户(获取最新角色)
	// 注意：claims.UserID 是 string，需要解析为 uuid.UUID
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	// 3. 生成新的 token pair (轮换)
	accessToken, accessExp, err := util.GenerateAccessToken(
		user.ID, user.Role, s.config.AccessSecret, s.config.AccessTTL,
	)
	if err != nil {
		return nil, err
	}

	newRefreshToken, refreshExp, err := util.GenerateRefreshToken(
		user.ID, s.config.RefreshSecret, s.config.RefreshTTL,
	)
	if err != nil {
		return nil, err
	}
	pair := &util.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		AccessExp:    accessExp,
		RefreshExp:   refreshExp,
	}
	return pair, nil
}

// GetUserByID 通过 ID 获取用户信息。
// 提供给其他 service 或 handler 使用
func (s *UserService) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return s.store.GetByID(ctx, id)
}
