# Changelog v26.10.25.0

## Release Info
- **Tag**: v26.10.25.0
- **Date**: 2026-10-25
- **Previous**: v26.10.24
- **Commits**: 4（含 2 个 merge）
- **Tag 指向**: `1e283ab` (Merge branch 'gh-dev' into main) — 本 changelog 文件的提交 (`docs: 添加 v26.10.25.0 changelog`) 按 AGENTS.md 流程排在 tag 之后

> 本次发布的两个核心变化：**修复 `sync` 通道忽略 `keypath` 导致握手必然失败**（仅配置私钥的用户 `mysshw sync` 完全不可用），以及**修复主机指纹算法协商与 OpenSSH 不一致**（导致用 OpenSSH 写的 `known_hosts` 校验 mysshw 连接时误报"疑似 MITM"）。

---

## 🐛 修复

### fix: 修复 sync 通道 SSH 认证缺失与主机指纹算法协商不一致 (`2e08798`) ⭐ 本次主要修复

#### 问题 1: sync 通道忽略 keypath，握手阶段无任何认证方式

`cmd/sync_cobra.go::createSSHConfig` 只读取 `[sync.scp].password`，**完全忽略 `keypath` / `passphrase`**。因此仅配置私钥的用户在握手阶段得到：

```
ssh: handshake failed: ssh: unable to authenticate, attempted methods [none], no supported methods remain
```

**根因**：`Auth` 切片为空 → SSH 客户端没有可用的认证方法 → 握手直接失败。

**修改**：
- 新增 `ssh/sync_auth.go`：
  - `LoadKeySigner` — 读取私钥并解析（支持 `~` 展开 / `${ENV}` 口令引用）
  - `AgentAuthMethod` — 通过 SSH Agent 认证（cron / CI 场景，无需交互）
  - `TrustOnFirstUseCallback` / `SyncHostKeyCallback` — 统一 sync 通道的指纹校验策略
- `cmd/sync_cobra.go::createSSHConfig` 签名改为 `(*ClientConfig, cleanup, error)`，认证优先级：

  | 优先级 | 方式 | 配置项 |
  |:---:|------|--------|
  | 1 | 公钥认证 | `[sync.scp].keypath`（私钥口令取 `[sync.scp].passphrase`） |
  | 2 | 密码认证 | `[sync.scp].password`（支持 `${VAR}` / `${VAR:-default}`） |
  | 3 | SSH Agent | 前两者都未配置时自动回退 |

- 一个认证方式都不可用时，返回**可执行的中文错误**（明确列出 `keypath` / `password` / SSH Agent 三条出路），替代晦涩的 SSH 原生报错。

#### 问题 2: 主机指纹算法协商与 OpenSSH 不一致（既有缺陷，主 SSH 路径同样存在）

SSH 握手中，**服务端按客户端提供的算法列表顺序**挑选 host key。而：

- `x/crypto/ssh` 默认列表把 `ssh-ed25519` 放在 **最后**
- OpenSSH 客户端把 `ssh-ed25519` 放在 **最前**

于是同一台服务器：OpenSSH 协商出 ed25519 host key，mysshw 却协商出 ecdsa-sha2-nistp256 → 两者写进 `known_hosts` 的**指纹不同** → 用 OpenSSH 写的 `known_hosts` 校验 mysshw 的连接时必然报"主机指纹与 known_hosts 不匹配"（疑似 MITM 告警，实为算法差异）。

**修改**：
- `ssh/hostkey.go` 新增 `PreferredHostKeyAlgorithms` — 与 OpenSSH 客户端一致的偏好顺序（ed25519 优先，rsa-sha2 系列其次，证书算法兜底）
- `ssh/client.go`（主 SSH 路径）与 `cmd/sync_cobra.go`（sync 路径）**同时**应用该偏好 → 两条路径现在协商出**同一个** host key，共用同一份 `known_hosts`

#### 顺带修复: sync 通道未校验主机指纹

`sync` 通道原先使用 `InsecureIgnoreHostKey()`，备份通道同样可被 MITM 窃取配置。现改为复用 ssh 包的 `known_hosts` TOFU 机制，与普通 SSH 登录一致；`MYSSHW_SKIP_HOST_KEY_CHECK=true` 保留为临时逃生口。

#### 测试

- `cmd/sync_cobra_test.go` 11 个（keypath / keypath+passphrase / env 口令 / 口令错误 / 私钥文件缺失 / keypath+password / 无认证方式 / TOFU 回调）
- `ssh/sync_auth_test.go` 9 个（私钥加载 / 口令 / env 引用 / agent / 指纹回调）
- `ssh/hostkey_algo_test.go` 5 个（算法顺序锁定 ed25519 优先 / 主路径生效 / 返回副本防污染）

#### 验证

- `go build ./...` / `go vet ./...` / `gofmt -l .` 全部干净
- `go test ./... -count=1` → **721 passed**（8 packages）
- 真实环境（tp229.com:65535）端到端：
  - `sync --upload` → 远程文件以 `MSEC` magic 开头（Argon2id + AES-256-GCM 密文），权限 `0600`
  - `sync --down` → 本地配置与上传前完全一致（round-trip OK）
  - 新建 `known_hosts` 条目为 `ssh-ed25519`，指纹与 OpenSSH 写入的**完全相同**

---

## 📝 文档

- `example/mysshw.toml` — `[sync.scp]` 补充认证优先级说明与 `keypath` / `passphrase` 用法
- `readme.md` / `readme.zh.md` — 补充 sync 通道 SSH 认证方式与主机指纹策略说明

---

## 🔀 合并

### merge: 合并 gh-dev（sync 认证 + 指纹算法修复）到 main (`1e283ab`)
### merge: 合并 main 到 gh-dev (`0bdb8ba`)

---

## 📊 统计

- **4 个 commit**（含 2 个 merge，其中 1 个为功能修复）
- **+809 / -28 行** 跨 12 文件
  - 生产代码：**+241 / -11**（5 文件）
  - 测试：**+372 / -7**（3 文件）
  - 文档：**+196 / -10**（4 文件）
- **21 个新测试函数** / 2 个新测试文件
- **全量测试 721 passed**，vet 0 警告，gofmt 干净
- **向后兼容**：✅ 是
  - CLI 既有参数与退出码语义不变
  - 新增能力（keypath / passphrase / SSH Agent 认证）均为**可选**，未配置时行为与之前一致
  - 主机指纹算法偏好变更会改变**协商出的 host key 类型**（从 ecdsa-sha2-nistp256 变为 ssh-ed25519），首次连接会写入新的 `known_hosts` 条目 —— 这是修复本身的目的（与 OpenSSH 对齐）；若旧条目仍在，`known_hosts` 可同时保留多种算法条目，不影响使用
- **推荐升级**：✅ 强烈推荐（仅配私钥的用户当前 `sync` 完全不可用；指纹告警影响所有使用 OpenSSH `known_hosts` 的用户）
