package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mysshw/config"
	"mysshw/ssh"

	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
)

// ============================================================
// hostkey 子命令家族
//
// 主命令 + 4 个子命令:
//   mysshw hostkey list            列出全部 known_hosts 条目 (JSON)
//   mysshw hostkey show HOST       显示某个 host 的 fingerprint
//   mysshw hostkey remove HOST    删除某个 host 的条目
//   mysshw hostkey update HOST --node N  通过 mysshw 连接主动获取新 key 并更新
//
// 设计目标:
//   - JSON schema 稳定, 供 AI agent 写入自动化脚本
//   - 默认走现有的 ~/.ssh/known_hosts (与 OpenSSH 共享)
//   - update 子命令用以替代 --rotate-known-hosts flag (更可控, 用户显式触发 + AI 友好)
// ============================================================

var hostkeyCmd = &cobra.Command{
	Use:   "hostkey",
	Short: "Manage known_hosts entries (list / show / remove / update)",
	Long: `Inspect and modify the host key trust database (known_hosts).

Unlike --rotate-known-hosts flag (which silently replaces keys on mismatch),
this command gives you explicit, auditable control over individual host entries.

Default location: ~/.ssh/known_hosts (shared with OpenSSH).
Override via MYSSHW_KNOWN_HOSTS env or config.known_hosts_path.

All subcommands accept -o plain|json for machine parsing.

Examples:
  mysshw hostkey list
  mysshw hostkey list -o json | jq '.[] | select(.host=="prod-1")'
  mysshw hostkey show prod-1.example.com
  mysshw hostkey remove prod-1.example.com
  mysshw hostkey update prod-1.example.com --node prod-1

Exit codes (BSD sysexits):
  0    成功
  64   usage
  66   输入文件缺失
  78   not found / config error
  124  timeout
  255  SSH error`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// ============================================================
// hostkeyCmd 子命令 flags + RunE
// ============================================================

// hostkeyListCmd 列出全部 known_hosts 条目
var hostkeyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known_hosts entries",
	Long: `Parse the known_hosts file and print every entry as a structured record.

JSON schema:
  [
    {
      "host": "[prod-1.example.com]:2222",  // 原始 host 字段 (含可选 port/comma list)
      "host_names": ["prod-1.example.com"], // 解出来的 host(s)
      "algorithm": "ssh-ed25519",
      "fingerprint": "SHA256:abc...",
      "is_hashed": false
    }
  ]`,
	Args: cobra.NoArgs,
	RunE: runHostkeyList,
}

var hostkeyShowCmd = &cobra.Command{
	Use:   "show <host>",
	Short: "Show known_hosts entries for a specific host",
	Long: `Look up a specific host and return its known_hosts entries as JSON.

If the host is not present, exit code 78 (EX_CONFIG).`,
	Args: cobra.MinimumNArgs(1),
	RunE: runHostkeyShow,
}

var hostkeyRemoveCmd = &cobra.Command{
	Use:   "remove <host>",
	Short: "Remove all known_hosts entries for a specific host",
	Long: `Delete every entry whose host field matches <host> from known_hosts.

Useful after confirmed key rotation (admin has distributed new fingerprint through
a secure out-of-band channel). Compare with --rotate-known-hosts flag:
  - remove <host>     : explicit delete, no SSH connection
  - --rotate-known-hosts : replace on next dial (after explicit opt-in)

Exit code 0 even if host was not found (no-op).`,
	Args: cobra.MinimumNArgs(1),
	RunE: runHostkeyRemove,
}

var hostkeyUpdateCmd = &cobra.Command{
	Use:   "update <host> --node <name> [--confirm] [--expect-fingerprint SHA256:...]",
	Short: "Update known_hosts by actively re-fetching the key from a node",
	Long: `Connect to <node>, fetch its server host public key(s), and replace
(or insert) the entry for <host> in known_hosts. Safer than --rotate-known-hosts
flag because the operation is explicit and audit-friendly.

By default runs in DRY-RUN mode: prints planned action without modifying known_hosts.
Pass --confirm to actually apply.

Security workflow:
  1. Admin rotates server key, distributes new fingerprint through trusted channel
  2. (Optional) You: mysshw hostkey update prod-1 --node prod-1 \
                       --expect-fingerprint SHA256:abc...
       -- only writes if server's actual fingerprint matches expected
  3. Without --expect-fingerprint: any fingerprint accepted (still requires --confirm)

Examples:
  # Dry-run: see what would be done
  mysshw hostkey update prod-1.example.com --node prod-1

  # Confirm-after-rotate (replace old key with server's new key)
  mysshw hostkey update prod-1.example.com --node prod-1 --confirm

  # Confirm-with-expected-fingerprint (MITM-protected: only if matches)
  mysshw hostkey update prod-1.example.com --node prod-1 \
            --expect-fingerprint SHA256:abc... \
            --confirm

Exit codes:
  0    成功 (或 dry-run OK 供 review)
  64   usage
  78   node not found / config error
  124  timeout
  124  fingerprint mismatch (用了 --expect-fingerprint 但不匹配)
  255  SSH error`,
	Args: cobra.MinimumNArgs(1),
	RunE: runHostkeyUpdate,
}

