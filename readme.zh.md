# mysshw

**开源免费的SSH命令行客户端工具**

[English Documentation](readme.md)

## 功能特性

- 🚀 **多协议支持**
  - SSH 2.0协议全功能实现
  - SCP文件传输协议支持
  - 终端会话管理
  
- 🔑 **灵活认证方式**
  - 密码认证
  - RSA/DSA密钥认证
  - 带密码短语的密钥支持
  - 交互式键盘认证

- 🛠 **配置管理**
  - TOML格式配置文件
  - 支持节点分组管理
  - 配置同步功能（SCP/GitHub/Gitee）
  - 自动生成默认配置

- 🖥 **终端体验**
  - 自适应窗口大小
  - 支持KeepAlive保活
  - 颜色高亮显示
  - 历史命令记录

## 安装指南

### 从源码安装
```bash
go get -u github.com/cnphpbb/mysshw
```
### 下载二进制
访问 Release页面 下载对应平台版本

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
mysshw --help

# 启动程序（无参数时默认进入交互模式）
mysshw

# 指定配置文件路径
mysshw -c ./mysshw.toml

# 同步配置文件
mysshw sync

# 查看同步命令帮助
mysshw sync --help
```

## 命令行选项
```bash
# 全局选项
-c, --cfg string: 指定配置文件路径（默认为 $HOME/.mysshw.toml）
-h, --help: 显示帮助信息

# sync 子命令选项
-h, --help: 显示同步命令帮助
```
## 贡献指南
欢迎提交Issue和PR！项目遵循MIT开源协议。

## 许可证
MIT

## go pkg
github.com/magefile/mage
github.com/spf13/cobra
github.com/BurntSushi/toml
github.com/spf13/viper

## 项目编译

```bash
docker compose -p base -f ./docker-compose.yml up -d
docker exec -it build_go bash
go mod tidy
go install github.com/magefile/mage@latest
git config --global --add safe.directory /app
mage build // 开发编译
mage pack // 发布打包编译
./mysshw -h // 查看帮助信息
./mysshw -c ./mysshw.toml // 启动程序, 指定配置文件 然后做个alias
//参考：
//alias mysshw='./mysshw -c ./mysshw.toml'
./mysshw // 查找默认配置文件, 位置 ~/.mysshw.toml。 如果没有默认配置文件, 则第一次会报错并自动生成默认配置文件
```