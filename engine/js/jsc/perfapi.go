// API 级性能插桩（WB_PERF_DISPATCH=1 开启；默认关闭零开销）。
//
// 目的：把 listener 回调总耗时（[DISP] listenerTime）二级分解为
// 「引擎 API 调用耗时」与「goja 纯 JS 执行」，用于判定滚动 1.6~1.8s 的成本
// 到底落在引擎侧（存在削减空间）还是宿主框架 JS 执行量（不存在）。
//
// 包裹点是 binding 层的两个**统一注册入口** —— JSObject.Set（方法注册）与
// JSObject.SetAccessor（属性读写注册）—— 所以不需要逐调用点改插桩：只要 API
// 名出现在 perfAPINames，注册时自动包上计时器。默认关闭时注册路径只多一次
// bool 判断，运行时零开销（不装 wrapper）。
package jsc

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// perfAPIDispatchOn 与 dom 包的 perfDispOn 同源（同一个环境变量），默认关闭。
var perfAPIDispatchOn = os.Getenv("WB_PERF_DISPATCH") != "" && os.Getenv("WB_PERF_DISPATCH") != "0"

// perfAPINames 统计名单：属性读/写名与方法名共用一张表。按“几何/样式查询原语”
// 圈定范围（这些正是 webkit 移植里最可能退化的 API：要么 O(DOM 规模)，要么触发
// 隐式重算）。名单外的名字不装 wrapper。
var perfAPINames = map[string]bool{
	// CSSOM View 几何
	"getBoundingClientRect": true,
	"getClientRects":        true,
	"offsetWidth":           true,
	"offsetHeight":          true,
	"offsetTop":             true,
	"offsetLeft":            true,
	"offsetParent":          true,
	"clientWidth":           true,
	"clientHeight":          true,
	"clientTop":             true,
	"clientLeft":            true,
	"scrollTop":             true,
	"scrollLeft":            true,
	"scrollHeight":          true,
	"scrollWidth":           true,
	// 样式
	"getComputedStyle": true,
	"style":            true,
	// 命中测试
	"elementFromPoint":  true,
	"elementsFromPoint": true,
	// 选择器
	"querySelector":    true,
	"querySelectorAll": true,
	"matches":          true,
	"closest":          true,
	// Range
	"createRange":      true,
	"getRangeAt":       true,
	"comparePoint":     true,
	"intersectsNode":   true,
	"startContainer":   true,
	"endContainer":     true,
	"commonAncestorContainer": true,
	// Selection
	"getSelection":    true,
	"selection":       true,
	"addRange":        true,
	"removeAllRanges": true,
	"removeRange":     true,
	"collapse":        true,
	"collapseToEnd":   true,
	"rangeCount":      true,
	"anchorNode":      true,
	"focusNode":       true,
}

type perfAPIStat struct {
	n   int           // 调用次数（含嵌套层级）
	dur time.Duration // 累计耗时（仅 depth==0 的最外层计入，避免嵌套重复计同一段时间）
}

var (
	perfAPITbl   = map[string]*perfAPIStat{}
	perfAPIDepth int           // 当前嵌套深度：>0 表示处于另一 API 内部
	perfAPIOn    bool          // listener 作用域开关（由 dom 包在进入/退出 listener 时置位）
	perfAPITotal time.Duration // 最外层 API 耗时之和
	perfAPITotalN int
)

// PerfAPIEnter 进入 listener 回调：开启 API 计时（由 dom 包经 hook 调用）。
func PerfAPIEnter() {
	perfAPIOn = true
	perfAPIDepth = 0
}

// PerfAPIExit 退出 listener 回调：关闭 API 计时。
func PerfAPIExit() { perfAPIOn = false }

// PerfAPIReset 清空本轮的统计（每次待观测派发开始前调用）。
func PerfAPIReset() {
	perfAPITbl = map[string]*perfAPIStat{}
	perfAPIDepth = 0
	perfAPITotal = 0
	perfAPITotalN = 0
}

func perfAPIRecord(name string, d time.Duration) {
	s := perfAPITbl[name]
	if s == nil {
		s = &perfAPIStat{}
		perfAPITbl[name] = s
	}
	s.n++
	perfAPITotalN++
	if perfAPIDepth == 0 {
		s.dur += d
		perfAPITotal += d
	}
}

