/**
 * @Author   DenysGeng <cnphp@hotmail.com>
 *
 * @Description: Simple scp package to copy files over SSH
 * @Version: 1.0.0
 * @Date     2021/9/22
 */

// Simple scp package to copy files over SSH
package scp

import (
	"time"

	"golang.org/x/crypto/ssh"
)

// Returns a new scp.Client with provided host and ssh.clientConfig
// It has a default timeout of one minute.
func NewClient(host string, config *ssh.ClientConfig) Client {
	return NewConfigurer(host, config).Create()
}

// Returns a new scp.Client with provides host, ssh.ClientConfig and timeout
func NewClientWithTimeout(host string, config *ssh.ClientConfig, timeout time.Duration) Client {
	return NewConfigurer(host, config).Timeout(timeout).Create()
}

// Returns a new scp.Client using an already existing established SSH connection
func NewClientBySSH(ssh *ssh.Client) (Client, error) {
	session, err := ssh.NewSession()
	if err != nil {
		return Client{}, err
	}
	return NewConfigurer("", nil).Session(session).Create(), nil
}

// Same as NewClientWithTimeout but uses an existing SSH client
func NewClientBySSHWithTimeout(ssh *ssh.Client, timeout time.Duration) (Client, error) {
	session, err := ssh.NewSession()
	if err != nil {
		return Client{}, err
	}
	return NewConfigurer("", nil).Session(session).Timeout(timeout).Create(), nil
}
