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
)
