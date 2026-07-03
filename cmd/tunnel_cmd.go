package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mysshw/config"
	"mysshw/ssh"

	"github.com/spf13/cobra"
)

var (
	tunnelLocal  string
	tunnelRemote string
	tunnelIdle   string // duration string
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel [--node N | --alias A | --host H [-u U] [-p P]] --local 127.0.0.1:8080 --remote 127.0.0.1:80",
	Short: "Local port forwarding over SSH",
	Long: `Forward a local TCP port through an SSH tunnel to a remote endpoint
(reachable from the SSH server).

Mode: local port forwarding (SSH client listens locally, forwards via SSH to remote).

This is non-interactive: it connects on first use, then blocks. Send SIGINT (Ctrl+C)
or SIGTERM to stop.

Exit codes:
  0    tunnel stopped cleanly (signal or ctx cancelled)
  64   usage error (missing/invalid flags)
  73   local bind failed (port in use / permission)
  74   IO error (SSH dial failed, mid-stream drop)
  78   node not found
  255  raw SSH protocol error

Examples:
  # Forward localhost:8080 → prod-1:80 (典型场景: 看 web 服务)
  mysshw tunnel --node prod-1 --local 127.0.0.1:8080 --remote 127.0.0.1:80

  # 监听所有接口
  mysshw tunnel --node prod-1 --local :8080 --remote db.internal:5432

  # 远程目标用别名 (依赖 /etc/hosts on SSH server)
  mysshw tunnel --node bastion --local :2222 --remote internal-host:22`,
	Args: cobra.ArbitraryArgs,
	RunE: runTunnel,
}

func init() {
	f := tunnelCmd.Flags()
	f.StringVar(&tunnelLocal, "local", "",
		"Local listen address (e.g. 127.0.0.1:8080 or :8080). Required.")
	f.StringVar(&tunnelRemote, "remote", "",
		"Remote address reachable from the SSH server (e.g. 127.0.0.1:80). Required.")
	f.StringVar(&tunnelIdle, "idle-timeout", "0",
		"Per-connection idle timeout (e.g. 30s, 5m); 0=no limit")
	rootCmd.AddCommand(tunnelCmd)
}

func runTunnel(cmd *cobra.Command, args []string) error {
	if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
		config.CFG_PATH = cfgPath
	}
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("load config: %w", err))
	}

	// 参数校验
	if tunnelLocal == "" {
		return NewExitError(ExitUsage, "missing --local (e.g. --local 127.0.0.1:8080)")
	}
	if tunnelRemote == "" {
		return NewExitError(ExitUsage, "missing --remote (e.g. --remote 127.0.0.1:80)")
	}
	if !strings.Contains(tunnelLocal, ":") {
		return NewExitError(ExitUsage, "--local must include port (e.g. 127.0.0.1:8080 or :8080)")
	}
	if _, _, err := net.SplitHostPort(tunnelRemote); err != nil {
		return NewExitError(ExitUsage, "--remote invalid (expected host:port): %v", err)
	}

	idle, err := parseDuration(tunnelIdle)
	if err != nil {
		return NewExitError(ExitUsage, "invalid --idle-timeout %q: %v", tunnelIdle, err)
	}

	selector := ssh.NodeSelector{
		Names: tunnelSelectorNames, // 从根命令同名 flag 读取
		Alias: tunnelAlias,
		Host:  tunnelHost,
		User:  tunnelUser,
		Port:  tunnelPort,
	}
	nodes, err := ssh.ResolveNodes(config.CFG, selector)
	if err != nil && !isSoftErr(err) {
		if isNodeNotFound(err) {
			return WrapExit(ExitConfig, err)
		}
		return WrapExit(ExitConfig, err)
	}
	if len(nodes) != 1 {
		// tunnel 暂只支持单节点
		return NewExitError(ExitUsage,
			"tunnel requires exactly one node, got %d (use --node/--alias/--host to disambiguate)",
			len(nodes))
	}

	t, err := ssh.NewTunnel(nodes[0], ssh.TunnelOptions{
		LocalBind:   tunnelLocal,
		RemoteAddr:  tunnelRemote,
		IdleTimeout: idle,
	})
	if err != nil {
		return WrapExit(ExitUsage, err)
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Ctrl+C / SIGTERM 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(cmd.ErrOrStderr(), "mysshw:: received signal, stopping tunnel...")
		_ = t.Close()
	}()

	// 启动提示
	fmt.Fprintf(cmd.ErrOrStderr(),
		"mysshw:: tunnel started: local %s ↔ ssh://%s@%s → %s (Ctrl+C to stop)\n",
		tunnelLocal, nodes[0].SetUser(), nodes[0].Host, tunnelRemote)

	// 阻塞运行
	if startErr := t.Start(ctx); startErr != nil {
		// 区分错误类型
		if isCancelLike(startErr) {
			fmt.Fprintln(cmd.ErrOrStderr(), "mysshw:: tunnel stopped")
			return nil
		}
		if strings.Contains(startErr.Error(), "listen") {
			return WrapExit(ExitCantCreate, startErr)
		}
		return WrapExit(ExitSSHError, startErr)
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "mysshw:: tunnel stopped")
	return nil
}

// --- 内部辅助 ---

// 全局变量: tunnel 自己的 selector flags
var (
	tunnelSelectorNames []string
	tunnelAlias         string
	tunnelHost          string
	tunnelUser          string
	tunnelPort          int
)

func init() {
	// 在 tunnel_cmd.go 内复用一份 selector flags
	// 注: cobra 的 flag binding 用 cmd-local variable, 这里共享全局以避免每个子命令重复
	tunnelCmd.Flags().StringSliceVar(&tunnelSelectorNames, "node", nil,
		"Node name (exact). For tunnel, must select exactly one node.")
	tunnelCmd.Flags().StringVar(&tunnelAlias, "alias", "", "Node alias (exact)")
	tunnelCmd.Flags().StringVar(&tunnelHost, "host", "", "Node host (combine with -u/-p)")
	tunnelCmd.Flags().StringVarP(&tunnelUser, "user", "u", "", "User for --host selector")
	tunnelCmd.Flags().IntVarP(&tunnelPort, "port", "p", 0, "Port for --host selector (0=any)")
}

func isSoftErr(err error) bool {
	if err == nil {
		return false
	}
	// ssh.ResolveNodes 对 Names 部分缺失返回 soft error (含 matches)
	return strings.Contains(err.Error(), "continuing with")
}

func isNodeNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "node not found")
}

func isCancelLike(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "context canceled") ||
		strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "closed pipe")
}

// parseDuration 解析 --idle-timeout
func parseDuration(s string) (time.Duration, error) {
	if s == "" || s == "0" {
		return 0, nil
	}
	return time.ParseDuration(s)
}
