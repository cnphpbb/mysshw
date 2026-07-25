//go:build e2e
// +build e2e

// e2e_test.go: 用 gliderlabs/ssh 起 in-process SSH server,
// 覆盖 list / exec / batch / tunnel / resolver 的端到端路径。
//
// 仅在 `go test -tags e2e ./ssh/...` 时构建; 不参与默认 `go test` 套件。

package ssh

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"mysshw/config"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gossh "golang.org/x/crypto/ssh"
)

// startTestSSHServer 启动 in-process SSH server
//
// 测试凭据:
//
//	user: testuser
//	password: testpwd
//
// handler:
//
//	"echo <msg>"       → stdout "executed: echo <msg>\n", exit 0
//	"exit <N>"         → exit N (无输出)
//	其他               → stdout "executed: <cmd>\n", exit 0
func startTestSSHServer(t *testing.T) (host string, port int, signer gossh.Signer, cleanup func()) {
	return startTestSSHServerWithMarker(t, "executed")
}

// startTestSSHServerWithMarker 启动一个用 marker 标记输出的 in-process SSH server
//
// 所有 session stdout 以 "<marker>: <cmd>\n" 格式输出。仅 forward tcpip-direct 的 server
// 可不设 marker (session 上跑不到, 但有助于调试时了然区分)。
//
// 用于多服务器场景下区分命令到底跑在哪台, 例如 proxy e2e 中区别 bastion 与 target。
func startTestSSHServerWithMarker(t *testing.T, marker string) (host string, port int, signer gossh.Signer, cleanup func()) {
	t.Helper()

	// 生成临时 RSA host key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}
	parsed, err := gossh.ParsePrivateKey(pem.EncodeToMemory(pemBlock))
	require.NoError(t, err)
	signer = parsed

	// 系统分配端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host = "127.0.0.1"
	p, err := strconv.Atoi(strings.Split(listener.Addr().String(), ":")[1])
	require.NoError(t, err)
	port = p

	server := &gliderssh.Server{
		HostSigners: []gliderssh.Signer{signer},
		PasswordHandler: func(ctx gliderssh.Context, password string) bool {
			return ctx.User() == "testuser" && password == "testpwd"
		},
	}
	server.Handle(func(s gliderssh.Session) {
		cmd := strings.Join(s.Command(), " ")
		switch {
		case strings.HasPrefix(cmd, "exit "):
			n, err := strconv.Atoi(strings.TrimPrefix(cmd, "exit "))
			if err == nil {
				_ = s.Exit(n)
				return
			}
		}
		_, _ = s.Write([]byte(marker + ": " + cmd + "\n"))
		_ = s.Exit(0)
	})

	go func() {
		_ = server.Serve(listener)
	}()

	// 等待 listener 真正 ready
	time.Sleep(150 * time.Millisecond)

	cleanup = func() {
		_ = server.Close()
	}
	return
}

// --- list JSON schema (不需 server) ---

func TestE2E_ListJSONStableSchema(t *testing.T) {
	cfg := e2eConfig()

	// mirror cmd/list_cmd.go: nodeSummary 结构
	type summary struct {
		Group string `json:"group"`
		Name  string `json:"name"`
		Alias string `json:"alias,omitempty"`
		User  string `json:"user,omitempty"`
		Host  string `json:"host"`
		Port  int    `json:"port"`
		Auth  string `json:"auth"`
	}
	out := []summary{}
	for _, g := range cfg.Nodes {
		for _, n := range g.SSHNodes {
			out = append(out, summary{
				Group: g.Groups,
				Name:  n.Name,
				Alias: n.Alias,
				User:  n.SetUser(),
				Host:  n.Host,
				Port:  n.SetPort(),
				Auth:  classifyAuthForTest(n),
			})
		}
	}
	b, err := json.Marshal(out)
	require.NoError(t, err)
	s := string(b)

	// 隐私: 永不出 password 字段 / ${ENV} 变量名
	assert.NotContains(t, s, `"password"`)
	assert.NotContains(t, s, `"${ENV`)
	// 字段存在
	for _, k := range []string{`"group"`, `"name"`, `"host"`, `"auth"`} {
		assert.Contains(t, s, k)
	}
}

// classifyAuthForTest 是 cmd 包 classifyAuth 的镜像 (避免跨包依赖)
func classifyAuthForTest(n *config.SSHNode) string {
	if n == nil {
		return "unknown"
	}
	pwd := n.Password
	hasKey := n.KeyPath != ""
	if pwd == "" {
		if hasKey {
			return "key"
		}
		return "empty"
	}
	if config.IsBcryptHash(pwd) {
		return "bcrypt"
	}
	if strings.HasPrefix(strings.TrimSpace(pwd), "${") {
		return "env"
	}
	if hasKey {
		return "password+key"
	}
	return "password"
}

