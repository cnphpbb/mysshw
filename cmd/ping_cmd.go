package cmd

import (
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"mysshw/config"
	"mysshw/ssh"

	"github.com/spf13/cobra"
)

// ============================================================
// pingCmd TCP 端口连通性检测 (AI / 脚本友好)
//
// 设计目标:
//   - 不依赖 ICMP (普通用户无权限), 走 TCP dial 测 SSH 端口
//   - 单节点 / 批量节点 / 直接 host 三种入口, 与 exec/copy/tunnel 一致
//   - 输出 JSON, 字段包含 reachable / latency_ms / 错误信息
//   - 默认 timeout 5s, 可通过 --timeout 调整
//
// 注意: 与系统 ping(1) 不同, 这里:
//   - 不发 ICMP echo
//   - 仅测 TCP 握手 (SYN → SYN-ACK) 的可达性 + 延迟
//   - 不需要 root 权限, 适合 AI 在受限环境下预检
// ============================================================

var (
	pingNodesFlag []string
	pingAlias     string
	pingHost      string
	pingPort      int
	pingTimeout   time.Duration
	pingParallel  int
	pingCount     int
)

var pingCmd = &cobra.Command{
	Use:   "ping [--node N [--node M] | --alias A | --host H [-p P]] [--timeout 5s] [--count N] [--parallel N]",
	Short: "TCP-level connectivity check (no ICMP; AI / script friendly)",
	Long: `Test whether SSH port (or custom port) is reachable on one or more nodes.

Unlike system ping(1), this command:
  - Uses TCP dial instead of ICMP echo (works without root)
  - Measures round-trip time of the TCP handshake (SYN -> SYN-ACK)
  - Returns structured JSON suitable for AI agents / scripts

JSON schema (single / batch):
  [
    {
      "node": "prod-1",
      "host": "10.0.0.1",
      "port": 22,
      "reachable": true,
      "latency_ms": 12.4,
      "attempts": 1,
      "error": ""
    },
    {
      "node": "prod-2",
      "host": "10.0.0.2",
      "port": 22,
      "reachable": false,
      "latency_ms": 0,
      "attempts": 1,
      "error": "dial tcp 10.0.0.2:22: i/o timeout"
    }
  ]

Exit codes (BSD sysexits):
  0    All nodes reachable
  1    One or more nodes unreachable
  64   usage
  78   node not found / config error

Examples:
  # Single node by name
  mysshw ping --node prod-1

  # Batch by alias
  mysshw ping --alias WebAlias

  # Direct host:port (no config needed)
  mysshw ping --host 10.0.0.5 -p 22

  # Custom timeout + 3 attempts each
  mysshw ping --node prod-1 --timeout 3s --count 3

  # JSON for AI agent
  mysshw ping --node prod-1 -o json | jq '.[] | select(.reachable==false)'`,
	Args: cobra.NoArgs,
	RunE: runPing,
}

func init() {
	f := pingCmd.Flags()
	f.StringSliceVar(&pingNodesFlag, "node", nil,
		"Node name(s) or alias(es). Repeat for batch: --node X --node Y")
	f.StringVar(&pingAlias, "alias", "", "Node alias (exact)")
	f.StringVar(&pingHost, "host", "", "Node host (combine with -p; standalone, no config required)")
	f.IntVarP(&pingPort, "port", "p", 0,
		"Port for --host selector or override default 22")
	f.DurationVar(&pingTimeout, "timeout", 5*time.Second,
		"Per-attempt dial timeout (e.g. 5s, 1m)")
	f.IntVar(&pingCount, "count", 1,
		"Number of attempts per node; reports average latency_ms")
	f.IntVar(&pingParallel, "parallel", 10,
		"Max concurrent nodes for batch")

	pingCmd.MarkFlagsMutuallyExclusive("node", "alias", "host")
	rootCmd.AddCommand(pingCmd)
}

// pingResult 单节点的 ping 结果 (JSON-safe)
type pingResult struct {
	Node      string  `json:"node"`
	Host      string  `json:"host"`
	Port      int     `json:"port"`
	Reachable bool    `json:"reachable"`
	LatencyMs float64 `json:"latency_ms"`
	Attempts  int     `json:"attempts"`
	Error     string  `json:"error,omitempty"`
}

