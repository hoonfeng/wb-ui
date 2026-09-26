#!/usr/bin/env bash
# setup-v8go-fork.sh —— 把 v8go v0.9.0 拷成本地 fork 并打上 Windows/V8-11.9 补丁。
#
# 产物：third_party/v8go/（module 路径仍是 rogchap.com/v8go，用 go.mod 的 replace 指向它）
# 说明：拷贝时删除 deps/*/*.a（约 279MB 的 Linux/macOS 预编译库）以保持仓库轻量——
#       非 Windows 平台需要它们时，从 module cache 原样拷回，或自行构建 V8。
#
# 用法：bash scripts/v8/setup-v8go-fork.sh
set -euo pipefail

VER="${V8GO_VERSION:-v0.9.0}"
MOD="$(go env GOMODCACHE)/rogchap.com/v8go@$VER"
DEST="third_party/v8go"

[[ -d "$MOD" ]] || { echo "找不到 $MOD，先执行：go mod download rogchap.com/v8go@$VER" >&2; exit 1; }

rm -rf "$DEST"
mkdir -p "$(dirname "$DEST")"
cp -r "$MOD" "$DEST"
chmod -R u+w "$DEST"
find "$DEST/deps" -name '*.a' -delete

# 应用补丁（cgo windows 分支 + V8 11.9 API 适配）
patch -p0 --forward "$DEST/cgo.go" <(sed -n '/cgo\.go$/,/^--- .*v8go\.cc$/p' scripts/v8/v8go-win.patch) 2>/dev/null || {
  echo "补丁自动应用失败：请手动按 scripts/v8/README.md 第 3 节修改 $DEST/cgo.go 与 v8go.cc" >&2
  exit 1
}
echo "fork 就绪：$DEST（记得在 go.mod 加：replace rogchap.com/v8go => ./$DEST）"
