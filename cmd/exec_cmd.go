package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mysshw/config"
	"mysshw/ssh"

	"github.com/spf13/cobra"
)

var (
	execNodesFlag        []string
	execAlias            string
	execHost             string
	execUser             string
	execPort             int
	execTimeout          time.Duration
	execParallel         int
	execNoPTY            bool
	execStopOnError      bool
	execDryRun           bool
	execRotateKnownHosts bool
	execStream           bool // --follow/--stream: 边输出边推送 JSON 事件
)

// execCmd 非交互 SSH 命令执行（AI / 脚本友好）
//
// 与默认 `mysshw` 走交互式 TUI 选择菜单不同:
//   - 必须显式指定节点定位参数（无菜单）
//   - 默认 PTY 开启，支持 sudo / 进度条
//   - 一次性执行返回，不像 Login 持续附着 shell
//   - 输出格式: plain / JSON（与 list 共用 -o flag）
//   - 退出码: BSD sysexits; 单节点模式下命令自身的 exit code 透传
//
// 批量: --node 可重复, 同一条命令扇出到所有匹配节点
var execCmd = &cobra.Command{
	Use:   "exec [--node N [--node M] | --alias A | --host H [-u U] [-p P]] [--timeout 30s] [--parallel N] [--no-pty] [-f] -- <command> [args...]",
	Short: "Non-interactive: log into a node and run a command",
	Long: `Connect to one or more SSH nodes and run a command without TUI menu or PTY shell.

Designed for AI agents and scripts:
  - Requires explicit node selector (no menu)
  - PTY enabled by default (for sudo / progress bars)
  - Output format via -o plain|json
  - --follow (-f) for streaming long-running commands (tail -f, journalctl -f)
  - BSD sysexits-style exit codes

Selectors (priority: --node > --alias > --host [+ -u / -p]):
  --node <name>     Match by Name or Alias (exact, repeatable for batch)
  --alias <alias>   Exact match of SSHNode.Alias
  --host <host>     Match by Host (combine with -u/-p to disambiguate)

Examples:
  # Single node
  mysshw exec --node prod-1 -- uptime
  mysshw exec --host 10.0.0.5 -u app -- "systemctl status nginx"

  # Batch fan-out
  mysshw exec --node prod-1 --node prod-2 --node prod-3 -- "uptime"

  # Machine-readable output
  mysshw exec -o json --node prod-1 -- "df -h" | jq '.'

  # Stream output line-by-line as JSON events (for AI agents or tail -f)
  mysshw exec -f --node prod-1 -- "journalctl -u nginx -f"
  # Outputs: {"event":"start",...} {"event":"stdout","line":"..."} {"event":"exit",...}

  # Disable PTY (pure pipe)
  mysshw exec --no-pty --node prod-1 -- "tar czf - /var/log" > out.tgz

  # Dry-run: print the plan without connecting
  mysshw exec --dry-run --node prod-1 -- "rm -rf /"   # 不会实际连接!

Exit codes (BSD sysexits):
  0    成功
  1    批量场景下部分/全部失败
  64   命令行参数错误
  78   节点未找到 (EX_CONFIG)
  124  超时
  255  SSH 协议错误
  -    单节点模式下, exit code = 命令自身的退出码 (透传)`,
	Args: cobra.ArbitraryArgs,
	RunE: runExec,
}

