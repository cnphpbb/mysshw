# mysshw

**Open source free SSH command line client tool**  
**Go 1.23.0 and above**  
**Supports Linux, macOS, Windows platforms**  

[中文文档](readme.zh.md)

## Installation Guide

### Install from source
```bash
git clone https://github.com/cnphpbb/mysshw.git mysshw
cd mysshw
go mod tidy
go install github.com/magefile/mage@latest
mage build
```
### Download binary
Visit the Release page to download the version for your platform
https://github.com/cnphpbb/mysshw/releases

## Features

- 🚀 **Multi-protocol support**
  - Full SSH 2.0 protocol implementation
  - SCP file transfer protocol support
  - Terminal session management
  
- 🔑 **Flexible authentication methods**
  - Password authentication
  - Key authentication
  - Key with passphrase support
  - Interactive keyboard authentication

- 🛠 **Configuration management**
  - TOML format configuration file
  - Support for node group management
  - Configuration sync function (SCP implemented, GitHub/Gitee in development)
  - Auto-generate default configuration
  - Comprehensive configuration file validation
  - Support for custom configuration file paths
  - Cross-platform path format support (Windows/Linux/MacOS)
  - Remote backup and restore of configurations
  - Automatic configuration file backup

- 🖥 **Terminal experience**
  - Adaptive window size
  - KeepAlive support
  - Color highlighting
  - Command history (in development)
  - Multiple exit methods (Ctrl+d, Ctrl+c, input q)
  - Automatically return to main interface after exiting SSH session
  - Exit method: First input Ctrl+c, then input q or Q or Ctrl+d

- 💻 **Cross-platform compatibility**
  - Support for Linux, macOS, Windows operating systems
  - Path handling optimization for different platforms

## Configuration file
Default path: ~/.mysshw.toml

```toml
cfg_dir = "~/.mysshw.toml"

[sync]
type = "scp"
remote_uri = "127.0.0.1:22"
remote_path = "/path/to/backup"
[sync.scp]
username = "root"
password = "$ZK7M@~1RY#Scp"
keyPath = "~/.ssh/id_rsa"
passphrase = ""

[[nodes]]
groups = "Production Servers"
ssh = [
    { name="web01", host="192.168.1.101", user="admin", port=22 },
    { name="db01", host="192.168.1.102", keypath="~/.ssh/id_rsa" }
]

[[nodes]]
groups = "Test Environment"

[[nodes.ssh]]
host = 'dev.example.com'
name = 'dev01'
password = 'test123'
user = 'root'
port = 22
 ```
## Go Packages dependencies

- github.com/magefile/mage
- github.com/spf13/cobra
- github.com/GuanceCloud/toml
- github.com/spf13/viper
- github.com/charmbracelet/huh
- github.com/charmbracelet/lipgloss
- github.com/pkg/sftp
- golang.org/x/crypto/ssh

Detailed dependency information can be found in the [go.mod](go.mod) file

## TODO

### Feature development

- RunSSH feature
  - [ ] Main interface supports themes
- Configuration management
  - [ ] Add configuration file encryption option
- Sync function
  - [x] SCP/SFTP
  - [x] WebDAV
  - [x] S3 (RustFS, MinIO community edition, cloud platform S3)
- Release plan
  - [ ] Create Docker image
  - [ ] Automated build and test process
- Code optimization
  - [ ] Add integration tests

### Completed features

- RunSSH
  - Exit session and return to main interface
  - Support multiple exit methods(Ctrl+d/Ctrl+c/q)
  - Main interface supports search
- Configuration management
  - File validation feature
  - Custom configuration file path
  - Cross-platform path support(Windows/Linux/MacOS)
  - sshw configuration import
  - Remote backup and restore of configurations
  - Automatic configuration file backup
- User interface
  - Command auto-completion
  - Replace promptui with charmbracelet/huh
- Release
  - GitHub Releases

## Usage examples
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
mysshw sync --down | -z

# Sync with custom configuration file path
mysshw sync --cfg /path/to/custom/config.toml --upload | --down
# Or mix short options
mysshw sync -c /path/to/custom/config.toml -u | -z

