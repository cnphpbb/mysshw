# Changelog v26.06.13.2 (2026-06-13)

> P3 收尾：Docker 镜像、架构图、Changelog 流程规则。搁置了 CI/CD / Homebrew / Scoop（非项目目标）。

## ✨ 新功能

- 🐳 **Docker 镜像** — 多阶段构建（golang:1.25-bookworm → alpine:3.19），最终镜像 ~21 MB，非 root 用户，含 ca-certificates + tzdata。详见 `Dockerfile`。

## 📝 文档

- 🏛️ **架构图** — readme 中英文同步新增「架构」章节，含 mermaid 图（用户层/核心层/协议层/外部 4 层）+ 4 条关键设计决策说明
- 📋 **AGENTS.md 新规则** — 「Tag 与 Changelog 流程」：每次打 tag 后必须生成 `changelog.${tag}.md` 并提交入库（**先 commit changelog 再 push tag**）
- 🗒️ **TODO.md 同步** — 标记 Docker / 架构图 / changelog 规则 ✅，搁置 CI/CD / Homebrew / Scoop ⏸️

## 🏠 构建

- 🐳 **Dockerfile** — 支持 `--build-arg VERSION/BUILD/BUILD_TIME` 注入 ldflags，CGO_ENABLED=0 静态链接
- 📦 **.dockerignore** — 排除 `.git` / `dist` / `.idea` / README 等无关文件，构建上下文 ~6 MB

## 🧪 测试

无新增测试（P3 主要为发布基础设施 + 文档）

## ⏸️ 明确搁置（按用户决定）

- CI/CD（`.github/workflows/`）— 非生产环境工具，无需
- Homebrew tap（macOS）
- Scoop bucket（Windows）
- Huh 主题切换

## 📊 统计

- **4 个 commit**（自 v26.06.13.1 起）：+318 / -17 行
- **6 个新文件**：`Dockerfile`、`.dockerignore`
- **3 个修改**：`AGENTS.md`、`readme.md`、`readme.zh.md`、`TODO.md`
- **镜像体积**：20.9 MB（alpine 基础 + 静态二进制）
- **远端 tag 累计**：3 个（v25.08.24 / v26.06.13 / v26.06.13.1 / **v26.06.13.2**）

## 🔗 完整 commit 列表

```
844796e docs: 同步 TODO.md 反映 P3 进展 (Docker / 架构图 / changelog 规则)
f6bbf22 docs: 新增「架构」章节 (mermaid 图 + 设计决策说明)
b07c047 chore: 添加 Dockerfile (多阶段构建) + .dockerignore
9f6e6e6 docs(AGENTS): 新增「Tag 与 Changelog 流程」规则
```

## 📦 Docker 使用

```bash
# 构建
docker build -t mysshw:latest .
docker build -t mysshw:v26.06.13.2 \
  --build-arg VERSION=v26.06.13.2 \
  --build-arg BUILD=$(git rev-parse --short HEAD) \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) .

# 运行
docker run --rm mysshw:latest --version
docker run --rm -v $PWD/mysshw.toml:/home/mysshw/.mysshw.toml:ro mysshw
```

## ⬆️ 升级指引

从 v26.06.13.1 升级：
- 无 API 变更
- 新增 Docker 部署方式
- readme 新增「架构」章节供设计参考
- 建议维护者遵循新增的 changelog 流程
