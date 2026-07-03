package ssh

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"mysshw/config"

	gossh "golang.org/x/crypto/ssh"
)

// MaxProxyChainLen 跳板链最大节点数 (含 target)
//
// 限制: 1 (target) + MaxProxyChainLen-1 次跳跃。
// 防止配置错误/恶意配置导致死循环或超长链。
const MaxProxyChainLen = 6

// resolveProxyChain 从 target 沿 Proxy 字段逐层解析, 返回 [target, proxy1, proxy2, ...]
//
// 第一项总是 target 本身, 后续是逐层 Proxy 引用的节点。
//   - 未设置 Proxy (空字符串) → 返回 [target]
//   - Proxy 指向不存在的节点 → 错误
//   - 检测环引用 → 错误
//   - 超过 MaxProxyHops → 错误
func resolveProxyChain(target *config.SSHNode, cfg *config.Configs) ([]*config.SSHNode, error) {
	if target == nil {
		return nil, fmt.Errorf("mysshw:: nil target node")
	}
	chain := []*config.SSHNode{target}
	seen := map[string]bool{target.Name: true}
	cur := target
	for cur.Proxy != "" {
		// 在 append 前检查 (下一个就是 chain[len(chain)])
		if len(chain) >= MaxProxyChainLen {
			return nil, fmt.Errorf("mysshw:: proxy chain too deep (max %d nodes); possible cycle: %v",
				MaxProxyChainLen, chainNames(chain))
		}
		next := FindNodeByName(cfg, cur.Proxy)
		if next == nil {
			return nil, fmt.Errorf("mysshw:: proxy node %q (referenced by %q) not found in config",
				cur.Proxy, cur.Name)
		}
		if seen[next.Name] {
			return nil, fmt.Errorf("mysshw:: proxy cycle: %s -> ... -> %s", cur.Name, next.Name)
		}
		seen[next.Name] = true
		chain = append(chain, next)
		cur = next
	}
	return chain, nil
}

// FindNodeByName 在 cfg 中按 name 精确查找 (跨所有 group)
func FindNodeByName(cfg *config.Configs, name string) *config.SSHNode {
	if cfg == nil {
		return nil
	}
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			if n != nil && n.Name == name {
				return n
			}
		}
	}
	return nil
}

func chainNames(chain []*config.SSHNode) []string {
	out := make([]string, 0, len(chain))
	for _, n := range chain {
		if n != nil {
			out = append(out, n.Name)
		}
	}
	return out
}

// dialViaProxyChain 建立跳板链 SSH 连接
//
// chain 的语义:
//
//	chain[0]   = target (要访问的最终节点)
//	chain[1..] = proxy 跳板节点 (按 SSH 客户端连接顺序)
//
// 实现:
//  1. 直接 Dial chain[0] (第一跳, 可能是 bastion)
//  2. 如果还有更多 hop, 在外层 SSH Client 上用 Client.Dial("tcp", next)
//     建立 direct-tcpip channel, 然后用 NewClientConn 在加密通道内
//     二次 SSH 握手连接下一跳
//  3. 全部 stack 在最内层 client
//
// 返回的最内层 *gossh.Client 可直接用于 Session 创建。
// 调用方负责 Close(), Close 会通过 SSH 协议栈级联关闭所有 transport。
//
// 清理: 中间 hop 失败时, 已建立的 client 一并 Close (防止资源泄漏)
func dialViaProxyChain(chain []*config.SSHNode) (*gossh.Client, error) {
	if len(chain) == 0 {
		return nil, errors.New("mysshw:: empty chain")
	}

	// 拨号第一跳: 直接走 plain TCP + SSH handshake
	first := chain[0]
	firstC := NewClientNonInteractive(first)
	if firstC == nil {
		return nil, fmt.Errorf("mysshw:: failed to build SSH client for %q (proxy hop 1)", first.Name)
	}
	firstDC, ok := firstC.(*defaultClient)
	if !ok || firstDC == nil {
		return nil, fmt.Errorf("mysshw:: internal: defaultClient cast failed at hop 1")
	}
	cli, err := firstDC.connect()
	if err != nil {
		return nil, fmt.Errorf("mysshw:: dial first hop %q: %w", first.Name, err)
	}
	transports := []*gossh.Client{cli}

	// 逐 hop 透传
	for i := 1; i < len(chain); i++ {
		cur := chain[i] // 当前要连的节点
		curC := NewClientNonInteractive(cur)
		if curC == nil {
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: failed to build SSH client for hop %q (%s -> %s)",
				cur.Name, chain[i-1].Name, cur.Name)
		}
		curDC, ok := curC.(*defaultClient)
		if !ok || curDC == nil {
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: internal: defaultClient cast failed at hop %d (%s)", i+1, cur.Name)
		}

		// 通过外层 SSH client dial 内层 host:port
		addr := net.JoinHostPort(cur.Host, strconv.Itoa(cur.SetPort()))
		if cur.Host == "" {
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: hop %q (%s -> %s) has empty host",
				cur.Name, chain[i-1].Name, cur.Name)
		}
		conn, dialErr := transports[i-1].Dial("tcp", addr)
		if dialErr != nil {
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: forward dial %s -> %s: %w",
				chain[i-1].Name, cur.Name, dialErr)
		}

		// 在已加密的 conn 上做 SSH 客户端握手
		ncc, chans, reqs, handshakeErr := gossh.NewClientConn(conn, addr, curDC.clientConfig)
		if handshakeErr != nil {
			_ = conn.Close()
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: SSH handshake over tunnel to %q: %w", cur.Name, handshakeErr)
		}
		cli = gossh.NewClient(ncc, chans, reqs)
		transports = append(transports, cli)
	}

	return cli, nil
}

func closeTransports(transport []*gossh.Client) {
	for i := len(transport) - 1; i >= 0; i-- {
		if transport[i] != nil {
			_ = transport[i].Close()
		}
	}
}
