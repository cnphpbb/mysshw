//go:build mage
// +build mage

package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Default target to run when none is specified
// If not set, running mage will list available targets
// var Default = Build

// tidy code
//func Fmt() error {
//	packages := strings.Split("cmd", " ")
//	files, _ := filepath.Glob("*.go")
//	packages = append(packages, files...)
//	return sh.Run("gofmt", append([]string{"-s", "-l", "-w"}, packages...)...)
//}

// for local machine build
func Build() error {
	printPlatformWarning()
	return buildTarget(runtime.GOOS, runtime.GOARCH, nil, false)
}

// build all platform
func Pack() error {
	printPlatformWarning()
	buildTarget("darwin", "amd64", nil, true)
	buildTarget("darwin", "arm64", nil, true)
	buildTarget("freebsd", "amd64", nil, true)
	buildTarget("freebsd", "arm64", nil, true)
	buildTarget("linux", "amd64", nil, true)
	buildTarget("linux", "arm64", nil, true)
	buildTarget("windows", "amd64", nil, true)
	buildTarget("windows", "arm64", nil, true)
	return genCheckSum()
}

// Test all packages
func Test() error {
	err := sh.RunV("go", "test", "-v", "-coverprofile", ".cover.out", "./...")
	if err != nil {
		return err
	}

	err = sh.RunV("go", "tool", "cover", "-func=.cover.out")
	if err != nil {
		return err
	}

	err = sh.Run("go", "tool", "cover", "-html=.cover.out", "-o", ".cover.html")
	if err != nil {
		return err
	}

	return sh.Rm(".cover.out")
}

// build to target (cross build)
// createTar is an optional parameter (default: false) that controls whether to create a tar archive
func buildTarget(OS, arch string, envs map[string]string, createTar bool) error {
	tag := tag()
	name := fmt.Sprintf("mysshw-%s-%s-%s", OS, arch, tag)
	dir := fmt.Sprintf("dist/%s", name)
	target_unix := fmt.Sprintf("%s/mysshw", dir)
	target_win := fmt.Sprintf("%s/mysshw.exe", dir)
	target := ""
	if OS == "windows" {
		target = target_win
	} else {
		target = target_unix
	}

	// Determine if we should create tar archive (default: false)
	shouldCreateTar := false
	if createTar == true {
		shouldCreateTar = createTar
	}

	args := make([]string, 0, 10)
	args = append(args, "build", "-o", target)
	args = append(args, "-ldflags", flags(), "main.go")

	fmt.Println("args: ", args)
	fmt.Println("build", target)
	env := make(map[string]string)
	env["GOOS"] = OS
	env["GOARCH"] = arch
	env["CGO_ENABLED"] = "0"

	if envs != nil {
		for k, v := range envs {
			env[k] = v
		}
	}

	if err := sh.RunWith(env, mg.GoCmd(), args...); err != nil {
		return err
	}
	// 根据操作系统选择合适的复制命令
	if runtime.GOOS == "windows" {
		// Windows系统使用copy命令
		sh.Run("copy", "/Y", "example\\mysshw.toml", fmt.Sprintf("%s\\mysshw.toml", dir))
	} else {
		// 非Windows系统使用cp命令
		sh.Run("cp", "-a", "example/mysshw.toml", fmt.Sprintf("%s/mysshw.toml", dir))
	}

	// Only create archive if requested
	if shouldCreateTar == true {
		if runtime.GOOS == "windows" {
			// Windows系统使用PowerShell的Compress-Archive命令
			// 使用单引号包裹路径，避免复杂的转义问题
			sh.Run("powershell", "-Command", fmt.Sprintf("Compress-Archive -Path '%s\\*' -DestinationPath '%s.zip'", dir, dir))
		} else {
			// 非Windows系统使用tar命令
			sh.Run("tar", "-czf", fmt.Sprintf("%s.tar.gz", dir), "-C", "dist", name)
		}
	}

	return nil
}

func flags() string {
	hash := hash()
	tag := tag()
	gitBranchCommit := fmt.Sprintf("%s-%s", tag, hash)
	buildTime := buildTime()
	verStr := versionStr()
	return fmt.Sprintf(`-s -w -X "main.Version=%s" -X "main.Build=%s" -X "main.BuildTime=%s" -extldflags "-static"`, verStr, gitBranchCommit, buildTime)
}

