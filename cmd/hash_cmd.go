package cmd

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"mysshw/config"

	"github.com/spf13/cobra"
)

// ============================================================
// hashCmd 密码哈希 / secret_key 生成工具 (Phase B 引入)
//
// 用途:
//   - 生成节点 password 字段的 bcrypt 哈希 (替代明文落盘)
//   - 生成 bcrypt-pepper 模式下的 pepper / secret_key
//   - 验证已有 hash 是否匹配明文 (用于审计)
//
// 子命令:
//   gen       生成 bcrypt / bcrypt-pepper / argon2id / pbkdf2-sha256 哈希
//   verify    验证 hash 是否匹配明文 (从 stdin 读取明文, 不 echo)
//   pepper    生成新的随机 secret_key (32 字节, base64)
// ============================================================

// 通用 flags (subcommand 间共享)
var (
	hashAlgo   string // --algo: bcrypt | bcrypt-pepper | argon2id | pbkdf2-sha256
	hashPepper string // --pepper: server 侧 secret (仅 bcrypt-pepper)
	hashCost   int    // --cost: bcrypt cost factor (默认 10)
	hashStdout bool   // --stdout: 输出到 stdout (默认 stderr, 便于管道)
)

var hashCmd = &cobra.Command{
	Use:   "hash <subcommand> [args]",
	Short: "Password hashing utilities: gen / verify / pepper",
	Long: `Generate and verify password hashes for use in mysshw.toml.

Subcommands:
  gen <plain>       Generate a bcrypt hash (or with --algo specify algorithm)
  verify <hash>     Verify a hash matches a plaintext password (read from stdin)
  pepper            Generate a new random secret_key (32 bytes, base64)

Examples:
  # Generate bcrypt hash
  mysshw hash gen 'my-password'
  # Output: $2a$10$xxxxx...

  # Generate bcrypt-pepper hash (server 侧 secret 拼到 hash 里)
  mysshw hash gen --algo=bcrypt-pepper --pepper='my-server-secret' 'plaintext'

  # Verify a hash
  echo -n 'my-password' | mysshw hash verify '$2a$10$xxx...'
  echo -n 'my-password' | mysshw hash verify '$2a$10$xxx...' --pepper='my-secret'
  # Exit 0 if match, exit 1 if not

  # Generate a new secret_key (paste into mysshw.toml)
  mysshw hash pepper
  export MYSSHW_SECRET_KEY=$(mysshw hash pepper)

Common workflow:
  1. Generate pepper:  mysshw hash pepper > /tmp/pepper
  2. Generate hash:    mysshw hash gen --algo=bcrypt-pepper --pepper="$(cat /tmp/pepper)" 'real-pwd' > /tmp/hash
  3. Paste into TOML:  password = "$(cat /tmp/hash)"
                        secret_key = "$(cat /tmp/pepper)"
  4. Test:             mysshw exec --node foo -- 'echo OK'

Algorithm support (Phase B):
  bcrypt (default)        Standard bcrypt, $2a$10$...        [implemented]
  bcrypt-pepper           bcrypt + secret_key                  [implemented]
  argon2id                $argon2id$...                        [forward-compat: fallback to bcrypt]
  pbkdf2-sha256           $pbkdf2-sha256$...                   [forward-compat: fallback to bcrypt]`,
	Args: cobra.NoArgs,
}

func init() {
	// 通用 flags
	hashCmd.PersistentFlags().StringVar(&hashAlgo, "algo", "bcrypt",
		"Hash algorithm: bcrypt | bcrypt-pepper | argon2id | pbkdf2-sha256")
	hashCmd.PersistentFlags().StringVar(&hashPepper, "pepper", "",
		"Server-side secret for bcrypt-pepper mode")
	hashCmd.PersistentFlags().IntVar(&hashCost, "cost", 10,
		"bcrypt cost factor (default 10, higher = slower + more secure)")
	hashCmd.PersistentFlags().BoolVar(&hashStdout, "stdout", false,
		"Output to stdout (default: stderr). Useful for piping: $(mysshw hash gen ...)")

	// 注册 subcommands
	hashCmd.AddCommand(hashGenCmd, hashVerifyCmd, hashPepperCmd)

	rootCmd.AddCommand(hashCmd)
}

// outputWriter 返回应该写输出的 io.Writer
// 默认 stderr (避免污染 stdout 管道), --stdout 切到 stdout
func outputWriter() io.Writer {
	if hashStdout {
		return os.Stdout
	}
	return os.Stderr
}

// outputLine 写入一行 + 换行
func outputLine(format string, args ...any) {
	fmt.Fprintf(outputWriter(), format+"\n", args...)
}

// ==================== Subcommand: gen ====================

var hashGenCmd = &cobra.Command{
	Use:   "gen <plaintext>",
	Short: "Generate a password hash",
	Long: `Generate a password hash for use in mysshw.toml password fields.

The plaintext password is read from the command line argument (or stdin if
argument is "-"). Output is the hash string, suitable for direct paste into
the mysshw.toml file.

Examples:
  mysshw hash gen 'my-password'
  echo -n 'my-password' | mysshw hash gen -
  mysshw hash gen --algo=bcrypt-pepper --pepper='my-secret' 'plaintext'
  mysshw hash gen --cost=12 'my-password'  # higher cost = slower + more secure`,
	Args: cobra.ExactArgs(1),
	RunE: runHashGen,
}

