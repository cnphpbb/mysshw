# TODO List

## 项目待办事项

### RunSSH 功能
- [x] 退出 SSH 会话并返回主界面
- [x] 支持 `Ctrl+d` 退出程序
- [x] 支持 `q` 退出程序 （试验性）
- [x] 支持 `Ctrl+c` 退出程序 （试验性，）

已成功在 ssh/client.go 和 cmd/cobra_cmd.go 文件中实现退出 SSH 会话并返回主界面功能，并更新了 TODO.md 文件标记任务完成。具体实现包括：

#### 退出功能实现
1. 添加了清屏功能，使用 `fmt.Print("\033[H\033[2J")` 命令清除终端屏幕内容
2. 优化了主界面显示，在返回主界面时重新显示节点列表
3. 实现了多种退出方式：`Ctrl+d`， `Ctrl+c` 或者 输入 `q` 退出

#### 功能亮点
- 支持多种退出方式：先按 `Ctrl+c`，再按 `Ctrl+d`  或  `Ctrl+c`  或 输入 `q | Q` 退出
- 清屏功能提升了用户体验，使界面更加整洁
- 改进了配置路径获取逻辑，确保默认配置路径在不同操作系统上正确工作


### 配置管理
- [x] 完善配置文件校验功能
- [x] 支持自定义配置文件路径
- [x] 对Windows路径格式的支持
- [x] 对Linux路径格式的支持
- [x] 对MacOS路径格式的支持
- [x] 对配置文件路径的校验
- [x] 对配置文件内容的校验
- [x] 支持sshw项目的配置文件的导入
- [ ] 添加配置文件加密选项
- [x] 实现配置的远程备份与恢复
- [x] 实现配置文件的自动备份

已成功在 config/config.go 文件中实现配置校验功能，并更新了 TODO.md 文件标记任务完成。具体实现包括：

#### 配置校验功能实现
1. 添加了 ValidateConfig 主函数，对整个配置进行全面验证
2. 实现了四个专用校验函数：
   - validateSyncConfig : 验证同步配置的类型和必要字段
   - validateNodeGroup : 验证节点组的有效性
   - validateSSHNode : 验证SSH节点的配置、主机、端口和认证方式
3. 在 LoadViperConfig 函数中调用校验函数，确保配置加载后立即进行验证

#### 校验功能亮点
- 支持验证同步类型的合法性和必要参数
- 检查SSH节点的主机、端口范围和认证方式
- 验证密钥文件路径的存在性
- 提供清晰具体的错误信息，方便用户定位问题
- 兼容Windows路径格式


### Sync Actions 功能
- [x] SCP 同步功能 (已实现)
- [ ] Github - Gist 同步集成
- [ ] Gitee - Gist 同步集成
- [ ] API - HTTP(s) 同步接口
- [ ] RPC 同步功能

### 用户界面
- [x] 实现命令自动补全
- [x] 替换pkg "github.com/manifoldco/promptui" （此项目已经停止维护）
- [x] 实现 "github.com/charmbracelet/huh" 替换 "github.com/manifoldco/promptui"

### 其他功能
- [ ] 添加日志记录
- [ ] 实现命令历史记录


## 代码优化
- [x] 重构重复代码
- [x] 完善单元测试
- [ ] 添加集成测试
- [x] 优化代码性能
- [x] 改进代码注释
- [x] 优化平台兼容性

**支持 `D:\sbin\mysshw.exe -c D:\mydata\mysshw\mysshw.toml` 启动程序, 指定配置文件** by 2025-08-24

## 发布计划
- [ ] 制作 Docker 镜像
- [ ] 自动化构建与测试流程