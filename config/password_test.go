package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBcryptHash(t *testing.T) {
	// 真实 bcrypt 哈希示例 (cost 10, "password" 的哈希)
	realHash, _ := HashPassword("password")
	assert.Equal(t, 60, len(realHash), "bcrypt 哈希应为 60 字符")
	assert.True(t, IsBcryptHash(realHash), "真实 bcrypt 哈希应被识别")

	cases := []struct {
		input    string
		expected bool
	}{
		// 正例
		{"$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"$2b$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"$2y$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		// 负例: 长度不对
		{"$2a$10$short", false},
		{"$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhW", false},  // 59
		{"$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWyx", false}, // 61
		// 负例: 前缀不对
		{"$2x$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", false},
		{"$3a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", false},
		// 负例: 明文
		{"my-secret-password", false},
		{"", false},
		// 负例: env ref
		{"${MYSSHW_PWD_x}", false},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			assert.Equal(t, c.expected, IsBcryptHash(c.input), "input=%q", c.input)
		})
	}
}

func TestIsEnvReference(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"${MYSSHW_PWD_x}", true},
		{"${A}", true},
		{"${}", true}, // 空也算 (由 ResolveEnvRef 后续处理)
		{"plain-text", false},
		{"$2a$10$xxx", false},
		{"${unclosed", false},
		{"closed}", false},
		{"", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.expected, IsEnvReference(c.input), "input=%q", c.input)
	}
}

func TestHashPasswordAndVerify(t *testing.T) {
	plain := "my-secret-密码-🔐" // 测试非 ASCII + emoji

	hash, err := HashPassword(plain)
	assert.NoError(t, err)
	assert.Equal(t, 60, len(hash))
	assert.True(t, IsBcryptHash(hash), "HashPassword 返回值应通过 IsBcryptHash")

	// 正确密码
	err = VerifyPassword(hash, plain)
	assert.NoError(t, err)

	// 错误密码
	err = VerifyPassword(hash, "wrong-password")
	assert.Error(t, err)
}

func TestHashPassword_DifferentSalts(t *testing.T) {
	// 相同明文, 不同次调用 → 不同哈希 (bcrypt 自带随机 salt)
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	assert.NotEqual(t, h1, h2, "bcrypt 应每次生成不同 salt")
	assert.True(t, IsBcryptHash(h1))
	assert.True(t, IsBcryptHash(h2))

	// 但都应能验证通过
	assert.NoError(t, VerifyPassword(h1, "same"))
	assert.NoError(t, VerifyPassword(h2, "same"))
}
