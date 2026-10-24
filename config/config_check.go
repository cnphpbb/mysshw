package config

import (
	"fmt"
	"os"
	"strings"
)

// ============================================================
// 配置文件关键项检查 (config check)
//
// 目的: mysshw 启动时主动体检本地配置文件, 把"关键配置项没配 / 配得不安全"
// 的问题在连远程之前就暴露出来, 而不是等到真正用到时才发现
// (例如 sync 时才发现没配加密口令, 传到一半失败)。
//
// 与 ValidateConfig 的分工:
//   - ValidateConfig  : 硬校验, 不满足直接 error (阻止启动)
//   - CheckConfigItems : 软检查, 缺项/风险项以 warn / info 列出, 不阻止启动
//
// 输出原则: 一律写 stderr, 不污染 stdout
// (避免破坏 `mysshw list -o json` 等机器可读输出)。
// ============================================================

// IssueLevel 检查结果级别
type IssueLevel string

const (
	// IssueError 关键项缺失, 对应功能不可用
	IssueError IssueLevel = "error"
	// IssueWarn 已配置但有风险, 建议修正
	IssueWarn IssueLevel = "warn"
	// IssueInfo 可选项未配置, 会走默认值
	IssueInfo IssueLevel = "info"
)

// ConfigIssue 单条配置检查结果
type ConfigIssue struct {
	// Level 问题级别 (error / warn / info)
	Level IssueLevel `json:"level"`
	// Item 出问题的配置项路径 (如 node "web-1".user)
	Item string `json:"item"`
	// Message 问题描述
	Message string `json:"message"`
	// Hint 修复建议 (可为空)
	Hint string `json:"hint,omitempty"`
}

// CheckConfigItems 检查本地配置文件的关键配置项是否已配置
//
// 只做检查, 不修改任何配置; 返回问题列表 (无问题时返回空切片)
func CheckConfigItems(cfg *Configs) []ConfigIssue {
	if cfg == nil {
		return []ConfigIssue{{
			Level:   IssueError,
			Item:    "config",
			Message: "configuration is not loaded",
			Hint:    "check the config file with: mysshw config check",
		}}
	}
	issues := make([]ConfigIssue, 0)
	issues = append(issues, checkNodeItems(cfg)...)
	issues = append(issues, checkSyncItems(cfg)...)
	issues = append(issues, checkEncryptionItems(cfg)...)
	return issues
}

// checkNodeItems 检查节点相关关键项
func checkNodeItems(cfg *Configs) []ConfigIssue {
	issues := make([]ConfigIssue, 0)

	if len(cfg.Nodes) == 0 {
		return append(issues, ConfigIssue{
			Level:   IssueWarn,
			Item:    "nodes",
			Message: "no SSH node configured",
			Hint:    "add one with: mysshw config add --group <group> --name <name> --host <host> --user <user>",
		})
	}

	// 收集所有节点名 / 别名, 用于重复与 proxy 引用检查
	allNames := make(map[string]bool)
	allAliases := make(map[string]bool)
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n == nil {
				continue
			}
			if n.Name != "" {
				allNames[n.Name] = true
			}
			if n.Alias != "" {
				allAliases[n.Alias] = true
			}
		}
	}

	seenName := make(map[string]string)  // name -> 首次出现的组
	seenAlias := make(map[string]string) // alias -> 首次出现的组

	for gi, g := range cfg.Nodes {
		group := g.Groups
		if strings.TrimSpace(group) == "" {
			issues = append(issues, ConfigIssue{
				Level:   IssueError,
				Item:    fmt.Sprintf("nodes[%d].groups", gi),
				Message: "group name is empty",
				Hint:    "set [[nodes]] groups = \"<group>\"",
			})
			group = fmt.Sprintf("(unnamed group #%d)", gi)
		}

		if len(g.SSHNodes) == 0 {
			issues = append(issues, ConfigIssue{
				Level:   IssueWarn,
				Item:    fmt.Sprintf("nodes[%s].ssh", group),
				Message: fmt.Sprintf("group %q has no SSH node", group),
				Hint:    "add a node with: mysshw config add --group " + group + " --name <name> --host <host>",
			})
			continue
		}

		for _, n := range g.SSHNodes {
			if n == nil {
				continue
			}
			issues = append(issues, checkSSHNodeItems(n, group, allNames)...)
			if n.Name != "" {
				if prev, ok := seenName[n.Name]; ok {
					issues = append(issues, ConfigIssue{
						Level:   IssueWarn,
						Item:    fmt.Sprintf("node %q.name", n.Name),
						Message: fmt.Sprintf("duplicated node name (also in group %q)", prev),
						Hint:    "node names must be unique, rename with: mysshw config update " + n.Name + " --alias <alias>",
					})
				} else {
					seenName[n.Name] = group
				}
			}
			if n.Alias != "" {
				if prev, ok := seenAlias[n.Alias]; ok {
					issues = append(issues, ConfigIssue{
						Level:   IssueWarn,
						Item:    fmt.Sprintf("node %q.alias", n.Name),
						Message: fmt.Sprintf("duplicated alias %q (also in group %q)", n.Alias, prev),
						Hint:    "aliases must be unique",
					})
				} else {
					seenAlias[n.Alias] = group
				}
			}
		}
	}
	return issues
}

