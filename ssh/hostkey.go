package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mysshw/config"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// 主机指纹存储文件名（与 OpenSSH 共享 ~/.ssh/known_hosts）
const knownHostsFileName = "known_hosts"

// 保护 known_hosts 并发写
var knownHostsMu sync.Mutex

// currentHomeDir 返回当前用户主目录, 使用包级变量便于测试 mock
// Windows 上 os/user.Current() 不读取 HOME 环境变量, 必须通过此抽象 mock
var currentHomeDir = func() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

// GetKnownHostsPath 返回 known_hosts 文件的绝对路径 (公开版本)
//
// 调度 hostkey 列表 / 重删 / 更新等需求的对外 API。
// 与 getKnownHostsPath 逻辑相同 (以下是前者 alias),
// 路径解析逻辑: MYSSHW_KNOWN_HOSTS env > config.CFG.KnownHostsPath > ~/.ssh/known_hosts.
//
// 如果需要修改逻辑, 同步本函数。
func GetKnownHostsPath() (string, error) {
	return getKnownHostsPath()
}

// getKnownHostsPath 返回 known_hosts 文件的绝对路径
// 优先级: MYSSHW_KNOWN_HOSTS 环境变量 > config.CFG.KnownHostsPath > ~/.ssh/known_hosts
func getKnownHostsPath() (string, error) {
	// 最高优先: 环境变量 (CI/脚本场景下可临时指定)
	if envPath := os.Getenv(EnvKnownHosts); envPath != "" {
		return resolveKnownHostsPath(envPath)
	}

	// 次优先: 用户在配置文件中指定的自定义路径
	if config.CFG != nil && config.CFG.KnownHostsPath != "" {
		return resolveKnownHostsPath(config.CFG.KnownHostsPath)
	}

	// 默认: ~/.ssh/known_hosts (与 OpenSSH 共享同一份指纹库)
	homeDir, err := currentHomeDir()
	if err != nil {
		return "", fmt.Errorf("mysshw:: failed to get current user: %w", err)
	}

	// 确保 .ssh 目录存在
	sshDir := filepath.Join(homeDir, ".ssh")
	if _, err := os.Stat(sshDir); os.IsNotExist(err) {
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			return "", fmt.Errorf("mysshw:: failed to create .ssh dir: %w", err)
		}
	}

	return filepath.Join(sshDir, knownHostsFileName), nil
}

// resolveKnownHostsPath 处理自定义 known_hosts 路径:
//  1. 展开 ~ 和 $HOME
//  2. 自动创建父目录 (0700 权限)
func resolveKnownHostsPath(path string) (string, error) {
	expanded, err := expandHomeDir(path)
	if err != nil {
		return "", fmt.Errorf("mysshw:: failed to expand known_hosts path: %w", err)
	}

	// 自动创建父目录 (如不存在)
	parentDir := filepath.Dir(expanded)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		if err := os.MkdirAll(parentDir, 0700); err != nil {
			return "", fmt.Errorf("mysshw:: failed to create known_hosts dir: %w", err)
		}
	}

	return expanded, nil
}

