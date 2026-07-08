package config

import "golang.org/x/crypto/ssh"

const CFGPATH string = "~/.mysshw.toml"

// DefaultConfig 包含默认配置的字符串
//
// 供 mysshw 首次运行 / 配置文件丢失时自动生成. 与 example/mysshw.toml 保持一致.
// 注意: 默认配置不含明文密码, 鼓励使用 ${MYSSHW_PWD_xxx} 环境变量引用.
//
// 完整字段参考: https://github.com/cnphpbb/mysshw/blob/master/readme.md#config
// MCP 集成文档: https://github.com/cnphpbb/mysshw/blob/master/examples/mcp-integration.md
const DefaultTomlConfig = `# mysshw 默认配置 (v26.07.04.0+)
# 首次运行自动生成. 重复运行仅在配置文件丢失时才会重新创建.
# 完整字段参考: https://github.com/cnphpbb/mysshw/blob/master/readme.md#config
#
# 推荐 workflow: 复制本文件到 ~/.mysshw.toml, 按需修改 sync 块与 [[nodes]] 块
# 密码字段使用 ${MYSSHW_PWD_xxx} env 引用, 避免明文落盘

cfg_dir = "./.mysshw.toml"   # default:  $HOME/.mysshw.toml

# known_hosts_path: 自定义主机指纹存储路径 (留空用 ~/.ssh/known_hosts)
# known_hosts_path = "~/.mysshw/known_hosts"

# backup_keep: 写 TOML 时保留的本地备份数量 (默认 5)
# backup_keep = 5

[sync]
type = "scp" # type: ( scp || webdav || s3 ) default: scp
remote_uri = "127.0.0.1:22"
remote_path = "/data/backup/mysshw/mysshw.toml" # remote file path

[sync.scp]
username = "root"
password = "${MYSSHW_PWD_sync}"  # 支持 ${VAR} 或 ${VAR:-default} 引用环境变量
keypath = ""        # 可选, 留空走密码认证
passphrase = ""     # 可选, 密钥口令也支持 env 引用

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


# === SSH 节点配置 (示例) ===
# 字段约定: name 必填 + 同 group 唯一, host 必填, user 必填 (留空默认 root),
#          port 选填 (默认 22), alias 全局唯一, password/keypath 至少填一个
# 推荐: 生产环境用 ${MYSSHW_PWD_xxx} 引用 env 变量, 避免明文落盘

[[nodes]]
groups = "Production"

# 节点 1.1: password = ${ENV} 引用 (推荐)
[[nodes.ssh]]
name = "prod-web-1"
alias = "Web01"
host = "10.0.0.1"
user = "app"
port = 22
password = "${MYSSHW_PWD_prod_web_1}"

# 节点 1.2: password = bcrypt 哈希 (推荐, 防明文落盘)
# 运行时 prompt 提示输入明文 + bcrypt 验证
# 生成新哈希: go run -ldflags="-s -w" - <<EOF  (见 mysshw 项目源码 config/password.go)
# import "golang.org/x/crypto/bcrypt"
# h,_ := bcrypt.GenerateFromPassword([]byte("your-password"), bcrypt.DefaultCost)
# fmt.Println(string(h))
[[nodes.ssh]]
name = "prod-db-1"
alias = "DB01"
host = "10.0.0.5"
user = "postgres"
port = 5432
password = "$2a$10$BBEDysNBBCnf/RwKMLiea.ufVrsybvoes.HYz8w.6wtff7TSLhjsC"
proxy = "bastion"

# 节点 1.3: password = 明文 (⚠️ 仅测试用, 不推荐生产)
[[nodes.ssh]]
name = "prod-test-1"
host = "10.0.0.99"
user = "app"
password = "test-password-do-not-use-in-prod"
strict_host_key_checking = false

# 节点 1.4: 纯密钥认证 (推荐用于 CI / 自动化)
# password 留空 + keypath; 验证: list -o json → auth="key"
[[nodes.ssh]]
name = "prod-deploy-1"
alias = "DeployBot"
host = "10.0.0.50"
user = "deploy"
# keypath = "~/.ssh/id_ed25519"

[[nodes]]
groups = "Staging"

# 节点 2.1: 简化的内联写法
[[nodes.ssh]]
name = "stag-web-1"
alias = "StagWeb"
host = "192.168.10.10"
user = "deploy"

[[nodes]]
groups = "DevOps"

# 节点 3.1: 跳板机 (其它节点的 proxy 引用此节点)
[[nodes.ssh]]
name = "bastion"
host = "bastion.example.com"
user = "ops"
port = 2222

# 节点 3.2: 通过 bastion 访问的内网机器
[[nodes.ssh]]
name = "internal-app-1"
host = "172.16.0.5"
user = "app"
proxy = "bastion"

[[nodes]]
groups = "Batch"

# 内联数组写法, 适合节点多且配置相似的场景
ssh = [
    { name="batch-1", host="10.1.0.1", user="worker", password="${MYSSHW_PWD_batch}" },
    { name="batch-2", host="10.1.0.2", user="worker", password="${MYSSHW_PWD_batch}" },
    { name="batch-3", host="10.1.0.3", user="worker", password="${MYSSHW_PWD_batch}" },
]
`

