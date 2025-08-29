# mysshw V25.08.30 更新日志

## 主要变更

本版本主要添加了从 sshw YAML 配置迁移到 mysshw TOML 配置的功能，以及更新了依赖和配置文件处理逻辑。

### 功能增强
- **新增 YAML 配置迁移功能**：添加了 `yml` 命令，支持从 sshw 项目的 YAML 配置文件导入并转换为 mysshw 的 TOML 配置格式
- **配置文件处理逻辑更新**：改进了配置文件的处理和验证逻辑
- **UI 样式优化**：更新了 SSH 交互选择界面的颜色方案

### 依赖更新
- 将 toml 库从 `github.com/BurntSushi/toml` 替换为 `github.com/GuanceCloud/toml`
- 显式添加 `gopkg.in/yaml.v3 v3.0.1` 依赖以支持 YAML 配置解析

### 文档更新
- 在中英文 README 中添加了 `yml` 命令的使用说明
- 更新了 TODO.md，将 "支持配置文件的导入/导出" 更新为 "支持sshw项目的配置文件的导入" 并标记为已完成

## 详细变更

### 新增文件和模块
- `cmd/yml_cmd.go`：实现从 sshw YAML 配置迁移到 mysshw TOML 配置的命令

### 代码改进
- `ssh/cmd.go`：更新了 SSH 交互界面的颜色样式，将用户@主机信息从灰色改为蓝色，提升可读性
- `config/model_config.go`：更新了配置文件示例，添加了更多样例配置
- 实现了 YAML 配置解析和 TOML 格式转换的核心逻辑
- 添加了配置文件内容转义功能，确保特殊字符在 TOML 格式中正确表示
- 支持将转换后的配置内容追加到现有配置文件中

### 功能实现详情

#### YAML 配置迁移功能
- 支持通过 `mysshw yml -f ~/.sshw.yml` 命令导入 sshw 的 YAML 配置
- 自动解析 YAML 格式的节点配置并转换为 mysshw 的 TOML 格式
- 支持处理带有用户、密码、密钥路径等各种 SSH 连接参数的配置
- 自动将转换后的配置追加到用户的配置文件中

#### 配置文件处理优化
- 改进了配置文件路径的处理和验证
- 更新了配置文件示例，添加了更多实用的配置样例
- 支持在追加配置时自动添加空白行，保持配置文件的整洁性

## 提交历史

```
f46bf74 (tag: V25.08.30, origin/main, origin/HEAD, main) Merge pull request #9 from cnphpbb/gh-dev
e27bc1e (HEAD -> gh-dev, origin/gh-dev) docs: 添加从 sshw YAML 配置迁移说明
4e9c954 feat: 支持sshw项目配置文件的导入
d4f38cd feat(yml): 添加从sshw YAML配置迁移到TOML配置的功能
148b363 feat(config): 更新配置处理逻辑并添加YML转换功能
f6420db fix: 更新依赖和配置文件
```