// trustOnFirstUseCallback 创建一个带回退机制的主机指纹校验回调
// 行为:
//  1. 主机在 known_hosts 中: 严格校验
//     - key 匹配: 通过
//     - key 不匹配: 报错拒绝 (默认), 或 rotate (node.RotateKnownHosts=true)
//  2. 主机不在 known_hosts 中:
//     - 终端交互 (stdin 是 TTY): 提示用户确认, 信任后追加写入
//     - 非交互 (stdin 不是 TTY / pipe): 自动 trust 并写入 known_hosts
//     (OpenSSH StrictHostKeyChecking=accept-new 的语义；供 AI/脚本场景使用)
//
// node 参数用于读取 RotateKnownHosts 标志 (是否在 fingerprint 不匹配时自动 rotate)。
// 传入 nil 等价于 node.RotateKnownHosts=false (默认严格行为)。
func trustOnFirstUseCallback(node *config.SSHNode) (ssh.HostKeyCallback, error) {
	path, err := getKnownHostsPath()
	if err != nil {
		return nil, err
	}

	// 如果文件不存在, 创建一个空文件（避免 knownhosts.New 报错）
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, fmt.Errorf("mysshw:: failed to create known_hosts: %w", err)
		}
		_ = f.Close()
	}

	// 基础 callback: 仅严格校验, 不写入
	baseCallback, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: failed to load known_hosts: %w", err)
	}

	// 捕获 rotate 标志 (避免 callback 内闭包变量, 让 func 决定)
	rotateOnMismatch := node != nil && node.RotateKnownHosts

	// 包装: 处理"未知主机" + "mismatch + rotate" 分支
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// knownhosts 内部 addr.check 调用 net.SplitHostPort(hostname),
		// 对不含 :port 的 hostname 返回 generic 'missing port in address' 错误
		// (不是 *knownhosts.KeyError), 导致 errors.As(&keyErr) 失败。
		//
		// 解决: 手动确保 hostname 含 :port 再传给 baseCallback。
		//   - ssh callback 给的 hostname 可能无 port (server 报告的 server host)
		//   - known_hosts 文件中的明文条目通常也只列 host (无 port)
		//   - 我们用 :22 fallback; 若实际 port 不是 22 但 known_hosts 已含 :port 条目会找不到,
		//     走 accept-new 即可追加新 port 条目 (SSH 默认行为)
		normalized := hostname
		if _, _, e := net.SplitHostPort(normalized); e != nil {
			normalized = net.JoinHostPort(hostname, "22")
		}
		err := baseCallback(normalized, remote, key)
		if err == nil {
			return nil
		}

		// 区分"未知主机"和"指纹不匹配"
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			// 非 KeyError, 直接返回（如 host key not provided / 未知 transport error 等）
			return err
		}

		if len(keyErr.Want) == 0 {
			// 主机不在 known_hosts 中: TTY 下确认, 非 TTY 下自动 trust
			if isTerminal(os.Stdin) {
				return promptAndAddHost(path, hostname, remote, key)
			}
			// 非交互场景: 自动 trust (OpenSSH accept-new 语义)
			if err := appendKnownHost(path, hostname, key); err != nil {
				return fmt.Errorf("mysshw:: failed to auto-trust host %s: %w", hostname, err)
			}
			fmt.Fprintf(os.Stderr,
				"mysshw:: auto-trusted new host (non-interactive): %s [%s]; saved to %s\n",
				hostname, ssh.FingerprintSHA256(key), path)
			return nil
		}

		// 指纹不匹配: 默认严格拒绝; rotate flag 开启则覆盖
		if rotateOnMismatch {
			// 计算 known_hosts 中该 host 的旧指纹列表 (for audit message)
			var wantFPs []string
			for _, k := range keyErr.Want {
				wantFPs = append(wantFPs, ssh.FingerprintSHA256(k.Key))
			}
			// 5. ⚠️ 安全警告: rotate 会删除旧 key, 等价于降低了防 MITM 防护
			// 用户通过 --rotate-known-hosts flag 显式申请; 仍然关键盘写明 warning
			now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
			fmt.Fprintf(os.Stderr,
				"⚠️ mysshw:: ROTATING known_hosts (--rotate-known-hosts): host=%s old=%s new=%s at=%s\n",
				hostname,
				strings.Join(wantFPs, ", "),
				ssh.FingerprintSHA256(key),
				now,
			)
			if err := rotateKnownHost(path, hostname, key); err != nil {
				return fmt.Errorf("mysshw:: rotate known_hosts failed for %s: %w", hostname, err)
			}
			return nil // 接受新 key
		}

		// 默认严格拒绝 (不受交互模式影响)
		// knownhosts.KeyError 没有 Got 字段, 实际收到的 key 通过 callback 参数 key 传入
		var wantFPs []string
		for _, k := range keyErr.Want {
			wantFPs = append(wantFPs, ssh.FingerprintSHA256(k.Key))
		}
		return fmt.Errorf(HostKeyMismatchErrorStr,
			hostname,
			strings.Join(wantFPs, ", "),
			ssh.FingerprintSHA256(key))
	}, nil
}

