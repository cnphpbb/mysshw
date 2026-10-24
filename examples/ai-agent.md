# mysshw 与 AI Agent 集成指南

mysshw 是为非交互场景设计的 SSH CLI：所有节点访问可机器可读地调用，
不依赖 TUI 菜单或 stdin 交互。本指南说明如何把 mysshw 当作 AI Agent (Claude、
Cursor、Cline、Aider 等) 的工具使用。

## 1. 快速开始 — Claude Tool Use

`examples/anthropic-tool-use.json` 是 Anthropic Claude 的官方 Tool Use 格式，
包含 4 个工具：`ssh_list_nodes`、`ssh_exec`、`ssh_copy`、`ssh_tunnel`。

### 最小可工作示例 (Python + Anthropic SDK)

```python
import json
import subprocess
import anthropic

# 加载 tool schema
with open("examples/anthropic-tool-use.json") as f:
    TOOLS = [json.load(f)["tools"][i] for i in range(len(json.load(open("examples/anthropic-tool-use.json"))["tools"]))]

client = anthropic.Anthropic()

def call_mysshw(tool_name, tool_input):
    """把 Claude 的 tool call 翻译成 mysshw 命令"""
    if tool_name == "ssh_list_nodes":
        args = ["mysshw", "list", "-o", "json"]
        if "group" in tool_input: args += ["--group", tool_input["group"]]
        if "search" in tool_input: args += ["--search", tool_input["search"]]
    elif tool_name == "ssh_exec":
        args = ["mysshw", "exec", "-o", "json"]
        for n in tool_input.get("node", []): args += ["--node", n]
        if "alias" in tool_input: args += ["--alias", tool_input["alias"]]
        if "host" in tool_input:
            args += ["--host", tool_input["host"]]
            if "user" in tool_input: args += ["-u", tool_input["user"]]
            if "port" in tool_input: args += ["-p", str(tool_input["port"])]
        if tool_input.get("no_pty"): args.append("--no-pty")
        if tool_input.get("dry_run"): args.append("--dry-run")
        args += ["--timeout", tool_input.get("timeout", "30s")]
        if tool_input.get("parallel"): args += ["--parallel", str(tool_input["parallel"])]
        args += ["--", tool_input["command"]]
    elif tool_name == "ssh_copy":
        args = ["mysshw", "copy", "-o", "json"]
        for n in tool_input.get("node", []): args += ["--node", n]
        if "alias" in tool_input: args += ["--alias", tool_input["alias"]]
        args += ["-" + ("U" if tool_input["direction"] == "upload" else "d")]
        args += ["--local", tool_input["local"], "--remote", tool_input["remote"]]
        if "mode" in tool_input and tool_input["direction"] == "upload":
            args += ["--mode", tool_input["mode"]]
    elif tool_name == "ssh_tunnel":
        # tunnel 是阻塞的, 通常不在 AI 单轮调用里跑
        args = ["mysshw", "tunnel"]
        for n in tool_input.get("node", []): args += ["--node", n]
        args += ["--local", tool_input["local_bind"], "--remote", tool_input["remote_addr"]]
    else:
        raise ValueError(f"unknown tool {tool_name}")

    r = subprocess.run(args, capture_output=True, text=True)
    return {"exit_code": r.returncode, "stdout": r.stdout, "stderr": r.stderr}

# Claude 循环
messages = [{"role": "user", "content": "查看我的 Production 集群节点状态"}]
while True:
    resp = client.messages.create(
        model="claude-sonnet-4-5",
        max_tokens=4096,
        tools=TOOLS,
        messages=messages,
    )
    if resp.stop_reason != "tool_use":
        print(resp.content[0].text)
        break
    tool_result = []
    for block in resp.content:
        if block.type == "tool_use":
            result = call_mysshw(block.name, block.input)
            tool_result.append({
                "type": "tool_result",
                "tool_use_id": block.id,
                "content": json.dumps(result, ensure_ascii=False),
            })
    messages.append({"role": "assistant", "content": resp.content})
    messages.append({"role": "user", "content": tool_result})
```

## 2. 核心约定

### 2.1 退出码遵循 BSD sysexits

AI 可直接读退出码分类错误，**无需解析 stderr 文案**。

| 退出码 | 含义 (AI 应对) |
|--------|---------------|
| 0 | 成功 |
| 1 | 部分失败（批量场景）或命令非 0 退出 |
| 2 | 认证失败 — 检查密钥/密码/环境变量 |
| 64 | 参数错误 — 检查 flag 拼写 |
| 66 | 本地文件不存在（copy / sync） |
| 73 | 无法创建本地文件 |
| 74 | IO 错误 |
| 78 | 节点未找到 — 用 list 看可用节点 |
| 124 | 超时 — 增加 --timeout 或排查慢命令 |
| 255 | SSH 协议错误 — 网络或服务端问题 |

### 2.2 list 永不泄露密码

`mysshw list -o json` 的 schema 中**永远不包含** `password` / `passphrase` 字段。
`auth` 字段仅标识认证方式类别（`key` / `password` / `env` / `bcrypt` / `empty`）。

AI 在把节点列表复制到上下文前不需要担心泄露——但 `~/.mysshw.toml` 本身仍含敏感信息，AI 应**避免直接读**该文件。

