package ssh

import (
	"fmt"
	"net"
	"sync"

	"mysshw/config"

	gossh "golang.org/x/crypto/ssh"
)

// DialCaptureHostKeys 拨号并返回 server 提供的所有 host public keys
//
// 用法: 用户主动校验 server 的 key fingerprint (例如 hostkey update 子命令)
// 与 ssh.Dial 不同: 本函数额外把 handshake 阶段的 server host keys 收集返回。
//
// 实现: 临时注入 HostKeyCallback 抓 key, 然后调用原 callback (默认 TOFU)
// 注意: HostKeyCallback 在 handshake 阶段被回调一次 (per key), 多个 key 对应 server
//
//	多种加密算法 (ssh-ed25519 / ecdsa / rsa)
//
// 返回 cli.Close() 由调用方负责
func DialCaptureHostKeys(node *config.SSHNode) (cli *gossh.Client, hostKeys []gossh.PublicKey, err error) {
	if node == nil {
		return nil, nil, fmt.Errorf("mysshw:: nil node")
	}

	c := NewClientNonInteractive(node)
	if c == nil {
		return nil, nil, fmt.Errorf("mysshw:: failed to build SSH client for node %q", node.Name)
	}
	dc, ok := c.(*defaultClient)
	if !ok || dc == nil {
		return nil, nil, fmt.Errorf("mysshw:: internal: defaultClient cast failed")
	}

	// 注入 wrapper HostKeyCallback 抓 server keys
	var (
		mu     sync.Mutex
		keys   []gossh.PublicKey
		origCb = dc.clientConfig.HostKeyCallback
	)
	dc.clientConfig.HostKeyCallback = func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		mu.Lock()
		keys = append(keys, key)
		mu.Unlock()
		// 调用原 callback (TOFU 等); 它返回 err 时 handshake 失败
		if origCb != nil {
			return origCb(hostname, remote, key)
		}
		return nil
	}

	cli, err = dc.connect()
	if err != nil {
		return nil, nil, fmt.Errorf("mysshw:: dial: %w", err)
	}
	return cli, keys, nil
}
