# mysshw 安全规范与漏洞归档

> 版本：v1.0  
> 生成日期：2026-07-06 (北京时间)  
> 审查范围：`/home/cnphpbb/mydata/cnphpbb/6t7net/mysshw/` 全量源码（v26.07.06 系列）  
> 审查方法：静态代码审计 + 配置面建模 + 攻击面分析  
> 适用版本：mysshw ≥ v26.07.06（早于此版本可能未引入 bcrypt / `${ENV}` 支持）  
> 维护人：耿勇（项目作者）

---

## 0. 文档用途

本文档归档 mysshw 项目中已识别的安全漏洞、对应的代码位置、修复策略与协作约定。三类读者：

- **贡献者**：提交 PR 前对照本文档第 3-5 节，确认改动不引入新的高危项。
- **运维人员**：按第 6 节的部署 checklist 落地缓解措施。
- **审计人员**：按第 7 节的验证矩阵快速复核当前代码状态。

发现新的漏洞请直接追加到第 3 节末尾，保持"代码位置 + CWE + 风险 + 修复"四要素齐全。

---

## 1. 风险等级定义

| 等级 | 颜色 | 含义 | SLA |
|---|---|---|---|
| **P0** | 🔴 | 凭据泄露 / 远程认证绕过 / 远程代码执行 | 72 小时内修复或显式接受风险 |
| **P1** | 🟡 | 中危 DoS / 信息泄露 / 弱加密 | 下个 minor 版本修复 |
| **P2** | 🟢 | 低危理论风险 / 代码卫生 | 下一个开发窗口顺手处理 |

---

## 2. 漏洞汇总表

| ID | 等级 | 类型 | 主要组件 | CWE | 修复成本 |
|---|---|---|---|---|---|
| H1 | 🔴 P0 | 凭据明文存储 | `config/` + TOML | CWE-256 / CWE-312 | 中（需批迁脚本） |
| H2 | 🔴 P0 | TOFU 自动信任主机指纹 | `ssh/hostkey.go` | CWE-295 | 低 |
| H3 | 🔴 P0 | `--rotate-known-hosts` 无二次确认 | `ssh/hostkey.go` + `cmd/` | CWE-295 / CWE-640 | 低 |
| M1 | 🟡 P1 | 弱加密算法被显式启用 | `ssh/client.go` | CWE-327 | 极低 |
| M2 | 🟡 P1 | K-I 回调无大小限制（DoS） | `ssh/client.go` | CWE-400 | 低 |
| M3 | 🟡 P1 | K-I 总被追加（钓鱼面） | `ssh/client.go` | CWE-287 | 低 |
| M4 | 🟡 P1 | `clientConfig.Auth` 并发竞态 | `ssh/client.go` | CWE-362 | 低 |
| M5 | 🟡 P1 | sync 把含密码 toml 上行 | `cmd/sync_cobra.go` | CWE-200 | 中 |
| L1 | 🟢 P2 | 内存中 passphrase 残留 | `config/` + `ssh/` | CWE-316 | 低 |
| L2 | 🟢 P2 | 配置路径 `..` 未拒绝 | `config/utils.go` | CWE-22 | 极低 |
| L3 | 🟢 P2 | 配置文件权限检查跨平台不一致 | `config/config.go` | CWE-732 | 极低 |

**统计**：🔴 3 项 / 🟡 5 项 / 🟢 3 项 = 共 11 项。

---

## 3. 高危项（🔴 P0）

### H1 — 配置文件中明文存储密码

**位置**：
- `config/password.go:9-19`（明文兜底逻辑）
- `config/model_config.go:67-139`（`SSHNode.Password` 字段无强制约束）
- 用户实际配置：`~/sbin/mysshw.toml`（71 个节点）

**风险**：

TOML 三种密码格式中，**明文是默认兼容的兜底**：
```toml
password = "MySecretP@ssw0rd"   # 触发明文分支
password = "${MYSSHW_PWD_x}"   # 触发环境变量分支
password = "$2a$10$..."        # 触发 bcrypt 分支
```

