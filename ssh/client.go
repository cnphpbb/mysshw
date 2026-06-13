package ssh

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mysshw/config"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

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
func genSSHConfig(node *config.SSHNode) *defaultClient {
	if node == nil {
		return nil
	}
	u, err := user.Current()
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var authMethods []ssh.AuthMethod

	var pemBytes []byte
	if node.KeyPath == "" {
		pemBytes, err = os.ReadFile(filepath.Join(u.HomeDir, ".ssh", "id_rsa"))
	} else {
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
		fmt.Println(err)
	} else {
		var signer ssh.Signer
		if node.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(node.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(pemBytes)
		}
		if err != nil {
			fmt.Println(err)
		} else {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	password := node.SetPassword()

	if password != nil {
		authMethods = append(authMethods, password)
	} else {
		// 当密码为空时，提示用户输入密码
		fmt.Print("请输入SSH密码: ")
		var passwordStr string
		fmt.Scanln(&passwordStr)
		if passwordStr != "" {
			password = ssh.Password(passwordStr)
			authMethods = append(authMethods, password)
		} else {
			fmt.Println("警告: 密码认证方式不可用，将尝试其他认证方式")
		}
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
	return genSSHConfig(node)
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

	client1, err := ssh.Dial("tcp", net.JoinHostPort(host, port), c.clientConfig)
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