// checkSSHNodeItems 检查单个 SSH 节点的关键项
func checkSSHNodeItems(n *SSHNode, group string, allNames map[string]bool) []ConfigIssue {
	issues := make([]ConfigIssue, 0)
	item := func(field string) string {
		if n.Name != "" {
			return fmt.Sprintf("node %q.%s", n.Name, field)
		}
		return fmt.Sprintf("node %s.%s", group, field)
	}

	// user / port 未配置 → 走默认值
	if strings.TrimSpace(n.User) == "" {
		issues = append(issues, ConfigIssue{
			Level:   IssueInfo,
			Item:    item("user"),
			Message: "user not configured, defaulting to root",
			Hint:    "set user explicitly: mysshw config update " + n.Name + " --user <user>",
		})
	}
	if n.Port <= 0 {
		issues = append(issues, ConfigIssue{
			Level:   IssueInfo,
			Item:    item("port"),
			Message: "port not configured, defaulting to 22",
			Hint:    "set port explicitly: mysshw config update " + n.Name + " --port <port>",
		})
	} else if n.Port > 65535 {
		issues = append(issues, ConfigIssue{
			Level:   IssueError,
			Item:    item("port"),
			Message: fmt.Sprintf("invalid port %d, must be between 1 and 65535", n.Port),
			Hint:    "fix it with: mysshw config update " + n.Name + " --port <port>",
		})
	}

	// 认证方式: password / keypath 至少要有一个
	hasPassword := strings.TrimSpace(n.Password) != ""
	hasKeyPath := strings.TrimSpace(n.KeyPath) != ""
	if !hasPassword && !hasKeyPath {
		issues = append(issues, ConfigIssue{
			Level:   IssueWarn,
			Item:    item("password"),
			Message: "no credential configured (neither password nor keypath), login fails unless an SSH agent provides the key",
			Hint:    "set a credential: mysshw config update " + n.Name + " --password '${MYSSHW_PWD_" + n.Name + "}'",
		})
	}

	// 明文密码落盘
	if hasPassword && IsEnvRefPlaintext(n.Password) {
		issues = append(issues, ConfigIssue{
			Level:   IssueWarn,
			Item:    item("password"),
			Message: "password is stored in plaintext",
			Hint:    "use a bcrypt hash or ${ENV} reference (see: mysshw hash gen)",
		})
	}

	// 密钥文件不存在
	if hasKeyPath {
		keyPath := strings.ReplaceAll(n.KeyPath, "\\", "/")
		if _, err := os.Stat(keyPath); err != nil {
			issues = append(issues, ConfigIssue{
				Level:   IssueError,
				Item:    item("keypath"),
				Message: fmt.Sprintf("key file not found: %s", keyPath),
				Hint:    "check the path or use an absolute path",
			})
		}
	}

	// 跳板节点引用是否存在
	if n.Proxy != "" && !allNames[n.Proxy] {
		issues = append(issues, ConfigIssue{
			Level:   IssueWarn,
			Item:    item("proxy"),
			Message: fmt.Sprintf("proxy node %q not found in configuration", n.Proxy),
			Hint:    "proxy must reference the name of another node",
		})
	}

	// 关闭主机指纹校验 (安全风险)
	if n.StrictHostKeyChecking != nil && !*n.StrictHostKeyChecking {
		issues = append(issues, ConfigIssue{
			Level:   IssueWarn,
			Item:    item("strict_host_key_checking"),
			Message: "host key checking is disabled (MITM risk)",
			Hint:    "remove the field to enable strict checking again",
		})
	}

	return issues
}

