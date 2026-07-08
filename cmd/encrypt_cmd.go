package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"mysshw/config"

	"github.com/spf13/cobra"
)

// ============================================================
// encryptCmd 配置文件加密 / 解密 (Phase C 实现)
//
// 用途:
//   - encrypt  把现有 mysshw.toml 加密 (生成新的加密文件)
//   - decrypt  把加密文件解密 (生成明文 TOML)
//
// 安全提示:
//   - encrypt 默认会覆盖原文件 (除非指定 --out)
//   - decrypt 后原文件应删除 (默认不会自动)
//   - passphrase 不要落盘, 推荐用 env 变量或交互输入
// ============================================================

var (
	encryptInput   string // --in       输入文件 (默认 --cfg)
	encryptOutput  string // --out      输出文件 (默认原文件覆盖或 stdout)
	encryptInPlace bool   // --in-place 直接覆盖原文件 (默认 true for encrypt)
	encryptStdout  bool   // --stdout   输出到 stdout (用于管道)
)

var encryptCmd = &cobra.Command{
	Use:   "encrypt",
	Short: "Encrypt mysshw.toml with a passphrase (Argon2id + AES-256-GCM)",
	Long: `Encrypt the mysshw.toml file using a passphrase.

Algorithm (Phase C):
  KDF:    Argon2id (time=3, memory=64MB, threads=4, keylen=32)
  Cipher: AES-256-GCM
  Format: MSEC magic header + kdf_id + salt(16) + nonce(12) + ct_len(4) + ciphertext

Workflow:
  # 1. Encrypt existing config
  mysshw encrypt                    # prompts passphrase, overwrites mysshw.toml
  mysshw encrypt -o encrypted.toml  # write to specific file
  echo "$passphrase" | mysshw encrypt  # passphrase via stdin (scriptable)

  # 2. Use encrypted config (transparent)
  mysshw list -c encrypted.toml    # prompts passphrase on load

  # 3. Decrypt back to plain TOML (e.g. for editing)
  mysshw encrypt decrypt -o plain.toml

Security notes:
  - Passphrase is NOT stored anywhere; lost = data lost
  - Encrypted file alone is safe to share (but don't share passphrase)

Examples:
  mysshw encrypt
  mysshw encrypt -c /etc/mysshw/mysshw.toml
  mysshw encrypt -c in.toml -o out.toml
  mysshw encrypt decrypt -i encrypted.toml -o plain.toml`,
	Args: cobra.NoArgs,
	RunE: runEncrypt,
}

func init() {
	encryptCmd.Flags().StringVar(&encryptInput, "in", "",
		"Input file (default: --cfg from root or ~/.mysshw.toml)")
	encryptCmd.Flags().StringVar(&encryptOutput, "out", "",
		"Output file (default: in-place for encrypt, stdout for decrypt)")
	encryptCmd.Flags().BoolVar(&encryptInPlace, "in-place", true,
		"Overwrite input file in place (encrypt only; default true)")
	encryptCmd.Flags().BoolVar(&encryptStdout, "stdout", false,
		"Write to stdout instead of file (for piping)")

	// 添加 decrypt 子命令
	encryptCmd.AddCommand(encryptDecryptCmd)

	rootCmd.AddCommand(encryptCmd)
}

func runEncrypt(cmd *cobra.Command, args []string) error {
	// 默认: 加密 (无子命令时)
	return runEncryptOrDecrypt(cmd, true)
}

// ==================== Subcommand: decrypt ====================

var encryptDecryptCmd = &cobra.Command{
	Use:   "decrypt",
	Short: "Decrypt an encrypted mysshw.toml to plain TOML",
	Long: `Decrypt a previously encrypted mysshw.toml to plain TOML.

Use case: edit the config (you need plain text to modify), then re-encrypt.

Examples:
  mysshw encrypt decrypt -i encrypted.toml -o plain.toml
  mysshw encrypt decrypt -c ~/.mysshw.toml  # decrypt to stdout`,
	Args: cobra.NoArgs,
	RunE: runDecrypt,
}

