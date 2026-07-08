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
	RunStream(ctx context.Context, cmd string, onLine StreamCallback) (*ExecResult, error)
	Close() error
}

// StreamCallback 流式输出回调函数
//
//	stream  = "stdout" | "stderr"
//	line    = 不含换行符的一行
//	命令退出后会被调用一次 (stream="exit", line="") 传达结束信号
type StreamCallback func(stream, line string)

// 内部使用, 避免在 RunStream 接口外暴露 io.Writer 类型
var _ = io.Discard

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

// RunStream 边输出边回调, 不 buffer
//
// 适用场景:
//   - 长命令 (e.g. tail -f, journalctl -f, journal logs, apt upgrade)
//   - 实时管道 (管道 | xargs | tee)
//
// 行为保证:
//   - 每一行输出都通过 onLine 回调送出 (newline 分隔)
//   - onLine 不会被并发调用 (单 goroutine 按顺序调用)
//   - ctx 取消时 session 强制关闭, 后续 output 不会再送出
//   - 命令结束后不发出 exit 事件 (由 BatchExecutor.RunStream 统一发出)
func (e *executorImpl) RunStream(ctx context.Context, cmd string, onLine StreamCallback) (*ExecResult, error) {
	start := time.Now()
	res := &ExecResult{
		Node:    safeNodeName(e.underlying.node),
		Command: cmd,
	}

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

	// streaming 模式下需要 PTY (sudo / 进度条 / 交互) 还是 pure pipe 取决于参数 usePTY
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
	session.Stdin = strings.NewReader("")

	// stdout/stderr 各接一个 line splitter
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	session.Stdout = stdoutW
	session.Stderr = stderrW

	// 启动两个 splitter goroutine 读 line-by-line
	var streamWg sync.WaitGroup
	streamWg.Add(2)

	go func() {
		defer streamWg.Done()
		emitLines(stdoutR, "stdout", onLine)
	}()
	go func() {
		defer streamWg.Done()
		emitLines(stderrR, "stderr", onLine)
	}()

	runDone := make(chan error, 1)
	go func() {
		runDone <- session.Run(cmd)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		session.Close()
		runErr = fmt.Errorf("mysshw:: exec cancelled: %w", ctx.Err())
	case runErr = <-runDone:
		// 让 stdout/stderr 写入端关闭, 触发 reader EOF
		_ = stdoutW.Close()
		_ = stderrW.Close()
		streamWg.Wait()
	}

	// 解析 exit code
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
			res.Duration = time.Since(start).String()
			return res, runErr
		}
	}

	res.Duration = time.Since(start).String()
	return res, nil
}

// emitLines 从 r 逐行读, 调 onLine(stream, line). 遇到 EOF/错误退出.
func emitLines(r io.Reader, stream string, onLine StreamCallback) {
	br := make([]byte, 4096)
	var pending []byte
	for {
		n, err := r.Read(br)
		if n > 0 {
			pending = append(pending, br[:n]...)
			// 按 \n 拆分
			for {
				idx := bytes.IndexByte(pending, '\n')
				if idx < 0 {
					break
				}
				line := string(pending[:idx])
				pending = pending[idx+1:]
				if onLine != nil {
					onLine(stream, line)
				}
			}
		}
		if err != nil {
			// 读完后将残余部分也送出 (未以 \n 结尾的尾部)
			if len(pending) > 0 && onLine != nil {
				onLine(stream, string(pending))
				pending = nil
			}
			return
		}
	}
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

// StreamEvent 流式事件 (output layer 调 onStream)
// 事件名: "start" | "stdout" | "stderr" | "exit" | "error"
type StreamEvent struct {
	Event     string `json:"event"`          // start/stdout/stderr/exit/error
	Node      string `json:"node"`           // 节点名
	Line      string `json:"line,omitempty"` // stdout/stderr 内容 (不含换行)
	ExitCode  int    `json:"exit_code"`      // 总是输出 (exit event 必需)
	Duration  string `json:"duration,omitempty"`
	Timestamp string `json:"ts"` // RFC3339
}

// StreamHandler 流式事件处理函数
// 同步调用, 需及时返回以免拼出口
type StreamHandler func(ev StreamEvent)

// RunStream 边输出边以事件流形式回调, 不 buffer stdout/stderr.
//
// 适用场景:
//   - 长命令 (tail -f, journalctl -f, apt upgrade progress)
//   - AI agent 实时处理 (进度条 / 增量输出 / 交互式 prompt)
//
// 语义:
//   - 每节点先发出 "start" 事件
//   - 之后每行 stdout/stderr 以 "stdout" / "stderr" 事件同步回调
//   - 节点退出后发出 "exit" 事件 (含 ExitCode + Duration)
//   - 节点错误 (dial 失败 / session 异常) 发出 "error" 事件
//   - 回调同步调用, 调用方应避免阻塞
//   - 总体返回所有节点的 *ExecResult (含汇总 Duration)
func (be *BatchExecutor) RunStream(ctx context.Context, cmd string, onEvent StreamHandler) []*ExecResult {
	n := len(be.Nodes)
	results := make([]*ExecResult, n)

	if be.Options.Parallel <= 0 {
		be.Options.Parallel = 10
	}
	sem := make(chan struct{}, be.Options.Parallel)
	var wg sync.WaitGroup

	for i, node := range be.Nodes {
		if node == nil {
			results[i] = &ExecResult{Node: "<nil>", Command: cmd, Error: "nil node", ExitCode: -1}
			if onEvent != nil {
				onEvent(StreamEvent{
					Event:     "error",
					Node:      "<nil>",
					Line:      "nil node",
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			}
			continue
		}
		wg.Add(1)
		go func(idx int, nd *config.SSHNode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			runCtx := ctx
			if be.Options.Timeout > 0 {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(ctx, be.Options.Timeout)
				defer cancel()
			}

			// start 事件
			if onEvent != nil {
				onEvent(StreamEvent{
					Event:     "start",
					Node:      nd.Name,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			}

			exec, err := NewExecutor(nd, ExecOptions{UsePTY: be.Options.UsePTY})
			if err != nil {
				if onEvent != nil {
					onEvent(StreamEvent{
						Event:     "error",
						Node:      nd.Name,
						Line:      fmt.Sprintf("new executor: %v", err),
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					})
				}
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

			// 包装 onEvent 为 StreamCallback
			cb := func(stream, line string) {
				if onEvent != nil {
					onEvent(StreamEvent{
						Event:     stream, // stdout/stderr
						Node:      nd.Name,
						Line:      line,
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					})
				}
			}

			r, runErr := exec.RunStream(runCtx, cmd, cb)
			results[idx] = r

			if runErr != nil {
				if onEvent != nil {
					onEvent(StreamEvent{
						Event:     "error",
						Node:      nd.Name,
						Line:      runErr.Error(),
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					})
				}
				return
			}

			// exit 事件
			if onEvent != nil {
				onEvent(StreamEvent{
					Event:     "exit",
					Node:      nd.Name,
					ExitCode:  r.ExitCode,
					Duration:  r.Duration,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			}
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
