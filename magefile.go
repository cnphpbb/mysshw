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
	return buildTarget(runtime.GOOS, runtime.GOARCH, nil, false)
}

// build all platform
func Pack() error {
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
	// cp -a ./example/mysshw.toml ./dist/{name}/mysshw.toml
	sh.Run("cp", "-a", "example/mysshw.toml", fmt.Sprintf("%s/mysshw.toml", dir))

	// Only create tar archive if requested
	if shouldCreateTar == true {
		sh.Run("tar", "-czf", fmt.Sprintf("%s.tar.gz", dir), "-C", "dist", name)
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
	for _, f := range fs {
		if f.IsDir() {
			continue
		}

		if strings.HasSuffix(f.Name(), ".tar.gz") {
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
	s, _ := sh.Output("date", "+%Y-%m-%d %H:%M:%S")
	return s
}

func versionStr() string {
	s, _ := sh.Output("date", "+%y.%m.%d")
	return fmt.Sprintf("v%s", s)
}
