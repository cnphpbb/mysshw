package ssh

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"mysshw/config"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var plainPwdWarnOnce sync.Once

var (
	DefaultCiphers = []string{
		"aes128-ctr",
		"aes192-ctr",
		"aes256-ctr",
		"aes128-gcm@openssh.com",
		"chacha20-poly1305@openssh.com",
		"arcfour256",
		"arcfour128",
		"arcfour",
		"aes128-cbc",
		"3des-cbc",
		"blowfish-cbc",
		"cast128-cbc",
		"aes192-cbc",
		"aes256-cbc",
	}
)

// Client 定义SSH客户端接口
type Client interface {
	// Login 建立SSH连接并启动会话，sessionEndCallback在会话结束时被调用
	Login(sessionEndCallback func())
}

// DefaultClient 默认SSH客户端实现
// 实现了 Client 接口
type defaultClient struct {
	clientConfig *ssh.ClientConfig
	node         *config.SSHNode
}

// expandHomeDir 解析路径中的波浪号和$HOME环境变量，将它们替换为用户主目录
// 使用 currentHomeDir 变量获取家目录, 以便测试 mock
func expandHomeDir(path string) (string, error) {
	// 处理波浪号路径
	if strings.HasPrefix(path, "~") {
		// 获取当前用户主目录 (走 currentHomeDir 以便 mock)
		homeDir, err := currentHomeDir()
		if err != nil {
			return "", err
		}

		// 替换 ~ 为用户主目录
		if path == "~" {
			return homeDir, nil
		} else if len(path) > 1 {
			// 兼容不同操作系统的路径分隔符
			if path[1] == '/' || path[1] == '\\' {
				// 规范化路径分隔符，确保在任何操作系统上都能正确工作
				relativePath := path[2:]
				// 将反斜杠替换为正斜杠，然后让 filepath.Join 处理系统特定的分隔符
				relativePath = strings.ReplaceAll(relativePath, "\\", "/")
				return filepath.Join(homeDir, relativePath), nil
			}
		}
	} else if strings.HasPrefix(path, "$HOME") {
		// 获取当前用户主目录 (走 currentHomeDir 以便 mock)
		homeDir, err := currentHomeDir()
		if err != nil {
			return "", err
		}

		if path == "$HOME" {
			return homeDir, nil
		} else if len(path) > 5 {
			// 处理$HOME/或$HOME\开头的路径
			if path[5] == '/' || path[5] == '\\' {
				// 规范化路径分隔符，确保在任何操作系统上都能正确工作
				relativePath := path[6:]
				// 将反斜杠替换为正斜杠，然后让 filepath.Join 处理系统特定的分隔符
				relativePath = strings.ReplaceAll(relativePath, "\\", "/")
				return filepath.Join(homeDir, relativePath), nil
			}
		}
	}

	return path, nil
}

