// Package bindings — 媒体元素的音频播放接线（主线 A3）。
//
// 这一层只做三件事，其余全在宿主：
//
//	① 打开/关闭会话：play() 时按 src 请宿主打开（宿主在那里起 ffmpeg 解码与
//	   waveOut 输出），pause/切源/卸载时关掉；
//	② **音频为主时钟**：播放中 currentTime 直接取会话 Position()，不再自己数
//	   挂钟步长（规范与所有浏览器的做法；挂钟与声卡时钟会漂移）；
//	③ 回退：没有音频会话（纯视频 / 宿主没注册音频源 / 打开失败）时，一切照旧走
//	   media_element.go 的挂钟时钟——引擎独立使用、单测默认行为都不受影响。
//
// 为什么时钟必须改成「问会话要位置」：挂在 tick 上的固定步进累加看着也能播完
// 1 秒的样本，但两处会立刻失真——①设备的启动延迟（waveOut 打开到真正出声有
// 几十毫秒，挂钟已经先跑了）；②声卡时钟与系统时钟的漂移（几十 ppm 的晶振差异
// 在 1 秒内看不出来，在长音轨上会让音画不同步肉眼可见）。以输出位置为唯一真相
// 后，currentTime 与「耳朵听到的位置」严格一致，这就是 TC-M-602 要的定点判据。
package bindings

import (
	"math"
	"strings"

	"wb-ui/engine/rendering"
)

// mediaAudioTickMs 是**音频驱动**时的时钟推进步长（毫秒）。
//
// 比挂钟模式（mediaTimeUpdateMs = 250ms）细得多：音频模式下一步就是一次
// timeupdate，250ms 的粒度会让脚本读到的 currentTime 以 0.25 秒为台阶跳
// （规范只要求 timeupdate 频率「不低于 4Hz」，细一些完全合规）。取 50ms 是因为
// 它同时是「设备缓冲深度」的量级（宿主每个输出缓冲 100ms），再细就只是空转。
const mediaAudioTickMs = 50

// mediaAudioEndEpsilonSec 是判定「播到末尾」的容差（秒）：设备位置是整数采样
// 帧换算来的，末帧的边界上难免差一两个采样（< 0.1ms），但宿主可能因解码器
// 提前 EOF 而少交付几毫秒——容差取一帧时钟步长的量级足够，又不至于把「还差
// 200ms」误判成结束（那会让 ended 早触发、脚本提前切下一首）。
const mediaAudioEndEpsilonSec = 0.02

// mediaAudioState 是元素上的一次音频会话。
type mediaAudioState struct {
	session rendering.AudioSession
	// totalSeconds 是会话的总时长（秒；0 = 未知）。优先取会话的采样帧总数
	// （解码侧的真实长度），退化到元素元数据的 duration。
	totalSeconds float64
}

// startAudio 为当前源打开音频会话（从 fromSeconds 开始播）。
//
// 调用点：play()。宿主没注册音频源（引擎独立使用）、源为空、宿主拒绝（纯视频
// 资源没有音轨）时安静返回——这不是错误，只是「这个元素没有音频时钟」。
func (st *mediaElementState) startAudio(fromSeconds float64) {
	st.closeAudio()
	// 元数据表明资源没有音轨（宿主解析容器头得到）就直接不打开：引擎不能给纯
	// 视频资源挂一条永不推进的音频时钟（currentTime 会钉死在 0）。
	if !st.hasAudio {
		return
	}
	if st.el == nil || !rendering.AudioSessionSourceRegistered() {
		return
	}
	src := st.effectiveSrc()
	if strings.TrimSpace(src) == "" {
		return
	}
	sess, ok := rendering.OpenAudioSession(src, fromSeconds)
	if !ok || sess == nil {
		return
	}
	st.audio = &mediaAudioState{session: sess, totalSeconds: st.audioTotalSeconds(sess)}
}

// audioTotalSeconds 取会话总时长：优先采样帧总数（解码侧的真实长度，不受容器
// 头是否准确影响），否则退回元素元数据的 duration（宿主探测的容器时长）。
func (st *mediaElementState) audioTotalSeconds(sess rendering.AudioSession) float64 {
	if sess == nil {
		return 0
	}
	if n := sess.TotalFrames(); n > 0 {
		if d := sess.Format().Duration(n); d > 0 {
			return d
		}
	}
	if !math.IsNaN(st.duration) && st.duration > 0 {
		return st.duration
	}
	return 0
}

// closeAudio 关闭会话（幂等）。切源、load()、ended、页面卸载都会走到这里。
func (st *mediaElementState) closeAudio() {
	if st.audio == nil {
		return
	}
	if st.audio.session != nil {
		_ = st.audio.session.Close()
	}
	st.audio = nil
}

// pauseAudio 暂停输出（无会话时 no-op）。
func (st *mediaElementState) pauseAudio() {
	if st.audio == nil || st.audio.session == nil {
		return
	}
	_ = st.audio.session.Pause()
}

// resumeAudio 恢复输出（无会话时 no-op）。pause() 之后又 play() 时用。
func (st *mediaElementState) resumeAudio() {
	if st.audio == nil || st.audio.session == nil {
		return
	}
	_ = st.audio.session.Resume()
}

// seekAudio 把输出位置移到 seconds（无会话时 no-op）。
func (st *mediaElementState) seekAudio(seconds float64) {
	if st.audio == nil || st.audio.session == nil {
		return
	}
	if seconds < 0 {
		seconds = 0
	}
	_ = st.audio.session.Seek(seconds)
}

// audioPosition 返回输出侧已播放的媒体时间（秒）与「会话是否可用」。
func (st *mediaElementState) audioPosition() (float64, bool) {
	if st.audio == nil || st.audio.session == nil {
		return 0, false
	}
	p := st.audio.session.Position()
	if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 {
		return 0, false
	}
	return p, true
}

// audioHasOutput 报告当前会话是否接了真实输出设备（支持矩阵与报告用）。
func (st *mediaElementState) audioHasOutput() bool {
	if st.audio == nil || st.audio.session == nil {
		return false
	}
	return st.audio.session.HasOutput()
}

// audioActive 报告「时钟是否由音频驱动」——tick 据此在两个时钟之间二选一。
func (st *mediaElementState) audioActive() bool {
	if st.audio == nil || st.audio.session == nil {
		return false
	}
	// 会话自己报告「这个源没有可播的音频」（无音轨/解码器失败）时不占用时钟：
	// tick 会摘掉会话并退回挂钟。
	if st.audio.session.Failed() {
		return false
	}
	_, ok := st.audioPosition()
	return ok
}

// audioDurationSeconds 返回音频会话的时长（秒；0 = 未知）。会话的采样帧总数最
// 权威（解码侧长度），其次才是元素元数据里的容器时长。
func (st *mediaElementState) audioDurationSeconds() float64 {
	if st.audio != nil && st.audio.totalSeconds > 0 {
		return st.audio.totalSeconds
	}
	if !math.IsNaN(st.duration) && st.duration > 0 {
		return st.duration
	}
	return 0
}
