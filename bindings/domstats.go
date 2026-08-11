package bindings

// DOM API 调用统计（调试用）：WB_DOM_STATS=1 时累计热点 API 的调用次数
// 与耗时，DumpDOMStats 打印。用于定位「事件响应极慢」的瓶颈（CM6
// measure 的 getClientRects → computedStyleFor/GetElementBoxRect 热点）。
import (
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var (
	domStatsOn     bool
	domStatsOnce   sync.Once
	domStatCount   = map[string]*int64{}
	domStatTotalNS = map[string]*int64{}
)

func domStatsEnabled() bool {
	domStatsOnce.Do(func() {
		domStatsOn = os.Getenv("WB_DOM_STATS") != ""
	})
	return domStatsOn
}

func domStat(name string, d time.Duration) {
	if !domStatsEnabled() {
		return
	}
	c, ok := domStatCount[name]
	if !ok {
		c = new(int64)
		domStatCount[name] = c
		t := new(int64)
		domStatTotalNS[name] = t
	}
	atomic.AddInt64(c, 1)
	atomic.AddInt64(domStatTotalNS[name], int64(d))
}

// DumpDOMStats 打印统计（WB_DOM_STATS=1 时有效）。
func DumpDOMStats() {
	if !domStatsEnabled() {
		return
	}
	println("[domstats] === DOM API 统计 ===")
	for name, c := range domStatCount {
		n := atomic.LoadInt64(c)
		ns := atomic.LoadInt64(domStatTotalNS[name])
		if n > 0 {
			println("[domstats] ", name, "calls=", n, "total=", float64(ns)/1e6, "ms avg=", float64(ns)/float64(n)/1e6, "ms")
		}
	}
}

// ResetDOMStats 清零统计（分段测量用）。
func ResetDOMStats() {
	if !domStatsEnabled() {
		return
	}
	for name, c := range domStatCount {
		atomic.StoreInt64(c, 0)
		atomic.StoreInt64(domStatTotalNS[name], 0)
	}
}
