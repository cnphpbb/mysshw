package config

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// 密码字段格式 (Phase 2 引入, Phase B 增强):
//   - 明文:    "my-secret-password"
//   - 环境变量: "${MYSSHW_PWD_node1}"  (由 ssh.ResolveEnvRef 处理)
//   - bcrypt 哈希: "$2a$10$..."  (60 字符, 必须 bcrypt 验证后使用)
//   - bcrypt-pepper: "$2a$10$..." (同上, 但额外需要 server 侧 secret_key)
//   - argon2id:  "$argon2id$v=19$..."  (Phase B+ 计划, 当前以标准 bcrypt 处理)
//   - pbkdf2-sha256: "$pbkdf2-sha256$..." (Phase B+ 计划)
//   - 空字符串: ""  (由 ssh 包交互提示输入)
//
// 自动检测策略 (B1 决策, Phase B 增强):
//   1. 长度 60 且以 "$2a$"/"$2b$"/"$2y$" 开头 → bcrypt 哈希 (按 Configs.EncryptionAlgorithm 决定是否需要 pepper)
//   2. 以 "${" 开头且以 "}" 结尾 → 环境变量引用 (由 ResolveEnvRef 后续处理)
//   3. 否则 → 明文 (兼容老版本)

// BcryptHashPrefixes bcrypt 哈希的有效前缀 (算法版本标识)
var BcryptHashPrefixes = []string{"$2a$", "$2b$", "$2y$"}

// BcryptHashLength 标准 bcrypt 哈希长度 (60 字符: 2a$10$[22 salt][31 hash])
const BcryptHashLength = 60

// EncryptionAlgorithm 值常量 (Phase B 引入, 严格匹配, 大小写敏感)
const (
	EncryptionAlgorithmAuto         = ""              // 默认: 兼容 v26.07.04.0 行为
	EncryptionAlgorithmBcrypt       = "bcrypt"        // 标准 bcrypt, 无 pepper
	EncryptionAlgorithmBcryptPepper = "bcrypt-pepper" // bcrypt + secret_key
	EncryptionAlgorithmArgon2id     = "argon2id"      // Phase B+ 计划
	EncryptionAlgorithmPbkdf2Sha256 = "pbkdf2-sha256" // Phase B+ 计划
)

// IsBcryptHash 判断字符串是否为有效 bcrypt 哈希
// 规则: 必须以 $2a$/$2b$/$2y$ 之一开头 + 长度恰好 60
func IsBcryptHash(s string) bool {
	if len(s) != BcryptHashLength {
		return false
	}
	for _, prefix := range BcryptHashPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// IsEnvReference 判断是否为 ${...} 环境变量引用
func IsEnvReference(s string) bool {
	return strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}")
}

// IsArgon2idHash 判断字符串是否为 Argon2id 哈希 (Phase B+ 占位识别)
// 格式: $argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>
func IsArgon2idHash(s string) bool {
	return strings.HasPrefix(s, "$argon2id$")
}

// IsPbkdf2Sha256Hash 判断字符串是否为 PBKDF2-SHA256 哈希 (Phase B+ 占位识别)
// 格式: $pbkdf2-sha256$iter$salt$hash
func IsPbkdf2Sha256Hash(s string) bool {
	return strings.HasPrefix(s, "$pbkdf2-sha256$")
}

// IsPepperRequired 判断当前 encryption_algorithm 是否需要 secret_key
// 返回 true: "bcrypt-pepper" 节点必须有 secret_key
func IsPepperRequired(algorithm string) bool {
	return algorithm == EncryptionAlgorithmBcryptPepper
}

// HashPassword 用 bcrypt 哈希明文密码
// cost 使用 bcrypt.DefaultCost (10), ~60ms 单次哈希 (足够慢以抗暴力)
// 返回: 完整 bcrypt 哈希字符串 (60 字符), 可直接存入 TOML
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword 验证明文密码是否与 bcrypt 哈希匹配
// 返回 nil 表示匹配, ErrMismatchedHashAndPassword 表示不匹配
func VerifyPassword(hash, plaintext string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
}

// HashPasswordWithPepper 用 bcrypt + server 侧 pepper 哈希明文密码 (Phase B 新增)
//
// 机制:
//   - 计算 bcrypt(pepper + ":" + plaintext) 作为存储哈希
//   - pepper 是 server 侧静态密钥 (推荐从 K8s Secret 注入, 不与 hash 同址存储)
//   - 验证时也需要提供相同 pepper
//
// 优势:
//   - 配置文件泄露 → 攻击者拿到 hash 但没有 pepper, 无法离线爆破
//   - 与明文 / bcrypt 互不兼容 (混合部署时需逐节点 rehash)
//
// 限制:
//   - pepper 不能为空 (否则退化为标准 bcrypt, 失去保护)
//   - pepper 一旦变更, 所有节点都需要 rehash (提供 mysshw hash gen --with-pepper 工具)
//
// 返回: 完整 bcrypt 哈希字符串 (60 字符)
func HashPasswordWithPepper(plaintext, pepper string) (string, error) {
	if pepper == "" {
		return "", errors.New("mysshw:: pepper cannot be empty for bcrypt-pepper mode")
	}
	// pepper 与明文之间用 ":" 分隔, 避免 pepper="abc" plaintext="def" 与
	// pepper="ab" plaintext="cdef" 的哈希冲突
	mixed := pepper + ":" + plaintext
	return HashPassword(mixed)
}

// VerifyPasswordWithPepper 验证明文密码是否与 bcrypt-pepper 哈希匹配
//
// 返回:
//
//	nil                         → 匹配
//	ErrMismatchedHashAndPassword → 不匹配
//	其他 error                  → pepper 格式错误等
func VerifyPasswordWithPepper(hash, plaintext, pepper string) error {
	if pepper == "" {
		return errors.New("mysshw:: pepper cannot be empty for bcrypt-pepper mode")
	}
	mixed := pepper + ":" + plaintext
	return VerifyPassword(hash, mixed)
}

// ResolveEncryptionAlgorithm 解析 encryption_algorithm 字段, 允许大小写无关匹配
// 返回: 标准化的算法常量, 或 error (未知算法)
func ResolveEncryptionAlgorithm(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return EncryptionAlgorithmAuto, nil
	case "bcrypt":
		return EncryptionAlgorithmBcrypt, nil
	case "bcrypt-pepper", "bcrypt_pepper":
		return EncryptionAlgorithmBcryptPepper, nil
	case "argon2id":
		return EncryptionAlgorithmArgon2id, nil
	case "pbkdf2-sha256", "pbkdf2_sha256":
		return EncryptionAlgorithmPbkdf2Sha256, nil
	default:
		return "", fmt.Errorf("mysshw:: unknown encryption_algorithm %q (supported: bcrypt, bcrypt-pepper, argon2id, pbkdf2-sha256)", s)
	}
}