func init() {
	// hostkey show / remove / update: 第一个位置参数为 host
	hostkeyShowCmd.Args = cobra.ExactArgs(1)
	hostkeyRemoveCmd.Args = cobra.ExactArgs(1)
	hostkeyUpdateCmd.Args = cobra.ExactArgs(1)

	// update 子命令专属 flag
	updateF := hostkeyUpdateCmd.Flags()
	updateF.StringVar(&hostkeyUpdateNode, "node", "",
		"Node name to connect to (the SSH server corresponding to <host>)")
	updateF.DurationVar(&hostkeyUpdateTimeout, "timeout", 30*time.Second,
		"Dial timeout")
	updateF.BoolVar(&hostkeyUpdateConfirm, "confirm", false,
		"Actually modify known_hosts (default: dry-run; print plan only)")
	updateF.StringVar(&hostkeyUpdateExpectFP, "expect-fingerprint", "",
		"Expected server fingerprint (SHA256:base64). If set and mismatch, exit 124 without modifying.")

	hostkeyCmd.AddCommand(hostkeyListCmd, hostkeyShowCmd, hostkeyRemoveCmd, hostkeyUpdateCmd)
	rootCmd.AddCommand(hostkeyCmd)
}

// hostkeyUpdate 子命令 flag 变量
var (
	hostkeyUpdateNode     string
	hostkeyUpdateTimeout  time.Duration
	hostkeyUpdateConfirm  bool
	hostkeyUpdateExpectFP string
)

// ============================================================
// list / show / remove / update handlers
// ============================================================

func runHostkeyList(cmd *cobra.Command, args []string) error {
	path, err := ssh.GetKnownHostsPath()
	if err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("locate known_hosts: %w", err))
	}

	entries, parseErr := parseKnownHosts(path)
	if parseErr != nil {
		return WrapExit(ExitIOErr, fmt.Errorf("parse known_hosts: %w", parseErr))
	}

	// sort: host ascending, 然后 algorithm, 再 fingerprint
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Host != entries[j].Host {
			return entries[i].Host < entries[j].Host
		}
		if entries[i].Algorithm != entries[j].Algorithm {
			return entries[i].Algorithm < entries[j].Algorithm
		}
		return entries[i].Fingerprint < entries[j].Fingerprint
	})

	return WriteOut(cmd.OutOrStdout(), entries, GetOutputFormat(cmd))
}

func runHostkeyShow(cmd *cobra.Command, args []string) error {
	host := normalizeHostArg(args[0])

	path, err := ssh.GetKnownHostsPath()
	if err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("locate known_hosts: %w", err))
	}

	all, _ := parseKnownHosts(path)
	var matches []knownhostsEntry
	for _, e := range all {
		if entryHostMatches(e.Host, host) {
			matches = append(matches, e)
		}
	}

	if len(matches) == 0 {
		return WrapExit(ExitConfig,
			fmt.Errorf("host %q not found in %s", host, path))
	}

	result := struct {
		Host        string            `json:"host"`
		RequestedAs string            `json:"requested_as"`
		Entries     []knownhostsEntry `json:"entries"`
		FoundInFile string            `json:"found_in_file"`
	}{
		Host:        host,
		RequestedAs: args[0],
		Entries:     matches,
		FoundInFile: path,
	}
	return WriteOut(cmd.OutOrStdout(), result, GetOutputFormat(cmd))
}

func runHostkeyRemove(cmd *cobra.Command, args []string) error {
	host := normalizeHostArg(args[0])

	path, err := ssh.GetKnownHostsPath()
	if err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("locate known_hosts: %w", err))
	}

	before, _ := parseKnownHosts(path)
	beforeCount := 0
	for _, e := range before {
		if entryHostMatches(e.Host, host) {
			beforeCount++
		}
	}

	if err := ssh.RemoveKnownHost(path, host); err != nil {
		return WrapExit(ExitIOErr, fmt.Errorf("remove: %w", err))
	}

	after, _ := parseKnownHosts(path)
	afterCount := 0
	for _, e := range after {
		if entryHostMatches(e.Host, host) {
			afterCount++
		}
	}

	removed := beforeCount - afterCount

	result := struct {
		Host            string `json:"host"`
		RemovedFromFile string `json:"removed_from_file"`
		LinesRemoved    int    `json:"lines_removed"`
	}{
		Host:            host,
		RemovedFromFile: path,
		LinesRemoved:    removed,
	}
	return WriteOut(cmd.OutOrStdout(), result, GetOutputFormat(cmd))
}

