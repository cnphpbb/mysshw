package cmd

import (
	"fmt"
	"os"
	"strings"

	"mysshw/config"
	"mysshw/ssh"

	"golang.org/x/term"
)

// ============================================================
// sync 加密参数预检
//
// 目的: 在连上远程服务器之前, 先确认"本地配置文件是否已经配置加密参数",
// 避免传到一半才发现没法加密 (或者 cron/CI 里卡在交互输入上)。
//
// 与 config 包的配置体检 (config.CheckConfigItems) 的分工:
//   - config 包: 全面体检 (节点/sync/加密字段/文件权限), 启动时自动跑
//   - 本文件:   sync 专属的 fail-fast 预检, 只看"能不能完成本次同步"
// ============================================================

// passphraseSource sync 口令来源
type passphraseSource int

const (
	passphraseNone passphraseSource = iota
	passphraseFromEnv
	passphraseFromConfig
)

// detectPassphraseSource 检测 sync 口令来源 (env > [sync].passphrase)
//
// 返回 (来源, 人类可读描述); 未配置时来源为 passphraseNone
func detectPassphraseSource(opts syncCryptoOptions) (passphraseSource, string) {
	envName := strings.TrimSpace(opts.PassphraseEnv)
	if envName == "" {
		envName = DefaultSyncPassphraseEnv
	}
	if v, ok := os.LookupEnv(envName); ok && v != "" {
		return passphraseFromEnv, fmt.Sprintf("env %s", envName)
	}
	if v := strings.TrimSpace(ssh.ResolveEnvRef(opts.PassphraseCfg)); v != "" {
		return passphraseFromConfig, "config [sync].passphrase"
	}
	return passphraseNone, ""
}

// checkSyncEncryptionConfigured sync 加密参数预检
//
// 规则:
//   - 上传 + 加密开启: 必须有口令来源 (env / [sync].passphrase)
//     非交互环境 (cron / CI / MCP) 下缺失 → 直接报错退出 (不猜、不挂起)
//     交互环境 → 告警后转交互输入
//   - 上传 + 加密关闭: 明文上传告警
//   - 下载: 远端可能是密文, 缺口令只告警 (远端若是明文则不需要)
func checkSyncEncryptionConfigured(syncCfg config.SyncInfo, opts syncCryptoOptions, upload bool) error {
	src, detail := detectPassphraseSource(opts)

	// 上传但关闭了加密 → 明文上传告警
	if upload && !opts.Encrypt {
		fmt.Fprintln(os.Stderr, "mysshw:: warning: sync encryption is disabled, the remote server will keep the plaintext config")
		return nil
	}

	if upload {
		if src == passphraseNone {
			hint := fmt.Sprintf("set [sync].passphrase = \"${%s}\" in the config file, or export %s", opts.PassphraseEnv, opts.PassphraseEnv)
			if !isTerminalStdin() {
				return fmt.Errorf("mysshw:: sync encryption is enabled but no passphrase is configured and no interactive terminal is available: %s", hint)
			}
			fmt.Fprintf(os.Stderr, "mysshw:: warning: sync encryption is enabled but no passphrase is configured, falling back to interactive input (%s)\n", hint)
			return nil
		}
		fmt.Printf("mysshw:: sync encryption pre-check: enabled (Argon2id + AES-256-GCM), passphrase source: %s\n", detail)
		return nil
	}

	// 下载: 口令只在远端是密文时才需要
	if src == passphraseNone {
		fmt.Fprintf(os.Stderr, "mysshw:: warning: no sync passphrase configured; if the remote backup is encrypted, downloading will fail (set [sync].passphrase or export %s)\n", opts.PassphraseEnv)
		return nil
	}
	fmt.Printf("mysshw:: sync decryption pre-check: passphrase source: %s\n", detail)
	return nil
}

// isTerminalStdin checks whether stdin is attached to a terminal.
// Used to decide whether an interactive passphrase prompt is possible
// (false in cron / CI / MCP scenarios).
func isTerminalStdin() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
