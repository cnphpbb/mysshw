package ssh

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProxyChain_NoProxy(t *testing.T) {
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "G", SSHNodes: []*config.SSHNode{{Name: "n1", Host: "10.0.0.1"}}},
		},
	}
	chain, err := resolveProxyChain(cfg.Nodes[0].SSHNodes[0], cfg)
	require.NoError(t, err)
	assert.Len(t, chain, 1)
	assert.Equal(t, "n1", chain[0].Name)
}

func TestResolveProxyChain_SingleHop(t *testing.T) {
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "Bastion", SSHNodes: []*config.SSHNode{{Name: "bastion", Host: "b.example.com", User: "u"}}},
			{Groups: "Internal", SSHNodes: []*config.SSHNode{
				{Name: "internal", Host: "10.0.0.5", User: "u", Proxy: "bastion"},
			}},
		},
	}
	internal := cfg.Nodes[1].SSHNodes[0]
	chain, err := resolveProxyChain(internal, cfg)
	require.NoError(t, err)
	require.Len(t, chain, 2)
	assert.Equal(t, "internal", chain[0].Name)
	assert.Equal(t, "bastion", chain[1].Name)
}

func TestResolveProxyChain_MultiHop(t *testing.T) {
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "A", SSHNodes: []*config.SSHNode{{Name: "a", Host: "1.1.1.1", Proxy: "b"}}},
			{Groups: "B", SSHNodes: []*config.SSHNode{{Name: "b", Host: "2.2.2.2", Proxy: "c"}}},
			{Groups: "C", SSHNodes: []*config.SSHNode{{Name: "c", Host: "3.3.3.3"}}},
		},
	}
	chain, err := resolveProxyChain(cfg.Nodes[0].SSHNodes[0], cfg)
	require.NoError(t, err)
	require.Len(t, chain, 3)
	assert.Equal(t, "a", chain[0].Name)
	assert.Equal(t, "b", chain[1].Name)
	assert.Equal(t, "c", chain[2].Name)
}

func TestResolveProxyChain_ProxyNotFound(t *testing.T) {
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "X", SSHNodes: []*config.SSHNode{{Name: "x", Host: "1.1.1.1", Proxy: "ghost"}}},
		},
	}
	_, err := resolveProxyChain(cfg.Nodes[0].SSHNodes[0], cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestResolveProxyChain_CycleDetect(t *testing.T) {
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "X", SSHNodes: []*config.SSHNode{{Name: "a", Host: "1.1.1.1", Proxy: "b"}}},
			{Groups: "Y", SSHNodes: []*config.SSHNode{{Name: "b", Host: "2.2.2.2", Proxy: "a"}}},
		},
	}
	a := cfg.Nodes[0].SSHNodes[0]
	_, err := resolveProxyChain(a, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestResolveProxyChain_TooDeep(t *testing.T) {
	// 构造 7 层 chain (超过 MaxProxyChainLen=6)
	cfg := &config.Configs{}
	for i := 0; i < 7; i++ {
		next := ""
		if i < 6 {
			next = "n" + string(rune('1'+i+1))
		}
		var proxy string
		if next != "" {
			proxy = next
		}
		cfg.Nodes = append(cfg.Nodes, config.Nodes{
			Groups: "G",
			SSHNodes: []*config.SSHNode{
				{Name: "n" + string(rune('1'+i)), Host: "1.1.1.1", Proxy: proxy},
			},
		})
	}
	_, err := resolveProxyChain(cfg.Nodes[0].SSHNodes[0], cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too deep")
}

func TestFindNodeByName(t *testing.T) {
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "G1", SSHNodes: []*config.SSHNode{{Name: "alpha"}, {Name: "beta"}}},
			{Groups: "G2", SSHNodes: []*config.SSHNode{{Name: "gamma"}}},
		},
	}
	assert.NotNil(t, FindNodeByName(cfg, "alpha"))
	assert.NotNil(t, FindNodeByName(cfg, "beta"))
	assert.NotNil(t, FindNodeByName(cfg, "gamma"))
	assert.Nil(t, FindNodeByName(cfg, "missing"))
	assert.Nil(t, FindNodeByName(nil, "alpha"))
}

func TestFindNodeByName_SkipsNilNodes(t *testing.T) {
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "G", SSHNodes: []*config.SSHNode{nil, {Name: "real"}}},
		},
	}
	assert.Nil(t, FindNodeByName(cfg, "noname"))
	assert.NotNil(t, FindNodeByName(cfg, "real"))
}

func TestChainNames(t *testing.T) {
	n1 := &config.SSHNode{Name: "n1"}
	n2 := &config.SSHNode{Name: "n2"}
	out := chainNames([]*config.SSHNode{n1, n2, nil})
	assert.Equal(t, []string{"n1", "n2"}, out)
}

