package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"mysshw/config"
	"mysshw/ssh"

	"github.com/spf13/cobra"
)

// ============================================================
// infoCmd 收集远程节点的系统信息 (AI / 运维预检)
//
// 设计目标:
//   - 通过 SSH 在节点上执行一组轻量命令, 收集 hostname/OS/kernel/CPU/memory/disk/uptime
//   - 支持单节点 / 批量, 复用 ssh.BatchExecutor 做并发
//   - 输出 JSON, 跨平台: 优先 Linux (/proc), 失败时回退到通用命令
//   - 不解析命令输出文本里的所有字段, 仅提取 AI 决策关心的关键信息
//
// 收集策略:
//   - hostname:     `hostname` (Linux/macOS/BSD)
//   - os:          `uname -s`
//   - kernel:      `uname -r`
//   - arch:        `uname -m`
//   - cpu_cores:   `nproc` (Linux/macOS)
//   - cpu_model:   `grep -m1 'model name' /proc/cpuinfo` (Linux) / `sysctl -n machdep.cpu.brand_string` (macOS)
//   - mem_total:   `grep MemTotal /proc/meminfo` (Linux) / `sysctl -n hw.memsize` (macOS)
//   - disk /:      `df -B1 /`  (Linux/macOS, 输出易解析)
//   - uptime_sec:  `cat /proc/uptime` (Linux) / `sysctl -n kern.boottime` (macOS, 计算差值)
// ============================================================

var (
	infoNodesFlag []string
	infoAlias     string
	infoHost      string
	infoUser      string
	infoPort      int
	infoTimeout   time.Duration
	infoParallel  int
	infoNoPTY     bool
)

var infoCmd = &cobra.Command{
	Use:   "info [--node N [--node M] | --alias A | --host H [-u U] [-p P]] [--timeout 30s] [--parallel N]",
	Short: "Collect system info from SSH nodes (hostname, OS, CPU, memory, disk, uptime)",
	Long: `Collect structured system information from one or more SSH nodes.

Connects to each selected node via SSH and runs a lightweight set of commands
(uname, nproc, /proc/meminfo, df, uptime) to build a JSON snapshot.

JSON schema (per node):
  {
    "node": "prod-1",
    "hostname": "prod-1.internal",
    "os": "linux",
    "kernel": "6.8.0-31-generic",
    "arch": "x86_64",
    "cpu": {"cores": 4, "model": "Intel(R) Xeon(R) ..."},
    "memory": {"total_bytes": 17179869184},
    "disk": [{"mount": "/", "size_bytes": ..., "used_bytes": ..., "percent": 42}],
    "uptime_seconds": 2592000,
    "uptime_human": "30 days",
    "collected_at": "2026-07-06T...",
    "error": ""
  }

Cross-platform:
  - Linux: uses /proc filesystem
  - macOS: uses sysctl
  - Other Unix (BSD): best-effort, may have missing fields

Exit codes (BSD sysexits):
  0    All nodes info collected
  1    Some nodes failed
  64   usage
  78   node not found / config error
  124  timeout
  255  SSH error

Examples:
  # Single node by name
  mysshw info --node prod-1

  # Batch (parallel)
  mysshw info --node web-1 --node web-2 --node web-3

  # Filter by group
  mysshw list --group production -o json | jq -r '.[].name' | \
    xargs mysshw info -o json

  # AI agent decision-making:
  #   mysshw info --node target -o json | jq '.[0].memory.total_bytes > 8589934592'`,
	Args: cobra.NoArgs,
	RunE: runInfo,
}

func init() {
	f := infoCmd.Flags()
	f.StringSliceVar(&infoNodesFlag, "node", nil,
		"Node name(s) or alias(es). Repeat for batch: --node X --node Y")
	f.StringVar(&infoAlias, "alias", "", "Node alias (exact)")
	f.StringVar(&infoHost, "host", "", "Node host (combine with -u/-p)")
	f.StringVarP(&infoUser, "user", "u", "", "User for --host selector")
	f.IntVarP(&infoPort, "port", "p", 0, "Port for --host selector (0=any)")
	f.DurationVar(&infoTimeout, "timeout", 30*time.Second,
		"Per-node info collection timeout")
	f.IntVar(&infoParallel, "parallel", 10,
		"Max concurrent nodes for batch")
	f.BoolVar(&infoNoPTY, "no-pty", false,
		"Disable PTY (PTY enabled by default for command compatibility)")

	infoCmd.MarkFlagsMutuallyExclusive("node", "alias", "host")
	rootCmd.AddCommand(infoCmd)
}

