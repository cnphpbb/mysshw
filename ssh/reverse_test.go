package ssh

import (
	"net"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReverseTunnel_NilNode(t *testing.T) {
	_, err := NewReverseTunnel(nil, ReverseTunnelOptions{
		BindAddr: ":8080", TargetAddr: "127.0.0.1:80",
	})
	assert.Error(t, err)
}

func TestNewReverseTunnel_MissingBindAddr(t *testing.T) {
	_, err := NewReverseTunnel(&config.SSHNode{Name: "x"}, ReverseTunnelOptions{
		TargetAddr: "127.0.0.1:80",
	})
	assert.Error(t, err)
}

func TestNewReverseTunnel_MissingTargetAddr(t *testing.T) {
	_, err := NewReverseTunnel(&config.SSHNode{Name: "x"}, ReverseTunnelOptions{
		BindAddr: ":8080",
	})
	assert.Error(t, err)
}

func TestNewReverseTunnel_OK(t *testing.T) {
	rt, err := NewReverseTunnel(&config.SSHNode{Name: "x"}, ReverseTunnelOptions{
		BindAddr: ":8080", TargetAddr: "127.0.0.1:80",
	})
	require.NoError(t, err)
	assert.NotNil(t, rt)
	assert.Equal(t, ":8080", rt.BoundAddr())

	// Close 不 panic
	assert.NoError(t, rt.Close())
}

func TestReverseTunnelClose_Idempotent(t *testing.T) {
	rt, err := NewReverseTunnel(&config.SSHNode{Name: "x"}, ReverseTunnelOptions{
		BindAddr: ":8080", TargetAddr: "127.0.0.1:80",
	})
	require.NoError(t, err)
	assert.NoError(t, rt.Close())
	assert.NoError(t, rt.Close())
}

// 端到端: 起 echo server + reverse tunnel, 通过 SSH server 端口连接到 echo
// 用 gliderlabs 起一个 SSH server, 监听 :0 上 client.Listen (reverse tunnel)
// 客户端从 :0 上 client.Dial → 应被 reverse tunnel 转给 echo
//
// 需要 gliderlabs/ssh, 仅在 build tag e2e 下编译

func TestReverseTunnel_E2E_Stub(t *testing.T) {
	t.Skip("see reverse_e2e_test.go for real e2e (build tag e2e)")
}

func TestNewDynamicTunnel_NilNode(t *testing.T) {
	_, err := NewDynamicTunnel(nil, DynamicTunnelOptions{ListenAddr: "127.0.0.1:1080"})
	assert.Error(t, err)
}

func TestNewDynamicTunnel_MissingAddr(t *testing.T) {
	_, err := NewDynamicTunnel(&config.SSHNode{Name: "x"}, DynamicTunnelOptions{})
	assert.Error(t, err)
}

func TestNewDynamicTunnel_OK(t *testing.T) {
	dt, err := NewDynamicTunnel(&config.SSHNode{Name: "x"}, DynamicTunnelOptions{
		ListenAddr: "127.0.0.1:1080",
	})
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:1080", dt.Addr())
	assert.NoError(t, dt.Close())
}

func TestDynamicTunnel_Close_Idempotent(t *testing.T) {
	dt, err := NewDynamicTunnel(&config.SSHNode{Name: "x"}, DynamicTunnelOptions{
		ListenAddr: "127.0.0.1:1080",
	})
	require.NoError(t, err)
	assert.NoError(t, dt.Close())
	assert.NoError(t, dt.Close())
}

// SOCKS5 协议单元测试 (无 server): 直接调 socks5Handshake + socks5Reply
// 通过 net.Pipe 模拟 client ↔ server
//
// 注意: net.Pipe 是同步全双工, 一边 Write 必须对端 Read 才能不阻塞
func TestSOCKS5_Handshake_Success(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		<-done
	}()

	// 客户端: 写握手 → 读 reply → 验证
	go func() {
		defer close(done)
		_, _ = clientConn.Write([]byte{0x05, 0x01, 0x00})
		resp := make([]byte, 2)
		_, _ = clientConn.Read(resp)
		assert.Equal(t, byte(0x05), resp[0])
		assert.Equal(t, byte(0x00), resp[1])
	}()

	err := socks5Handshake(serverConn)
	assert.NoError(t, err)
}

func TestSOCKS5_Handshake_NoAcceptableMethod(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		<-done
	}()

	go func() {
		defer close(done)
		_, _ = clientConn.Write([]byte{0x05, 0x01, 0x01})
		// 读握手响应 (server 应回 0x05 0xFF)
		resp := make([]byte, 2)
		_, _ = clientConn.Read(resp)
	}()

	err := socks5Handshake(serverConn)
	assert.Error(t, err)
}

func TestSOCKS5_Handshake_BadVersion(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		<-done
	}()

	go func() {
		defer close(done)
		// SOCKS4 而非 5
		_, _ = clientConn.Write([]byte{0x04, 0x01, 0x00})
		// server 检测到 bad version 会返回, 但仍需 drain
		buf := make([]byte, 16)
		_, _ = clientConn.Read(buf)
	}()

	err := socks5Handshake(serverConn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not SOCKS5")
}

func TestSOCKS5_Reply_WritesCorrectBytes(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		<-done
	}()

	go func() {
		defer close(done)
		buf := make([]byte, 10)
		_, _ = clientConn.Read(buf)
		// 期望 [0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]
		assert.Equal(t, byte(0x05), buf[0])
		assert.Equal(t, byte(0x00), buf[1])
	}()

	err := socks5Reply(serverConn, 0x00)
	assert.NoError(t, err)
}

func TestSOCKS5_Reply_Failure(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		<-done
	}()

	go func() {
		defer close(done)
		buf := make([]byte, 10)
		_, _ = clientConn.Read(buf)
		assert.Equal(t, byte(0x01), buf[1])
	}()
	err := socks5Reply(serverConn, 0x01)
	assert.NoError(t, err)
}
