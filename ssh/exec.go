package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"mysshw/config"

	"golang.org/x/crypto/ssh"
)

// Executor 非交互命令执行接口（面向 AI agent / 脚本）
//
// 与 Client.Login 的 PTY shell 模式相反:
//   - 不需要 TTY / stdin 交互
//   - 一次性执行，返回结构化结果
//   - 失败时同时返回 error 与 ExecResult.Error 字段；命令执行成功但 exit≠0 时仅 result.ExitCode!=0
type Executor interface {
	RunCommand(ctx context.Context, cmd string) (*ExecResult, error)
	Close() error
}

// ExecResult 一次命令执行的结构化记录（JSON-safe）
type ExecResult struct {
	Node     string `json:"node"`
	Command  string `json:"command"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
	// Error 调用层错误：dial 失败、session 异常、超时等
	// 命令本身退出非 0 时 ExitCode != 0 但 Error == ""
	Error string `json:"error,omitempty"`
}

// ExecOptions 单节点执行选项
type ExecOptions struct {
	UsePTY bool // true: 申请 PTY（sudo / 进度条场景）
}

const (
	execPTYType  = "xterm"
	execPTYRows  = 40
	execPTYCols  = 200
	defaultSpeed = 14400
)

// executorImpl 基于 defaultClient 的 Executor 实现
//
// 复用其 clientConfig: 密钥 / 密码 / ${ENV} / bcrypt / 空密码交互 等认证逻辑
// 同一 Executor 实例多次 RunCommand 复用底层 SSH 连接 (无并发保证; 调用方需自行串行)
type executorImpl struct {
	underlying *defaultClient
	usePTY     bool

	mu     sync.Mutex
	dialed *ssh.Client
}

// NewExecutor 为节点构造 Executor
func NewExecutor(node *config.SSHNode, opts ExecOptions) (Executor, error) {
	if node == nil {
		return nil, fmt.Errorf("mysshw:: nil node")
	}
	// 使用 NewClientNonInteractive: AI/脚本场景 stdin 不可用, 不允许 bcrypt/空密码交互
	c := NewClientNonInteractive(node)
	if c == nil {
		return nil, fmt.Errorf("mysshw:: failed to build SSH client for node %q (check auth: password / keypath / bcrypt / ${ENV})", node.Name)
	}
	dc, ok := c.(*defaultClient)
	if !ok || dc == nil {
		return nil, fmt.Errorf("mysshw:: internal: defaultClient cast failed for node %q", node.Name)
	}
	return &executorImpl{underlying: dc, usePTY: opts.UsePTY}, nil
}

func (e *executorImpl) ensureDialed() (*ssh.Client, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dialed != nil {
		return e.dialed, nil
	}
	cli, err := e.underlying.connect()
	if err != nil {
		return nil, err
	}
	e.dialed = cli
	return cli, nil
}

// RunCommand 执行 cmd 并采集结果。ctx 用于超时/取消控制。
//
// 实现要点:
//   - session.Stdout/Stderr 接到 buffer, 命令结束后一次性读出
//   - PTY 模式提供空 stdin, 避免 sudo 等命令 TTY 提示阻塞
//   - 超时通过 ctx.Done() 触发 session.Close() 强行中断
//   - session.Run() 返回 *ssh.ExitError 时解出真实退出码
func (e *executorImpl) RunCommand(ctx context.Context, cmd string) (*ExecResult, error) {
	start := time.Now()
	res := &ExecResult{
		Node:    safeNodeName(e.underlying.node),
		Command: cmd,
	}

	// dial 包在 goroutine, 让 ctx 超时可中断慢 TCP (TEST-NET 等场景)
	type dialResult struct {
		client *ssh.Client
		err    error
	}
	dialDone := make(chan dialResult, 1)
	go func() {
		c, err := e.ensureDialed()
		dialDone <- dialResult{client: c, err: err}
	}()

	var client *ssh.Client
	select {
	case <-ctx.Done():
		res.Error = fmt.Sprintf("cancelled before dial: %v", ctx.Err())
		res.Duration = time.Since(start).String()
		// 后台 goroutine 仍会完成 dial; 关闭获得的连接 (若有)
		go func() {
			dr := <-dialDone
			if dr.client != nil {
				_ = dr.client.Close()
			}
		}()
		return res, fmt.Errorf("mysshw:: exec cancelled: %w", ctx.Err())
	case dr := <-dialDone:
		if dr.err != nil {
			res.Error = fmt.Sprintf("dial failed: %v", dr.err)
			res.Duration = time.Since(start).String()
			return res, fmt.Errorf("mysshw:: dial failed: %w", dr.err)
		}
		client = dr.client
	}

	session, err := client.NewSession()
	if err != nil {
		res.Error = fmt.Sprintf("new session: %v", err)
		res.Duration = time.Since(start).String()
		return res, fmt.Errorf("mysshw:: new session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf
	// 空 stdin: PTY 模式下 sudo 等命令若等待密码会立即 EOF -> 失败退出 (而非阻塞)
	session.Stdin = strings.NewReader("")

	if e.usePTY {
		modes := ssh.TerminalModes{
			ssh.ECHO:          0,
			ssh.TTY_OP_ISPEED: defaultSpeed,
			ssh.TTY_OP_OSPEED: defaultSpeed,
		}
		if err := session.RequestPty(execPTYType, execPTYRows, execPTYCols, modes); err != nil {
			res.Error = fmt.Sprintf("request pty: %v", err)
			res.Duration = time.Since(start).String()
			return res, fmt.Errorf("mysshw:: request pty: %w", err)
		}
	}

	runDone := make(chan error, 1)
	go func() {
		// session.Run(cmd) 在命令结束后返回:
		//   nil → 退出码 0
		//   *ssh.ExitError → 内含 ExitStatus()
		//   io.EOF → PTY 偶发, 视为 0
		runDone <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		// 强行关闭 session 会让内部 Run() 返回, 但 goroutine 仍会发送
		// 已 buffered channel 直接丢弃即可
		session.Close()
		res.Error = fmt.Sprintf("cancelled: %v", ctx.Err())
		res.Duration = time.Since(start).String()
		return res, fmt.Errorf("mysshw:: exec cancelled: %w", ctx.Err())

	case runErr := <-runDone:
		switch v := runErr.(type) {
		case nil:
			res.ExitCode = 0
		case *ssh.ExitError:
			res.ExitCode = v.ExitStatus()
		case *ssh.ExitMissingError:
			res.ExitCode = -1
		default:
			if runErr == io.EOF {
				res.ExitCode = 0
			} else {
				res.Error = runErr.Error()
				res.Stdout = stdoutBuf.String()
				res.Stderr = stderrBuf.String()
				res.Duration = time.Since(start).String()
				return res, fmt.Errorf("mysshw:: session run: %w", runErr)
			}
		}
	}

	res.Stdout = stdoutBuf.String()
	res.Stderr = stderrBuf.String()
	res.Duration = time.Since(start).String()
	return res, nil
}

func (e *executorImpl) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dialed != nil {
		err := e.dialed.Close()
		e.dialed = nil
		return err
	}
	return nil
}

// BatchOptions 多节点扇出配置
type BatchOptions struct {
	UsePTY   bool          // 单节点 PTY 开关
	Timeout  time.Duration // 单节点超时
	Parallel int           // 最大并发，0 或负 = 不限制
}

// BatchExecutor 在多个节点上并发执行同一条命令
//
// 结果按 Nodes 顺序保留 (确定性顺序, AI 可按 index 解析)
// 出错的节点也保留占位 (ExitCode=-1 或 Error 非空)
type BatchExecutor struct {
	Nodes   []*config.SSHNode
	Options BatchOptions
}

// RunFanOut 启动并发执行, 阻塞到所有节点完成 (或 ctx 整体取消)
func (be *BatchExecutor) RunFanOut(ctx context.Context, cmd string) []*ExecResult {
	n := len(be.Nodes)
	results := make([]*ExecResult, n)

	sem := newSemaphore(be.Options.Parallel)
	var wg sync.WaitGroup

	for i, node := range be.Nodes {
		if node == nil {
			results[i] = &ExecResult{Node: "<nil>", Command: cmd, Error: "nil node", ExitCode: -1}
			continue
		}
		wg.Add(1)
		sem.acquire()
		go func(idx int, nd *config.SSHNode) {
			defer wg.Done()
			defer sem.release()

			runCtx := ctx
			if be.Options.Timeout > 0 {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(ctx, be.Options.Timeout)
				defer cancel()
			}

			exec, err := NewExecutor(nd, ExecOptions{UsePTY: be.Options.UsePTY})
			if err != nil {
				results[idx] = &ExecResult{
					Node:     nd.Name,
					Command:  cmd,
					ExitCode: -1,
					Error:    fmt.Sprintf("new executor: %v", err),
					Duration: "0s",
				}
				return
			}
			defer exec.Close()

			r, _ := exec.RunCommand(runCtx, cmd)
			results[idx] = r
		}(i, node)
	}
	wg.Wait()
	return results
}

// --- 内部辅助 ---

// semaphore 简单 channel-based 信号量 (零值 = 无限制)
//
// 语义:
//   - acquire:  拿一个槽位; 满时阻塞
//   - release:  放一个槽位; 若 over-release (超過 acquire 次数) 则静默忽略
//   - 零值 (ch == nil): acquire/release 都是 no-op (不限制)
type semaphore struct {
	ch chan struct{}
}

func newSemaphore(n int) semaphore {
	if n <= 0 {
		return semaphore{}
	}
	return semaphore{ch: make(chan struct{}, n)}
}

func (s *semaphore) acquire() {
	if s.ch == nil {
		return
	}
	s.ch <- struct{}{}
}

func (s *semaphore) release() {
	if s.ch == nil {
		return
	}
	select {
	case <-s.ch:
	default:
		// 已无槽位可释放 -- over-release, 静默忽略
	}
}

func safeNodeName(n *config.SSHNode) string {
	if n == nil {
		return "<nil>"
	}
	return n.Name
}
