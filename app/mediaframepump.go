package app

import (
	"sync"
	"time"

	"wb-ui/engine/rendering"
)

/* 异步抽帧执行器（实现路径主线 A2：播放推进不阻塞渲染线程）。

为什么需要它：抽帧是**外部进程**调用（`ffmpeg -ss … -frames:v 1`，几十毫秒），
而 painter 在渲染线程上取帧。同步取帧意味着每换一帧就卡住一次绘制——播放 1 秒
10fps 的样本时尤其明显。宿主因此提供第二个（异步）帧源：渲染层提交请求就立刻
返回，帧由 worker 交付（engine/rendering 的 VideoAsyncFrameSource）。

两个刻意的设计：

	惰性启动 + 空闲自退：没有请求时不留任何 goroutine（测试与嵌入式用法都不会
	  泄漏，宿主也不必持有一个 Close 生命周期）；
	队列满丢最旧：过期的时刻没有显示价值——播放中堆积旧时刻比丢帧更糟。
*/

const (
	// framePumpWorkers 是并发跑解码器的 worker 上限。抽帧是进程调用，2 个足够
	// 跟上 250ms 的播放时钟步长，又不至于让一屏 <video> 同时把 CPU 打满。
	framePumpWorkers = 2
	// framePumpQueue 是待抽帧任务的队列容量（满了丢最旧的）。
	framePumpQueue = 8
	// framePumpIdle 是 worker 的空闲存活时长：空闲即退出。
	framePumpIdle = 3 * time.Second
)

// frameJob 是一次抽帧任务（渲染层已经解析好的本地路径与时刻）。
type frameJob struct {
	path    string
	at      float64
	deliver func([]byte, bool)
}

// framePump 把抽帧搬离渲染线程。
type framePump struct {
	probe *MediaProbe
	jobs  chan frameJob

	mu   sync.Mutex
	live int // 当前活跃的 worker 数（惰性启动、空闲自减）
}

func newFramePump(probe *MediaProbe) *framePump {
	return &framePump{probe: probe, jobs: make(chan frameJob, framePumpQueue)}
}

// push 提交任务。★ 无论走哪条路径都必须**恰好交付一次**：渲染层按「已提交」
// 去重未交付的键，漏交就会让那个时刻的帧永远缺席（一直显示上一帧）。
func (fp *framePump) push(j frameJob) {
	if fp == nil {
		j.deliver(nil, false)
		return
	}
	select {
	case fp.jobs <- j:
	default:
		// 队列满：丢最旧的一个（它的时刻已经过期）。被丢的任务同样要交付失败。
		select {
		case dropped := <-fp.jobs:
			dropped.deliver(nil, false)
		default:
		}
		select {
		case fp.jobs <- j:
		default:
			j.deliver(nil, false)
			return
		}
	}
	fp.startWorkers()
}

// startWorkers 按需拉起 worker（补足到 framePumpWorkers 个）。
func (fp *framePump) startWorkers() {
	fp.mu.Lock()
	for i := fp.live; i < framePumpWorkers; i++ {
		fp.live++
		go fp.worker()
	}
	fp.mu.Unlock()
}

// worker 取任务 → 抽帧 → 交付；空闲超过 framePumpIdle 后自行退出。
func (fp *framePump) worker() {
	idle := time.NewTimer(framePumpIdle)
	defer idle.Stop()
	for {
		select {
		case j := <-fp.jobs:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(framePumpIdle)
			data, ok := fp.probe.FrameAt(j.path, j.at)
			j.deliver(data, ok)
		case <-idle.C:
			fp.mu.Lock()
			fp.live--
			fp.mu.Unlock()
			return
		}
	}
}

// AsyncFrameSourceFor 返回可直接交给 rendering.SetVideoAsyncFrameSource 的异步
// 帧源：解析 src → 提交给 framePump（立刻返回），帧由 worker 交付。
//
// 解析失败（非本地文件 / data:/http(s)/blob:）时**同步**交付失败——这类源不可能
// 抽到帧，没必要占用 worker。
func (p *MediaProbe) AsyncFrameSourceFor(baseURL func() string) rendering.VideoAsyncFrameSource {
	return func(src string, atSeconds float64, deliver func([]byte, bool)) {
		if p == nil {
			deliver(nil, false)
			return
		}
		base := ""
		if baseURL != nil {
			base = baseURL()
		}
		path, ok := mediaSrcToPath(src, base)
		if !ok {
			deliver(nil, false)
			return
		}
		if p.pump == nil {
			// 手工构造的 MediaProbe（测试/嵌入式用法）：退化成同步抽帧。
			data, ok := p.FrameAt(path, atSeconds)
			deliver(data, ok)
			return
		}
		p.pump.push(frameJob{path: path, at: atSeconds, deliver: deliver})
	}
}