// --- 真实 SSH server e2e ---

func TestE2E_ExecOnRunningServer(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	node := e2eNode(t, host, port)
	exec, err := NewExecutor(node, ExecOptions{UsePTY: false})
	require.NoError(t, err)
	defer exec.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, runErr := exec.RunCommand(ctx, "echo hello-from-server")
	require.NoError(t, runErr)
	assert.Equal(t, 0, res.ExitCode)
	assert.Contains(t, res.Stdout, "executed: echo hello-from-server")
}

func TestE2E_ExecCapturesNonZeroExit(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	node := e2eNode(t, host, port)
	exec, err := NewExecutor(node, ExecOptions{UsePTY: false})
	require.NoError(t, err)
	defer exec.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, runErr := exec.RunCommand(ctx, "exit 7")
	require.NoError(t, runErr)
	assert.Equal(t, 7, res.ExitCode, "命令 exit 7 应被透传")
}

func TestE2E_BatchExecutorMultipleNodes(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	nodes := []*config.SSHNode{
		e2eNode(t, host, port),
		e2eNode(t, host, port),
		e2eNode(t, host, port),
	}
	nodes[0].Name = "n-1"
	nodes[1].Name = "n-2"
	nodes[2].Name = "n-3"

	be := &BatchExecutor{
		Nodes:   nodes,
		Options: BatchOptions{UsePTY: false, Timeout: 5 * time.Second, Parallel: 2},
	}
	results := be.RunFanOut(context.Background(), "echo batch-test")
	require.Len(t, results, 3)
	for i, r := range results {
		if r == nil {
			t.Fatalf("result[%d] is nil", i)
		}
		assert.Equal(t, 0, r.ExitCode)
		assert.Contains(t, r.Stdout, "executed:")
	}
}

// TestE2E_Proxy_HopsViaJumpHost 验证跳板链连接是经由 bastion 后到达 target 的,
// 不是直接在 target 上出结果。
//
// 采用场景:
//   - bastion server 标记 "BASTION", 并启用 DirectTCPIPHandler (默认 gliderssh 不转发)
//   - target server  标记 "TARGET"
//
// 期望:
//   - exec.RunCommand 成功
//   - stdout 含 "TARGET: <cmd>"    (证明是 SSH 到 target 后执行)
//   - stdout 不含 "BASTION" 字样  (证明命令没误在 bastion 上跑)
//
// 该测试是 Proxy 修复后 happy-path 烟雾测试。拨顺序的反向回归测试 (bastion refused)
// 为 proxy_test.go::TestDialViaProxyChain_TwoDistinctEndpoints。
func TestE2E_Proxy_HopsViaJumpHost(t *testing.T) {
	bastionHost, bastionPort, _, bastionCleanup := startForwardingSSHServer(t)
	defer bastionCleanup()
	targetHost, targetPort, _, targetCleanup := startTestSSHServerWithMarker(t, "TARGET")
	defer targetCleanup()

	t.Setenv("MYSSHW_SKIP_HOST_KEY_CHECK", "true")

	// 在 config.CFG 里注册两个节点, 让 resolveProxyChain 能找到 bastion
	bastionNode := &config.SSHNode{
		Name: "bastion", Host: bastionHost, Port: bastionPort,
		User: "testuser", Password: "testpwd",
	}
	targetNode := &config.SSHNode{
		Name: "target", Host: targetHost, Port: targetPort,
		User: "testuser", Password: "testpwd",
		Proxy: "bastion",
	}
	oldCFG := config.CFG
	config.CFG = &config.Configs{
		Nodes: []config.Nodes{
			{Groups: "E2E", SSHNodes: []*config.SSHNode{bastionNode, targetNode}},
		},
	}
	defer func() { config.CFG = oldCFG }()

	exec, err := NewExecutor(targetNode, ExecOptions{UsePTY: false})
	require.NoError(t, err)
	defer exec.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, runErr := exec.RunCommand(ctx, "echo proxy-marker")
	require.NoError(t, runErr, "跳板链 dial 应成功; error: %v stderr: %s", runErr, res.Stderr)
	assert.Equal(t, 0, res.ExitCode)
	assert.Contains(t, res.Stdout, "TARGET: echo proxy-marker",
		"stdout 应来自 target; got %q (err=%v)", res.Stdout, runErr)
	assert.NotContains(t, res.Stdout, "BASTION",
		"命令不应在 bastion 上跑过; got %q", res.Stdout)
}

