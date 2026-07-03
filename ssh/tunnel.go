package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"mysshw/config"

	gossh "golang.org/x/crypto/ssh"
)

// Tunnel 本地端口转发接口
//
// 行为: 在本地 (127.0.0.1:LocalPort) 监听, 每个进来的连接通过 SSH 通道
//
//	转发到 SSH server 上的 RemoteHost:RemotePort.
//
// 与 ssh.Client.Listen 不同 (那是 remote→local 反向转发):
//   - 这里建立的是 local→remote 端口转发
//   - 由 SSH server 主动拨号 RemoteAddr (从 server 视角是 outbound)
type Tunnel interface {
	// Start 拨号 SSH 并开始接受本地连接; 阻塞直到 ctx 取消或错误发生
	Start(ctx context.Context) error
	// Addr 返回实际绑定的本地地址 (含 :0 自动分配的端口)
	Addr() string
	// Close 关闭所有活跃连接并停止 accept loop
	Close() error
}

// TunnelOptions 配置
//
//	LocalBind   本地监听地址, 如 "127.0.0.1:8080" 或 ":8080"
//	RemoteAddr  SSH server 上的目标, 如 "127.0.0.1:80" (相对 server)
//	IdleTimeout 单连接空闲超时 (0 = 不限制)
type TunnelOptions struct {
	LocalBind   string
	RemoteAddr  string
	IdleTimeout time.Duration
}

// localTunnel 是基于 ssh.Client.Dial 的本地端口转发实现
type localTunnel struct {
	node *config.SSHNode
	opts TunnelOptions

	listener net.Listener
	client   *gossh.Client

	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{} // 当前活跃连接 (本地 + 远端)
	done   chan struct{}         // accept loop 退出信号
}

// NewTunnel 创建一个本地端口转发器
//
//	启动前不拨号 (lazy); 第一次 Start 才建立 SSH 连接 + 本地监听器.
func NewTunnel(node *config.SSHNode, opts TunnelOptions) (Tunnel, error) {
	if node == nil {
		return nil, fmt.Errorf("mysshw:: nil node")
	}
	if opts.LocalBind == "" {
		return nil, fmt.Errorf("mysshw:: local bind address required (use --local)")
	}
	if opts.RemoteAddr == "" {
		return nil, fmt.Errorf("mysshw:: remote address required (use --remote)")
	}
	return &localTunnel{
		node:  node,
		opts:  opts,
		conns: make(map[net.Conn]struct{}),
		done:  make(chan struct{}),
	}, nil
}

func (t *localTunnel) Addr() string {
	if t.listener == nil {
		return t.opts.LocalBind
	}
	return t.listener.Addr().String()
}

// Start 阻塞运行直到 ctx 取消 / 监听失败 / SSH 掉线
//
// 启动顺序:
//  1. net.Listen 本地端口
//  2. 建立 SSH 连接
//  3. accept loop 处理本地连接并通过 SSH 拨号 RemoteAddr
func (t *localTunnel) Start(ctx context.Context) error {
	// 1) 本地监听器
	listener, err := net.Listen("tcp", t.opts.LocalBind)
	if err != nil {
		return fmt.Errorf("mysshw:: listen %s: %w", t.opts.LocalBind, err)
	}
	t.listener = listener

	// 2) SSH 客户端 (复用 defaultClient, 但不走 Login 而走 dial)
	c := NewClientNonInteractive(t.node)
	if c == nil {
		_ = listener.Close()
		return fmt.Errorf("mysshw:: failed to build SSH client for tunnel node %q", t.node.Name)
	}
	dc, ok := c.(*defaultClient)
	if !ok || dc == nil {
		_ = listener.Close()
		return fmt.Errorf("mysshw:: internal: defaultClient cast failed")
	}

	// 通过 connect() 拨号 (作为 Tunnel 我们不要 Login 那套 PTY / keepalive)
	dialed, err := dc.connect()
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("mysshw:: tunnel dial: %w", err)
	}
	t.client = dialed

	// 监听 ctx 取消
	go func() {
		<-ctx.Done()
		_ = t.Close()
	}()

	// 3) accept loop
	for {
		localConn, err := listener.Accept()
		if err != nil {
			if t.isClosed() {
				return nil
			}
			// accept 瞬态错误, 短暂等待后继续
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			// listener 已关闭 (Close 后再 Accept)
			return nil
		}

		go t.handleConn(ctx, localConn)
	}
}

// handleConn 处理单条本地连接: SSH 拨号 remote → 双向 pipe
func (t *localTunnel) handleConn(ctx context.Context, local net.Conn) {
	defer local.Close()

	if t.isClosed() {
		return
	}

	// 通过 SSH 客户端拨 RemoteAddr (相对 SSH server)
	remote, err := t.client.Dial("tcp", t.opts.RemoteAddr)
	if err != nil {
		// dial 失败, 给本地连接一个短 EOF 让客户端立刻知道
		_ = local.SetDeadline(time.Now().Add(1 * time.Second))
		return
	}
	defer remote.Close()

	t.trackConn(local)
	t.trackConn(remote)
	defer func() {
		t.untrackConn(local)
		t.untrackConn(remote)
	}()

	// 空闲超时
	if t.opts.IdleTimeout > 0 {
		deadline := time.Now().Add(t.opts.IdleTimeout)
		_ = local.SetDeadline(deadline)
		_ = remote.SetDeadline(deadline)
	}

	// 双向转发
	errCh := make(chan error, 2)
	go func() { _, e := io.Copy(remote, local); errCh <- e }()
	go func() { _, e := io.Copy(local, remote); errCh <- e }()

	select {
	case <-ctx.Done():
		_ = local.Close()
		_ = remote.Close()
		<-errCh
		<-errCh
	case <-errCh:
		// 任一方向结束, 关另一方向
		_ = local.Close()
		_ = remote.Close()
		<-errCh
	}
}

func (t *localTunnel) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	if t.listener != nil {
		_ = t.listener.Close()
	}
	if t.client != nil {
		_ = t.client.Close()
	}

	t.mu.Lock()
	for c := range t.conns {
		_ = c.Close()
	}
	t.conns = nil
	t.mu.Unlock()

	close(t.done)
	return nil
}

func (t *localTunnel) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

func (t *localTunnel) trackConn(c net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conns != nil {
		t.conns[c] = struct{}{}
	}
}

func (t *localTunnel) untrackConn(c net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.conns, c)
}