// genSSHConfig 生成SSH客户端配置
//
// allowStdin: true (Login 用) 时, bcrypt/空密码分支允许走 promptPassword;
//
//	false (Executor 用) 时, 这些分支直接返回 nil AuthMethod,
//	避免在无 terminal 的 AI/脚本场景下阻塞。
func genSSHConfig(node *config.SSHNode, allowStdin bool) *defaultClient {
	var err error
	if node == nil {
		return nil
	}
	// u, err := user.Current()
	// if err != nil {
	// 	fmt.Println(err)
	// 	return nil
	// }

	var authMethods []ssh.AuthMethod

	var pemBytes []byte
	// if node.KeyPath == "" {
	// 	pemBytes, err = os.ReadFile(filepath.Join(u.HomeDir, ".ssh", "id_rsa"))
	// } else {
	// 	// 处理波浪号路径, 将 ~ 替换为用户主目录;
	// 	// 兼容不同操作系统的路径分隔符,
	// 	// 主要是在Mac系统的配置文件，在Windows系统中使用不需要替换
	// 	keyPath, expandErr := expandHomeDir(node.KeyPath)
	// 	if expandErr != nil {
	// 		fmt.Printf("路径解析错误: %v\n", expandErr)
	// 	} else {
	// 		pemBytes, err = os.ReadFile(keyPath)
	// 	}
	// }

	if node.KeyPath != "" {
		// 处理波浪号路径, 将 ~ 替换为用户主目录;
		// 兼容不同操作系统的路径分隔符,
		// 主要是在Mac系统的配置文件，在Windows系统中使用不需要替换
		keyPath, expandErr := expandHomeDir(node.KeyPath)
		if expandErr != nil {
			fmt.Printf("路径解析错误: %v\n", expandErr)
		} else {
			pemBytes, err = os.ReadFile(keyPath)
		}
	}

	if err != nil {
		//fmt.Println(err)
		fmt.Printf("mysshw:: 无法读取私钥 %s: %v\n", node.KeyPath, err)

	} else {
		var signer ssh.Signer
		// 私钥口令也支持 ${ENV_VAR} 引用
		passphrase := ResolveEnvRef(node.Passphrase)
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(pemBytes)
		}
		if err != nil {
			fmt.Println(err)
		} else {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	// 密码支持多种格式 (Phase 3 引入):
	//   1. 明文:    "my-secret-password"
	//   2. ${ENV}:   "${MYSSHW_PWD_node1}"  (由 ResolveEnvRef 处理)
	//   3. bcrypt:  "$2a$10$..."           (Phase 2 自动检测, 需交互输入明文并验证)
	//   4. 空字符串: ""                    (Phase 3 新行为: 交互输入, 不写回)
	resolvedPwd := ResolveEnvRef(node.Password)
	if resolvedPwd != node.Password {
		fmt.Printf("mysshw:: password resolved from env var for %s@%s\n", node.SetUser(), node.Host)
	}

	password, pwdCleartext := resolvePasswordForSSH(node, resolvedPwd, allowStdin)
	_ = pwdCleartext // 明文由 Go GC 负责回收, 无需手动 zero
	// 注: Go string 不可变, 显式 zeroBytes 对 string 无效 (只能用于 []byte)
	// term.ReadPassword 返回的 []byte 转换为 string 后已无引用, 由 GC 回收

	if password != nil {
		authMethods = append(authMethods, password)
	} else if allowStdin {
		// 非交互路径下, AuthMethod=nil 是预期 (Exec 不需要密码); 静默即可
		fmt.Println("警告: 密码认证方式不可用，将尝试其他认证方式")
	}

	authMethods = append(authMethods, ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, 0, len(questions))
		for i, q := range questions {
			fmt.Print(q)
			if echos[i] {
				scan := bufio.NewScanner(os.Stdin)
				if scan.Scan() {
					answers = append(answers, scan.Text())
				}
				err := scan.Err()
				if err != nil {
					return nil, err
				}
			} else {
				b, err := term.ReadPassword(int(syscall.Stdin))
				if err != nil {
					return nil, err
				}
				fmt.Println()
				answers = append(answers, string(b))
			}
		}
		return answers, nil
	}))

	// 主机指纹校验策略选择 (详见 ssh/hostkey.go)
	// 优先级: MYSSHW_SKIP_HOST_KEY_CHECK env > per-node strict_host_key_checking > 默认 TOFU
	hostKeyCallback, cbErr := getHostKeyCallback(node)
	if cbErr != nil {
		fmt.Printf("mysshw:: failed to initialize host key verification: %s\n", cbErr)
		return nil
	}

	config := &ssh.ClientConfig{
		User:            node.SetUser(),
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         time.Second * 10,
	}

	config.SetDefaults()
	config.Ciphers = append(config.Ciphers, DefaultCiphers...)

	return &defaultClient{
		clientConfig: config,
		node:         node,
	}
}

// NewClient 创建SSH客户端
func NewClient(node *config.SSHNode) Client {
	return genSSHConfig(node, true)
}

// NewClientNonInteractive 构造一个不触发 stdin 交互的 SSH 客户端
//
// 与 NewClient 的区别: 当 node.Password 为空 / bcrypt 哈希时,
// NewClientNonInteractive 直接跳过, AuthMethod=nil (不读 stdin 不阻塞);
// 用于 ssh.Executor (AI / 脚本 / CI 场景, 这些场景没有可用 stdin)。
//
// 注意: 其他 ssh.AuthMethod (PublicKeys, KeyboardInteractive) 仍按现有逻辑处理。
func NewClientNonInteractive(node *config.SSHNode) Client {
	return genSSHConfig(node, false)
}

// connect 建立到节点的 SSH 连接
//
// 该方法抽离自原 Login 中的 Dial 段，供 RunCommand（非交互执行器）复用，
// 保持 Login 现有的密码重试分支不变。
//
// 不做交互式密码重试——非交互场景（exec）无 stdin 可用，调用方应确保
// 认证材料（password/keypath/${ENV}/bcrypt）在执行前已就绪。
//
// Proxy 跳板: 若节点 SSHNode.Proxy 非空, 自动走 resolveProxyChain + dialViaProxyChain;
//
//	不影响现有无 Proxy 路径。
func (c *defaultClient) connect() (*ssh.Client, error) {
	if c == nil || c.clientConfig == nil || c.node == nil {
		return nil, fmt.Errorf("mysshw:: defaultClient not initialised")
	}

	// Proxy 跳板链优先
	if c.node.Proxy != "" {
		chain, err := resolveProxyChain(c.node, config.CFG)
		if err != nil {
			return nil, err
		}
		if len(chain) > 1 {
			return dialViaProxyChain(chain)
		}
		// chain 只有自身 (理论上不应该到这, 防御编程)
	}

	host := c.node.Host
	port := strconv.Itoa(c.node.SetPort())
	if host == "" {
		return nil, fmt.Errorf("mysshw:: node %q has empty host", c.node.Name)
	}
	return ssh.Dial("tcp", net.JoinHostPort(host, port), c.clientConfig)
}

