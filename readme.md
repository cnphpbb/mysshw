# mysshw

**mysshw - a free and open source ssh cli client soft.**

[Chinese Documentation](readme.zh.md)
## install

go version <= 1.16.*    
use `go get`

```
go get -u github.com/cnphpbb/mysshw
```

go version >= 1.17.*
use `go install`

```
go install github.com/cnphpbb/mysshw
```

or download binary from [releases](//github.com/cnphpbb/mysshw/releases).

## config

put config file in `~/.mysshw` or `~/.mysshw.tml` or `~/.mysshw.toml` or `./.mysshw` or `./.mysshw.tml` or `./.mysshw.toml`.

config example:

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

## testing
use [testify](http://github.com/stretchr/testify) Go testing framework.

## Sync Actions Type List
1. [x] SCP
2. [ ] Github - gist
3. [ ] Gitee - gist
4. [ ] API - http(s)
5. [ ] RPC

## build

```bash
docker compose -p base -f ./docker-compose.yml up -d
docker exec -it build_go bash
go mod tidy
go install github.com/magefile/mage@latest
git config --global --add safe.directory /app
mage build // Development build
mage pack // Release build
./mysshw -h // view help information
./mysshw -c ./mysshw.toml // run with config file
```