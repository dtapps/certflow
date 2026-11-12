#!/usr/bin/env bash
# Copyright (c) 2018-Present Lea Anthony
# SPDX-License-Identifier: MIT

# Fail script on any error
set -euxo pipefail

# 容器里默认没有 /dev/fuse，而 linuxdeploy 自己就是一个 AppImage（运行时要挂 squashfs）。
# 检测不到 fuse 设备时自动改为「解包运行」，GitHub 容器 job 与原生 runner 都能跑通；
# 原生 runner / 本地有 fuse 时不设这个变量，行为与上游模板完全一致。
if [ ! -e /dev/fuse ]; then
    export APPIMAGE_EXTRACT_AND_RUN=1
    echo "未检测到 /dev/fuse，APPIMAGE_EXTRACT_AND_RUN=1（解包运行 AppImage）"
fi

# Define variables
APP_DIR="${APP_NAME}.AppDir"

# Create AppDir structure
mkdir -p "${APP_DIR}/usr/bin"
cp -r "${APP_BINARY}" "${APP_DIR}/usr/bin/"
cp "${ICON_PATH}" "${APP_DIR}/"
cp "${DESKTOP_FILE}" "${APP_DIR}/"

if [[ $(uname -m) == *x86_64* ]]; then
    # Download linuxdeploy and make it executable
    wget -q -4 -N https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-x86_64.AppImage
    chmod +x linuxdeploy-x86_64.AppImage

    # Run linuxdeploy to bundle the application
    ./linuxdeploy-x86_64.AppImage --appdir "${APP_DIR}" --output appimage
else
    # Download linuxdeploy and make it executable (arm64)
    wget -q -4 -N https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-aarch64.AppImage
    chmod +x linuxdeploy-aarch64.AppImage

    # Run linuxdeploy to bundle the application (arm64)
    ./linuxdeploy-aarch64.AppImage --appdir "${APP_DIR}" --output appimage
fi

# Rename the generated AppImage
mv "${APP_NAME}"*.AppImage "${APP_NAME}.AppImage"