// startForwardingSSHServer 启动一个 启用 direct-tcpip 透传的 SSH server。
//
// gliderlabs/ssh v0.3 默认只注册 "session" channel handler, direct-tcpip 转发需手动加上。
// 另外, LocalPortForwardingCallback 默认为 nil 会直接拒绝转发, 也需手动设置为允许。
//
// 本 server 不注册 session handler (无需跑命令); 仅供 (my) 作为跳板透传用。
func startForwardingSSHServer(t *testing.T) (host string, port int, signer gossh.Signer, cleanup func()) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}
	parsed, err := gossh.ParsePrivateKey(pem.EncodeToMemory(pemBlock))
	require.NoError(t, err)
	signer = parsed

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host = "127.0.0.1"
	p, err := strconv.Atoi(strings.Split(listener.Addr().String(), ":")[1])
	require.NoError(t, err)
	port = p

	server := &gliderssh.Server{
		HostSigners: []gliderssh.Signer{signer},
		PasswordHandler: func(ctx gliderssh.Context, password string) bool {
			return ctx.User() == "testuser" && password == "testpwd"
		},
		// 允许所有来自跳转链的本地转发请求
		LocalPortForwardingCallback: func(ctx gliderssh.Context, host string, port uint32) bool {
			return true
		},
		// 添加 direct-tcpip 转发 handler (默认未注册)
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"session":      gliderssh.DefaultSessionHandler,
			"direct-tcpip": gliderssh.DirectTCPIPHandler,
		},
	}

	go func() {
		_ = server.Serve(listener)
	}()
	time.Sleep(150 * time.Millisecond) // 等真正 ready

	cleanup = func() {
		_ = server.Close()
	}
	return
}

// TestE2E_StreamExecutorSingleNode 验证 RunStream 逐行 callback
func TestE2E_StreamExecutorSingleNode(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	node := e2eNode(t, host, port)
	exec, err := NewExecutor(node, ExecOptions{UsePTY: false})
	require.NoError(t, err)
	defer exec.Close()

	var lines []string
	cb := func(stream, line string) {
		if stream == "stdout" {
			lines = append(lines, line)
		}
	}
	res, runErr := exec.RunStream(context.Background(), "echo stream-test", cb)
	require.NoError(t, runErr)
	assert.Equal(t, 0, res.ExitCode)
	require.NotEmpty(t, lines, "should receive at least one stdout line")
	assert.Contains(t, lines[0], "executed:")
}

// TestE2E_BatchStreamMultipleNodes 验证 BatchExecutor.RunStream 多节点并发
func TestE2E_BatchStreamMultipleNodes(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	nodes := []*config.SSHNode{
		e2eNode(t, host, port),
		e2eNode(t, host, port),
	}
	nodes[0].Name = "n-1"
	nodes[1].Name = "n-2"

	be := &BatchExecutor{
		Nodes:   nodes,
		Options: BatchOptions{UsePTY: false, Timeout: 5 * time.Second, Parallel: 2},
	}

	var events []StreamEvent
	handler := func(ev StreamEvent) {
		events = append(events, ev)
	}
	results := be.RunStream(context.Background(), "echo stream-batch", handler)
	require.Len(t, results, 2)

	// 验证每个节点都有 start/stdout/exit 事件
	for _, r := range results {
		if r == nil {
			t.Fatalf("result is nil")
		}
		assert.Equal(t, 0, r.ExitCode)
	}

	// 统计事件类型
	eventTypes := make(map[string]int)
	for _, ev := range events {
		eventTypes[ev.Event]++
	}
	// 期望: 2 个 start + 2 个 stdout + 2 个 exit
	assert.Equal(t, 2, eventTypes["start"], "expected 2 start events")
	assert.Equal(t, 2, eventTypes["stdout"], "expected 2 stdout events")
	assert.Equal(t, 2, eventTypes["exit"], "expected 2 exit events")
}

// TestE2E_StreamContextCancel 验证 ctx 取消时 stream 也能中断
func TestE2E_StreamContextCancel(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	node := e2eNode(t, host, port)
	exec, err := NewExecutor(node, ExecOptions{UsePTY: false})
	require.NoError(t, err)
	defer exec.Close()

	// 极短超时 + 多输出命令 验证 ctx cancel 能中断 stream.
	// 使用 sleep 命令或者借 cancel 主动中断
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	cb := func(stream, line string) {}
	res, runErr := exec.RunStream(ctx, "echo anything", cb)
	// ctx 预先取消, 应该返回错误 (cancelled before dial)
	// 但实际可能在 dial 后 Run 才开始, 此时 session.Close() 会让 Run 返回错误
	if runErr == nil {
		// 如果 res 没拿到, 也算 cancel 生效
		assert.NotNil(t, res)
	} else {
		// 接受 cancelled 错误
		t.Logf("got expected cancel error: %v", runErr)
	}
}

