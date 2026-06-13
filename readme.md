# mysshw

**mysshw - A free and open source SSH command line client software.**  
**Go 1.23.0 and above**  
**Supports Linux, macOS, and Windows platforms**

[中文文档](readme.zh.md)

## Features

- 🚀 **SSH 2.0 full protocol**
  - SCP file transfer
  - Terminal session with raw mode
  - Adaptive window resize, KeepAlive

- 🔑 **Flexible authentication**
  - Password, public key, key with passphrase
  - Keyboard-interactive
  - Trust On First Use (TOFU) host key verification

- 🛠 **Configuration management**
  - TOML format
  - Node group organization
  - `${ENV_VAR}` password references (avoid plain-text storage)
  - Automatic config file permission tightening (0600)
  - Config backup & restore (SCP / WebDAV / S3)
  - Cross-platform path handling (Windows / Linux / macOS)
  - sshw YAML → TOML migration (`mysshw yml`)

- 🖥 **Terminal experience**
  - huh-based interactive menus
  - Lipgloss color highlighting
  - Multiple exit methods (Ctrl+D / Ctrl+C / `q`)
  - Return to main menu after SSH session ends

- 🔌 **Config sync (remote backup)**
  - SCP, WebDAV, S3 (MinIO / RustFS / cloud S3)

## Installation

### From Source

```bash
git clone https://github.com/cnphpbb/mysshw.git mysshw
cd mysshw
go mod tidy
go install github.com/magefile/mage@latest
mage build
```

### Binary Release

