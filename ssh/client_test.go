package ssh

import (
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandHomeDir(t *testing.T) {
	// 获取当前用户信息用于测试
	currentUser, err := user.Current()
	assert.NoError(t, err)
	userHomeDir := currentUser.HomeDir

	// 测试用例
	testCases := []struct {
		name           string
		inputPath      string
		expectHomeDir  bool
		relativePath   string
		expectError    bool
	}{{
		name:           "不包含波浪号和$HOME的路径",
		inputPath:      "/absolute/path/to/file",
		expectHomeDir:  false,
		expectError:    false,
	}, {
		name:           "只有波浪号",
		inputPath:      "~",
		expectHomeDir:  true,
		expectError:    false,
	}, {
		name:           "波浪号后跟正斜杠",
		inputPath:      "~/.ssh/id_rsa",
		expectHomeDir:  true,
		relativePath:   ".ssh/id_rsa",
		expectError:    false,
	}, {
		name:           "波浪号后跟反斜杠",
		inputPath:      "~\\.ssh\\id_rsa",
		expectHomeDir:  true,
		relativePath:   ".ssh/id_rsa",
		expectError:    false,
	}, {
		name:           "波浪号后无分隔符",
		inputPath:      "~username/path",
		expectHomeDir:  false,
		expectError:    false,
	}, {
		name:           "只有$HOME",
		inputPath:      "$HOME",
		expectHomeDir:  true,
		expectError:    false,
	}, {
		name:           "$HOME后跟正斜杠",
		inputPath:      "$HOME/.ssh/id_rsa",
		expectHomeDir:  true,
		relativePath:   ".ssh/id_rsa",
		expectError:    false,
	}, {
		name:           "$HOME后跟反斜杠",
		inputPath:      "$HOME\\.ssh\\id_rsa",
		expectHomeDir:  true,
		relativePath:   ".ssh/id_rsa",
		expectError:    false,
	}, {
		name:           "$HOME后无分隔符",
		inputPath:      "$HOMEusername/path",
		expectHomeDir:  false,
		expectError:    false,
	}}

	// 运行测试用例
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resultPath, err := expandHomeDir(tc.inputPath)
			
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// 根据测试用例的期望检查结果
				if tc.expectHomeDir {
					// 应该以用户主目录开头
					assert.True(t, strings.HasPrefix(resultPath, userHomeDir), "结果路径应该以用户主目录开头")
					
					// 如果指定了相对路径，检查相对部分是否正确
					if tc.relativePath != "" {
						expectedFullPath := filepath.Join(userHomeDir, tc.relativePath)
						assert.Equal(t, expectedFullPath, resultPath, "路径解析结果不符合预期")
					}
				} else {
					// 不应该修改路径
					assert.Equal(t, tc.inputPath, resultPath, "非波浪号路径应该保持不变")
				}
			}
		})
	}
}