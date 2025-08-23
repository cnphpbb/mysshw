package main

import (
	"runtime"

	"mysshw/cmd"

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