type (
	Configs struct {
		CfgDir string `toml:"cfg_dir" mapstructure:"cfg_dir"`
		// KnownHostsPath 自定义主机指纹存储路径, 留空时使用 ~/.ssh/known_hosts
		// 支持 ~ 和 $HOME 展开, 父目录不存在时会自动创建
		KnownHostsPath string `toml:"known_hosts_path,omitempty" mapstructure:"known_hosts_path"`
		// BackupKeep 写 TOML 时保留的本地备份数量 (默认 5)
		// 备份命名: <name>.<YYYYMMDD_HHMMSS>.bak
		// 0 或负数 → 使用 DefaultBackupKeep
		BackupKeep int      `toml:"backup_keep,omitempty" mapstructure:"backup_keep"`
		SyncCfg    SyncInfo `toml:"sync" mapstructure:"sync"`
		Nodes      []Nodes  `toml:"nodes" mapstructure:"nodes"`
	}

	SyncInfo struct {
		Type         string       `toml:"type" mapstructure:"type"`
		RemoteUri    string       `toml:"remote_uri" mapstructure:"remote_uri"`
		RemotePath   string       `toml:"remote_path" mapstructure:"remote_path"`
		SCPConfig    SCPConfig    `toml:"scp" mapstructure:"scp"`
		S3Config     S3Config     `toml:"s3" mapstructure:"s3"`
		WebDAVConfig WebDAVConfig `toml:"webdav" mapstructure:"webdav"`
	}
	WebDAVConfig struct {
		Auth     string `toml:"auth" mapstructure:"auth"`
		Username string `toml:"username" mapstructure:"username"`
		Password string `toml:"password" mapstructure:"password"`
	}
	SCPConfig struct {
		Username   string `toml:"username" mapstructure:"username"`
		Password   string `toml:"password" mapstructure:"password"`
		KeyPath    string `toml:"keypath,omitempty" mapstructure:"keypath"`
		Passphrase string `toml:"passphrase,omitempty" mapstructure:"passphrase"`
	}
	S3Config struct {
		AccessKey  string `toml:"access_key" mapstructure:"access_key"`
		SecretKey  string `toml:"secret_key" mapstructure:"secret_key"`
		BucketName string `toml:"bucket_name" mapstructure:"bucket_name"`
		Region     string `toml:"region" mapstructure:"region"`
		Endpoint   string `toml:"endpoint" mapstructure:"endpoint"`
	}
	Nodes struct {
		Groups   string     `toml:"groups"`
		SSHNodes []*SSHNode `toml:"ssh" mapstructure:"ssh"`
	}
	SSHNode struct {
		Name       string `toml:"name" mapstructure:"name"`
		Alias      string `toml:"alias,omitempty" mapstructure:"alias"`
		Host       string `toml:"host" mapstructure:"host"`
		User       string `toml:"user,omitempty" mapstructure:"user"`
		Port       int    `toml:"port,omitempty" mapstructure:"port"`
		KeyPath    string `toml:"keypath,omitempty" mapstructure:"keypath"`
		Passphrase string `toml:"passphrase,omitempty" mapstructure:"passphrase"`
		Password   string `toml:"password,omitempty" mapstructure:"password"`
		// Proxy 跳板节点 name (指向同一配置中的另一个 SSHNode)
		// 启用后, 连接顺序: client -> Proxy 节点 -> 本节点
		// 典型场景: 通过 bastion 跳板访问内网不可直连的机器
		// 注: 当前实现仅支持单跳, 多跳 (A -> B -> C) 不在阶段 4 范围
		Proxy string `toml:"proxy,omitempty" mapstructure:"proxy"`
		// StrictHostKeyChecking 控制对当前节点的主机指纹校验策略
		// nil  = 启用严格校验 (默认, 推荐, 防中间人攻击)
		// true = 启用严格校验
		// false = 跳过主机指纹校验 (不推荐, 测试环境可临时使用)
		StrictHostKeyChecking *bool `toml:"strict_host_key_checking,omitempty" mapstructure:"strict_host_key_checking"`
		// RotateKnownHosts 当检测到主机指纹 mismatch (key 已变更) 时:
		//   自动删除 known_hosts 中该 host 的旧条目, 并写入新 key
		// 适用场景: 服务器密钥轮换 (legitimate key rotation)
		// ⚠️ 安全警告: 此选项无法区分 "合法轮换" 与 "MITM 攻击 (伪造 key)"
		//   一旦误用, 等价于 OpenSSH StrictHostKeyChecking=no, 永久失去防 MITM 能力
		// 推荐用法: 仅在 --rotate-known-hosts flag 显式开启时由 cmd 层临时设置,
		//           不建议在 TOML 中持久开启
		RotateKnownHosts bool `toml:"rotate_known_hosts,omitempty" mapstructure:"rotate_known_hosts"`
	}
)

