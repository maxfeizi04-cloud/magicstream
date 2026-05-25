package util

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	t.Run("正常密码", func(t *testing.T) {
		hash, err := HashPassword("mysecret123")
		if err != nil {
			t.Fatalf("HashPassword 失败: %v", err)
		}
		if hash == "" {
			t.Fatal("hash 为空")
		}
		if hash == "mysecret123" {
			t.Fatal("hash 不应等于明文")
		}
		// bcrypt hash 以 $2a$ 开头
		if len(hash) < 4 || hash[:4] != "$2a$" {
			t.Fatalf("hash 格式异常: %s", hash[:8])
		}
	})

	t.Run("空密码", func(t *testing.T) {
		_, err := HashPassword("")
		if err == nil {
			t.Fatal("空密码应返回错误")
		}
	})
}

func TestCheckPassword(t *testing.T) {
	hash, _ := HashPassword("correct")

	t.Run("正确密码", func(t *testing.T) {
		if !CheckPassword(hash, "correct") {
			t.Fatal("正确密码应验证通过")
		}
	})

	t.Run("错误密码", func(t *testing.T) {
		if CheckPassword(hash, "wrong") {
			t.Fatal("错误密码不应验证通过")
		}
	})

	t.Run("空hash或空明文", func(t *testing.T) {
		if CheckPassword("", "anything") {
			t.Fatal("空hash应返回false")
		}
		if CheckPassword(hash, "") {
			t.Fatal("空明文应返回false")
		}
	})
}

func TestGenerateStreamKey(t *testing.T) {
	key1, err := GenerateStreamKey()
	if err != nil {
		t.Fatalf("GenerateStreamKey 失败: %v", err)
	}
	if len(key1) != 64 { // 32 bytes hex = 64 chars
		t.Fatalf("key 长度应为 64，实际 %d", len(key1))
	}

	key2, _ := GenerateStreamKey()
	if key1 == key2 {
		t.Fatal("两次生成的 key 不应相同")
	}
}