// perfWrapNative 包装一个原生方法：仅在「开关开启 + 名字在名单内」时生效。
func perfWrapNative(name string, fn NativeFunc) NativeFunc {
	if !perfAPIDispatchOn || !perfAPINames[name] {
		return fn
	}
	return func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if !perfAPIOn {
			return fn(in, this, args)
		}
		t := time.Now()
		perfAPIDepth++
		defer func() {
			perfAPIDepth--
			perfAPIRecord(name, time.Since(t))
		}()
		return fn(in, this, args)
	}
}

// perfWrapGet1 / perfWrapGet2 包装 SetAccessor 的两种 getter 签名。
func perfWrapGet1(name string, fn func(*Interpreter) JSValue) func(*Interpreter) JSValue {
	if !perfAPIDispatchOn || !perfAPINames[name] {
		return fn
	}
	return func(in *Interpreter) JSValue {
		if !perfAPIOn {
			return fn(in)
		}
		t := time.Now()
		perfAPIDepth++
		defer func() {
			perfAPIDepth--
			perfAPIRecord(name, time.Since(t))
		}()
		return fn(in)
	}
}

func perfWrapGet2(name string, fn func(*Interpreter, JSValue) JSValue) func(*Interpreter, JSValue) JSValue {
	if !perfAPIDispatchOn || !perfAPINames[name] {
		return fn
	}
	return func(in *Interpreter, this JSValue) JSValue {
		if !perfAPIOn {
			return fn(in, this)
		}
		t := time.Now()
		perfAPIDepth++
		defer func() {
			perfAPIDepth--
			perfAPIRecord(name, time.Since(t))
		}()
		return fn(in, this)
	}
}

// perfWrapSet 包装 SetAccessor 的 setter。
func perfWrapSet(name string, fn func(*Interpreter, JSValue, JSValue)) func(*Interpreter, JSValue, JSValue) {
	if !perfAPIDispatchOn || !perfAPINames[name] {
		return fn
	}
	return func(in *Interpreter, this, v JSValue) {
		if !perfAPIOn {
			fn(in, this, v)
			return
		}
		t := time.Now()
		perfAPIDepth++
		defer func() {
			perfAPIDepth--
			perfAPIRecord(name, time.Since(t))
		}()
		fn(in, this, v)
	}
}

// PerfAPIDump 输出本轮分解表（按累计耗时降序 topN），返回可直接打印的多行文本。
// listenerTime 由调用方给出，用于计算「API 占比」。
func PerfAPIDump(limit int, listenerTime time.Duration) string {
	type row struct {
		name string
		st   *perfAPIStat
	}
	rows := make([]row, 0, len(perfAPITbl))
	for k, v := range perfAPITbl {
		rows = append(rows, row{k, v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].st.dur != rows[j].st.dur {
			return rows[i].st.dur > rows[j].st.dur
		}
		return rows[i].name < rows[j].name
	})
	var b strings.Builder
	pct := func(d time.Duration) float64 {
		if listenerTime <= 0 {
			return 0
		}
		return float64(d) / float64(listenerTime) * 100
	}
	fmt.Fprintf(&b, "[DISP-API] listenerTime=%v apiCallN=%d apiSum=%v (%.2f%% of listenerTime) gojaJS≈%v\n",
		listenerTime, perfAPITotalN, perfAPITotal, pct(perfAPITotal), listenerTime-perfAPITotal)
	if len(rows) == 0 {
		fmt.Fprintf(&b, "[DISP-API]   (名单内 API 零调用)\n")
		return b.String()
	}
	for i, r := range rows {
		if limit > 0 && i >= limit {
			break
		}
		avg := time.Duration(0)
		if r.st.n > 0 {
			avg = r.st.dur / time.Duration(r.st.n)
		}
		fmt.Fprintf(&b, "[DISP-API]   #%-2d %-26s n=%-8d sum=%-12v avg=%-10v %.2f%%\n",
			i+1, r.name, r.st.n, r.st.dur, avg, pct(r.st.dur))
	}
	return b.String()
}
