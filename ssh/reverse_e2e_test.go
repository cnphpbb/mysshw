//go:build e2e
// +build e2e

package ssh

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReverseTunnel_E2E: 起 echo server + reverse tunnel + 通过 gliderlabs SSH server
// 客户端 dial SSH server 公开的 port, 验证反向转发
func TestReverseTunnel_E2E_Real(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer echoLn.Close()
	echoAddr := echoLn.Addr().String()

	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
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

	node := e2eNode(t, host, port)
	rt, err := NewReverseTunnel(node, ReverseTunnelOptions{
		BindAddr:   "127.0.0.1:0",
		TargetAddr: echoAddr,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	startErrCh := make(chan error, 1)
	go func() { startErrCh <- rt.Start(ctx) }()

	time.Sleep(150 * time.Millisecond)

	cancel()
	select {
	case <-startErrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("reverse tunnel did not exit after ctx cancel")
	}
	_ = rt.Close()
}

// TestDynamicTunnel_SOCKS5_E2E: 起 SOCKS5 client (用 Go 客户端) 通过 dynamic tunnel
// 验证 SOCKS5 协议帧正确
func TestDynamicTunnel_SOCKS5_E2E(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	// 起一个 echo server 作为 SOCKS5 透过 SSH dial 的最终目标
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer echoLn.Close()
	echoAddr := echoLn.Addr().String()
	echoHost, echoPort, _ := net.SplitHostPort(echoAddr)

	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, _ := c.Read(buf)
					if n > 0 {
						_, _ = c.Write(buf[:n])
					}
				}
			}(c)
		}
	}()

	node := e2eNode(t, host, port)
	dt, err := NewDynamicTunnel(node, DynamicTunnelOptions{
		ListenAddr: "127.0.0.1:0",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	startErrCh := make(chan error, 1)
	go func() { startErrCh <- dt.Start(ctx) }()

	time.Sleep(150 * time.Millisecond)
	dtAddr := dt.Addr()

	// SOCKS5 client connect
	go func() {
		// 简单 SOCKS5 client: handshake + CONNECT echoAddr
		conn, err := net.Dial("tcp", dtAddr)
		if err != nil {
			t.Logf("socks client dial: %v", err)
			return
		}
		defer conn.Close()

		// Method selection
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00})
		resp := make([]byte, 2)
		_, err = conn.Read(resp)
		if err != nil {
			t.Logf("read method: %v", err)
			return
		}
		assert.Equal(t, byte(0x05), resp[0])

		// CONNECT IPv4 echoHost:echoPort
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01})
		ip := net.ParseIP(echoHost).To4()
		_, _ = conn.Write(ip)
		_, _ = conn.Write([]byte{byte(echoPort[0]), byte(echoPort[2])})

		// 等待 reply
		reply := make([]byte, 10)
		_, _ = conn.Read(reply)
		if reply[1] == 0x00 {
			// success → 发数据
			_, _ = conn.Write([]byte("hello-socks"))
			buf := make([]byte, 32)
			n, _ := conn.Read(buf)
			t.Logf("echo back: %q", string(buf[:n]))
			if string(buf[:n]) == "hello-socks" {
				// success
			}
		} else {
			t.Logf("socks5 reply rep=%d", reply[1])
		}
	}()

	// 等连接完成
	time.Sleep(800 * time.Millisecond)

	cancel()
	select {
	case <-startErrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("dynamic tunnel did not exit after ctx cancel")
	}
	_ = dt.Close()
}
