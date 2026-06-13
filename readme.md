# mysshw

**mysshw - A free and open source SSH command line client software.**  
**Go 1.23.0 and above**  
**Supports Linux, macOS, and Windows platforms**  

[中文文档](readme.zh.md)

### Binary Release
Download the latest binary from [releases](https://github.com/cnphpbb/mysshw/releases).

## Configuration

The configuration file can be placed in one of the following locations:
- `~/.mysshw`
- `~/.mysshw.tml`
- `~/.mysshw.toml`
- `./.mysshw`
- `./.mysshw.tml`
- `./.mysshw.toml`

Default configuration path: `$HOME/.mysshw.toml`

### Configuration Example:

```toml
cfg_dir = "~/.mysshw.toml"   # default:  $HOME/.sshw.toml

[sync]
type = "scp" # type: ( scp || webdav || s3 ) default: scp
remote_uri = "127.0.0.1:22"
remote_path = "/data/backup/mysshw/mysshw.toml"

[sync.scp]
username = "root"
password = "${MYSSHW_PWD_sync}"
keyPath = ""
passphrase = ""

[sync.webdav]
auth = "Basic" # Basic || Digest
username = "root"
password = "${MYSSHW_PWD_webdav}"

[sync.s3]
access_key = "" # 访问密钥
secret_key = "" # 密钥
bucket_name = "" # 桶名
region = "" # 区域
endpoint = "" # 终端节点 这个值为空，按 remote_uri 的值

[[nodes]]
groups = "Groups01"
ssh = [
    { name="vm-00", host="192.168.10.100", user="vm00", port=62922, password="qwe123!@#qwe" },
    { name="vm-01", host="192.168.10.101", user="vm00", port=22, password="qwe123!@#qwe", keypath="~/.ssh/id_rsa" },
    { name="vm-02", host="192.168.10.102", user="vm00", port=22, password="qwe123!@#qwe", keypath="~/.ssh/id_rsa", passphrase="abcdefghijklmn" },
    { name="vm-00", alias="vm-03", host="192.168.10.100", user="vm00", port=62922, password="qwe123!@#qwe" },
]

[[nodes]]
groups = "Groups02"
ssh = [
    { name="server 1", user="root", host="192.168.10.1", password="qwe123!@#qwe" },
    { name="server 1", user="root", host="192.168.10.2" },
    { name="server 2", host="192.168.10.3" },
]

[[nodes]]
groups = "Groups03"

[[nodes.ssh]]
alias = 'Test-1'
host = "192.168.10.1"
name="server 1"
password ="qwe123!@#qwe"
user = 'root'

[[nodes.ssh]]
alias = 'Test-2'
host = "192.168.10.2"
name="server 2"
password ="qwe123!@#qwe"
user = 'root'
port = 65522

[[nodes.ssh]]
alias = 'Test-3'
host = "192.168.10.3"
name="server 3"
password ="qwe123!@#qwe"
user = 'root'
port = 65522
keypath="~/.ssh/id_rsa"
passphrase = "cnphpbb#Yong"
```

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

### Config File Permissions

mysshw automatically tightens the loaded config file to mode `0600` (owner-only), same for backup files.
On Windows, `Chmod` is a no-op; permissions are controlled by NTFS ACL.

## Go Packages

- github.com/magefile/mage
- github.com/spf13/cobra
- github.com/GuanceCloud/toml
- github.com/spf13/viper
- github.com/charmbracelet/huh
- github.com/charmbracelet/lipgloss
- github.com/pkg/sftp
- golang.org/x/crypto/ssh

## Testing
We use [testify](http://github.com/stretchr/testify) as our Go testing framework.

## TODO

### RunSSH todo
- [x] Exit SSH session and return to main interface 
- [x] Support `Ctrl+d` to exit program, not supported on Windows system
- [x] Support `q` exit program
- [x] Support `Ctrl+c` exit program

### Sync Actions Type List
1. [x] SCP
2. [ ] Github - Gist
3. [ ] Gitee - Gist
4. [ ] API - HTTP(s)
5. [ ] RPC

## Usage Examples
```bash
# View help information
mysshw --help | -h

# Start the program (enter interactive mode by default without parameters)
mysshw

# Specify configuration file path
mysshw --cfg /path/to/custom/config.toml
# Or use short option
mysshw -c /path/to/custom/config.toml

# View version information
mysshw version | --version | -v

# Sync configuration file to remote server
mysshw sync --upload | -u

# Download configuration file from remote server
mysshw sync --down |-z

# Sync with custom configuration file path
mysshw sync --cfg /path/to/custom/config.toml --upload
# Or mix short options
mysshw sync -c /path/to/custom/config.toml -u

# Migrate from old SSHW YAML config to mysshw TOML config
mysshw yml -f ~/.sshw.yml
# Or use long option
mysshw yml --file ~/.sshw.yml

# View sync command help
mysshw sync --help | -h

# View yml command help
mysshw yml --help | -h
```

## Build
```bash
# Optional: Use Docker to build
docker compose -p base -f ./docker-compose.yml up -d
docker exec -it build_go bash
git config --global --add safe.directory /app
# Must be in project root directory
go mod tidy
go install github.com/magefile/mage@latest
mage clean  // Clean build directory dist
mage build  // Development build (without tar package)
mage pack   // Release build (with tar package)
./mysshw -h   // View help information
./mysshw -c ./mysshw.toml   // Run with config file

# Create an alias for convenience
# echo "alias mysshw='./mysshw -c ./mysshw.toml'" >> ~/.bashrc
# source ~/.bashrc
```