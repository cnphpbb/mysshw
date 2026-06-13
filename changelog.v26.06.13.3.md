# Changelog v26.06.13.3 (2026-06-13)

> 用户提议的 4 个新功能: 备份机制 / 密码多格式 / 交互输入 / 首次运行向导 + What's New。完整兼容老版本。

## ✨ 新功能

- 🛡️ **密码 bcrypt 哈希存储** — 替代明文密码存储。TOML 存 `$2a$10$...` (60 字符), 运行时 term.ReadPassword 交互输入明文并 VerifyPassword 验证
- ⌨️ **空密码 = 交互输入** — TOML 留空 `password = ""` 时, 每次 SSH 连接前用 term.ReadPassword 提示输入, **不写回 TOML**。用户每次手动输入
- 📦 **TOML 本地备份机制** — 写 TOML 时自动备份已存在文件为 `<name>.<YYYYMMDD_HHMMSS>.bak`, 按 `backup_keep` (默认 5) 数量裁剪
- 🧙 **首次运行向导** (`mysshw init`) — huh 驱动的 4 步交互: 同步类型 → SSH 节点 → 认证方式 → 预览确认。生成 TOML 到默认位置, 提示用户退出后重新启动
- 📢 **What's New 页面** — 新用户首次启动自动显示本次版本主要更新清单 (TOFU / bcrypt / Docker / 备份等); 升级用户首次运行显示一次后写入 `~/.mysshw.lastseen`

## 🔐 安全

- **决策 B1 自动检测** — `password` 字段支持 4 种格式自动识别: 明文 / `${ENV}` / bcrypt 哈希 / 空字符串
- **决策 A1 向后兼容** — 老版本 TOML 仍可工作, 进程级 `sync.Once` 一次性提示用户改用更安全方式
- **决策 D2 备份配置** — `backup_keep` TOML 字段, 用户可自定义 (默认 5)

## 🏠 配置管理

- 新增 `Configs.BackupKeep int` 字段 (toml: `backup_keep,omitempty`, 默认 5)
- 新增 `config.DefaultBackupKeep = 5` 常量
- 新增 `config.WriteTomlWithBackup(path, data) error` (导出) — 原子写 + 自动备份 + prune
- 新增 `config.pruneBackups(path, keep int) error` — 按字母序 (= 时间序) 删除最旧
- 新增 `config.effectiveBackupKeep() int` — 根据全局 CFG 解析 (0/负 → 5)
- `cmd/yml_cmd.go::appendToConfigFile` 行为变更: 追加模式 → read-merge-rewrite (触发自动备份)

## 🔑 密码管理 (新文件 `config/password.go`)

- `IsBcryptHash(s) bool` — 长度 60 + 前缀 `$2a$/$2b$/$2y$` 自动检测
- `IsEnvReference(s) bool` — `${...}` 格式检测
- `HashPassword(plain) (string, error)` — bcrypt.DefaultCost (10) 哈希
- `VerifyPassword(hash, plain) error` — 验证明文与哈希匹配
- `BcryptHashPrefixes` / `BcryptHashLength` 常量

## ⌨️ SSH 客户端 (新文件 helpers in `ssh/client.go`)

- `resolvePasswordForSSH(node, resolvedPwd) (AuthMethod, cleartext)` — 三分支决策:
  1. bcrypt 哈希 → term.ReadPassword 交互输入 → VerifyPassword, 失败返回 nil
  2. 明文 → 直接使用, 进程级 sync.Once 一次性提示迁移
  3. 空字符串 → term.ReadPassword 交互输入, **不写回**
- `promptPassword(node, prompt) (string, error)` — terminal 用 term.ReadPassword; 非 terminal 降级 fmt.Scanln (CI/管道)
- `isTerminal(f *os.File) bool` — ModeCharDevice 检查
- `isTerminalStdout() bool` — x/term.IsTerminal (cmd 包用)

## 🧙 向导 (新文件 `cmd/init_cmd.go`)

