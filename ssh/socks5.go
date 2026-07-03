package ssh

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"

	"mysshw/config"

	gossh "golang.org/x/crypto/ssh"
)

// DynamicTunnel SOCKS5 / -D 等价: 在本地端口起 SOCKS5 代理,
// 每个连接透过 SSH server 转发到请求的目标 host:port.
//
// 典型用途: 通过 bastion 当 SOCKS5 网关访问内网资源
//
//	curl --socks5 127.0.0.1:1080 http://internal.host/
type DynamicTunnel interface {
	Start(ctx context.Context) error
	Addr() string
	Close() error
}

// DynamicTunnelOptions
//
//	ListenAddr client-side 监听地址 (如 "127.0.0.1:1080")
type DynamicTunnelOptions struct {
	ListenAddr string
}

type dynamicTunnel struct {
	node *config.SSHNode
	opts DynamicTunnelOptions

	listener net.Listener
	client   *gossh.Client

	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{}
}

// NewDynamicTunnel 构造 SOCKS5 动态转发器 (未拨号)
func NewDynamicTunnel(node *config.SSHNode, opts DynamicTunnelOptions) (DynamicTunnel, error) {
	if node == nil {
		return nil, fmt.Errorf("mysshw:: nil node")
	}
	if opts.ListenAddr == "" {
		return nil, fmt.Errorf("mysshw:: listen address required (use --local)")
	}
	return &dynamicTunnel{
		node:  node,
		opts:  opts,
		conns: make(map[net.Conn]struct{}),
	}, nil
}

func (t *dynamicTunnel) Addr() string {
	if t.listener == nil {
		return t.opts.ListenAddr
	}
	return t.listener.Addr().String()
}

func (t *dynamicTunnel) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", t.opts.ListenAddr)
	if err != nil {
		return fmt.Errorf("mysshw:: listen %s: %w", t.opts.ListenAddr, err)
	}
	t.listener = listener

	c := NewClientNonInteractive(t.node)
	if c == nil {
		_ = listener.Close()
		return fmt.Errorf("mysshw:: failed to build SSH client for node %q", t.node.Name)
	}
	dc, ok := c.(*defaultClient)
	if !ok || dc == nil {
		_ = listener.Close()
		return fmt.Errorf("mysshw:: internal: defaultClient cast failed")
	}
	cli, err := dc.connect()
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("mysshw:: dynamic tunnel dial: %w", err)
	}
	t.client = cli

	go func() {
		<-ctx.Done()
		_ = t.Close()
	}()

	for {
		localConn, err := listener.Accept()
		if err != nil {
			if t.isClosed() {
				return nil
			}
			return nil
		}
		go t.handleConn(localConn)
	}
}

// handleConn: 解析 SOCKS5 客户端, 通过 SSH 转发, pipe 数据
func (t *dynamicTunnel) handleConn(localConn net.Conn) {
	defer localConn.Close()

	if t.isClosed() {
		return
	}

	t.trackConn(localConn)
	defer t.untrackConn(localConn)

	// 1) 握手
	if err := socks5Handshake(localConn); err != nil {
		// 写错误响应
		_ = socks5Reply(localConn, 0x01) // general failure
		return
	}

	// 2) CONNECT 命令
	target, err := socks5Connect(localConn, t.client)
	if err != nil {
		_ = socks5Reply(localConn, 0x01) // general failure
		return
	}
	defer target.Close()

	// 3) 200 OK
	if err := socks5Reply(localConn, 0x00); err != nil {
		return
	}

	// 4) 双向 pipe
	errCh := make(chan error, 2)
	go func() { _, e := io.Copy(target, localConn); errCh <- e }()
	go func() { _, e := io.Copy(localConn, target); errCh <- e }()
	<-errCh
}

// socks5Handshake 处理 client-method-selection 阶段
// 简单实现: 只支持 NO AUTH (0x00), 其他方法一律拒绝
func socks5Handshake(conn net.Conn) error {
	// [ver=1 byte][nmethods=1 byte][methods...]
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	if header[0] != 0x05 {
		return fmt.Errorf("not SOCKS5 (ver=%d)", header[0])
	}
	nmethods := int(header[1])
	if nmethods == 0 {
		return errors.New("no methods offered")
	}
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	// 优先 NO AUTH
	for _, m := range methods {
		if m == 0x00 {
			_, err := conn.Write([]byte{0x05, 0x00}) // ver=5, method=NO AUTH
			return err
		}
	}

	// 不支持任何 method, 返回 0xFF
	_, _ = conn.Write([]byte{0x05, 0xFF})
	return errors.New("no acceptable method")
}

// socks5Connect 处理 CONNECT 请求: 拿目标 host:port, 走 SSH 拨号, 返回 net.Conn
func socks5Connect(conn net.Conn, sshCli *gossh.Client) (net.Conn, error) {
	// [ver=1][cmd=1][rsv=1][atyp=1][addr][port]
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("read cmd header: %w", err)
	}
	if header[0] != 0x05 {
		return nil, fmt.Errorf("not SOCKS5 (ver=%d)", header[0])
	}
	if header[1] != 0x01 { // 仅支持 CONNECT
		return nil, fmt.Errorf("only CONNECT supported (cmd=%d)", header[1])
	}

	var host string
	switch header[3] {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return nil, err
		}
		host = net.IP(ip).String()
	case 0x03: // DOMAINNAME
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return nil, err
		}
		buf := make([]byte, l[0])
		if _, err := io.ReadFull(conn, buf); err != nil {
			return nil, err
		}
		host = string(buf)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return nil, err
		}
		host = net.IP(ip).String()
	default:
		return nil, fmt.Errorf("unsupported atyp=%d", header[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return nil, err
	}
	port := binary.BigEndian.Uint16(portBuf)

	dst := net.JoinHostPort(host, strconv.Itoa(int(port)))
	return sshCli.Dial("tcp", dst)
}

// socks5Reply 写 SOCKS5 响应 (server → client)
func socks5Reply(conn net.Conn, rep byte) error {
	// [ver=5][rep][rsv=0][atyp=1 IPv4][addr=0.0.0.0][port=0]
	reply := []byte{0x05, rep, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	_, err := conn.Write(reply)
	return err
}

func (t *dynamicTunnel) Close() error {
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
	return nil
}

func (t *dynamicTunnel) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

func (t *dynamicTunnel) trackConn(c net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conns != nil {
		t.conns[c] = struct{}{}
	}
}

func (t *dynamicTunnel) untrackConn(c net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.conns, c)
}
