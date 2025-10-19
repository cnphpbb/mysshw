package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/GuanceCloud/toml"
)

// ValidateConfig 验证配置的有效性
func ValidateConfig(cfg *Configs) error {
	// 验证同步配置
	if err := validateSyncConfig(&cfg.SyncCfg); err != nil {
		return err
	}

	// 验证节点配置
	if len(cfg.Nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}

	for i, nodeGroup := range cfg.Nodes {
		if err := validateNodeGroup(nodeGroup, i); err != nil {
			return err
		}
	}
	return nil
}

// validateSyncConfig 验证同步配置
func validateSyncConfig(sync *SyncInfo) error {
	// 验证同步类型
	supportedTypes := map[string]bool{
		"scp":    true,
		"webdav": true,
		"s3":     true,
	}

	if sync.Type != "" && !supportedTypes[strings.ToLower(sync.Type)] {
		return fmt.Errorf("unsupported sync type: %s. Supported types: scp, webdav, s3", sync.Type)
	}

	// 根据同步类型验证必要字段
	if strings.ToLower(sync.Type) == "scp" {
		if sync.RemoteUri == "" {
			return fmt.Errorf("remote_uri is required for scp sync type")
		}
		if sync.RemotePath == "" {
			return fmt.Errorf("remote_path is required for scp sync type")
		}
		// SCP需要至少一种认证方式
		if sync.SCPConfig.Username == "" && sync.SCPConfig.Password == "" && sync.SCPConfig.KeyPath == "" {
			fmt.Println("The configuration file has changed, please modify it to the new mode.")
			fmt.Println("see: https://github.com/cnphpbb/mysshw/blob/main/example/mysshw.toml")
			return fmt.Errorf("either password, username or keyPath is required for scp sync type")
		}
	} else if strings.ToLower(sync.Type) == "s3" {
		if sync.S3Config.AccessKey == "" {
			return fmt.Errorf("access_key is required for s3 sync type")
		}
		if sync.S3Config.SecretKey == "" {
			return fmt.Errorf("secret_key is required for s3 sync type")
		}
		if sync.S3Config.BucketName == "" {
			return fmt.Errorf("bucket_name is required for s3 sync type")
		}
		if sync.RemotePath == "" {
			return fmt.Errorf("remote_path is required for s3 sync type")
		}
		// 如果endpoint为空，则使用remote_uri作为endpoint
		if sync.S3Config.Endpoint == "" && sync.RemoteUri == "" {
			return fmt.Errorf("either endpoint or remote_uri is required for s3 sync type")
		}
	} else if strings.ToLower(sync.Type) == "webdav" {
		if sync.WebDAVConfig.Username == "" && sync.WebDAVConfig.Password == "" {
			return fmt.Errorf("either username or password is required for webdav sync type")
		}
	}

	return nil
}

// validateNodeGroup 验证节点组配置
func validateNodeGroup(group Nodes, index int) error {
	if group.Groups == "" {
		return fmt.Errorf("group at index %d has empty group name", index)
	}

	if len(group.SSHNodes) == 0 {
		return fmt.Errorf("group '%s' has no SSH nodes configured", group.Groups)
	}

	for i, sshNode := range group.SSHNodes {
		if err := validateSSHNode(sshNode, group.Groups, i); err != nil {
			return err
		}
	}

	return nil
}

// validateSSHNode 验证SSH节点配置
func validateSSHNode(node *SSHNode, group string, index int) error {
	if node.Name == "" {
		return fmt.Errorf("SSH node at index %d in group '%s' has no name", index, group)
	}

	if node.Host == "" {
		return fmt.Errorf("SSH node '%s' in group '%s' has no host", node.Name, group)
	}

	// 验证端口范围
	if node.Port < 0 || node.Port > 65535 {
		return fmt.Errorf("SSH node '%s' in group '%s' has invalid port: %d. Must be between 1 and 65535", node.Name, group, node.Port)
	}

	// // 确保至少有一种认证方式
	// if node.Password == "" && node.KeyPath == "" {
	// 	return fmt.Errorf("SSH node '%s' in group '%s' has no authentication method (password or keyPath)", node.Name, group)
	// }

	// 如果提供了密钥路径，检查是否存在
	if node.KeyPath != "" {
		// 处理路径格式，兼容Windows
		node.KeyPath = strings.ReplaceAll(node.KeyPath, "\\", "/")
		if _, err := os.Stat(node.KeyPath); os.IsNotExist(err) {
			return fmt.Errorf("SSH node '%s' in group '%s' key file not found: %s", node.Name, group, node.KeyPath)
		}
	}

	return nil
}

// ValidateConfigFile 验证配置文件TOML格式是否有错
func ValidateConfigFile(cfgPath string) error {
	var config interface{}
	_, err := toml.DecodeFile(cfgPath, &config)
	if err != nil {
		return fmt.Errorf("TOML parsing error: %v", err)
	}
	return nil
}
