// Package pg 提供基于 PostgreSQL + pgx 的存储层实现。
//
// 设计原则：
//  1. 所有方法接收 context.Context 作为第一个参数
//     —— 支持超时控制、请求链路追踪、优雅取消
//  2. 接口与实现分离 —— UserStore 是接口，userStore 是实现
//     —— 方便测试时 mock 整个存储层
//  3. 使用 raw SQL 而非 ORM
//     —— 避免反射开销，SQL 完全可控，便于 DBA 调优
//     —— pgx 的 prepared statement 缓存已经提供 ORM 的大部分性能优势
package pg

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maxfeizi04-cloud/magicstream/internal/model"
)

// UserStore 定义用户存储层的全部操作
type UserStore interface {
	// Create 创建新用户,通过 INSERT ... RETURNING 一次性返回完整记录
	Create(ctx context.Context, user *model.User) error

	// GetByID 通过主键查找用户
	GetByID(ctx context.Context, id uuid.UUID) (*model.User, error)

	// GetByEmail 通过邮箱查找用户
	GetByEmail(ctx context.Context, email string) (*model.User, error)

	// GetByUsername 通过用户名查找用户
	GetByUsername(ctx context.Context, username string) (*model.User, error)

	// Update 更新用户信息
	Update(ctx context.Context, user *model.User) error
}

// userStore 是 UserStore 的 pgx 实现
type userStore struct {
	pool *pgxpool.Pool
}

// NewUserStore 创建 UserStore 实例
func NewUserStore(pool *pgxpool.Pool) UserStore {
	return &userStore{pool: pool}
}

// Create 插入新用户并返回数据库生成的字段(id, create_at, update_at)
func (s *userStore) Create(ctx context.Context, user *model.User) error {
	query := `
		INSERT INTO users (username, email, password_hash, avatar_url, role) 
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
		`

	row := s.pool.QueryRow(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.AvatarURL,
		user.Role,
	)

	err := row.Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}

// GetByID 通过主键查找用户
func (s *userStore) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	query := `
		SELECT id, username, password_hash, avatar_url, role, created_at, updated_at FROM users WHERE id = $1
		FROM users
		WHERE id = $1
`
	user := &model.User{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// GetByEmail 通过邮箱查找用户
func (s *userStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SElECT id, username, email, password_hash, avatar_url, role, created_at, updated_at 
		FROM users
		WHERE email = $1`

	user := &model.User{}
	err := s.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// GetByUsername 通过用户名查找用户
func (s *userStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `
		SELECT id, username, email, password_hash, avatar_url, role, created_at, updated_at
		FROM users
		WHERE username = $1
		`

	user := &model.User{}
	err := s.pool.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// Update 更新用户信息
func (s *userStore) Update(ctx context.Context, user *model.User) error {
	query := `
		UPDATE users
		SET 
		    username = COALESE(NULLIF($2,''),username), 
		    email = COALESCE(NULLIF($3,''),email), 
		    password_hash = COALESCE(NULLIF($4,''),password_hash), 
		    avatar_url = COALESCE(NULLIF($5,''),avatar_url),
		    updated_at = NOW()
	    WHERE ID = $1
	    RETURNING id, username, email, password_hash, avatar_url, role, created_at, updated_at
	    `

	row := s.pool.QueryRow(ctx, query,
		user.ID,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.AvatarURL,
	)
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return err
	}
	return nil

}

// 确保 userStore 实现了 UserStore 接口。
// 这是编译期检查 —— 如果接口方法签名不匹配，编译直接报错，
// 比运行时才发现接口未满足要安全得多。
var _ UserStore = (*userStore)(nil)
