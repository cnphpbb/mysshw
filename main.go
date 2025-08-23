package main

import (
	"fmt"
	"os"
	"runtime"

	"mysshw/cmd"
	"mysshw/config"
	"mysshw/ssh"
)

var (
	Version   = "v21.10.06"
	Build     = "master"
	BuildTime string
	GoVersion string = runtime.Version()
)

func main() {
	// 设置版本信息
	cmd.SetVersion(Version, Build, BuildTime, GoVersion)
	// 使用 cobra 命令
	cmd.ExecuteCobra()
}

func RunSSH() {
	if err := config.LoadViperConfig(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	node := ssh.Choose(config.CFG)
	client := ssh.NewClient(node)
	client.Login()
}
