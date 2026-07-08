package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/argon2"
)

// ============================================================
// 文件级加密 (Phase C 实现)
//
// 用途: 加密整个 mysshw.toml, 防止配置文件泄露后明文密码 / keypath 被读取
// 注意: 这是 OPTIONAL 功能, 默认配置文件不加密 (兼容 v26.07.04.0 行为)
//
// 格式 (二进制, 小端):
//   +--------+--------+----------+----------+----------+
//   | magic  | kdf_id | salt     | nonce    | ct_len   | ciphertext
//   | 4B     | 1B     | 16B      | 12B      | 4B       | N bytes
//   +--------+--------+----------+----------+----------+
//   magic   = "MSEC" (4 bytes, mysshw Encrypted Config)
//   kdf_id  = 0x01 (Argon2id) | 0xFF (未知 / 错误)
//   salt    = Argon2id salt (16 bytes)
//   nonce   = AES-256-GCM nonce (12 bytes)
//   ct_len  = uint32 big-endian, ciphertext 长度 (含 16-byte GCM tag)
//   ciphertext = AES-256-GCM(plaintext, key, nonce) + 16B tag
//
// 密钥派生: Argon2id (time=3, memory=64MB, threads=4, keylen=32)
// ============================================================

// 魔数 (4 bytes ASCII)
var magicEncrypted = []byte{'M', 'S', 'E', 'C'}

// KDF 算法 ID
const (
	kdfIDArgon2id uint8 = 0x01
)

// Argon2id 参数 (OWASP 2023 推荐 64MB / t=3 / p=4)
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // 64 MiB in KB
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32 // AES-256 需要 32 字节 key
)

// Salt 与 nonce 长度
const (
	encryptedSaltLen  = 16
	encryptedNonceLen = 12 // AES-GCM 标准 nonce 长度
	// encryptedHeaderLen = 4 (magic) + 1 (kdf_id) + 16 (salt) + 12 (nonce) + 4 (ct_len) = 37
	encryptedHeaderLen = 4 + 1 + encryptedSaltLen + encryptedNonceLen + 4
)

// ErrNotEncrypted 文件不是加密格式错误
var ErrNotEncrypted = errors.New("mysshw:: file is not encrypted (magic header missing)")

// ErrUnknownKDF 不支持的 KDF 算法
var ErrUnknownKDF = errors.New("mysshw:: unknown KDF algorithm in encrypted file")

// IsEncryptedFile 快速检查文件头部是否包含加密魔数
// (无需读完整文件, 适合在 LoadViperConfig 入口处判断)
func IsEncryptedFile(b []byte) bool {
	return len(b) >= len(magicEncrypted) && string(b[:len(magicEncrypted)]) == string(magicEncrypted)
}

// EncryptConfig 用 passphrase 加密明文 TOML 字节
//
// 输出: 二进制格式 (magic + kdf_id + salt + nonce + ct_len + ciphertext)
// 输出文件可被 DecryptConfig 解密
func EncryptConfig(plaintext []byte, passphrase []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("mysshw:: passphrase cannot be empty for encryption")
	}
	if len(plaintext) == 0 {
		return nil, errors.New("mysshw:: plaintext cannot be empty")
	}

	// 1. 生成 salt
	salt := make([]byte, encryptedSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("mysshw:: generate salt: %w", err)
	}

	// 2. 用 Argon2id 派生 32 字节 key
	key := argon2.IDKey(passphrase, salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// 3. 生成 nonce
	nonce := make([]byte, encryptedNonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("mysshw:: generate nonce: %w", err)
	}

	// 4. AES-256-GCM 加密 (返回 ciphertext + 16-byte auth tag)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: cipher.NewGCM: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil) // Seal appends tag

	// 5. 组装 header
	out := make([]byte, 0, encryptedHeaderLen+len(ciphertext))
	out = append(out, magicEncrypted...)
	out = append(out, kdfIDArgon2id)
	out = append(out, salt...)
	out = append(out, nonce...)
	// ct_len: 4 bytes big-endian
	ctLen := make([]byte, 4)
	binary.BigEndian.PutUint32(ctLen, uint32(len(ciphertext)))
	out = append(out, ctLen...)
	out = append(out, ciphertext...)

	return out, nil
}

// DecryptConfig 用 passphrase 解密加密的 TOML 字节
//
// 输入: EncryptConfig 输出的字节
// 输出: 解密后的明文 TOML 字节 (可送 viper/toml 解析)
func DecryptConfig(cipherdata []byte, passphrase []byte) ([]byte, error) {
	if len(cipherdata) < encryptedHeaderLen {
		return nil, ErrNotEncrypted
	}

	// 1. 解析 header
	if string(cipherdata[:4]) != string(magicEncrypted) {
		return nil, ErrNotEncrypted
	}
	kdfID := cipherdata[4]
	if kdfID != kdfIDArgon2id {
		return nil, fmt.Errorf("%w (id=0x%02x)", ErrUnknownKDF, kdfID)
	}

	salt := cipherdata[5 : 5+encryptedSaltLen]
	nonceStart := 5 + encryptedSaltLen
	nonce := cipherdata[nonceStart : nonceStart+encryptedNonceLen]
	ctLenStart := nonceStart + encryptedNonceLen
	ctLen := binary.BigEndian.Uint32(cipherdata[ctLenStart : ctLenStart+4])
	ciphertext := cipherdata[ctLenStart+4:]
	if uint32(len(ciphertext)) != ctLen {
		return nil, fmt.Errorf("mysshw:: ciphertext length mismatch: header=%d actual=%d", ctLen, len(ciphertext))
	}

	// 2. 派生 key
	key := argon2.IDKey(passphrase, salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// 3. AES-256-GCM 解密 + 验证 tag
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: cipher.NewGCM: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// GCM 认证失败 = 密码错 OR 文件被篡改
		return nil, fmt.Errorf("mysshw:: decrypt failed (wrong passphrase or corrupted file): %w", err)
	}
	return plaintext, nil
}

// EstimateDecryptTime 估算解密耗时 (用于 UI 提示)
// 基于 Argon2id (time=3, memory=64MB) 一次派生
func EstimateDecryptTime() time.Duration {
	// Argon2id 64MB 单次约 200-500ms (取决于硬件)
	// 保守估计 500ms
	return 500 * time.Millisecond
}
