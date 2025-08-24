# 变更文档 (v25.08.24 - V25.08.25)

## 主要变更

### 1. 代码结构重构
- 将功能模块拆分到独立文件，提高代码可维护性
- 重构配置模块，分离模型定义到单独文件
- 优化命令行代码结构，使功能更清晰

### 2. 依赖更新与替换
- 将 `github.com/manifoldco/promptui` 替换为 `github.com/charmbracelet/huh` 和 `github.com/charmbracelet/lipgloss`
- 更新 Go 容器镜像版本至 1.25.0
- 更新 go.mod 文件以反映最新依赖

### 3. 功能增强
- 重构交互式选择界面，提供更美观的用户体验
- 添加国际化支持，集中管理界面文本
- 改进SSH客户端登录流程并添加会话结束回调
- 实现配置文件自动备份功能
- 完善配置验证功能，支持跨平台路径处理
- 优化节点选择逻辑，增加返回上级功能

### 4. 用户体验改进
- 支持多种退出方式（Ctrl+d、Ctrl+c、输入q/Q）
- 添加清屏功能，提升界面整洁度
- 优化主界面显示，返回时重新显示节点列表

### 5. 文档更新
- 更新依赖列表和退出方式说明
- 更新TODO文档中的功能描述和进度
- 添加变更文档记录
- 完善配置文件格式说明

## 详细变更

### 新增文件
- `TODO.md`: 项目待办事项列表
- `ssh/messages.go`: 集中管理界面文本，实现国际化
- `config/model_config.go`: 配置模型定义
- `config/validated_config.go`: 配置验证功能
- `config/backup_config.go`: 配置备份功能
- `cmd/messages.go`: 命令行相关文本
- `cmd/runssh_cmd.go`: SSH运行命令功能
- `cmd/sync_cobra.go`: 同步命令功能
- `cmd/version_cobra.go`: 版本命令功能

### 代码改进
- `cmd/cobra_cmd.go`: 重构命令结构，拆分功能到独立文件
- `main.go`: 简化主函数逻辑
- `ssh/client.go`: 改进SSH客户端登录流程和会话管理
- `config/config.go`: 重构配置加载和验证逻辑
- `readme.md` 和 `readme.zh.md`: 更新文档内容和依赖列表

### 功能实现详情
1. **配置验证功能**
   - 添加了 `ValidateConfig` 函数对整个配置进行验证
   - 实现了专用校验函数验证同步配置、节点组和SSH节点
   - 提供清晰具体的错误信息
   - 兼容Windows、Linux和MacOS路径格式

2. **国际化支持**
   - 新增 `ssh/messages.go` 文件集中管理界面文本
   - 支持中英文提示信息

3. **退出功能优化**
   - 支持多种退出方式：`Ctrl+d`、`Ctrl+c` 和输入 `q/Q`
   - 先按 `Ctrl+c` 再按 `Ctrl+d` 或输入 `q/Q` 退出
   - 清屏功能提升用户体验

## 提交历史
```
fad9e08 (HEAD -> main, tag: V25.08.25, origin/main, origin/HEAD) Merge pull request #6 from cnphpbb/feature-dev
e83c224 docs: 更新依赖列表和退出方式说明
cfe0d4a refactor(cmd): 重构代码结构，将功能模块拆分到独立文件
ed5ca58 feat(ssh): 重构交互式选择界面并添加国际化支持
26729fe docs: 更新TODO文档中的功能描述和进度
19e07ac chore: 更新.gitignore文件以忽略mysshw文件
503154a (tag: v25.08.24.2) Merge pull request #5 from cnphpbb/gh-dev
b8ead72 feat(config): 重构配置模块并添加验证功能
266a954 feat(ssh): 改进SSH客户端登录流程并添加会话结束回调
c4e6959 feat: 改进跨平台兼容性和构建流程
2f9d51d build(docker): 更新 Go 容器镜像版本至 1.25.0
dc69d63 chore: 更新.gitignore文件以忽略更多toml配置文件
15327a5 docs: 添加变更文档记录v25.06.06至v25.08.24的主要变更
```