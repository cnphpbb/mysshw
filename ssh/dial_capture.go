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
// 实现: 临时注入 HostKeyCallback 抓 key, 然后调用原 callback (默认 TOFU accept-new)
// 注意: HostKeyCallback 在 handshake 阶段被回调一次 (per key), 多个 key 对应 server
//
//	多种加密算法 (ssh-ed25519 / ecdsa / rsa)
//
// 返回 cli.Close() 由调用方负责
func DialCaptureHostKeys(node *config.SSHNode) (cli *gossh.Client, hostKeys []gossh.PublicKey, err error) {
	return dialAndHook(node, true)
}

// DialVerifyHostKeys 拨号但**不写** known_hosts, 返回 server 的所有 host keys
//
// 与 DialCaptureHostKeys 的区别:
//   - DialVerifyHostKeys 临时将 HostKeyCallback 设为 always-accept (不调用 TOFU)
//   - 适合 "获取 server key 但不污染 known_hosts" 的场景:
//   - hostkey update 在 --expect-fingerprint 不匹配时应拒绝后续写入
//   - 测试/审计
//   - DialCaptureHostKeys 保留原 callback 行为, 可能触发 accept-new 写入
//
// 返回 cli.Close() 由调用方负责
func DialVerifyHostKeys(node *config.SSHNode) (cli *gossh.Client, hostKeys []gossh.PublicKey, err error) {
	return dialAndHook(node, false)
}

// dialAndHook 是上述两个函数的共享实现
//
//	writeOnAccept=true  : 调原 callback (可能 accept-new 触发 known_hosts 写入)
//	writeOnAccept=false : 临时设 always-accept callback (不写 known_hosts)
func dialAndHook(node *config.SSHNode, writeOnAccept bool) (cli *gossh.Client, hostKeys []gossh.PublicKey, err error) {
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

	var (
		mu     sync.Mutex
		keys   []gossh.PublicKey
		origCb = dc.clientConfig.HostKeyCallback
	)
	dc.clientConfig.HostKeyCallback = func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		mu.Lock()
		keys = append(keys, key)
		mu.Unlock()
		if !writeOnAccept {
			return nil
		}
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