func runHostkeyUpdate(cmd *cobra.Command, args []string) error {
	if hostkeyUpdateNode == "" {
		return NewExitError(ExitUsage, "missing --node (the node name to connect to)")
	}
	host := normalizeHostArg(args[0])

	if cfgPath, _ := cmd.Flags().GetString("cfg"); cfgPath != "" {
		config.CFG_PATH = cfgPath
	}
	if err := config.LoadViperConfig(config.CFG_PATH); err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("load config: %w", err))
	}
	node, err := ssh.ResolveNodes(config.CFG, ssh.NodeSelector{Names: []string{hostkeyUpdateNode}})
	if err != nil {
		if isNodeNotFound(err) {
			return WrapExit(ExitConfig, err)
		}
		return WrapExit(ExitConfig, err)
	}
	if len(node) == 0 {
		return WrapExit(ExitConfig, fmt.Errorf("node %q not found", hostkeyUpdateNode))
	}

	// 拨号拿 server keys
	cli, hostKeys, dialErr := ssh.DialCaptureHostKeys(node[0])
	if dialErr != nil {
		return WrapExit(ExitSSHError, fmt.Errorf("dial %s: %w", node[0].Name, dialErr))
	}
	defer cli.Close()

	if len(hostKeys) == 0 {
		return WrapExit(ExitSSHError, fmt.Errorf("no host keys reported by server"))
	}

	// 比对 expected fingerprint (如果有)
	if hostkeyUpdateExpectFP != "" {
		matched := false
		for _, key := range hostKeys {
			if gossh.FingerprintSHA256(key) == hostkeyUpdateExpectFP {
				matched = true
				break
			}
		}
		if !matched {
			actualFPs := make([]string, 0, len(hostKeys))
			for _, key := range hostKeys {
				actualFPs = append(actualFPs, gossh.FingerprintSHA256(key))
			}
			return NewExitError(ExitTimeout,
				"--expect-fingerprint mismatch: server actual=%v, expected=%s (refusing to modify known_hosts)",
				actualFPs, hostkeyUpdateExpectFP)
		}
	}

	// 收集旧 fingerprint
	kh, err := ssh.GetKnownHostsPath()
	if err != nil {
		return WrapExit(ExitConfig, fmt.Errorf("locate known_hosts: %w", err))
	}
	before, _ := parseKnownHosts(kh)
	var oldFPs []string
	for _, e := range before {
		if entryHostMatches(e.Host, host) {
			oldFPs = append(oldFPs, e.Fingerprint)
		}
	}

	// 新 fingerprint 列表
	newFPs := make([]string, 0, len(hostKeys))
	for _, key := range hostKeys {
		newFPs = append(newFPs, gossh.FingerprintSHA256(key))
	}

	result := struct {
		Host            string            `json:"host"`
		ConnectedNode   string            `json:"connected_node"`
		OldEntries      []knownhostsEntry `json:"old_entries"`
		NewKeys         []string          `json:"new_keys"` // fingerprints
		WouldReplace    bool              `json:"would_replace"`
		ReplaceApproved bool              `json:"replace_approved"`
		UpdatedAt       string            `json:"updated_at"`
		WrittenToFile   string            `json:"written_to_file"`
	}{
		Host:            host,
		ConnectedNode:   node[0].Name,
		OldEntries:      oldEntriesFor(before, host),
		NewKeys:         newFPs,
		WouldReplace:    hostkeyUpdateConfirm,
		ReplaceApproved: hostkeyUpdateConfirm,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
		WrittenToFile:   kh,
	}

	if !hostkeyUpdateConfirm {
		// Dry-run: 仅打印计划
		_ = oldFPs // suppress unused warning
		return WriteOut(cmd.OutOrStdout(), result, GetOutputFormat(cmd))
	}

	// 实际写入: 先删除旧 entry, 再 append 新 keys
	if err := ssh.RemoveKnownHost(kh, host); err != nil {
		return WrapExit(ExitIOErr, fmt.Errorf("remove old: %w", err))
	}
	for _, key := range hostKeys {
		if err := ssh.AppendKnownHost(kh, host, key); err != nil {
			return WrapExit(ExitIOErr, fmt.Errorf("append new: %w", err))
		}
	}

	// 重读以便 audit 输出新 entries
	after, _ := parseKnownHosts(kh)
	result.OldEntries = oldEntriesFor(before, host)
	_ = after

	return WriteOut(cmd.OutOrStdout(), result, GetOutputFormat(cmd))
}

