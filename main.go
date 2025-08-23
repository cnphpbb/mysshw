package main

import (
	"runtime"

	"mysshw/cmd"
)

// Version information - These variables are parameters passed during Go build
// Go build need
var (
	Version   = "v21.10.06"                     // Application version:: "date +%y.%m.%d"
	Build     = "master"                        // Git branch name + commit id
	BuildTime string                            // Build timestamp
	GoVersion string        = runtime.Version() // Go version used for building
)

func main() {
	// 设置版本信息
	cmd.SetVersion(Version, Build, BuildTime, GoVersion)
	// 使用 cobra 命令
	cmd.ExecuteCobra()
}
