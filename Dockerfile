# syntax=docker/dockerfile:1.7
#
# mysshw Dockerfile - multi-stage build for minimal runtime image
#
# Build:
#   docker build -t mysshw:latest .
#   docker build -t mysshw:v26.06.13.2 --build-arg VERSION=v26.06.13.2 .
#
# Run:
#   docker run --rm -it mysshw --help
#   docker run --rm -it -v ~/.mysshw.toml:/home/mysshw/.mysshw.toml mysshw
#

# ---- Stage 1: Build ----
FROM golang:1.25.0-bookworm AS builder

# Build-time metadata (注入到 ldflags)
ARG VERSION="v0.0.0-dev"
ARG BUILD="docker"
ARG BUILD_TIME=""

WORKDIR /src

# 设置国内 Go proxy (goproxy.cn) 避免 proxy.golang.org 网络问题
ENV GOPROXY=https://goproxy.cn,direct \
    GO111MODULE=on

# 缓存依赖层: 先复制 go.mod / go.sum 单独下载依赖
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 复制源码并构建
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags "-s -w \
        -X 'main.Version=${VERSION}' \
        -X 'main.Build=${BUILD}' \
        -X 'main.BuildTime=${BUILD_TIME}' \
        -extldflags '-static'" \
      -o /out/mysshw ./

# 复制示例配置 (运行时默认配置)
RUN cp example/mysshw.toml /out/mysshw.toml.example

# ---- Stage 2: Runtime ----
# 使用 alpine (体积小, 含 nobody 用户, 镜像仓库可达)
# 备选: distroless (gcr.io 不可达时降级到 alpine)
FROM alpine:3.19

LABEL org.opencontainers.image.title="mysshw" \
      org.opencontainers.image.description="A free and open source SSH command line client" \
      org.opencontainers.image.source="https://github.com/cnphpbb/mysshw" \
      org.opencontainers.image.licenses="MIT"

# 安装 CA 证书 (SSH/SCP/WebDAV 等 TLS 服务需要)
# ca-certificates 已是 alpine 默认组件, 显式 install 一下确保存在
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S mysshw && adduser -S -G mysshw mysshw

# 从 builder 复制二进制和示例配置
COPY --from=builder /out/mysshw /usr/local/bin/mysshw
COPY --from=builder /out/mysshw.toml.example /etc/mysshw/mysshw.toml.example

# 使用非 root 用户
USER mysshw:mysshw
WORKDIR /home/mysshw

# 默认配置路径优先级:
#   1. -c / --cfg 命令行参数
#   2. ~/.mysshw.toml (推荐挂在 /home/mysshw/.mysshw.toml)
#
# 挂载示例:
#   docker run --rm -it \
#     -v $PWD/mysshw.toml:/home/mysshw/.mysshw.toml:ro \
#     mysshw

# ENTRYPOINT 形式: mysshw <args>
# 用户运行: docker run --rm mysshw --help
ENTRYPOINT ["/usr/local/bin/mysshw"]
CMD ["--help"]
