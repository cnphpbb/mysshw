package ssh

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mysshw/config"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
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

type Client interface {
	Login()
}

type defaultClient struct {
	clientConfig *ssh.ClientConfig
	node         *config.SSHNode
}

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
		pemBytes, err = os.ReadFile(path.Join(u.HomeDir, ".ssh/id_rsa"))
	} else {
		pemBytes, err = os.ReadFile(node.KeyPath)
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
				b, err := terminal.ReadPassword(int(syscall.Stdin))
				if err != nil {
					return nil, err
				}
				fmt.Println()
				answers = append(answers, string(b))
			}
		}
		return answers, nil
	}))

	config := &ssh.ClientConfig{
		User:            node.SetUser(),
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Second * 10,
	}

	config.SetDefaults()
	config.Ciphers = append(config.Ciphers, DefaultCiphers...)

	return &defaultClient{
		clientConfig: config,
		node:         node,
	}
}

func NewClient(node *config.SSHNode) Client {
	return genSSHConfig(node)
}

func (c *defaultClient) Login() {
	if c == nil {
		return
	}
	host := c.node.Host
	port := strconv.Itoa(c.node.SetPort())
	//jNodes := c.node.Jump

	var client *ssh.Client

	//if len(jNodes) > 0 {
	//	jNode := jNodes[0]
	//	jc := genSSHConfig(jNode)
	//	proxyClient, err := ssh.Dial("tcp", net.JoinHostPort(jNode.Host, strconv.Itoa(jNode.port())), jc.clientConfig)
	//	if err != nil {
	//		mysshw.l.Error(err)
	//		return
	//	}
	//	conn, err := proxyClient.Dial("tcp", net.JoinHostPort(host, port))
	//	if err != nil {
	//		mysshw.l.Error(err)
	//		return
	//	}
	//	ncc, chans, reqs, err := ssh.NewClientConn(conn, net.JoinHostPort(host, port), c.clientConfig)
	//	if err != nil {
	//		mysshw.l.Error(err)
	//		return
	//	}
	//	client = ssh.NewClient(ncc, chans, reqs)
	//} else {
	client1, err := ssh.Dial("tcp", net.JoinHostPort(host, port), c.clientConfig)
	client = client1
	if err != nil {
		msg := err.Error()
		// use terminal password retry
		if strings.Contains(msg, "no supported methods remain") && !strings.Contains(msg, "password") {
			fmt.Printf("%s@%s's password:", c.clientConfig.User, host)
			var b []byte
			b, err = terminal.ReadPassword(int(syscall.Stdin))
			if err == nil {
				p := string(b)
				if p != "" {
					c.clientConfig.Auth = append(c.clientConfig.Auth, ssh.Password(p))
				}
				fmt.Println()
				client, err = ssh.Dial("tcp", net.JoinHostPort(host, port), c.clientConfig)
			}
		}
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	//}
	defer client.Close()

	fmt.Printf("connect server ssh -p %d %s@%s version: %s \n", c.node.SetPort(), c.node.SetUser(), host, string(client.ServerVersion()))

	session, err := client.NewSession()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer session.Close()

	fd := int(os.Stdin.Fd())
	state, err := terminal.MakeRaw(fd)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer terminal.Restore(fd, state)

	//OS:windows
	if runtime.GOOS == "windows" {
		fd = int(os.Stdout.Fd())
	}
	w, h, err := terminal.GetSize(fd)
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

	// then callback
	//for i := range c.node.CallbackShells {
	//	shell := c.node.CallbackShells[i]
	//	time.Sleep(shell.Delay * time.Millisecond)
	//	stdinPipe.Write([]byte(shell.Cmd + "\r"))
	//}

	// change stdin to user
	go func() {
		_, err = io.Copy(stdinPipe, os.Stdin)
		fmt.Println(err)
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
			cw, ch, err := terminal.GetSize(fd)
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
