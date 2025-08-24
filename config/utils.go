package config

import (
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
)

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

// getConfigPath 获取配置文件路径
// 兼容Windows路径格式
func getConfigPath(cfgPath string) (string, error) {
	var _cfgPath string
	u, err := user.Current()
	if err != nil {
		return "", err
	}

	// 兼容Windows路径格式，将反斜杠替换为正斜杠
	cfgPath = strings.ReplaceAll(cfgPath, "\\", "/")

	_cfgDir, _cfgFile := path.Split(cfgPath)

	// 处理家目录路径
	if _cfgDir == "~/" {
		_cfgPath = u.HomeDir
	} else if strings.HasPrefix(_cfgDir, "/") {
		// 绝对路径
		_cfgPath = _cfgDir
	} else if strings.Contains(_cfgDir, ":/") {
		// Windows绝对路径 (如 D:/mydata/)
		_cfgPath = _cfgDir
	} else {
		// 相对路径
		currentDir, _ := os.Getwd()
		_cfgPath = path.Join(currentDir, _cfgDir)
	}

	CFG_PATH = path.Join(_cfgPath, _cfgFile)
	return CFG_PATH, err
}
