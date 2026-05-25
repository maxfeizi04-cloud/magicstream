// Package util — JWT 工具函数
package util

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims 是 JWT 的自定义负载
type Claims struct {
	jwt.RegisteredClaims
	UserID       string `json:"uid"`
	Role         string `json:"rol"`
	TokenVersion int    `json:"tv"` // 仅 refresh token 携带, 用于检测 token 复用
}

// TokenPair 是一对 access + refresh token
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	AccessExp    time.Time `json:"access_exp"` // 过期时间
	RefreshExp   time.Time `json:"refresh_exp"`
}

// GenerateAccessToken 生成短期访问令牌
// 返回: token 字符串、过期时间、错误
func GenerateAccessToken(userID uuid.UUID, role, secret string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "magicstream",
		},
		UserID: userID.String(),
		Role:   role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return tokenStr, expiresAt, nil
}

// GenerateRefreshToken 生成长期刷新令牌
// tokenVersion 用于检测 token 复用: 每次刷新时版本号 +1
func GenerateRefreshToken(userID uuid.UUID, tokenVersion int, secret string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "magicstream",
		},
		UserID:       userID.String(),
		TokenVersion: tokenVersion,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return tokenStr, expiresAt, nil
}

// ValidateAccessToken 验证访问令牌
func ValidateAccessToken(tokenStr, secret string) (*Claims, error) {
	return validateToken(tokenStr, secret)
}

// ValidateRefreshToken 验证刷新令牌
func ValidateRefreshToken(tokenStr, secret string) (*Claims, error) {
	return validateToken(tokenStr, secret)
}

// validateToken 是通用的 token 验证逻辑
func validateToken(tokenStr, secret string) (*Claims, error) {
	if tokenStr == "" {
		return nil, errors.New("token 不能为空")
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		// 显式验证签名算法，防止 alg=none 攻击
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名算法")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("token 无效")
	}
	return claims, nil

}