// NodeInfo 节点系统信息 (JSON-safe)
type NodeInfo struct {
	Node          string     `json:"node"`
	Hostname      string     `json:"hostname,omitempty"`
	OS            string     `json:"os,omitempty"`
	Kernel        string     `json:"kernel,omitempty"`
	Arch          string     `json:"arch,omitempty"`
	CPU           CPUInfo    `json:"cpu"`
	Memory        MemoryInfo `json:"memory"`
	Disk          []DiskInfo `json:"disk,omitempty"`
	UptimeSeconds float64    `json:"uptime_seconds,omitempty"`
	UptimeHuman   string     `json:"uptime_human,omitempty"`
	CollectedAt   string     `json:"collected_at"`
	Error         string     `json:"error,omitempty"`
}

// CPUInfo CPU 信息
type CPUInfo struct {
	Cores int    `json:"cores"`
	Model string `json:"model,omitempty"`
}

// MemoryInfo 内存信息 (bytes)
type MemoryInfo struct {
	TotalBytes int64 `json:"total_bytes"`
}

// DiskInfo 磁盘信息 (单 mount point)
type DiskInfo struct {
	Mount     string `json:"mount"`
	SizeBytes int64  `json:"size_bytes"`
	UsedBytes int64  `json:"used_bytes"`
	Percent   int    `json:"percent,omitempty"`
}

// cmdResult 单条远程命令的输出 (内部用)
type cmdResult struct {
	name string
	out  string
	err  error
}

func runInfo(cmd *cobra.Command, args []string) error {
	if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
		config.CFG_PATH = cfgPath
	}
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("load config: %w", err))
	}

	selector := ssh.NodeSelector{
		Names: infoNodesFlag,
		Alias: infoAlias,
		Host:  infoHost,
		User:  infoUser,
		Port:  infoPort,
	}
	nodes, err := ssh.ResolveNodes(config.CFG, selector)
	if err != nil {
		if isNodeNotFound(err) {
			return WrapExit(ExitConfig, err)
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "mysshw::", err)
	}
	if len(nodes) == 0 {
		return WrapExit(ExitConfig,
			fmt.Errorf("no nodes matched (--node=%v --alias=%q --host=%q)",
				infoNodesFlag, infoAlias, infoHost))
	}

	results := collectInfoBatch(nodes, infoTimeout, infoParallel, !infoNoPTY)
	return emitInfoResults(cmd, results)
}

// collectInfoBatch 并发收集多个节点信息
func collectInfoBatch(nodes []*config.SSHNode, timeout time.Duration, parallel int, usePTY bool) []NodeInfo {
	results := make([]NodeInfo, len(nodes))
	if parallel <= 0 {
		parallel = 10
	}

	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup

	for i, n := range nodes {
		if n == nil {
			results[i] = NodeInfo{Node: "<nil>", Error: "nil node"}
			continue
		}
		wg.Add(1)
		go func(idx int, nd *config.SSHNode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = collectInfoOne(nd, timeout, usePTY)
		}(i, n)
	}
	wg.Wait()
	return results
}