`config/password.go:IsBcryptHash` 仅当字符串恰好 60 字符且以 `$2a$` / `$2b$` / `$2y$` 开头时才走 bcrypt。`${ENV}` 需要用户手动写引用。任何不满足上述两条件的内容都被视为**明文**。

威胁放大效应：

- `Work-NodePi-Remote` / `Home-NodePi-Remote` / `gov211-remote` 三个节点经 frp 公网映射对外暴露（参见历史巡检记录）。
- 71 个节点的明文密码集中在单个 toml 文件。
- 攻击者只需获得本机任意读权限（git 误传、备份、同步到 webdav/s3、磁盘失窃），即可一次性获取全部 root 凭据。

**修复策略**（按优先级）：

1. **立即**：把所有 71 个节点密码从明文迁到 `${MYSSHW_PWD_xxx}` env 引用，或使用 `mysshw init` 的 bcrypt 路径。
2. **强制**：在 `cmd/init_cmd.go` 写 TOML 时增加 lint：检测到 password 既不是 `${...}` 也不是 `$2*$` 开头时，**默认强制 bcrypt**，需 `--allow-plain` 显式 opt-in 才允许明文写入。
3. **长期**：对接系统 keyring（macOS Keychain / Linux `secret-tool` / Windows Credential Manager）。

---

### H2 — TOFU 在非交互模式下自动信任主机指纹

**位置**：`ssh/hostkey.go:160-174`

**风险**：

```go
if len(keyErr.Want) == 0 {
    if isTerminal(os.Stdin) {
        return promptAndAddHost(path, hostname, remote, key)
    }
    // 非交互场景：自动 trust  ← 风险点
    if err := appendKnownHost(path, hostname, key); err != nil {
        return fmt.Errorf(...)
    }
    return nil
}
```

触发条件：`stdin` 不是 TTY 时（管道 / CI / AI agent / 重定向），任何**首次连接的新主机自动信任**并写入 known_hosts。

攻击场景：

- 内网或中转节点 ARP / DNS 劫持，将 `192.168.100.184`（Work-NodePi-LanRoot）的 SSH 流量劫持到攻击者机器。
- cron / AI agent 跑 `mysshw exec --node ...` 时 stdin 是管道，**自动信任攻击者伪造的 host key**。
- known_hosts 被写入攻击者 key，**之后所有连接都走攻击者**（无任何告警）。
- 与 2026-06-17 categraf ibex 拉取主机巡检任务导致树莓派 crash 的攻击面属于同类（LAN 内低权限中间人 + 默认信任新主机）。

**修复策略**：

1. 对 `StrictHostKeyChecking` 显式 `true` 的节点，**禁止非交互 auto-trust**（只允许 TTY 提示；非 TTY 直接报错）。
2. 增加 `--yes-trust-new-host` flag 显式 opt-in（默认禁止）。
3. 将 "auto-trusted" 事件**强制写入独立 audit log**（不被覆盖，带 SHA256 fingerprint + UTC 时间戳），便于事后追溯。
4. 提供 `mysshw hostkey scan` 子命令批量预录入已知节点的指纹到 known_hosts，使首次连接即走严格校验。

---

### H3 — `--rotate-known-hosts` 无二次确认

**位置**：
- `cmd/exec_cmd.go:106-110`
- `cmd/copy_cmd.go:104-105`
- `ssh/hostkey.go:177-197`

**风险**：

```
mysshw exec --rotate-known-hosts --node X -- <cmd>
```

代码注释自己标注了 ⚠️ "无法区分 MITM 与合法轮换"，但实际**没有任何二次确认**：

```go
if rotateOnMismatch {
    // 直接删除旧 key + 写入新 key
    return nil // 接受新 key
}
```

攻击场景：

- 任何能影响 stdin 的进程（脚本注入、CI 模板污染、AI prompt injection）都可塞入 `--rotate-known-hosts`。
- `exec` / `copy` 子命令专为 AI agent 设计（JSON 输出、`--dry-run`、节点自动匹配），**恰好是 AI 容易生成 `mysshw exec --rotate-known-hosts ...` 的场景**。
- 一旦执行，known_hosts 被覆写，**之后撤掉 flag 也无法恢复**（默认严格校验会用新 key 校验，而新 key 就是攻击者的）。
- 71 节点批量任务下，**一条命令可污染所有节点指纹**。

