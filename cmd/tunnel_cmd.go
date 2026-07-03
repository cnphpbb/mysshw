package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"mysshw/config"
	"mysshw/ssh"

	"github.com/spf13/cobra"
)

// tunnel group flags (本地 + 远端)
var (
	tunnelNodesFlag []string
	tunnelAlias     string
	tunnelHost      string
	tunnelUser      string
	tunnelPort      int
)

// 本地端口转发 (默认, 等价 `ssh -L`)
var tunnelLocalCmd = &cobra.Command{
	Use:   "local --local 127.0.0.1:8080 --remote 127.0.0.1:80",
	Short: "Local port forwarding (client listens, forwards via SSH)",
	Long: `Forward a local TCP port through an SSH tunnel to a remote endpoint
(reachable from the SSH server).

Equivalent to OpenSSH -L.`,
	Args: cobra.ArbitraryArgs,
	RunE: runTunnelLocal,
}

// 反向端口转发 (等价 `ssh -R`)
var tunnelReverseCmd = &cobra.Command{
	Use:   "reverse --bind :8080 --remote 127.0.0.1:80",
	Short: "Reverse port forwarding (SSH server listens, forwards to client)",
	Long: `Bind a port on the SSH server side, and forward incoming connections
back to the client side, then forward to a local target.

Equivalent to OpenSSH -R.

Use case: expose an internal-only service to the outside through a bastion.`,
	Args: cobra.ArbitraryArgs,
	RunE: runTunnelReverse,
}

// 动态转发 SOCKS5 (等价 `ssh -D`)
var tunnelDynamicCmd = &cobra.Command{
	Use:   "dynamic --local 127.0.0.1:1080",
	Short: "Dynamic port forwarding (SOCKS5 proxy over SSH)",
	Long: `Start a SOCKS5 proxy on the local side. Each connection's destination
is forwarded through the SSH tunnel.

Equivalent to OpenSSH -D.

Use case: route all traffic through a bastion. Configure your browser/system
proxy to use the listening address.`,
	Args: cobra.ArbitraryArgs,
	RunE: runTunnelDynamic,
}

// tunnelCmd 是 parent, 仅作为路由器 / 显示帮助
var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "SSH port forwarding: local / reverse / dynamic (SOCKS5)",
	Long: `Establish various kinds of port forwarding over an existing SSH connection.

Subcommands:
  local     -L  Client listens, forwards to remote target via SSH server
  reverse   -R  SSH server listens, forwards to client-side target
  dynamic   -D  Client SOCKS5 proxy, target resolved per request via SSH server

Examples:
  mysshw tunnel local   --node prod-1 --local 127.0.0.1:8080 --remote 127.0.0.1:80
  mysshw tunnel reverse --node bastion --bind 0.0.0.0:8080 --remote 10.0.0.5:80
  mysshw tunnel dynamic --node bastion --local 127.0.0.1:1080`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	// 子命令共享的节点选择 flags (cobra flag set 各自独立, 但语义一致)
	for _, sub := range []*cobra.Command{tunnelLocalCmd, tunnelReverseCmd, tunnelDynamicCmd} {
		f := sub.Flags()
		f.StringSliceVar(&tunnelNodesFlag, "node", nil,
			"Node name (exact). For tunnel, must select exactly one node.")
		f.StringVar(&tunnelAlias, "alias", "", "Node alias (exact)")
		f.StringVar(&tunnelHost, "host", "", "Node host (combine with -u/-p)")
		f.StringVarP(&tunnelUser, "user", "u", "", "User for --host selector")
		f.IntVarP(&tunnelPort, "port", "p", 0, "Port for --host selector (0=any)")
	}

	// local 子命令专属 flag
	localF := tunnelLocalCmd.Flags()
	localF.String("local", "", "Local listen address (e.g. 127.0.0.1:8080)")
	localF.String("remote", "", "Remote address reachable from SSH server (e.g. 127.0.0.1:80)")
	localF.String("idle-timeout", "0", "Per-connection idle timeout (e.g. 30s)")

	// reverse 子命令专属 flag
	revF := tunnelReverseCmd.Flags()
	revF.String("bind", "", "Bind address on the SSH server (e.g. 0.0.0.0:8080)")
	revF.String("remote", "", "Client-side target (e.g. 127.0.0.1:80)")

	// dynamic 子命令专属 flag
	dynF := tunnelDynamicCmd.Flags()
	dynF.String("local", "", "Local SOCKS5 listen address (e.g. 127.0.0.1:1080)")

	tunnelCmd.AddCommand(tunnelLocalCmd, tunnelReverseCmd, tunnelDynamicCmd)
	rootCmd.AddCommand(tunnelCmd)
}

// --- subcommands dispatcher ---

