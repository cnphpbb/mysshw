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
type = "scp" # type: ( scp || github || gitee || Api-http || rpc ) default: scp
remote_uri = "127.0.0.1:22"
username = "root"
password = "qweqwe123"
keyPath = ""
passphrase = ""
remote_path = "/data/backup/mysshw/mysshw.toml"
access_token = "" # gitee_access_token
gist_id = ""  #gist_id

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
```
## Go Packages

- github.com/magefile/mage
- github.com/spf13/cobra
- github.com/BurntSushi/toml
- github.com/spf13/viper
- github.com/manifoldco/promptui
- github.com/pkg/sftp
- golang.org/x/crypto/ssh

## Testing
We use [testify](http://github.com/stretchr/testify) as our Go testing framework.

## TODO

### RunSSH todo
- [x] Exit SSH session and return to main interface 
- [x] Support `Ctrl+d` exit program
- [x] Support `q` exit program （experimental）
- [x] Support `Ctrl+c` exit program （experimental，may exit with exception）

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

# View sync command help
mysshw sync --help | -h
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