### 2.3 危险命令的 `--dry-run` 预演

建议默认在生成破坏性命令（`rm`、`kill`、systemctl restart、`dd`、`mkfs` 等）
时**先 dry-run 一次**，再实际执行：

```bash
mysshw exec --dry-run -o json --node prod-1 -- "rm -rf /var/log/*.tmp"
```

返回的 JSON 让 Claude 检查：

- `nodes[]` 是预期的目标吗？
- `auth` 类别是否合理？（`env` 或 `key` 优于 `password`）
- `command` 字符串是否被正确解析为单条命令（避免 shell 注入）
| --download | --local ./backup.sql --remote /var/db/backup.sql |

```

### 2.4 单节点 vs 批量的退出码差异

- **单节点 exec**: `os.Exit(命令退出码)`，透传命令真实 exit code
- **批量 exec**: 任一失败 → exit 1，详情在 JSON `exit_code` 字段

AI 应同时读 `$?` 和 JSON `exit_code`，不要假设 0 即成功。

### 2.5 主机密钥轮换（合法运维 & 防 MITM）

服务器管理员在常规运维中轮换 host key 是合法的。但 **AI 无法区分 "合法轮换" 与 "MITM 攻击"**。

mysshw 提供两层工具：

**A. 单调用 opt-in (应急): `--rotate-known-hosts`**
- `mysshw exec --node prod-1 --rotate-known-hosts -- uptime`
- 当 fingerprint 不匹配时自动 rotate known_hosts (刪旧 + 加新 + stderr warning)
- 等价单次调用的 `StrictHostKeyChecking=no`，但仍写 stdout 警告供 audit

**B. 显式 audit 工具: `mysshw hostkey` 子命令**
- `mysshw hostkey list`               列出所有 known_hosts 条目 (JSON)
- `mysshw hostkey show HOST`          查询某 host 的 fingerprints
- `mysshw hostkey remove HOST`        刪除某 host
- `mysshw hostkey update HOST --node NODE [--confirm] [--expect-fingerprint SHA256:...]`
  拨号连上 NODE, 主动获取 server key, 替换 HOST 的条目
  - 默认 dry-run: 打印 plan, 不写盘
  - `--confirm` 才实际写入
  - `--expect-fingerprint` 进一步验证 (MITM 保护): 与 server 实际指纹不匹配时拒绝写入
- `mysshw hostkey audit [--check-config]`  审查 known_hosts (文件状态 + algorithm 分布 +
  重复 host 警告 + 与 config 跨底对齐). read-only, 不会动任何东西

**AI 推荐用法:**
- **合格制 update** (admin 主动告知新指纹后): AI 调 `hostkey update --expect-fingerprint SHA256:new --confirm`
- **定期审查** (运维自动任务): AI 调 `hostkey audit --check-config` 定期检查老算法 + 缺失 host
- **不要启用** `--rotate-known-hosts` flag 在常规脚本中, 除非 admin 说“可以”

`MYSSHW_KNOWN_HOSTS_UPDATE=always` (永久自更新) 存在项目设计为不实现,以防 AI Agent 丢失防 MITM 能力。

## 3. 不推荐的用法

| 模式 | 理由 |
|------|------|
| `mysshw tunnel` 在 AI 单轮里跑 | tunnel 是阻塞的；AI 无法自然取消；除非用户明确请求 |
| AI 直接读 `~/.mysshw.toml` | 该文件含明文密码或 ${ENV} 变量名；只通过 mysshw 间接访问 |
| 用 `mysshw` (无参数) 进入交互 TUI | 阻塞在 huh.Select，AI 无法驱动 |
| `mysshw sync --upload/--down` | 同步配置到 *单个*远端；不适合分发到多节点（用 `copy`） |

## 4. 集成检查表

把 mysshw 接到一个新的 AI Agent 时，确认：

- [ ] Agent 通过 `subprocess.run(["mysshw", ...])` 调用，捕获 stdout/stderr/returncode
- [ ] 默认走 `-o json` 输出，Agent 用 `json.loads()` 解析
- [ ] 危险命令前自动加 `--dry-run` 二次确认
- [ ] 错误码 (78/124/2 等) 在 Agent 的逻辑分支里区分处理
- [ ] 不让 Agent 接触 `~/.mysshw.toml` 文件路径
- [ ] 批量操作优先用 `--node X --node Y` 重复而不是多次 subprocess 调用
- [ ] AI 输出的 shell 命令在 mysshw 看来是单条命令（`--` 后整段），避免 shell 注入

### 4.1 配置体检（可选的启动前步骤）

mysshw 每次运行都会在 **stderr** 输出配置体检结果（`warn` / `error` 级）。
Agent 如需结构化结果，可显式调用：

```bash
mysshw config check -o json
```

返回 `{"config_path": "...", "ok": true, "counts": {"error": 0, "warn": 1, "info": 2}, "issues": [...]}`

- 退出码 `0` → 无 error 级问题；`1` → 存在 error 级问题
- 建议在自动化流程开头先跑一次，把 `issues[].item` / `issues[].hint` 回传给用户
- 注意：体检结果默认写 stderr，`-o json` 时 stdout 是纯 JSON，解析时不要混用两个流
