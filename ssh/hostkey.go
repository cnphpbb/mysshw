package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

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

// getKnownHostsPath 返回 known_hosts 文件的绝对路径
// 优先使用 config.CFG.KnownHostsPath 自定义路径, 否则使用 ~/.ssh/known_hosts
func getKnownHostsPath() (string, error) {
	// 优先: 用户在配置文件中指定的自定义路径
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
//  1. 主机在 known_hosts 中: 严格校验, 指纹不匹配则拒绝连接（防中间人）
//  2. 主机不在 known_hosts 中: 提示用户确认, 信任后追加写入
//
// 注意: 此回调依赖 stdin 交互, 在非交互式场景 (CI/脚本) 中会失败
func trustOnFirstUseCallback() (ssh.HostKeyCallback, error) {
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

	// 包装: 处理"未知主机"分支
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := baseCallback(hostname, remote, key)
		if err == nil {
			return nil
		}

		// 区分"未知主机"和"指纹不匹配"
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			// 非 KeyError, 直接返回（如 host key not provided 等）
			return err
		}

		if len(keyErr.Want) == 0 {
			// 主机不在 known_hosts 中: 走交互式确认流程
			return promptAndAddHost(path, hostname, remote, key)
		}

		// 指纹不匹配: 详细诊断信息
		// 注意: knownhosts.KeyError 没有 Got 字段, 实际收到的 key 通过 callback 参数 key 传入
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

// promptAndAddHost 提示用户确认新主机的指纹, 确认后追加到 known_hosts
func promptAndAddHost(knownHostsPath, hostname string, remote net.Addr, key ssh.PublicKey) error {
	fingerprint := ssh.FingerprintSHA256(key)
	fmt.Fprintf(os.Stderr, "\n[ %s ]\n", HostKeyFirstTimeTitleStr)
	fmt.Fprintf(os.Stderr, "  主机: %s\n", hostname)
	fmt.Fprintf(os.Stderr, "  指纹: %s\n", fingerprint)
	fmt.Fprintf(os.Stderr, HostKeyFirstTimePromptStr, hostname, fingerprint)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New(HostKeyFirstTimeRejectedStr)
		}
		return fmt.Errorf("mysshw:: failed to read user input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input != "y" && input != "yes" {
		return errors.New(HostKeyFirstTimeRejectedStr)
	}

	if err := appendKnownHost(knownHostsPath, hostname, key); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, HostKeyFirstTimeAcceptedStr+"\n", knownHostsPath)
	return nil
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
