# Changelog v26.07.09.0 (2026-07-09)

> 新增 export/import 子命令（配置可移植性）+ 文档同步更新

## ✨ 新功能
- feat: 新增 `export` 子命令 (JSON/TOML 双格式, 密码脱敏)
- feat: 新增 `import` 子命令 (格式自动检测, dry-run 默认)
- feat: `config` 子命令 (CRUD) 标记为已完成
- docs: 补全 rootCmd.Example 涵盖全部 18+ 子命令示例

## 🐛 修复
- (无)

## 🔧 重构 / 🧪 测试 / 📝 文档 / 🏠 构建
- test: 新增 cmd/import_export_test.go (20+ 集成测试)
- docs: TODO.md + AGENTS.md 状态同步更新

## 📊 统计
- 5 个 commit
- 新增 cmd/export_cmd.go + cmd/import_cmd.go + cmd/import_export_test.go
- 修改 cmd/cobra_cmd.go + TODO.md + AGENTS.md
- 新增 20+ 集成测试用例