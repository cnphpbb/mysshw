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
// chain 语义 (由 resolveProxyChain 填充):
//
//	chain[0]       = target  (要访问的最终节点, 内层)
//	chain[1..N-1]  = proxy   (跳板节点, 越靠后越外层)
//	chain[N-1]     = 最外层跳板 = 唯一 TCP 可直接触达的节点
//
// SSH 隧道堆叠顺序 (从客户端视角):
//
//	client ──TCP+SSH──> chain[N-1]   (outermost, 直连)
//	                        │
//	                        └──DirectTCPIP──> chain[N-2] ──SSH handshake──> ...
//	                                  │
//	                                  └────── ... ──> chain[0] (target, 最内层)
//
// 实现:
//  1. 第一跳 (outermost = chain[N-1]): 直接 plain TCP + SSH 握手 (这是 client 唯一能直连的)
//  2. 后续跳 (i = N-2 .. 0): 用当前 transports 末尾 client 的 Dial("tcp", chain[i])
//     开启 direct-tcpip 通道, 然后用 gossh.NewClientConn 在加密通道内做 SSH 客户端握手,
//     把新 client append 到 transports 末端 (保持 "最后一个 = 当前外层" 的不变量)
//  3. 返回 transports 末尾 (即 chain[0] target 的连接), 可直接 NewSession
//
// 错误信息: 区分 outermost / forward-dial / handshake 三类, 便于定位失败点。
//
// 清理: 中间 hop 失败时按 LIFO 顺序 Close 已建立 client, 防止资源泄漏
//
// ponytail: 第一跳必须用 gossh.Dial 而不是再走 defaultClient.connect(), 否则
// outermost.Proxy 非空 → connect() 再次进 dialViaProxyChain → 无限递归 (在引入
// 多层代理前这是个隐藏风险, 未来支持多层时务必保留此约束)。
func dialViaProxyChain(chain []*config.SSHNode) (*gossh.Client, error) {
	if len(chain) == 0 {
		return nil, errors.New("mysshw:: empty chain")
	}
	n := len(chain)
	chainLabel := make([]string, 0, n)
	for _, nd := range chain {
		if nd != nil {
			chainLabel = append(chainLabel, nd.Name)
		}
	}

	// === 步骤 1: 第一跳 = chain 末尾 (outermost proxy, 唯一 TCP 可直连) ===
	outermost := chain[n-1]
	if outermost.Host == "" {
		return nil, fmt.Errorf("mysshw:: outermost hop %q has empty host (chain: %v)", outermost.Name, chainLabel)
	}
	outermostC := NewClientNonInteractive(outermost)
	if outermostC == nil {
		return nil, fmt.Errorf("mysshw:: failed to build SSH client for outermost hop %q (chain: %v)", outermost.Name, chainLabel)
	}
	outermostDC, ok := outermostC.(*defaultClient)
	if !ok || outermostDC == nil {
		return nil, fmt.Errorf("mysshw:: internal: defaultClient cast failed at outermost hop 1 (%q)", outermost.Name)
	}
	outerAddr := net.JoinHostPort(outermost.Host, strconv.Itoa(outermost.SetPort()))
	outerCli, err := gossh.Dial("tcp", outerAddr, outermostDC.clientConfig)
	if err != nil {
		return nil, fmt.Errorf("mysshw:: dial outermost hop %q (%s): %w", outermost.Name, outerAddr, err)
	}
	transports := []*gossh.Client{outerCli}

	// === 步骤 2: 逐层往内 i = n-2; i >= 0; i-- ===
	// 每层: 用 transports 末尾 client 开 direct-tcpip 到 chain[i], 然后 SSH 握手
	for i := n - 2; i >= 0; i-- {
		inner := chain[i]
		if inner.Host == "" {
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: inner hop %q (in chain %v) has empty host", inner.Name, chainLabel)
		}
		// 通过当前最外层 transports[last] 隧道化访问 chain[i]
		outerHopName := chain[i+1].Name // 用于错误信息
		innerAddr := net.JoinHostPort(inner.Host, strconv.Itoa(inner.SetPort()))

		innerC := NewClientNonInteractive(inner)
		if innerC == nil {
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: failed to build SSH client for inner hop %q (%s -> %s, chain: %v)",
				inner.Name, outerHopName, inner.Name, chainLabel)
		}
		innerDC, ok := innerC.(*defaultClient)
		if !ok || innerDC == nil {
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: internal: defaultClient cast failed at inner hop (%s -> %q)", outerHopName, inner.Name)
		}

		// 步骤 2a: 通过 SSH 协议开 direct-tcpip 通道
		conn, dialErr := transports[len(transports)-1].Dial("tcp", innerAddr)
		if dialErr != nil {
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: forward dial via %q to %q (%s): %w",
				outerHopName, inner.Name, innerAddr, dialErr)
		}

		// 步骤 2b: 在已加密的 conn 上做 SSH 客户端握手
		ncc, chans, reqs, handshakeErr := gossh.NewClientConn(conn, innerAddr, innerDC.clientConfig)
		if handshakeErr != nil {
			_ = conn.Close()
			closeTransports(transports)
			return nil, fmt.Errorf("mysshw:: SSH handshake over tunnel %q -> %q: %w",
				outerHopName, inner.Name, handshakeErr)
		}
		cli := gossh.NewClient(ncc, chans, reqs)
		transports = append(transports, cli)
	}

	// transports 末尾就是 chain[0] (target) 的 SSH 连接
	return transports[len(transports)-1], nil
}

func closeTransports(transport []*gossh.Client) {
	for i := len(transport) - 1; i >= 0; i-- {
		if transport[i] != nil {
			_ = transport[i].Close()
		}
	}
}
