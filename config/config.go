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
	"strings"

	"github.com/BurntSushi/toml"
)

type (
	Configs struct {
		CfgDir  string   `toml: "cfg_dir"`
		SyncCfg *SyncCfg `toml: "sysc"`
		Nodes   *Nodes   `toml: "nodes"`
	}

	SyncCfg struct {
		Type        string `toml: "type"`
		RemoteUri   string `toml: "remote_uri"`
		UserName    string `toml: "username"`
		Password    string `toml: "password"`
		KeyPath     string `toml: "keyPath"`
		Passphrase  string `toml: "passphrase"`
		RemotePath  string `toml: "remote_path"`
		AccessToken string `toml: "access_token"`
		GistID      string `tome: "gist_id"`
	}
	Nodes struct {
		Groups   string	`tome:"groups"`
		SSHNodes []*SSHNode `toml:"ssh"`
	}
	SSHNode struct {
		Name       string `toml:"name"`
		Alias      string `toml:"alias"`
		Host       string `toml:"host"`
		User       string `toml:"user"`
		Port       int		`toml:"port"`
		KeyPath    string	`toml:"keypath"`
		Passphrase string	`toml:"passphrase"`
		Password   string `toml:"password"`
	}
)

var (
	CFG_PATH string = "~/.sshw.toml"
	//APP_VER  string = "2.5.6.1211"
	//LOG_PATH  = "mysshw.log"
	CFG *Configs
)


func LoadConfigBytes(sshwpath string) ([]byte, error) {
	var cfgPath string
	u,err := user.Current()
	if err != nil {
		return  nil, err
	}
	_cfgs := strings.SplitAfter(sshwpath, "/")
	if _cfgs[0] == "~" {
		cfgPath = u.HomeDir
	} else {
		cfgPath = _cfgs[0]
	}
	if _cfgs[1] != ".sshw.toml" {
		CFG_PATH = path.Join(cfgPath, _cfgs[1])
	} else {
		CFG_PATH = path.Join(cfgPath, ".sshw.toml")
	}
	cfgBytes,err := ioutil.ReadFile(CFG_PATH)
	if err == nil {
		return cfgBytes, nil
	}
	return nil, err
}

func LoadConfig() error {
	cfgBytes, err := LoadConfigBytes(CFG_PATH)
	if err != nil {
		return err
	}
	fmt.Println(string(cfgBytes))
	var c *Configs
	err = toml.Unmarshal(cfgBytes, &c)
	if err != nil {
		return err
	}
	CFG = c
	return nil
}