Download the latest binary from [releases](https://github.com/cnphpbb/mysshw/releases).

## Configuration

The configuration file can be placed in one of the following locations:
- `~/.mysshw.toml` (recommended)
- `~/.mysshw`
- `./.mysshw.toml`
- `./.mysshw`

Default configuration path: `$HOME/.mysshw.toml`

You can also use a custom path: `mysshw -c /path/to/your.toml`.

### Configuration Example

```toml
cfg_dir = "~/.mysshw.toml"   # default: $HOME/.mysshw.toml

[sync]
type = "scp"                  # scp | webdav | s3
remote_uri = "127.0.0.1:22"
remote_path = "/data/backup/mysshw/mysshw.toml"

[sync.scp]
username = "root"
password = "${MYSSHW_PWD_sync}"
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
#strict_host_key_checking = false  # skip host key check (test only)

[[nodes]]
groups = "Groups02"
ssh = [
    { name = "web01", host = "192.168.1.101", user = "admin", port = 22 },
    { name = "db01",  host = "192.168.1.102", user = "admin", keypath = "~/.ssh/id_rsa" },
]
```

> ⚠️ 配置文件可能含密码/密钥引用, mysshw 加载后会自动收紧到 `0600` (Windows 由 NTFS ACL 控制)。

## Environment Variables

mysshw supports the following environment variables for non-interactive scenarios (CI / scripts / containers).

### Host Key Verification

| Variable | Values | Description |
|----------|--------|-------------|
| `MYSSHW_SKIP_HOST_KEY_CHECK` | `true` / `1` / `yes` | Skip all host key checks (uses `InsecureIgnoreHostKey`). **MITM risk**, recommended only for test or isolated networks. |
| `MYSSHW_KNOWN_HOSTS` | absolute path | Custom `known_hosts` path. Highest priority, overrides `config.KnownHostsPath` and default `~/.ssh/known_hosts`. Supports `~` and `$HOME` expansion. |

### Password & Key Passphrase (`${VAR}` References)

`password` and `passphrase` fields support shell-style `${VAR}` references, **avoiding plain-text storage**:

```toml
[[nodes.ssh]]
name = "web01"
host = "192.168.1.101"
user = "admin"
password = "${MYSSHW_PWD_web01}"            # required
# passphrase = "${MYSSHW_KEY_PASSPHRASE}"  # key passphrase also supported
# password = "${MYSSHW_PWD_x:-fallback}"   # default value when unset
# password = "${MYSSHW_PWD_x:-}"           # empty default, fall back to interactive prompt
```

**Syntax**:
- `${VAR}` — substitute `$VAR`; empty if unset
- `${VAR:-default}` — use `default` when `$VAR` is unset/empty (default may contain spaces, punctuation)
- Other text preserved as-is

**Usage examples**:

```bash
# Method 1: export before launch
export MYSSHW_PWD_web01='S3cret!'
mysshw -c ~/.mysshw.toml

# Method 2: integrate with secret managers (Vault, 1Password CLI, pass, etc.)
export MYSSHW_PWD_web01=$(op read 'op://vault/web01/password')
mysshw -c ~/.mysshw.toml
```

### Config File Permissions

mysshw automatically tightens the loaded config file to mode `0600` (owner-only), same for backup files.
On Windows, `Chmod` is a no-op; permissions are controlled by NTFS ACL.

## Usage Examples

```bash
# View help information
mysshw --help | -h

# Start the program (enter interactive mode by default without parameters)
mysshw

# Specify configuration file path
mysshw --cfg /path/to/custom/config.toml
mysshw -c /path/to/custom/config.toml   # short option

# View version information
mysshw version | --version | -v

# Sync configuration file to remote server
mysshw sync --upload | -u

# Download configuration file from remote server
mysshw sync --down | -z

# Sync with custom configuration file path
mysshw sync --cfg /path/to/custom/config.toml --upload
mysshw sync -c /path/to/custom/config.toml -u   # mix short options

# Migrate from old SSHW YAML config to mysshw TOML config
mysshw yml -f ~/.sshw.yml
mysshw yml --file ~/.sshw.yml   # long option

# View subcommand help
mysshw sync --help | -h
mysshw yml --help | -h
```

## TODO

Quick status — see [TODO.md](TODO.md) for full list.

- ✅ P0: Repository hygiene (`.gitattributes`)
- ✅ P0/P1: Critical bug fixes (SCP panic, Login callback)
- ✅ P1: Security (TOFU, env vars, chmod 0600, `${VAR}` password refs)
- 🟡 P2: Code quality, integration tests
- ❌ P3: CI/CD, Docker image, Homebrew/Scoop

## Go Packages

- [github.com/magefile/mage](https://github.com/magefile/mage) — build system
- [github.com/spf13/cobra](https://github.com/spf13/cobra) — CLI framework
- [github.com/spf13/viper](https://github.com/spf13/viper) — config loading
- [github.com/GuanceCloud/toml](https://github.com/GuanceCloud/toml) — TOML codec
- [github.com/charmbracelet/huh](https://github.com/charmbracelet/huh) — TUI menus
- [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) — terminal styling
- [github.com/minio/minio-go/v7](https://github.com/minio/minio-go) — S3 client
- [github.com/studio-b12/gowebdav](https://github.com/studio-b12/gowebdav) — WebDAV client
- [golang.org/x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) — SSH protocol
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) — raw terminal
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) — YAML migration
- [github.com/stretchr/testify](https://github.com/stretchr/testify) — testing

See [go.mod](go.mod) for the complete list with versions.

## Testing

```bash
go test -v ./...
mage test          # with coverage report
```

We use [testify](http://github.com/stretchr/testify) as our Go testing framework.

## Contributing

Issues and PRs welcome! Project is MIT licensed.

## License

[MIT](../LICENSE)

## Build

```bash
# Optional: build in Docker
docker compose -p base -f ./docker-compose.yml up -d
docker exec -it build_go bash
git config --global --add safe.directory /app

# Must run in project root
go mod tidy
go install github.com/magefile/mage@latest
mage clean   # clean dist/
mage build   # development build (no tar)
mage pack    # release build (8 platforms + tar/zip + sha1)

./mysshw -h                # view help
./mysshw -c ./mysshw.toml  # run with config

# Create alias for convenience
echo "alias mysshw='./mysshw -c ./mysshw.toml'" >> ~/.bashrc
source ~/.bashrc
```

### Windows Platform

- Use PowerShell, Windows Terminal, WSL, or Git Bash for best experience.
- Install OpenSSH client, Git, mingw64.
- Add the following to system `PATH`:
  - `C:\Windows\System32\OpenSSH`
  - `C:\Program Files\Git\usr\bin`
  - `C:\Program Files\Git\mingw64\bin`
  - `C:\Program Files\Git\usr\sbin`
  - `C:\Program Files\Git\usr\libexec\git-core`
  - `C:\Program Files\Git\mingw64\libexec\git-core`
- Restart the terminal.

**Supported**: `.\mysshw.exe -c D:\mydata\mysshw\mysshw.toml`

> Note: `Ctrl+D` does not work on Windows. Use `Ctrl+C` or type `q | Q`.

#### PowerShell Alias

PowerShell's `Set-Alias` doesn't support parameters, so create a function in your profile:

```powershell
notepad $profile   # open profile (create $PROFILE first if missing)

# Add this to the profile:
function mysshw {
    D:\sbin\mysshw.exe -c D:\mydata\mysshw\mysshw.toml $args
}
```

Restart PowerShell to activate.