- `initCmd` cobra 子命令 `mysshw init [--force]`
- `runInitWizard(force bool) error` 主流程
- `runWizardSteps(cfgPath) (*Configs, error)` 4 步 huh 向导
- `previewNode(group, node)` 生成节点预览
- `marshalConfigToToml(cfg) ([]byte, error)` 字符串拼接序列化 (避免新依赖)
- `showWhatsNew()` 打印本次版本主要更新清单
- `IsFirstRun() bool` / `IsUpgradedFromOlder() bool` 检测
- `markVersionSeen() error` 写 `~/.mysshw.lastseen` (0600)

`cmd/cobra_cmd.go` 集成:
- 首次运行 (stdout 是 terminal) → 自动跳向导
- 升级运行 (TOML 存在但 lastseen 旧) → 仅显示 What's New, 更新标记

## 🧪 测试 (本版本新增 22+ 用例)

| 文件 | 用例 | 覆盖 |
|---|---|---|
| `config/backup_config_test.go` (扩充) | 7 | WriteTomlWithBackup (新/存在/裁剪) + PruneBackups + EffectiveBackupKeep |
| `config/password_test.go` (新) | 14 | IsBcryptHash (12 子用例) + IsEnvReference (8) + HashPassword/Verify + Salt 随机性 |
| `ssh/client_password_test.go` (新) | 7 | 三分支 (空/bcrypt/明文/失败) + isTerminal + zeroBytes |
| `cmd/init_cmd_test.go` (新) | 8 | markVersionSeen + IsUpgradedFromOlder (3) + IsFirstRun (2) + isTerminal + itoaPtr |

## 🔗 兼容性 (用户决定 1)

| 场景 | 行为 |
|---|---|
| 旧版明文密码 `password = "secret"` | ✅ 仍可工作, 一次性提示迁移 |
| 旧版 `${ENV}` 引用 | ✅ 不变 (ResolveEnvRef 仍先处理) |
| 旧版 TOML 无 `backup_keep` 字段 | ✅ 默认 5 (effectiveBackupKeep) |
| 旧版 yml 命令 | ✅ 改用 WriteTomlWithBackup, 触发备份 |
| 旧版 `appendToConfigFile` 调用方 | ✅ 不变 (只改实现) |

## 📊 统计

- **5 个新 commit** (自 v26.06.13.2): +1109 / -22 行
- **新文件**: 4 (`config/password.go`、`config/password_test.go`、`ssh/client_password_test.go`、`cmd/init_cmd.go`、`cmd/init_cmd_test.go` — 实际 5 个)
- **修改**: 4 (`config/model_config.go`、`config/backup_config.go`、`config/backup_config_test.go`、`cmd/yml_cmd.go`、`cmd/cobra_cmd.go` — 实际 5 个)
- **新测试用例**: ~36 个 (累计 ~150+)
- **远端 tags 累计**: 4 个 (v26.06.13 / v26.06.13.1 / v26.06.13.2 / **v26.06.13.3**)

## 🔗 完整 commit 列表

```
522ac2f feat(cmd): 首次运行向导 + What's New 页面 (用户决定 4)
f81fc14 feat(ssh): 交互输入密码 (空 + bcrypt 三分支决策)
515d531 feat(config): 密码字段多格式支持 (bcrypt 自动检测)
d9ef0c0 feat(config): 备份机制 (BackupKeep 字段 + writeTomlWithBackup + pruneBackups)
6c990e7 docs: 全面重写 TODO.md / readme (中英) + analysis.md 本地重写
```

## 🔼 升级指引 (从 v26.06.13.2 升级)

无破坏性变更。直接拉取新代码即可。首次运行会自动显示 What's New; 也可手动 `mysshw init` 重新生成配置。

**建议**: 现有明文密码可在新版本首次运行时一次性迁移为 bcrypt 哈希 (按提示回答 y 即可)。如不希望迁移, 选 "N" 保持现状 (空密码 + 每次交互输入也是更安全的方式)。

## 📦 Docker 使用

```bash
docker build -t mysshw:v26.06.13.3 \
  --build-arg VERSION=v26.06.13.3 .
docker run --rm -it \
  -v $PWD/mysshw.toml:/home/mysshw/.mysshw.toml:ro \
  mysshw:v26.06.13.3 init
```