// collectInfoOne 单节点信息收集
//
// 在节点上跑一组并行命令, 每条命令单独超时. 总耗时受 --timeout 控制.
func collectInfoOne(node *config.SSHNode, timeout time.Duration, usePTY bool) NodeInfo {
	info := NodeInfo{
		Node:        node.Name,
		CollectedAt: time.Now().UTC().Format(time.RFC3339),
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	exec, err := ssh.NewExecutor(node, ssh.ExecOptions{UsePTY: usePTY})
	if err != nil {
		info.Error = fmt.Sprintf("new executor: %v", err)
		return info
	}
	defer exec.Close()

	// 并发跑 7 条命令, 每条独立超时 (整体 ctx 取消时全部中断)
	cmds := map[string]string{
		"hostname":  `hostname`,
		"os":        `uname -s`,
		"kernel":    `uname -r`,
		"arch":      `uname -m`,
		"cpu_cores": `nproc 2>/dev/null || echo 1`,
		"cpu_model": `grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2 | sed 's/^ *//' || sysctl -n machdep.cpu.brand_string 2>/dev/null`,
		"mem_total": `awk '/MemTotal/ {print $2 * 1024}' /proc/meminfo 2>/dev/null || sysctl -n hw.memsize 2>/dev/null`,
		"disk":      `df -B1 / 2>/dev/null | tail -1`,
		"uptime":    `cat /proc/uptime 2>/dev/null || sysctl -n kern.boottime 2>/dev/null`,
	}

	results := make(map[string]cmdResult, len(cmds))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, c := range cmds {
		wg.Add(1)
		go func(n, c string) {
			defer wg.Done()
			res, runErr := exec.RunCommand(ctx, c)
			stdout := ""
			if res != nil {
				stdout = res.Stdout
			}
			mu.Lock()
			results[n] = cmdResult{name: n, out: stdout, err: runErr}
			mu.Unlock()
		}(name, c)
	}
	wg.Wait()

	// 解析
	info.Hostname = strings.TrimSpace(results["hostname"].out)
	info.OS = strings.TrimSpace(results["os"].out)
	info.Kernel = strings.TrimSpace(results["kernel"].out)
	info.Arch = strings.TrimSpace(results["arch"].out)

	if cores, err := strconv.Atoi(strings.TrimSpace(results["cpu_cores"].out)); err == nil && cores > 0 {
		info.CPU.Cores = cores
	} else {
		info.CPU.Cores = 1 // fallback
	}
	info.CPU.Model = strings.TrimSpace(results["cpu_model"].out)

	if memStr := strings.TrimSpace(results["mem_total"].out); memStr != "" {
		if mem, err := strconv.ParseInt(memStr, 10, 64); err == nil && mem > 0 {
			info.Memory.TotalBytes = mem
		}
	}

	// disk: df -B1 / 输出 "Filesystem 1B-blocks Used Available Capacity Mounted-on"
	// 取第 2/3/5 字段: size, used, percent (e.g. "42%")
	if diskOut := strings.TrimSpace(results["disk"].out); diskOut != "" {
		fields := strings.Fields(diskOut)
		if len(fields) >= 5 {
			size, _ := strconv.ParseInt(fields[1], 10, 64)
			used, _ := strconv.ParseInt(fields[2], 10, 64)
			pctStr := strings.TrimSuffix(fields[4], "%")
			pct, _ := strconv.Atoi(pctStr)
			info.Disk = append(info.Disk, DiskInfo{
				Mount:     fields[5],
				SizeBytes: size,
				UsedBytes: used,
				Percent:   pct,
			})
		}
	}

	// uptime: /proc/uptime 第一列是秒数; macOS sysctl kern.boottime 是 epoch 秒
	if upStr := strings.TrimSpace(results["uptime"].out); upStr != "" {
		if idx := strings.Index(upStr, " "); idx > 0 {
			upStr = upStr[:idx] // /proc/uptime: "12345.67 6789.01"
		}
		if secs, err := strconv.ParseFloat(upStr, 64); err == nil && secs > 0 {
			info.UptimeSeconds = secs
			info.UptimeHuman = humanDuration(time.Duration(secs) * time.Second)
		} else {
			// macOS boottime 解析: 取出 epoch 秒数
			if epoch := parseBootTimeEpoch(upStr); epoch > 0 {
				uptime := time.Since(time.Unix(epoch, 0))
				if uptime > 0 {
					info.UptimeSeconds = uptime.Seconds()
					info.UptimeHuman = humanDuration(uptime)
				}
			}
		}
	}

	// 整体错误检查
	if errCount := countNonEmptyErrors(results); errCount > 0 && !info.isComplete() {
		info.Error = fmt.Sprintf("%d/%d collection commands failed", errCount, len(cmds))
	}
	return info
}

// isComplete 检查关键字段是否都采集到了
func (n NodeInfo) isComplete() bool {
	return n.Hostname != "" && n.OS != "" && n.CPU.Cores > 0
}

func countNonEmptyErrors(results map[string]cmdResult) int {
	n := 0
	for _, r := range results {
		if r.err != nil && r.out == "" {
			n++
		}
	}
	return n
}

// parseBootTimeEpoch 从 macOS sysctl kern.boottime 输出提取 epoch 秒
// 格式: "{ sec = 1700000000, usec = 0 }"
func parseBootTimeEpoch(s string) int64 {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return 0
	}
	if idx := strings.Index(s, "sec = "); idx >= 0 {
		rest := s[idx+len("sec = "):]
		if end := strings.IndexAny(rest, ", }"); end > 0 {
			if v, err := strconv.ParseInt(strings.TrimSpace(rest[:end]), 10, 64); err == nil {
				return v
			}
		}
	}
	return 0
}

