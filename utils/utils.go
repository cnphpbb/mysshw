package utils

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func cleanup() {
	fmt.Println("\nCaught exit signal, exiting program...")
	fmt.Println("\n")
}

// setupSignalHandler 设置信号处理器以捕获退出信号
func SetupSignalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		originalState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			term.Restore(int(os.Stdin.Fd()), originalState)
		}
		cleanup()
		exec.Command("stty", "echo").Run()
		exec.Command("reset").Run()
		os.Exit(0)
	}()
}