// oldEntriesFor 提取 host 的旧条目
func oldEntriesFor(all []knownhostsEntry, host string) []knownhostsEntry {
	var out []knownhostsEntry
	for _, e := range all {
		if entryHostMatches(e.Host, host) {
			out = append(out, e)
		}
	}
	return out
}

// ============================================================
// 共享 helper
// ============================================================

// knownhostsEntry 单条 known_hosts 记录 (JSON-safe)
type knownhostsEntry struct {
	Host        string   `json:"host"`        // 原始 host 字段
	HostNames   []string `json:"host_names"`  // 解析后的 host 列表 (去 port / 拆 comma)
	Algorithm   string   `json:"algorithm"`   // e.g. "ssh-ed25519"
	RawKey      string   `json:"raw_key"`     // base64 编码 (raw public key wire format)
	Fingerprint string   `json:"fingerprint"` // SHA256:base64
	IsHashed    bool     `json:"is_hashed"`   // host 以 |1|salt|hash 形式
	LineNo      int      `json:"line_no"`     // 1-based, 用于审计
}

// parseKnownHosts 解析 known_hosts 文件
// 输出按文件中出现顺序 (sorted by caller, 通常)
// 注释行 / 空行 忽略
func parseKnownHosts(path string) ([]knownhostsEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []knownhostsEntry
	// 读取时跟踪行号 (1-based)
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	for lineNo, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		hostField := fields[0]
		algo := fields[1]
		rawKeyB64 := fields[2]
		lineNumber := lineNo + 1

		isHashed := strings.HasPrefix(hostField, "|1|") || strings.HasPrefix(hostField, "|2|")

		// 解析 host names (去 brackets/port/comma 列表)
		hostNames := extractHostNames(hostField)
		keyUnparseable := false

		// 解码 raw key 并计算 fingerprint
		wire, decErr := base64.StdEncoding.DecodeString(rawKeyB64)
		fp := "(decode error)"
		if decErr == nil {
			key, parseErr := gossh.ParsePublicKey(wire)
			if parseErr == nil {
				fp = gossh.FingerprintSHA256(key)
			} else {
				fp = "(parse error)"
				keyUnparseable = true
			}
		} else {
			keyUnparseable = true
		}
		if keyUnparseable {
			// 标记损坏行但不加额外 host name token
			if fp == "(decode error)" || fp == "(parse error)" {
				fp = fp
			}
		}

		entries = append(entries, knownhostsEntry{
			Host:        hostField,
			HostNames:   hostNames,
			Algorithm:   algo,
			RawKey:      rawKeyB64,
			Fingerprint: fp,
			IsHashed:    isHashed,
			LineNo:      lineNumber,
		})
	}
	return entries, nil
}

// extractHostNames 从 host 字段解析出 host 列表
//
// 处理:
//
//	"example.com"               → ["example.com"]
//	"[example.com]:2222"          → ["example.com"]
//	"a.example.com,b.example.com" → ["a.example.com", "b.example.com"]
//	"|1|salt|hash"              → ["<hashed>"]
func extractHostNames(hostField string) []string {
	if strings.HasPrefix(hostField, "|1|") || strings.HasPrefix(hostField, "|2|") {
		return []string{"<hashed>"}
	}
	cleaned := hostField
	if strings.HasPrefix(cleaned, "[") && strings.Contains(cleaned, "]") {
		cleaned = strings.TrimPrefix(cleaned, "[")
		if idx := strings.Index(cleaned, "]"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
	} else if idx := strings.Index(cleaned, ":"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	out := []string{}
	for _, p := range strings.Split(cleaned, ",") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// normalizeHostArg 规范化用户传入的 host 参数
//
//   - 去末尾 ':'  (bash completion 容易带)
//   - 默认无 port 时, 加 :22 (与 callback 路径一致)
func normalizeHostArg(h string) string {
	h = strings.TrimSpace(h)
	h = strings.TrimSuffix(h, ":")
	if h == "" {
		return h
	}
	if _, _, e := net.SplitHostPort(h); e != nil {
		// 无 port, 不动 (调用方按 host 名处理)
	}
	return h
}

// entryHostMatches 判断某 entry 是否 match 用户请求的 host
func entryHostMatches(entryHostField, queryHost string) bool {
	for _, n := range extractHostNames(entryHostField) {
		if n == queryHost {
			return true
		}
	}
	return false
}

// 防止 unused import 警告 (os/exit 用得到, time 在 update 用)
var _ = filepath.Join
var _ = context.Background
