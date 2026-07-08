package config

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== IsEncryptedFile ====================

func TestIsEncryptedFile_True(t *testing.T) {
	// 真实加密文件以 "MSEC" 开头
	data, _ := EncryptConfig([]byte("test"), []byte("pass"))
	assert.True(t, IsEncryptedFile(data), "encrypted file should be detected")
}

func TestIsEncryptedFile_False_PlainToml(t *testing.T) {
	plain := []byte("[[nodes]]\ngroups = \"Test\"\n")
	assert.False(t, IsEncryptedFile(plain), "plain TOML should not be detected as encrypted")
}

func TestIsEncryptedFile_False_TooShort(t *testing.T) {
	assert.False(t, IsEncryptedFile([]byte("MS")), "too-short file should not be detected")
}

func TestIsEncryptedFile_False_Empty(t *testing.T) {
	assert.False(t, IsEncryptedFile(nil), "nil should not be detected")
	assert.False(t, IsEncryptedFile([]byte{}), "empty should not be detected")
}

func TestIsEncryptedFile_False_WrongMagic(t *testing.T) {
	wrong := []byte{'X', 'X', 'X', 'X', 0x01}
	wrong = append(wrong, make([]byte, 32)...)
	assert.False(t, IsEncryptedFile(wrong), "wrong magic should not be detected")
}

// ==================== EncryptConfig + DecryptConfig 闭环 ====================

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	cases := []struct {
		name       string
		plaintext  []byte
		passphrase []byte
	}{
		{"ascii", []byte("hello world\n# comment\n[[nodes]]\ngroups = \"T\"\n"), []byte("test-pass-1")},
		{"unicode", []byte("密码 + 🔐 + 中文注释\n[[nodes]]\nname = \"测试\"\n"), []byte("密码-🔐-pepper")},
		{"empty_lines", []byte("\n\n\n# only comment\n"), []byte("simple")},
		{"large", make([]byte, 10000), []byte("test-pwd")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 填充 large 用随机数据
			if c.name == "large" {
				_, _ = rand.Read(c.plaintext)
			}
			cipher, err := EncryptConfig(c.plaintext, c.passphrase)
			require.NoError(t, err)
			require.NotEmpty(t, cipher)

			// 解密
			decrypted, err := DecryptConfig(cipher, c.passphrase)
			require.NoError(t, err)
			assert.Equal(t, c.plaintext, decrypted, "decrypted should match original")
		})
	}
}

func TestEncryptDecrypt_DifferentCiphertextsSameInput(t *testing.T) {
	// 相同 plaintext + passphrase 应产生不同 ciphertext (随机 salt + nonce)
	plain := []byte("same input")
	pass := []byte("same pass")

	c1, _ := EncryptConfig(plain, pass)
	c2, _ := EncryptConfig(plain, pass)

	assert.NotEqual(t, c1, c2, "相同输入应产生不同 ciphertext (随机 salt+nonce)")
}

// ==================== 错误处理 ====================

