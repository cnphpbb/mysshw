# mysshw

**开源免费的SSH命令行客户端工具**  
**Go 1.23.0 及以上版本**  
**支持 Linux、macOS、Windows 平台**  

[English Documentation](readme.md)

## 功能特性

- 🚀 **多协议支持**
  - SSH 2.0协议全功能实现
  - SCP文件传输协议支持
  - 终端会话管理
  
- 🔑 **灵活认证方式**
  - 密码认证
  - 密钥认证
  - 带密码短语的密钥支持
  - 交互式键盘认证

- 🛠 **配置管理**
  - TOML格式配置文件
  - 支持节点分组管理
  - 配置同步功能（SCP已实现，GitHub/Gitee开发中）
  - 自动生成默认配置
  - 完善的配置文件校验功能
  - 支持自定义配置文件路径
  - 跨平台路径格式支持（Windows/Linux/MacOS）
  - 配置的远程备份与恢复
  - 配置文件的自动备份

- 🖥 **终端体验**
  - 自适应窗口大小
  - 支持KeepAlive保活
  - 颜色高亮显示
  - 历史命令记录(开发中)
  - 多种退出方式（Ctrl+d、Ctrl+c、输入q）
  - 退出SSH会话后自动返回主界面

- 💻 **跨平台兼容**
  - 支持Linux、macOS、Windows操作系统
  - 针对不同平台的路径处理优化

## 安装指南

### 从源码安装
```bash
git clone https://github.com/cnphpbb/mysshw.git mysshw
cd mysshw
go mod tidy
go install github.com/magefile/mage@latest
mage build
```
### 下载二进制
访问 Release页面 下载对应平台版本
https://github.com/cnphpbb/mysshw/releases

## TODO

### RunSSH todo
- [x] 退出 SSH 会话, 返回主界面
- [x] 支持 `Ctrl+d` 退出程序
- [x] 支持 `q` 退出程序 （试验性）
- [x] 支持 `Ctrl+c` 退出程序 （试验性，可能会有异常退出）

### Sync Actions Type List
1. [x] SCP
2. [ ] Github - Gist
3. [ ] Gitee - Gist
4. [ ] API - HTTP(s)
5. [ ] RPC

## 配置文件
默认路径： ~/.mysshw.toml

```toml
cfg_dir = "~/.mysshw.toml"

[sync]
type = "scp"
remote_uri = "127.0.0.1:22"
username = "root"
password = "your_password"
remote_path = "/path/to/backup"

[[nodes]]
groups = "生产服务器"
ssh = [
    { name="web01", host="192.168.1.101", user="admin", port=22 },
    { name="db01", host="192.168.1.102", keypath="~/.ssh/id_rsa" }
]

[[nodes]]
groups = "测试环境"
ssh = [
    { name="dev01", host="dev.example.com", password="test123" }
]
 ```

## 使用示例
```bash
# 查看帮助信息
mysshw --help | -h

# 启动程序（无参数时默认进入交互模式）
mysshw

# 指定配置文件路径
mysshw --cfg /path/to/custom/config.toml
# 或使用短选项
mysshw -c /path/to/custom/config.toml

# 查看版本信息
mysshw version | --version | -v

# 同步配置文件到远程服务器
mysshw sync --upload | -u

# 从远程服务器下载配置文件
mysshw sync --down | -z

# 使用自定义配置文件路径进行同步
mysshw sync --cfg /path/to/custom/config.toml --upload | --down
# 或混合使用短选项
mysshw sync -c /path/to/custom/config.toml -u | -z

# 查看同步命令帮助
mysshw sync --help | -h
```

## 贡献指南
欢迎提交Issue和PR！项目遵循MIT开源协议。

## 许可证
MIT

## Go Packages 依赖

- github.com/magefile/mage
- github.com/spf13/cobra
- github.com/BurntSushi/toml
- github.com/spf13/viper
- github.com/manifoldco/promptui
- github.com/pkg/sftp
- golang.org/x/crypto/ssh


## 项目编译

```bash
# 可选：使用 Docker 编译
docker compose -p base -f ./docker-compose.yml up -d
docker exec -it build_go bash
# 可选：添加安全目录
git config --global --add safe.directory /app
# 以下必须在项目根目录执行
go mod tidy
go install github.com/magefile/mage@latest
mage clean  // 清理编译目录 dist
mage build  // 开发编译，不打tar包
mage pack   // 发布打包编译
./mysshw -h   // 查看帮助信息
./mysshw -c ./mysshw.toml   // 启动程序, 指定配置文件 然后做个alias
# 参考：
# alias mysshw='./mysshw -c ./mysshw.toml'
# 或者
# echo "alias mysshw='./mysshw -c ./mysshw.toml'" >> ~/.bashrc
# source ~/.bashrc
# 可以直接使用 mysshw 命令
./mysshw // 查找默认配置文件, 位置 ~/.mysshw.toml。 如果没有默认配置文件, 则第一次会报错并自动生成默认配置文件
```
### Windows 平台
- 在 Windows 平台下，推荐使用 PowerShell、Windows Terminal、Windows Subsystem for Linux (WSL) 或 Git Bash 等终端工具，以获得最佳使用体验。
- 确保已安装 OpenSSH 客户端, git, mingw64 等工具
- 配置环境变量
  - 确保 `C:\Windows\System32\OpenSSH` 目录已添加到系统环境变量 `PATH` 中
  - 确保 `C:\Program Files\Git\usr\bin` 目录已添加到系统环境变量 `PATH` 中
  - 确保 `C:\Program Files\Git\mingw64\bin` 目录已添加到系统环境变量 `PATH` 中
  - 确保 `C:\Program Files\Git\usr\sbin` 目录已添加到系统环境变量 `PATH` 中
  - 确保 `C:\Program Files\Git\usr\libexec\git-core` 目录已添加到系统环境变量 `PATH` 中
  - 确保 `C:\Program Files\Git\mingw64\libexec\git-core` 目录已添加到系统环境变量 `PATH` 中
  - 确保 `C:\Program Files\Git\mingw64\bin` 目录已添加到系统环境变量 `PATH` 中
  - 确保 `C:\Program Files\Git\usr\libexec\git-core` 目录已添加到系统环境变量 `PATH` 中

- 重启终端，使环境变量生效  

**支持 `.\mysshw.exe -c D:\mydata\mysshw\mysshw.toml` 启动程序, 指定配置文件**
- 支持 `Ctrl+d` 退出程序
- 支持 `q` 退出程序 （试验性）

**支持 `D:\sbin\mysshw.exe -c D:\mydata\mysshw\mysshw.toml` 启动程序, 指定配置文件** by 2025-08-24
