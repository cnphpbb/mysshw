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
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
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
	var err error
	CFG_PATH, err = getConfigPath(cfgPath)
	if err != nil {
		return nil, err
	}
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

	return nil
}

// LoadViperConfig 加载配置文件
// 兼容Windows路径格式
func LoadViperConfig(cfgPath string) error {
	var c = new(Configs)
	var _cfgDir string
	var _cfgFile string

	_cfgPath, _ := getConfigPath(cfgPath)
	_cfgDir, _cfgFile, _ = isCfgPath(_cfgPath)

	// 兼容Windows路径格式，将反斜杠替换为正斜杠
	_cfgDir = strings.ReplaceAll(_cfgDir, "\\", "/")

	// 检查文件名是否正确
	if !strings.HasSuffix(strings.ToLower(_cfgFile), "mysshw.toml") && !strings.HasSuffix(strings.ToLower(_cfgFile), "mysshw") {
		return fmt.Errorf("mysshw:: The configuration file must be named '~/.mysshw.toml', '~/mysshw.toml', './mysshw.toml' or custom path with 'mysshw' prefix")
	}

	viper.SetConfigName(_cfgFile)
	viper.AddConfigPath(_cfgDir)
	viper.SetConfigType(CFG_EXT_TYPE)

	// 读取配置文件
	err := viper.ReadInConfig()
	if err != nil {
		// 先备份一份已经存在的配置文件, 如果有的话
		if err := BackupConfig(); err != nil {
			return fmt.Errorf("mysshw:: Backup Config Error:: %s", err)
		}

		// 验证配置文件 提示错误并创建一个默认的配置文件
		if err := ValidateConfigFile(_cfgPath); err != nil {
			// 如果配置文件不存在，则创建一个默认的配置文件
			// 不使用viper的WriteConfigAs方法，而是使用os.WriteFile,是因为
			// WriteConfigAs方法会自动解析配置文件不使用注释，而我们希望保持原貌
			// 所以我们使用os.WriteFile方法来写入配置文件
			////
			// viper.ReadConfig(strings.NewReader(DefaultConfig))
			// viper.Set("cfg_dir", _cfgPath)
			// viper.WriteConfigAs(_cfgPath)
			if writeErr := writeConfigFile(_cfgPath); writeErr != nil {
				return fmt.Errorf("mysshw:: Write Config Error:: %s", err)
			}
			// 提示使用者配置文件不存在，但是已经生成了一个默认的配置文件
			fmt.Printf(configReadInConfigPrintStr, cfgPath, _cfgPath, _cfgPath)

			return fmt.Errorf("mysshw:: Configuration file validation failed: %v", err)
		}
	}

	// 解析并验证配置
	if err := unmarshalAndValidateConfig(c); err != nil {
		return err
	}

	return nil
}