func TestEncrypt_EmptyPassphrase(t *testing.T) {
	_, err := EncryptConfig([]byte("test"), []byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "passphrase cannot be empty")

	_, err = EncryptConfig([]byte("test"), nil)
	assert.Error(t, err)
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	_, err := EncryptConfig([]byte{}, []byte("pass"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plaintext cannot be empty")
}

func TestDecrypt_TooShort(t *testing.T) {
	_, err := DecryptConfig([]byte("short"), []byte("pass"))
	assert.ErrorIs(t, err, ErrNotEncrypted)
}

func TestDecrypt_WrongMagic(t *testing.T) {
	bad := make([]byte, 64)
	bad[0] = 'X'
	bad[1] = 'X'
	_, err := DecryptConfig(bad, []byte("pass"))
	assert.ErrorIs(t, err, ErrNotEncrypted)
}

func TestDecrypt_WrongPassphrase(t *testing.T) {
	plain := []byte("secret content")
	cipher, err := EncryptConfig(plain, []byte("correct-pass"))
	require.NoError(t, err)

	_, err = DecryptConfig(cipher, []byte("wrong-pass"))
	assert.Error(t, err, "wrong passphrase should fail")
	// GCM 认证失败错误应该被包含
	assert.Contains(t, err.Error(), "decrypt failed")
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	plain := []byte("secret")
	cipher, _ := EncryptConfig(plain, []byte("pass"))

	// 篡改 ciphertext 的中间字节
	if len(cipher) > encryptedHeaderLen+5 {
		cipher[encryptedHeaderLen+5] ^= 0xFF
	}

	_, err := DecryptConfig(cipher, []byte("pass"))
	assert.Error(t, err, "tampered ciphertext should fail (GCM auth)")
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	plain := []byte("x")
	cipher, _ := EncryptConfig(plain, []byte("pass"))

	// 截断 ciphertext
	truncated := cipher[:len(cipher)-5]
	_, err := DecryptConfig(truncated, []byte("pass"))
	assert.Error(t, err)
}

func TestDecrypt_UnknownKDF(t *testing.T) {
	// 构造一个 header, kdf_id 是未知值
	bad := make([]byte, encryptedHeaderLen+16)
	copy(bad[0:4], magicEncrypted)
	bad[4] = 0xFF // 未知 kdf
	_, err := DecryptConfig(bad, []byte("pass"))
	assert.ErrorIs(t, err, ErrUnknownKDF)
}

func TestDecrypt_LengthMismatch(t *testing.T) {
	// 构造一个 header, ct_len 跟实际长度不符
	bad := make([]byte, encryptedHeaderLen+5)
	copy(bad[0:4], magicEncrypted)
	bad[4] = kdfIDArgon2id
	// salt + nonce 默认全 0
	// ct_len 设为 9999 (明显不对)
	bad[encryptedHeaderLen-4] = 0x00
	bad[encryptedHeaderLen-3] = 0x00
	bad[encryptedHeaderLen-2] = 0x27
	bad[encryptedHeaderLen-1] = 0x0F // 9999
	_, err := DecryptConfig(bad, []byte("pass"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext length mismatch")
}

// ==================== Header 格式校验 ====================

func TestEncryptedFile_HeaderStructure(t *testing.T) {
	cipher, err := EncryptConfig([]byte("x"), []byte("p"))
	require.NoError(t, err)

	// magic 头 4 字节
	assert.True(t, bytes.Equal(cipher[0:4], magicEncrypted))

	// kdf_id
	assert.Equal(t, kdfIDArgon2id, cipher[4])

	// salt 16 字节
	salt := cipher[5 : 5+encryptedSaltLen]
	assert.Len(t, salt, encryptedSaltLen)

	// nonce 12 字节
	nonceStart := 5 + encryptedSaltLen
	nonce := cipher[nonceStart : nonceStart+encryptedNonceLen]
	assert.Len(t, nonce, encryptedNonceLen)

	// ct_len 4 bytes (big-endian)
	ctLenStart := nonceStart + encryptedNonceLen
	_ = ctLenStart
	assert.GreaterOrEqual(t, len(cipher), encryptedHeaderLen)
}

// ==================== 实际配置文件 (TOML) 闭环 ====================

func TestEncryptDecrypt_TOMLConfig(t *testing.T) {
	// 用 DefaultTomlConfig 作为测试输入 (确保真实配置文件能 round-trip)
	original := []byte(DefaultTomlConfig)
	passphrase := []byte("test-master-passphrase-32chars")

	cipher, err := EncryptConfig(original, passphrase)
	require.NoError(t, err)

	decrypted, err := DecryptConfig(cipher, passphrase)
	require.NoError(t, err)
	assert.Equal(t, original, decrypted)

	// 验证: 加密后的字节不再是合法 TOML (因为含二进制 header)
	assert.False(t, IsEncryptedFile(original), "原始 TOML 不应被识别为加密")
	assert.True(t, IsEncryptedFile(cipher), "加密后应被识别为加密")
}
