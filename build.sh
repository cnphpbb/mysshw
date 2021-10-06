# CGO_ENABLED=0 ./build.sh

#当前版本号,每次更新服务时都必须更新版本号
CurrentVersion=v`date "+%Y%m%d"`

#项目名
Project=mysshw
BuildTime=`date "+%Y-%m-%d %H:%M:%S"`
GoVersion=`go version`
GitCommit=$(git rev-parse --short=9 HEAD || echo unsupported)


#Path=${Project}/sshw

go build -o ./build/$Project \
-ldflags \
"-w -s -X main.Version=$CurrentVersion.$GitCommit \
-X 'main.BuildTime=$BuildTime' \
-X 'main.Build=${GitCommit}' " \
main.go

echo "build finish !!"
echo "Version:" $CurrentVersion
echo "Git commit:" $GitCommit
echo "Go version:" $GoVersion
echo "Build Time:" $BuildTime