**修复策略**：

1. **强制校验**：要求 `--rotate-known-hosts --expected-fingerprint <SHA256:xxxx>` 同时出现，匹配才允许 rotate。
2. **作用域隔离**：rotate 结果写入独立的 `known_hosts.pending` 文件，由 `mysshw hostkey approve` 显式合入，不污染全局。
3. **环境变量门禁**：`MYSSHW_ALLOW_UNVERIFIED_ROTATE=1` 必须显式 export 才允许该 flag 生效（防 AI 静默调用）。

---

## 4. 中危项（🟡 P1）

### M1 — 弱加密算法被显式列入支持列表

**位置**：`ssh/client.go:25-42`

```go
DefaultCiphers = []string{
    "aes128-ctr", "aes192-ctr", "aes256-ctr",
    "aes128-gcm@openssh.com",
    "chacha20-poly1305@openssh.com",
    "arcfour256", "arcfour128", "arcfour",       // ← RC4，已不安全
    "aes128-cbc", "3des-cbc", "blowfish-cbc", "cast128-cbc",  // ← CBC / 3DES
    "aes192-cbc", "aes256-cbc",
}
```

**修复**：仅保留 CTR / GCM / ChaCha20，删除 RC4、3DES、CBC 全系。

---

### M2 — Keyboard-Interactive 回调无大小限制（DoS）

**位置**：`ssh/client.go:193-216`

```go
for i, q := range questions {
    if echos[i] {
        scan := bufio.NewScanner(os.Stdin)   // ← 默认 64KB 上限
        ...
    } else {
        b, err := term.ReadPassword(int(syscall.Stdin))   // ← 无大小限制
        ...
    }
}
```

混用 `bufio.Scanner`（默认 64KB 上限）和 `term.ReadPassword`（无限制），行为不一致。恶意服务端可发送超大 password prompt，触发内存膨胀。

**修复**：统一使用 `term.ReadPassword`，加超时；或显式 `scan.Buffer(buf, maxLen)`。

---

### M3 — Keyboard-Interactive 总被追加（钓鱼面）

**位置**：`ssh/client.go:193`

无论节点是否声明 keyboard-interactive 需求，**都注册该回调**。攻击者可主动要求 K-I（SSH 协议层允许），弹任意问题（伪装的 prompt），诱导用户输入密码到攻击者控制的 channel。

**修复**：从 server 发起 K-I 请求时，先弹"非预期 K-I 询问，要继续吗？"（TTY）；或干脆禁用 K-I 走纯 password/key。

---

### M4 — `clientConfig.Auth` 并发竞态

**位置**：`ssh/client.go:310-339`

```go
if strings.Contains(msg, "no supported methods remain") && !strings.Contains(msg, "password") {
    // 读密码后直接修改 c.clientConfig.Auth
    c.clientConfig.Auth = append(c.clientConfig.Auth, ssh.Password(p))
    ...
}
```

`clientConfig.Auth` 在并发调用同一 SSHNode 时是共享 slice，`append` 不加锁会触发 race（`go test -race` 应能复现）。

**修复**：用 mutex 保护 `clientConfig`；或在每次重试时 copy 一份独立的 ClientConfig。

---

### M5 — sync 把含密码 toml 上行

**位置**：
- `config/model_config.go:82-107`
- `cmd/sync_cobra.go`

`sync.type` 支持 `scp / webdav / s3` 三种备份方式，**toml 文件本身（含 71 节点密码）会被推到远程**。即便用户把密码改成 `${ENV}`，sync 凭证（webdav / s3 的 password / access_key）又是新的明文。

**修复**：

- sync 凭证强制走 env（目前支持，但默认模板 `${MYSSHW_PWD_webdav}` 易被覆写）。
- sync 上行前做 dry-run，提示"将同步 N 字节到远程，其中含 password / keypath 字段"。
- sync 上行前**自动 strip password 字段**（只同步节点结构 + 公钥），下行时合并。

---

## 5. 低危项（🟢 P2）

### L1 — 内存中 passphrase 残留

**位置**：`config/model_config.go:99`、`ssh/client.go:158`

