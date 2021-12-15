/**
 * @Author   DenysGeng <cnphp@hotmail.com>
 *
 * @Description: 采用toml做为新版本的配置文件
 * @Version: 	1.0.0
 * @Date:     	2021/9/23
 */

package config

import (
	"fmt"
	"io/ioutil"
	"os/user"
	"path"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"
)

type (
	Configs struct {
		CfgDir  string   `toml:"cfg_dir" mapstructure:"cfg_dir"`
		SyncCfg SyncInfo `toml:"sync" mapstructure:"sync"`
		Nodes   []Nodes  `toml:"nodes" mapstructure:"nodes"`
	}

	SyncInfo struct {
		Type        string `toml:"type" mapstructure:"type"`
		RemoteUri   string `toml:"remote_uri" mapstructure:"remote_uri"`
		UserName    string `toml:"username" mapstructure:"username"`
		Password    string `toml:"password" mapstructure:"password"`
		KeyPath     string `toml:"keyPath" mapstructure:"keyPath"`
		Passphrase  string `toml:"passphrase" mapstructure:"passphrase"`
		RemotePath  string `toml:"remote_path" mapstructure:"remote_path"`
		AccessToken string `toml:"access_token" mapstructure:"access_token"`
		GistID      string `tome:"gist_id" mapstructure:"gist_id"`
	}
	Nodes struct {
		Groups   string     `toml:"groups"`
		SSHNodes []*SSHNode `toml:"ssh" mapstructure:"ssh"`
	}
	SSHNode struct {
		Name       string `toml:"name" mapstructure:"name"`
		Alias      string `toml:"alias,omitempty" mapstructure:"alias"`
		Host       string `toml:"host" mapstructure:"host"`
		User       string `toml:"user,omitempty" mapstructure:"user"`
		Port       int    `toml:"port,omitempty" mapstructure:"port"`
		KeyPath    string `toml:"keypath,omitempty" mapstructure:"keypath"`
		Passphrase string `toml:"passphrase,omitempty" mapstructure:"passphrase"`
		Password   string `toml:"password,omitempty" mapstructure:"password"`
	}
)

//type AutoGenerated struct {
//	CfgDir string `toml:"cfg_dir"`
//	Sync   struct {
//		Type        string `toml:"type"`
//		RemoteURI   string `toml:"remote_uri"`
//		Username    string `toml:"username"`
//		Password    string `toml:"password"`
//		KeyPath     string `toml:"keyPath"`
//		Passphrase  string `toml:"passphrase"`
//		RemotePath  string `toml:"remote_path"`
//		AccessToken string `toml:"access_token"`
//		GistID      string `toml:"gist_id"`
//	} `toml:"sync"`
//	Nodes []struct {
//		Groups string `toml:"groups"`
//		SSH    []struct {
//			Name       string `toml:"name"`
//			User       string `toml:"user,omitempty"`
//			Host       string `toml:"host"`
//			Port       int    `toml:"port,omitempty"`
//			Keypath    string `toml:"keypath,omitempty"`
//			Passphrase string `toml:"passphrase,omitempty"`
//			Password   string `toml:"password,omitempty"`
//			Alias      string `toml:"alias,omitempty"`
//		} `toml:"ssh"`
//	} `toml:"nodes"`
//}

var (
	CFG_PATH string = "~/.mysshw.toml"
	CFG_EXT_TYPE string = "toml"
	//LOG_PATH  = "mysshw.log"
	CFG *Configs
)

func (n *SSHNode) SetUser() string {
	if n.User == "" {
		return "root"
	}
	return n.User
}

func (n *SSHNode) SetPort() int {
	if n.Port <= 0 {
		return 22
	}
	return n.Port
}

func (n *SSHNode) SetKeyPath() string {
	if n.KeyPath == "" {
		return ""
	}
	return n.KeyPath
}

func (n *SSHNode) SetPassword() ssh.AuthMethod {
	if n.Password == "" {
		return nil
	}
	return ssh.Password(n.Password)
}

func LoadConfigBytes(cfgPath string) ([]byte, error) {
	var err error
	CFG_PATH, err = GetCfgPath(cfgPath)
	if err != nil {
		return nil, err
	}
	cfgBytes, err1 := ioutil.ReadFile(CFG_PATH)
	if err1 == nil {
		return cfgBytes, nil
	}
	return nil, err
}

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

func GetCfgPath(cfgPath string) (string, error) {
	var _cfgPath string
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	_cfgDir, _cfgFile := path.Split(cfgPath)
	if _cfgDir == "~/" {
		_cfgPath = u.HomeDir
	} else {
		_cfgPath = _cfgDir
	}
	CFG_PATH = path.Join(_cfgPath, _cfgFile)
	return CFG_PATH, err
}

func LoadViperConfig() error {
	var c = new(Configs)
	_cfgDir, _cfgFile, _ := isCfgPath(CFG_PATH)
	if strings.HasSuffix(_cfgFile, "mysshw") {
		return  fmt.Errorf("mysshw:: The configuration file '~/.mysshw.toml' || '~/mysshw.toml' || './mysshw.toml'")
	}
	viper.SetConfigName(_cfgFile)
	viper.AddConfigPath(_cfgDir)
	viper.SetConfigType(CFG_EXT_TYPE)

	err := viper.ReadInConfig()
	if err != nil {
		//log.Println(CFG_PATH)
		GetCfgPath(CFG_PATH)
		viper.SetDefault("cfg_dir", "~/.mysshw.toml")
		viper.WriteConfigAs(CFG_PATH)

		return fmt.Errorf("mysshw:: The configuration file '~/.mysshw.toml' was not detected, \n" +
			"  and a default configuration file '~/.myshw.toml' was generated. \n" +
			"  vim ~/.mysshw.toml -> Run mysshw again. \n" +
			"  see https://github.com/cnphpbb/mysshw/blob/master/readme.md#config \n")
	}
	err = viper.Unmarshal(c)
	CFG = c
	return err
}

func isCfgPath(cfgPath string) (dir, file, ext string) {
	_cfgDir, _cfgFile := path.Split(cfgPath)
	_cfgDir = path.Dir(_cfgDir)
	_cfgExt := filepath.Ext(cfgPath)
	if cfgPath != CFG_PATH {
		// _cfgFile ".myConfig.toml"
		if strings.HasPrefix(_cfgFile, ".") {
			if strings.HasSuffix(_cfgFile, ".toml") {
				_cfgFile = _cfgFile[:len(_cfgFile)-len(_cfgExt)]
			}
		} else { 	// _cfgFile "myConfig.toml"
			if strings.HasSuffix(_cfgFile, ".toml") {
				_cfgFile = _cfgFile[:(len(_cfgFile)-len(_cfgExt))]
			}
		}
	} else {
		_cfgFile = ".mysshw"
		_cfgExt = CFG_EXT_TYPE
		_cfgDir = "$HOME"
	}


	return _cfgDir, _cfgFile, _cfgExt
}

//todo:
//func WriteViperConfig() error {}