// humanDuration 把 duration 格式化为 "Nd Nh Nm" 形式 (避免引入额外依赖)
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	mins := int(d / time.Minute)
	d -= time.Duration(mins) * time.Minute
	secs := int(d / time.Second)

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// emitInfoResults 输出结果, 失败节点计数 → exit 1
func emitInfoResults(cmd *cobra.Command, results []NodeInfo) error {
	format := GetOutputFormat(cmd)
	if format == OutputPlain {
		return printInfoTable(cmd.OutOrStdout(), results)
	}
	if err := WriteOut(cmd.OutOrStdout(), results, format); err != nil {
		return err
	}

	// 任一节点失败 → exit 1
	for _, r := range results {
		if r.Error != "" {
			return WrapExit(ExitGeneric,
				fmt.Errorf("%d/%d nodes failed", countInfoFailed(results), len(results)))
		}
	}
	return nil
}

func countInfoFailed(results []NodeInfo) int {
	n := 0
	for _, r := range results {
		if r.Error != "" {
			n++
		}
	}
	return n
}

func printInfoTable(w interface{ Write([]byte) (int, error) }, results []NodeInfo) error {
	// 简化为多行文本, 每节点一段
	type writer interface {
		Write([]byte) (int, error)
	}
	ww, ok := w.(writer)
	if !ok {
		return fmt.Errorf("invalid writer for info table")
	}
	for _, r := range results {
		ww.Write([]byte(fmt.Sprintf("=== %s ===\n", r.Node)))
		if r.Error != "" {
			ww.Write([]byte(fmt.Sprintf("  error: %s\n", r.Error)))
			continue
		}
		ww.Write([]byte(fmt.Sprintf("  hostname:  %s\n", r.Hostname)))
		ww.Write([]byte(fmt.Sprintf("  os:        %s\n", r.OS)))
		ww.Write([]byte(fmt.Sprintf("  kernel:    %s\n", r.Kernel)))
		ww.Write([]byte([]byte(fmt.Sprintf("  arch:      %s\n", r.Arch))))
		ww.Write([]byte(fmt.Sprintf("  cpu:       %d cores", r.CPU.Cores)))
		if r.CPU.Model != "" {
			ww.Write([]byte(fmt.Sprintf(" (%s)", r.CPU.Model)))
		}
		ww.Write([]byte("\n"))
		if r.Memory.TotalBytes > 0 {
			ww.Write([]byte(fmt.Sprintf("  memory:    %s\n",
				humanBytes(r.Memory.TotalBytes))))
		}
		for _, d := range r.Disk {
			ww.Write([]byte(fmt.Sprintf("  disk %s:   %s used / %s total (%d%%)\n",
				d.Mount, humanBytes(d.UsedBytes), humanBytes(d.SizeBytes), d.Percent)))
		}
		if r.UptimeHuman != "" {
			ww.Write([]byte(fmt.Sprintf("  uptime:    %s\n", r.UptimeHuman)))
		}
	}
	return nil
}

// humanBytes 字节数人类可读
func humanBytes(b int64) string {
	const (
		KiB = 1 << 10
		MiB = 1 << 20
		GiB = 1 << 30
		TiB = 1 << 40
	)
	switch {
	case b >= TiB:
		return fmt.Sprintf("%.2f TiB", float64(b)/float64(TiB))
	case b >= GiB:
		return fmt.Sprintf("%.2f GiB", float64(b)/float64(GiB))
	case b >= MiB:
		return fmt.Sprintf("%.2f MiB", float64(b)/float64(MiB))
	case b >= KiB:
		return fmt.Sprintf("%.2f KiB", float64(b)/float64(KiB))
	}
	return fmt.Sprintf("%d B", b)
}
