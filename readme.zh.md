# mysshw

**开源免费的SSH命令行客户端工具**  
**Go 1.23.0 及以上版本**  
**支持 Linux、macOS、Windows 平台（Windows 强烈推荐使用 Docker）**

[English Documentation](readme.md) · [更新日志](changelog.v26.06.13.2.md)

> 🐳 **Docker 用户**：官方镜像（`mysshw:latest`）是 Windows 以及任何终端行为与 Unix 不一致的运行环境下的**推荐运行方式**。详见 [Docker](#docker)。

## 功能特性

- 🚀 **SSH 2.0 协议全功能** — 终端会话（Raw 模式）、窗口自适应、KeepAlive 保活
- 🔑 **灵活认证方式** — 密码、密钥、带密码短语密钥、键盘交互
- 🛡 **主机指纹校验 (TOFU)** — Trust On First Use，共享 `~/.ssh/known_hosts`
- 🛠 **TOML 配置管理** — 节点分组、`${ENV_VAR}` 密码引用（避免明文落盘）
- 🖥 **huh 交互式菜单** — Lipgloss 颜色高亮、虚拟"返回上级"节点
- 🔌 **配置远程同步** — SCP、WebDAV、S3（MinIO / RustFS / 云 S3）
- 🔁 **会话可循环** — 退出 SSH 自动返回主菜单，无需重启进程
- 🐳 **Docker 镜像** — 多阶段构建，~21 MB，非 root 用户
- 📦 **静态二进制** — `CGO_ENABLED=0`，跨平台直接运行

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

访问 [Release 页面](https://github.com/cnphpbb/mysshw/releases) 下载对应平台版本。

### Docker

```bash
# 构建
docker build -t mysshw:latest .
docker build -t mysshw:v26.06.13.2 \
  --build-arg VERSION=v26.06.13.2 \
  --build-arg BUILD=$(git rev-parse --short HEAD) \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) .

# 运行
docker run --rm -it mysshw:latest
docker run --rm -v $PWD/mysshw.toml:/home/mysshw/.mysshw.toml:ro mysshw
```

镜像基于 `alpine:3.19`，以非 root 用户 `mysshw` 运行，包含 ca-certificates + tzdata。详见 `Dockerfile`。

> **为什么 Windows 推荐 Docker**：mysshw 深度使用 Unix 终端语义（Raw 模式、KeepAlive goroutines、scp 二进制调用）。Windows 原生环境有多项限制（`Ctrl+D` 无效、需将 `scp.exe` 加入 PATH 等）。Docker 提供一致的 Linux 用户态，是 Windows 下的推荐运行方式。

## 配置文件

配置文件可放在以下任一位置：
- `~/.mysshw.toml`（推荐）
- `~/.mysshw`
- `./.mysshw.toml`
- `./.mysshw`

默认路径：`$HOME/.mysshw.toml`。可用 `--cfg /path/to/your.toml` 自定义。

### 配置示例

```toml
cfg_dir = "~/.mysshw.toml"   # default: $HOME/.mysshw.toml

[sync]
type = "scp"                  # scp | webdav | s3
remote_uri = "127.0.0.1:22"
remote_path = "/data/backup/mysshw/mysshw.toml"

[sync.scp]
username = "root"
password = "${MYSSHW_PWD_sync}"  # 支持 ${VAR} 和 ${VAR:-default}
keypath = ""
passphrase = ""

[sync.webdav]
auth = "Basic"                # Basic | Digest
username = "root"
password = "${MYSSHW_PWD_webdav}"

[sync.s3]
access_key = ""               # 访问密钥
secret_key = ""               # 密钥
bucket_name = ""              # 桶名
region = ""                   # 区域
endpoint = ""                 # 终端节点 (空 → 用 remote_uri)

[[nodes]]
groups = "Groups01"

[[nodes.ssh]]
alias = "Test"
name = "vm-test-1"
host = "127.0.0.1"
user = "root"
password = "${MYSSHW_PWD_vm_test_1}"
#keypath = "~/.ssh/id_rsa"
#passphrase = "${MYSSHW_KEY_PASSPHRASE}"
#strict_host_key_checking = false  # 跳过主机指纹校验 (仅测试)

[[nodes]]
groups = "Groups02"
ssh = [
    { name = "web01", host = "192.168.1.101", user = "admin", port = 22 },
    { name = "db01",  host = "192.168.1.102", user = "admin", keypath = "~/.ssh/id_rsa" },
]
```

> ⚠️ 配置文件可能含密码/密钥引用，mysshw 加载后会自动收紧到 `0600`（Windows 由 NTFS ACL 控制）。

## 架构

```mermaid
graph TB
    subgraph User["用户层"]
        UI[huh 交互菜单<br/>两级选择]
        CLI[命令行<br/>sync / yml / version]
    end

    subgraph Core["核心层"]
        Config[config 包<br/>TOML 加载 + 校验<br/>chmod 0600]
        Env[env 解析<br/>${MYSSHW_PWD_*}<br/>MYSSHW_SKIP_HOST_KEY_CHECK]
        TOFU[TOFU 主机指纹<br/>~/.ssh/known_hosts<br/>per-node StrictHostKeyChecking]
    end

    subgraph Protocols["协议层"]
        SSH[SSH 客户端<br/>xterm + Resize<br/>KeepAlive 10s]
        SCP[SCP 客户端<br/>自实现协议]
        S3[S3 客户端<br/>MinIO SDK]
        WebDAV[WebDAV 客户端<br/>gowebdav]
    end

    subgraph External["外部"]
        Server[SSH/SFTP 服务器]
        S3Store[MinIO / RustFS<br/>AWS S3]
        DAV[WebDAV 服务<br/>坚果云等]
    end

    UI --> Config
    CLI --> Config
    Config --> Env
    Config --> UI
    UI -->|Login| SSH
    Env -->|password/passphrase| SSH
    SSH --> TOFU
    SSH --> Server
    CLI -->|sync --upload/--down| SCP
    CLI -->|sync --upload/--down| S3
    CLI -->|sync --upload/--down| WebDAV
    SCP --> Server
    S3 --> S3Store
    WebDAV --> DAV
```

**关键设计决策**：

- **config 是唯一真相源**：所有流程都从 `LoadViperConfig` → `ValidateConfig` → `chmod 0600` 开始
- **环境变量在运行时解析**：`password` / `passphrase` 中的 `${MYSSHW_PWD_*}` 在 SSH/SFTP 连接时（而非加载配置时）替换
- **TOFU 默认开启，opt-out 关闭**：`SSHNode.StrictHostKeyChecking = false` 关闭单节点；`MYSSHW_SKIP_HOST_KEY_CHECK=true` 全局关闭
- **远程写入只能通过 sync**：没有隐式远程写，`mysshw sync -u` / `-z` 是用户显式动作

## 环境变量 (Environment Variables)

mysshw 支持以下环境变量，用于非交互式场景（CI/脚本/容器）。

### 主机指纹校验

| 变量名 | 取值 | 说明 |
|--------|------|------|
| `MYSSHW_SKIP_HOST_KEY_CHECK` | `true` / `1` / `yes` | 跳过所有主机指纹校验（用 `InsecureIgnoreHostKey`）。**有 MITM 风险**，仅推荐测试或隔离网络环境。 |
| `MYSSHW_KNOWN_HOSTS` | 绝对路径 | 自定义 `known_hosts` 文件路径。优先级最高，覆盖 `config.KnownHostsPath` 和默认 `~/.ssh/known_hosts`。支持 `~` 和 `$HOME` 展开。 |

### 密码与私钥口令（`${VAR}` 引用）

`password` 和 `passphrase` 字段支持 Shell 风格的 `${VAR}` 引用，**避免明文落盘**：

```toml
[[nodes.ssh]]
name = "web01"
host = "192.168.1.101"
user = "admin"
password = "${MYSSHW_PWD_web01}"            # 必填
# passphrase = "${MYSSHW_KEY_PASSPHRASE}"  # 私钥口令同样支持
# password = "${MYSSHW_PWD_x:-fallback}"   # 未设置时使用默认值
# password = "${MYSSHW_PWD_x:-}"           # 未设置时为空, 走交互输入
```

**语法**：
- `${VAR}` — 取环境变量 `$VAR`；未设置或为空时替换为空字符串
- `${VAR:-default}` — `$VAR` 未设置或为空时使用 `default`（default 可含空格、标点）
- 其他文本原样保留

**使用示例**：

```bash
# 方式 1: 启动前 export
export MYSSHW_PWD_web01='S3cret!'
mysshw -c ~/.mysshw.toml

# 方式 2: 配合 secret 管理工具 (Vault, 1Password CLI, pass 等)
export MYSSHW_PWD_web01=$(op read 'op://vault/web01/password')
mysshw -c ~/.mysshw.toml
```

### 配置文件权限

mysshw 加载配置后会自动将权限收紧到 `0600`（仅所有者可读写），备份文件同样处理。
Windows 上 `Chmod` 是 no-op，权限由 NTFS ACL 控制。

## 使用示例

```bash
# 查看帮助信息
mysshw --help | -h

# 启动程序（无参数时默认进入交互模式）
mysshw

# 指定配置文件路径
mysshw --cfg /path/to/custom/config.toml
mysshw -c /path/to/custom/config.toml   # 短选项

# 查看版本信息
mysshw version | --version | -v

# 同步配置文件到远程服务器
mysshw sync --upload | -u

# 从远程服务器下载配置文件
mysshw sync --down | -z

# 使用自定义配置文件路径进行同步
mysshw sync --cfg /path/to/custom/config.toml --upload
mysshw sync -c /path/to/custom/config.toml -u   # 混合短选项

# 从 sshw 的 YAML 配置迁移到 mysshw TOML 配置
mysshw yml -f ~/.sshw.yml
mysshw yml --file ~/.sshw.yml   # 长选项

# 查看子命令帮助
mysshw sync --help | -h
mysshw yml --help | -h
```

## TODO

简要状态 — 完整列表见 [TODO.md](TODO.md)。

- ✅ P0：仓库健康（`.gitattributes`、`analysis.md` 忽略、filter-repo 清理）
- ✅ P0/P1：关键 Bug 修复（SCP panic、Login 回调）
- ✅ P1：安全（TOFU、env vars、chmod 0600、`${VAR}` 密码引用）
- ✅ P2：代码质量（auth 删除、tag 统一、context 传播等）
- 🟡 P2：UI 主题（搁置）、S3/WebDAV mock 测试、端到端 SSH 测试
- ✅ P3：Dockerfile、架构图、changelog 规则
- ⏸️ P3：CI/CD、Homebrew、Scoop（非目标）
- ❌ P3：`go install` 文档化、readme 中的 changelog 链接

## Go Packages 依赖

- [github.com/magefile/mage](https://github.com/magefile/mage) — 构建系统
- [github.com/spf13/cobra](https://github.com/spf13/cobra) — CLI 框架
- [github.com/spf13/viper](https://github.com/spf13/viper) — 配置加载
- [github.com/GuanceCloud/toml](https://github.com/GuanceCloud/toml) — TOML 编解码
- [github.com/charmbracelet/huh](https://github.com/charmbracelet/huh) — TUI 菜单
- [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) — 终端样式
- [github.com/minio/minio-go/v7](https://github.com/minio/minio-go) — S3 客户端
- [github.com/studio-b12/gowebdav](https://github.com/studio-b12/gowebdav) — WebDAV 客户端
- [golang.org/x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) — SSH 协议
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) — 终端 Raw 模式
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) — YAML 迁移
- [github.com/stretchr/testify](https://github.com/stretchr/testify) — 测试框架

完整列表及版本见 [go.mod](go.mod)。

## 测试

```bash
go test -v ./...
mage test          # 含覆盖率报告
```

使用 [testify](http://github.com/stretchr/testify) 作为 Go 测试框架。测试覆盖包括：

- **110+ 个测试用例**，分布于 13 个测试文件
- SCP 协议层：100% 覆盖
- `ValidateConfig`：27 个用例覆盖全部分支
- `ssh/hostkey.go` TOFU + env var + callback 策略
- Mock SSH server（用 `golang.org/x/crypto/ssh`）端到端传输测试

## 贡献指南

欢迎提交 Issue 和 PR！项目遵循 MIT 开源协议。详见 [AGENTS.md](AGENTS.md) 中的 AI 代理开发指南和流程规范（commit 格式、tag/changelog 流程、分支分类）。

## 许可证

[MIT](../LICENSE)

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
mage clean   # 清理 dist/
mage build   # 开发编译 (不打 tar)
mage pack    # 发布打包 (8 平台 + tar/zip + sha1)

./mysshw -h                # 查看帮助
./mysshw -c ./mysshw.toml  # 指定配置运行

# 设置别名方便使用
echo "alias mysshw='./mysshw -c ./mysshw.toml'" >> ~/.bashrc
source ~/.bashrc
```

### Windows 平台（原生）

> ⚠️ **不推荐** — 请改用 [Docker](#docker)。Windows 原生有多项限制：

- `Ctrl+D` 不支持（请使用 `Ctrl+C` 或输入 `q | Q`）
- `sync` 子命令需要 `scp.exe` 在 `PATH` 中
- 终端 Raw 模式行为与 Unix 不同

如必须在 Windows 原生运行：

1. 使用 PowerShell、Windows Terminal、WSL 或 Git Bash
2. 确保已安装 OpenSSH 客户端、Git、mingw64
3. 添加以下路径到系统 `PATH`：
   - `C:\Windows\System32\OpenSSH`
   - `C:\Program Files\Git\usr\bin`
   - `C:\Program Files\Git\mingw64\bin`
   - `C:\Program Files\Git\usr\sbin`
   - `C:\Program Files\Git\usr\libexec\git-core`
   - `C:\Program Files\Git\mingw64\libexec\git-core`
4. 重启终端，使环境变量生效

**支持**： `.\mysshw.exe -c D:\mydata\mysshw\mysshw.toml`

#### PowerShell 中设置别名

PowerShell 的 `Set-Alias` 不支持带参数命令，需在 profile 中定义函数：

```powershell
notepad $profile   # 打开 profile (如不存在需先创建)

# 在 profile 中添加：
function mysshw {
    D:\sbin\mysshw.exe -c D:\mydata\mysshw\mysshw.toml $args
}
```

重启 PowerShell 激活。

## 更新日志

最新发布见 [changelog.v26.06.13.2.md](changelog.v26.06.13.2.md)。流程规则：每次在 `main` 分支打版本 tag 前必须生成 `changelog.${tag}.md`（详见 [AGENTS.md](AGENTS.md)）。
