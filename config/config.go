/**
 * @Author   DenysGeng <cnphp@hotmail.com>
 *
 * @Description: 采用toml做为新版本的配置文件
 * @Version: 	1.0.0
 * @Date:     	2021/9/23
 * @Updated:     2025-04-04
 */

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GuanceCloud/toml"
	"github.com/spf13/viper"
)

var (
	CFG_PATH     string = "~/.mysshw.toml"
	CFG_EXT_TYPE string = "toml"
	//LOG_PATH  = "mysshw.log"
	CFG *Configs
)

// LoadConfigBytes 加载配置文件内容为字节切片
func LoadConfigBytes(cfgPath string) ([]byte, error) {
	expanded, err := getConfigPath(cfgPath)
	if err != nil {
		return nil, err
	}
	CFG_PATH = expanded
	// 使用 os.ReadFile 替代已弃用的 ioutil.ReadFile
	cfgBytes, err1 := os.ReadFile(CFG_PATH)
	if err1 == nil {
		return cfgBytes, nil
	}
	return nil, err
}

// LoadConfig 加载配置文件
func LoadConfig() error {
	cfgBytes, err := LoadConfigBytes(CFG_PATH)
	if err != nil {
		return err
	}
	var c *Configs
	err = toml.Unmarshal(cfgBytes, &c)
	if err != nil {
		return err
	}
	CFG = c
	return nil
}

// unmarshalAndValidateConfig 解析并验证配置
func unmarshalAndValidateConfig(c *Configs) error {
	err := viper.Unmarshal(c)
	if err != nil {
		return fmt.Errorf("mysshw:: Failed to unmarshal configuration: %v", err)
	}
	CFG = c

	// 验证配置
	if err := ValidateConfig(c); err != nil {
		return fmt.Errorf("mysshw:: Configuration validation failed: %v", err)
	}

	// 关键配置项体检 (软检查, 不阻止启动; 告警写 stderr 不污染 stdout)
	ReportConfigIssues(c, CFG_PATH)

	// 安全加固: 加载成功后强制配置文件权限为 0600
	// 配置文件可能含密码/keypath 等敏感信息
	secureConfigFile(CFG_PATH)

	return nil
}

// configCheckOutput 配置检查报告的输出目标 (默认 stderr)
// 设为 io.Discard 可关闭报告 (例如 `mysshw config check` 自己输出结果)
var configCheckOutput io.Writer = os.Stderr

// SetConfigCheckOutput 设置配置检查报告的输出目标 (传 nil 恢复默认 stderr)
func SetConfigCheckOutput(w io.Writer) {
	if w == nil {
		configCheckOutput = os.Stderr
		return
	}
	configCheckOutput = w
}

// ReportConfigIssues 检查本地配置文件的关键配置项并把问题输出到 stderr
//
// 启动时 (以及任何加载配置的命令) 自动调用:
// 让"关键配置项没配 / 配得不安全"在真正用到之前就暴露出来。
// 输出走 stderr, 保证 `mysshw list -o json` 之类的机器可读输出不被污染。
//
// 默认只输出 warn / error 级别 (info 级别如 "user 未配置, 用默认 root" 过于刷屏,
// 需要完整视图时用 `mysshw config check`)。
func ReportConfigIssues(cfg *Configs, cfgPath string) []ConfigIssue {
	return ReportConfigIssuesAt(cfg, cfgPath, IssueWarn)
}

// ReportConfigIssuesAt 与 ReportConfigIssues 相同, 但可指定最低输出级别
func ReportConfigIssuesAt(cfg *Configs, cfgPath string, minLevel IssueLevel) []ConfigIssue {
	issues := CheckConfigItems(cfg)
	if cfgPath != "" {
		issues = append(issues, CheckConfigFilePerm(cfgPath)...)
	}
	shown := make([]ConfigIssue, 0, len(issues))
	for _, it := range issues {
		if levelRank(it.Level) >= levelRank(minLevel) {
			shown = append(shown, it)
		}
	}
	if len(shown) > 0 {
		fmt.Fprint(configCheckOutput, FormatConfigIssues(shown))
	}
	return issues
}

// levelRank 给级别排序 (数值越大越严重)
func levelRank(l IssueLevel) int {
	switch l {
	case IssueError:
		return 3
	case IssueWarn:
		return 2
	case IssueInfo:
		return 1
	}
	return 0
}

