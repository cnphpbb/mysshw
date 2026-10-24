package cmd

import (
	"fmt"
	"io"
	"os"

	"mysshw/config"

	"github.com/spf13/cobra"
)

// ============================================================
// mysshw config check
//
// 对本地配置文件做一次"关键配置项体检":
//   - 节点项: 凭据 / 明文密码 / 重复名 / proxy 引用 / 主机指纹校验 / 端口
//   - sync 项: 协议必填项 + 加密参数 (encrypt / passphrase)
//   - 加密字段: encryption_algorithm + secret_key 配对
//   - 文件权限: 建议 0600
//
// 与启动时的自动检查区别: 启动时只报 warn / error (不刷屏),
// `config check` 报全部级别 (含 info), 且支持 -o json 供 AI agent 消费。
// ============================================================

var configCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check whether the local config's key items are configured correctly",
	Long: `Check the key configuration items of the local config file.

Checks:
  - SSH nodes: credential, plaintext password, duplicated name/alias,
    proxy reference, strict_host_key_checking, port range
  - Sync: protocol required fields + encryption parameters (encrypt / passphrase)
  - Encryption fields: encryption_algorithm + secret_key pairing
  - File permission: 0600 recommended

Exit code:
  0  no error-level issue
  1  at least one error-level issue found
  2  config file could not be read`,
	RunE: runConfigCheck,
}

func init() {
	configCmd.AddCommand(configCheckCmd)
}

// configCheckResult config check 的 JSON 输出结构
type configCheckResult struct {
	// ConfigPath 实际检查的配置文件路径
	ConfigPath string `json:"config_path"`
	// OK 是否没有 error 级别问题
	OK bool `json:"ok"`
	// Counts 各级别问题数量
	Counts map[string]int `json:"counts"`
	// Issues 问题明细
	Issues []config.ConfigIssue `json:"issues"`
}

func runConfigCheck(cmd *cobra.Command, args []string) error {
	cfgPath := config.CFG_PATH
	if cfgPath == "" {
		cfgPath = "~/.mysshw.toml"
	}

	resolved, err := config.GetCfgPath(cfgPath)
	if err != nil {
		return writeConfigCheckResult(cmd, cfgPath, []config.ConfigIssue{{
			Level:   config.IssueError,
			Item:    "config_file",
			Message: fmt.Sprintf("cannot resolve config path: %v", err),
			Hint:    "check the --cfg value",
		}}, true)
	}

	// 只读方式读取 (不修改用户配置)
	data, readErr := os.ReadFile(resolved)
	if readErr != nil {
		return writeConfigCheckResult(cmd, resolved, []config.ConfigIssue{{
			Level:   config.IssueError,
			Item:    "config_file",
			Message: fmt.Sprintf("cannot read config file: %v", readErr),
			Hint:    "check the --cfg value, or run: mysshw init",
		}}, true)
	}

	var cfg *config.Configs
	if config.IsEncryptedFile(data) {
		// 加密配置: 走加载器 (会交互式提示输入口令), 并抑制加载器的检查报告
		config.SetConfigCheckOutput(io.Discard)
		defer config.SetConfigCheckOutput(nil)
		if loadErr := config.LoadViperConfig(cfgPath); loadErr != nil {
			return writeConfigCheckResult(cmd, resolved, []config.ConfigIssue{{
				Level:   config.IssueError,
				Item:    "config_file",
				Message: fmt.Sprintf("failed to load encrypted config: %v", loadErr),
				Hint:    "check the passphrase",
			}}, true)
		}
		cfg = config.CFG
	} else {
		cfg, err = config.ReadConfigOrEmpty(resolved)
		if err != nil {
			return writeConfigCheckResult(cmd, resolved, []config.ConfigIssue{{
				Level:   config.IssueError,
				Item:    "config_file",
				Message: fmt.Sprintf("failed to parse config: %v", err),
				Hint:    "fix the TOML syntax, or restore a backup",
			}}, true)
		}
	}

	issues := config.CheckConfigItems(cfg)
	// 文件权限检查
	issues = append(issues, config.CheckConfigFilePerm(resolved)...)

	return writeConfigCheckResult(cmd, resolved, issues, false)
}

// writeConfigCheckResult 输出检查结果并按 error 数量决定退出码
func writeConfigCheckResult(cmd *cobra.Command, cfgPath string, issues []config.ConfigIssue, loadFailed bool) error {
	counts := config.CountIssuesByLevel(issues)
	ok := counts["error"] == 0

	switch GetOutputFormat(cmd) {
	case OutputJSON:
		result := configCheckResult{
			ConfigPath: cfgPath,
			OK:         ok,
			Counts:     counts,
			Issues:     issues,
		}
		if result.Issues == nil {
			result.Issues = []config.ConfigIssue{}
		}
		if err := WriteOut(cmd.OutOrStdout(), result, OutputJSON); err != nil {
			return err
		}
	default:
		fmt.Fprint(cmd.OutOrStdout(), config.FormatConfigIssues(issues))
	}

	if !ok {
		return WrapExit(ExitConfig, fmt.Errorf("config check found %d error-level issue(s)", counts["error"]))
	}
	if loadFailed {
		return WrapExit(ExitConfig, fmt.Errorf("config check could not load the config file"))
	}
	return nil
}
