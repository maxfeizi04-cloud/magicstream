package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/maxfeizi04-cloud/magicstream/internal/model"
)

// mockUserStore 实现 pg.UserStore 接口用于测试
type mockUserStore struct {
	users  map[string]*model.User // email -> user
	byName map[string]*model.User // username -> user
	byID   map[uuid.UUID]*model.User
}

func newMockStore() *mockUserStore {
	return &mockUserStore{
		users:  make(map[string]*model.User),
		byName: make(map[string]*model.User),
		byID:   make(map[uuid.UUID]*model.User),
	}
}

func (m *mockUserStore) Create(ctx context.Context, user *model.User) error {
	user.TokenVersion = 0
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	m.users[user.Email] = user
	m.byName[user.Username] = user
	m.byID[user.ID] = user
	return nil
}

func (m *mockUserStore) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	u, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockUserStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockUserStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	u, ok := m.byName[username]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockUserStore) Update(ctx context.Context, user *model.User) error {
	return nil
}

func (m *mockUserStore) IncrementTokenVersion(ctx context.Context, id uuid.UUID) error {
	u := m.byID[id]
	if u != nil {
		u.TokenVersion++
	}
	return nil
}

func testConfig() UserServiceConfig {
	return UserServiceConfig{
		AccessSecret:  "test-access-secret-32-bytes-long!",
		RefreshSecret: "test-refresh-secret-32-bytes!!",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    7 * 24 * time.Hour,
	}
}

func TestRegister(t *testing.T) {
	svc := NewUserService(newMockStore(), testConfig())
	ctx := context.Background()

	t.Run("成功注册", func(t *testing.T) {
		user, err := svc.Register(ctx, "alice", "alice@test.com", "password123")
		if err != nil {
			t.Fatalf("注册失败: %v", err)
		}
		if user.ID == uuid.Nil {
			t.Fatal("用户 ID 为零值")
		}
		if user.Username != "alice" {
			t.Fatalf("用户名不匹配: %s", user.Username)
		}
		if user.Role != model.RoleUser {
			t.Fatalf("默认角色应为 user: %s", user.Role)
		}
		if user.PasswordHash == "" {
			t.Fatal("密码哈希为空")
		}
		if user.PasswordHash == "password123" {
			t.Fatal("密码哈希不应等于明文")
		}
	})

	t.Run("用户名已存在", func(t *testing.T) {
		_, err := svc.Register(ctx, "alice", "another@test.com", "password123")
		if err != ErrUsernameAlreadyExists {
			t.Fatalf("应返回 ErrUsernameAlreadyExists, got: %v", err)
		}
	})

	t.Run("邮箱已存在", func(t *testing.T) {
		_, err := svc.Register(ctx, "bob", "alice@test.com", "password123")
		if err != ErrEmailAlreadyExists {
			t.Fatalf("应返回 ErrEmailAlreadyExists, got: %v", err)
		}
	})
}

func TestLogin(t *testing.T) {
	svc := NewUserService(newMockStore(), testConfig())
	ctx := context.Background()

	// 先注册一个用户
	svc.Register(ctx, "charlie", "charlie@test.com", "correct-password")

	t.Run("成功登录", func(t *testing.T) {
		user, pair, err := svc.Login(ctx, "charlie@test.com", "correct-password")
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}
		if user == nil {
			t.Fatal("user 为 nil")
		}
		if pair.AccessToken == "" {
			t.Fatal("access token 为空")
		}
		if pair.RefreshToken == "" {
			t.Fatal("refresh token 为空")
		}
		if pair.AccessExp.Before(time.Now()) {
			t.Fatal("access token 已过期")
		}
		if pair.RefreshExp.Before(pair.AccessExp) {
			t.Fatal("refresh token 应比 access token 更晚过期")
		}
	})

	t.Run("密码错误", func(t *testing.T) {
		_, _, err := svc.Login(ctx, "charlie@test.com", "wrong-password")
		if err != ErrInvalidCredentials {
			t.Fatalf("应返回 ErrInvalidCredentials, got: %v", err)
		}
	})

	t.Run("用户不存在", func(t *testing.T) {
		_, _, err := svc.Login(ctx, "nonexist@test.com", "anything")
		if err != ErrInvalidCredentials {
			t.Fatalf("应返回 ErrInvalidCredentials(不暴露用户不存在), got: %v", err)
		}
	})
}

func TestRefreshTokenRotation(t *testing.T) {
	svc := NewUserService(newMockStore(), testConfig())
	ctx := context.Background()

	svc.Register(ctx, "dave", "dave@test.com", "password123")
	_, pair, _ := svc.Login(ctx, "dave@test.com", "password123")

	// 第一次刷新
	pair2, err := svc.RefreshToken(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("首次刷新失败: %v", err)
	}
	if pair2.RefreshToken == pair.RefreshToken {
		t.Fatal("刷新后 token 应改变")
	}

	// 复用旧 token → 应被拒绝
	_, err = svc.RefreshToken(ctx, pair.RefreshToken)
	if err != ErrTokenReused {
		t.Fatalf("旧 token 应返回 ErrTokenReused, got: %v", err)
	}

	// 新 token 仍有效
	pair3, err := svc.RefreshToken(ctx, pair2.RefreshToken)
	if err != nil {
		t.Fatalf("使用新 token 刷新失败: %v", err)
	}
	if pair3.RefreshToken == pair2.RefreshToken {
		t.Fatal("每次刷新 token 应轮换")
	}
}

func TestGetUserByID(t *testing.T) {
	svc := NewUserService(newMockStore(), testConfig())
	ctx := context.Background()

	user, _ := svc.Register(ctx, "eve", "eve@test.com", "password123")

	t.Run("存在的用户", func(t *testing.T) {
		found, err := svc.GetUserByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if found == nil {
			t.Fatal("user 为 nil")
		}
		if found.Email != "eve@test.com" {
			t.Fatalf("email 不匹配: %s", found.Email)
		}
	})

	t.Run("不存在的用户", func(t *testing.T) {
		found, err := svc.GetUserByID(ctx, uuid.New())
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if found != nil {
			t.Fatal("不存在的用户应返回 nil")
		}
	})
}
