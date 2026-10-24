package ssh

import (
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"mysshw/config"

	"github.com/stretchr/testify/assert"
)

func TestExpandHomeDir(t *testing.T) {
	// 获取当前用户信息用于测试
	currentUser, err := user.Current()
	assert.NoError(t, err)
	userHomeDir := currentUser.HomeDir

	// 测试用例
	testCases := []struct {
		name          string
		inputPath     string
		expectHomeDir bool
		relativePath  string
		expectError   bool
	}{{
		name:          "不包含波浪号和$HOME的路径",
		inputPath:     "/absolute/path/to/file",
		expectHomeDir: false,
		expectError:   false,
	}, {
		name:          "只有波浪号",
		inputPath:     "~",
		expectHomeDir: true,
		expectError:   false,
	}, {
		name:          "波浪号后跟正斜杠",
		inputPath:     "~/.ssh/id_rsa",
		expectHomeDir: true,
		relativePath:  ".ssh/id_rsa",
		expectError:   false,
	}, {
		name:          "波浪号后跟反斜杠",
		inputPath:     "~\\.ssh\\id_rsa",
		expectHomeDir: true,
		relativePath:  ".ssh/id_rsa",
		expectError:   false,
	}, {
		name:          "波浪号后无分隔符",
		inputPath:     "~username/path",
		expectHomeDir: false,
		expectError:   false,
	}, {
		name:          "只有$HOME",
		inputPath:     "$HOME",
		expectHomeDir: true,
		expectError:   false,
	}, {
		name:          "$HOME后跟正斜杠",
		inputPath:     "$HOME/.ssh/id_rsa",
		expectHomeDir: true,
		relativePath:  ".ssh/id_rsa",
		expectError:   false,
	}, {
		name:          "$HOME后跟反斜杠",
		inputPath:     "$HOME\\.ssh\\id_rsa",
		expectHomeDir: true,
		relativePath:  ".ssh/id_rsa",
		expectError:   false,
	}, {
		name:          "$HOME后无分隔符",
		inputPath:     "$HOMEusername/path",
		expectHomeDir: false,
		expectError:   false,
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

// TestGenSSHConfig_NoStdoutPollution 锁定 MCP stdio 断连回归:
// genSSHConfig 被 exec/copy (NewClientNonInteractive) 复用, 任何写 os.Stdout 的
// 诊断输出都会污染 MCP JSON-RPC 流导致 AI 客户端解析失败 → 断开。
// 历史 bug: ${ENV} 密码节点每次连接都写一行 "password resolved from env var"
// 到 stdout, 只在 env 节点 / 私钥读失败时触发, 故表现为"偶发断开"。
// 修复: 所有诊断输出改走 os.Stderr。
func TestGenSSHConfig_NoStdoutPollution(t *testing.T) {
	cases := []struct {
		name string
		node *config.SSHNode
		env  string // ${ENV} 引用名 (env-password 用)
	}{
		{
			name: "env-password-resolved",
			node: &config.SSHNode{Name: "n", Host: "127.0.0.1", User: "u", Password: "${MYSSHW_REGRESS_PWD}"},
			env:  "MYSSHW_REGRESS_PWD",
		},
		{
			name: "unreadable-keypath",
			node: &config.SSHNode{Name: "n", Host: "127.0.0.1", User: "u", KeyPath: "/nonexistent/regression-key"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.env != "" {
				t.Setenv(c.env, "secret")
			}
			// 跳过 known_hosts 副作用 (本测试只关心 stdout, 不验证 host key 行为)
			t.Setenv("MYSSHW_SKIP_HOST_KEY_CHECK", "true")

			orig := os.Stdout
			r, w, err := os.Pipe()
			assert.NoError(t, err)
			os.Stdout = w
			func() {
				defer func() { os.Stdout = orig }()
				// allowStdin=false = exec/copy 路径; 不会 prompt / 读 stdin
				genSSHConfig(c.node, false)
			}()
			_ = w.Close()
			out, _ := io.ReadAll(r)
			_ = r.Close()

			assert.Empty(t, string(out),
				"genSSHConfig 不应写 os.Stdout (MCP stdio 下会污染 JSON-RPC 流导致断开); got: %q", string(out))
		})
	}
}
