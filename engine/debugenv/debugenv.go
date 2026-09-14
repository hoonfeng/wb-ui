// Package debugenv 提供进程启动时的调试开关快照。
//
// os.Getenv 在 Windows（及其他平台）上是系统调用，并且**每次调用都会分配内存**。
// 布局与绘制的热路径（每个盒的 Layout、每个绘制对象的 paint）都要检查 WB_* /
// WBUI_* 调试开关：BenchmarkLayoutFull 的 -memprofile 显示 os.Getenv 占布局
// **分配量的 72%**（238MB / 330MB），随之而来的 GC 扫描是 profile 里的第二大
// 开销。本包在初始化时一次性快照环境变量，运行时只做一次无锁 map 查找、零分配。
//
// 语义差异：调试开关必须在**进程启动前**设置（os.Setenv 之后不再生效）。仓库内
// 没有测试用 Setenv 中途切换这些开关——诊断脚本、夹具工具与人工调试都是在启动
// 时设置环境变量。
package debugenv

import (
	"os"
	"strings"
)

var flags = capture()

func capture() map[string]bool {
	m := make(map[string]bool, 16)
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		name, val := kv[:i], kv[i+1:]
		if strings.HasPrefix(name, "WB_") || strings.HasPrefix(name, "WBUI_") {
			m[name] = val != ""
		}
	}
	return m
}

// Enabled 报告调试开关 name（如 "WB_LAYOUT_DEBUG"、"WBUI_IFC_DEBUG"）是否已启用。
// 等价于 os.Getenv(name) != ""，但不做系统调用、不分配内存。
func Enabled(name string) bool {
	return flags[name]
}
