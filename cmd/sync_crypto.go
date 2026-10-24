package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mysshw/config"
	"mysshw/ssh"
)

// ============================================================
// sync 命令的文件级加解密 (远程备份防拖库)
//
// 威胁模型: 远程备份服务器 (SCP/WebDAV/S3) 被入侵后, 攻击者会把备份文件
// 拖走离线破解。防护方式 = 远程永远只存密文, 口令只留在本地。
//
// 流程:
//   上传: 本地明文 → Argon2id+AES-256-GCM 加密 → 传输 → 远程密文
//   下载: 远程密文 → 传输 → 本地解密 → 落盘明文 (或 --keep-encrypted 保留密文)
//
// 口令来源 (优先级从高到低):
//   1. 环境变量 (--passphrase-env 指定, 默认 MYSSHW_SYNC_PASSPHRASE)
//   2. 配置文件 [sync].passphrase (支持 ${ENV} / ${ENV:-default} 引用)
//   3. 交互输入 (加密时输入两次确认, 防打错 —— 口令丢了文件就找不回来)
//
// 口令任何情况下都不落盘、不进日志、不进命令行参数 (避免 ps / history 泄露)。
// ============================================================

// DefaultSyncPassphraseEnv 默认的 sync 加解密口令环境变量名
// (实现在 config 包, 此处保留别名供 cmd 包使用)
const DefaultSyncPassphraseEnv = config.DefaultSyncPassphraseEnv

// syncCryptoOptions 同步加解密相关选项
type syncCryptoOptions struct {
	// Encrypt 上传前是否加密 (远程不落明文)
	Encrypt bool
	// KeepEncrypted 下载后是否保留密文 (不落明文到本地)
	KeepEncrypted bool
	// KeepEncryptedSet 用户是否显式指定了 --keep-encrypted (显式指定时不保留本地加密状态)
	KeepEncryptedSet bool
	// PassphraseEnv 口令环境变量名 (空 → DefaultSyncPassphraseEnv)
	PassphraseEnv string
	// PassphraseCfg 配置文件中的口令 (支持 ${ENV} 引用)
	PassphraseCfg string
}

// payloadKind 下载内容的类型 (决定落地时的提示)
type payloadKind int

const (
	payloadPlain       payloadKind = iota // 明文 (解密成功)
	payloadEncrypted                      // 密文 (--keep-encrypted 保留)
	payloadLegacyPlain                    // 远端是旧版本明文备份, 原样落地
)

// resolveSyncPassphrase 解析同步加解密口令
//
// 优先级: 环境变量 > 配置文件 [sync].passphrase > 交互输入
// confirm=true 时交互输入需两次一致 (加密口令不可找回, 防打错)
func resolveSyncPassphrase(opts syncCryptoOptions, confirm bool) (string, error) {
	// 1. 环境变量
	envName := strings.TrimSpace(opts.PassphraseEnv)
	if envName == "" {
		envName = DefaultSyncPassphraseEnv
	}
	if v, ok := os.LookupEnv(envName); ok && v != "" {
		return v, nil
	}

	// 2. 配置文件 (支持 ${ENV} / ${ENV:-default} 引用, 与 SSH 密码同一套语义)
	if v := strings.TrimSpace(ssh.ResolveEnvRef(opts.PassphraseCfg)); v != "" {
		return v, nil
	}

	// 3. 交互输入
	prompt := "Enter passphrase for sync decryption: "
	if confirm {
		prompt = "Enter passphrase for sync encryption: "
	}
	first, err := readPassphraseInteractive(prompt)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(first) == "" {
		return "", fmt.Errorf("mysshw:: empty passphrase")
	}
	if confirm {
		second, err := readPassphraseInteractive("Confirm passphrase: ")
		if err != nil {
			return "", err
		}
		if first != second {
			return "", fmt.Errorf("mysshw:: passphrases do not match")
		}
	}
	return first, nil
}

// encryptForSync 上传前加密
//
// 幂等: 本地文件本身已是密文 (MSEC magic) 时原样返回, 不做双重加密
// (双重加密后, 下载时需要两层口令, 且无法用原口令还原)
func encryptForSync(data []byte, passphrase string) ([]byte, error) {
	if config.IsEncryptedFile(data) {
		return data, nil
	}
	enc, err := config.EncryptConfig(data, []byte(passphrase))
	if err != nil {
		return nil, err
	}
	return enc, nil
}

