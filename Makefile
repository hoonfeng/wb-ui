# wb-ui Makefile
#
# 前置条件（CGO 环境）：
#   - MinGW-w64 GCC（如 MSYS2 Mingw64 的 gcc）
#   - goskia Skia DLL (libSkiaSharp.dll) 在 SKIA_DLL_DIR 中
#
# 运行前确保：
#   set CGO_ENABLED=1
#   set PATH=F:\syproject\goskia\bin;%PATH%
#
# macOS/Linux 用户还需安装 libglfw3-dev

# CGO 配置
CGO_ENABLED := 1
SKIA_DLL_DIR := $(shell cygpath -d "F:/syproject/goskia/bin" 2>nul || echo "F:\\syproject\\goskia\\bin")

export CGO_ENABLED := 1
export PATH := $(SKIA_DLL_DIR):$(PATH)

.PHONY: build test vet fmt clean translation-progress

GO ?= go

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