// checkSyncItems 检查 sync 相关关键项 (含加密参数)
func checkSyncItems(cfg *Configs) []ConfigIssue {
	issues := make([]ConfigIssue, 0)
	sync := cfg.SyncCfg
	syncType := strings.ToLower(strings.TrimSpace(sync.Type))

	if syncType == "" {
		return append(issues, ConfigIssue{
			Level:   IssueInfo,
			Item:    "sync.type",
			Message: "sync type not configured, `mysshw sync` is unavailable",
			Hint:    "set [sync].type = \"scp\" | \"webdav\" | \"s3\"",
		})
	}

	switch syncType {
	case "scp":
		if sync.RemoteUri == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueError, Item: "sync.remote_uri",
				Message: "remote_uri is required for scp sync type",
				Hint:    "set [sync].remote_uri = \"user@host:port\"",
			})
		}
		if sync.RemotePath == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueError, Item: "sync.remote_path",
				Message: "remote_path is required for scp sync type",
				Hint:    "set [sync].remote_path = \"/remote/path/mysshw.toml\"",
			})
		}
		if sync.SCPConfig.Username == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueWarn, Item: "sync.scp.username",
				Message: "scp username not configured",
				Hint:    "set [sync.scp].username = \"root\"",
			})
		}
		if sync.SCPConfig.KeyPath != "" {
			keyPath := strings.ReplaceAll(sync.SCPConfig.KeyPath, "\\", "/")
			if _, err := os.Stat(keyPath); err != nil {
				issues = append(issues, ConfigIssue{
					Level: IssueError, Item: "sync.scp.keypath",
					Message: fmt.Sprintf("scp key file not found: %s", keyPath),
					Hint:    "check the path or use an absolute path",
				})
			}
		}
	case "webdav":
		if sync.RemoteUri == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueError, Item: "sync.remote_uri",
				Message: "remote_uri is required for webdav sync type",
				Hint:    "set [sync].remote_uri = \"https://dav.example.com\"",
			})
		}
		if sync.WebDAVConfig.Username == "" && sync.WebDAVConfig.Password == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueError, Item: "sync.webdav",
				Message: "neither username nor password configured for webdav sync type",
				Hint:    "set [sync.webdav].username / .password",
			})
		}
	case "s3":
		if sync.S3Config.AccessKey == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueError, Item: "sync.s3.access_key",
				Message: "access_key is required for s3 sync type",
				Hint:    "set [sync.s3].access_key = \"${MYSSHW_S3_AK}\"",
			})
		}
		if sync.S3Config.SecretKey == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueError, Item: "sync.s3.secret_key",
				Message: "secret_key is required for s3 sync type",
				Hint:    "set [sync.s3].secret_key = \"${MYSSHW_S3_SK}\"",
			})
		}
		if sync.S3Config.BucketName == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueError, Item: "sync.s3.bucket_name",
				Message: "bucket_name is required for s3 sync type",
				Hint:    "set [sync.s3].bucket_name = \"my-backup\"",
			})
		}
		if sync.S3Config.Endpoint == "" && sync.RemoteUri == "" {
			issues = append(issues, ConfigIssue{
				Level: IssueError, Item: "sync.s3.endpoint",
				Message: "either endpoint or remote_uri is required for s3 sync type",
				Hint:    "set [sync.s3].endpoint = \"https://s3.example.com\"",
			})
		}
	default:
		issues = append(issues, ConfigIssue{
			Level:   IssueError,
			Item:    "sync.type",
			Message: fmt.Sprintf("unsupported sync type: %s (supported: scp, webdav, s3)", sync.Type),
			Hint:    "fix [sync].type in the config file",
		})
	}

	// 加密参数 (上传加密默认开启)
	switch {
	case sync.Encrypt == nil:
		issues = append(issues, ConfigIssue{
			Level:   IssueInfo,
			Item:    "sync.encrypt",
			Message: "sync.encrypt not configured, upload encryption defaults to enabled (remote stores ciphertext only)",
			Hint:    "set [sync].encrypt = true explicitly, or false to upload plaintext",
		})
	case !*sync.Encrypt:
		issues = append(issues, ConfigIssue{
			Level:   IssueWarn,
			Item:    "sync.encrypt",
			Message: "sync encryption is disabled, remote server will keep the plaintext config",
			Hint:    "re-enable with: mysshw sync --upload --encrypt",
		})
	}

	// 口令是否已配置 (env 或 [sync].passphrase)
	if !SyncPassphraseConfigured(sync.Passphrase) {
		msg := "sync passphrase not configured (no env value, no [sync].passphrase), mysshw will ask for it interactively"
		hint := "set [sync].passphrase = \"${MYSSHW_SYNC_PASSPHRASE}\" or export MYSSHW_SYNC_PASSPHRASE (interactive input fails in cron/CI)"
		if strings.TrimSpace(sync.Passphrase) != "" {
			// 字段配了, 但 ${VAR} 引用的环境变量没取到值
			msg = fmt.Sprintf("sync passphrase references an environment variable that is not set: %s", sync.Passphrase)
			hint = fmt.Sprintf("export the referenced variable, or set [sync].passphrase to a literal value (env name: %s)", DefaultSyncPassphraseEnv)
		}
		issues = append(issues, ConfigIssue{
			Level:   IssueWarn,
			Item:    "sync.passphrase",
			Message: msg,
			Hint:    hint,
		})
	}

	return issues
}