//type AutoGenerated struct {
//	CfgDir string `toml:"cfg_dir"`
//	Sync   struct {
//		Type        string `toml:"type"`
//		RemoteURI   string `toml:"remote_uri"`
//		Username    string `toml:"username"`
//		Password    string `toml:"password"`
//		KeyPath     string `toml:"keypath,omitempty"`
//		Passphrase  string `toml:"passphrase,omitempty"`
//		RemotePath  string `toml:"remote_path"`
//		AccessToken string `toml:"access_token"`
//		GistID      string `toml:"gist_id"`
//	} `toml:"sync"`
//	Nodes []struct {
//		Groups string `toml:"groups"`
//		SSH    []struct {
//			Name       string `toml:"name"`
//			User       string `toml:"user,omitempty"`
//			Host       string `toml:"host"`
//			Port       int    `toml:"port,omitempty"`
//			Keypath    string `toml:"keypath,omitempty"`
//			Passphrase string `toml:"passphrase,omitempty"`
//			Password   string `toml:"password,omitempty"`
//			Alias      string `toml:"alias,omitempty"`
//		} `toml:"ssh"`
//	} `toml:"nodes"`
//}

func (n *SSHNode) SetUser() string {
	if n.User == "" {
		return "root"
	}
	return n.User
}

func (n *SSHNode) SetPort() int {
	if n.Port <= 0 {
		return 22
	}
	return n.Port
}

func (n *SSHNode) SetKeyPath() string {
	if n.KeyPath == "" {
		return ""
	}
	return n.KeyPath
}

func (n *SSHNode) SetPassword() ssh.AuthMethod {
	if n.Password == "" {
		return nil
	}
	return ssh.Password(n.Password)
}

// IsStrictHostKeyChecking 返回该节点是否启用严格主机指纹校验
// StrictHostKeyChecking 为 nil 时默认启用 (返回 true)
func (n *SSHNode) IsStrictHostKeyChecking() bool {
	if n.StrictHostKeyChecking == nil {
		return true
	}
	return *n.StrictHostKeyChecking
}

const configReadInConfigPrintStr = `mysshw:: The configuration file '%s' was not detected,
    and a default configuration file '%s' was generated.
    vim %s -> Run mysshw again. `
