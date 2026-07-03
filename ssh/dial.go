package ssh

import (
	"fmt"

	"mysshw/config"

	gossh "golang.org/x/crypto/ssh"
)

// Dial 建立到节点的非交互式 SSH 连接, 返回底层 *gossh.Client
//
// 用途: scp.Client / 自定义端口转发等需要直接拿 SSH 客户端的场景
//
// 与 NewExecutor 的区别:
//   - NewExecutor 返回包装的 Executor, 屏蔽 SSH 细节
//   - Dial 直接返回 *gossh.Client, 调用方自己管理 session/lifecycle
//
// 非交互: 复用 NewClientNonInteractive 的认证路径 (bcrypt/空密码直接 nil,
// 不触发 stdin 读取)
func Dial(node *config.SSHNode) (*gossh.Client, error) {
	if node == nil {
		return nil, fmt.Errorf("mysshw:: nil node")
	}
	c := NewClientNonInteractive(node)
	if c == nil {
		return nil, fmt.Errorf("mysshw:: failed to build SSH client for node %q (check auth)", node.Name)
	}
	dc, ok := c.(*defaultClient)
	if !ok || dc == nil {
		return nil, fmt.Errorf("mysshw:: internal: defaultClient cast failed for node %q", node.Name)
	}
	return dc.connect()
}