// TestConnectWithProxy_DoesNotRecurse 回归测试: 节点 Proxy 非空时,
// dialViaProxyChain 第一跳必须直接 ssh.Dial, 否则 chain[0].Proxy 非空
// → connect() 再次进 dialViaProxyChain → 无限递归 (表现为 hang)。
//
// 用一个 accept-立即-close 的 TCP listener 作目标: TCP 能成, SSH 必败。
// 这里允许 bastion 和 target 共享同一 listener (这是被测代码路径不允许它们指向
// 不可达的内网 host 的近似场景; 强灬语义验证交给下方 TestDialViaProxyChain_TwoDistinctEndpoints
// 用两个独立 listener 区分 dial 顺序)。
func TestConnectWithProxy_DoesNotRecurse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	host, portStr, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	bastion := &config.SSHNode{
		Name: "bastion", Host: host, Port: port, User: "u", Password: "p",
	}
	target := &config.SSHNode{
		Name: "target", Host: host, Port: port, User: "u", Password: "p",
		Proxy: "bastion",
	}
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "G", SSHNodes: []*config.SSHNode{bastion, target}},
		},
	}
	oldCFG := config.CFG
	config.CFG = cfg
	defer func() { config.CFG = oldCFG }()

	c := NewClientNonInteractive(target).(*defaultClient)

	done := make(chan error, 1)
	go func() {
		_, err := c.connect()
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err, "expected connection error (mock server closes immediately), got nil")
		// 错误可能是 "dial outermost hop" 或 "forward dial" (取决于 SSH handshake 何时失败)
		// 关键是必须有错且不超时。提示二人足。
		errStr := err.Error()
		assert.True(t, strings.Contains(errStr, "outermost") || strings.Contains(errStr, "forward") || strings.Contains(errStr, "handshake"),
			"error should reference hop-type (outermost/forward/handshake); got %q", errStr)
	case <-time.After(10 * time.Second):
		t.Fatal("connect hung > 10s: 无限递归 (回归 bug)")
	}
}

// TestDialViaProxyChain_TwoDistinctEndpoints 是 dial-order 的严格回归测试。
//
// 场景:
//   - bastion 端口 主动关闭后无人监听
//   - target 端口  跑一个正常工作的 SSH server
//
// 期望 (修复后):
//
//	chain = [target, bastion]
//	第一跳 = chain[len-1] = bastion (refused) → 错 "dial outermost hop \"bastion\""
//
// 旧 (修复前) buggy 行为:
//
//	第一跳 = chain[0] = target (working) → 连接成功, 进入下一步
//	→ 错误不是 "bastion refused", 而是 后期的 “SSH handshake over tunnel”
//	→ 或甚至成功走到 target
//
// 断言: 错误信息必须提到 bastion; 从而区分修复前/后。
func TestDialViaProxyChain_TwoDistinctEndpoints(t *testing.T) {
	// target: 跑一个接受连接后立即关闭的 mock server (区别于下面的 bastion 关闭端口)
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer targetListener.Close()
	go func() {
		for {
			c, aErr := targetListener.Accept()
			if aErr != nil {
				return
			}
			_ = c.Close()
		}
	}()
	targetHost, targetPortStr, err := net.SplitHostPort(targetListener.Addr().String())
	require.NoError(t, err)
	targetPort, err := strconv.Atoi(targetPortStr)
	require.NoError(t, err)

	// bastion: 主动拿一个端口后立即关闭 → connection refused
	bastionListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	bastionHost, bastionPortStr, err := net.SplitHostPort(bastionListener.Addr().String())
	require.NoError(t, err)
	bastionPort, err := strconv.Atoi(bastionPortStr)
	require.NoError(t, err)
	require.NoError(t, bastionListener.Close()) // 现在 bastion 端口没人监听

	bastion := &config.SSHNode{
		Name: "bastion", Host: bastionHost, Port: bastionPort, User: "u", Password: "p",
	}
	target := &config.SSHNode{
		Name: "target", Host: targetHost, Port: targetPort, User: "u", Password: "p",
		Proxy: "bastion",
	}
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "G", SSHNodes: []*config.SSHNode{bastion, target}},
		},
	}
	oldCFG := config.CFG
	config.CFG = cfg
	defer func() { config.CFG = oldCFG }()

	c := NewClientNonInteractive(target).(*defaultClient)

	done := make(chan error, 1)
	go func() {
		_, err := c.connect()
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err, "expected connect error (bastion 端口已关闭)")
		errStr := err.Error()
		// 修复后: 必须提及 bastion (因为失败发生在 bastion 这一跳)
		// 修复前: 会提到 target 而不是 bastion
		assert.Contains(t, errStr, "bastion",
			"错误必须指明是 bastion 失败 (修复后第一跳是 bastion); 当前: %q", errStr)
		assert.Contains(t, errStr, "outermost",
			"错误必须包含 outermost 关键词 (修复后第一跳叫 outermost hop); 当前: %q", errStr)
		assert.NotContains(t, errStr, "forward dial",
			"修复后连接不应该走到 forward dial (那说明已经走错了顺序); 当前: %q", errStr)
	case <-time.After(10 * time.Second):
		t.Fatal("connect hung > 10s: 无限递归 (回归 bug)")
	}
}
