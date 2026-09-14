# wb-ui Makefile
#
# 前置条件（CGO 环境）：
#   - MinGW-w64 GCC（如 MSYS2 Mingw64 的 gcc）
#   - goskia 提供的 Skia 原生库（libSkiaSharp.dll / .so / .dylib）
#   - macOS/Linux 用户还需安装 libglfw3-dev
#
# 本 Makefile 自动配置 CGO 与原生库路径：未设置 SKIA_DLL_DIR 时，
# 从 go.mod 依赖的 goskia 模块定位原生库（检出后需先 `go mod download`）。
# 自定义位置：make build SKIA_DLL_DIR=/path/to/goskia/skia/lib/<os>_<arch>

CGO_ENABLED := 1
GO ?= go

# Skia 原生库目录（含 libSkiaSharp.*）；外部已设置时尊重外部值
SKIA_DLL_DIR ?= $(shell $(GO) list -m -f '{{.Dir}}' github.com/hoonfeng/goskia 2>/dev/null)/skia/lib/$(shell $(GO) env GOOS)_$(shell $(GO) env GOARCH)

export CGO_ENABLED := 1
export PATH := $(SKIA_DLL_DIR):$(PATH)

.PHONY: build test vet fmt clean translation-progress

build:
	$(GO) build ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

clean:
	$(GO) clean -cache

translation-progress:
	$(GO) run ./cmd/translation-progress > translation-progress.txt
	@echo "Translation progress written to translation-progress.txt"
