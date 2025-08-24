package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of mysshw",
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

// printVersion 打印版本信息
func printVersion() {
	fmt.Println(" mysshw - a free and open source ssh cli client soft.")
	fmt.Printf("    - Version:: %s\n", Version)
	fmt.Printf("    - GitVersion:: %s\n", Build)
	if BuildTime != "" {
		fmt.Printf("    - BuildTime :: %s\n", BuildTime)
	}
	fmt.Printf("    - Go Version :: %s\n", GoVersion)
}

// SetVersion 设置版本信息
func SetVersion(version, build, buildTime, goVersion string) {
	Version = version
	Build = build
	BuildTime = buildTime
	GoVersion = goVersion
}
