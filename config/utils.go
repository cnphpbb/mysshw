package config

import (
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
)

// isCfgPath checks if the given path is a valid configuration path
func isCfgPath(cfgPath string) (dir, file, ext string) {
	// 兼容Windows路径格式，将反斜杠替换为正斜杠
	cfgPath = strings.ReplaceAll(cfgPath, "\\", "/")

	_cfgDir, _cfgFile := path.Split(cfgPath)
	_cfgDir = path.Clean(_cfgDir)
	_cfgExt := filepath.Ext(cfgPath)

	if cfgPath != CFGPATH {
		if strings.HasSuffix(strings.ToLower(_cfgFile), ".toml") {
			_cfgFile = _cfgFile[:(len(_cfgFile) - len(_cfgExt))]
		}
	} else {
		// 获取实际的家目录路径
		u, err := user.Current()
		if err != nil {
			_cfgDir = "$HOME"
		} else {
			_cfgDir = u.HomeDir
		}
		_cfgFile = ".mysshw"
		_cfgExt = CFG_EXT_TYPE
	}

	// 确保目录路径以斜杠结尾
	if !strings.HasSuffix(_cfgDir, "/") {
		_cfgDir += "/"
	}

	return _cfgDir, _cfgFile, _cfgExt
}

// GetCfgPath 获取配置文件路径
func GetCfgPath(cfgPath string) (string, error) {
	return getConfigPath(cfgPath)
}

// getConfigPath 展开配置文件路径 (~ / $HOME / 相对路径 → 绝对路径)
// 纯函数: 不修改全局状态, 调用方需自行更新 CFG_PATH (如需)
func getConfigPath(cfgPath string) (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}

	// 兼容Windows路径格式，将反斜杠替换为正斜杠
	cfgPath = strings.ReplaceAll(cfgPath, "\\", "/")

	_cfgDir, _cfgFile := path.Split(cfgPath)

	// 处理家目录路径
	var _cfgPath string
	switch {
	case _cfgDir == "~/":
		_cfgPath = u.HomeDir
	case strings.HasPrefix(_cfgDir, "/"):
		// 绝对路径 (Unix)
		_cfgPath = _cfgDir
	case strings.Contains(_cfgDir, ":/"):
		// Windows 绝对路径 (如 D:/mydata/)
		_cfgPath = _cfgDir
	default:
		// 相对路径
		currentDir, _ := os.Getwd()
		_cfgPath = path.Join(currentDir, _cfgDir)
	}

	return path.Join(_cfgPath, _cfgFile), nil
}