func runTunnelLocal(cmd *cobra.Command, args []string) error {
	if err := loadTunnelConfig(); err != nil {
		return err
	}
	localBind, _ := cmd.Flags().GetString("local")
	remote, _ := cmd.Flags().GetString("remote")
	idleStr, _ := cmd.Flags().GetString("idle-timeout")

	if localBind == "" {
		return NewExitError(ExitUsage, "missing --local (e.g. --local 127.0.0.1:8080)")
	}
	if remote == "" {
		return NewExitError(ExitUsage, "missing --remote (e.g. --remote 127.0.0.1:80)")
	}
	if _, _, err := net.SplitHostPort(remote); err != nil {
		return NewExitError(ExitUsage, "--remote invalid (expected host:port): %v", err)
	}
	idle, err := parseDuration(idleStr)
	if err != nil {
		return NewExitError(ExitUsage, "invalid --idle-timeout: %v", err)
	}

	node, err := resolveSingleTunnelNode(cmd)
	if err != nil {
		return err
	}

	t, err := ssh.NewTunnel(node, ssh.TunnelOptions{
		LocalBind:   localBind,
		RemoteAddr:  remote,
		IdleTimeout: idle,
	})
	if err != nil {
		return WrapExit(ExitUsage, err)
	}
	runTunnelWithSignal(cmd, t, fmt.Sprintf("local %s ↔ ssh://%s@%s → %s", localBind, node.SetUser(), node.Host, remote))
	return nil
}

func runTunnelReverse(cmd *cobra.Command, args []string) error {
	if err := loadTunnelConfig(); err != nil {
		return err
	}
	bind, _ := cmd.Flags().GetString("bind")
	remote, _ := cmd.Flags().GetString("remote")

	if bind == "" {
		return NewExitError(ExitUsage, "missing --bind (e.g. --bind 0.0.0.0:8080)")
	}
	if remote == "" {
		return NewExitError(ExitUsage, "missing --remote (e.g. --remote 127.0.0.1:80)")
	}
	if _, _, err := net.SplitHostPort(remote); err != nil {
		return NewExitError(ExitUsage, "--remote invalid (expected host:port): %v", err)
	}

	node, err := resolveSingleTunnelNode(cmd)
	if err != nil {
		return err
	}

	rt, err := ssh.NewReverseTunnel(node, ssh.ReverseTunnelOptions{
		BindAddr:   bind,
		TargetAddr: remote,
	})
	if err != nil {
		return WrapExit(ExitUsage, err)
	}
	runTunnelWithSignal(cmd, rt, fmt.Sprintf("reverse ssh://%s:%s ↔ client → %s", node.Host, bind, remote))
	return nil
}

func runTunnelDynamic(cmd *cobra.Command, args []string) error {
	if err := loadTunnelConfig(); err != nil {
		return err
	}
	local, _ := cmd.Flags().GetString("local")
	if local == "" {
		return NewExitError(ExitUsage, "missing --local (e.g. --local 127.0.0.1:1080)")
	}

	node, err := resolveSingleTunnelNode(cmd)
	if err != nil {
		return err
	}

	dt, err := ssh.NewDynamicTunnel(node, ssh.DynamicTunnelOptions{
		ListenAddr: local,
	})
	if err != nil {
		return WrapExit(ExitUsage, err)
	}
	runTunnelWithSignal(cmd, dt, fmt.Sprintf("dynamic SOCKS5 127.0.0.1:%s ↔ ssh://%s@%s (any target)", splitPort(local), node.SetUser(), node.Host))
	return nil
}

// --- shared helpers ---

func loadTunnelConfig() error {
	return loadConfig()
}

// resolveSingleTunnelNode 解析节点选择; tunnel 系列命令必须 exactly 1 个
func resolveSingleTunnelNode(cmd *cobra.Command) (*config.SSHNode, error) {
	selector := ssh.NodeSelector{
		Names: tunnelNodesFlag,
		Alias: tunnelAlias,
		Host:  tunnelHost,
		User:  tunnelUser,
		Port:  tunnelPort,
	}
	nodes, err := ssh.ResolveNodes(config.CFG, selector)
	if err != nil && !isSoftErr(err) {
		if isNodeNotFound(err) {
			return nil, WrapExit(ExitConfig, err)
		}
		return nil, WrapExit(ExitConfig, err)
	}
	if len(nodes) != 1 {
		return nil, NewExitError(ExitUsage,
			"tunnel requires exactly one node, got %d (use --node/--alias/--host)",
			len(nodes))
	}
	return nodes[0], nil
}

// runTunnelWithSignal 通用 signal handler + 输出 banner
func runTunnelWithSignal(cmd *cobra.Command, t interface {
	Close() error
}, banner string) {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(cmd.ErrOrStderr(), "mysshw:: received signal, stopping tunnel...")
		_ = t.Close()
	}()

	fmt.Fprintf(cmd.ErrOrStderr(), "mysshw:: tunnel started: %s (Ctrl+C to stop)\n", banner)

	type starter interface {
		Start(context.Context) error
	}
	st, ok := t.(starter)
	if !ok {
		fmt.Fprintln(cmd.ErrOrStderr(), "mysshw:: internal: tunnel does not implement Start")
		return
	}
	if err := st.Start(ctx); err != nil {
		if isCancelLike(err) {
			fmt.Fprintln(cmd.ErrOrStderr(), "mysshw:: tunnel stopped")
			return
		}
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "listen"):
			HandleExit(WrapExit(ExitCantCreate, err))
		default:
			HandleExit(WrapExit(ExitSSHError, err))
		}
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "mysshw:: tunnel stopped")
}

func splitPort(addr string) string {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "?"
	}
	return p
}