func runHashGen(cmd *cobra.Command, args []string) error {
	plain, err := readHashInput(args[0])
	if err != nil {
		return err
	}

	switch hashAlgo {
	case "bcrypt":
		hash, err := config.HashPassword(plain)
		if err != nil {
			return fmt.Errorf("mysshw:: bcrypt gen failed: %w", err)
		}
		outputLine("%s", hash)

	case "bcrypt-pepper":
		if hashPepper == "" {
			return fmt.Errorf("mysshw:: bcrypt-pepper requires --pepper flag")
		}
		hash, err := config.HashPasswordWithPepper(plain, hashPepper)
		if err != nil {
			return fmt.Errorf("mysshw:: bcrypt-pepper gen failed: %w", err)
		}
		outputLine("%s", hash)

	case "argon2id", "pbkdf2-sha256":
		// Phase B+ 占位: 当前退化为标准 bcrypt
		// 实际 argon2id / pbkdf2 实现见 Phase C
		hash, err := config.HashPassword(plain)
		if err != nil {
			return fmt.Errorf("mysshw:: %s fallback gen failed: %w", hashAlgo, err)
		}
		fmt.Fprintf(os.Stderr,
			"mysshw:: WARNING: %s not yet implemented, falling back to bcrypt (Phase B+)\n", hashAlgo)
		outputLine("%s", hash)

	default:
		return fmt.Errorf("mysshw:: unknown --algo %q (supported: bcrypt, bcrypt-pepper, argon2id, pbkdf2-sha256)", hashAlgo)
	}
	return nil
}

// ==================== Subcommand: verify ====================

var hashVerifyCmd = &cobra.Command{
	Use:   "verify <hash>",
	Short: "Verify a hash matches a plaintext password (from stdin)",
	Long: `Verify that a hash matches a plaintext password.

Reads plaintext from stdin (no echo). Compares with the provided hash using
the appropriate algorithm. Exits 0 on match, exit 1 on mismatch.

Examples:
  echo -n 'my-password' | mysshw hash verify '$2a$10$xxx...'
  echo -n 'my-password' | mysshw hash verify '$2a$10$xxx...' --pepper='my-secret'

The --algo flag is currently informational only: the hash format itself
determines the algorithm. --pepper is required for bcrypt-pepper hashes.`,
	Args: cobra.ExactArgs(1),
	RunE: runHashVerify,
}

func runHashVerify(cmd *cobra.Command, args []string) error {
	hash := args[0]
	plain, err := readHashInput("-")
	if err != nil {
		return err
	}

	// 自动检测算法
	if config.IsArgon2idHash(hash) || config.IsPbkdf2Sha256Hash(hash) {
		return fmt.Errorf("mysshw:: %s verify not yet implemented (Phase B+)", algoFromHash(hash))
	}

	if !config.IsBcryptHash(hash) {
		return fmt.Errorf("mysshw:: invalid hash format: %q", hash)
	}

	var verifyErr error
	if hashPepper != "" {
		// 显式指定 pepper → bcrypt-pepper 验证
		verifyErr = config.VerifyPasswordWithPepper(hash, plain, hashPepper)
	} else if config.IsPepperRequired(hashAlgo) {
		// --algo=bcrypt-pepper 但未提供 --pepper
		return fmt.Errorf("mysshw:: --algo=bcrypt-pepper requires --pepper flag")
	} else {
		// 标准 bcrypt
		verifyErr = config.VerifyPassword(hash, plain)
	}

	if verifyErr != nil {
		return fmt.Errorf("mysshw:: verify FAILED: %w", verifyErr)
	}
	outputLine("mysshw:: verify OK (hash matches plaintext)")
	return nil
}

// ==================== Subcommand: pepper ====================

var hashPepperCmd = &cobra.Command{
	Use:   "pepper",
	Short: "Generate a new random secret_key (pepper) for bcrypt-pepper mode",
	Long: `Generate a cryptographically random 32-byte secret suitable for use as
a bcrypt-pepper secret_key.

The output is base64-encoded (44 characters). Paste it into mysshw.toml:

  [top-level]
  secret_key = "<output-of-this-command>"

Or reference an env var for portability:
  secret_key = "${MYSSHW_SECRET_KEY}"

The output is sent to stdout (not stderr) so it can be captured in shell:
  export MYSSHW_SECRET_KEY=$(mysshw hash pepper)`,
	Args: cobra.NoArgs,
	RunE: runHashPepper,
}

func runHashPepper(cmd *cobra.Command, args []string) error {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("mysshw:: random read failed: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(buf)
	// 强制输出到 stdout (便于 shell 捕获: export X=$(...))
	fmt.Println(encoded)
	return nil
}

// ==================== 内部 helper ====================

// readHashInput 从 arg 或 stdin 读取明文
// arg == "-" 时从 stdin 读
func readHashInput(arg string) (string, error) {
	if arg != "-" {
		return arg, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("mysshw:: read stdin: %w", err)
	}
	// 去除尾部换行
	return strings.TrimRight(string(data), "\r\n"), nil
}

// algoFromHash 从 hash 字符串提取算法名
func algoFromHash(hash string) string {
	if config.IsArgon2idHash(hash) {
		return "argon2id"
	}
	if config.IsPbkdf2Sha256Hash(hash) {
		return "pbkdf2-sha256"
	}
	if config.IsBcryptHash(hash) {
		return "bcrypt"
	}
	return "unknown"
}

// bufio.NewReader is used by readHashInput to wrap os.Stdin if needed in future
var _ = bufio.NewReader
