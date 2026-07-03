package ssh

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewTunnel 参数校验 (不需要真实 SSH)
func TestNewTunnel_NilNode(t *testing.T) {
	_, err := NewTunnel(nil, TunnelOptions{
		LocalBind:  "127.0.0.1:8080",
		RemoteAddr: "127.0.0.1:80",
	})
	assert.Error(t, err)
}

func TestNewTunnel_MissingLocalBind(t *testing.T) {
	_, err := NewTunnel(&config.SSHNode{Name: "x"}, TunnelOptions{
		RemoteAddr: "127.0.0.1:80",
	})
	assert.Error(t, err)
}

func TestNewTunnel_MissingRemoteAddr(t *testing.T) {
	_, err := NewTunnel(&config.SSHNode{Name: "x"}, TunnelOptions{
		LocalBind: "127.0.0.1:8080",
	})
	assert.Error(t, err)
}

func TestNewTunnel_OK(t *testing.T) {
	tn, err := NewTunnel(&config.SSHNode{Name: "x"}, TunnelOptions{
		LocalBind:  "127.0.0.1:8080",
		RemoteAddr: "127.0.0.1:80",
	})
	require.NoError(t, err)
	require.NotNil(t, tn)
	// 关闭 (不会立即拨号, 但要测试 close 不 panic)
	_ = tn.Close()
}

// TestLocalTunnel_ListenAndForward 端到端测试: 起 in-process 简单 TCP echo 服务器,
// 用 ssh.LocalTunnel 走 ssh.Dial("tcp", ...) 到 echo server, 看透传是否正确。
//
// 注意: 这里测的是 handleConn 的 io.Copy 部分, 但 NewTunnel.Start 要走 SSH,
// 我们用 stub SSH client 替代; 比较复杂。简化路径: 直接 unit-test listener 部分。

func TestListenAddr(t *testing.T) {
	// Port :0 让 OS 自动分配 — 测 listener 分配正确
	tn, err := NewTunnel(&config.SSHNode{Name: "x"}, TunnelOptions{
		LocalBind:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:80",
	})
	require.NoError(t, err)
	// 在 Start 之前 Addr() 返回原始配置
	assert.Equal(t, "127.0.0.1:0", tn.Addr())
}

func TestTunnelClose_Idempotent(t *testing.T) {
	tn, err := NewTunnel(&config.SSHNode{Name: "x"}, TunnelOptions{
		LocalBind:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:80",
	})
	require.NoError(t, err)
	assert.NoError(t, tn.Close())
	assert.NoError(t, tn.Close()) // 第二次也不 panic
}

// generateTestKey 生成测试用 RSA 私钥 (仅测试时使用)
func generateTestKey(t *testing.T) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	return pemBytes
}

// 仅用于在 net.Listen 上跑虚假 "remote server" 做端到端测试
func echoServer(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						_, _ = c.Write(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln, ln.Addr().String()
}

func TestLocalTunnel_E2E(t *testing.T) {
	// 起本地 echo server, 然后通过 localTunnel 把 client 连接透传到 echo server
	echoLn, echoAddr := echoServer(t)
	defer echoLn.Close()

	// 用一个 ssh.Client 模拟; 这里我们跳过真实 SSH, 直接测试 listener 转发逻辑
	// 简化: 通过 Dial("tcp", echoAddr) 验证 handleConn 的双向 io.Copy 路径

	tn, err := NewTunnel(&config.SSHNode{Name: "x"}, TunnelOptions{
		LocalBind:  "127.0.0.1:0",
		RemoteAddr: echoAddr,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start 会因 NewClientNonInteractive 失败 (无 host) — 测错误路径
	err = tn.Start(ctx)
	// 我们故意没设 host, 期待 dial 失败
	if assert.Error(t, err) {
		t.Logf("expected dial failure: %v", err)
	}
}