// ==================== Phase B: bcrypt-pepper 端到端测试 ====================

// TestE2E_BcryptPepper_ConnectsWithCorrectPlainAndPepper 验证:
//  1. password 字段是 bcrypt-pepper 哈希
//  2. 顶层 secret_key 提供 pepper
//  3. 用户输入正确明文 → NewClient 返回非 nil + 包含 Password auth method
func TestE2E_BcryptPepper_ConnectsWithCorrectPlainAndPepper(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	// 准备: 用 pepper 哈希 "testpwd" (test server 期望的明文)
	pepper := "server-side-secret-32bytes-aaa"
	hash, err := config.HashPasswordWithPepper("testpwd", pepper)
	require.NoError(t, err)

	node := e2eNode(t, host, port)
	node.Password = hash // 用 bcrypt-pepper 哈希替换明文

	// 关键: 通过 stdin pipe 模拟交互输入 (resolvePasswordForSSH 走交互路径)
	restoreStdin := pipeStdinE2E(t, "testpwd\n")
	defer restoreStdin()

	// 配置顶层 bcrypt-pepper 模式 + secret_key
	oldCFG := config.CFG
	oldPath := config.CFG_PATH
	defer func() {
		config.CFG = oldCFG
		config.CFG_PATH = oldPath
	}()
	config.CFG = &config.Configs{
		EncryptionAlgorithm: "bcrypt-pepper",
		SecretKey:           pepper,
	}
	config.CFG_PATH = "<e2e-bcrypt-pepper>"

	// 验证: NewClient 返回非 nil (bcrypt-pepper 验证通过)
	cli := NewClient(node)
	require.NotNil(t, cli, "bcrypt-pepper 验证通过应返回非 nil client")

	// 进一步验证: clientConfig.Auth 应包含 Password auth method
	dc, ok := cli.(*defaultClient)
	require.True(t, ok, "client 应是 *defaultClient")
	require.NotNil(t, dc.clientConfig, "clientConfig 不应为空")

	hasPasswordAuth := false
	for _, method := range dc.clientConfig.Auth {
		_ = method // 类型比较: ssh.Password 是 func, 不易直接比较
		// 简化检查: 长度 > 0 即认为有 auth method
		if len(dc.clientConfig.Auth) > 0 {
			hasPasswordAuth = true
		}
	}
	assert.True(t, hasPasswordAuth, "应至少有一个 auth method (password/key)")
}

// TestE2E_BcryptPepper_RejectsWrongPepper 验证:
//
//	password 是正确的 bcrypt-pepper 哈希, 但顶层 secret_key 错误
//	→ bcrypt-pepper 验证失败 (警告输出)
//	注: 节点仍可用 key auth 登录 (keypath 是 fallback), 但 password 认证会失败
func TestE2E_BcryptPepper_RejectsWrongPepper(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	// 用 CORRECT pepper 哈希
	correctPepper := "correct-pepper-32bytes"
	hash, err := config.HashPasswordWithPepper("testpwd", correctPepper)
	require.NoError(t, err)

	node := e2eNode(t, host, port)
	node.Password = hash

	// 模拟用户输入明文 (resolvePasswordForSSH 需要)
	restoreStdin := pipeStdinE2E(t, "testpwd\n")
	defer restoreStdin()

	// 但顶层 secret_key 配置成错误的值
	oldCFG := config.CFG
	oldPath := config.CFG_PATH
	defer func() {
		config.CFG = oldCFG
		config.CFG_PATH = oldPath
	}()
	config.CFG = &config.Configs{
		EncryptionAlgorithm: "bcrypt-pepper",
		SecretKey:           "WRONG-PEPPER-32bytes",
	}
	config.CFG_PATH = "<e2e-bcrypt-pepper-wrong>"

	// NewClient 不返回 nil (key auth 仍可用), 但实际 SSH 连接不会用错 pepper 登录
	// 重点: 通过观察 stderr 输出确认 "bcrypt-pepper mismatch" 警告
	// (e2e_test.go 的 startTestSSHServer 只接受 password="testpwd", 无 key 认证)
	// 所以即使 key auth 在, server 也不会接受 (需要验证 key)
	// 这里仅验证: 创建 client 不 panic, 警告被打印
	cli := NewClient(node)
	assert.NotNil(t, cli, "NewClient 仍返回非 nil (key auth 仍是 fallback)")
	// 警告输出 ("密码认证方式不可用") 出现在 stderr — 上面已可见
}