// decryptAfterSync 下载后解密 (纯函数, 不做交互)
//
// 返回 (落盘内容, 是否仍为密文, error):
//   - 远端是密文且 keepEncrypted=false → 解密为明文返回
//   - 远端是密文且 keepEncrypted=true  → 原样返回 (本地也保持密文)
//   - 远端是明文 (旧版本备份)          → 原样返回, 由调用方给出提示
//
// 解密失败 (口令错 / 文件被篡改) 直接返回错误, 调用方不得覆盖本地配置
func decryptAfterSync(data []byte, passphrase string, keepEncrypted bool) ([]byte, bool, error) {
	if !config.IsEncryptedFile(data) {
		return data, false, nil // 旧版本明文备份, 兼容处理
	}
	if keepEncrypted {
		return data, true, nil
	}
	plain, err := config.DecryptConfig(data, []byte(passphrase))
	if err != nil {
		return nil, true, fmt.Errorf("mysshw:: decrypt remote config failed: %w", err)
	}
	return plain, false, nil
}

// encryptUploadPayload 上传前的加密处理 (口令解析 + 加密)
//
// opts.Encrypt=false 时原样返回 (明文上传, 会打印告警)
func encryptUploadPayload(data []byte, opts syncCryptoOptions) ([]byte, error) {
	if !opts.Encrypt {
		fmt.Fprintln(os.Stderr, "mysshw:: warning: uploading plaintext config (encryption disabled)")
		return data, nil
	}
	if config.IsEncryptedFile(data) {
		fmt.Println("Local config already encrypted, uploading ciphertext as-is.")
		return data, nil
	}
	passphrase, err := resolveSyncPassphrase(opts, true)
	if err != nil {
		return nil, err
	}
	enc, err := encryptForSync(data, passphrase)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: encrypt config failed: %w", err)
	}
	fmt.Println("Config encrypted (Argon2id + AES-256-GCM); remote stores ciphertext only.")
	return enc, nil
}

// decryptDownloaded 下载内容的解密处理 (含口令提示)
//
// 返回 (落盘内容, 类型, error); 口令错误时返回错误, 调用方不得覆盖本地配置
func decryptDownloaded(data []byte, opts syncCryptoOptions) ([]byte, payloadKind, error) {
	if !config.IsEncryptedFile(data) {
		fmt.Fprintln(os.Stderr, "mysshw:: warning: remote backup is not encrypted (legacy plaintext), storing as-is")
		return data, payloadLegacyPlain, nil
	}
	if opts.KeepEncrypted {
		return data, payloadEncrypted, nil
	}
	passphrase, err := resolveSyncPassphrase(opts, false)
	if err != nil {
		return nil, payloadPlain, err
	}
	plain, _, err := decryptAfterSync(data, passphrase, false)
	if err != nil {
		return nil, payloadPlain, err
	}
	return plain, payloadPlain, nil
}

// localConfigIsEncrypted 判断本地既有配置文件是否为密文 (文件级加密)
func localConfigIsEncrypted(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return config.IsEncryptedFile(data)
}

// preserveLocalEncryptionState 保留本地既有配置的加密状态
//
// 本地配置本来就是密文 (用户手动 mysshw encrypt 过) → 下载后也保持密文,
// 避免把用户刻意加密的配置降级成明文;
// 显式指定 --keep-encrypted / --keep-encrypted=false 时以用户意图为准。
func preserveLocalEncryptionState(localPath string, opts syncCryptoOptions) syncCryptoOptions {
	if opts.KeepEncryptedSet || opts.KeepEncrypted || !localConfigIsEncrypted(localPath) {
		return opts
	}
	fmt.Println("Local config is encrypted, keeping the downloaded file encrypted (use --keep-encrypted=false to force decryption).")
	opts.KeepEncrypted = true
	return opts
}

// saveDownloadedConfig 把下载内容落地到本地配置文件
// (原子写 + 自动备份旧文件 + 0600 权限)
func saveDownloadedConfig(localPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("mysshw:: create local dir failed: %w", err)
	}
	if err := config.WriteTomlWithBackup(localPath, data); err != nil {
		return fmt.Errorf("mysshw:: write local config failed: %w", err)
	}
	return nil
}

// reportPayloadKind 打印下载内容落地后的提示
func reportPayloadKind(kind payloadKind) {
	switch kind {
	case payloadEncrypted:
		fmt.Println("Remote config kept encrypted locally (--keep-encrypted).")
	case payloadLegacyPlain:
		fmt.Println("Remote legacy plaintext backup stored locally as-is.")
	default:
		fmt.Println("Remote config decrypted and saved locally.")
	}
}
