package ssh

const (
	MsgSelectNodeGroup     = "Select SSH Node Groups.(选择主机组)"
	MsgSelectNode          = "Select SSH Node.(选择主机)"
	MsgSelectDesc          = "Use arrow keys to navigate, press Enter to select."
	MsgPrintLnStr          = "Operation cancelled"
	NodeParentAlias        = "返回上级"
	NodeParentName         = "-parent-"
	SSHConnectInfoStr      = "connect server ssh -p %d %s@%s version: %s \n"
	SSHClientConnectPwdStr = "Contains %s@%s's password:"
	errFormRunError        = "interrupted"

	// 主机指纹校验相关提示
	HostKeyFirstTimeTitleStr    = "主机指纹校验 - 首次连接"
	HostKeyFirstTimePromptStr   = "无法确认主机 %q 的真实性。\n  指纹: %s\n是否信任并继续连接? [y/N]: "
	HostKeyFirstTimeAcceptedStr = "已信任主机指纹, 保存到: %s"
	HostKeyFirstTimeRejectedStr = "用户拒绝信任主机指纹, 连接终止"
	HostKeyMismatchErrorStr     = "主机指纹与 known_hosts 不匹配! 可能的中间人攻击或主机密钥已变更\n  主机: %s\n  期望: %s\n  实际: %s"

	// 环境变量名 (用于非交互场景)
	EnvSkipHostKeyCheck = "MYSSHW_SKIP_HOST_KEY_CHECK"
	EnvKnownHosts       = "MYSSHW_KNOWN_HOSTS"
	EnvPasswordPrefix   = "MYSSHW_PWD_"
)
