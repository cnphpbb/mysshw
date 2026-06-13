package config

import (
	"fmt"
	"os"
	"time"
)

// backupConfigFile creates a timestamped backup of the configuration file
func backupConfigFile() (string, error) {
	cfgPath, err := getConfigPath(CFG_PATH)
	if err != nil {
		return "", fmt.Errorf("failed to get config path: %v", err)
	}

	// Read original file
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %v", err)
	}

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupPath := cfgPath + "." + timestamp + ".bak"

	// Write backup file (0600: 备份可能含敏感信息, 仅所有者可读写)
	if err := writeSecureFile(backupPath, content); err != nil {
		return "", fmt.Errorf("failed to write backup file: %v", err)
	}

	return backupPath, nil
}

// writeConfigFile writes the configuration file to the specified path
func writeConfigFile(cfgPath string) error {
	// 0600: 配置文件可能含密码/密钥引用, 仅所有者可读写
	if err := writeSecureFile(cfgPath, []byte(DefaultTomlConfig)); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}
	return nil
}

// writeSecureFile 写入文件并强制设置 0600 权限 (仅所有者可读写)
// 用于配置文件和备份, 避免密码等敏感信息被同机其他用户读取
// Windows 上 Chmod 是 no-op, 权限由 NTFS ACL 控制
func writeSecureFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

// secureConfigFile 收紧已存在配置文件的权限到 0600
// 加载成功后调用, 用于自动修复旧版 0644 的配置
// 失败时仅打印警告, 不阻塞启动
func secureConfigFile(path string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}
	mode := info.Mode().Perm()
	if mode&0o077 == 0 {
		return // 已经是安全的 (group/other 无权限)
	}
	if err := os.Chmod(path, 0600); err != nil {
		fmt.Printf("mysshw:: warning: failed to chmod 0600 %s: %v\n", path, err)
		return
	}
	fmt.Printf("mysshw:: config file permissions tightened to 0600: %s\n", path)
}