// LoadViperConfig 加载配置文件
// 兼容Windows路径格式
func LoadViperConfig(cfgPath string) error {
	var c = new(Configs)
	var _cfgDir string
	var _cfgFile string

	_cfgPath, pathErr := getConfigPath(cfgPath)
	if pathErr != nil {
		return fmt.Errorf("mysshw:: resolve config path: %w", pathErr)
	}
	CFG_PATH = _cfgPath
	_cfgDir, _cfgFile, _ = isCfgPath(_cfgPath)

	// 兼容Windows路径格式，将反斜杠替换为正斜杠
	_cfgDir = strings.ReplaceAll(_cfgDir, "\\", "/")

	// 检查文件名是否正确
	if !strings.HasSuffix(strings.ToLower(_cfgFile), "mysshw.toml") && !strings.HasSuffix(strings.ToLower(_cfgFile), "mysshw") {
		return fmt.Errorf("mysshw:: The configuration file must be named '~/.mysshw.toml', '~/mysshw.toml', './mysshw.toml' or custom path with 'mysshw' prefix")
	}

	viper.SetConfigName(_cfgFile)
	viper.AddConfigPath(_cfgDir)
	viper.AddConfigPath("$HOME/")
	viper.AddConfigPath("./")
	viper.SetConfigType(CFG_EXT_TYPE)

	// 读取配置文件
	err := viper.ReadInConfig()
	if err != nil {
		// Phase C: 加密文件检测
		// 尝试读原始字节, 如果是加密格式则提示 passphrase 解密
		rawBytes, readErr := os.ReadFile(_cfgPath)
		if readErr == nil && IsEncryptedFile(rawBytes) {
			// 加密文件 → 需 passphrase 解密
			return decryptAndLoadConfig(rawBytes)
		}
		// 先备份一份已经存在的配置文件, 如果有的话
		backupPath, err := backupConfigFile()
		if err != nil {
			return fmt.Errorf("mysshw:: Backup Config Error:: %s", err)
		}
		fmt.Fprintf(os.Stderr, "mysshw:: Backup Config Success:: %s\n", backupPath)

		// 验证配置文件 提示错误并创建一个默认的配置文件
		if err := ValidateConfigFile(_cfgPath); err != nil {
			// 如果配置文件不存在，则创建一个默认的配置文件
			//
			// 不使用 viper 的 WriteConfigAs 方法，而是使用 os.WriteFile,
			// 因为 WriteConfigAs 会自动重新解析配置丢失注释。
			// Viper v1 不支持保留注释 (v2 在讨论), 这里手动写文件保持原貌。

			if writeErr := writeConfigFile(_cfgPath); writeErr != nil {
				return fmt.Errorf("mysshw:: Write Config Error:: %s", err)
			}
			// 提示使用者配置文件不存在，但是已经生成了一个默认的配置文件
			fmt.Fprintf(os.Stderr, configReadInConfigPrintStr, cfgPath, _cfgPath, _cfgPath)
			fmt.Fprintln(os.Stderr, "\nsee https://github.com/cnphpbb/mysshw/blob/master/readme.md#config")
			fmt.Fprintf(os.Stderr, "mysshw:: Please check the backup config file:: %s\n", backupPath)
			return fmt.Errorf("mysshw:: Configuration file validation failed: %v", err)
		}
	}

	// 解析并验证配置
	if err := unmarshalAndValidateConfig(c); err != nil {
		return err
	}

	return nil
}

// decryptAndLoadConfig 提示 passphrase, 解密后用 viper 解析
// 用于 Phase C 的加密配置文件
func decryptAndLoadConfig(cipherdata []byte) error {
	passphrase, err := readPassphraseForDecrypt()
	if err != nil {
		return err
	}
	if passphrase == "" {
		return errors.New("mysshw:: empty passphrase, cannot decrypt config")
	}
	plaintext, err := DecryptConfig(cipherdata, []byte(passphrase))
	if err != nil {
		return fmt.Errorf("mysshw:: config decrypt failed: %w", err)
	}
	// 用解密后的 bytes 作为 viper 输入
	viper.SetConfigType(CFG_EXT_TYPE)
	if err := viper.ReadConfig(bytes.NewReader(plaintext)); err != nil {
		return fmt.Errorf("mysshw:: parse decrypted config: %w", err)
	}
	// 验证 + 设置 CFG
	c := new(Configs)
	if err := viper.Unmarshal(c); err != nil {
		return fmt.Errorf("mysshw:: unmarshal decrypted config: %w", err)
	}
	if err := ValidateConfig(c); err != nil {
		return fmt.Errorf("mysshw:: configuration validation failed: %v", err)
	}
	CFG = c
	// 关键配置项体检 (与普通配置加载路径保持一致)
	ReportConfigIssues(c, CFG_PATH)
	return nil
}
