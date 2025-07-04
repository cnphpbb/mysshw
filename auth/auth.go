/**
 * @Author   DenysGeng <cnphp@hotmail.com>
 *
 * @Description //TODO
 * @Version: 1.0.0
 * @Date     2021/9/22
 */

package auth

import (
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// PrivateKey Loads a private and public key from "path" and returns a SSH ClientConfig to authenticate with the server
func PrivateKey(user, keyPath string, keyCallBack ssh.HostKeyCallback) (ssh.ClientConfig, error) {
	privateKey, err := os.ReadFile(keyPath)

	if err != nil {
		return ssh.ClientConfig{}, err
	}

	signer, err := ssh.ParsePrivateKey(privateKey)

	if err != nil {
		return ssh.ClientConfig{}, err
	}

	return ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: keyCallBack,
	}, nil
}

// Creates the configuration for a client that authenticates with a password protected private key
func PrivateKeyWithPassphrase(user, keyPath string, passpharase []byte, keyCallBack ssh.HostKeyCallback) (ssh.ClientConfig, error) {
	// 由于 ioutil.ReadFile 已弃用，从 Go 1.16 开始改用 os.ReadFile
	privateKey, err := os.ReadFile(keyPath)

	if err != nil {
		return ssh.ClientConfig{}, err
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(privateKey, passpharase)

	if err != nil {
		return ssh.ClientConfig{}, err
	}

	return ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: keyCallBack,
	}, nil
}

// Creates a configuration for a client that fetches public-private key from the SSH agent for authentication
func SshAgent(user string, keyCallBack ssh.HostKeyCallback) (ssh.ClientConfig, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return ssh.ClientConfig{}, err
	}

	agentClient := agent.NewClient(conn)
	return ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: keyCallBack,
	}, nil
}

// Creates a configuration for a client that authenticates using username and password
func PasswordKey(user, password string) ssh.AuthMethod {
	if password == "" {
		return ssh.PublicKeys() // 改为公钥认证
	}
	return ssh.Password(password)
}
