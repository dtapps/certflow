# 使用 golang 镜像作为基础
FROM golang:1.27.0-trixie

# ==========================================
# 定义版本变量
# https://github.com/nodejs/node/releases
# ==========================================
ARG NODE_VERSION=26.5.0

# 1. 设置 Go 环境变量
ENV GO111MODULE=on
ENV GOPROXY=https://mirrors.cloud.tencent.com/go/,direct
ENV GOINSECURE=go.dtapp.net
ENV GOPATH=/go
ENV PATH="$GOPATH/bin:$PATH"

# 2. 替换 Debian APT 加速源
RUN sed -i 's|deb.debian.org|mirrors.cloud.tencent.com|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null || \
    sed -i 's|deb.debian.org|mirrors.cloud.tencent.com|g' /etc/apt/sources.list

# 3. 安装基础构建工具
# 注意：libgtk-4-dev / libwebkitgtk-6.0-dev 是 wails3 CLI 从源码 `go install` 及
# 后续 go build/test 编译 webview 所必须的（提供 gtk4.pc / webkitgtk-6.0.pc），
# 否则 make tool-deps / make check 会因 pkg-config 找不到头文件而失败。
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    xz-utils \
    ca-certificates \
    curl \
    gnupg \
    zip \
    unzip \
    pkg-config \
    libgtk-4-dev \
    libwebkitgtk-6.0-dev \
    jq

# 4. 安装 Node.js 工具
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then NODE_ARCH="x64"; \
    elif [ "$ARCH" = "aarch64" ]; then NODE_ARCH="arm64"; \
    else echo "Unsupported architecture: $ARCH" && exit 1; fi && \
    \
    # 构建下载地址
    NODE_DIR="node-v${NODE_VERSION}-linux-${NODE_ARCH}" && \
    NODE_URL="https://nodejs.org/dist/v${NODE_VERSION}/${NODE_DIR}.tar.xz" && \
    \
    cd /tmp && \
    wget -q -O node.tar.xz "${NODE_URL}" && \
    tar -xJf node.tar.xz -C /usr/local --strip-components=1 && \
    \
    # 配置 npm/pnpm 使用腾讯云源
    npm config set registry https://mirrors.cloud.tencent.com/npm/ && \
    npm install -g pnpm && \
    pnpm config set registry https://mirrors.cloud.tencent.com/npm/ && \
    pnpm config set ignore-scripts false && \
    \
    # 清理
    rm -rf node.tar.xz && \
    echo "--- Node.js installed ---" && \
    node -v && npm -v && pnpm -v

# 5. 卸载不需要的工具，并清空所有 apt 缓存
RUN apt-get purge -y wget xz-utils && \
    apt-get autoremove -y && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /root/.npm

CMD ["bash"]