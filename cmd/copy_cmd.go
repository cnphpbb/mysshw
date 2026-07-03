package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mysshw/config"
	"mysshw/scp"
	"mysshw/ssh"

	"github.com/spf13/cobra"
)

var (
	copyNodesFlag []string
	copyAlias     string
	copyHost      string
	copyUser      string
	copyPort      int
	copyDirection string // "upload" | "download", 由 --upload/--download 解析
	copyLocal     string
	copyRemote    string
	copyMode      string
	copyTimeout   time.Duration
	copyParallel  int
)

var copyCmd = &cobra.Command{
	Use:   "copy [--node N [--node M] | --alias A | --host H [-u U] [-p P]] [--upload | --download] --local <path> --remote <path>",
	Short: "Non-interactive: copy files to/from SSH nodes",
	Long: `Copy a file between local machine and SSH nodes via SCP protocol.

Compared to ` + "`mysshw sync`" + ` (which syncs the whole config file to a single location),
this command copies arbitrary files non-interactively, supports batching across multiple nodes,
and emits structured JSON suitable for AI agents.

Modes:
  --upload    local  → remote on every selected node
  --download  remote → local on the first selected node (batch download not supported)

JSON output (single/batch):
  {
    "node": "prod-1",
    "action": "upload",
    "local": "/local/path",
    "remote": "/remote/path",
    "bytes": 12345,
    "duration": "234ms",
    "error": ""
  }

Exit codes (BSD sysexits):
  0    成功
  1    任一节点失败
  64   usage
  66   本地文件不存在 (EX_NOINPUT)
  73   本地文件创建失败 (EX_CANTCREAT)
  74   IO 错误 (远端路径校验失败 / scp 协议错)
  78   节点未找到 / 配置错
  124  超时
  255  SSH 协议错

Examples:
  # Upload config to a single node
  mysshw copy --node prod-1 --upload --local ./mysshw.toml --remote /etc/mysshw.toml

  # Batch upload to multiple nodes (parallel)
  mysshw copy --node prod-1 --node prod-2 --upload --local ./deploy.sh --remote /tmp/deploy.sh

  # Download to local (single node only)
  mysshw copy --node prod-1 --download --local ./backup.sql --remote /var/db/backup.sql

  # Custom permissions (upload only, octal string)
  mysshw copy --node prod-1 --upload --mode 0755 --local ./script.sh --remote /opt/script.sh

Note: --upload uses -U (uppercase) to avoid clash with -u/--user used by exec/list/tunnel.`,
	Args: cobra.ArbitraryArgs,
	RunE: runCopy,
}

func init() {
	f := copyCmd.Flags()
	f.StringSliceVar(&copyNodesFlag, "node", nil,
		"Node name(s). Repeat for batch: --node X --node Y")
	f.StringVar(&copyAlias, "alias", "", "Node alias (exact)")
	f.StringVar(&copyHost, "host", "", "Node host (combine with -u/-p)")
	f.StringVarP(&copyUser, "user", "u", "", "User for --host selector")
	f.IntVarP(&copyPort, "port", "p", 0, "Port for --host selector (0=any)")
	f.BoolP("upload", "U", false, "Upload local → remote on every node")
	f.BoolP("download", "d", false, "Download remote → local (single node)")
	f.StringVar(&copyLocal, "local", "", "Local file path")
	f.StringVar(&copyRemote, "remote", "", "Remote file path")
	f.StringVar(&copyMode, "mode", "0644",
		"Remote file permissions (octal string, upload only; e.g. 0644, 0755)")
	f.DurationVar(&copyTimeout, "timeout", 5*time.Minute,
		"Per-node transfer timeout")
	f.IntVar(&copyParallel, "parallel", 5,
		"Max concurrent uploads (download always single-node)")
	rootCmd.AddCommand(copyCmd)
}