// checkEncryptionItems 检查加密相关字段配置
func checkEncryptionItems(cfg *Configs) []ConfigIssue {
	issues := make([]ConfigIssue, 0)

	algo := strings.ToLower(strings.TrimSpace(cfg.EncryptionAlgorithm))
	if algo != "" {
		switch algo {
		case "bcrypt", "bcrypt-pepper", "argon2id", "pbkdf2-sha256":
		default:
			issues = append(issues, ConfigIssue{
				Level:   IssueWarn,
				Item:    "encryption_algorithm",
				Message: fmt.Sprintf("unknown encryption algorithm: %s", cfg.EncryptionAlgorithm),
				Hint:    "supported: bcrypt, bcrypt-pepper, argon2id, pbkdf2-sha256",
			})
		}
		if algo == "bcrypt-pepper" && strings.TrimSpace(cfg.SecretKey) == "" {
			issues = append(issues, ConfigIssue{
				Level:   IssueError,
				Item:    "secret_key",
				Message: "encryption_algorithm = bcrypt-pepper requires secret_key (pepper)",
				Hint:    "generate one with: mysshw hash pepper",
			})
		}
	}

	if strings.TrimSpace(cfg.SecretKey) != "" && algo != "bcrypt-pepper" {
		issues = append(issues, ConfigIssue{
			Level:   IssueInfo,
			Item:    "secret_key",
			Message: "secret_key is configured but encryption_algorithm is not bcrypt-pepper (pepper is not applied)",
			Hint:    "set encryption_algorithm = \"bcrypt-pepper\" to make use of it",
		})
	}

	cipher := strings.ToLower(strings.TrimSpace(cfg.Cipher))
	if cipher != "" {
		switch cipher {
		case "aes-256-gcm", "chacha20-poly1305":
		default:
			issues = append(issues, ConfigIssue{
				Level:   IssueWarn,
				Item:    "cipher",
				Message: fmt.Sprintf("unknown cipher: %s", cfg.Cipher),
				Hint:    "supported: aes-256-gcm, chacha20-poly1305",
			})
		}
	}

	return issues
}