func init() {
	f := execCmd.Flags()
	f.StringSliceVar(&execNodesFlag, "node", nil,
		"Node name or alias (exact). Repeat for batch: --node X --node Y --node Z")
	f.StringVar(&execAlias, "alias", "", "Node alias (exact)")
	f.StringVar(&execHost, "host", "", "Node host (combine with -u/-p)")
	f.StringVarP(&execUser, "user", "u", "", "User for --host selector")
	f.IntVarP(&execPort, "port", "p", 0, "Port for --host selector (0=any)")
	f.DurationVar(&execTimeout, "timeout", 30*time.Second,
		"Per-node command timeout (e.g. 5s, 1m); 0=no limit")
	f.IntVar(&execParallel, "parallel", 10,
		"Max concurrent nodes for batch (0=unbounded)")
	f.BoolVar(&execNoPTY, "no-pty", false,
		"Disable PTY (default: PTY enabled for sudo/progress)")
	f.BoolVar(&execStopOnError, "stop-on-error", false,
		"Abort batch on first node failure (default: wait all)")
	f.BoolVar(&execDryRun, "dry-run", false,
		"Print the resolved plan (nodes, auth, command) without actually connecting. "+
			"Useful for verifying AI-generated commands are pointing at the right nodes.")
	f.BoolVar(&execRotateKnownHosts, "rotate-known-hosts", false,
		"On key fingerprint mismatch, auto-rotate known_hosts (delete old + add new). "+
			"Equivalent to OpenSSH StrictHostKeyChecking=no for THIS call only. "+
			"\u26a0\ufe0f Cannot distinguish MITM from legitimate key rotation. "+
			"Use only when the server key has actually been rotated by an admin.")
	f.BoolVarP(&execStream, "follow", "f", false,
		"Stream output as JSON events (one line per event, no buffering). "+
			"For long-running commands (tail -f, journalctl -f) or AI agents that need "+
			"real-time feedback. Implies JSON output regardless of --output flag.")
	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	// Args 验证移到 RunE 内部, 才能用 ExitError(64) 而不是 cobra 默认 1
	if len(args) == 0 {
		return NewExitError(ExitUsage, "missing <command> after `--` (e.g. `mysshw exec --node X -- uptime`)")
	}

	if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
		config.CFG_PATH = cfgPath
	}
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("load config: %w", err))
	}

	selector := ssh.NodeSelector{
		Names: execNodesFlag,
		Alias: execAlias,
		Host:  execHost,
		User:  execUser,
		Port:  execPort,
	}

	nodes, err := ssh.ResolveNodes(config.CFG, selector)
	if err != nil && !errors.Is(err, ssh.ErrNodeNotFound) {
		// Names 部分缺失 = 软错误, 继续
		fmt.Fprintln(cmd.ErrOrStderr(), "mysshw::", err)
	}
	if errors.Is(err, ssh.ErrNodeNotFound) {
		return WrapExit(ExitConfig, err)
	}
	if len(nodes) == 0 {
		return WrapExit(ExitConfig,
			fmt.Errorf("no nodes matched (--node=%v --alias=%q --host=%q -u=%q -p=%s)",
				execNodesFlag, execAlias, execHost, execUser, portOrEmpty(execPort)))
	}

	// --rotate-known-hosts: 临时设到本调用涉及的所有节点上 (不跨调用泄露)
	if execRotateKnownHosts {
		for _, n := range nodes {
			n.RotateKnownHosts = true
		}
	}

	command := strings.Join(args, " ")
	if command == "" {
		return NewExitError(ExitUsage, "missing command after `--`")
	}

	usePTY := !execNoPTY

	// --dry-run: 打印计划, 不实际连接
	if execDryRun {
		return runExecDryRun(cmd, selector, nodes, command, usePTY)
	}

	// --follow/--stream: 边输出边推送 JSON 事件 (全节点走 BatchExecutor.RunStream)
	if execStream {
		return runExecStream(cmd, nodes, command, usePTY)
	}

	// 单节点: 直接透传命令 exit code
	if len(nodes) == 1 {
		return runExecSingle(cmd, nodes[0], command, usePTY)
	}
	// 批量: 任一失败 → 1
	return runExecBatch(cmd, nodes, command, usePTY)
}

// execPlan 是 --dry-run 输出的 JSON-safe 结构
type execPlan struct {
	DryRun   bool           `json:"dry_run"`
	Selector execPlanSelect `json:"selector"`
	Nodes    []execPlanNode `json:"nodes"`
	Command  string         `json:"command"`
	UsePTY   bool           `json:"use_pty"`
	Timeout  string         `json:"timeout"`
	Parallel int            `json:"parallel,omitempty"`
	Warning  string         `json:"warning,omitempty"`
}

type execPlanSelect struct {
	Names []string `json:"names,omitempty"`
	Alias string   `json:"alias,omitempty"`
	Host  string   `json:"host,omitempty"`
	User  string   `json:"user,omitempty"`
	Port  int      `json:"port,omitempty"`
}