// getHostKeyCallback 根据环境变量和节点配置返回合适的主机指纹校验策略
// 优先级: MYSSHW_SKIP_HOST_KEY_CHECK 环境变量 > per-node strict_host_key_checking > 默认 TOFU
func getHostKeyCallback(node *config.SSHNode) (ssh.HostKeyCallback, error) {
	if IsEnvTrue(EnvSkipHostKeyCheck) {
		// 环境变量: 最高优先级, 用于 CI/脚本场景临时跳过校验
		fmt.Fprintf(os.Stderr, "mysshw:: WARNING: %s=true, host key verification disabled (MITM risk)\n", EnvSkipHostKeyCheck)
		return ssh.InsecureIgnoreHostKey(), nil
	}

	if !node.IsStrictHostKeyChecking() {
		// per-node 配置: 测试环境跳过
		fmt.Fprintf(os.Stderr, "mysshw:: WARNING: host key verification disabled for %s@%s (MITM risk)\n",
			node.SetUser(), node.Host)
		return ssh.InsecureIgnoreHostKey(), nil
	}

	// 默认: TOFU (Trust On First Use)
	return trustOnFirstUseCallback(node)
}

// promptAndAddHost 提示用户确认新主机的指纹, 确认后追加到 known_hosts
//
// stdin 是 TTY 时, 等待用户输入 y/N
// stdin 不是 TTY 或读不到输入 (pipe/redirect/error) 时, 自动 trust
//
//	(OpenSSH StrictHostKeyChecking=accept-new; 让 AI/脚本场景不被卡住)
func promptAndAddHost(knownHostsPath, hostname string, remote net.Addr, key ssh.PublicKey) error {
	fingerprint := ssh.FingerprintSHA256(key)

	if isTerminal(os.Stdin) {
		fmt.Fprintf(os.Stderr, "\n[ %s ]\n", HostKeyFirstTimeTitleStr)
		fmt.Fprintf(os.Stderr, "  主机: %s\n", hostname)
		fmt.Fprintf(os.Stderr, "  指纹: %s\n", fingerprint)
		fmt.Fprintf(os.Stderr, HostKeyFirstTimePromptStr, hostname, fingerprint)

		reader := bufio.NewReader(os.Stdin)
		input, readErr := reader.ReadString('\n')
		if readErr != nil {
			// stdin 读失败 (CI 中 SIGTERM / pipe 关闭), 走自动 trust
			fmt.Fprintf(os.Stderr,
				"mysshw:: stdin read failed (%v); auto-trusting host %s [%s]\n",
				readErr, hostname, fingerprint)
		} else {
			input = strings.TrimSpace(strings.ToLower(input))
			if input != "y" && input != "yes" {
				return errors.New(HostKeyFirstTimeRejectedStr)
			}
		}
	}
	// 非 TTY 或 read 失败: 直接走 auto-trust

	if err := appendKnownHost(knownHostsPath, hostname, key); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, HostKeyFirstTimeAcceptedStr+"\n", knownHostsPath)
	return nil
}

// AppendKnownHost 将主机指纹追加到 known_hosts 文件 (公开 API)
//
// 使用 knownhosts.Line 标准化格式 (含端口信息)，用于 hostkey update 子命令。
// 并发安全: 内部上 knownHostsMu。
func AppendKnownHost(path, hostname string, key ssh.PublicKey) error {
	return appendKnownHost(path, hostname, key)
}