# Migrate from sshw's YAML configuration to mysshw TOML configuration
mysshw yml -f ~/.sshw.yml
# Or use long option
mysshw yml --file ~/.sshw.yml

# View sync command help
mysshw sync --help | -h

# View yml command help
mysshw yml --help | -h
```

## Contribution guide
Welcome to submit Issues and PRs! The project follows the MIT open source license.

## License
MIT

## Project compilation

```bash
# Optional: Use Docker to build
docker compose -p base -f ./docker-compose.yml up -d
docker exec -it build_go bash
# Optional: Add safe directory
git config --global --add safe.directory /app
# The following must be executed in the project root directory
go mod tidy
go install github.com/magefile/mage@latest
mage clean  // Clean build directory dist
mage build  // Development build, without tar package
mage pack   // Release packaging build
./mysshw -h   // View help information
./mysshw -c ./mysshw.toml   // Start program, specify configuration file and create an alias
# Reference:
# alias mysshw='./mysshw -c ./mysshw.toml'
# Or
# echo "alias mysshw='./mysshw -c ./mysshw.toml'" >> ~/.bashrc
# source ~/.bashrc
# You can use the mysshw command directly
./mysshw // Find default configuration file, location ~/.mysshw.toml. If there is no default configuration file, it will report an error and automatically generate a default configuration file for the first time
```
### Windows platform
- On Windows platform, it is recommended to use PowerShell, Windows Terminal, Windows Subsystem for Linux (WSL) or Git Bash and other terminal tools for the best experience.
- Ensure OpenSSH client, git, mingw64 and other tools are installed
- Configure environment variables
  - Ensure `C:\Windows\System32\OpenSSH` directory has been added to the system environment variable `PATH`
  - Ensure `C:\Program Files\Git\usr\bin` directory has been added to the system environment variable `PATH`
  - Ensure `C:\Program Files\Git\mingw64\bin` directory has been added to the system environment variable `PATH`
  - Ensure `C:\Program Files\Git\usr\sbin` directory has been added to the system environment variable `PATH`
  - Ensure `C:\Program Files\Git\usr\libexec\git-core` directory has been added to the system environment variable `PATH`
  - Ensure `C:\Program Files\Git\mingw64\libexec\git-core` directory has been added to the system environment variable `PATH`
  - Ensure `C:\Program Files\Git\mingw64\bin` directory has been added to the system environment variable `PATH`
  - Ensure `C:\Program Files\Git\usr\libexec\git-core` directory has been added to the system environment variable `PATH`

- Restart the terminal to make the environment variables take effect  

**Support `\.\mysshw.exe -c D:\mydata\mysshw\mysshw.toml` to start the program, specify configuration file**
- Support `Ctrl+d` to exit the program, not supported on Windows
- Support `q | Q` to exit the program

**Support `D:\sbin\mysshw.exe -c D:\mydata\mysshw\mysshw.toml` to start the program, specify configuration file** by 2025-08-24

#### Set alias in PowerShell
- Open PowerShell terminal
- Execute the following command to set alias
- Since Set-Alias does not support commands with parameters, you cannot directly set alias `mysshw="D:\sbin\mysshw.exe -c D:\mydata\mysshw\mysshw.toml"`

  ```powershell
  Set-Alias -Name mysshw -Value "D:\sbin\mysshw.exe"
  ```
- Execute the following command to check if the alias is set successfully

  ```powershell
  Get-Alias -Name mysshw | Format-List
  ```

- Execute the following command to create a PowerShell function with parameters
  > Open PowerShell configuration file (path obtained via $profile), add the following content:
  > Note: If there is no configuration file, you need to create one first
  
  - Execute the following command to open the configuration file
  ```powershell
  notepad $profile
  ```
  - Add the following content

  ```powershell
  function mysshw {
    D:\sbin\mysshw.exe -c D:\mydata\mysshw\mysshw.toml $args
  }
  ```

- Restart PowerShell terminal to make the alias take effect