type execPlanNode struct {
	Name string `json:"name"`
	User string `json:"user"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Auth string `json:"auth"` // key/password/env/bcrypt/empty/...
}

func runExecDryRun(cmd *cobra.Command, selector ssh.NodeSelector,
	nodes []*config.SSHNode, command string, usePTY bool) error {

	plan := execPlan{
		DryRun:   true,
		Selector: execPlanSelect{Names: selector.Names, Alias: selector.Alias, Host: selector.Host, User: selector.User, Port: selector.Port},
		Nodes:    make([]execPlanNode, 0, len(nodes)),
		Command:  command,
		UsePTY:   usePTY,
		Timeout:  execTimeout.String(),
	}
	if len(nodes) > 1 {
		plan.Parallel = execParallel
	}
	for _, n := range nodes {
		plan.Nodes = append(plan.Nodes, execPlanNode{
			Name: n.Name, User: n.SetUser(), Host: n.Host, Port: n.SetPort(),
			Auth: classifyAuth(n),
		})
	}
	if len(nodes) == 0 {
		plan.Warning = "no nodes would be targeted"
	}

	return WriteOut(cmd.OutOrStdout(), plan, GetOutputFormat(cmd))
}

func runExecSingle(cmd *cobra.Command, node *config.SSHNode, command string, usePTY bool) error {
	ctx, cancel := withCmdTimeout(cmd.Context(), execTimeout)
	defer cancel()

	exec, err := ssh.NewExecutor(node, ssh.ExecOptions{UsePTY: usePTY})
	if err != nil {
		return WrapExit(ExitAuthFailed, err)
	}
	defer exec.Close()

	result, runErr := exec.RunCommand(ctx, command)
	if runErr != nil {
		// 先输出已有 result (若 dial 成功但 Run 失败)
		_ = WriteOut(cmd.OutOrStdout(), result, GetOutputFormat(cmd))
		if isCtxErr(runErr) {
			return WrapExit(ExitTimeout, runErr)
		}
		return WrapExit(ExitSSHError, runErr)
	}

	if err := WriteOut(cmd.OutOrStdout(), result, GetOutputFormat(cmd)); err != nil {
		return WrapExit(ExitIOErr, err)
	}

	// 透传命令自身的 exit code
	switch {
	case result.ExitCode == 0:
		return nil
	case result.ExitCode > 0 && result.ExitCode < 256:
		// 透传: 返回 ExitError 让 cobra 顶层 Execute() 调 os.Exit(Code)
		// 不能直接 os.Exit() —— 通过 runSubcommandCaptured 重入时会杀掉整个 MCP server 进程
		return WrapExit(result.ExitCode, nil)
	default:
		return WrapExit(ExitSSHError,
			fmt.Errorf("command exited with code %d (likely connection closed)", result.ExitCode))
	}
}

func runExecBatch(cmd *cobra.Command, nodes []*config.SSHNode, command string, usePTY bool) error {
	be := &ssh.BatchExecutor{
		Nodes: nodes,
		Options: ssh.BatchOptions{
			UsePTY:   usePTY,
			Timeout:  execTimeout,
			Parallel: execParallel,
		},
	}
	results := be.RunFanOut(cmd.Context(), command)

	format := GetOutputFormat(cmd)
	if err := WriteOut(cmd.OutOrStdout(), results, format); err != nil {
		return WrapExit(ExitIOErr, err)
	}

	// 任一节点失败 → 1
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Error != "" || r.ExitCode != 0 {
			return NewExitError(ExitGeneric,
				"batch completed with failures (see -o json for details)")
		}
	}
	return nil
}

// runExecStream 流式执行 (--follow): 边输出边推送 JSON 事件
//
// 事件 schema (每个事件一行 JSON, 供 AI agent 实时处理):
//
//	start   {event,node,ts}
//	stdout  {event,node,line,ts}
//	stderr  {event,node,line,ts}
//	exit    {event,node,exit_code,duration,ts}
//	error   {event,node,line,ts}
//
// --follow 总是输出 JSON (不论 -o flag), 退出码与批 exec 一致
func runExecStream(cmd *cobra.Command, nodes []*config.SSHNode, command string, usePTY bool) error {
	out := cmd.OutOrStdout()
	enc := json.NewEncoder(out)

	be := &ssh.BatchExecutor{
		Nodes: nodes,
		Options: ssh.BatchOptions{
			UsePTY:   usePTY,
			Timeout:  execTimeout,
			Parallel: execParallel,
		},
	}

	handler := func(ev ssh.StreamEvent) {
		// 同步逐行写出, JSON 一行一个事件, AI agent 可逐行 parse
		if err := enc.Encode(ev); err != nil {
			// 输出错误不可恢复, 只能记日志到 stderr
			fmt.Fprintf(cmd.ErrOrStderr(), "mysshw:: stream encode: %v\n", err)
		}
	}

	results := be.RunStream(cmd.Context(), command, handler)

	// 总体退出码语义: 任一节点失败 → exit 1
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Error != "" || r.ExitCode != 0 {
			return NewExitError(ExitGeneric,
				"stream batch completed with failures")
		}
	}
	return nil
}

// --- 内部辅助 ---

// withCmdTimeout 给 cmd context 加 timeout (timeout<=0 时直接返回 parent)
func withCmdTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, d)
}

// isCtxErr 判断 error 链中是否含 context 错误 (timeout/cancel)
func isCtxErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// portOrEmpty 把 0 端口格式化为空字符串 (便于错误信息显示)
func portOrEmpty(p int) string {
	if p == 0 {
		return ""
	}
	return strconv.Itoa(p)
}
