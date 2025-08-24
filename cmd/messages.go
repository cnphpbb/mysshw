package cmd

const (
	RunSSHClientLoginSessionEndCallbackStr = "SSH session ended, returning to main menu..."
	GlobalScreenClearingStr                = "\033[H\033[2J"
	GlobalExitingDescStr                   = `Press "Ctrl+C" and then press "q | Q" or "Ctrl+D" to exit, or select the SSH node:`
	GlobalExitingStr                       = "Exiting..."
	RunSSHCtrlCHintStr                     = "\nReceived termination signal, mysshw:: Exiting..."
	RunSSHCtrlCResultStr                   = "\nReceived termination signal, mysshw:: Exiting..."
	RunSSHCtrlDResultStr                   = "\nReceived Ctrl+D, mysshw:: Exiting..."
	RunSSHInputQResultStr                  = "\nReceived input %s, mysshw:: Exiting...\n"
)