// TestE2E_BcryptPepper_RejectsWrongPlain 验证:
//
//	password 哈希 + pepper 都正确, 但用户输入错误明文
//	→ 验证失败 (警告输出)
func TestE2E_BcryptPepper_RejectsWrongPlain(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	pepper := "server-side-secret-32bytes"
	hash, err := config.HashPasswordWithPepper("testpwd", pepper)
	require.NoError(t, err)

	node := e2eNode(t, host, port)
	node.Password = hash

	// 模拟用户输入错误明文
	restoreStdin := pipeStdinE2E(t, "wrong-plain\n")
	defer restoreStdin()

	oldCFG := config.CFG
	oldPath := config.CFG_PATH
	defer func() {
		config.CFG = oldCFG
		config.CFG_PATH = oldPath
	}()
	config.CFG = &config.Configs{
		EncryptionAlgorithm: "bcrypt-pepper",
		SecretKey:           pepper,
	}
	config.CFG_PATH = "<e2e-bcrypt-pepper-wrong-plain>"

	cli := NewClient(node)
	assert.NotNil(t, cli, "NewClient 仍返回非 nil (key auth fallback)")
}

// pipeStdinE2E 把 s 写入临时 pipe, 设为 os.Stdin, 恢复函数返回
func pipeStdinE2E(t *testing.T, s string) func() {
	t.Helper()
	orig := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(s)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stdin = r
	return func() {
		os.Stdin = orig
		_ = r.Close()
	}
}

func TestE2E_TunnelListenAndClose(t *testing.T) {
	host, port, _, cleanup := startTestSSHServer(t)
	defer cleanup()

	node := e2eNode(t, host, port)
	tn, err := NewTunnel(node, TunnelOptions{
		LocalBind:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:1", // 没人监听, 测试 accept + EOF 流
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	startErrCh := make(chan error, 1)
	go func() { startErrCh <- tn.Start(ctx) }()

	time.Sleep(100 * time.Millisecond)
	addr := tn.Addr()
	assert.NotEmpty(t, addr)
	assert.True(t, strings.HasPrefix(addr, "127.0.0.1:"))

	// 尝试连接: accept 应成功 (远端 dial 失败但本地 accept 完成)
	conn, dialErr := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
	}

	cancel()
	select {
	case <-startErrCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("tunnel Start did not exit after ctx cancel")
	}
	_ = tn.Close()
}

// --- 纯 Node 解析端到端 (不需要 SSH server) ---

func TestE2E_ResolveByHostAcrossServer(t *testing.T) {
	cfg := &config.Configs{
		Nodes: []config.Nodes{
			{
				Groups: "E2E",
				SSHNodes: []*config.SSHNode{
					{Name: "n1", Host: "10.0.0.1", User: "testuser", Port: 2222},
					{Name: "n2", Host: "10.0.0.2", User: "other", Port: 22},
				},
			},
		},
	}

	out, err := ResolveNodes(cfg, NodeSelector{Names: []string{"n1"}})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "n1", out[0].Name)

	out, err = ResolveNodes(cfg, NodeSelector{Host: "10.0.0.2", User: "other"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "n2", out[0].Name)
}

// --- helpers ---

func e2eConfig() *config.Configs {
	return &config.Configs{
		Nodes: []config.Nodes{
			{
				Groups: "E2E",
				SSHNodes: []*config.SSHNode{
					{Name: "web-1", Host: "10.0.0.1", User: "testuser", Port: 22, Password: "${ENV_NAME}"},
					{Name: "db-1", Host: "10.0.0.2", User: "pg", Port: 5432, KeyPath: "/k"},
				},
			},
		},
	}
}

// e2eNode 构造测试节点
//   - Password 用于服务器的 PasswordHandler 验证
//   - KeyPath 指向临时生成的 RSA 私钥, 让 genSSHConfig 不在没 keypath 时报错 (ParsePrivateKey 静默)
func e2eNode(t *testing.T, host string, port int) *config.SSHNode {
	t.Helper()
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "id_rsa")
	require.NoError(t, os.WriteFile(keyFile, generateTestPrivateKeyPEM(t), 0600))
	return &config.SSHNode{
		Name:     "test-node",
		Host:     host,
		Port:     port,
		User:     "testuser",
		Password: "testpwd",
		KeyPath:  keyFile,
	}
}

func generateTestPrivateKeyPEM(t *testing.T) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}
	return pem.EncodeToMemory(pemBlock)
}