// appendKnownHost 将主机指纹追加到 known_hosts 文件
// 使用 knownhosts.Line 标准化格式（含端口信息）
func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()

	line := knownhosts.Line([]string{hostname}, key)
	if line == "" {
		return fmt.Errorf("mysshw:: failed to encode known_hosts line")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("mysshw:: failed to open known_hosts: %w", err)
	}
	defer f.Close()

	// knownhosts.Line 不含末尾换行符, 需要手动添加以保证多条记录独立成行
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("mysshw:: failed to write known_hosts: %w", err)
	}
	return nil
}

// RemoveKnownHost 删除 known_hosts 文件中 hostname 匹配的所有条目 (公开 API)
//
// 用于 hostkey remove/update 子命令。原子写 (tmp + rename)。
func RemoveKnownHost(path, hostname string) error {
	return removeKnownHost(path, hostname)
}

// removeKnownHost 删除 known_hosts 文件中 hostname 匹配的所有条目
//
// "匹配" 的语义: 同时匹配 host 字段 (含端口的 host 名)
// hashed entries (|1|salt|hash) 因为无法在不重新计算的情况下判断,
// 这里采用保守策略: 只删除与 hostname 字段完全匹配的明文条目 (含 [host]:port 形式)
//
// 并发安全: 内部上 knownHostsMu
func removeKnownHost(path, hostname string) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在视为"无旧条目可删"
		}
		return fmt.Errorf("mysshw:: failed to read known_hosts: %w", err)
	}

	// 重新组织文件: 保留未匹配的明文条目 (hashed entry 因无法判断保留)
	keepLines := []string{}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|1|") {
			// 注释行 / hashed entry: 不动
			keepLines = append(keepLines, line)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			keepLines = append(keepLines, line)
			continue
		}
		// fields[0] 是 hostname 部分 (含可能的 [host]:port / pattern1,pattern2)
		// 简化匹配: 检查 fields[0] 是否含 hostname 作为独立 token
		// (这是一个保守的字符串匹配, 不会误删无关条目)
		if !lineContainsHostToken(fields[0], hostname) {
			keepLines = append(keepLines, line)
		}
	}

	newContent := strings.Join(keepLines, "\n")
	if len(keepLines) > 0 && !strings.HasSuffix(newContent, "\n") && len(newContent) > 0 {
		newContent += "\n"
	}

	// 原子写: tmp + rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(newContent), 0600); err != nil {
		return fmt.Errorf("mysshw:: failed to write tmp known_hosts: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("mysshw:: failed to rename tmp known_hosts: %w", err)
	}
	return nil
}

// lineContainsHostToken 检查 known_hosts line 的 hostname 字段是否含 target
//
// known_hosts hostname 字段格式 (per RFC 4255):
//   - 单 hostname: "example.com"
//   - hostname:port: "example.com:2222"
//   - bracketed host:port: "[example.com]:2222"
//   - pattern list (multiple names / wildcards): "a.example.com,b.example.com"
//
// 简化: 先去掉括号 + port, 再按 ',' 分隔, 任一分隔项 == target 则 true。
func lineContainsHostToken(hostField, target string) bool {
	cleaned := hostField
	// 1. [host]:port 形式: 保留方括号内, 去掉 :port
	if strings.HasPrefix(cleaned, "[") && strings.Contains(cleaned, "]") {
		cleaned = strings.TrimPrefix(cleaned, "[")
		if idx := strings.Index(cleaned, "]"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
	} else if idx := strings.Index(cleaned, ":"); idx >= 0 {
		// 2. host:port 形式
		cleaned = cleaned[:idx]
	}
	// 3. 按 ',' 分隔 (多个备用名 / wildcards)
	for _, p := range strings.Split(cleaned, ",") {
		if p == target {
			return true
		}
	}
	return false
}

// rotateKnownHost 在 known_hosts 中 replace hostname 的所有旧条目为新 key
// 原子操作: 写 tmp 文件 + rename
func rotateKnownHost(path, hostname string, key ssh.PublicKey) error {
	// 1. 删除旧条目
	if err := removeKnownHost(path, hostname); err != nil {
		return err
	}
	// 2. 追加新 key
	return appendKnownHost(path, hostname, key)
}