func runPing(cmd *cobra.Command, args []string) error {
	if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
		config.CFG_PATH = cfgPath
	}

	// 1) --host 直连模式: 不读 config, 仅 TCP dial
	if pingHost != "" {
		port := pingPort
		if port <= 0 {
			port = 22
		}
		r := pingOnce("<direct>", pingHost, port, pingTimeout, pingCount)
		return emitPingResults(cmd, []pingResult{r})
	}

	// 2) 配置模式: 必须加载 config
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("load config: %w", err))
	}

	selector := ssh.NodeSelector{
		Names: pingNodesFlag,
		Alias: pingAlias,
		Host:  pingHost,
		User:  "",
		Port:  pingPort,
	}
	nodes, err := ssh.ResolveNodes(config.CFG, selector)
	if err != nil {
		if isNodeNotFound(err) {
			return WrapExit(ExitConfig, err)
		}
		// soft error (部分 node 未找到), 继续
		fmt.Fprintln(cmd.ErrOrStderr(), "mysshw::", err)
	}
	if len(nodes) == 0 {
		return WrapExit(ExitConfig,
			fmt.Errorf("no nodes matched (--node=%v --alias=%q --host=%q)",
				pingNodesFlag, pingAlias, pingHost))
	}

	results := pingBatch(nodes, pingTimeout, pingCount, pingParallel)
	return emitPingResults(cmd, results)
}

// pingBatch 多节点并发探测 (用 WaitGroup 正确同步)
func pingBatch(nodes []*config.SSHNode, timeout time.Duration, count, parallel int) []pingResult {
	results := make([]pingResult, len(nodes))

	if parallel <= 0 {
		parallel = 10
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup

	for i, n := range nodes {
		if n == nil {
			results[i] = pingResult{Node: "<nil>", Error: "nil node"}
			continue
		}
		wg.Add(1)
		go func(idx int, nd *config.SSHNode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = pingOnce(nd.Name, nd.Host, nd.SetPort(), timeout, count)
		}(i, n)
	}
	wg.Wait()
	return results
}

// pingOnce 单节点 TCP 拨号探测 (count 次取平均)
func pingOnce(nodeName, host string, port int, timeout time.Duration, count int) pingResult {
	if count <= 0 {
		count = 1
	}

	r := pingResult{
		Node:     nodeName,
		Host:     host,
		Port:     port,
		Attempts: count,
	}

	var totalMs float64
	for i := 0; i < count; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
		latency := time.Since(start)

		if err != nil {
			r.Reachable = false
			r.Error = err.Error()
			r.LatencyMs = 0
			r.Attempts = i + 1
			return r
		}
		_ = conn.Close()
		totalMs += float64(latency.Microseconds()) / 1000.0
	}

	r.Reachable = true
	r.LatencyMs = totalMs / float64(count)
	return r
}

// emitPingResults 输出结果, 根据 --output flag 决定 plain/JSON
// 任一节点 unreachable → exit 1 (BSD sysexits)
func emitPingResults(cmd *cobra.Command, results []pingResult) error {
	// 按 Node 名字典序排序 (确定性, AI 解析友好)
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Node < results[j].Node
	})

	format := GetOutputFormat(cmd)
	if format == OutputPlain {
		if err := printPingTable(cmd.OutOrStdout(), results); err != nil {
			return err
		}
	} else if err := WriteOut(cmd.OutOrStdout(), results, format); err != nil {
		return err
	}

	// 任一失败 → exit 1
	for _, r := range results {
		if !r.Reachable {
			return WrapExit(ExitGeneric, fmt.Errorf("%d/%d nodes unreachable", countUnreachable(results), len(results)))
		}
	}
	return nil
}

func countUnreachable(results []pingResult) int {
	n := 0
	for _, r := range results {
		if !r.Reachable {
			n++
		}
	}
	return n
}

func printPingTable(w io.Writer, results []pingResult) error {
	if w == nil {
		w = io.Discard
	}
	if len(results) == 0 {
		fmt.Fprintln(w, "(no ping results)")
		return nil
	}

	headers := []string{"NODE", "HOST", "PORT", "STATUS", "LATENCY_MS"}
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		status := "OK"
		if !r.Reachable {
			status = "FAIL"
		}
		rows = append(rows, []string{
			r.Node, r.Host,
			fmt.Sprintf("%d", r.Port),
			status,
			fmt.Sprintf("%.2fms", r.LatencyMs),
		})
	}
	return PrintTable(w, headers, rows)
}
