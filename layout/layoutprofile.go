// 布局性能 profile（WB_LAYOUT_PROFILE=1）：统计一次渲染循环中 BFC / FFC /
// IFC / Grid 等各格式化上下文的总耗时与调用次数，定位「拖拽不跟手」的
// 布局瓶颈（全树布局每帧重跑，某类 context 可能是大头）。
package layout

import (
	"wb-ui/debugenv"
	"log"
	"time"
)

var (
	layProfileEnabled bool
	layProfileInited  bool
	layBFCTime, layFFCTime, layIFCTime, layGridTime, layTableTime, layOtherTime time.Duration
	layBFCCalls, layFFCCalls, layIFCCalls, layGridCalls, layTableCalls, layOtherCalls int
)

func layProfileInit() {
	if !layProfileInited {
		layProfileInited = true
		layProfileEnabled = debugenv.Enabled("WB_LAYOUT_PROFILE")
	}
}

// profileLayout 在每个格式化上下文 Layout 入口 defer 调用，统计耗时。
func profileLayout(kind string) func() {
	layProfileInit()
	if !layProfileEnabled {
		return func() {}
	}
	start := time.Now()
	return func() {
		d := time.Since(start)
		switch kind {
		case "bfc":
			layBFCTime += d
			layBFCCalls++
		case "ffc":
			layFFCTime += d
			layFFCCalls++
		case "ifc":
			layIFCTime += d
			layIFCCalls++
		case "grid":
			layGridTime += d
			layGridCalls++
		case "table":
			layTableTime += d
			layTableCalls++
		default:
			layOtherTime += d
			layOtherCalls++
		}
	}
}

// DumpLayoutProfile 打印累积的布局耗时分布并清零（host 每帧调用）。
func DumpLayoutProfile() {
	layProfileInit()
	if !layProfileEnabled {
		return
	}
	total := layBFCTime + layFFCTime + layIFCTime + layGridTime + layTableTime + layOtherTime
	log.Printf("[layout-profile] bfc=%v(%d) ffc=%v(%d) ifc=%v(%d) grid=%v(%d) table=%v(%d) other=%v(%d) total=%v",
		layBFCTime.Round(time.Microsecond), layBFCCalls,
		layFFCTime.Round(time.Microsecond), layFFCCalls,
		layIFCTime.Round(time.Microsecond), layIFCCalls,
		layGridTime.Round(time.Microsecond), layGridCalls,
		layTableTime.Round(time.Microsecond), layTableCalls,
		layOtherTime.Round(time.Microsecond), layOtherCalls,
		total.Round(time.Microsecond))
	layBFCTime, layFFCTime, layIFCTime = 0, 0, 0
	layGridTime, layTableTime, layOtherTime = 0, 0, 0
	layBFCCalls, layFFCCalls, layIFCCalls = 0, 0, 0
	layGridCalls, layTableCalls, layOtherCalls = 0, 0, 0
}
