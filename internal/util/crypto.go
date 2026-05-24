// Package util 提供全平台共享的工具函数。
//
// 本文件包含密码学相关工具：
//   - HashPassword / CheckPassword：用户密码的哈希与验证
//   - GenerateStreamKey：直播推流密钥的安全生成
package util

import (
	"crypto/rand"
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost 是 bcrypt 的计算轮数（以 2 为底的指数）
const bcryptCost = 12

// HashPassword 对明文密码进行 bcrypt 哈希
func HashPassword(plain string) (string, error) {
	if plain == "" {
		return "", errors.New("密码不能为空")
	}
	// bcrypt 自动处理密码长度截取(最大 72 字节)
	bytes, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword 验证明文密码是否匹配 bcrypt 哈希
func CheckPassword(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	return err == nil
}

// GenerateStreamKey 生成加密安全的推流密钥
func GenerateStreamKey() (string, error) {
	bytes := make([]byte, 32) // 32 字节 = 256 为熵
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
