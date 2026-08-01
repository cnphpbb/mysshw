//go:build e2e
// +build e2e

// mcp_parallel_e2e_test.go: 端到端复现/回归测试 — MCP parallel=N flag 是否真的限制
// BatchExecutor.RunFanOut 的 max concurrent. 6 节点全指向同一 mock SSH server,
// server handler sleep 300ms. 期望: Parallel=2 → peak=2 + ~900ms 总耗时.
package cmd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mysshw/config"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	gliderssh "github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

var (
	mcpReproBH   *mcpBlackhole
	mcpReproCur  int32
	mcpReproPeak int32
)

type mcpBlackhole struct {
	delay time.Duration
}

func startMCPReproServer(t *testing.T, delay time.Duration) (host string, port int, cleanup func()) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}
	signer, err := gossh.ParsePrivateKey(pem.EncodeToMemory(pemBlock))
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host = "127.0.0.1"
	port, err = strconv.Atoi(strings.Split(listener.Addr().String(), ":")[1])
	require.NoError(t, err)

	mcpReproBH = &mcpBlackhole{delay: delay}
	srv := &gliderssh.Server{
		HostSigners: []gliderssh.Signer{signer},
		PasswordHandler: func(ctx gliderssh.Context, password string) bool {
			return ctx.User() == "testuser" && password == "testpwd"
		},
	}
	srv.Handle(func(s gliderssh.Session) {
		cur := atomic.AddInt32(&mcpReproCur, 1)
		for {
			peak := atomic.LoadInt32(&mcpReproPeak)
			if cur > peak {
				if atomic.CompareAndSwapInt32(&mcpReproPeak, peak, cur) {
					break
				}
			} else {
				break
			}
		}
		time.Sleep(mcpReproBH.delay)
		atomic.AddInt32(&mcpReproCur, -1)
		_, _ = s.Write([]byte("ok\n"))
		_ = s.Exit(0)
	})
	go func() { _ = srv.Serve(listener) }()
	time.Sleep(150 * time.Millisecond)
	cleanup = func() { _ = srv.Close() }
	return host, port, cleanup
}

func TestMCP_Parallel_RealSSH(t *testing.T) {
	t.Setenv("MYSSHW_SKIP_HOST_KEY_CHECK", "true")
	host, port, stop := startMCPReproServer(t, 300*time.Millisecond)
	defer stop()

	// 写一个临时 key 文件用于 SSH 认证
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600))

	nodeNames := []string{"n1", "n2", "n3", "n4", "n5", "n6"}
	var b strings.Builder
	b.WriteString("[[nodes]]\ngroups = \"T\"\n")
	for _, name := range nodeNames {
		fmt.Fprintf(&b, "\n[[nodes.ssh]]\nname = %q\nhost = %q\nport = %d\nuser = %q\npassword = %q\nkeypath = %q\n",
			name, host, port, "testuser", "testpwd", keyPath)
	}
	cfgPath := withTempConfig(t, b.String())
	_ = config.CFG

	for _, parallel := range []int{2, 5} {
		resetRootCmd(t)
		atomic.StoreInt32(&mcpReproCur, 0)
		atomic.StoreInt32(&mcpReproPeak, 0)
		t.Run(fmt.Sprintf("parallel=%d", parallel), func(t *testing.T) {
			resetRootCmd(t)
			req := mcp.CallToolRequest{}
			req.Params.Name = "ssh_exec"
			req.Params.Arguments = map[string]any{
				"cfg":      cfgPath,
				"node":     nodeNames,
				"command":  "echo hi",
				"parallel": parallel,
			}
			t0 := time.Now()
			result, err := sshExecHandler(context.Background(), req)
			elapsed := time.Since(t0)
			require.NoError(t, err)
			require.NotNil(t, result)
			peak := atomic.LoadInt32(&mcpReproPeak)
			t.Logf("parallel=%d: elapsed=%v peak=%d (expected %d)", parallel, elapsed, peak, parallel)
			if result.IsError {
				if len(result.Content) > 0 {
					if tc, ok := result.Content[0].(mcp.TextContent); ok {
						t.Logf("MCP error: %s", tc.Text[:min(200, len(tc.Text))])
					}
				}
			}
			if peak > int32(parallel) {
				t.Errorf("BUG: peak=%d > parallel=%d — parallel limit not honored", peak, parallel)
			}
		})
	}
}