// Login 建立SSH连接并启动会话，sessionEndCallback在会话结束时被调用
func (c *defaultClient) Login(sessionEndCallback func()) {
	if c == nil {
		if sessionEndCallback != nil {
			sessionEndCallback()
		}
		return
	}
	host := c.node.Host
	port := strconv.Itoa(c.node.SetPort())
	//jNodes := c.node.Jump

	var client *ssh.Client

	client1, err := c.connect()
	client = client1
	if err != nil {
		msg := err.Error()
		// use terminal password retry
		if strings.Contains(msg, "no supported methods remain") && !strings.Contains(msg, "password") {
			fmt.Printf(SSHClientConnectPwdStr, c.clientConfig.User, host)
			var b []byte
			var readPasswordErr error

			b, readPasswordErr = term.ReadPassword(int(syscall.Stdin))
			if readPasswordErr == nil {

				p := string(b)
				if p != "" {
					c.clientConfig.Auth = append(c.clientConfig.Auth, ssh.Password(p))
				}
				fmt.Println()
				clientC, errclientC := ssh.Dial("tcp", net.JoinHostPort(host, port), c.clientConfig)
				if errclientC != nil {
					fmt.Println(errclientC)
					if sessionEndCallback != nil {
						sessionEndCallback()
					}
					return
				}
				client = clientC
				// 密码重试成功：重置 err 以避免外层 if err != nil 分支重复触发 callback
				err = nil
			}
		}
	}
	if err != nil {
		fmt.Println(err)
		if sessionEndCallback != nil {
			sessionEndCallback()
		}
		return
	}
	//}
	// client 可能仍为 nil（连接失败且未进入密码重试 / 重试也失败的场景），
	// 在 nil receiver 上调用 Close() 会 panic，这里加 nil check 保护。
	defer func() {
		if client != nil {
			client.Close()
		}
	}()

	fmt.Printf(SSHConnectInfoStr, c.node.SetPort(), c.node.SetUser(), host, string(client.ServerVersion()))

	session, err := client.NewSession()
	if err != nil {
		fmt.Println(err)
		if sessionEndCallback != nil {
			sessionEndCallback()
		}
		return
	}
	defer session.Close()
	defer func() {
		if sessionEndCallback != nil {
			sessionEndCallback()
		}
	}()

	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer term.Restore(fd, state)

	//OS:windows
	if runtime.GOOS == "windows" {
		fd = int(os.Stdout.Fd())
	}
	w, h, err := term.GetSize(fd)
	if err != nil {
		fmt.Println(err)
		return
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	err = session.RequestPty("xterm", h, w, modes)
	if err != nil {
		fmt.Println(err)
		return
	}

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	stdinPipe, err := session.StdinPipe()
	if err != nil {
		fmt.Println(err)
		return
	}

	err = session.Shell()
	if err != nil {
		fmt.Println(err)
		return
	}

	// change stdin to user
	go func() {
		_, err = io.Copy(stdinPipe, os.Stdin)
		// 忽略EOF错误，因为这是正常的会话结束情况
		if err != nil && err != io.EOF {
			fmt.Println(err)
		}
		session.Close()
	}()

	// interval get terminal size
	// fix resize issue
	go func() {
		var (
			ow = w
			oh = h
		)
		for {
			cw, ch, err := term.GetSize(fd)
			if err != nil {
				break
			}

			if cw != ow || ch != oh {
				err = session.WindowChange(ch, cw)
				if err != nil {
					break
				}
				ow = cw
				oh = ch
			}
			time.Sleep(time.Second)
		}
	}()

	// send keepalive
	go func() {
		for {
			time.Sleep(time.Second * 10)
			client.SendRequest("keepalive@openssh.com", false, nil)
		}
	}()

	session.Wait()
}

// resolvePasswordForSSH 解析 SSH 认证用的明文密码 + (可选) AuthMethod
// 输入 resolvedPwd 是 ResolveEnvRef 处理后的值 (可能是 明文 / bcrypt 哈希 / env 解析结果 / "")
// 返回:
//
//	password:     ssh.AuthMethod (明文) 或 nil (不可用)
//	pwdCleartext: 明文字符串, 供调用方 defer zeroBytes 清理
//
// 决策 B1 自动检测分支:
//   - bcrypt 哈希 → term.ReadPassword 交互输入 → VerifyPassword
//   - 空字符串  → term.ReadPassword 交互输入, **不写回 TOML** (默认行为, 用户可后续 init 向导设置)
//   - 明文     → 直接使用, 可选提示迁移为 bcrypt
func resolvePasswordForSSH(node *config.SSHNode, resolvedPwd string, allowStdin bool) (ssh.AuthMethod, string) {
	// 提前: 非交互场景 (AI/脚本/CI) — 不允许任何 stdin 操作
	// 限制仅针对需要用户输入密码的场景 (bcrypt / 空) — 明文/${ENV} 仍直接使用
	if !allowStdin {
		if config.IsBcryptHash(resolvedPwd) {
			fmt.Fprintln(os.Stderr, "mysshw:: bcrypt 哈希节点需要在终端中输入明文密码, 或改用 ${ENV} 引用")
			return nil, ""
		}
		if resolvedPwd == "" {
			// 空密码: 非交互场景不会提示, 直接 nil (仍可走密钥)
			return nil, ""
		}
		// 明文 + ${ENV}: 跟交互路径一致处理 (fall through)
	}

	// 情况 1: bcrypt 哈希 → 交互输入 + 验证
	if config.IsBcryptHash(resolvedPwd) {
		cleartext, err := promptPassword(node, "TOML 中存的是 bcrypt 哈希, 请输入明文密码以验证: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "mysshw:: failed to read password: %v\n", err)
			return nil, ""
		}
		if err := config.VerifyPassword(resolvedPwd, cleartext); err != nil {
			fmt.Fprintln(os.Stderr, "mysshw:: 密码验证失败 (bcrypt mismatch)")
			zeroBytes(cleartext)
			return nil, ""
		}
		fmt.Fprintln(os.Stderr, "mysshw:: bcrypt 验证通过")
		return ssh.Password(cleartext), cleartext
	}

	// 情况 2: 明文 (可能含 ${ENV} 未解析) → 直接使用, 可选迁移提示
	if resolvedPwd != "" {
		// 进程级一次性提示用户改用更安全的方式
		plainPwdWarnOnce.Do(func() {
			if isTerminal(os.Stdin) {
				fmt.Fprintf(os.Stderr, "mysshw:: 注意: TOML 中存有明文密码, 建议改用 ${ENV_VAR} 或 bcrypt 哈希 (参见 `mysshw init` 向导)\n")
			}
		})
		return ssh.Password(resolvedPwd), resolvedPwd
	}

	// 情况 3: 空字符串 → 交互输入, 不写回 (默认行为, Phase 3 核心变更)
	cleartext, err := promptPassword(node, "请输入 SSH 密码 (不写入 TOML): ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mysshw:: failed to read password: %v\n", err)
		return nil, ""
	}
	if cleartext == "" {
		fmt.Fprintln(os.Stderr, "mysshw:: 密码为空, 跳过密码认证")
		return nil, ""
	}
	fmt.Fprintln(os.Stderr, "mysshw:: 密码已临时读取 (本次会话不持久化)")
	return ssh.Password(cleartext), cleartext
}

