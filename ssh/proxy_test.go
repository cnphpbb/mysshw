package ssh

import (
	"net"
	"strconv"
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
		assert.Contains(t, err.Error(), "first hop", "error should reference first hop")
	case <-time.After(10 * time.Second):
		t.Fatal("connect hung > 10s: 无限递归 (回归 bug)")
	}
}