func runCopy(cmd *cobra.Command, args []string) error {
	if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
		config.CFG_PATH = cfgPath
	}
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("load config: %w", err))
	}

	// 解析 --upload / --download (二选一)
	upload, _ := cmd.Flags().GetBool("upload")
	download, _ := cmd.Flags().GetBool("download")
	switch {
	case upload && download:
		return NewExitError(ExitUsage, "--upload and --download are mutually exclusive")
	case upload:
		copyDirection = "upload"
	case download:
		copyDirection = "download"
	default:
		return NewExitError(ExitUsage, "specify either --upload or --download")
	}

	if copyLocal == "" {
		return NewExitError(ExitUsage, "missing --local")
	}
	if copyRemote == "" {
		return NewExitError(ExitUsage, "missing --remote")
	}

	// 解析节点
	selector := ssh.NodeSelector{
		Names: copyNodesFlag,
		Alias: copyAlias,
		Host:  copyHost,
		User:  copyUser,
		Port:  copyPort,
	}
	nodes, err := ssh.ResolveNodes(config.CFG, selector)
	if err != nil && !isSoftErr(err) {
		if isNodeNotFound(err) {
			return WrapExit(ExitConfig, err)
		}
		return WrapExit(ExitConfig, err)
	}
	if len(nodes) == 0 {
		return WrapExit(ExitConfig, fmt.Errorf("no nodes matched"))
	}
	if copyDirection == "download" && len(nodes) > 1 {
		return NewExitError(ExitUsage,
			"--download requires exactly one node (got %d); use --node to disambiguate",
			len(nodes))
	}

	// 准备本地端前置校验
	var localSize int64
	if copyDirection == "upload" {
		fi, statErr := os.Stat(copyLocal)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				return WrapExit(ExitNoInput,
					fmt.Errorf("local file not found: %s", copyLocal))
			}
			return WrapExit(ExitNoInput, statErr)
		}
		if fi.IsDir() {
			return NewExitError(ExitUsage,
				"local path %s is a directory; this command only supports single files", copyLocal)
		}
		localSize = fi.Size()
	} else {
		// download: 确保父目录存在, 准备好目标文件
		if err := os.MkdirAll(filepath.Dir(copyLocal), 0755); err != nil && filepath.Dir(copyLocal) != "." {
			return WrapExit(ExitCantCreate, fmt.Errorf("create local dir: %w", err))
		}
	}

	// 跑批量
	results := runCopyBatch(cmd, nodes, copyDirection, copyLocal, copyRemote,
		localSize, copyMode, copyTimeout, copyParallel)

	format := GetOutputFormat(cmd)
	if err := WriteOut(cmd.OutOrStdout(), results, format); err != nil {
		return WrapExit(ExitIOErr, err)
	}

	// 任一失败 → ExitGeneric(=1)
	for _, r := range results {
		if r == nil || r.Error != "" {
			return NewExitError(ExitGeneric,
				"copy completed with failures (see -o json for details)")
		}
	}
	return nil
}

// --- 结果 / Batch ---

// CopyResult 单节点传输结果 (JSON-safe)
type CopyResult struct {
	Node     string `json:"node"`
	Action   string `json:"action"` // "upload" | "download"
	Local    string `json:"local"`
	Remote   string `json:"remote"`
	Bytes    int64  `json:"bytes"`
	Duration string `json:"duration"`
	Error    string `json:"error,omitempty"`
}

func runCopyBatch(cmd *cobra.Command, nodes []*config.SSHNode,
	direction, localPath, remotePath string, localSize int64,
	mode string, timeout time.Duration, parallel int) []*CopyResult {
	n := len(nodes)
	results := make([]*CopyResult, n)
	sem := newCopySem(parallel)
	var wg sync.WaitGroup

	for i, node := range nodes {
		wg.Add(1)
		sem.acquire()
		go func(idx int, nd *config.SSHNode) {
			defer wg.Done()
			defer sem.release()

			start := time.Now()
			res := &CopyResult{
				Node:   nd.Name,
				Action: direction,
				Local:  localPath,
				Remote: remotePath,
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			err := doCopyOne(ctx, nd, direction, localPath, remotePath, localSize, mode, res)
			res.Duration = time.Since(start).String()
			if err != nil {
				res.Error = err.Error()
			}
			results[idx] = res
		}(i, node)
	}
	wg.Wait()
	return results
}

func doCopyOne(ctx context.Context, node *config.SSHNode,
	direction, localPath, remotePath string, localSize int64,
	mode string, res *CopyResult) error {

	// 拨号 (利用 ssh.Dial 拿到 *gossh.Client, 复用 NewClientNonInteractive 的认证)
	cli, err := ssh.Dial(node)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer cli.Close()

	// ctx 控制: 用户取消/超时时关 session 会让 scp 命令中断
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = cli.Close()
		}()
	}

	// 通过 scp 包做传输
	sc, err := scp.NewClientBySSHWithTimeout(cli, 0)
	if err != nil {
		return fmt.Errorf("new scp session: %w", err)
	}

	switch direction {
	case "upload":
		// 打开本地文件 + 把 Reader 交给 scp.CopyFile (内部 CopyPassThru 会拷贝到内存再发送)
		f, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("open local: %w", err)
		}
		defer f.Close()

		if err := sc.CopyFile(f, remotePath, mode); err != nil {
			return fmt.Errorf("scp upload: %w", err)
		}
		res.Bytes = localSize
		return nil

	case "download":
		f, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("create local: %w", err)
		}
		defer f.Close()

		if err := sc.CopyFromRemote(f, remotePath); err != nil {
			return fmt.Errorf("scp download: %w", err)
		}

		// 下载后查询字节数
		fi, _ := f.Stat()
		if fi != nil {
			res.Bytes = fi.Size()
		}
		return nil

	default:
		return fmt.Errorf("internal: unknown direction %q", direction)
	}
}

// --- 内部 helper ---

// newSemaphore 与 ssh 包内的相同签名; 这里重新声明一份避免 import cycle
// 实际可以在 cmd 包下构造; 这里直接 borrow 即可 (semaphore 简单到不需要 import)
type copySem chan struct{}

func newCopySem(n int) copySem {
	if n <= 0 {
		return nil
	}
	return make(copySem, n)
}

func (s copySem) acquire() {
	if s == nil {
		return
	}
	s <- struct{}{}
}

func (s copySem) release() {
	if s == nil {
		return
	}
	select {
	case <-s:
	default:
	}
}