func init() {
	// encryptDecryptCmd 也需要 --in / --out / --stdout
	encryptDecryptCmd.Flags().StringVar(&encryptInput, "in", "",
		"Input encrypted file (default: --cfg from root)")
	encryptDecryptCmd.Flags().StringVar(&encryptOutput, "out", "",
		"Output decrypted file (default: stdout)")
	encryptDecryptCmd.Flags().BoolVar(&encryptStdout, "stdout", false,
		"Write to stdout instead of file (for piping)")
}

func runDecrypt(cmd *cobra.Command, args []string) error {
	return runEncryptOrDecrypt(cmd, false)
}

// runEncryptOrDecrypt 加密或解密的共享逻辑
func runEncryptOrDecrypt(cmd *cobra.Command, encryptMode bool) error {
	// 1. 确定输入文件 (优先级: --in > --cfg from root > ~/.mysshw.toml)
	inputPath := encryptInput
	if inputPath == "" {
		// 优先从 rootCmd 的 --cfg flag 取 (cobra 子命令的 cobra.Command.Flags() 不含 parent 的 persistent flag 值)
		if rootCmd != nil {
			if cfgFlag, _ := rootCmd.PersistentFlags().GetString("cfg"); cfgFlag != "" {
				inputPath = cfgFlag
			}
		}
		if inputPath == "" {
			inputPath = GetCtxConfigPath(cmd.Context())
		}
	}

	// 2. 读输入文件
	cipherdata, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("mysshw:: read input %q: %w", inputPath, err)
	}

	// 3. 获取 passphrase (从 stdin)
	passphrase, err := readPassphraseInteractive("Enter passphrase: ")
	if err != nil {
		return err
	}
	if len(passphrase) == 0 {
		return fmt.Errorf("mysshw:: empty passphrase")
	}

	var output []byte
	if encryptMode {
		// 加密: 拒绝重复加密 (避免双重加密)
		if config.IsEncryptedFile(cipherdata) {
			return fmt.Errorf("mysshw:: file %q is already encrypted; decrypt first", inputPath)
		}
		output, err = config.EncryptConfig(cipherdata, []byte(passphrase))
		if err != nil {
			return fmt.Errorf("mysshw:: encrypt failed: %w", err)
		}
	} else {
		// 解密
		if !config.IsEncryptedFile(cipherdata) {
			return fmt.Errorf("mysshw:: file %q is not encrypted (no MSEC magic)", inputPath)
		}
		output, err = config.DecryptConfig(cipherdata, []byte(passphrase))
		if err != nil {
			return fmt.Errorf("mysshw:: decrypt failed (wrong passphrase?): %w", err)
		}
	}

	// 4. 输出
	if encryptStdout {
		_, err = os.Stdout.Write(output)
		return err
	}

	outputPath := encryptOutput
	if outputPath == "" {
		if encryptMode && encryptInPlace {
			outputPath = inputPath
		} else {
			// 默认输出到 stdout
			_, err = os.Stdout.Write(output)
			return err
		}
	}

	// Atomic write: 临时文件 + rename (防半写状态)
	tmpPath := outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, output, 0600); err != nil {
		return fmt.Errorf("mysshw:: write tmp %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("mysshw:: rename %q: %w", outputPath, err)
	}

	verb := "encrypted"
	if !encryptMode {
		verb = "decrypted"
	}
	fmt.Fprintf(os.Stderr, "mysshw:: %s %s (%d bytes -> %d bytes)\n",
		verb, outputPath, len(cipherdata), len(output))
	return nil
}

// readPassphraseInteractive 读取 passphrase (从 stdin)
//
// 注: 真正的 secure input (不回显) 需要 term.ReadPassword, 但为简化
// 依赖, 用 bufio.Scanner。用户可以用管道: echo "pass" | mysshw encrypt
func readPassphraseInteractive(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("mysshw:: read passphrase: %w", err)
		}
		return "", nil
	}
	return strings.TrimRight(scanner.Text(), "\r\n"), nil
}