// CheckConfigFilePerm 检查配置文件权限是否安全 (建议 0600)
func CheckConfigFilePerm(path string) []ConfigIssue {
	issues := make([]ConfigIssue, 0)
	info, err := os.Stat(path)
	if err != nil {
		return append(issues, ConfigIssue{
			Level:   IssueError,
			Item:    "config_file",
			Message: fmt.Sprintf("cannot stat config file: %v", err),
			Hint:    "check the path passed with --cfg",
		})
	}
	if mode := info.Mode().Perm(); mode&0077 != 0 {
		issues = append(issues, ConfigIssue{
			Level:   IssueWarn,
			Item:    "config_file.permission",
			Message: fmt.Sprintf("config file %s is group/world readable (%04o), it may contain secrets", path, mode),
			Hint:    "fix it with: chmod 600 " + path,
		})
	}
	return issues
}

// DefaultSyncPassphraseEnv 默认的 sync 加解密口令环境变量名
// 定义在 config 包 (而非 cmd) 是因为配置检查 (config_check.go) 需要感知
// 环境变量里是否真的有口令, cmd 层的 --passphrase-env 可覆盖它。
const DefaultSyncPassphraseEnv = "MYSSHW_SYNC_PASSPHRASE"

// SyncPassphraseConfigured 判断 sync 口令是否已配置
//
// 规则: 口令来源任一命中即视为已配置
//  1. 环境变量 (默认 MYSSHW_SYNC_PASSPHRASE) 有非空值
//  2. [sync].passphrase 非空, 且其中的 ${VAR} 引用能取到实际值
//     (纯 ${VAR} 且 env 未设置 → 视为未配置)
func SyncPassphraseConfigured(passphrase string) bool {
	if v, ok := os.LookupEnv(DefaultSyncPassphraseEnv); ok && v != "" {
		return true
	}
	p := strings.TrimSpace(passphrase)
	if p == "" {
		return false
	}
	if HasEnvRef(p) {
		return strings.TrimSpace(ResolveEnvRef(p)) != ""
	}
	return true
}

// FormatConfigIssues 把检查结果格式化为可读文本
func FormatConfigIssues(issues []ConfigIssue) string {
	if len(issues) == 0 {
		return "mysshw:: config check: all key configuration items are configured\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "mysshw:: config check: %d issue(s) found\n", len(issues))
	for _, it := range issues {
		fmt.Fprintf(&sb, "  [%s] %s: %s\n", it.Level, it.Item, it.Message)
		if it.Hint != "" {
			fmt.Fprintf(&sb, "         hint: %s\n", it.Hint)
		}
	}
	return sb.String()
}

// CountIssuesByLevel 统计各级别问题数量 (给 JSON 输出用)
func CountIssuesByLevel(issues []ConfigIssue) map[string]int {
	counts := map[string]int{"error": 0, "warn": 0, "info": 0}
	for _, it := range issues {
		counts[string(it.Level)]++
	}
	return counts
}
