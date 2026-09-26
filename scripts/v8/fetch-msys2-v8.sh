#!/usr/bin/env bash
# fetch-msys2-v8.sh —— 在 Windows(MSYS2 / mingw-w64) 上准备 V8 SDK。
#
# 产出（全部解包到 _temp/v8sdk，**不改动系统 MSYS2 目录**）：
#   _temp/v8sdk/mingw64/include/v8.h            V8 C++ API 头文件
#   _temp/v8sdk/mingw64/lib/libv8.dll.a         导入库（-lv8 可用）
#   _temp/v8sdk/mingw64/bin/libv8.dll           运行时（约 28.7 MB）
#   _temp/v8sdk/mingw64/bin/libicu*.dll         ICU 依赖（约 24 MB）
#
# 用法：bash scripts/v8/fetch-msys2-v8.sh [目标目录]
set -euo pipefail

DEST="${1:-_temp/v8sdk}"
PACMAN="${PACMAN:-/f/msys64/usr/bin/pacman}"

if [[ ! -x "$PACMAN" ]]; then
  echo "找不到 pacman（$PACMAN）：请确认已安装 MSYS2（默认 F:/msys64），或用 PACMAN=... 指定" >&2
  exit 1
fi

mkdir -p "$DEST"
for pkg in mingw-w64-x86_64-v8 mingw-w64-x86_64-icu; do
  url=$("$PACMAN" -Sp "$pkg" | tail -1)
  echo "==> $pkg"
  echo "    $url"
  curl -sL -o "$DEST/$pkg.pkg.tar.zst" "$url"
  tar -xf "$DEST/$pkg.pkg.tar.zst" -C "$DEST"
  echo "    解包完成"
done

echo
echo "SDK 就绪：$DEST/mingw64/{bin,include,lib}"
echo "构建时："
echo "  export CGO_ENABLED=1 GOWORK=off"
echo "  export PATH=\"\$(pwd)/$DEST/mingw64/bin:/f/msys64/mingw64/bin:\$PATH\""
echo "  go build ./..."
