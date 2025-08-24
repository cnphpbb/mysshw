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

	err := viper.ReadInConfig()
	if err != nil {
		// 先备份一份已经存在的配置文件, 如果有的话
		BackupConfig()
		// 如果配置文件不存在，则创建一个默认的配置文件
		viper.ReadConfig(strings.NewReader(DefaultConfig))
		viper.Set("cfg_dir", _cfgPath)
		viper.WriteConfigAs(_cfgPath)

		return fmt.Errorf(`mysshw:: The configuration file '%s' was not detected,
and a default configuration file '%s' was generated.
vim %s -> Run mysshw again. 
see https://github.com/cnphpbb/mysshw/blob/master/readme.md#config`, cfgPath, _cfgPath, _cfgPath)
	}
	err = viper.Unmarshal(c)
	if err != nil {
		return fmt.Errorf("mysshw:: Failed to unmarshal configuration: %v", err)
	}
	CFG = c

	// 验证配置
	if err := ValidateConfig(CFG); err != nil {
		return fmt.Errorf("mysshw:: Configuration validation failed: %v", err)
	}

	return nil
}
