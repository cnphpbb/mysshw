# Changelog v26.07.12.0

## Release Info
- **Tag**: v26.07.12.0
- **Date**: 2026-07-12
- **Commits**: 3

---

## Commits

### docs: 重构并扩充中英文README文档
- 简化项目描述，增加Releases链接
- 新增Windows原生（不推荐）详细安装步骤
- 新增完整的环境变量参考表格（主机指纹、密码引用）
- 新增配置文件权限说明和使用示例章节
- 移动架构章节到文档后半部分
- 补充AI友好模式描述
- 统一标点符号和格式

### fix(ssh): 修复 Proxy Jump 第一跳无限递归导致 hang 的 bug
`ssh/proxy.go::dialViaProxyChain` 第一跳调用 `defaultClient.connect()`,
而 `connect()` 内部检查 `node.Proxy` 并递归调用 `dialViaProxyChain`.
`chain[0] = target` 必然带 Proxy, 导致 `chain[0]->chain[0]->...` 无限递归,
最终栈溢出 (超时前表现为 hang).

修复: 第一跳直接 `gossh.Dial`, 跟 `connect()` 非-proxy fallback 同款,
显式跳过 Proxy 分支.

测试: 新增 `ssh/proxy_test.go::TestConnectWithProxy_DoesNotRecurse` 回归测试,
用 accept-立即-close 的 TCP listener 模拟目标, 验证 `connect()` 10s 内返回
错误而非 hang.

### chore: 整理 go.mod 依赖结构
将嵌套的 require 块合并为单一块，添加 mcp-go、go-toml 依赖，并调整 pflag 到主 require 块
