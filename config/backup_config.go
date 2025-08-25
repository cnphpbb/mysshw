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

	// Write backup file
	err = os.WriteFile(backupPath, content, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write backup file: %v", err)
	}

	return backupPath, nil
}

func writeConfigFile(cfgPath string) error {
	// Write backup file
	err := os.WriteFile(cfgPath, []byte(DefaultTomlConfig), 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}
	return nil
}
