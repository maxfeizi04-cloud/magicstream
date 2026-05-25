package util

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAccessToken(t *testing.T) {
	userID := uuid.New()
	secret := "test-access-secret"
	ttl := 15 * time.Minute

	token, exp, err := GenerateAccessToken(userID, "user", secret, ttl)
	if err != nil {
		t.Fatalf("GenerateAccessToken 失败: %v", err)
	}
	if token == "" {
		t.Fatal("token 为空")
	}
	if exp.Before(time.Now()) {
		t.Fatal("过期时间在过去")
	}
	if exp.After(time.Now().Add(ttl + time.Second)) {
		t.Fatal("过期时间超出预期")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	userID := uuid.New()
	secret := "test-refresh-secret"
	ttl := 7 * 24 * time.Hour
	version := 0

	token, exp, err := GenerateRefreshToken(userID, version, secret, ttl)
	if err != nil {
		t.Fatalf("GenerateRefreshToken 失败: %v", err)
	}
	if token == "" {
		t.Fatal("token 为空")
	}
	if exp.Before(time.Now()) {
		t.Fatal("过期时间在过去")
	}
}

func TestValidateAccessToken(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"
	ttl := 15 * time.Minute

	token, _, _ := GenerateAccessToken(userID, "admin", secret, ttl)

	t.Run("有效token", func(t *testing.T) {
		claims, err := ValidateAccessToken(token, secret)
		if err != nil {
			t.Fatalf("有效token验证失败: %v", err)
		}
		if claims.UserID != userID.String() {
			t.Fatalf("UserID 不匹配: got %s, want %s", claims.UserID, userID.String())
		}
		if claims.Role != "admin" {
			t.Fatalf("Role 不匹配: got %s, want admin", claims.Role)
		}
	})

	t.Run("错误的secret", func(t *testing.T) {
		_, err := ValidateAccessToken(token, "wrong-secret")
		if err == nil {
			t.Fatal("错误 secret 应验证失败")
		}
	})

	t.Run("空token", func(t *testing.T) {
		_, err := ValidateAccessToken("", secret)
		if err == nil {
			t.Fatal("空 token 应验证失败")
		}
	})
}

func TestRefreshTokenCarriesVersion(t *testing.T) {
	userID := uuid.New()
	secret := "test-refresh-secret"
	ttl := 7 * 24 * time.Hour

	token, _, _ := GenerateRefreshToken(userID, 3, secret, ttl)

	claims, err := ValidateRefreshToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateRefreshToken 失败: %v", err)
	}
	if claims.TokenVersion != 3 {
		t.Fatalf("TokenVersion 不匹配: got %d, want 3", claims.TokenVersion)
	}
}

func TestTokenExpiry(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	token, _, _ := GenerateAccessToken(userID, "user", secret, -1*time.Second)

	_, err := ValidateAccessToken(token, secret)
	if err == nil {
		t.Fatal("过期 token 应验证失败")
	}
}

func TestTokenRejectsNoneAlg(t *testing.T) {
	// 构造一个 alg=none 的 JWT（无签名）
	// 验证我们的 validateToken 会拒绝它
	_, err := validateToken("eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ1aWQiOiJ0ZXN0In0.", "any-secret")
	if err == nil {
		t.Fatal("alg=none 攻击应被拒绝")
	}
}

func TestTokenPairExpiry(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	t.Run("access token 过期时间正确", func(t *testing.T) {
		ttl := 15 * time.Minute
		_, exp, err := GenerateAccessToken(userID, "user", secret, ttl)
		if err != nil {
			t.Fatalf("生成失败: %v", err)
		}
		diff := time.Until(exp)
		if diff < 14*time.Minute || diff > 16*time.Minute {
			t.Fatalf("过期时间偏差过大: %v", diff)
		}
	})

	t.Run("refresh token 不带 role", func(t *testing.T) {
		token, _, _ := GenerateRefreshToken(userID, 0, secret, 1*time.Hour)
		claims, err := ValidateRefreshToken(token, secret)
		if err != nil {
			t.Fatalf("验证失败: %v", err)
		}
		if claims.Role != "" {
			t.Fatalf("refresh token 不应携带 role, got: %s", claims.Role)
		}
	})
}
