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
	"github.com/BurntSushi/toml"
)

type (
	Configs struct {
		CfgDir  string
		SyncCfg *SyncCfg
		Nodes *Nodes
	}

	SyncCfg struct {
		Type        string
		RemoteUri   string
		UserName    string
		Password    string
		KeyPath     string
		Passphrase  string
		RemotePath  string
		AccessToken string
	}
	Nodes struct {
		Groups   string
		SSHNodes []*SSHNode
	}
	SSHNode struct {
		Name       string
		Alias      string
		Host       string
		User       string
		Port       int
		KeyPath    string
		Passphrase string
		Password   string
	}
)

