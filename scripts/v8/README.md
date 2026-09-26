# V8 引擎接入方案（Windows / MSYS2 mingw-w64）

> 实测日期：2026-09-27　机器：Windows + Go 1.26.4 + MSYS2 mingw64 gcc 14.2.0（**无 MSVC、无 clang**）
> 结论：**Windows 上嵌入 V8 可行且已跑通**（编译/链接/运行/基准全通过），但需自带 V8 SDK 并维护 v8go fork。

## 1. 为什么不能用 v8go 的官方产物

- `rogchap.com/v8go` 最新 tag **v0.9.0（2023-04-10）**，之后停更。
- PR [#234](https://github.com/rogchap/v8go/pull/234)（2021-11 合并）**移除了 Windows 二进制支持**：Windows 版当年靠
  MSYS2 的 mingw-w64-v8 补丁构建，补丁未跟上 V8 升级 → CI 只能产 Linux/macOS 的 `libv8.a`。
- 实测：`deps/` 只有 `linux_{x86_64,arm64}` 与 `darwin_{x86_64,arm64}`；Windows 上构建直接
  `cannot find -lv8: No such file or directory`。

## 2. 采用的方案：MSYS2 的 V8 11.9 + 本地 fork 的 v8go

MSYS2 维护着 mingw-w64 版 V8（正是 v8go 当年依赖的构建路线）：

| 项 | 值 |
|---|---|
| 包 | `mingw-w64-x86_64-v8` **11.9.169.6-4**（2025-01 构建；另有 clang64/ucrt64/clangarm64 变体） |
| 下载/安装体积 | 17.94 MiB / 87.59 MiB |
| 提供 | `include/v8.h`、`lib/libv8.dll.a`、`bin/libv8.dll`(28.7MB)、`libv8_libbase`、`libv8_libplatform`、`d8.exe` |
| 构建配置 | `is_component_build=true`（共享库）；**pointer compression ON + sandbox ON**（= v8go 默认宏一致） |
| 依赖 | `mingw-w64-x86_64-icu`（76.1，另需解包 ICU DLL）+ zlib |

准备 SDK（不改动系统）：`bash scripts/v8/fetch-msys2-v8.sh`

## 3. 对 v8go 的改动（`scripts/v8/v8go-win.patch`，共 6 行）

1. `cgo.go`：CXXFLAGS/LDFLAGS 加 `windows` 分支，改指向 `_temp/v8sdk/mingw64/{include,lib}`，链接
   `-lv8 -lv8_libbase -lv8_libplatform`；非 Windows 分支保持原样。
   宏 `-DV8_COMPRESS_POINTERS -DV8_31BIT_SMIS_ON_64BIT_ARCH -DV8_ENABLE_SANDBOX`
   **必须保留**（与 MSYS2 库的构建配置一致，否则运行时 fatal：*Embedder-vs-V8 build configuration mismatch*）。
2. `v8go.cc`：两处 V8 11.1 → 11.9 的 API 变化
   - `Persistent::Empty()` 已移除（`TemplateFreeWrapper`）→ 删除该行（随后 `delete` 的析构同样不释放 handle）
   - `Object::GetInternalField()` 返回类型改为 `Local<Data>` → 用 `Local<Value>::Cast(...)`

## 4. 使用

```bash
bash scripts/v8/fetch-msys2-v8.sh                 # 下载并解包 V8 SDK 到 _temp/v8sdk
# v8go fork 就位后（含补丁）：
export CGO_ENABLED=1 GOWORK=off
export PATH="$(pwd)/_temp/v8sdk/mingw64/bin:/f/msys64/mingw64/bin:$PATH"   # 运行期找 DLL；编译期找 g++
go build ./...
# 分发时随程序携带：libv8.dll(28.7MB) + libv8_libbase.dll + libv8_libplatform.dll + libicu*.dll(约 24MB)
```

## 5. 已跑通的验证（2026-09-27）

- 最小程序：`ctx.RunScript("1+1")` → `2`；5 万次字符串拼接 → `288890`（长度与 goja/node 一致）。
- d8.exe（MSYS2 自带 V8 shell）跑 `dev/output/jsbench.js`：**与 node 22 同级**
  （fib 4ms / 数组 26ms / 字符串 9ms / 对象属性 6ms），而 goja 为 91 / 690 / 2700 / 590 ms。

### 边界（穿梭）成本 —— 100 万次调用，ns/op

| 操作 | goja | V8 (v8go) | 倍数 |
|---|---|---|---|
| JS→Go 空回调 | 143.6 | 1062.1 | **7.4×** |
| JS→Go 读参+返回数 | 174.2 | 1702.1 | **9.8×** |
| Go→JS 调用（每次新值） | 229.5 | 4801.4 | **20.9×** |
| Go 侧 `obj.Get` | 13.0 | 3125.0 | **240×** |
| Go 侧 `obj.Set` | 69.1 | 4627.1 | **67×** |
| 纯 JS 属性读写（引擎内） | 182.6 | 4.4 | V8 快 41× |
| 纯 JS 函数调用（引擎内） | 151.5 | 2.1 | V8 快 72× |

**读法**：V8 的跨界是 **1–5 µs/次**（不是 cgo 裸调用的 76 ns：每次跨界要 Locker + Isolate::Scope +
HandleScope + Persistent 装箱 + Go 侧回调注册表查找）。因此**跨界次数是新的性能杠杆**：
每帧 1 万次跨界 ≈ 多花 15 ms；5 万次 ≈ 75 ms。相较之下 JS 执行本身快 20–300×。

复现基准：`go run ./dev/probes/jsboundary`（goja 侧）、`_temp/v8spike/`（V8 侧，含 bench_main.go.txt）

## 6. 已知坑（务必遵守）

1. **`NewValue(iso, int64)` 造出的是 BigInt**（`int32`/`float64` 才是 Number）。DOM 绑定层传整数必须用
   `int32`/`float64`；`Value.Integer()` 对 BigInt/非 Number 会触发 **V8 fatal（进程崩溃，不是 error）**。
2. **FunctionTemplate / ObjectTemplate 必须在 `NewContext` 之前注册**，之后 `Set` 无效（静默：JS 侧报 not defined）。
3. 一旦「Go 回调的返回值」再作为参数传回回调，务必确认它是 Number 而非 BigInt（见坑 1）。
4. Windows 上 V8 版本由 MSYS2 决定（11.9）；Linux/macOS 走 v8go 自带预编译库（V8 11.1）→ **跨平台版本不一致**，
   行为差异需各自回归。
5. 分发体积增加约 **60 MB**（libv8.dll + ICU）。
