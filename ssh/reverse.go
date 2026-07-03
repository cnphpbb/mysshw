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

// ReverseTunnel 反向端口转发 (-R 等价)
//
// 行为: SSH server 在 BindAddr 上 listen, 每条进来的 TCP 连接:
//  1. 通过 SSH server-side forward 还原为 net.Conn
//  2. client 端 net.Dial("tcp", TargetAddr) 建立到 TargetAddr 的真实连接
//  3. 双向 pipe SSH channel ↔ client→target 连接
//
// 典型用例: 把内网服务通过 bastion 暴露给外网 (target=internal-svc:80)
type ReverseTunnel interface {
	Start(ctx context.Context) error
	BoundAddr() string
	Close() error
}

// ReverseTunnelOptions
//
//	BindAddr   SSH server 上的监听地址 (如 "0.0.0.0:8080" 或 ":8080")
//	TargetAddr client-side 真实目标 (如 "127.0.0.1:80")
type ReverseTunnelOptions struct {
	BindAddr   string
	TargetAddr string
}

type reverseTunnel struct {
	node *config.SSHNode
	opts ReverseTunnelOptions

	client *gossh.Client

	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{}
}

// NewReverseTunnel 构造反向端口转发 (未拨号)
func NewReverseTunnel(node *config.SSHNode, opts ReverseTunnelOptions) (ReverseTunnel, error) {
	if node == nil {
		return nil, fmt.Errorf("mysshw:: nil node")
	}
	if opts.BindAddr == "" {
		return nil, fmt.Errorf("mysshw:: bind address required (use --bind)")
	}
	if opts.TargetAddr == "" {
		return nil, fmt.Errorf("mysshw:: target address required (use --remote)")
	}
	return &reverseTunnel{
		node:  node,
		opts:  opts,
		conns: make(map[net.Conn]struct{}),
	}, nil
}

func (t *reverseTunnel) BoundAddr() string { return t.opts.BindAddr }

func (t *reverseTunnel) Start(ctx context.Context) error {
	c := NewClientNonInteractive(t.node)
	if c == nil {
		return fmt.Errorf("mysshw:: failed to build SSH client for node %q", t.node.Name)
	}
	dc, ok := c.(*defaultClient)
	if !ok || dc == nil {
		return fmt.Errorf("mysshw:: internal: defaultClient cast failed")
	}
	cli, err := dc.connect()
	if err != nil {
		return fmt.Errorf("mysshw:: reverse tunnel dial: %w", err)
	}
	t.client = cli

	listener, err := cli.Listen("tcp", t.opts.BindAddr)
	if err != nil {
		_ = cli.Close()
		return fmt.Errorf("mysshw:: ssh server listen %s: %w", t.opts.BindAddr, err)
	}

	go func() {
		<-ctx.Done()
		_ = t.Close()
	}()

	for {
		sshConn, err := listener.Accept()
		if err != nil {
			if t.isClosed() {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return nil
		}
		go t.handleConn(sshConn)
	}
}

func (t *reverseTunnel) handleConn(sshConn net.Conn) {
	defer sshConn.Close()

	if t.isClosed() {
		return
	}

	target, err := net.DialTimeout("tcp", t.opts.TargetAddr, 5*time.Second)
	if err != nil {
		// dial 失败时, 给 sshConn 一个 deadline 让其立刻 EOF
		_ = sshConn.SetDeadline(time.Now().Add(1 * time.Second))
		return
	}
	defer target.Close()

	t.mu.Lock()
	t.conns[target] = struct{}{}
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.conns, target)
		t.mu.Unlock()
	}()

	errCh := make(chan error, 2)
	go func() { _, e := io.Copy(target, sshConn); errCh <- e }()
	go func() { _, e := io.Copy(sshConn, target); errCh <- e }()
	<-errCh
}

func (t *reverseTunnel) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	if t.client != nil {
		_ = t.client.Close()
	}

	t.mu.Lock()
	for c := range t.conns {
		_ = c.Close()
	}
	t.conns = nil
	t.mu.Unlock()
	return nil
}

func (t *reverseTunnel) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}