// tag returns the git tag for the current branch or "" if none.
func tag() string {
	s, _ := sh.Output("git", "branch", "--show-current")
	// fmt.Println("tag: ", s)
	// if s == "" {
	// 	return "main"
	// }
	return s
}

// hash returns the git hash for the current repo or "" if none.
func hash() string {
	hash, _ := sh.Output("git", "rev-parse", "--short", "HEAD")
	return hash
}

func mod() string {
	f, err := os.Open("go.mod")
	if err == nil {
		reader := bufio.NewReader(f)
		line, _, _ := reader.ReadLine()
		return strings.Replace(string(line), "module ", "", 1)
	}
	return ""
}

// cleanup all build files
func Clean() {
	printPlatformWarning()
	sh.Rm("dist")
	sh.Rm(".cover.html")
}

func genCheckSum() error {
	fmt.Println("generate checksum.txt file")
	fs, err := ioutil.ReadDir("dist")
	if err != nil {
		return err
	}

	file, err := os.OpenFile("dist/checksum.txt", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	// 根据操作系统类型决定要处理的文件类型
	isWindows := runtime.GOOS == "windows"

	for _, f := range fs {
		if f.IsDir() {
			continue
		}

		// Windows平台处理.zip文件，其他平台处理.tar.gz文件
		isTargetFile := false
		if isWindows {
			isTargetFile = strings.HasSuffix(f.Name(), ".zip")
		} else {
			isTargetFile = strings.HasSuffix(f.Name(), ".tar.gz")
		}

		if isTargetFile {
			sum, _ := fileHash(fmt.Sprintf("dist/%s", f.Name()))
			fmt.Println(sum, f.Name())
			file.WriteString(fmt.Sprintf("%s  %s\n", sum, f.Name()))
		}
	}
	return nil
}

func fileHash(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}

	hash := sha1.New()
	io.Copy(hash, file)
	ret := hash.Sum(nil)
	return hex.EncodeToString(ret[:]), nil
}

func buildTime() string {
	// 使用time包获取当前时间，避免调用外部命令
	now := time.Now()
	// 所有平台使用统一格式
	return now.Format("2006-01-02 15:04:05")
}

func versionStr() string {
	// 使用time包获取当前时间，避免调用外部命令
	now := time.Now()
	// 所有平台使用统一格式
	return "v" + now.Format("06.01.02")
}

// printPlatformWarning 打印平台兼容性警告信息
func printPlatformWarning() {
	if runtime.GOOS == "windows" {
		fmt.Println("========================================================")
		fmt.Println("提示: 本工具尽量在Linux或类Unix环境下使用")
		fmt.Println("      Windows系统兼容性可能不佳, 部分功能可能无法正常工作")
		fmt.Println("      建议使用 PowerShell、Windows Terminal、Windows Subsystem for Linux (WSL) 或 Git Bash 等终端工具")
		fmt.Println("========================================================")
	}
}

// Version 打印版本信息
func Version() {
	hash := hash()
	tag := tag()
	gitBranchCommit := fmt.Sprintf("%s-%s", tag, hash)
	printPlatformWarning()
	fmt.Println("mysshw 版本信息")
	fmt.Println("Go version:", runtime.Version())
	fmt.Println("Git version:", gitBranchCommit)
	fmt.Println("构建平台:", runtime.GOOS)
	fmt.Println("构建架构:", runtime.GOARCH)
}

// Help 打印帮助信息
func Help() {
	printPlatformWarning()
	fmt.Println("mysshw 构建工具使用说明")
	fmt.Println("可用的构建目标:")
	fmt.Println("  Build    - 为当前平台构建项目, 默认不创建压缩包")
	fmt.Println("  Pack     - 为所有平台构建项目并打包,默认创建压缩包")
	fmt.Println("  Test     - 运行所有测试")
	fmt.Println("  Clean    - 清理构建文件 dist 目录")
	fmt.Println("  Help     - 显示此帮助信息")
	fmt.Println("  Version  - 打印版本信息")
	fmt.Println("使用示例:")
	fmt.Println("  mage Build       # 为当前平台构建")
	fmt.Println("  mage Pack        # 为所有平台构建并打包")
	fmt.Println("  mage Test        # 运行测试")
	fmt.Println("  mage Clean       # 清理构建文件")
	fmt.Println("  mage Help        # 显示帮助信息")
}
