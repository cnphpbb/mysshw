# Changelog v26.07.08.0 (2026-07-08)

> 修复 `config` 子命令的 `--cfg` flag 读取、`--confirm` flag 缺失、以及测试基础设施 Bug。

## ✨ 新功能
- feat(config): 文件级加密 Phase C — Argon2id + AES-256-GCM (#?)
- feat(cmd): 新增 hash 子命令 (gen/verify/pepper), Phase B 配套工具 (#?)
- feat(ssh): 实现 bcrypt-pepper 验证 (Phase B 核心) (#?)
- feat(config): 增加 encryption_algorithm / secret_key / cipher / kdf_params 字段 (forward-compatible) (#?)
- feat(ssh): exec --follow/--stream 支持长命令流式输出 (#?)
- feat(cmd): 新增 info 子命令, 远程节点系统信息收集 (AI 决策前置) (#?)
- feat(cmd): 新增 ping 子命令, TCP 端口连通性检测 (AI 友好) (#?)
- feat(cmd): 新增 mcp 子命令, 通过 MCP (stdio) 暴露 SSH 操作为 AI 工具 (#?)
- feat(cmd): 新增 completions 子命令, 动态输出 CLI Schema (JSON) (#?)
- feat(ssh): --node 参数支持同时匹配 Name 和 Alias 字段 (#?)

## 🐛 修复
- fix(config): loadOrCreateConfig 正确读取 --cfg flag (#?)
- fix(config): configAddCmd 补全缺失的 --confirm flag 注册 (#?)
- fix(config): runConfigRemove 删除最后一个节点后清理空组 (#?)
- fix(config): ValidateConfig 允许 0 个节点 (空配置合法) (#?)

## 🧪 测试
- fix(test): TestMain 使用 runtime.Caller(0) 定位项目根目录, 修复 go test ./cmd/ 构建错误二进制的问题 (#?)

## 📝 文档
- docs: 同步 Phase B/C 文档 (AGENTS / TODO / example) (#?)
- docs(example): 演示 4 种 password 形式 (#?)
- docs(example): 同步 example/mysshw.toml 与最新 schema (#?)
- docs(examples): 新增 mcp-integration.md 完整 MCP 集成指南 (#?)
- docs(security): 新增 SECURITY.md 安全规范 (#?)
- docs(todo): 更新优先级总览，标记已完成功能 (#?)

## 🔧 构建
- go.mod: pelletier/go-toml/v2 从 indirect 提升为 direct (#?)
