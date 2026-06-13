package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// DefaultBackupKeep 写 TOML 时保留的本地备份数量 (用户可在 TOML 中 backup_keep 字段覆盖)
const DefaultBackupKeep = 5

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

// effectiveBackupKeep 根据全局 CFG 和默认值计算实际保留的备份数
// 规则: 0 或负数 → DefaultBackupKeep (5)
func effectiveBackupKeep() int {
	if CFG != nil && CFG.BackupKeep > 0 {
		return CFG.BackupKeep
	}
	return DefaultBackupKeep
}

// WriteTomlWithBackup 原子地写入 TOML: 先备份已存在文件 (再按 keep 数量裁剪),
// 然后写新内容。已存在文件不存在时直接写。
// 用途: 所有"修改已有 TOML"的场景 (yml 命令、未来的设置编辑等)
// 首次创建 (wizard) 不走此函数, 直接 writeSecureFile 即可。
func WriteTomlWithBackup(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		// 1. 备份现有文件 (原子重命名, 不占双倍空间)
		ts := time.Now().Format("20060102_150405")
		backupPath := path + "." + ts + ".bak"
		if err := os.Rename(path, backupPath); err != nil {
			return fmt.Errorf("failed to backup existing config: %v", err)
		}

		// 2. 裁剪旧备份 (按字母序 = 时间序, 保留最新 keep 个)
		if err := pruneBackups(path, effectiveBackupKeep()); err != nil {
			// 非致命: 仅打印警告
			fmt.Fprintf(os.Stderr, "mysshw:: warning: failed to prune old backups: %v\n", err)
		}
	}

	// 3. 写新文件 (0600)
	return writeSecureFile(path, data)
}

// pruneBackups 保留 path 最新 keep 个备份, 删除其余。
// 备份命名: <path>.<YYYYMMDD_HHMMSS>.bak
// keep <= 0 时直接返回 (不裁剪)
func pruneBackups(path string, keep int) error {
	if keep <= 0 {
		return nil
	}
	pattern := path + ".*.bak"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(matches) <= keep {
		return nil
	}
	// 字母序 = 时间序 (YYYYMMDD_HHMMSS 可字典序排序)
	sort.Strings(matches)
	// 删除最旧的 (前 len-keep 个)
	for _, m := range matches[:len(matches)-keep] {
		if err := os.Remove(m); err != nil {
			return err
		}
	}
	return nil
}

// writeConfigFile writes the configuration file to the specified path
// 首次生成 (TOML 损坏 / 不存在) 时使用, 不做备份 (无原文件可备)
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