```go
passphrase := ResolveEnvRef(node.Passphrase)
signer, err = ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(passphrase))
```

Go string 不可变，转 `[]byte` 后底层共享 backing array。`passphrase` 这条 string 在 GC 前一直驻留内存（可被 heap dump 抓到）。

**修复**：用 `[]byte` 字段 + 显式 `runtime.KeepAlive` + `memclr`；根本解决是迁到 env。

---

### L2 — 配置路径 `..` 未拒绝

**位置**：`config/utils.go:51-80`

`cfgPath = "../../etc/passwd"` 会被 `path.Join` 规范化。理论上能被 `mysshw --cfg ../../xxx.toml` 利用，但限制是路径里必须含 `mysshw` 子串（`config.go:92`），实际影响很小。`viper.AddConfigPath("$HOME/")` + `viper.AddConfigPath("./")` 多位置查找，行为有歧义。

**修复**：显式拒绝 `..`，只接受 `~` / `/` / `./` 三种前缀。

---

### L3 — 配置文件权限检查跨平台不一致

**位置**：`config/config.go:73` 调用 `secureConfigFile(CFG_PATH)`

进程以 root 跑（`umask` 022）时 `os.Chmod 0600` 会成功；macOS / Windows 行为不同（Windows NTFS 无 POSIX 权限位）。

**修复**：加载时检测 `stat.Mode().Perm()`，不是 0600 直接拒绝并提示 `chmod 600 ~/.mysshw.toml`。

---

## 6. 运维缓解 checklist

下列步骤不依赖代码改动，可立即落地：

- [ ] **[H1]** 把所有 71 个节点密码从明文迁到 `${MYSSHW_PWD_xxx}` env 引用，env 写入 `~/.hermes/env`（不写入 shell rc 文件）。
- [ ] **[H1]** 备份旧 toml 到只读路径：`chmod 400 ~/sbin/mysshw.toml.backup-20260706`。
- [ ] **[H2]** 对所有外网可达的 frp 节点（`Work-NodePi-Remote` / `Home-NodePi-Remote` / `gov211-remote`），**预先生成 known_hosts 条目**，禁用 TOFU auto-trust。
- [ ] **[H2]** 在 CI / cron 脚本中显式 `export MYSSHW_REQUIRE_TTY=1`（待 H2 修复后生效）。
- [ ] **[H3]** 在 AI agent 模板中禁用 `--rotate-known-hosts` flag（grep + 黑名单）。
- [ ] **[M5]** 关闭 toml 的自动 sync 推送，手动触发；sync 前 diff 内容确认无 password 字段。
- [ ] **[L3]** 立即执行 `chmod 600 ~/.mysshw.toml` 和 `chmod 700 ~/.ssh/known_hosts`。

---

## 7. 验证矩阵

提交 PR 时跑下列命令确认无回归：

```bash
# 静态扫描：新增代码不得引入已知危险模式
git diff --cached | grep "^+" | grep -iE "(api_key|secret|password|token)\s*=\s*['\"][^'\"]+['\"]"
git diff --cached | grep "^+" | grep -E "InsecureIgnoreHostKey|shell=True|eval\(|exec\("

# 单元测试（含 race detector）
go test ./... -race -count=1

# 编译期检查 cipher 列表变更
go build ./... && go vet ./...

# bcrypt / ${ENV} 路径覆盖（已有测试）
go test ./config/... -run "Bcrypt|Env|Hash" -v
go test ./ssh/... -run "Client|HostKey|TOFU" -v
```

每个漏洞修复时应在第 3 / 4 / 5 节末尾追加"**修复状态**"行（含 commit hash + PR 号），便于追溯。

---

## 8. 修订记录

| 日期 (北京) | 版本 | 修订内容 | 作者 |
|---|---|---|---|
| 2026-07-06 | v1.0 | 首次生成；基于 v26.07.06 系列源码静态审计 | Hermes + 耿勇 |

---

## 9. 参考

- OWASP Top 10（2021）
- CWE/SANS Top 25 Most Dangerous Software Weaknesses
- Go Security Best Practices（golang.org/x/crypto/ssh 文档）
- OpenSSH Manual：`StrictHostKeyChecking` / `UserKnownHostsFile`