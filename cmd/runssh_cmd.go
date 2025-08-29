package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"mysshw/config"
	"mysshw/ssh"
	"os"
	"os/signal"
	"os/user"
	"path"
	"syscall"
	"time"
)

// RunSSH 执行 SSH 登录
// 修复初始化循环问题，将 context 作为参数传入
func RunSSH(ctx context.Context) {
	cfgPath := GetCtxConfigPath(ctx)
	fmt.Println("mysshw:: Config path changed to:", cfgPath)
	if err := config.LoadViperConfig(cfgPath); err != nil {
		fmt.Println("mysshw:: Load Config Error::", err)
		os.Exit(1)
	}

	// 设置信号处理捕获Ctrl+C和SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println(RunSSHCtrlCResultStr)
		os.Exit(0)
	}()

	// 创建一个读取器用于检测键盘输入
	reader := bufio.NewReader(os.Stdin)

	for {
		// huh 中 Ctrl+d & /{string} 被占用了
		fmt.Print(GlobalScreenClearingStr)
		fmt.Println(GlobalExitingDescStr)
		node := ssh.Choose(config.CFG)
		client := ssh.NewClient(node)
		// 检查是否按下q键
		select {
		case <-time.After(100 * time.Millisecond):
			// 没有按键输入，继续执行
		default:
			char, _, err := reader.ReadRune()
			if err != nil {
				// 检测到Ctrl+D (EOF)
				if err == io.EOF {
					fmt.Println(RunSSHCtrlDResultStr)
					os.Exit(0)
				}
				continue
			}
			if char == 'q' || char == 'Q' {
				fmt.Printf(RunSSHInputQResultStr, string(char))
				os.Exit(0)
			}
		}

		// 传递会话结束回调函数，在SSH会话结束后返回主界面
		client.Login(func() {
			fmt.Println(RunSSHClientLoginSessionEndCallbackStr)
			// 清屏
			fmt.Print(GlobalScreenClearingStr)
		})
		// 会话结束后继续循环，重新显示菜单
	}
}

// 从 context 中获取配置路径，如果不存在则返回默认路径
func GetCtxConfigPath(ctx context.Context) string {
	if path, ok := ctx.Value(cfgKey).(string); ok {
		return path
	}
	// 默认配置路径: $HOME/.mysshw.toml
	usr, err := user.Current()
	if err != nil {
		return config.CFGPATH
	}
	return path.Join(usr.HomeDir, ".mysshw.toml")
}
