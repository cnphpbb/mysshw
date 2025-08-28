package cmd

import (
	"fmt"
	"os"
	"strings"

	"mysshw/config"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// YMLCmd 提供了从旧版SSHW的YAML配置文件迁移到新版mysshw的TOML配置文件的功能
var YMLCmd = &cobra.Command{
	Use:   "yml",
	Short: "Migrate from old SSHW YAML config to mysshw TOML config",
	Long: `Parse and migrate content from the .sshw.yml file of the original SSHW project to the .toml config file of the mysshw project.

Example usage:
  mysshw yml -f ~/.sshw.yml

This command will read the specified YAML config file, convert it to TOML format, and append it to the current mysshw config file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 处理配置文件路径
		cfgPath, _ := cmd.Flags().GetString("cfg")
		if cfgPath != "" {
			config.CFG_PATH = cfgPath
		}

		// 读取YAML文件
		ymlContent, err := os.ReadFile(ymlFilePath)
		if err != nil {
			return fmt.Errorf("无法读取YAML文件: %v", err)
		}

		// 解析YAML内容
		var ymlGroups []YMLServerGroup
		if err := yaml.Unmarshal(ymlContent, &ymlGroups); err != nil {
			return fmt.Errorf("YAML解析错误: %v", err)
		}

		// 转换为TOML配置
		result, err := convertYAMLToTOML(ymlGroups)
		if err != nil {
			return fmt.Errorf("转换失败: %v", err)
		}

		// 追加到配置文件
		if err := appendToConfigFile(result); err != nil {
			return fmt.Errorf("追加到配置文件失败: %v", err)
		}

		// 提示用户
		fmt.Println("配置文件已成功更新!")
		fmt.Println("请重启mysshw服务以应用新的配置。")

		return nil
	},
}

var ymlFilePath string

func init() {
	// 配置命令行参数
	YMLCmd.Flags().StringVarP(&ymlFilePath, "file", "f", "", "旧版SSHW的YAML配置文件路径")
	YMLCmd.MarkFlagRequired("file") // 将文件路径参数标记为必需
}

// YMLServerGroup 表示从YAML文件中解析的服务器组结构
type YMLServerGroup struct {
	Name     string      `yaml:"name"`
	Children []YMLServer `yaml:"children"`
}

// YMLServer 表示从YAML文件中解析的服务器信息
type YMLServer struct {
	Name       string `yaml:"name"`
	Alias      string `yaml:"alias,omitempty"`
	Host       string `yaml:"host"`
	User       string `yaml:"user,omitempty"`
	Port       int    `yaml:"port,omitempty"`
	KeyPath    string `yaml:"keypath,omitempty"`
	Passphrase string `yaml:"passphrase,omitempty"`
	Password   string `yaml:"password,omitempty"`
}

// convertYAMLToTOML 将YAML配置转换为TOML格式
func convertYAMLToTOML(ymlGroups []YMLServerGroup) (string, error) {
	var result string

	// 遍历每个服务器组
	for _, group := range ymlGroups {
		// 添加组注释
		result += fmt.Sprintf("# %s\n", group.Name)
		// 添加组配置
		result += fmt.Sprintf("[[nodes]]\ngroups = \"%s\"\n\n", group.Name)

		// 遍历组内的每个服务器
		for i, server := range group.Children {
			// 处理第一个服务器
			if i == 0 {
				result += "[[nodes.ssh]]\n"
			} else {
				result += "[[nodes.ssh]]\n"
			}

			// 添加服务器配置
			result += fmt.Sprintf("alias = '%s'\n", escapeTomlString(server.Alias))
			result += fmt.Sprintf("host = '%s'\n", escapeTomlString(server.Host))
			result += fmt.Sprintf("name = '%s'\n", escapeTomlString(server.Name))

			// 添加可选字段
			if server.User != "" {
				result += fmt.Sprintf("user = '%s'\n", escapeTomlString(server.User))
			}

			if server.Port > 0 {
				result += fmt.Sprintf("port = %d\n", server.Port)
			} else if server.Port == 0 {
				// 默认端口22
				result += "port = 22\n"
			}

			if server.Password != "" {
				result += fmt.Sprintf("password = '%s'\n", escapeTomlString(server.Password))
			}

			if server.KeyPath != "" {
				result += fmt.Sprintf("keypath = '%s'\n", escapeTomlString(server.KeyPath))
			}

			if server.Passphrase != "" {
				result += fmt.Sprintf("passphrase = '%s'\n", escapeTomlString(server.Passphrase))
			}

			result += "\n"
		}

		result += "\n"
	}

	return result, nil
}

// escapeTomlString 转义TOML字符串中的特殊字符
func escapeTomlString(s string) string {
	// 替换单引号和反斜杠等特殊字符
	result := s
	result = strings.ReplaceAll(result, "\\", "\\\\")
	result = strings.ReplaceAll(result, "'", "\\'")
	return result
}

// appendToConfigFile 将内容追加到配置文件
func appendToConfigFile(content string) error {
	// 获取完整的配置文件路径
	cfgPath, err := config.GetCfgPath(config.CFG_PATH)
	if err != nil {
		return err
	}

	// 检查文件是否存在
	fileInfo, err := os.Stat(cfgPath)
	fileExists := err == nil && !fileInfo.IsDir()

	// 以追加模式打开文件
	file, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// 如果文件已存在，在追加新内容前添加空行
	if fileExists {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}

	// 写入转换后的TOML内容
	if _, err := file.WriteString(content); err != nil {
		return err
	}

	return nil
}

// 配置文件示例:
// # server group 1
// - name: server group 1
//   children:
//   - { name: server 1, user: root, host: 192.168.1.2 }
//   - { name: server 2, user: root, host: 192.168.1.3 }
//   - { name: server 3, user: root, host: 192.168.1.4 }

// # server group 2
// - name: server group 2
//   children:
//   - { name: server 1, user: root, host: 192.168.2.2 }
//   - { name: server 2, user: root, host: 192.168.3.3 }
//   - { name: dev server fully configured, user: appuser, host: 192.168.8.35, port: 22, password: 123456 }
//   - { name: dev server with key path, user: appuser, host: 192.168.8.35, port: 22, keypath: /root/.ssh/id_rsa }
//   - { name: dev server with passphrase key, user: appuser, host: 192.168.8.35, port: 22, keypath: /root/.ssh/id_rsa, passphrase: abcdefghijklmn}
//   - { name: dev server without port, user: appuser, host: 192.168.8.35 }
