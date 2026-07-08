package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// readPassphraseForDecrypt 读取解密用的 passphrase
//
// 行为:
//   - 从 stdin 读一行 (trim 末尾 \n)
//   - 非交互场景 (CI/管道) 直接读 stdin
//   - 交互场景 (terminal) 仍走 stdin, 但用户应使用 pipe
//
// 注: 真正的 secure input (不回显) 需要 term.ReadPassword, 但为避免
// 在 config 包引入 syscall/term 依赖, 简化用 bufio.Scanner。
// 安全要求高的场景: 用环境变量 / pipe 输入。
func readPassphraseForDecrypt() (string, error) {
	fmt.Fprintln(os.Stderr, "mysshw:: config file is encrypted, enter passphrase:")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("mysshw:: read passphrase: %w", err)
		}
		return "", nil
	}
	return strings.TrimRight(scanner.Text(), "\r\n"), nil
}

// readPassphraseEncryptVariant 保留以备未来扩展 (e.g. 确认输入)
// 当前未使用, 留作 placeholder
func readPassphraseEncryptVariant(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", nil
	}
	return strings.TrimRight(scanner.Text(), "\r\n"), nil
}
