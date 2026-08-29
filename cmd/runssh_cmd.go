package cmd

import (
	"context"
	"fmt"
	"mysshw/config"
	"mysshw/ssh"
	"os"
	"os/signal"
	"os/user"
	"path"
	"runtime"
	"syscall"
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

	// 主循环: huh 选择节点 → SSH 登录 → 退到循环
	// 退出方式:
	//   - huh 表单内按 Esc / Ctrl+C: ssh.Choose 返回 nil → 主循环 break
	//   - SSH 会话结束后返回主菜单
	// 历史变更: 原代码另起一个 bufio.NewReader(os.Stdin) 与 huh 抢 stdin,
	//  	且 select 中 time.After + default 是死代码 (default 总是立即触发).
	//  	那套机制会造成 huh 读到脏数据, 已删除. huh 自带 Esc 退出足够。
	for {
		fmt.Print(GlobalScreenClearingStr)
		fmt.Println(fmtExitingDesc())
		node := ssh.Choose(config.CFG)
		if node == nil {
			// huh 表单被用户取消 (Esc / Ctrl+C)
			fmt.Println(RunSSHCtrlCResultStr)
			return
		}
		client := ssh.NewClient(node)

		// 传递会话结束回调函数，在SSH会话结束后返回主界面
		client.Login(func() {
			fmt.Println(RunSSHClientLoginSessionEndCallbackStr)
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

// fmtExitingDesc 兼容系统的退出描述
func fmtExitingDesc() string {
	if runtime.GOOS == "windows" {
		return GlobalExitingDescWindowsStr
	}
	return GlobalExitingDescUnixStr
}