// promptPassword 用 term.ReadPassword 安全读取密码 (不回显)
// 在非 terminal stdin (CI/管道) 下降级为 fmt.Scanln
func promptPassword(node *config.SSHNode, prompt string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s", prompt)

	if isTerminal(os.Stdin) {
		fd := int(os.Stdin.Fd())
		// Windows 上 term 需要 stdout fd
		if runtime.GOOS == "windows" {
			fd = int(os.Stdout.Fd())
		}
		b, err := term.ReadPassword(fd)
		if err != nil {
			return "", err
		}
		fmt.Fprintln(os.Stderr) // 换行
		return string(b), nil
	}

	// 非 terminal 降级路径
	var pw string
	if _, err := fmt.Scanln(&pw); err != nil {
		return "", err
	}
	return pw, nil
}

// isTerminal 检查 fd 是否关联到 terminal
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// zeroBytes 覆写 byte slice 内容 (降低内存中明文密码残留时间)
// 注: Go string 不可变, 此函数对 string 无效; 仅供 []byte 类型的明文使用
// 当前流程 term.ReadPassword 返回 []byte 转为 string 后已无引用, 由 GC 回收,
// 故此函数暂未在主流程调用, 保留供未来若改用 []byte 传递明文时使用。
func zeroBytes(s string) {
	b := []byte(s)
	for i := range b {
		b[i] = 0
	}
	_ = b
}
