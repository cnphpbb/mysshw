/**
 * @Author   DenysGeng <cnphp@hotmail.com>
 *
 * @Description sync (配置备份) 通道专用的 SSH 认证助手
 * @Version: 1.0.0
 * @Date     2025-10-25
 */

package ssh

import (
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// ============================================================
// sync 通道的 SSH 认证助手
//
// 背景: sync 命令走独立于普通 SSH 登录的通道 (SCP / WebDAV / S3 里的 SCP),
// 它复用同一份 known_hosts 与私钥体系, 但不经过 ssh 包的节点配置,
// 因此这里提供一组可复用的底层能力, 供 cmd 包组装 ClientConfig。
// ============================================================

// LoadKeySigner 读取私钥文件并解析为 ssh.Signer
//
// keyPath: 私钥路径, 支持 ~ / $HOME 展开与跨平台路径分隔符
// passphrase: 私钥口令, 支持 ${ENV} / ${ENV:-default} 引用;
//
//	为空时按无口令私钥解析 (OpenSSH 新格式私钥即使无口令也带 ENCRYPTED 标记,
//	由 ParsePrivateKey 自动识别)
//
// 失败时返回带路径信息的错误, 便于用户定位是"路径写错"还是"口令不对"
func LoadKeySigner(keyPath, passphrase string) (ssh.Signer, error) {
	expanded, err := expandHomeDir(keyPath)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: failed to resolve key path %q: %w", keyPath, err)
	}

	pemBytes, err := os.ReadFile(expanded)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: failed to read private key %q: %w", expanded, err)
	}

	pass := ResolveEnvRef(passphrase)
	if pass != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(pass))
		if err != nil {
			return nil, fmt.Errorf("mysshw:: failed to parse private key %q (passphrase may be wrong): %w", expanded, err)
		}
		return signer, nil
	}

	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: failed to parse private key %q (missing or wrong passphrase?): %w", expanded, err)
	}
	return signer, nil
}

// AgentAuthMethod 通过 SSH Agent 做公钥认证
//
// 返回 (AuthMethod, closer, error):
//   - SSH_AUTH_SOCK 未设置 → 错误 (调用方据此降级为"无 agent")
//   - closer 用于关闭与 agent 的 unix socket 连接, 调用方应在用完后 Close
//
// 适用于 cron / CI 等无交互场景: 密钥留在 agent 里, 配置文件无需存私钥
func AgentAuthMethod() (ssh.AuthMethod, io.Closer, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, nil, fmt.Errorf("mysshw:: SSH_AUTH_SOCK is not set (no SSH agent available)")
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, nil, fmt.Errorf("mysshw:: failed to connect to SSH agent at %s: %w", socket, err)
	}

	client := agent.NewClient(conn)
	return ssh.PublicKeysCallback(client.Signers), conn, nil
}

// TrustOnFirstUseCallback 导出的 TOFU (Trust On First Use) 主机指纹校验回调
//
// 与普通 SSH 登录走同一套 known_hosts 体系:
//  1. 主机已在 known_hosts → 严格校验, 指纹不符直接拒绝
//  2. 主机不在 known_hosts → TTY 下提示确认, 非交互下自动信任并写入
//
// 供 sync 等非节点场景使用 (没有 config.SSHNode 可传)
func TrustOnFirstUseCallback() (ssh.HostKeyCallback, error) {
	return trustOnFirstUseCallback(nil)
}

// SyncHostKeyCallback 按项目统一的优先级返回 sync 通道的主机指纹校验策略
//
// 优先级: MYSSHW_SKIP_HOST_KEY_CHECK env > 默认 TOFU
func SyncHostKeyCallback() (ssh.HostKeyCallback, error) {
	if IsEnvTrue(EnvSkipHostKeyCheck) {
		fmt.Fprintf(os.Stderr, "mysshw:: WARNING: %s=true, host key verification disabled (MITM risk)\n", EnvSkipHostKeyCheck)
		return ssh.InsecureIgnoreHostKey(), nil
	}
	return trustOnFirstUseCallback(nil)
}
